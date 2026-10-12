package meshmapper

import (
	"encoding/json"
	"fmt"
	"os"
)

type cellSpool struct {
	region Region
	data   regionData
	file   *os.File
}

func (s *cellSpool) append(c Cell, dir string, maxCells int) error {
	if s.data.Count >= maxCells {
		s.data.Truncated = true
		return nil
	}
	if s.file == nil {
		var err error
		s.file, err = os.CreateTemp(dir, "cells-*.jsonl")
		if err != nil {
			return err
		}
		s.data.path = s.file.Name()
	}
	c.Timestamp, c.FirstSeen = seconds(c.Timestamp), seconds(c.FirstSeen)
	if err := json.NewEncoder(s.file).Encode(c); err != nil {
		return err
	}
	s.data.Count++
	return nil
}

// The normal region-first envelope needs one spool. If its IATA comes after
// the array, keep a separate capped candidate for each configured region
// (at most 64). A distant region can never consume the selected region's cap.
// Memory remains one cell plus fixed per-region counters/file handles.
func readCells(d *json.Decoder, iata, dir string, regions []Region, contains func(float64, float64) bool, maxCells int) (result map[string]regionData, err error) {
	if err = delimiter(d, '['); err != nil {
		return nil, err
	}
	var spools []cellSpool
	for _, r := range regions {
		if iata == "" || r.IATA == iata {
			spools = append(spools, cellSpool{region: r})
		}
	}
	defer func() {
		result = make(map[string]regionData, len(spools))
		for _, s := range spools {
			if s.file != nil {
				if closeErr := s.file.Close(); err == nil && closeErr != nil {
					err = closeErr
				}
			}
			result[s.region.IATA] = s.data
		}
	}()
	for d.More() {
		if len(spools) == 0 {
			var ignored struct{}
			if err = d.Decode(&ignored); err != nil {
				return nil, err
			}
			continue
		}
		var c Cell
		if err = d.Decode(&c); err != nil {
			return nil, fmt.Errorf("invalid MeshMapper cell encoding: %w", err)
		}
		if err = stageCell(c, spools, dir, contains, maxCells); err != nil {
			return nil, err
		}
	}
	return nil, delimiter(d, ']')
}

func stageCell(c Cell, spools []cellSpool, dir string, contains func(float64, float64) bool, maxCells int) error {
	valid := validCell(c)
	countryChecked, inCountry := false, false
	for i := range spools {
		s := &spools[i]
		if !valid {
			s.data.Invalid++
			continue
		}
		lat, lon := (c.Bounds.South+c.Bounds.North)/2, (c.Bounds.West+c.Bounds.East)/2
		if !withinRegion([]Region{s.region}, s.region.IATA, lat, lon) {
			s.data.Excluded++
			continue
		}
		if !countryChecked {
			inCountry, countryChecked = contains(lat, lon), true
		}
		if !inCountry {
			s.data.Excluded++
			continue
		}
		if err := s.append(c, dir, maxCells); err != nil {
			return err
		}
	}
	return nil
}
