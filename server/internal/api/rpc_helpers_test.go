package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
)

// call invokes a unary handler directly with req, unwrapping the Connect
// request/response envelopes so tests can work with plain messages.
func call[Req, Res any](ctx context.Context, rpc func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error), req *Req) (*Res, error) {
	resp, err := rpc(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// newTestHTTPServer serves svc through a full Server (UI, API and probes)
// on an in-process HTTP server.
func newTestHTTPServer(t *testing.T, svc *LogService, opts ServerOptions) (*Server, *httptest.Server) {
	t.Helper()
	if opts.UI == nil {
		opts.UI = fstest.MapFS{}
	}
	srv := NewServer(opts, zap.NewNop())
	if svc != nil {
		srv.SetService(svc)
	}
	ts := httptest.NewUnstartedServer(srv.httpServer.Handler)
	ts.Config = srv.httpServer
	ts.Start()
	t.Cleanup(ts.Close)
	return srv, ts
}

// newTestClient returns a Connect client for svc served in-process.
func newTestClient(t *testing.T, svc *LogService, opts ...connect.ClientOption) simplelogv1connect.LogServiceClient {
	t.Helper()
	_, ts := newTestHTTPServer(t, svc, ServerOptions{})
	return simplelogv1connect.NewLogServiceClient(ts.Client(), ts.URL, opts...)
}

// lineStream is a server-streaming RPC running against an in-process server.
// Each received message's line is forwarded to lines; the RPC's final error
// (nil, or e.g. Canceled once ctx is cancelled) arrives on err once the
// stream ends.
type lineStream struct {
	lines chan string
	err   chan error
}

// startLineStream calls a line-carrying server-streaming RPC (StreamLogs,
// StreamWorkloadLogs, ...) on client and returns immediately. (Opening a
// server stream blocks until the handler sends its first message, which is
// why this runs in the background.)
func startLineStream[Req, Res any](
	ctx context.Context,
	client simplelogv1connect.LogServiceClient,
	rpc func(simplelogv1connect.LogServiceClient, context.Context, *connect.Request[Req]) (*connect.ServerStreamForClient[Res], error),
	req *Req,
) *lineStream {
	s := &lineStream{lines: make(chan string, 64), err: make(chan error, 1)}
	go func() {
		stream, err := rpc(client, ctx, connect.NewRequest(req))
		if err != nil {
			s.err <- err
			return
		}
		defer stream.Close()
		for stream.Receive() {
			msg := any(stream.Msg()).(interface{ GetLine() string })
			s.lines <- msg.GetLine()
		}
		s.err <- stream.Err()
	}()
	return s
}

// searchResults holds every response a SearchLogs call sent.
type searchResults struct {
	sent []*pb.SearchLogsResponse
}

// search runs SearchLogs against svc to completion.
func search(t *testing.T, svc *LogService, req *pb.SearchLogsRequest) (*searchResults, error) {
	t.Helper()
	stream, err := newTestClient(t, svc).SearchLogs(context.Background(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	results := &searchResults{}
	for stream.Receive() {
		results.sent = append(results.sent, stream.Msg())
	}
	return results, stream.Err()
}

func (r *searchResults) lines() []string {
	var lines []string
	for _, resp := range r.sent {
		if !resp.Truncated {
			lines = append(lines, resp.Line)
		}
	}
	return lines
}

func (r *searchResults) truncated() bool {
	return len(r.sent) > 0 && r.sent[len(r.sent)-1].Truncated
}
