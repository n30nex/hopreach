package meshmapper

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

const fixtureCell = `{"grid_id":"16100_-21000","bounds":{"south":43.4600,"west":-80.3800,"north":43.4627,"east":-80.37616},"coverage_type":"BIDIR","fill_color":"#1e7e34","border_color":"#14522d","snr":null,"timestamp":1791800000000,"count":2,"first_seen":1791700000000}`

func fixtureRegions() []Region {
	return []Region{{IATA: "YKF", Lat: 43.46, Lon: -80.38, RadiusKm: 200}, {IATA: "YYZ", Lat: 43.68, Lon: -79.63, RadiusKm: 200}}
}
func fixtureContains(lat, lon float64) bool { return lat >= 43 && lat < 50 && lon >= -81 && lon < -79 }
func feed(cells string) string {
	return `{"regions":[{"region":"SEA","grid_squares":[` + fixtureCell + `]},{"region":"YKF","grid_squares":[` + cells + `],"total_squares":2}],"success":true,"global":true,"schema_version":2,"generated_at":1791832854,"region_count":2,"regions_skipped":4}`
}

func TestStreamFiltersForeignAndInvalidPreservesGeometryAndNull(t *testing.T) {
	invalid := strings.Replace(fixtureCell, `"north":43.4627`, `"north":43.0`, 1)
	dir := t.TempDir()
	meta, regions, err := parseFeed(strings.NewReader(feed(fixtureCell+","+invalid)), dir, fixtureRegions(), fixtureContains, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || regions["YKF"].Count != 1 || regions["YKF"].Invalid != 1 || meta.RegionsSkipped != 4 {
		t.Fatalf("meta=%+v regions=%+v", meta, regions)
	}
	b, err := os.ReadFile(regions["YKF"].path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"snr":null`) || !strings.Contains(string(b), `"timestamp":1791800000`) || !strings.Contains(string(b), `"north":43.4627`) {
		t.Fatal("geometry, null SNR or epoch-millisecond normalization lost")
	}
}

func TestStreamLateRegionAndBoundedTruncation(t *testing.T) {
	input := `{"regions":[{"grid_squares":[` + fixtureCell + `,` + fixtureCell + `],"region":"YKF"}],"generated_at":1791832854,"schema_version":2,"global":true,"success":true}`
	_, regions, err := parseFeed(strings.NewReader(input), t.TempDir(), fixtureRegions(), fixtureContains, 1)
	if err != nil || regions["YKF"].Count != 1 || !regions["YKF"].Truncated {
		t.Fatalf("regions=%+v err=%v", regions, err)
	}
}

type measuredReader struct {
	io.Reader
	maxRead int
}

func (r *measuredReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	return r.Reader.Read(p)
}
func TestLargeGzipForeignRegionDoesNotBufferWholeArray(t *testing.T) {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		z := gzip.NewWriter(writer)
		defer z.Close()
		fmt.Fprint(z, `{"regions":[{"region":"SEA","grid_squares":[`)
		for i := 0; i < 60000; i++ {
			if i > 0 {
				fmt.Fprint(z, ",")
			}
			fmt.Fprint(z, fixtureCell)
		}
		fmt.Fprint(z, `]}],"success":true,"global":true,"schema_version":2,"generated_at":1791832854}`)
	}()
	z, err := gzip.NewReader(reader)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	r := &measuredReader{Reader: z}
	dir := t.TempDir()
	_, regions, err := parseFeed(r, dir, fixtureRegions(), fixtureContains, 3)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(regions) != 0 || len(files) != 0 || r.maxRead > 64*1024 {
		t.Fatalf("foreign retention=%d files=%d decoder buffer request=%d", len(regions), len(files), r.maxRead)
	}
}

func TestStreamRejectsRegionalMergedAndMalformedEnvelopes(t *testing.T) {
	for _, input := range []string{`{"success":true,"region":"YKF","grid_squares":[]}`, `{"success":true,"region":"YKF,YYZ","regions":["YKF","YYZ"]}`, feed(fixtureCell)[:100], strings.Replace(feed(fixtureCell), `"schema_version":2`, `"schema_version":3`, 1)} {
		if _, _, err := parseFeed(strings.NewReader(input), t.TempDir(), fixtureRegions(), fixtureContains, 3); err == nil {
			t.Fatal("unsupported/malformed feed accepted")
		}
	}
}

func TestStreamBudgetRejectsOversizeButAcceptsExactEOF(t *testing.T) {
	input := feed(fixtureCell)
	for _, delta := range []int64{-1, 0} {
		r := &byteBudget{r: strings.NewReader(input), remaining: int64(len(input)) + delta, label: "fixture"}
		_, _, err := parseFeed(r, t.TempDir(), fixtureRegions(), fixtureContains, 3)
		if (delta < 0) != (err != nil) {
			t.Fatalf("budget delta=%d err=%v", delta, err)
		}
	}
}
