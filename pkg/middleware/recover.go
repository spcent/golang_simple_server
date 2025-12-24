package middleware

import (
	"net/http"

	log "github.com/spcent/golang_simple_server/pkg/log"
)

// recover middleware
// recover from panic and return 500 internal server error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				WriteError(w, r, APIError{
					Status:   http.StatusInternalServerError,
					Code:     "internal_error",
					Category: CategoryServer,
					Message:  "internal server error",
					Details:  map[string]any{"panic": rec},
				})
				logger := log.NewGLogger()
				logger.WithFields(log.Fields{"panic": rec, "trace_id": TraceIDFromContext(r.Context())}).Error("panic recovered", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
