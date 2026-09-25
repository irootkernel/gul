package desktop

import (
	"context"
	"errors"
	"testing"
)

func TestDirectoryPickerFailsClosedWithoutHost(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (WailsDirectoryPicker{}).PickDirectory(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled picker = %v", err)
	}
	if _, err := (WailsDirectoryPicker{}).PickDirectory(t.Context()); err == nil {
		t.Fatal("picker used a missing Wails host")
	}
}
