package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/router"
	"github.com/spcent/golang_simple_server/pkg/webhookin"
)

type WebhookInHandler struct {
	// Optional: override secrets via struct fields (useful for tests)
	GitHubSecret string
	StripeSecret string

	// Stripe replay tolerance; default 5 minutes if zero.
	StripeTolerance time.Duration

	// Optional: max body size; default 1MB if zero.
	MaxBodyBytes int64
}

func (h *WebhookInHandler) Register(r *router.Router) {
	// Inbound webhooks
	r.PostCtx("/webhooks/in/github", h.GitHub)
	r.PostCtx("/webhooks/in/stripe", h.Stripe)
}

/*
|--------------------------------------------------------------------------
| GitHub Inbound Webhook
|--------------------------------------------------------------------------
| - Validates X-Hub-Signature-256
| - Reads body once (VerifyGitHub consumes body)
| - Returns event info summary
*/

func (h *WebhookInHandler) GitHub(ctx *contract.Ctx) {
	secret := strings.TrimSpace(h.GitHubSecret)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	}
	if secret == "" {
		ctx.ErrorJSON(http.StatusInternalServerError, "missing_secret", "GITHUB_WEBHOOK_SECRET is not configured", nil)
		return
	}

	maxBody := h.maxBody()

	body, err := webhookin.VerifyGitHub(ctx.R, secret, maxBody)
	if err != nil {
		// signature invalid or body read error
		ctx.ErrorJSON(http.StatusUnauthorized, "invalid_signature", "invalid GitHub signature", map[string]any{
			"provider": "github",
		})
		return
	}

	ghEvent := strings.TrimSpace(ctx.Headers.Get("X-GitHub-Event"))
	ghDelivery := strings.TrimSpace(ctx.Headers.Get("X-GitHub-Delivery"))

	// Parse common fields (best-effort)
	eventID := extractJSONFieldString(body, "id") // not always present for all GH events
	repoFullName := extractJSONPathString(body, "repository", "full_name")
	action := extractJSONFieldString(body, "action")

	ctx.JSON(http.StatusOK, map[string]any{
		"ok":          true,
		"provider":    "github",
		"event_type":  ghEvent,
		"delivery_id": ghDelivery,
		"event_id":    eventID,
		"repo":        repoFullName,
		"action":      action,
		"trace_id":    ctx.TraceID,
		"body_bytes":  len(body),
	})
}

/*
|--------------------------------------------------------------------------
| Stripe Inbound Webhook
|--------------------------------------------------------------------------
| - Validates Stripe-Signature
| - Enforces tolerance window (replay protection)
| - Returns Stripe event summary
*/

func (h *WebhookInHandler) Stripe(ctx *contract.Ctx) {
	secret := strings.TrimSpace(h.StripeSecret)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	}
	if secret == "" {
		ctx.ErrorJSON(http.StatusInternalServerError, "missing_secret", "STRIPE_WEBHOOK_SECRET is not configured", nil)
		return
	}

	maxBody := h.maxBody()
	tol := h.StripeTolerance
	if tol <= 0 {
		tol = 5 * time.Minute
	}

	body, err := webhookin.VerifyStripe(ctx.R, secret, webhookin.StripeVerifyOptions{
		MaxBody:   maxBody,
		Tolerance: tol,
	})
	if err != nil {
		ctx.ErrorJSON(http.StatusUnauthorized, "invalid_signature", "invalid Stripe signature", map[string]any{
			"provider": "stripe",
		})
		return
	}

	// Stripe event commonly has "id" and "type"
	stripeEventID := extractJSONFieldString(body, "id")
	stripeEventType := extractJSONFieldString(body, "type")

	// Optional: the request id / account / api_version can exist depending on event and Stripe config
	apiVersion := extractJSONFieldString(body, "api_version")
	livemode := extractJSONFieldBool(body, "livemode")

	ctx.JSON(http.StatusOK, map[string]any{
		"ok":          true,
		"provider":    "stripe",
		"event_type":  stripeEventType,
		"event_id":    stripeEventID,
		"api_version": apiVersion,
		"livemode":    livemode,
		"trace_id":    ctx.TraceID,
		"body_bytes":  len(body),
	})
}

func (h *WebhookInHandler) maxBody() int64 {
	if h.MaxBodyBytes > 0 {
		return h.MaxBodyBytes
	}
	return 1 << 20 // 1MB
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
