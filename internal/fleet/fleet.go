package fleet

import (
	"sort"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
)

// Registry is owned by the coordinator; it never learns state from the simulator directly.
type Registry map[string]model.Vehicle

func (r Registry) Observe(s model.Snapshot, now time.Time) bool {
	v, ok := r[s.ID]
	if !ok || (v.Snapshot.Epoch != "" && v.Snapshot.Epoch != s.Epoch) || s.Sequence <= v.Snapshot.Sequence {
		return false
	}
	v.Snapshot = s
	v.LastSeen = now
	v.Synchronized = true
	r[s.ID] = v
	return true
}

func (r Registry) List(now time.Time) []model.Vehicle {
	out := make([]model.Vehicle, 0, len(r))
	for _, v := range r {
		if now.Sub(v.LastSeen) > model.Freshness {
			v.Synchronized = false
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Snapshot.ID < out[j].Snapshot.ID })
	return out
}
