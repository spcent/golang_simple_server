package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/router"
	"github.com/spcent/golang_simple_server/pkg/webhook"
)

type WebhookHandler struct {
	Svc *webhook.Service
}

func (h *WebhookHandler) Register(r *router.Router) {
	// Targets
	r.PostCtx("/webhooks/targets", h.CreateTarget)
	r.GetCtx("/webhooks/targets", h.ListTargets)
	r.GetCtx("/webhooks/targets/:id", h.GetTarget)
	r.PatchCtx("/webhooks/targets/:id", h.PatchTarget)
	r.PostCtx("/webhooks/targets/:id/enable", h.EnableTarget)
	r.PostCtx("/webhooks/targets/:id/disable", h.DisableTarget)

	// Events
	r.PostCtx("/webhooks/events/:event", h.TriggerEvent)

	// Deliveries
	r.GetCtx("/webhooks/deliveries", h.ListDeliveries)
	r.GetCtx("/webhooks/deliveries/:id", h.GetDelivery)
	r.PostCtx("/webhooks/deliveries/:id/replay", h.ReplayDelivery)
}

/*
|--------------------------------------------------------------------------
| Targets (secret masked)
|--------------------------------------------------------------------------
*/

type TargetDTO struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`

	Headers map[string]string `json:"headers,omitempty"`

	TimeoutMs     int   `json:"timeout_ms,omitempty"`
	MaxRetries    int   `json:"max_retries,omitempty"`
	BackoffBaseMs int   `json:"backoff_base_ms,omitempty"`
	BackoffMaxMs  int   `json:"backoff_max_ms,omitempty"`
	RetryOn429    *bool `json:"retry_on_429,omitempty"`

	SecretMasked string    `json:"secret_masked"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (h *WebhookHandler) CreateTarget(ctx *contract.Ctx) {
	var req webhook.Target
	if err := ctx.BindJSON(&req); err != nil {
		ctx.ErrorJSON(http.StatusBadRequest, "invalid_json", "invalid JSON payload", nil)
		return
	}

	t, err := h.Svc.CreateTarget(ctx.R.Context(), req)
	if err != nil {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}

	ctx.JSON(http.StatusCreated, targetToDTO(t))
}

func (h *WebhookHandler) ListTargets(ctx *contract.Ctx) {
	q := ctx.Query

	var enabled *bool
	if v := strings.TrimSpace(q.Get("enabled")); v != "" {
		b := (v == "1" || strings.EqualFold(v, "true"))
		enabled = &b
	}
	event := strings.TrimSpace(q.Get("event"))

	items, err := h.Svc.ListTargets(ctx.R.Context(), webhook.TargetFilter{
		Enabled: enabled,
		Event:   event,
	})
	if err != nil {
		ctx.ErrorJSON(http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}

	out := make([]TargetDTO, 0, len(items))
	for _, t := range items {
		out = append(out, targetToDTO(t))
	}

	ctx.JSON(http.StatusOK, map[string]any{"items": out})
}

func (h *WebhookHandler) GetTarget(ctx *contract.Ctx) {
	id, ok := ctx.Param("id")
	if !ok || strings.TrimSpace(id) == "" {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "id is required", nil)
		return
	}

	t, ok := h.Svc.GetTarget(ctx.R.Context(), id)
	if !ok {
		ctx.ErrorJSON(http.StatusNotFound, "not_found", "target not found", nil)
		return
	}
	ctx.JSON(http.StatusOK, targetToDTO(t))
}

func (h *WebhookHandler) PatchTarget(ctx *contract.Ctx) {
	id, ok := ctx.Param("id")
	if !ok || strings.TrimSpace(id) == "" {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "id is required", nil)
		return
	}

	var patch webhook.TargetPatch
	if err := ctx.BindJSON(&patch); err != nil {
		ctx.ErrorJSON(http.StatusBadRequest, "invalid_json", "invalid JSON payload", nil)
		return
	}

	t, err := h.Svc.UpdateTarget(ctx.R.Context(), id, patch)
	if err != nil {
		if err == webhook.ErrNotFound {
			ctx.ErrorJSON(http.StatusNotFound, "not_found", "target not found", nil)
			return
		}
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}

	ctx.JSON(http.StatusOK, targetToDTO(t))
}

func (h *WebhookHandler) EnableTarget(ctx *contract.Ctx)  { h.setTargetEnabled(ctx, true) }
func (h *WebhookHandler) DisableTarget(ctx *contract.Ctx) { h.setTargetEnabled(ctx, false) }

func (h *WebhookHandler) setTargetEnabled(ctx *contract.Ctx, enabled bool) {
	id, ok := ctx.Param("id")
	if !ok || strings.TrimSpace(id) == "" {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "id is required", nil)
		return
	}

	t, err := h.Svc.UpdateTarget(ctx.R.Context(), id, webhook.TargetPatch{Enabled: &enabled})
	if err != nil {
		if err == webhook.ErrNotFound {
			ctx.ErrorJSON(http.StatusNotFound, "not_found", "target not found", nil)
			return
		}
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}

	ctx.JSON(http.StatusOK, targetToDTO(t))
}

