package fleet

import (
	"github.com/StarkTech-coder/sima/internal/model"
	"testing"
	"time"
)

func TestObservationOrderingAndFreshness(t *testing.T) {
	r := Registry{"AUV-01": {}}
	now := time.Unix(100, 0)
	s := model.Snapshot{ID: "AUV-01", Epoch: "one", Sequence: 2}
	if !r.Observe(s, now) {
		t.Fatal("fresh rejected")
	}
	if r.Observe(s, now) {
		t.Fatal("duplicate accepted")
	}
	s.Sequence = 1
	if r.Observe(s, now) {
		t.Fatal("stale accepted")
	}
	s.Sequence = 3
	s.Epoch = "two"
	if r.Observe(s, now) {
		t.Fatal("unknown epoch accepted")
	}
	if r.List(now.Add(model.Freshness + time.Second))[0].Synchronized {
		t.Fatal("stale telemetry eligible")
	}
}
