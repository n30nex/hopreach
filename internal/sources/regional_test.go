package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hopreach/internal/config"
	"hopreach/internal/meshsource"
)

type regionalFixture struct {
	meshsource.Source
	nodes []meshsource.Node
	calls int
}

func (f *regionalFixture) FetchRepeaters(context.Context) ([]meshsource.Node, error) {
	f.calls++
	return f.nodes, nil
}
func (f *regionalFixture) FetchReach(context.Context, string, int) ([]meshsource.ReachLink, error) {
	out := make([]meshsource.ReachLink, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, meshsource.ReachLink{PublicKey: n.PublicKey, Lat: n.Lat, Lon: n.Lon})
	}
	return out, nil
}

func TestRegionalSourceGeofenceMembershipAndCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "canada.geojson")
	if err := os.WriteFile(path, []byte(`{"type":"Polygon","coordinates":[[[-81,43],[-79,43],[-79,45],[-81,45],[-81,43]]]}`), 0600); err != nil {
		t.Fatal(err)
	}
	node := func(key string, lat, lon float64) meshsource.Node {
		return meshsource.Node{PublicKey: key, Lat: &lat, Lon: &lon}
	}
	f := &regionalFixture{nodes: []meshsource.Node{node("local", 43.46, -80.38), node("border", 42.9, -80.38), node("distant", 44.9, -80.38)}}
	c := config.Default()
	c.Region.BoundaryPath = path
	c.CoreScope.Region = "YKF"
	c.Regions.Entries = []config.IATARegion{{IATA: "YKF", CenterLat: 43.46, CenterLon: -80.38, RadiusKm: 100}}
	s, err := regional(f, c)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := s.FetchRepeaters(context.Background())
	if err != nil || len(nodes) != 1 || nodes[0].PublicKey != "local" {
		t.Fatalf("nodes=%v err=%v", nodes, err)
	}
	links, err := s.FetchReach(context.Background(), "local", 7)
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%v err=%v", links, err)
	}
	if _, err = s.FetchReach(context.Background(), "border", 7); err == nil {
		t.Fatal("out-of-region node accepted")
	}
	if f.calls != 1 {
		t.Fatalf("directory fetched %d times", f.calls)
	}
}
