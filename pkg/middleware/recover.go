package middleware

import (
	"log"
	"net/http"
)

// recover middleware
// recover from panic and return 500 internal server error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				JSON(w, http.StatusInternalServerError, Response{
					Code: http.StatusInternalServerError,
					Msg:  "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
