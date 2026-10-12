package sources

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"hopreach/internal/config"
	"hopreach/internal/geo"
	"hopreach/internal/meshsource"
	"hopreach/internal/propagation"
)

// regionalSource combines observed IATA membership (already selected by the
// backend) with the Canadian polygon and a bounded local computation radius.
// Airport distance is a resource limit, never an assignment of membership.
type regionalSource struct {
	meshsource.Source
	boundary geo.Boundary
	region   config.IATARegion
	mu       sync.Mutex
	nodes    []meshsource.Node
	keys     map[string]bool
	expires  time.Time
}

// The country polygon can be several megabytes. All configured regional
// adapters share one immutable parsed copy; changing the file needs a restart.
var countryBoundary struct {
	sync.Mutex
	path  string
	value geo.Boundary
}

func loadCountryBoundary(path string) (geo.Boundary, error) {
	countryBoundary.Lock()
	defer countryBoundary.Unlock()
	if countryBoundary.path == path {
		return countryBoundary.value, nil
	}
	b, err := geo.LoadBoundary(path, "", "", &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return geo.Boundary{}, err
	}
	countryBoundary.path, countryBoundary.value = path, b
	return b, nil
}

func regional(src meshsource.Source, cfg config.Config) (meshsource.Source, error) {
	if len(cfg.Regions.Entries) == 0 {
		return src, nil
	}
	var selected *config.IATARegion
	for i := range cfg.Regions.Entries {
		if cfg.Regions.Entries[i].IATA == cfg.CoreScope.Region {
			selected = &cfg.Regions.Entries[i]
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("regional source requires a configured IATA")
	}
	b, err := loadCountryBoundary(cfg.Region.BoundaryPath)
	if err != nil {
		return nil, err
	}
	if !b.Contains(selected.CenterLat, selected.CenterLon) {
		return nil, fmt.Errorf("%s centre is outside the Canadian boundary", selected.IATA)
	}
	return &regionalSource{Source: src, boundary: b, region: *selected}, nil
}

func (s *regionalSource) contains(lat, lon *float64) bool {
	return lat != nil && lon != nil && s.boundary.Contains(*lat, *lon) &&
		propagation.HaversineKm(s.region.CenterLat, s.region.CenterLon, *lat, *lon) <= s.region.RadiusKm
}

// IATA membership is not a MeshCore radio-scope catalogue. The source's
// global scope endpoint cannot prove completeness for one regional subset.
func (s *regionalSource) Capabilities() meshsource.Capabilities {
	return meshsource.Capabilities{}
}

func (s *regionalSource) FetchScopes(context.Context) ([]string, error) {
	return []string{}, nil
}

func (s *regionalSource) FetchRepeaters(ctx context.Context) ([]meshsource.Node, error) {
	// One cached directory per configured region, capped at 5000 nodes. This
	// also prevents one directory request per link in the simulator.
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.expires) {
		return s.nodes, nil
	}
	nodes, err := s.Source.FetchRepeaters(ctx)
	if err != nil {
		return nil, err
	}
	if len(nodes) > 5000 {
		return nil, fmt.Errorf("regional directory exceeds 5000 nodes")
	}
	selected := make([]meshsource.Node, 0, len(nodes))
	keys := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if s.contains(n.Lat, n.Lon) {
			selected = append(selected, n)
			keys[n.PublicKey] = true
		}
	}
	s.nodes, s.keys, s.expires = selected, keys, time.Now().Add(5*time.Minute)
	return selected, nil
}

func (s *regionalSource) FetchAllNodes(ctx context.Context) ([]meshsource.Node, error) {
	nodes, err := s.Source.FetchAllNodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]meshsource.Node, 0, len(nodes))
	for _, n := range nodes {
		if s.contains(n.Lat, n.Lon) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *regionalSource) FetchReach(ctx context.Context, pubkey string, days int) ([]meshsource.ReachLink, error) {
	if _, err := s.FetchRepeaters(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	// Copy just the membership map reference. Refresh replaces it, never
	// mutates it, so no network work happens while retaining this lock.
	keys := s.keys
	s.mu.Unlock()
	if !keys[pubkey] {
		return nil, fmt.Errorf("node is outside this Canadian IATA region")
	}
	links, err := s.Source.FetchReach(ctx, pubkey, min(max(days, 1), 14))
	if err != nil {
		return nil, err
	}
	out := make([]meshsource.ReachLink, 0, len(links))
	for _, l := range links {
		if keys[l.PublicKey] && s.contains(l.Lat, l.Lon) {
			out = append(out, l)
		}
	}
	return out, nil
}
