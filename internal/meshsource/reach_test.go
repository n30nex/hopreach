package meshsource

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type reachFixture struct {
	Source
	active, peak atomic.Int32
}

func (f *reachFixture) FetchReach(context.Context, string, int) ([]ReachLink, error) {
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for old := f.peak.Load(); n > old; old = f.peak.Load() {
		if f.peak.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	return []ReachLink{}, nil
}
func TestReachWorkersRespectConfiguredRequestLimit(t *testing.T) {
	f := &reachFixture{}
	nodes := make([]Node, 40)
	results := FetchAllReachWithWorkers(context.Background(), f, nodes, 7, 1, nil)
	if f.peak.Load() != 1 || len(results) != 1 {
		t.Fatalf("peak=%d results=%d", f.peak.Load(), len(results))
	}
}
