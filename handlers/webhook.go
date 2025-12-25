package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/router"
	"github.com/spcent/golang_simple_server/pkg/webhook"
)

type WebhookHandler struct {
	Svc *webhook.Service
}

func (h *WebhookHandler) Register(r *router.Router) {
	// Targets (management)
	r.PostCtx("/webhooks/targets", h.CreateTarget)
	r.GetCtx("/webhooks/targets", h.ListTargets)
	r.GetCtx("/webhooks/targets/:id", h.GetTarget)
	r.PatchCtx("/webhooks/targets/:id", h.PatchTarget)
	r.PostCtx("/webhooks/targets/:id/enable", h.EnableTarget)
	r.PostCtx("/webhooks/targets/:id/disable", h.DisableTarget)

	// Events (trigger outbound deliveries)
	r.PostCtx("/webhooks/events/:event", h.TriggerEvent)

	// Deliveries (inspection + replay)
	r.GetCtx("/webhooks/deliveries", h.ListDeliveries)
	r.GetCtx("/webhooks/deliveries/:id", h.GetDelivery)
	r.PostCtx("/webhooks/deliveries/:id/replay", h.ReplayDelivery)
}

/*
|--------------------------------------------------------------------------
| Targets
|--------------------------------------------------------------------------
*/

func (h *WebhookHandler) CreateTarget(ctx *contract.Ctx) {
	var req webhook.Target
	if err := ctx.BindJSON(&req); err != nil {
		// BindJSON should return *router.BindError-like (in your router pkg),
		// but here we just return a generic bad_request to keep handler decoupled.
		ctx.ErrorJSON(http.StatusBadRequest, "invalid_json", "invalid JSON payload", nil)
		return
	}

	t, err := h.Svc.CreateTarget(ctx.R.Context(), req)
	if err != nil {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}

	// never expose secret
	t.Secret = ""
	ctx.JSON(http.StatusCreated, t)
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

	for i := range items {
		items[i].Secret = ""
	}
	ctx.JSON(http.StatusOK, map[string]any{"items": items})
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
	t.Secret = ""
	ctx.JSON(http.StatusOK, t)
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

	t.Secret = ""
	ctx.JSON(http.StatusOK, t)
}

func (h *WebhookHandler) EnableTarget(ctx *contract.Ctx) {
	h.setTargetEnabled(ctx, true)
}

func (h *WebhookHandler) DisableTarget(ctx *contract.Ctx) {
	h.setTargetEnabled(ctx, false)
}

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

	t.Secret = ""
	ctx.JSON(http.StatusOK, t)
}

/*
|--------------------------------------------------------------------------
| Events
|--------------------------------------------------------------------------
*/

func (h *WebhookHandler) TriggerEvent(ctx *contract.Ctx) {
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

	ctx.JSON(http.StatusOK, map[string]any{
		"enqueued": enqueued,
	})
}

/*
|--------------------------------------------------------------------------
| Deliveries
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

	ctx.JSON(http.StatusOK, map[string]any{
		"items":       ds,
		"next_cursor": nextCursor,
	})
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

	// default omit payload unless requested
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

	// default omit payload unless requested
	if !includePayload(ctx) {
		d.PayloadJSON = nil
	}

	ctx.JSON(http.StatusOK, d)
}

func includePayload(ctx *contract.Ctx) bool {
	return strings.TrimSpace(ctx.Query.Get("include_payload")) == "1"
}
