package main

import (
	"net/http"
	"time"

	"hopreach/internal/config"
	"hopreach/internal/meshapi"
	"hopreach/internal/meshsource"
	"hopreach/internal/sources"
)

func registerRegionalAPI(c config.Config) error {
	regionalSources := make(map[string]meshsource.Source, len(c.Regions.Entries))
	for _, r := range c.Regions.Entries {
		selected, err := c.ForIATA(r.IATA)
		if err != nil {
			return err
		}
		src, err := sources.FromConfig(selected)
		if err != nil {
			return err
		}
		regionalSources[r.IATA] = src
	}
	meshapi.RegisterRegions(http.DefaultServeMux, regionalSources, c.Source.RequestConcurrency,
		time.Duration(c.CoreScope.RequestTimeoutSeconds*float64(time.Second)))
	return nil
}
