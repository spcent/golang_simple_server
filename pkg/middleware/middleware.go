package middleware

import (
	"net/http"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

var middlewares []Middleware

func Use(middleware Middleware) {
	middlewares = append(middlewares, middleware)
}

func Apply(h http.HandlerFunc, m ...Middleware) http.HandlerFunc {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}

	for _, m := range m {
		h = m(h)
	}

	return h
}
