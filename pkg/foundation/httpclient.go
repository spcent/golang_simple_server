package foundation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"
)

// RetryPolicy defines the interface for retry strategy.
type RetryPolicy interface {
	ShouldRetry(resp *http.Response, err error, attempt int) bool
}

// TimeoutRetryPolicy retries only on timeout errors.
type TimeoutRetryPolicy struct{}

func (p TimeoutRetryPolicy) ShouldRetry(resp *http.Response, err error, attempt int) bool {
	return isTimeoutError(err)
}

// StatusCodeRetryPolicy retries on specific HTTP status codes (e.g., 5xx).
type StatusCodeRetryPolicy struct {
	Codes []int
}

func (p StatusCodeRetryPolicy) ShouldRetry(resp *http.Response, err error, attempt int) bool {
	if resp == nil {
		return false
	}
	for _, code := range p.Codes {
		if resp.StatusCode == code {
			return true
		}
	}
	return false
}

// CompositeRetryPolicy combines multiple retry policies with OR logic.
type CompositeRetryPolicy struct {
	Policies []RetryPolicy
}

func (p CompositeRetryPolicy) ShouldRetry(resp *http.Response, err error, attempt int) bool {
	for _, policy := range p.Policies {
		if policy.ShouldRetry(resp, err, attempt) {
			return true
		}
	}
	return false
}

// HttpClient is a wrapper around http.Client with retry, timeout, and backoff support.
type HttpClient struct {
	client         *http.Client
	retryCount     int
	retryWait      time.Duration
	maxRetryWait   time.Duration
	retryPolicy    RetryPolicy
	defaultTimeout time.Duration
}

// NewHttpClient creates a new HttpClient with retry policy support.
func NewHttpClient(timeout time.Duration, retryCount int, retryWait, maxRetryWait time.Duration, policy RetryPolicy) *HttpClient {
	rand.Seed(time.Now().UnixNano())
	return &HttpClient{
		client: &http.Client{
			Timeout: timeout,
		},
		retryCount:     retryCount,
		retryWait:      retryWait,
		maxRetryWait:   maxRetryWait,
		retryPolicy:    policy,
		defaultTimeout: timeout,
	}
}

// isTimeoutError checks if an error is caused by timeout.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if strings.Contains(err.Error(), "timeout") {
		return true
	}
	return false
}

// backoffWithJitter calculates exponential backoff with jitter.
func backoffWithJitter(base time.Duration, attempt int, max time.Duration) time.Duration {
	backoff := float64(base) * math.Pow(2, float64(attempt))
	if backoff > float64(max) {
		backoff = float64(max)
	}
	jitterFactor := 0.5 + rand.Float64() // [0.5, 1.5)
	return time.Duration(backoff * jitterFactor)
}

// doRequest executes an HTTP request with retry and policy control.
func (hc *HttpClient) doRequest(req *http.Request) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for i := 0; i <= hc.retryCount; i++ {
		resp, lastErr = hc.client.Do(req)
		if lastErr == nil && (resp.StatusCode < 500) {
			// success (non-5xx) -> return immediately
			return resp, nil
		}

		// check retry policy
		if hc.retryPolicy == nil || !hc.retryPolicy.ShouldRetry(resp, lastErr, i) {
			break
		}

		// backoff with jitter before next attempt
		wait := backoffWithJitter(hc.retryWait, i, hc.maxRetryWait)
		time.Sleep(wait)
	}
	return resp, lastErr
}

// Get performs a GET request.
func (hc *HttpClient) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Post performs a POST request.
func (hc *HttpClient) Post(ctx context.Context, url string, body []byte, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := hc.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, errors.New("http error: " + resp.Status)
	}
	return io.ReadAll(resp.Body)
}
