package handlers

import (
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/pubsub"
	"github.com/spcent/golang_simple_server/pkg/router"
)

type PubSubDebugHandler struct {
	Pub pubsub.PubSub
}

func (h *PubSubDebugHandler) Register(r *router.Router) {
	r.GetCtx("/debug/pubsub", h.Snapshot)
}

func (h *PubSubDebugHandler) Snapshot(ctx *contract.Ctx) {
	if h.Pub == nil {
		ctx.ErrorJSON(http.StatusInternalServerError, "missing_pubsub", "pubsub is not configured", nil)
		return
	}

	// type assert to InProcPubSub (or any implementation that provides Snapshot())
	type snapshoter interface{ Snapshot() pubsub.MetricsSnapshot }

	if ps, ok := h.Pub.(snapshoter); ok {
		ctx.JSON(http.StatusOK, ps.Snapshot())
		return
	}

	ctx.ErrorJSON(http.StatusNotImplemented, "not_supported", "pubsub snapshot not supported by this implementation", nil)
}
