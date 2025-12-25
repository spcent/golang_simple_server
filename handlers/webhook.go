package handlers

import (
	"net/http"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/router"
	"github.com/spcent/golang_simple_server/pkg/webhook"
)

type WebhookHandler struct {
	Svc *webhook.Service
}

func (h *WebhookHandler) Register(r *router.Router) {
	r.GetCtx("/webhooks/deliveries", h.List)
	r.GetCtx("/webhooks/deliveries/:id", h.Get)
	r.PostCtx("/webhooks/deliveries/:id/replay", h.Replay)
}

func (h *WebhookHandler) List(ctx *contract.Ctx) {
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
	f.Cursor = strings.TrimSpace(q.Get("cursor"))
	// limit optional; keep default inside store
	ds, err := h.Svc.ListDeliveries(ctx.R.Context(), f)
	if err != nil {
		ctx.ErrorJSON(http.StatusOK, "store_error", err.Error(), nil)
		return
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

func (h *WebhookHandler) Get(ctx *contract.Ctx) {
	id, ok := ctx.Param("id")
	if !ok {
		ctx.ErrorJSON(http.StatusBadRequest, "bad_request", "id is required", nil)
		return
	}

	d, ok := h.Svc.GetDelivery(ctx.R.Context(), id)
	if !ok {
		ctx.ErrorJSON(http.StatusNotFound, "not_found", "delivery not found", nil)
		return
	}

	ctx.JSON(http.StatusOK, d)
}

func (h *WebhookHandler) Replay(ctx *contract.Ctx) {
	id, ok := ctx.Param("id")
	if !ok {
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

	ctx.JSON(http.StatusOK, d)
}
