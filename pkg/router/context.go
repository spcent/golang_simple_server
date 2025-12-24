package router

import (
	"context"
	"net/http"
)

// RequestContext contains request-scoped data that should be shared across middleware and handlers.
// It preserves compatibility with the standard library by living inside the request's context.
type RequestContext struct {
	Params map[string]string
}

type requestContextKey struct{}

// RequestContextFrom returns the RequestContext stored in the given context.
// If none is present, it falls back to parameters stored via ParamsFromContext for backward compatibility.
func RequestContextFrom(ctx context.Context) RequestContext {
	if ctx == nil {
		return RequestContext{}
	}

	if rc, ok := ctx.Value(requestContextKey{}).(RequestContext); ok {
		return rc
	}

	return RequestContext{Params: ParamsFromContext(ctx)}
}

// Param returns a single path parameter from the request's context.
// The boolean indicates whether the parameter was present.
func Param(r *http.Request, key string) (string, bool) {
	rc := RequestContextFrom(r.Context())
	if rc.Params == nil {
		return "", false
	}
	val, ok := rc.Params[key]
	return val, ok
}
