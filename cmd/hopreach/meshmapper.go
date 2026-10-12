package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"hopreach/internal/config"
	"hopreach/internal/geo"
	"hopreach/internal/meshmapper"
)

func syncMeasured(c config.Config) error {
	if !c.MeshMapper.Enabled {
		return nil
	}
	key, err := meshMapperKey(c.MeshMapper.APIKeyFile)
	if err != nil {
		return err
	}
	b, err := geo.LoadBoundary(c.Region.BoundaryPath, "", "", nil)
	if err != nil {
		return err
	}
	prepared := b.Prepare()
	regions := make([]meshmapper.Region, 0, len(c.Regions.Entries))
	for _, r := range c.Regions.Entries {
		regions = append(regions, meshmapper.Region{IATA: r.IATA, Lat: r.CenterLat, Lon: r.CenterLon, RadiusKm: r.RadiusKm})
	}
	_, err = meshmapper.Sync(context.Background(), meshmapper.Options{
		OutputDir: c.OutputDir, StateDir: c.MeshMapper.StateDir, Key: key, Regions: regions, Contains: prepared.Contains,
		Interval: time.Duration(c.MeshMapper.IntervalHours * float64(time.Hour)), MaxCells: c.MeshMapper.MaxCellsPerRegion,
	})
	return err
}

func meshMapperKey(path string) (string, error) {
	if key := strings.TrimSpace(os.Getenv("MESHMAPPER_API_KEY")); key != "" {
		return key, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read MeshMapper API key file")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return "", fmt.Errorf("invalid MeshMapper API key file")
	}
	return strings.TrimSpace(string(b)), nil
}
