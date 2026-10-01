package api

import (
	"context"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/app"
)

// Diagnostics exposes bounded availability, never raw runtime logs or identities.
type DiagnosticsHandler struct {
	Core      *app.Core
	Principal PrincipalResolver
}

func (h *DiagnosticsHandler) GetSummary(ctx context.Context, _ *connect.Request[gulv1.GetSummaryRequest]) (*connect.Response[gulv1.GetSummaryResponse], error) {
	subject, err := localAccess(ctx, h.Core, h.Principal)
	if err != nil {
		return nil, err
	}
	ready := h.Core.RequireProductAccess(ctx, app.Principal{Subject: subject}) == nil
	// localAccess has already verified persistence; an outage returns an error.
	return connect.NewResponse(&gulv1.GetSummaryResponse{ProviderReady: ready, PersistenceReady: true}), nil
}
