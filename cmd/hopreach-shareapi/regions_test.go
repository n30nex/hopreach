package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hopreach/internal/config"
)

func TestRegisterRegionalAPIUsesConfiguredCanadianSource(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("region") != "YKF" || r.URL.Query().Get("role") != "repeater" {
			t.Errorf("unscoped request %s", r.URL)
		}
		fmt.Fprint(w, `{"nodes":[{"public_key":"canada","lat":43.46,"lon":-80.38},{"public_key":"foreign","lat":42,"lon":-80.38}],"total":2,"has_more":false}`)
	}))
	defer upstream.Close()
	boundary := filepath.Join(t.TempDir(), "country.geojson")
	if err := os.WriteFile(boundary, []byte(`{"type":"Polygon","coordinates":[[[-81,43],[-79,43],[-79,45],[-81,45],[-81,43]]]}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Region.BoundaryPath = boundary
	c.CoreScope.APIURL = upstream.URL
	c.Regions = config.RegionsConfig{DefaultIATA: "YKF", Entries: []config.IATARegion{{IATA: "YKF", Name: "Waterloo", CenterLat: 43.46, CenterLon: -80.38, RadiusKm: 100, Zoom: 9}}}
	previous := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	defer func() { http.DefaultServeMux = previous }()
	if err := registerRegionalAPI(c); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(w, httptest.NewRequest("GET", "/mesh-api/regions/YKF/api/nodes", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "canada") || strings.Contains(w.Body.String(), "foreign") {
		t.Fatalf("response %d %s", w.Code, w.Body.String())
	}
}
