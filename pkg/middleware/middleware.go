package middleware

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

func ApplyMiddleware(h http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for _, m := range middlewares {
		h = m(h)
	}
	return h
}

func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		fmt.Printf("[%s] %s %s (%s)\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start))
	}
}

// Auth middleware (requires Header: X-Token: secret)
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Token")
		authToken := os.Getenv("AUTH_TOKEN")
		if authToken != "" && token != authToken {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next(w, r)
	}
}
