package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/pubsub"
	"github.com/spcent/golang_simple_server/pkg/router"
	"github.com/spcent/golang_simple_server/pkg/webhookin"
)

type WebhookInHandler struct {
	Pub pubsub.PubSub

	// secrets can be injected (tests) or read from env
	GitHubSecret string
	StripeSecret string

	// verify constraints
	MaxBodyBytes    int64
	StripeTolerance time.Duration

	// inbound idempotency
	Deduper  *webhookin.Deduper
	DedupTTL time.Duration
}

func (h *WebhookInHandler) Register(r *router.Router) {
	r.PostCtx("/webhooks/in/github", h.GitHub)
	r.PostCtx("/webhooks/in/stripe", h.Stripe)
}

func (h *WebhookInHandler) GitHub(ctx *contract.Ctx) {
	if h.Pub == nil {
		ctx.ErrorJSON(http.StatusInternalServerError, "missing_pubsub", "pubsub is not configured", nil)
		return
	}

	secret := strings.TrimSpace(h.GitHubSecret)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	}
	if secret == "" {
		ctx.ErrorJSON(http.StatusInternalServerError, "missing_secret", "GITHUB_WEBHOOK_SECRET is not configured", nil)
		return
	}

	maxBody := h.maxBody()
	raw, err := webhookin.VerifyGitHub(ctx.R, secret, maxBody)
	if err != nil {
		ctx.ErrorJSON(http.StatusUnauthorized, "invalid_signature", "invalid GitHub signature", nil)
		return
	}

	event := strings.TrimSpace(ctx.Headers.Get("X-GitHub-Event"))
	delivery := strings.TrimSpace(ctx.Headers.Get("X-GitHub-Delivery"))
	if event == "" {
		event = "unknown"
	}
	if delivery == "" {
		// still allow but cannot dedup reliably
		delivery = "unknown"
	}

	// Dedup: prefer GitHub delivery id
	d := h.ensureDeduper()
	if delivery != "unknown" && d.SeenBefore("github:"+delivery) {
		// at-least-once semantics: GitHub may resend; we return 200 to stop further retries
		ctx.JSON(http.StatusOK, map[string]any{
			"ok":          true,
			"provider":    "github",
			"event_type":  event,
			"delivery_id": delivery,
			"deduped":     true,
			"trace_id":    ctx.TraceID,
		})
		return
	}

	topic := "in.github." + sanitizeTopicSuffix(event)

	msg := pubsub.Message{
		ID:    delivery, // good enough for in-proc
		Topic: topic,
		Type:  event,
		Time:  time.Now().UTC(),
		Data:  json.RawMessage(raw), // immutable bytes; consumers can unmarshal if needed
		Meta: map[string]string{
			"source":      "github",
			"trace_id":    ctx.TraceID,
			"delivery_id": delivery,
			"event_type":  event,
			"client_ip":   ctx.ClientIP,
		},
	}

	_ = h.Pub.Publish(topic, msg)

	// Minimal response for webhook sender
	ctx.JSON(http.StatusOK, map[string]any{
		"ok":          true,
		"provider":    "github",
		"topic":       topic,
		"event_type":  event,
		"delivery_id": delivery,
		"deduped":     false,
		"trace_id":    ctx.TraceID,
		"body_bytes":  len(raw),
	})
}

func (h *WebhookInHandler) Stripe(ctx *contract.Ctx) {
	if h.Pub == nil {
		ctx.ErrorJSON(http.StatusInternalServerError, "missing_pubsub", "pubsub is not configured", nil)
		return
	}

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

	raw, err := webhookin.VerifyStripe(ctx.R, secret, webhookin.StripeVerifyOptions{
		MaxBody:   maxBody,
		Tolerance: tol,
	})
	if err != nil {
		ctx.ErrorJSON(http.StatusUnauthorized, "invalid_signature", "invalid Stripe signature", nil)
		return
	}

	// Stripe event id and type (best effort)
	evtID := extractJSONFieldString(raw, "id")
	evtType := extractJSONFieldString(raw, "type")
	if evtType == "" {
		evtType = "unknown"
	}
	if evtID == "" {
		evtID = "unknown"
	}

	// Dedup: prefer Stripe event id
	d := h.ensureDeduper()
	if evtID != "unknown" && d.SeenBefore("stripe:"+evtID) {
		ctx.JSON(http.StatusOK, map[string]any{
			"ok":         true,
			"provider":   "stripe",
			"event_type": evtType,
			"event_id":   evtID,
			"deduped":    true,
			"trace_id":   ctx.TraceID,
		})
		return
	}

	topic := "in.stripe." + sanitizeTopicSuffix(evtType)

	msg := pubsub.Message{
		ID:    evtID,
		Topic: topic,
		Type:  evtType,
		Time:  time.Now().UTC(),
		Data:  json.RawMessage(raw),
		Meta: map[string]string{
			"source":      "stripe",
			"trace_id":    ctx.TraceID,
			"delivery_id": evtID,
			"event_type":  evtType,
			"client_ip":   ctx.ClientIP,
		},
	}

	_ = h.Pub.Publish(topic, msg)

	ctx.JSON(http.StatusOK, map[string]any{
		"ok":         true,
		"provider":   "stripe",
		"topic":      topic,
		"event_type": evtType,
		"event_id":   evtID,
		"deduped":    false,
		"trace_id":   ctx.TraceID,
		"body_bytes": len(raw),
	})
}

func (h *WebhookInHandler) maxBody() int64 {
	if h.MaxBodyBytes > 0 {
		return h.MaxBodyBytes
	}
	return 1 << 20 // 1MB
}

func (h *WebhookInHandler) ensureDeduper() *webhookin.Deduper {
	if h.Deduper != nil {
		return h.Deduper
	}
	ttl := h.DedupTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	h.Deduper = webhookin.NewDeduper(ttl)
	return h.Deduper
}

// sanitizeTopicSuffix keeps topic safe and consistent.
// Allow [a-zA-Z0-9._-], replace others with '_'.
func sanitizeTopicSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-'
		if ok {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	return strings.ToLower(b.String())
}
