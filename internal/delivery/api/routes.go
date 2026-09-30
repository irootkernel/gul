package api

import (
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
)

// FeatureHandlers are completed components supplied by the host. Missing
// services remain absent. Construction copies handlers and replaces every
// resolver with the shared boundary; no injected browser identity survives.
type FeatureHandlers struct {
	Runtime     *RuntimeHandler
	Workspace   *WorkspaceHandler
	Direct      *DirectPresentationHandler
	Artifact    *ArtifactHandler
	Interaction *InteractionHandler
	Writer      *WriterHandler
	Files       *FileHandler
	Events      *ClientEventHandler
}

func NewAuthenticatedRoutes(boundary *BrowserBoundary, features FeatureHandlers) (http.Handler, error) {
	if boundary == nil || boundary.Accounts == nil || boundary.Sessions == nil {
		return nil, errors.New("authentication boundary unavailable")
	}
	mux := http.NewServeMux()
	path, handler := gulv1connect.NewAuthServiceHandler(&AuthHandler{Boundary: boundary}, connect.WithReadMaxBytes(4096))
	mux.Handle(path, handler)
	opts := []connect.HandlerOption{connect.WithReadMaxBytes(256 * 1024)}
	if features.Runtime != nil {
		h := *features.Runtime
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewRuntimeServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Workspace != nil {
		h := *features.Workspace
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewWorkspacePresentationServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Direct != nil {
		h := *features.Direct
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewDirectSessionServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Artifact != nil {
		h := *features.Artifact
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewArtifactPresentationServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Interaction != nil {
		h := *features.Interaction
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewInteractionPresentationServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Writer != nil {
		h := *features.Writer
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewWriterActionServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Files != nil {
		h := *features.Files
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewFileServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	if features.Events != nil {
		h := *features.Events
		h.Principal = boundary.Principal
		path, handler = gulv1connect.NewClientEventServiceHandler(&h, opts...)
		mux.Handle(path, handler)
	}
	return boundary.Protect(mux), nil
}
