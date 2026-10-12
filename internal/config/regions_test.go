package config

import "testing"

func regionalConfig() Config {
	c := Default()
	c.Region.BoundaryPath = "canada.geojson"
	c.Regions = RegionsConfig{DefaultIATA: "YKF", Entries: []IATARegion{
		{IATA: "YKF", Name: "Waterloo", CenterLat: 43.46, CenterLon: -80.38, Zoom: 9, RadiusKm: 100},
		{IATA: "YYZ", Name: "Toronto", CenterLat: 43.68, CenterLon: -79.63, Zoom: 9, RadiusKm: 100},
	}}
	return c
}

func TestRegionsValidateAndSelect(t *testing.T) {
	c := regionalConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	y, err := c.ForIATA("YYZ")
	if err != nil {
		t.Fatal(err)
	}
	if y.CoreScope.Region != "YYZ" || y.OutputDir != "public/data/regions/YYZ" || y.Map.CenterLon != -79.63 {
		t.Fatalf("wrong region config: %+v", y)
	}
	if _, err := c.ForIATA("../SEA"); err == nil {
		t.Fatal("invalid region accepted")
	}
	if c.CoreScope.Region != "" {
		t.Fatal("selection mutated base config")
	}
}

func TestRegionsRejectUnsafeConfig(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"unknown default":  func(c *Config) { c.Regions.DefaultIATA = "SEA" },
		"unsafe code":      func(c *Config) { c.Regions.Entries[0].IATA = "../" },
		"duplicate":        func(c *Config) { c.Regions.Entries[1].IATA = "YKF" },
		"unbounded radius": func(c *Config) { c.Regions.Entries[0].RadiusKm = 1000 },
		"missing fence":    func(c *Config) { c.Region.BoundaryPath = "" },
		"wrong source":     func(c *Config) { c.Source.Type = "beacon" },
	} {
		t.Run(name, func(t *testing.T) {
			c := regionalConfig()
			change(&c)
			if c.Validate() == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}
