package middleware

import (
	"fmt"
	"net/http"
	"time"
)

func Logging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		fmt.Printf("[%s] %s %s (%s)\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start))
	}
}
