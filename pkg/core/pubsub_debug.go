package core

import (
	"net/http"
	"strings"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/pubsub"
)

// ConfigurePubSub registers a snapshot endpoint when enabled.
func (a *App) ConfigurePubSub() {
	cfg := a.config.PubSub
	if !cfg.Enabled {
		return
	}

	pub := cfg.Pub
	if pub == nil {
		pub = a.pub
	}
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = "/_debug/pubsub"
	}

	a.Router().GetCtx(path, func(ctx *contract.Ctx) {
		if pub == nil {
			ctx.ErrorJSON(http.StatusInternalServerError, "missing_pubsub", "pubsub is not configured", nil)
			return
		}

		type snapshoter interface{ Snapshot() pubsub.MetricsSnapshot }

		if ps, ok := pub.(snapshoter); ok {
			ctx.JSON(http.StatusOK, ps.Snapshot())
			return
		}

		ctx.ErrorJSON(http.StatusNotImplemented, "not_supported", "pubsub snapshot not supported by this implementation", nil)
	})
}
