package meshapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"hopreach/internal/meshsource"
)

// RegisterRegions mounts disjoint API namespaces. A region must be explicitly
// configured; neither a typo nor a missing IATA falls back to a global source.
// The shared gate caps observation-backend work across every region, while a
// small admission limit keeps queued request goroutines bounded.
func RegisterRegions(mux *http.ServeMux, sources map[string]meshsource.Source, concurrency int, timeout time.Duration) {
	handlers := make(map[string]*http.ServeMux, len(sources))
	for code, src := range sources {
		h := http.NewServeMux()
		New(src).Register(h)
		handlers[code] = h
	}
	gate := make(chan struct{}, max(1, min(concurrency, 12)))
	admitted := make(chan struct{}, 32)
	mux.HandleFunc(Prefix, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, Prefix), "/", 3)
		if len(parts) != 3 || parts[0] != "regions" || handlers[parts[1]] == nil {
			httpError(w, http.StatusNotFound, "unknown configured Canadian IATA region")
			return
		}
		if strings.HasPrefix(parts[2], "api/packets") {
			httpError(w, http.StatusNotImplemented, "Packet replay is unavailable: the observation backend cannot isolate complete packet observations by IATA. Terrain planning and synthetic flood simulation remain available.")
			return
		}
		select {
		case admitted <- struct{}{}:
			defer func() { <-admitted }()
		default:
			httpError(w, http.StatusServiceUnavailable, "regional API is busy; retry shortly")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		case <-ctx.Done():
			httpError(w, http.StatusGatewayTimeout, "regional API request timed out")
			return
		}
		request := r.Clone(ctx)
		request.URL.Path = Prefix + parts[2]
		handlers[parts[1]].ServeHTTP(w, request)
	})
}
