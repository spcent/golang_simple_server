package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/webhook"
)

type WebhookEventsHandler struct {
	Svc *webhook.Service
}

func (h *WebhookEventsHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	evt := strings.TrimSpace(pathParam(r, "event"))
	if evt == "" {
		writeErr(w, 400, "event_required")
		return
	}

	var body struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "invalid_json")
		return
	}

	n, err := h.Svc.TriggerEvent(r.Context(), webhook.Event{
		Type: evt,
		Data: body.Data,
		Meta: body.Meta,
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"enqueued": n})
}
