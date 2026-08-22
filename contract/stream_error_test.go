package contract_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	connectv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"
	"github.com/rootkernel/gul/contract/generated/go/fake"
)

func TestStreamingTypedErrorDetailRoundTrip(t *testing.T) {
	rpcError := connect.NewError(connect.CodeResourceExhausted, errors.New("typed stream fixture"))
	detail, err := connect.NewErrorDetail(&v1.DolgoraeErrorDetail{})
	if err != nil {
		t.Fatal(err)
	}
	rpcError.AddDetail(detail)

	server := httptest.NewUnstartedServer(fake.NewHandlerWithServer(&fake.Server{Error: rpcError}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	client := connectv1.NewObservationServiceClient(server.Client(), server.URL, connect.WithGRPC())
	stream, err := client.WatchRunEvents(context.Background(), connect.NewRequest(&v1.WatchRunEventsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	for stream.Receive() {
	}
	var connectErr *connect.Error
	if !errors.As(stream.Err(), &connectErr) {
		t.Fatalf("expected connect error, got %v", stream.Err())
	}
	details := connectErr.Details()
	if len(details) != 1 {
		t.Fatalf("expected one typed detail, got %d", len(details))
	}
	value, err := details[0].Value()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value.(*v1.DolgoraeErrorDetail); !ok {
		t.Fatalf("unexpected typed detail %T", value)
	}
}
