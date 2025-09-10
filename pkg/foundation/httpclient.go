package foundation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"time"
)

// HttpClient is a wrapper around http.Client with retry and timeout support.
type HttpClient struct {
	client        *http.Client
	retryCount    int           // maximum retry attempts
	retryWait     time.Duration // initial retry wait duration
	maxRetryWait  time.Duration // maximum backoff wait duration
	defaultTimeout time.Duration
}

// NewHttpClient creates a new HttpClient with timeout and retry configuration.
func NewHttpClient(timeout time.Duration, retryCount int, retryWait, maxRetryWait time.Duration) *HttpClient {
	return &HttpClient{
		client: &http.Client{
			Timeout: timeout,
		},
		retryCount:     retryCount,
		retryWait:      retryWait,
		maxRetryWait:   maxRetryWait,
		defaultTimeout: timeout,
	}
}

// doRequest executes an HTTP request with retry and exponential backoff.
func (hc *HttpClient) doRequest(req *http.Request) (*http.Response, error) {
	var lastErr error
	wait := hc.retryWait

	for i := 0; i <= hc.retryCount; i++ {
		resp, err := hc.client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// If not the last attempt, wait with exponential backoff
		if i < hc.retryCount {
			time.Sleep(wait)
			// Exponential backoff: double the wait, capped at maxRetryWait
			wait = time.Duration(math.Min(float64(wait*2), float64(hc.maxRetryWait)))
		}
	}
	return nil, lastErr
}

// Get performs a GET request with retry and timeout.
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

// Post performs a POST request with retry and timeout.
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
