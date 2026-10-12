package meshmapper

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestUnknownIATACandidatesKeepSeparateCapsAndDiscardUnusedFiles(t *testing.T) {
	regions := []Region{{IATA: "YKF", Lat: 43.46, Lon: -80.38, RadiusKm: 200}, {IATA: "YVR", Lat: 49.19, Lon: -123.18, RadiusKm: 200}}
	yvr := strings.NewReplacer("43.4600", "49.1900", "43.4627", "49.1927", "-80.3800", "-123.1800", "-80.37616", "-123.17616").Replace(fixtureCell)
	cells := yvr + "," + fixtureCell + "," + fixtureCell
	dir := t.TempDir()
	got, err := readCells(json.NewDecoder(strings.NewReader("["+cells+"]")), "", dir, regions, func(float64, float64) bool { return true }, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["YKF"].Count != 1 || !got["YKF"].Truncated || got["YVR"].Count != 1 || got["YVR"].Truncated {
		t.Fatalf("incorrect candidate caps: %+v", got)
	}
	for _, r := range got {
		data, err := os.ReadFile(r.path)
		if err != nil || strings.Count(string(data), "\n") != 1 {
			t.Fatalf("candidate exceeds its one-cell disk cap: %v", err)
		}
	}
	for _, iata := range []string{"YKF", "SEA"} {
		t.Run(iata, func(t *testing.T) {
			dir := t.TempDir()
			input := `{"grid_squares":[` + cells + `],"region":"` + iata + `"}`
			result, err := parseRegion(json.NewDecoder(strings.NewReader(input)), dir, regions, func(float64, float64) bool { return true }, 1)
			if err != nil {
				t.Fatal(err)
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if iata == "YKF" {
				want = 1
				if result.Count != 1 {
					t.Fatal("selected region's candidate was lost")
				}
			}
			if len(files) != want {
				t.Fatalf("unused candidate files were retained: got %d want %d", len(files), want)
			}
		})
	}
}
