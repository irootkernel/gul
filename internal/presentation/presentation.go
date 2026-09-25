// Package presentation owns Gul-local Workspace and Direct Session metadata.
package presentation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid presentation request")
	ErrNotFound    = errors.New("presentation not found")
	ErrUnavailable = errors.New("presentation persistence unavailable")
)

type Workspace struct {
	SubjectID   string
	WorkspaceID string
	DisplayName string
	Favorite    bool
	Hidden      bool
}

type DirectSession struct {
	SubjectID   string
	SessionID   string
	WorkspaceID string
	DisplayName string
	Favorite    bool
	Archived    bool
}

type Navigation struct {
	WorkspaceID string
	SessionID   string
}

type Repository interface {
	Workspace(context.Context, string, string) (Workspace, error)
	DirectSession(context.Context, string, string) (DirectSession, error)
	RenameWorkspace(context.Context, string, string, string) error
	SetWorkspaceFavorite(context.Context, string, string, bool) error
	// SetWorkspaceHidden clears navigation for the hidden workspace atomically.
	SetWorkspaceHidden(context.Context, string, string, bool) error
	// RemoveWorkspace removes only Gul-owned presentation and bindings.
	RemoveWorkspace(context.Context, string, string) error
	RenameDirectSession(context.Context, string, string, string) error
	SetDirectSessionFavorite(context.Context, string, string, bool) error
	// SetDirectSessionArchived clears a selected archived session atomically.
	SetDirectSessionArchived(context.Context, string, string, bool) error
	Navigation(context.Context, string) (Navigation, error)
	// SetNavigation accepts only a visible attachment and its visible session.
	SetNavigation(context.Context, string, Navigation) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Workspace(ctx context.Context, subjectID, workspaceID string) (Workspace, error) {
	if s == nil || s.repo == nil || subjectID == "" || workspaceID == "" {
		return Workspace{}, ErrInvalid
	}
	entry, err := s.repo.Workspace(ctx, subjectID, workspaceID)
	return entry, persistenceError(err)
}

func (s *Service) DirectSession(ctx context.Context, subjectID, sessionID string) (DirectSession, error) {
	if s == nil || s.repo == nil || subjectID == "" || sessionID == "" {
		return DirectSession{}, ErrInvalid
	}
	entry, err := s.repo.DirectSession(ctx, subjectID, sessionID)
	return entry, persistenceError(err)
}

func (s *Service) RenameWorkspace(ctx context.Context, subjectID, workspaceID, name string) (Workspace, error) {
	if subjectID == "" || workspaceID == "" || !ValidName(name) || s == nil || s.repo == nil {
		return Workspace{}, ErrInvalid
	}
	if err := s.repo.RenameWorkspace(ctx, subjectID, workspaceID, name); err != nil {
		return Workspace{}, persistenceError(err)
	}
	return s.Workspace(ctx, subjectID, workspaceID)
}

func (s *Service) SetWorkspaceFavorite(ctx context.Context, subjectID, workspaceID string, favorite bool) (Workspace, error) {
	if subjectID == "" || workspaceID == "" || s == nil || s.repo == nil {
		return Workspace{}, ErrInvalid
	}
	if err := s.repo.SetWorkspaceFavorite(ctx, subjectID, workspaceID, favorite); err != nil {
		return Workspace{}, persistenceError(err)
	}
	return s.Workspace(ctx, subjectID, workspaceID)
}

func (s *Service) SetWorkspaceHidden(ctx context.Context, subjectID, workspaceID string, hidden bool) (Workspace, error) {
	if subjectID == "" || workspaceID == "" || s == nil || s.repo == nil {
		return Workspace{}, ErrInvalid
	}
	if err := s.repo.SetWorkspaceHidden(ctx, subjectID, workspaceID, hidden); err != nil {
		return Workspace{}, persistenceError(err)
	}
	return s.Workspace(ctx, subjectID, workspaceID)
}

func (s *Service) RemoveWorkspace(ctx context.Context, subjectID, workspaceID string) error {
	if subjectID == "" || workspaceID == "" || s == nil || s.repo == nil {
		return ErrInvalid
	}
	return persistenceError(s.repo.RemoveWorkspace(ctx, subjectID, workspaceID))
}

func (s *Service) RenameDirectSession(ctx context.Context, subjectID, sessionID, name string) (DirectSession, error) {
	if subjectID == "" || sessionID == "" || !ValidName(name) || s == nil || s.repo == nil {
		return DirectSession{}, ErrInvalid
	}
	if err := s.repo.RenameDirectSession(ctx, subjectID, sessionID, name); err != nil {
		return DirectSession{}, persistenceError(err)
	}
	return s.DirectSession(ctx, subjectID, sessionID)
}

func (s *Service) SetDirectSessionFavorite(ctx context.Context, subjectID, sessionID string, favorite bool) (DirectSession, error) {
	if subjectID == "" || sessionID == "" || s == nil || s.repo == nil {
		return DirectSession{}, ErrInvalid
	}
	if err := s.repo.SetDirectSessionFavorite(ctx, subjectID, sessionID, favorite); err != nil {
		return DirectSession{}, persistenceError(err)
	}
	return s.DirectSession(ctx, subjectID, sessionID)
}

func (s *Service) SetDirectSessionArchived(ctx context.Context, subjectID, sessionID string, archived bool) (DirectSession, error) {
	if subjectID == "" || sessionID == "" || s == nil || s.repo == nil {
		return DirectSession{}, ErrInvalid
	}
	if err := s.repo.SetDirectSessionArchived(ctx, subjectID, sessionID, archived); err != nil {
		return DirectSession{}, persistenceError(err)
	}
	return s.DirectSession(ctx, subjectID, sessionID)
}

func (s *Service) Navigation(ctx context.Context, subjectID string) (Navigation, error) {
	if subjectID == "" || s == nil || s.repo == nil {
		return Navigation{}, ErrInvalid
	}
	value, err := s.repo.Navigation(ctx, subjectID)
	return value, persistenceError(err)
}

func (s *Service) SetNavigation(ctx context.Context, subjectID string, value Navigation) (Navigation, error) {
	if subjectID == "" || s == nil || s.repo == nil || (value.SessionID != "" && value.WorkspaceID == "") {
		return Navigation{}, ErrInvalid
	}
	if err := s.repo.SetNavigation(ctx, subjectID, value); err != nil {
		return Navigation{}, persistenceError(err)
	}
	return s.Navigation(ctx, subjectID)
}

// ValidName is the shared bound for stored user-visible presentation names.
func ValidName(name string) bool {
	if !utf8.ValidString(name) || name != strings.TrimSpace(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 120 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func persistenceError(err error) error {
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalid) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}
