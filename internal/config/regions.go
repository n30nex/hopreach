package config

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
)

// RegionsConfig enables separately cached regional maps. Entries are an
// operator-curated Canadian IATA catalogue, not nearest-airport assignments.
// The observation backend remains the authority for membership.
type RegionsConfig struct {
	DefaultIATA string       `yaml:"default_iata" json:"defaultIata"`
	Entries     []IATARegion `yaml:"entries" json:"entries"`
}

type IATARegion struct {
	IATA      string  `yaml:"iata" json:"iata"`
	Name      string  `yaml:"name" json:"name"`
	CenterLat float64 `yaml:"center_lat" json:"centerLat"`
	CenterLon float64 `yaml:"center_lon" json:"centerLon"`
	Zoom      int     `yaml:"zoom" json:"zoom"`
	// RadiusKm bounds computation even if an observer reports a distant node.
	RadiusKm float64 `yaml:"radius_km" json:"radiusKm"`
}

var iataCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (c Config) validateRegions() error {
	if c.Source.RequestConcurrency < 1 || c.Source.RequestConcurrency > 12 {
		return fmt.Errorf("source.request_concurrency must be between 1 and 12")
	}
	if len(c.Regions.Entries) == 0 {
		return nil
	}
	if !finite(c.CoreScope.RequestTimeoutSeconds) || c.CoreScope.RequestTimeoutSeconds <= 0 || c.CoreScope.RequestTimeoutSeconds > 120 {
		return fmt.Errorf("regional corescope.request_timeout_seconds must be in (0,120]")
	}
	if len(c.Regions.Entries) > 64 {
		return fmt.Errorf("at most 64 regional maps may be configured")
	}
	t, err := c.Resolve()
	if err != nil || t != SourceCoreScope {
		return fmt.Errorf("regional maps require source.type corescope")
	}
	if c.Region.BoundaryPath == "" {
		return fmt.Errorf("regional maps require region.boundary_path for the Canadian geofence")
	}
	seen := make(map[string]bool, len(c.Regions.Entries))
	for _, r := range c.Regions.Entries {
		if !iataCode.MatchString(r.IATA) || seen[r.IATA] {
			return fmt.Errorf("invalid or duplicate regional IATA %q", r.IATA)
		}
		if r.Name == "" || !finite(r.CenterLat) || !finite(r.CenterLon) || r.CenterLat < 40 || r.CenterLat > 84 || r.CenterLon < -142 || r.CenterLon > -52 {
			return fmt.Errorf("region %s requires a Canadian name and map centre", r.IATA)
		}
		if !finite(r.RadiusKm) || r.RadiusKm <= 0 || r.RadiusKm > 250 || r.Zoom < 1 || r.Zoom > 18 {
			return fmt.Errorf("region %s requires radius_km in (0,250] and zoom in [1,18]", r.IATA)
		}
		seen[r.IATA] = true
	}
	if !seen[c.Regions.DefaultIATA] {
		return fmt.Errorf("regions.default_iata must name a configured region")
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// ForIATA derives a single run's immutable configuration. The list remains in
// place for frontend preparation; callers must not recursively run a batch.
func (c Config) ForIATA(iata string) (Config, error) {
	for _, r := range c.Regions.Entries {
		if r.IATA != iata {
			continue
		}
		c.CoreScope.Region = r.IATA
		c.OutputDir = filepath.ToSlash(filepath.Join(c.OutputDir, "regions", r.IATA))
		c.Region.Name = "Canada / " + r.IATA + " — " + r.Name
		c.Map.CenterLat, c.Map.CenterLon, c.Map.Zoom = r.CenterLat, r.CenterLon, r.Zoom
		return c, nil
	}
	return Config{}, fmt.Errorf("unknown configured IATA %q", iata)
}
