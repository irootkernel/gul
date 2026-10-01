package submit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
)

type forbiddenState struct {
	action.Repository
	session.Workspace
	action.Provider
}
type forbiddenDispatch struct{ t *testing.T }

func (d forbiddenDispatch) Submit(context.Context, action.Bound, action.Input, string, string, action.WriteIntent) error {
	d.t.Fatal("invalid prompt reached dispatch")
	return nil
}

func TestInvalidPromptNeverReadsAuthorityOrDispatches(t *testing.T) {
	for _, q := range []struct{ name, attempt, text string }{
		{"invalid attempt", strings.Repeat("a", 257), "text"},
		{"invalid UTF8", "attempt", string([]byte{0xff})},
	} {
		t.Run(q.name, func(t *testing.T) {
			forbidden := forbiddenState{}
			s := Service{Actions: &action.Service{Repository: forbidden, Workspaces: forbidden, Provider: forbidden, Gate: func(string, string) bool { t.Fatal("invalid prompt read authority"); return false }}, Dispatcher: forbiddenDispatch{t}}
			if _, err := s.Send(t.Context(), "owner", "session", q.attempt, q.text, action.IntentRead); !errors.Is(err, action.ErrInvalid) {
				t.Fatalf("invalid prompt = %v", err)
			}
		})
	}
}
