package middleware

import (
	"net/http"
	"os"
)

// Auth middleware (requires Header: X-Token: secret)
func Auth(next http.HandlerFunc) http.HandlerFunc {
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
