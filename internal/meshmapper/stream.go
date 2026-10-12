// Package meshmapper imports a protected global coverage feed into bounded,
// public Canadian regional snapshots. It never retains the world feed.
package meshmapper

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"hopreach/internal/propagation"
)

type Bounds struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}
type Cell struct {
	ID         string   `json:"grid_id"`
	Bounds     *Bounds  `json:"bounds"`
	Type       string   `json:"coverage_type"`
	Fill       string   `json:"fill_color"`
	Border     string   `json:"border_color"`
	SNR        *float64 `json:"snr"`
	Timestamp  *int64   `json:"timestamp"`
	Count      int      `json:"count"`
	SNRMin     *float64 `json:"snr_min"`
	SNRMax     *float64 `json:"snr_max"`
	StatusMask int      `json:"status_mask"`
	FirstSeen  *int64   `json:"first_seen"`
	Noise      *float64 `json:"noise"`
	Effective  *float64 `json:"effective"`
}
type Region struct {
	IATA               string
	Lat, Lon, RadiusKm float64
}
type feedMeta struct {
	Success        bool   `json:"success"`
	Global         bool   `json:"global"`
	Schema         int    `json:"schema_version"`
	GeneratedAt    *int64 `json:"generated_at"`
	RegionCount    int    `json:"region_count"`
	RegionsSkipped int    `json:"regions_skipped"`
}
type regionData struct {
	IATA        string `json:"region"`
	Count       int    `json:"cell_count"`
	Invalid     int    `json:"invalid_cells"`
	Excluded    int    `json:"excluded_cells"`
	Truncated   bool   `json:"truncated"`
	SourceTotal int    `json:"source_total_squares"`
	path        string
}

func parseFeed(r io.Reader, dir string, regions []Region, contains func(float64, float64) bool, maxCells int) (feedMeta, map[string]regionData, error) {
	d := json.NewDecoder(r)
	d.UseNumber()
	out := make(map[string]regionData, len(regions))
	var meta feedMeta
	haveRegions := false
	if err := delimiter(d, '{'); err != nil {
		return meta, nil, err
	}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return meta, nil, err
		}
		switch key {
		case "success":
			err = d.Decode(&meta.Success)
		case "global":
			err = d.Decode(&meta.Global)
		case "schema_version":
			err = d.Decode(&meta.Schema)
		case "generated_at":
			err = d.Decode(&meta.GeneratedAt)
		case "region_count":
			err = d.Decode(&meta.RegionCount)
		case "regions_skipped":
			err = d.Decode(&meta.RegionsSkipped)
		case "regions":
			haveRegions = true
			err = parseRegions(d, dir, regions, contains, maxCells, out)
		case "grid_squares":
			return meta, nil, fmt.Errorf("global MeshMapper key required; merged grids cannot be assigned to IATAs")
		default:
			err = skipValue(d, 0)
		}
		if err != nil {
			return meta, nil, err
		}
	}
	if err := delimiter(d, '}'); err != nil {
		return meta, nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return meta, nil, fmt.Errorf("unexpected content after coverage envelope")
	}
	if !haveRegions || !meta.Success || !meta.Global || meta.Schema != 2 || meta.GeneratedAt == nil || *meta.GeneratedAt <= 0 {
		return meta, nil, fmt.Errorf("expected successful global MeshMapper schema 2 envelope")
	}
	meta.GeneratedAt = seconds(meta.GeneratedAt)
	return meta, out, nil
}

func parseRegions(d *json.Decoder, dir string, regions []Region, contains func(float64, float64) bool, maxCells int, out map[string]regionData) error {
	if err := delimiter(d, '['); err != nil {
		return fmt.Errorf("global regions array required")
	}
	for count := 0; d.More(); count++ {
		if count >= 10000 {
			return fmt.Errorf("too many MeshMapper region sections")
		}
		result, err := parseRegion(d, dir, regions, contains, maxCells)
		if err != nil {
			return err
		}
		if !allowed(regions, result.IATA) {
			if result.path != "" {
				_ = os.Remove(result.path)
			}
			continue
		}
		if _, exists := out[result.IATA]; exists {
			return fmt.Errorf("duplicate MeshMapper IATA section")
		}
		out[result.IATA] = result
	}
	return delimiter(d, ']')
}

