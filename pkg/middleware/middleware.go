package middleware

import (
	"net/http"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

var middlewares []Middleware

// Global middleware
func Use(middleware Middleware) {
	middlewares = append(middlewares, middleware)
}

// Apply applies all middlewares to the handler
func Apply(h http.HandlerFunc, m ...Middleware) http.HandlerFunc {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}

	return h
}

// ApplyGlobal applies all global middlewares to the handler
func ApplyGlobal(h http.HandlerFunc) http.HandlerFunc {
	return Apply(h, middlewares...)
}
