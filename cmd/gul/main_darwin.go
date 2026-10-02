package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rootkernel/gul/internal/deployment"
	"github.com/rootkernel/gul/internal/desktop"
	"github.com/rootkernel/gul/internal/host"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runCommand(ctx, args, func(view host.Attachment, onClose func()) error {
		return (desktop.WailsHost{URL: view.Origin, CertificateDER: view.CertificateDER, SetupCredential: view.SetupCredential}).RunContext(ctx, onClose)
	})
}

func runCommand(ctx context.Context, args []string, openDesktop func(host.Attachment, func()) error) (result error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	mode := "desktop"
	if len(args) > 0 && (args[0] == "serve" || args[0] == "launchd-plist" || args[0] == "diagnose") {
		mode = args[0]
		args = args[1:]
	}
	root, e := host.DefaultDirectory()
	if e != nil {
		return errors.New("Gul home directory unavailable")
	}
	flags := flag.NewFlagSet("gul", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	data := flags.String("data-directory", root, "absolute protected Gul data directory")
	port := flags.Int("port", host.DefaultPort, "fixed loopback HTTPS port")
	tailnet := flags.String("tailnet-host", "", "expected tailnet DNS name and HTTPS port")
	executable := flags.String("dolgorae-executable", "", "qualified Dolgorae executable (defaults to PATH)")
	var roots, policies repeatedStrings
	flags.Var(&roots, "workspace-root", "approved absolute Workspace root; may repeat")
	flags.Var(&policies, "policy", "approved launch policy; may repeat")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return errors.New("usage: gul [serve|launchd-plist|diagnose] [--data-directory PATH] [--port PORT] [--tailnet-host HOST:PORT]")
	}
	if mode == "launchd-plist" {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		binary, e := os.Executable()
		if e != nil {
			return e
		}
		plist, e := deployment.RenderLaunchdPlist(deployment.LaunchdConfig{Home: home, BinaryPath: binary, DataRoot: *data, Port: *port, TailnetHost: *tailnet, DolgoraeExecutable: *executable, WorkspaceRoots: roots, Policies: policies})
		if e != nil {
			return e
		}
		_, e = os.Stdout.Write(plist)
		return e
	}
	if mode == "diagnose" {
		if !filepath.IsAbs(*data) || filepath.Clean(*data) != *data {
			return errors.New("Gul data directory must be absolute and clean")
		}
		if _, err := os.Lstat(*data); errors.Is(err, os.ErrNotExist) {
			return errors.New("no Gul owner state at the selected directory; check the path or start Gul")
		}
		if _, e := host.Verify(ctx, *data); e != nil {
			return e
		}
		if *tailnet != "" {
			return host.CheckTailnet(ctx, *data, *tailnet)
		}
		fmt.Fprintln(os.Stdout, "Gul local HTTPS owner verified; remote access not selected")
		return nil
	}
	h := host.New(host.Config{DataDirectory: *data, Port: *port, TailnetHost: *tailnet, DolgoraeExecutable: *executable, WorkspaceRoots: roots, Policies: policies, ServeInspector: host.TailscaleInspector{HostPort: *tailnet}})
	e = h.Start(ctx)
	owned := e == nil
	if e != nil && (mode == "serve" || !errors.Is(e, host.ErrAlreadyRunning)) {
		return e
	}
	if owned {
		defer func() {
			stop, c := context.WithTimeout(context.Background(), 10*time.Second)
			defer c()
			result = errors.Join(result, h.Stop(stop))
		}()
	}
	if mode == "serve" {
		e = h.Wait(ctx)
		if errors.Is(e, context.Canceled) {
			return nil
		}
		return e
	}
	view, e := host.Attach(ctx, *data)
	if e != nil {
		return e
	}
	// Only a verified attachment reaches the native WebView. An attached window
	// never receives lifecycle authority over the existing headless owner.
	return openDesktop(view, cancel)
}

type repeatedStrings []string

func (s *repeatedStrings) String() string         { return fmt.Sprint([]string(*s)) }
func (s *repeatedStrings) Set(value string) error { *s = append(*s, value); return nil }
