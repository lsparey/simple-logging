package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// Server wraps a gRPC server with a gRPC-Web HTTP/1.1 handler so browser
// clients can connect directly without a proxy.
type Server struct {
	grpcServer *grpc.Server
	httpServer *http.Server
	log        *zap.Logger
}

// NewServer creates a Server that serves svc over gRPC-Web on the given port.
// When enableDebug is true, plain JSON REST endpoints are also mounted at /debug/*
// for testing purposes.
func NewServer(port int, svc *LogService, enableDebug bool, log *zap.Logger) *Server {
	grpcSrv := grpc.NewServer()
	pb.RegisterLogServiceServer(grpcSrv, svc)

	wrappedGrpc := grpcweb.WrapServer(
		grpcSrv,
		// Allow all origins; the app has no auth and is expected to run inside
		// a cluster. Tighten this if TLS + public exposure is added later.
		grpcweb.WithOriginFunc(func(_ string) bool { return true }),
		grpcweb.WithAllowedRequestHeaders([]string{"*"}),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if enableDebug {
		registerDebugRoutes(mux, svc)
		log.Info("REST debug endpoints enabled at /debug/*")
	}

	// Catch-all: only forward genuine gRPC / gRPC-Web requests to the gRPC
	// server. Any other request (e.g. a plain browser GET to an unknown path)
	// gets a 404 instead of the confusing "invalid gRPC request method" error.
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if wrappedGrpc.IsGrpcWebRequest(r) ||
			wrappedGrpc.IsGrpcWebSocketRequest(r) ||
			strings.HasPrefix(ct, "application/grpc") {
			wrappedGrpc.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return &Server{
		grpcServer: grpcSrv,
		httpServer: httpSrv,
		log:        log,
	}
}

// Start begins listening on the configured address. It blocks until the server
// stops. A nil error means the server was shut down gracefully via Shutdown.
func (s *Server) Start() error {
	s.log.Info("gRPC-Web server listening", zap.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("grpc-web server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server (waiting for in-flight requests)
// and then the underlying gRPC server.
func (s *Server) Shutdown(ctx context.Context) {
	s.log.Info("gRPC-Web server shutting down")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.log.Warn("http server shutdown error", zap.Error(err))
	}
	s.grpcServer.GracefulStop()
}
