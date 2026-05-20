package api

// REST debug endpoints — for testing only.
//
// Mount these with registerDebugRoutes when REST_DEBUG=true. They expose the
// same data as the gRPC-Web API via plain JSON over HTTP/1.1 so the service
// can be exercised with curl or a browser without a gRPC-Web client.
//
// Endpoints:
//   GET /debug/namespaces
//   GET /debug/pods?namespace=<ns>
//   GET /debug/logs?namespace=<ns>&pod=<pod>[&page_size=N][&page_token=T][&start_time=<unix>][&end_time=<unix>]

import (
	"encoding/json"
	"net/http"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// registerDebugRoutes mounts the /debug/* REST handlers onto mux.
func registerDebugRoutes(mux *http.ServeMux, svc *LogService) {
	mux.HandleFunc("/debug/namespaces", debugNamespacesHandler(svc))
	mux.HandleFunc("/debug/pods", debugPodsHandler(svc))
	mux.HandleFunc("/debug/logs", debugLogsHandler(svc))
}

func debugNamespacesHandler(svc *LogService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeDebugError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		resp, err := svc.ListNamespaces(r.Context(), &pb.ListNamespacesRequest{})
		if err != nil {
			writeDebugGRPCError(w, err)
			return
		}

		// Return an empty array rather than null when there are no namespaces.
		namespaces := resp.Namespaces
		if namespaces == nil {
			namespaces = []string{}
		}
		writeDebugJSON(w, http.StatusOK, map[string]any{
			"namespaces": namespaces,
		})
	}
}

func debugPodsHandler(svc *LogService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeDebugError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ns := r.URL.Query().Get("namespace")
		if ns == "" {
			writeDebugError(w, http.StatusBadRequest, "namespace query parameter is required")
			return
		}

		resp, err := svc.ListPods(r.Context(), &pb.ListPodsRequest{Namespace: ns})
		if err != nil {
			writeDebugGRPCError(w, err)
			return
		}

		type podJSON struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Active    bool   `json:"active"`
		}
		pods := make([]podJSON, len(resp.Pods))
		for i, p := range resp.Pods {
			pods[i] = podJSON{Name: p.Name, Namespace: p.Namespace, Active: p.Active}
		}
		writeDebugJSON(w, http.StatusOK, map[string]any{"pods": pods})
	}
}

func debugLogsHandler(svc *LogService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeDebugError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		q := r.URL.Query()
		req := &pb.GetLogsRequest{
			Namespace: q.Get("namespace"),
			Pod:       q.Get("pod"),
			PageToken: q.Get("page_token"),
		}

		if v := q.Get("page_size"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeDebugError(w, http.StatusBadRequest, "page_size must be a positive integer")
				return
			}
			req.PageSize = int32(n)
		}

		if v := q.Get("start_time"); v != "" {
			t, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				writeDebugError(w, http.StatusBadRequest, "start_time must be a Unix timestamp (integer seconds)")
				return
			}
			req.StartTime = t
		}

		if v := q.Get("end_time"); v != "" {
			t, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				writeDebugError(w, http.StatusBadRequest, "end_time must be a Unix timestamp (integer seconds)")
				return
			}
			req.EndTime = t
		}

		resp, err := svc.GetLogs(r.Context(), req)
		if err != nil {
			writeDebugGRPCError(w, err)
			return
		}

		lines := resp.Lines
		if lines == nil {
			lines = []string{}
		}
		writeDebugJSON(w, http.StatusOK, map[string]any{
			"lines":           lines,
			"next_page_token": resp.NextPageToken,
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeDebugJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeDebugError(w http.ResponseWriter, code int, msg string) {
	writeDebugJSON(w, code, map[string]string{"error": msg})
}

// writeDebugGRPCError maps a gRPC status error to an appropriate HTTP status code.
func writeDebugGRPCError(w http.ResponseWriter, err error) {
	httpCode := http.StatusInternalServerError
	msg := err.Error()

	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.NotFound:
			httpCode = http.StatusNotFound
		case codes.InvalidArgument:
			httpCode = http.StatusBadRequest
		case codes.Internal:
			httpCode = http.StatusInternalServerError
		}
		msg = s.Message()
	}

	writeDebugError(w, httpCode, msg)
}
