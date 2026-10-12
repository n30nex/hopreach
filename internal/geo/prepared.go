package geo

import "math"

// PreparedBoundary indexes edges by latitude for large measured-cell imports.
// Each bucket retains polygon/ring order, so queries preserve outer-ring,
// hole and multi-polygon union semantics without allocating per point.
type PreparedBoundary struct{ buckets [1024][]preparedEdge }
type preparedEdge struct {
	a, b          point
	polygon, ring int
	outer         bool
}

func latitudeBucket(lat float64) int { return max(0, min(1023, int(math.Floor((lat+90)*1024/180)))) }

func (b Boundary) Prepare() *PreparedBoundary {
	p := &PreparedBoundary{}
	for pi, poly := range b.polygons {
		for ri, ring := range poly {
			for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
				a, c := ring[j], ring[i]
				if a.lat == c.lat {
					continue
				}
				e := preparedEdge{a: a, b: c, polygon: pi, ring: ri, outer: ri == 0}
				for bin := latitudeBucket(math.Min(a.lat, c.lat)); bin <= latitudeBucket(math.Max(a.lat, c.lat)); bin++ {
					p.buckets[bin] = append(p.buckets[bin], e)
				}
			}
		}
	}
	return p
}

func (p *PreparedBoundary) Contains(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	edges := p.buckets[latitudeBucket(lat)]
	poly, ring := -1, -1
	inside, outer, hole, ringOuter := false, false, false, false
	finishRing := func() {
		if ringOuter {
			outer = inside
		} else {
			hole = hole || inside
		}
	}
	for _, e := range edges {
		if e.polygon != poly || e.ring != ring {
			finishRing()
			if e.polygon != poly {
				if outer && !hole {
					return true
				}
				outer, hole = false, false
			}
			poly, ring, ringOuter, inside = e.polygon, e.ring, e.outer, false
		}
		if (e.a.lat > lat) != (e.b.lat > lat) && lon < (e.b.lon-e.a.lon)*(lat-e.a.lat)/(e.b.lat-e.a.lat)+e.a.lon {
			inside = !inside
		}
	}
	finishRing()
	return outer && !hole
}
