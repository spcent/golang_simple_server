package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRecoveryMiddleware(t *testing.T) {
	tests := []struct {
		name                string
		handler             http.Handler
		shouldPanic         bool
		panicValue          any
		expectedStatus      int
		expectedLogContains string
	}{
		{
			name: "Normal handler execution - no panic",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			}),
			shouldPanic:    false,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Handler panics with string",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("something went wrong")
			}),
			shouldPanic:         true,
			panicValue:          "something went wrong",
			expectedStatus:      http.StatusInternalServerError,
			expectedLogContains: "panic recovered: something went wrong",
		},
		{
			name: "Handler panics with error",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic(http.ErrMissingFile)
			}),
			shouldPanic:         true,
			panicValue:          http.ErrMissingFile,
			expectedStatus:      http.StatusInternalServerError,
			expectedLogContains: "panic recovered: http: no such file",
		},
		{
			name: "Handler panics with integer",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic(42)
			}),
			shouldPanic:         true,
			panicValue:          42,
			expectedStatus:      http.StatusInternalServerError,
			expectedLogContains: "panic recovered: 42",
		},
		{
			name: "Handler panics with custom struct",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic(struct{ Msg string }{"custom error"})
			}),
			shouldPanic:         true,
			panicValue:          struct{ Msg string }{"custom error"},
			expectedStatus:      http.StatusInternalServerError,
			expectedLogContains: "panic recovered: {custom error}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var logBuffer bytes.Buffer
			log.SetOutput(&logBuffer)
			defer func() {
				log.SetOutput(os.Stderr) // Restore default log output
			}()

			// Create request and response recorder
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			// Wrap handler with recovery middleware
			recoveryHandler := RecoveryMiddleware(tt.handler)

			// Execute the handler
			recoveryHandler.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check log output for panics
			logOutput := logBuffer.String()
			if tt.shouldPanic {
				if tt.expectedLogContains != "" && !strings.Contains(logOutput, tt.expectedLogContains) {
					t.Errorf("Expected log to contain %q, got: %q", tt.expectedLogContains, logOutput)
				}

				// Verify JSON response structure
				var response Response
				err := json.Unmarshal(w.Body.Bytes(), &response)
				if err != nil {
					t.Errorf("Failed to unmarshal JSON response: %v", err)
				}

				if response.Code != http.StatusInternalServerError {
					t.Errorf("Expected response code %d, got %d", http.StatusInternalServerError, response.Code)
				}

				if response.Msg != "internal server error" {
					t.Errorf("Expected response message 'internal server error', got %q", response.Msg)
				}

				// Check Content-Type header
				contentType := w.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type 'application/json', got %q", contentType)
				}
			} else {
				// No panic should occur, log should be empty
				if logOutput != "" {
					t.Errorf("Expected no log output for normal execution, got: %q", logOutput)
				}

				// Check that original response is preserved
				body := w.Body.String()
				if body != "success" {
					t.Errorf("Expected body 'success', got %q", body)
				}
			}
		})
	}
}

// Test that recovery middleware doesn't interfere with normal responses
func TestRecoveryMiddleware_NormalFlow(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	// Handler that sets custom headers and status
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Custom-Header", "test-value")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("custom response"))
	})

	req := httptest.NewRequest("POST", "/test", nil)
	w := httptest.NewRecorder()

	recoveryHandler := RecoveryMiddleware(handler)
	recoveryHandler.ServeHTTP(w, req)

	// Verify response is unchanged
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	if w.Header().Get("Custom-Header") != "test-value" {
		t.Errorf("Custom header not preserved")
	}

	if w.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("Content-Type header not preserved")
	}

	if w.Body.String() != "custom response" {
		t.Errorf("Response body not preserved")
	}

	// No log output should occur
	if logBuffer.String() != "" {
		t.Errorf("Expected no log output, got: %q", logBuffer.String())
	}
}

// Test concurrent panic handling
func TestRecoveryMiddleware_Concurrent(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	// Handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("concurrent panic")
	})

	recoveryHandler := RecoveryMiddleware(panicHandler)

	const numRequests = 10
	done := make(chan bool, numRequests)

	// Launch concurrent requests that will panic
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			recoveryHandler.ServeHTTP(w, req)

			// Verify each request is handled properly
			if w.Code != http.StatusInternalServerError {
				t.Errorf("Request %d: expected status 500, got %d", id, w.Code)
			}

			var response Response
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Request %d: failed to unmarshal response: %v", id, err)
			}

			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}

	// Check that all panics were logged
	logOutput := logBuffer.String()
	panicCount := strings.Count(logOutput, "panic recovered: concurrent panic")
	if panicCount != numRequests {
		t.Errorf("Expected %d panic log entries, got %d", numRequests, panicCount)
	}
}

// Test that middleware properly handles different HTTP methods
func TestRecoveryMiddleware_DifferentMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			// Capture log output
			var logBuffer bytes.Buffer
			log.SetOutput(&logBuffer)
			defer func() {
				log.SetOutput(os.Stderr)
			}()

			panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("method panic")
			})

			req := httptest.NewRequest(method, "/test", nil)
			w := httptest.NewRecorder()

			recoveryHandler := RecoveryMiddleware(panicHandler)
			recoveryHandler.ServeHTTP(w, req)

			// All methods should be handled the same way
			if w.Code != http.StatusInternalServerError {
				t.Errorf("Method %s: expected status 500, got %d", method, w.Code)
			}

			logOutput := logBuffer.String()
			if !strings.Contains(logOutput, "panic recovered: method panic") {
				t.Errorf("Method %s: expected panic log, got: %q", method, logOutput)
			}
		})
	}
}

// Benchmark the recovery middleware
func BenchmarkRecoveryMiddleware_NoPanic(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	recoveryHandler := RecoveryMiddleware(handler)
	req := httptest.NewRequest("GET", "/benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		recoveryHandler.ServeHTTP(w, req)
	}
}

func BenchmarkRecoveryMiddleware_WithPanic(b *testing.B) {
	// Discard log output for benchmarking
	log.SetOutput(io.Discard)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("benchmark panic")
	})

	recoveryHandler := RecoveryMiddleware(panicHandler)
	req := httptest.NewRequest("GET", "/benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		recoveryHandler.ServeHTTP(w, req)
	}
}
