package meshmapper

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func catalogHash(regions []Region) string {
	copy := append([]Region(nil), regions...)
	sort.Slice(copy, func(i, j int) bool { return copy[i].IATA < copy[j].IATA })
	b, _ := json.Marshal(copy)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func readState(dir string) (State, error) {
	var s State
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("MeshMapper private quota state is invalid; refusing another request")
	}
	return s, nil
}
func writeState(o Options, s State) error {
	if err := atomicJSON(filepath.Join(o.StateDir, "state.json"), s, 0600); err != nil {
		return err
	}
	status := struct {
		Source         string    `json:"source"`
		LastAttempt    time.Time `json:"last_attempt"`
		LastSuccess    time.Time `json:"last_success"`
		NextAttempt    time.Time `json:"next_attempt"`
		IntervalHours  float64   `json:"interval_hours"`
		HTTPStatus     int       `json:"http_status"`
		Error          string    `json:"error,omitempty"`
		MissingRegions []string  `json:"missing_regions,omitempty"`
	}{"MeshMapper", s.LastAttempt, s.LastSuccess, s.NextAttempt, o.Interval.Hours(), s.HTTPStatus, s.Error, s.MissingRegions}

	return atomicJSON(filepath.Join(o.OutputDir, "meshmapper-status.json"), status, 0644)
}
func atomicJSON(path string, value any, mode os.FileMode) error {
	return atomicFile(path, mode, func(w io.Writer) error { return json.NewEncoder(w).Encode(value) })
}
func atomicFile(path string, mode os.FileMode, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		err = write(f)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func cachePath(o Options, code string) string {
	return filepath.Join(o.OutputDir, "regions", code, "meshmapper.json")
}
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func cacheHashes(o Options) (map[string]string, error) {
	result := make(map[string]string, len(o.Regions))
	for _, r := range o.Regions {
		hash, err := fileHash(cachePath(o, r.IATA))
		if err != nil {
			return nil, err
		}
		result[r.IATA] = hash
	}
	return result, nil
}
func cacheComplete(o Options, hashes map[string]string) bool {
	for _, r := range o.Regions {
		got, err := fileHash(cachePath(o, r.IATA))
		if err != nil || hashes[r.IATA] == "" || got != hashes[r.IATA] {
			return false
		}
	}
	return true
}

// The caller holds the pipeline lock. Reap only expired numeric MkdirTemp
// children from a killed import; never operator-named directories or symlinks.
func cleanupPending(o Options) error {
	entries, err := os.ReadDir(o.StateDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		suffix := strings.TrimPrefix(entry.Name(), "pending-")
		if !entry.IsDir() || suffix == entry.Name() || suffix == "" || strings.Trim(suffix, "0123456789") != "" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(o.Now.Add(-12 * time.Hour)) {
			continue
		}
		path := filepath.Join(o.StateDir, entry.Name())
		relative, err := filepath.Rel(o.StateDir, path)
		if err != nil || relative != entry.Name() {
			return fmt.Errorf("unsafe private import cleanup path")
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func publishCaches(o Options, meta feedMeta, regions map[string]regionData, dir string) error {
	// Finish and fsync every candidate privately before touching any live
	// regional file. A malformed or oversized feed therefore changes none.
	for _, r := range o.Regions {
		data, present := regions[r.IATA]
		if !present {
			data.IATA = r.IATA
			if got, e := fileHash(cachePath(o, r.IATA)); e == nil && o.previousHashes[r.IATA] != "" && got == o.previousHashes[r.IATA] {
				continue
			}
		}
		candidate := filepath.Join(dir, r.IATA+".json")
		if err := atomicFile(candidate, 0600, func(w io.Writer) error { return writeCache(w, o, meta, data, present) }); err != nil {
			return err
		}
	}
	for _, r := range o.Regions {
		candidate := filepath.Join(dir, r.IATA+".json")
		f, err := os.Open(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		err = atomicFile(cachePath(o, r.IATA), 0644, func(w io.Writer) error { _, e := io.Copy(w, f); return e })
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeCache(w io.Writer, o Options, meta feedMeta, data regionData, present bool) error {
	if _, err := io.WriteString(w, `{"cells":[`); err != nil {
		return err
	}
	count := 0
	if data.path != "" {
		f, err := os.Open(data.path)
		if err != nil {
			return err
		}
		defer f.Close()
		scan := bufio.NewScanner(f)
		scan.Buffer(make([]byte, 4096), 64*1024)
		for scan.Scan() {
			var c Cell
			if err := json.Unmarshal(scan.Bytes(), &c); err != nil {
				return err
			}
			lat, lon := (c.Bounds.South+c.Bounds.North)/2, (c.Bounds.West+c.Bounds.East)/2
			if !withinRegion(o.Regions, data.IATA, lat, lon) {
				data.Excluded++
				continue
			}
			if count > 0 {
				if _, err = io.WriteString(w, ","); err != nil {
					return err
				}
			}
			if _, err = w.Write(scan.Bytes()); err != nil {
				return err
			}
			count++
		}
		if err := scan.Err(); err != nil {
			return err
		}
	}
	data.Count = count
	tail := struct {
		regionData
		Source         string    `json:"source"`
		Available      bool      `json:"available"`
		Schema         int       `json:"schema_version"`
		GeneratedAt    *int64    `json:"generated_at"`
		FetchedAt      time.Time `json:"fetched_at"`
		RegionsSkipped int       `json:"source_regions_skipped"`
	}{data, "MeshMapper", present, meta.Schema, meta.GeneratedAt, o.Now, meta.RegionsSkipped}
	b, err := json.Marshal(tail)
	if err != nil {
		return err
	}
	if _, err = io.WriteString(w, "],"); err != nil {
		return err
	}
	_, err = w.Write(b[1:])
	return err
}
