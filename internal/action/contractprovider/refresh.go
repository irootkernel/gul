package contractprovider

import (
	"context"
	"errors"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
)

// ReadWriter refreshes only writer authority. Recovery coordinators use it
// independently of Read so another aggregate's failure cannot imply freshness.
func (p *Provider) ReadWriter(ctx context.Context, b action.Bound) (action.WriterProjection, error) {
	if err := ctx.Err(); err != nil {
		return action.WriterProjection{}, err
	}
	response, err := p.port.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: ref(b).Workspace})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return action.WriterProjection{}, err
		}
		return action.WriterProjection{}, safeError(err)
	}
	if response == nil || !known(response.ProtoReflect()) {
		return action.WriterProjection{}, action.ErrBlocked
	}
	return writerProjection(response.Writer, b)
}
