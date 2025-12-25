package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/router"
)

func RegisterRoutes(r *router.Router) {
	r.GetFunc("/hello/:name", func(w http.ResponseWriter, r *http.Request) {
		name, _ := contract.Param(r, "name")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello, %s!"}`, name)))
	})

	r.GetFunc("/users/:id/posts/:postID", func(w http.ResponseWriter, r *http.Request) {
		id, _ := contract.Param(r, "id")
		postID, _ := contract.Param(r, "postID")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"user": "%s", "post": "%s"}`, id, postID)))
	})

	r.Register(&HealthHandler{}, &UserHandler{}, &WebhookInHandler{})
	// r.Register(&WebhookHandler{Svc: webhook.NewWebhookService()})
}

/*
|--------------------------------------------------------------------------
| Small JSON helpers (best-effort, no schema dependency)
|--------------------------------------------------------------------------
*/

func extractJSONFieldString(raw []byte, key string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func extractJSONFieldBool(raw []byte, key string) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func extractJSONPathString(raw []byte, objKey string, fieldKey string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	obj, ok := m[objKey].(map[string]any)
	if !ok || obj == nil {
		return ""
	}
	v, ok := obj[fieldKey]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	n := len(s)
	if n <= 4 {
		return strings.Repeat("*", n)
	}
	return s[:2] + strings.Repeat("*", n-4) + s[n-2:]
}
