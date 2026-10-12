package geo

import (
	"math"
	"testing"
)

func TestPreparedBoundaryMatchesHolesBordersAndDisjointPolygons(t *testing.T) {
	b, err := Parse([]byte(`{"type":"MultiPolygon","coordinates":[[[[-82,42],[-78,42],[-78,46],[-82,46],[-82,42]],[[-81,43],[-79,43],[-79,45],[-81,45],[-81,43]]],[[[-76,44],[-75,44],[-75,45],[-76,45],[-76,44]]]]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := b.Prepare()
	for lat := 41.0; lat <= 47; lat += 0.125 {
		for lon := -83.0; lon <= -74; lon += 0.125 {
			if p.Contains(lat, lon) != b.Contains(lat, lon) {
				t.Fatalf("mismatch at %f,%f", lat, lon)
			}
		}
	}
}

func benchmarkBoundary() Boundary {
	ring := make([]point, 36000)
	for i := range ring {
		a := float64(i) * 2 * math.Pi / float64(len(ring))
		ring[i] = point{lon: -90 + 20*math.Cos(a), lat: 60 + 15*math.Sin(a)}
	}
	return Boundary{polygons: [][][]point{{ring}}}
}
func BenchmarkPreparedBoundary(b *testing.B) {
	country := benchmarkBoundary()
	prepared := country.Prepare()
	b.Run("scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			country.Contains(43+float64(i%400)*.05, -80)
		}
	})
	b.Run("indexed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			prepared.Contains(43+float64(i%400)*.05, -80)
		}
	})
}
