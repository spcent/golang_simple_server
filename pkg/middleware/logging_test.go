package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLogging(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		handlerDelay   time.Duration
		expectedMethod string
		expectedPath   string
	}{
		{
			name:           "GET request to root path",
			method:         "GET",
			path:           "/",
			handlerDelay:   0,
			expectedMethod: "GET",
			expectedPath:   "/",
		},
		{
			name:           "POST request to API endpoint",
			method:         "POST",
			path:           "/api/users",
			handlerDelay:   0,
			expectedMethod: "POST",
			expectedPath:   "/api/users",
		},
		{
			name:           "PUT request with query parameters",
			method:         "PUT",
			path:           "/users/123?update=true",
			handlerDelay:   0,
			expectedMethod: "PUT",
			expectedPath:   "/users/123", // URL.Path doesn't include query params
		},
		{
			name:           "DELETE request with slow handler",
			method:         "DELETE",
			path:           "/users/456",
			handlerDelay:   50 * time.Millisecond,
			expectedMethod: "DELETE",
			expectedPath:   "/users/456",
		},
		{
			name:           "PATCH request to long path",
			method:         "PATCH",
			path:           "/api/v1/organizations/123/users/456/profile",
			handlerDelay:   0,
			expectedMethod: "PATCH",
			expectedPath:   "/api/v1/organizations/123/users/456/profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout to test logging output
			oldStdout := os.Stdout
			reader, writer, _ := os.Pipe()
			os.Stdout = writer

			// Create a channel to capture the output
			outputChan := make(chan string)
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, reader)
				outputChan <- buf.String()
			}()

			// Mock handler that simulates processing time
			mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handlerDelay > 0 {
					time.Sleep(tt.handlerDelay)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("response"))
			})

			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			// Create and execute logging middleware
			loggingMiddleware := Logging(mockHandler)
			start := time.Now()
			loggingMiddleware(w, req)
			elapsed := time.Since(start)

			// Restore stdout and get captured output
			os.Stdout = oldStdout
			writer.Close()
			output := <-outputChan

			// Verify the handler was called successfully
			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			body := w.Body.String()
			if body != "response" {
				t.Errorf("Expected body 'response', got %q", body)
			}

			// Verify log output format
			// Expected format: [HH:MM:SS] METHOD PATH (duration)
			logPattern := fmt.Sprintf(`^\[\d{2}:\d{2}:\d{2}\] %s %s \(\d+.*\n$`,
				regexp.QuoteMeta(tt.expectedMethod),
				regexp.QuoteMeta(tt.expectedPath))

			matched, err := regexp.MatchString(logPattern, output)
			if err != nil {
				t.Fatalf("Regex compilation error: %v", err)
			}

			if !matched {
				t.Errorf("Log output doesn't match expected pattern.\nExpected pattern: %s\nActual output: %q", logPattern, output)
			}

			// Verify the log contains correct method and path
			if !strings.Contains(output, tt.expectedMethod) {
				t.Errorf("Log output should contain method %q, got: %q", tt.expectedMethod, output)
			}

			if !strings.Contains(output, tt.expectedPath) {
				t.Errorf("Log output should contain path %q, got: %q", tt.expectedPath, output)
			}

			// Verify timing makes sense (should be at least as long as handler delay)
			if tt.handlerDelay > 0 {
				if elapsed < tt.handlerDelay {
					t.Errorf("Total elapsed time (%v) should be at least handler delay (%v)", elapsed, tt.handlerDelay)
				}
			}

			// Verify timestamp format (HH:MM:SS)
			timestampPattern := `\[\d{2}:\d{2}:\d{2}\]`
			timestampMatched, _ := regexp.MatchString(timestampPattern, output)
			if !timestampMatched {
				t.Errorf("Log should contain timestamp in [HH:MM:SS] format, got: %q", output)
			}
		})
	}
}

// Test that logging middleware doesn't interfere with response
func TestLoggingPassthrough(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	reader, writer, _ := os.Pipe()
	os.Stdout = writer

	outputChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, reader)
		outputChan <- buf.String()
	}()

	// Handler that sets custom headers and status
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Custom-Header", "test-value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("custom response"))
	})

	req := httptest.NewRequest("POST", "/test", nil)
	w := httptest.NewRecorder()

	loggingMiddleware := Logging(mockHandler)
	loggingMiddleware(w, req)

	// Restore stdout
	os.Stdout = oldStdout
	writer.Close()
	<-outputChan // Drain output

	// Verify response is unchanged
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	if w.Header().Get("Custom-Header") != "test-value" {
		t.Errorf("Custom header not preserved")
	}

	if w.Body.String() != "custom response" {
		t.Errorf("Response body not preserved")
	}
}

// Test concurrent logging
func TestLoggingConcurrent(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outputChan <- buf.String()
	}()

	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Small delay to make timing more realistic
		time.Sleep(1 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	loggingMiddleware := Logging(mockHandler)

	const numRequests = 50
	done := make(chan bool, numRequests)

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", fmt.Sprintf("/test/%d", id), nil)
			w := httptest.NewRecorder()
			loggingMiddleware(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Request %d: expected status 200, got %d", id, w.Code)
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}

	// Restore stdout and get output
	os.Stdout = oldStdout
	w.Close()
	output := <-outputChan

	// Count log lines (should be equal to number of requests)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != numRequests {
		t.Errorf("Expected %d log lines, got %d", numRequests, len(lines))
	}

	logPattern := regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\] GET /test/\d+ \(\d+.*$`)
	// Verify each line is properly formatted
	for i, line := range lines {
		if line == "" {
			continue
		}

		matched := logPattern.MatchString(line)
		if !matched {
			t.Errorf("Line %d doesn't match expected format: %q", i, line)
		}
	}
}

// Benchmark the logging middleware
func BenchmarkLogging(b *testing.B) {
	// Discard stdout for benchmarking
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	loggingMiddleware := Logging(mockHandler)
	req := httptest.NewRequest("GET", "/benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		loggingMiddleware(w, req)
	}
}
