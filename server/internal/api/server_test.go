package api

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func newInProcessServer(t *testing.T, svc *LogService) pb.LogServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	pb.RegisterLogServiceServer(grpcSrv, svc)

	go grpcSrv.Serve(lis) //nolint:errcheck
	t.Cleanup(func() { grpcSrv.Stop() })

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return pb.NewLogServiceClient(conn)
}

func TestServerIntegration_ListNamespaces(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "default"), 0755)
	os.MkdirAll(filepath.Join(dir, "monitoring"), 0755)

	client := newInProcessServer(t, NewLogService(dir, &fakeChecker{}, noopDeploymentMapper{}))

	resp, err := client.ListNamespaces(context.Background(), &pb.ListNamespacesRequest{})
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}

	got := make(map[string]bool)
	for _, ns := range resp.Namespaces {
		got[ns] = true
	}
	for _, want := range []string{"default", "monitoring"} {
		if !got[want] {
			t.Errorf("missing namespace %q in response", want)
		}
	}
}

func TestServerIntegration_GetLogs_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		"2026-05-20T10:00:00Z [default/pod/app] alpha",
		"2026-05-20T10:00:01Z [default/pod/app] beta",
	}
	writeLogFile(t, dir, "default", "pod", lines)

	client := newInProcessServer(t, NewLogService(dir, &fakeChecker{}, noopDeploymentMapper{}))

	resp, err := client.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default",
		Pod:       "pod",
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(resp.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(resp.Lines))
	}
}

// Ensure NewServer compiles and wires correctly (smoke test).
func TestNewServer_Smoke(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, noopDeploymentMapper{})
	srv := NewServer(0, svc, false, zap.NewNop())
	if srv == nil {
		t.Fatal("expected non-nil Server")
	}
}
