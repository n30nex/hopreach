package meshmapper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheRechecksLateIATAWithoutCrossRegionCopies(t *testing.T) {
	regions := []Region{{IATA: "YKF", Lat: 43.46, Lon: -80.38, RadiusKm: 10}, {IATA: "YYZ", Lat: 43.68, Lon: -79.63, RadiusKm: 10}}
	yyz := strings.ReplaceAll(strings.ReplaceAll(fixtureCell, "43.4600", "43.6800"), "43.4627", "43.6827")
	yyz = strings.ReplaceAll(strings.ReplaceAll(yyz, "-80.3800", "-79.6300"), "-80.37616", "-79.62616")
	input := `{"regions":[{"grid_squares":[` + fixtureCell + `,` + yyz + `],"region":"YKF"}],"success":true,"global":true,"schema_version":2,"generated_at":1791832854}`
	meta, got, err := parseFeed(strings.NewReader(input), t.TempDir(), regions, fixtureContains, 10)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = writeCache(&out, Options{Regions: regions}, meta, got["YKF"], true); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Count int    `json:"cell_count"`
		Cells []Cell `json:"cells"`
	}
	if err = json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Count != 1 || len(decoded.Cells) != 1 || decoded.Cells[0].Bounds.West != -80.38 {
		t.Fatalf("cross-region cache: %s", out.String())
	}
}

func TestLateIATAAppliesCellCapOnlyToSelectedRegion(t *testing.T) {
	regions := []Region{
		{IATA: "YKF", Lat: 43.46, Lon: -80.38, RadiusKm: 200},
		{IATA: "YVR", Lat: 49.19, Lon: -123.18, RadiusKm: 200},
	}
	yvr := strings.NewReplacer("43.4600", "49.1900", "43.4627", "49.1927", "-80.3800", "-123.1800", "-80.37616", "-123.17616").Replace(fixtureCell)
	for _, tc := range []struct {
		name, iata, cells string
		truncated         bool
	}{
		{"YKF after YVR", "YKF", yvr + "," + fixtureCell, false},
		{"YVR after YKF", "YVR", fixtureCell + "," + yvr, false},
		{"selected region really exceeds cap", "YKF", yvr + "," + fixtureCell + "," + fixtureCell, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var early []byte
			for _, late := range []bool{false, true} {
				section := fmt.Sprintf(`{"region":%q,"grid_squares":[%s]}`, tc.iata, tc.cells)
				if late {
					section = fmt.Sprintf(`{"grid_squares":[%s],"region":%q}`, tc.cells, tc.iata)
				}
				input := `{"regions":[` + section + `],"success":true,"global":true,"schema_version":2,"generated_at":1791832854}`
				meta, got, err := parseFeed(strings.NewReader(input), t.TempDir(), regions, func(float64, float64) bool { return true }, 1)
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				if err = writeCache(&out, Options{Regions: regions}, meta, got[tc.iata], true); err != nil {
					t.Fatal(err)
				}
				var cache struct {
					Count     int    `json:"cell_count"`
					Truncated bool   `json:"truncated"`
					Cells     []Cell `json:"cells"`
				}
				if err = json.Unmarshal(out.Bytes(), &cache); err != nil {
					t.Fatal(err)
				}
				if cache.Count != 1 || len(cache.Cells) != 1 || cache.Truncated != tc.truncated {
					t.Fatalf("late=%v: selected-region cap lost or misclassified cells: %s", late, out.String())
				}
				if !late {
					early = append([]byte(nil), out.Bytes()...)
				} else if !bytes.Equal(early, out.Bytes()) {
					t.Fatal("JSON field order changed the selected region's cached data or counts")
				}
			}
		})
	}
}

func TestPendingCleanupPreservesOperatorDirectoriesAndCurrentImports(t *testing.T) {
	o := Options{StateDir: t.TempDir(), Now: time.Now()}
	for _, name := range []string{"pending-12345", "pending-notes", "pending-98765"} {
		path := filepath.Join(o.StateDir, name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if name != "pending-98765" {
			old := o.Now.Add(-13 * time.Hour)
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := cleanupPending(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(o.StateDir, "pending-12345")); !os.IsNotExist(err) {
		t.Fatal("expired generated child retained")
	}
	for _, name := range []string{"pending-notes", "pending-98765"} {
		if _, err := os.Stat(filepath.Join(o.StateDir, name)); err != nil {
			t.Fatal("operator or current data removed")
		}
	}
}
