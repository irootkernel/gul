// This opt-in driver controls only an isolated production host lifecycle.
// It never injects a Provider, Features or Assemble override.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	gv "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/host"
)

func main() {
	if os.Getenv("GUL_RUN_PUBLISHED_ACCEPTANCE") != "1" {
		panic("published acceptance requires explicit isolated opt-in")
	}
	if len(os.Args) != 4 {
		panic("usage: browser-driver ROOT PUBLISHED_BINARY PORT")
	}
	root := os.Args[1]
	port, err := strconv.Atoi(os.Args[3])
	must(err)
	h := host.New(host.Config{DataDirectory: filepath.Join(root, "Gul"), Port: port, DolgoraeExecutable: os.Args[2], ProviderHome: filepath.Join(root, "home"), WorkspaceRoots: []string{filepath.Join(root, "workspace")}, Policies: []string{"carrier-test"}})
	ctx := context.Background()
	start, cancel := context.WithTimeout(ctx, 30*time.Second)
	must(h.Start(start))
	cancel()
	defer func() { stop, cancel := context.WithTimeout(ctx, 10*time.Second); defer cancel(); must(h.Stop(stop)) }()
	cert, err := x509.ParseCertificate(h.CertificateDER())
	must(err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 40 * time.Second}
	auth := gulv1connect.NewAuthServiceClient(client, h.Origin())
	attachment, err := host.Attach(ctx, filepath.Join(root, "Gul"))
	must(err)
	setup := connect.NewRequest(&gv.FirstRunSetupRequest{Password: []byte("published browser fixture password")})
	setup.Header().Set("Origin", h.Origin())
	setup.Header().Set(api.BrowserHeader, "1")
	setup.Header().Set(api.SetupHeader, attachment.SetupCredential)
	_, err = auth.FirstRunSetup(ctx, setup)
	must(err)
	login := connect.NewRequest(&gv.LoginRequest{Password: []byte("published browser fixture password")})
	login.Header().Set("Origin", h.Origin())
	login.Header().Set(api.BrowserHeader, "1")
	logged, err := auth.Login(ctx, login)
	must(err)
	register := connect.NewRequest(&gv.RegisterFromAllowlistPathRequest{RootId: "root-1", RelativePath: "."})
	register.Header().Set("Origin", h.Origin())
	register.Header().Set(api.BrowserHeader, "1")
	register.Header().Set(api.CSRFHeader, logged.Msg.Session.CsrfToken)
	register.Header().Set("Cookie", strings.Split(logged.Header().Get("Set-Cookie"), ";")[0])
	_, err = gulv1connect.NewWorkspacePresentationServiceClient(client, h.Origin()).RegisterFromAllowlistPath(ctx, register)
	if err != nil {
		fmt.Fprintf(os.Stderr, "published host provider status: %+v\n", h.Core.ProviderStatus())
	}
	must(err)
	out := json.NewEncoder(os.Stdout)
	must(out.Encode(map[string]any{"ready": true, "origin": h.Origin()}))
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "restart":
			stop, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = h.Stop(stop)
			cancel()
			if err == nil {
				start, cancel := context.WithTimeout(ctx, 30*time.Second)
				err = h.Start(start)
				cancel()
			}
		case "lose-start-receipt", "restore-start-receipt":
			db, openErr := sql.Open("sqlite", "file:"+filepath.Join(root, "Gul/gul.sqlite"))
			err = openErr
			if err == nil {
				query := `DROP TRIGGER lose_start_receipt`
				if scanner.Text() == "lose-start-receipt" {
					query = `CREATE TRIGGER lose_start_receipt BEFORE UPDATE OF outcome_ref ON mutation_attempt_details WHEN (SELECT operation_kind FROM provider_operation_attempts WHERE operation_id=NEW.operation_id)='StartRun' BEGIN SELECT RAISE(ABORT,'fixture receipt loss'); END`
				}
				_, err = db.ExecContext(ctx, query)
				db.Close()
			}
		case "stop":
			return
		default:
			err = fmt.Errorf("unknown fixture command")
		}
		if err != nil {
			must(out.Encode(map[string]any{"ok": false, "error": err.Error()}))
		} else {
			must(out.Encode(map[string]any{"ok": true}))
		}
	}
	must(scanner.Err())
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
