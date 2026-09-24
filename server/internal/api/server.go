package api

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync/atomic"

	connectcors "connectrpc.com/cors"
	"github.com/rs/cors"
	"go.uber.org/zap"

	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
)

// ServerOptions configures NewServer.
type ServerOptions struct {
	// Port is the single port everything is served on: the UI, the API
	// (gRPC, gRPC-Web and Connect), /download, /healthz and /readyz.
	Port int

	// UI is the built frontend, served from / with a fallback to index.html
	// for client-side routes. Normally ui.FS().
	UI fs.FS

	// APIURL is written into /config.js for the frontend to use as its API
	// base URL. Empty (the default) means the frontend's own origin.
	APIURL string

	// CORSAllowedOrigins lists the origins allowed to call the API from a
	// browser. Empty (the default) disables CORS, which is all a same-origin
	// deployment needs.
	CORSAllowedOrigins []string
}

// Server serves the frontend and the LogService API over one HTTP port.
// Plaintext HTTP/2 is accepted alongside HTTP/1.1 so native gRPC clients
// work without TLS, while browsers use Connect or gRPC-Web over HTTP/1.1.
//
// The server can start listening before the LogService exists, so /healthz
// answers during a long startup (e.g. a storage migration). Until SetService
// is called, /readyz and every API route answer 503.
type Server struct {
	httpServer *http.Server
	api        atomic.Pointer[http.ServeMux]
	log        *zap.Logger

	// cancelRequests cancels the base context of every request, so long-lived
	// streaming RPCs end promptly on shutdown instead of holding it open.
	cancelRequests context.CancelFunc
}

// NewServer creates a Server from opts. Call SetService to start serving the
// API, then Start to begin listening.
func NewServer(opts ServerOptions, log *zap.Logger) *Server {
	s := &Server{log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if s.api.Load() == nil {
			http.Error(w, "starting up", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /config.js", configJSHandler(opts.APIURL))
	mux.Handle("/download", http.HandlerFunc(s.serveAPI))
	mux.Handle("/"+simplelogv1connect.LogServiceName+"/", http.HandlerFunc(s.serveAPI))
	mux.Handle("/", spaHandler(opts.UI))

	var handler http.Handler = mux
	if len(opts.CORSAllowedOrigins) > 0 {
		handler = cors.New(cors.Options{
			AllowedOrigins: opts.CORSAllowedOrigins,
			AllowedMethods: connectcors.AllowedMethods(),
			AllowedHeaders: connectcors.AllowedHeaders(),
			ExposedHeaders: connectcors.ExposedHeaders(),
		}).Handler(handler)
	}

	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	baseCtx, cancel := context.WithCancel(context.Background())
	s.cancelRequests = cancel
	s.httpServer = &http.Server{
		Addr:        fmt.Sprintf(":%d", opts.Port),
		Handler:     handler,
		Protocols:   &protocols,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}
	return s
}

// SetService starts serving svc on the API routes and marks the server ready.
func (s *Server) SetService(svc *LogService) {
	api := http.NewServeMux()
	api.Handle(simplelogv1connect.NewLogServiceHandler(svc))
	api.HandleFunc("/download", downloadHandler(svc))
	s.api.Store(api)
	s.log.Info("API ready")
}

// serveAPI forwards to the API routes once SetService has been called.
func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	api := s.api.Load()
	if api == nil {
		http.Error(w, "starting up", http.StatusServiceUnavailable)
		return
	}
	api.ServeHTTP(w, r)
}

// Start begins listening on the configured address. It blocks until the server
// stops. A nil error means the server was shut down gracefully via Shutdown.
func (s *Server) Start() error {
	s.log.Info("HTTP server listening", zap.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Shutdown ends in-flight streaming RPCs and then gracefully stops the HTTP
// server, waiting for remaining requests until ctx expires.
func (s *Server) Shutdown(ctx context.Context) {
	s.log.Info("HTTP server shutting down")
	s.cancelRequests()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.log.Warn("http server shutdown error", zap.Error(err))
	}
}
