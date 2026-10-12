package meshapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hopreach/internal/meshsource"
)

func TestRegionalAPINamespacesNeverFallBack(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRegions(mux, map[string]meshsource.Source{"YKF": &fakeSource{nodes: []meshsource.Node{{PublicKey: "waterloo"}}}, "YYZ": &fakeSource{nodes: []meshsource.Node{{PublicKey: "toronto"}}}}, 1, time.Second)
	for _, c := range []struct {
		path, want string
		status     int
	}{
		{"/mesh-api/regions/YKF/api/nodes", "waterloo", 200},
		{"/mesh-api/regions/YYZ/api/nodes", "toronto", 200},
		{"/mesh-api/regions/SEA/api/nodes", "unknown", 404},
		{"/mesh-api/api/nodes", "unknown", 404},
		{"/mesh-api/regions/YKF/api/packets/abcd", "Packet replay is unavailable", 501},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", c.path, nil))
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.want) {
			t.Fatalf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
	}
}

type blockingSource struct {
	fakeSource
	calls   atomic.Int32
	entered chan struct{}
}

func (s *blockingSource) FetchRepeaters(ctx context.Context) ([]meshsource.Node, error) {
	s.calls.Add(1)
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestRegionalAPISharedRequestBudget(t *testing.T) {
	s := &blockingSource{entered: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	RegisterRegions(mux, map[string]meshsource.Source{"YKF": s, "YYZ": s}, 1, 50*time.Millisecond)
	done := make(chan struct{})
	go func() {
		defer close(done)
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/mesh-api/regions/YKF/api/nodes", nil))
	}()
	<-s.entered
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/mesh-api/regions/YYZ/api/nodes", nil).WithContext(ctx))
	if w.Code != http.StatusGatewayTimeout || s.calls.Load() != 1 {
		t.Fatalf("status=%d calls=%d", w.Code, s.calls.Load())
	}
	<-done
}