func targetToDTO(t webhook.Target) TargetDTO {
	return TargetDTO{
		ID:            t.ID,
		Name:          t.Name,
		URL:           t.URL,
		Events:        t.Events,
		Enabled:       t.Enabled,
		Headers:       t.Headers,
		TimeoutMs:     t.TimeoutMs,
		MaxRetries:    t.MaxRetries,
		BackoffBaseMs: t.BackoffBaseMs,
		BackoffMaxMs:  t.BackoffMaxMs,
		RetryOn429:    t.RetryOn429,
		SecretMasked:  maskSecret(t.Secret),
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

/*
|--------------------------------------------------------------------------
| Events (trigger auth)
|--------------------------------------------------------------------------
*/

func (h *WebhookHandler) TriggerEvent(ctx *contract.Ctx) {
	if !h.authorizeTrigger(ctx) {
		return
	}

	event, ok := ctx.Param("event")
	if !ok || strings.TrimSpace(event) == "" {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "event is required", nil)
		return
	}

	var body struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		ctx.ErrorJSON(http.StatusBadRequest, "invalid_json", "invalid JSON payload", nil)
		return
	}

	enqueued, err := h.Svc.TriggerEvent(ctx.R.Context(), webhook.Event{
		Type: event,
		Data: body.Data,
		Meta: body.Meta,
	})
	if err != nil {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}

	ctx.JSON(http.StatusOK, map[string]any{"enqueued": enqueued})
}

func (h *WebhookHandler) authorizeTrigger(ctx *contract.Ctx) bool {
	// Shared secret token to prevent arbitrary external triggers.
	// env: WEBHOOK_TRIGGER_TOKEN
	expected := strings.TrimSpace(os.Getenv("WEBHOOK_TRIGGER_TOKEN"))

	// Security default: if not configured, deny in non-dev environments.
	// If you want "allow when empty" for local dev, flip this condition.
	if expected == "" {
		ctx.ErrorJSON(http.StatusForbidden, "forbidden", "webhook trigger token not configured", nil)
		return false
	}

	got := strings.TrimSpace(ctx.Headers.Get("X-Webhook-Trigger-Token"))
	if got == "" {
		// optionally accept "Authorization: Bearer <token>"
		auth := strings.TrimSpace(ctx.Headers.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			got = strings.TrimSpace(auth[7:])
		}
	}

	if got != expected {
		ctx.ErrorJSON(http.StatusUnauthorized, "unauthorized", "invalid trigger token", nil)
		return false
	}
	return true
}

/*
|--------------------------------------------------------------------------
| Deliveries (time filter + stats)
|--------------------------------------------------------------------------
*/

func (h *WebhookHandler) ListDeliveries(ctx *contract.Ctx) {
	q := ctx.Query

	var f webhook.DeliveryFilter
	if v := strings.TrimSpace(q.Get("target_id")); v != "" {
		f.TargetID = &v
	}
	if v := strings.TrimSpace(q.Get("event")); v != "" {
		f.Event = &v
	}
	if v := strings.TrimSpace(q.Get("status")); v != "" {
		st := webhook.DeliveryStatus(v)
		f.Status = &st
	}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	f.Cursor = strings.TrimSpace(q.Get("cursor"))

	// NEW: time range filter
	if from, ok := parseRFC3339(q.Get("from")); ok {
		f.From = &from
	}
	if to, ok := parseRFC3339(q.Get("to")); ok {
		f.To = &to
	}

	ds, err := h.Svc.ListDeliveries(ctx.R.Context(), f)
	if err != nil {
		ctx.ErrorJSON(http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}

	// default omit payload_json in list
	for i := range ds {
		ds[i].PayloadJSON = nil
	}

	nextCursor := ""
	if len(ds) > 0 {
		nextCursor = ds[len(ds)-1].ID
	}

	resp := map[string]any{
		"items":       ds,
		"next_cursor": nextCursor,
	}

	// NEW: stats aggregation (only for this page)
	if strings.TrimSpace(q.Get("include_stats")) == "1" {
		resp["stats"] = buildDeliveryStats(ds)
	}

	ctx.JSON(http.StatusOK, resp)
}

func (h *WebhookHandler) GetDelivery(ctx *contract.Ctx) {
	id, ok := ctx.Param("id")
	if !ok || strings.TrimSpace(id) == "" {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "id is required", nil)
		return
	}

	d, ok := h.Svc.GetDelivery(ctx.R.Context(), id)
	if !ok {
		ctx.ErrorJSON(http.StatusNotFound, "not_found", "delivery not found", nil)
		return
	}

	if !includePayload(ctx) {
		d.PayloadJSON = nil
	}
	ctx.JSON(http.StatusOK, d)
}

func (h *WebhookHandler) ReplayDelivery(ctx *contract.Ctx) {
	id, ok := ctx.Param("id")
	if !ok || strings.TrimSpace(id) == "" {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "id is required", nil)
		return
	}

	d, err := h.Svc.ReplayDelivery(ctx.R.Context(), id)
	if err != nil {
		if err == webhook.ErrNotFound {
			ctx.ErrorJSON(http.StatusNotFound, "not_found", "delivery not found", nil)
			return
		}
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}

	if !includePayload(ctx) {
		d.PayloadJSON = nil
	}
	ctx.JSON(http.StatusOK, d)
}

func includePayload(ctx *contract.Ctx) bool {
	return strings.TrimSpace(ctx.Query.Get("include_payload")) == "1"
}

func parseRFC3339(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func buildDeliveryStats(ds []webhook.Delivery) map[string]any {
	by := map[string]int{}
	for _, d := range ds {
		by[string(d.Status)]++
	}
	return map[string]any{
		"total":     len(ds),
		"by_status": by,
	}
}
