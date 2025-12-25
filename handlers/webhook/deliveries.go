package handlers

import (
	"net/http"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/webhook"
)

type WebhookDeliveriesHandler struct {
	Svc *webhook.Service
}

func (h *WebhookDeliveriesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

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
	ds, err := h.Svc.ListDeliveries(r.Context(), f)
	if err != nil {
		writeErr(w, 500, "store_error")
		return
	}
	nextCursor := ""
	if len(ds) > 0 {
		nextCursor = ds[len(ds)-1].ID
	}
	writeJSON(w, 200, map[string]any{"items": ds, "next_cursor": nextCursor})
}

func (h *WebhookDeliveriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	d, ok := h.Svc.GetDelivery(r.Context(), id)
	if !ok {
		writeErr(w, 404, "not_found")
		return
	}
	writeJSON(w, 200, d)
}

func (h *WebhookDeliveriesHandler) Replay(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	d, err := h.Svc.ReplayDelivery(r.Context(), id)
	if err != nil {
		if err == webhook.ErrNotFound {
			writeErr(w, 404, "not_found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, d)
}