func parseRegion(d *json.Decoder, dir string, regions []Region, contains func(float64, float64) bool, maxCells int) (result regionData, err error) {
	if err = delimiter(d, '{'); err != nil {
		return result, err
	}
	var f *os.File
	haveCells := false
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	for d.More() {
		key, e := d.Token()
		if e != nil {
			return result, e
		}
		switch key {
		case "region":
			err = d.Decode(&result.IATA)
		case "total_squares":
			err = d.Decode(&result.SourceTotal)
		case "grid_squares":
			if haveCells {
				return result, fmt.Errorf("duplicate grid_squares array")
			}
			haveCells = true
			err = readCells(d, &result, &f, dir, regions, contains, maxCells)
		default:
			err = skipValue(d, 0)
		}
		if err != nil {
			return result, err
		}
	}
	if err = delimiter(d, '}'); err != nil {
		return result, err
	}
	if allowed(regions, result.IATA) && !haveCells {
		return result, fmt.Errorf("selected MeshMapper region has no grid_squares array")
	}
	if f != nil {
		err = f.Close()
		f = nil
	}
	return result, err
}

func readCells(d *json.Decoder, result *regionData, f **os.File, dir string, regions []Region, contains func(float64, float64) bool, maxCells int) error {
	if err := delimiter(d, '['); err != nil {
		return err
	}
	for d.More() {
		// A known foreign region is discarded one object at a time, without
		// materialising a region-sized value. JSON object field order is not
		// assumed: before region is known, stage only Canadian/local cells.
		if result.IATA != "" && !allowed(regions, result.IATA) {
			var ignored struct{}
			if err := d.Decode(&ignored); err != nil {
				return err
			}
			continue
		}
		var c Cell
		if err := d.Decode(&c); err != nil {
			return fmt.Errorf("invalid MeshMapper cell encoding: %w", err)
		}
		if !validCell(c) {
			result.Invalid++
			continue
		}
		lat, lon := (c.Bounds.South+c.Bounds.North)/2, (c.Bounds.West+c.Bounds.East)/2
		if !withinRegion(regions, result.IATA, lat, lon) || !contains(lat, lon) {
			result.Excluded++
			continue
		}
		if result.Count >= maxCells {
			result.Truncated = true
			continue
		}
		if *f == nil {
			var err error
			*f, err = os.CreateTemp(dir, "cells-*.jsonl")
			if err != nil {
				return err
			}
			result.path = (*f).Name()
		}
		c.Timestamp, c.FirstSeen = seconds(c.Timestamp), seconds(c.FirstSeen)
		if err := json.NewEncoder(*f).Encode(c); err != nil {
			return err
		}
		result.Count++
	}
	return delimiter(d, ']')
}

func allowed(regions []Region, code string) bool {
	for _, r := range regions {
		if r.IATA == code {
			return true
		}
	}
	return false
}
func withinRegion(regions []Region, code string, lat, lon float64) bool {
	for _, r := range regions {
		if (code == "" || r.IATA == code) && math.Abs(lat-r.Lat) <= r.RadiusKm/110 && propagation.HaversineKm(lat, lon, r.Lat, r.Lon) <= r.RadiusKm {
			return true
		}
	}
	return false
}
func validCell(c Cell) bool {
	b := c.Bounds
	return b != nil && len(c.ID) <= 100 && c.Count >= 0 && b.South >= -90 && b.North <= 90 && b.South < b.North && b.West >= -180 && b.East <= 180 && b.West < b.East &&
		b.North-b.South <= .1 && b.East-b.West <= .1 && strings.Contains("|BIDIR|TX|RX|DISC|DEAD|DROP|", "|"+c.Type+"|") && c.Type != ""
}
func seconds(t *int64) *int64 {
	if t == nil || *t <= 0 {
		return nil
	}
	v := *t
	if v > 2e10 {
		v /= 1000
	}
	return &v
}
func delimiter(d *json.Decoder, want byte) error {
	t, e := d.Token()
	if e != nil {
		return e
	}
	if t != json.Delim(want) {
		return fmt.Errorf("invalid coverage JSON container")
	}
	return nil
}
func skipValue(d *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("coverage JSON nesting limit exceeded")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); ok {
		if delim != '{' && delim != '[' {
			return fmt.Errorf("invalid coverage JSON value")
		}
		for d.More() {
			if delim == '{' {
				if _, err = d.Token(); err != nil {
					return err
				}
			}
			if err = skipValue(d, depth+1); err != nil {
				return err
			}
		}
		_, err = d.Token()
	}
	return err
}

// byteBudget makes an oversized streamed feed an explicit failure rather than
// allowing an artificial EOF to look like a complete last-good snapshot.
type byteBudget struct {
	r         io.Reader
	remaining int64
	label     string
}

func (b *byteBudget) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.r.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("MeshMapper %s byte budget exceeded", b.label)
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.r.Read(p)
	b.remaining -= int64(n)
	return n, err
}
