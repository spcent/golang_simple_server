package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/webhook"
)

type WebhookTargetsHandler struct {
	Svc *webhook.Service
}

func (h *WebhookTargetsHandler) Register(app any) {
	// 伪代码：按你 router 的注册方式改一下即可
	// app.Post("/webhooks/targets", h.Create)
	// app.Get("/webhooks/targets", h.List)
	// app.Get("/webhooks/targets/:id", h.Get)
	// app.Patch("/webhooks/targets/:id", h.Patch)
	// app.Post("/webhooks/targets/:id/enable", h.Enable)
	// app.Post("/webhooks/targets/:id/disable", h.Disable)
}

func (h *WebhookTargetsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req webhook.Target
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "invalid_json")
		return
	}
	t, err := h.Svc.CreateTarget(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t.Secret = "" // do not expose
	writeJSON(w, 201, t)
}

func (h *WebhookTargetsHandler) List(w http.ResponseWriter, r *http.Request) {
	var enabled *bool
	if v := strings.TrimSpace(r.URL.Query().Get("enabled")); v != "" {
		b := (v == "1" || strings.EqualFold(v, "true"))
		enabled = &b
	}
	event := strings.TrimSpace(r.URL.Query().Get("event"))
	ts, err := h.Svc.ListTargets(r.Context(), webhook.TargetFilter{Enabled: enabled, Event: event})
	if err != nil {
		writeErr(w, 500, "store_error")
		return
	}
	// mask secrets
	for i := range ts {
		ts[i].Secret = ""
	}
	writeJSON(w, 200, map[string]any{"items": ts})
}

func (h *WebhookTargetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	t, ok := h.Svc.GetTarget(r.Context(), id)
	if !ok {
		writeErr(w, 404, "not_found")
		return
	}
	t.Secret = ""
	writeJSON(w, 200, t)
}

func (h *WebhookTargetsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	var patch webhook.TargetPatch
	if err := readJSON(r, &patch); err != nil {
		writeErr(w, 400, "invalid_json")
		return
	}
	t, err := h.Svc.UpdateTarget(r.Context(), id, patch)
	if err != nil {
		if err == webhook.ErrNotFound {
			writeErr(w, 404, "not_found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	t.Secret = ""
	writeJSON(w, 200, t)
}

func (h *WebhookTargetsHandler) Enable(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	b := true
	t, err := h.Svc.UpdateTarget(r.Context(), id, webhook.TargetPatch{Enabled: &b})
	if err != nil {
		if err == webhook.ErrNotFound {
			writeErr(w, 404, "not_found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	t.Secret = ""
	writeJSON(w, 200, t)
}

func (h *WebhookTargetsHandler) Disable(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	b := false
	t, err := h.Svc.UpdateTarget(r.Context(), id, webhook.TargetPatch{Enabled: &b})
	if err != nil {
		if err == webhook.ErrNotFound {
			writeErr(w, 404, "not_found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	t.Secret = ""
	writeJSON(w, 200, t)
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// pathParam: 按你 router 的实现替换。
// 如果你项目用的是 router.ParamsFromContext(r.Context())，这里读取它即可。
func pathParam(r *http.Request, key string) string {
	// TODO: integrate with your router
	return ""
}
