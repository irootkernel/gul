package desktop

import (
	"context"
	"errors"

	"github.com/rootkernel/gul/internal/workspace"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WailsDirectoryPicker obtains a directory from the host-owned native dialog.
// The browser request carries no absolute path.
type WailsDirectoryPicker struct{}

var _ workspace.Picker = WailsDirectoryPicker{}

func (WailsDirectoryPicker) PickDirectory(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	app := application.Get()
	if app == nil || app.Dialog == nil {
		return "", errors.New("host directory picker unavailable")
	}
	selected, err := app.Dialog.OpenFile().CanChooseFiles(false).CanChooseDirectories(true).
		CanCreateDirectories(false).SetTitle("Select an initialized Workspace").PromptForSingleSelection()
	if err != nil {
		return "", errors.New("host directory selection unavailable")
	}
	if selected == "" {
		return "", workspace.ErrSelectionCancelled
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return selected, nil
}
