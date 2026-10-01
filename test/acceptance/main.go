// This executable is an opt-in driver, never a Gul provider fallback.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/test/acceptance/fixture"
	"os"
	"strconv"
	"time"
)

func main() {
	if os.Getenv("GUL_RUN_ASSEMBLED_ACCEPTANCE") != "1" {
		panic("assembled acceptance is an opt-in isolated test fixture")
	}
	if len(os.Args) != 4 {
		panic("usage: acceptance DATA WORKSPACE PORT")
	}
	port, err := strconv.Atoi(os.Args[3])
	if err != nil {
		panic(err)
	}
	f, err := fixture.New(os.Args[1], os.Args[2], port)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	if err = f.Start(ctx); err != nil {
		panic(err)
	}
	defer func() {
		stop, c := context.WithTimeout(ctx, 10*time.Second)
		defer c()
		if err := f.Stop(stop); err != nil {
			panic(err)
		}
	}()
	out := json.NewEncoder(os.Stdout)
	_ = out.Encode(map[string]any{"ready": true, "origin": f.Host.Origin()})
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "initialize":
			err = f.Initialize(ctx)
		case "complete":
			err = f.Complete()
		case "approval":
			err = f.Approval()
		case "results":
			err = f.Results()
		case "restart":
			err = f.Restart(ctx)
		case "disconnect":
			f.Provider.Offline.Store(true)
			err = f.Runtime.Synchronize(ctx)
			if err == nil {
				err = errors.New("disconnected provider unexpectedly ready")
			} else {
				err = nil
			}
		case "reconnect":
			f.Provider.Offline.Store(false)
			err = f.Runtime.Synchronize(ctx)
		case "stream-loss":
			err = f.Provider.FailStream(f.Run.RunId, errors.New("fixture stream loss"))
		case "unknown-submit":
			err = f.Provider.FaultNext("SubmitTurn", scenario.AfterCommit, errors.New("fixture lost response"))
		case "settle-close":
			err = f.Provider.SetCloseProgress(f.Run.RunId, publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED)
		case "stop":
			return
		default:
			err = errors.New("unknown fixture command")
		}
		if err != nil {
			_ = out.Encode(map[string]any{"ok": false, "error": err.Error()})
		} else {
			_ = out.Encode(map[string]any{"ok": true, "sessionId": f.Binding.ID, "workspaceId": f.Binding.WorkspaceID, "closeCalls": len(f.Provider.CloseCalls()), "submitCalls": len(f.Provider.SubmitCalls())})
		}
	}
	if err = scanner.Err(); err != nil {
		panic(err)
	}
}
