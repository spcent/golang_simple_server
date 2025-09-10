package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
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

// Middleware is a wrapper around http.RoundTripper
type Middleware func(next http.RoundTripper) http.RoundTripper

// chainMiddlewares applies middleware in order
func chainMiddlewares(rt http.RoundTripper, mws []Middleware) http.RoundTripper {
	// Apply in reverse order (first added, first executed)
	for i := len(mws) - 1; i >= 0; i-- {
		rt = mws[i](rt)
	}
	return rt
}

// HttpClient is a wrapper around http.Client with retry, timeout, and backoff support.
type HttpClient struct {
	client         *http.Client
	retryCount     int
	retryWait      time.Duration
	maxRetryWait   time.Duration
	retryPolicy    RetryPolicy
	defaultTimeout time.Duration
	middlewares    []Middleware
}

// Option defines a functional option for HttpClient
type Option func(*HttpClient)

// WithTimeout sets the client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(hc *HttpClient) {
		hc.client.Timeout = timeout
		hc.defaultTimeout = timeout
	}
}

// WithRetryCount sets the maximum retry attempts.
func WithRetryCount(count int) Option {
	return func(hc *HttpClient) {
		hc.retryCount = count
	}
}

// WithRetryWait sets the base retry wait duration.
func WithRetryWait(wait time.Duration) Option {
	return func(hc *HttpClient) {
		hc.retryWait = wait
	}
}

// WithMaxRetryWait sets the maximum retry wait duration.
func WithMaxRetryWait(max time.Duration) Option {
	return func(hc *HttpClient) {
		hc.maxRetryWait = max
	}
}

// WithRetryPolicy sets a custom retry policy.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(hc *HttpClient) {
		hc.retryPolicy = policy
	}
}

func WithMiddleware(mw Middleware) Option {
	return func(hc *HttpClient) { hc.middlewares = append(hc.middlewares, mw) }
}

// NewHttpClient creates a new HttpClient with provided options.
func NewHttpClient(opts ...Option) *HttpClient {
	hc := &HttpClient{
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		retryCount:     3,
		retryWait:      1 * time.Second,
		maxRetryWait:   5 * time.Second,
		retryPolicy:    TimeoutRetryPolicy{},
		defaultTimeout: 3 * time.Second,
	}
	for _, opt := range opts {
		opt(hc)
	}

	// apply middleware to transport
	hc.client.Transport = chainMiddlewares(http.DefaultTransport, hc.middlewares)
	return hc
}

type requestConfig struct {
	retryCount  *int
	retryPolicy RetryPolicy
	timeout     *time.Duration
}

type RequestOption func(*requestConfig)

func WithRequestTimeout(timeout time.Duration) RequestOption {
	return func(c *requestConfig) { c.timeout = &timeout }
}
func WithRequestRetryCount(count int) RequestOption {
	return func(c *requestConfig) { c.retryCount = &count }
}
func WithRequestRetryPolicy(p RetryPolicy) RequestOption {
	return func(c *requestConfig) { c.retryPolicy = p }
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
func (hc *HttpClient) doRequest(req *http.Request, opts ...RequestOption) (*http.Response, error) {
	cfg := &requestConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// effective config = request override > client default
	retryCount := hc.retryCount
	if cfg.retryCount != nil {
		retryCount = *cfg.retryCount
	}
	retryPolicy := hc.retryPolicy
	if cfg.retryPolicy != nil {
		retryPolicy = cfg.retryPolicy
	}
	timeout := hc.defaultTimeout
	if cfg.timeout != nil {
		timeout = *cfg.timeout
	}

	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	var lastErr error
	var resp *http.Response
	for i := 0; i <= retryCount; i++ {
		resp, lastErr = hc.client.Do(req)
		if lastErr == nil && (resp.StatusCode < 500) {
			return resp, nil
		}

		if retryPolicy == nil || !retryPolicy.ShouldRetry(resp, lastErr, i) {
			break
		}

		wait := backoffWithJitter(hc.retryWait, i, hc.maxRetryWait)
		time.Sleep(wait)
	}
	return resp, lastErr
}

// Get performs a GET request.
func (hc *HttpClient) Get(ctx context.Context, url string, opts ...RequestOption) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.doRequest(req, opts...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Post performs a POST request.
func (hc *HttpClient) Post(ctx context.Context, url string, body []byte, contentType string, opts ...RequestOption) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := hc.doRequest(req, opts...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, errors.New("http error: " + resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// PostJson performs a POST request with JSON body.
func (hc *HttpClient) PostJson(ctx context.Context, url string, data any, opts ...RequestOption) ([]byte, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return hc.Post(ctx, url, body, "application/json", opts...)
}
