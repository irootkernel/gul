// This fixture uses the same verified private attach as the native host. It
// never prints the grant or places it in browser storage.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/host"
)

func main() {
	if os.Getenv("GUL_RUN_DELIVERY_FIXTURE") != "1" {
		panic("delivery setup is an opt-in isolated test fixture")
	}
	if len(os.Args) != 2 {
		panic("usage: delivery-setup DATA_DIRECTORY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	view, err := host.Attach(ctx, os.Args[1])
	if err != nil || view.SetupCredential == "" {
		panic("verified native setup grant unavailable")
	}
	certificate, err := x509.ParseCertificate(view.CertificateDER)
	if err != nil {
		panic(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}
	defer transport.CloseIdleConnections()
	client := gulv1connect.NewAuthServiceClient(&http.Client{Transport: transport, Timeout: 10 * time.Second}, view.Origin)
	password := []byte("chrome fixture password 2026")
	defer clear(password)
	request := connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: password})
	request.Header().Set("Origin", view.Origin)
	request.Header().Set(api.BrowserHeader, "1")
	request.Header().Set(api.SetupHeader, view.SetupCredential)
	if _, err = client.FirstRunSetup(ctx, request); err != nil {
		panic(errors.New("isolated native account setup failed"))
	}
}
