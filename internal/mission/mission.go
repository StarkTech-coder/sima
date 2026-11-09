// Package mission implements deterministic ownership and allocation rules, without networking.
package mission

import (
	"fmt"
	"slices"
	"sort"

	"github.com/StarkTech-coder/sima/internal/model"
)

func Eligible(v model.Vehicle, t model.Task) bool {
	return v.Synchronized && v.Health == "healthy" && v.Snapshot.Battery >= t.MinBattery && slices.Contains(v.Snapshot.Capabilities, t.Capability)
}

func Select(vehicles []model.Vehicle, t model.Task, excluded string) (string, string) {
	candidates := append([]model.Vehicle(nil), vehicles...)
	load := func(v model.Vehicle) int {
		n := 0
		for _, a := range v.Snapshot.Assignments {
			if a.Status == "running" || a.Status == "queued" {
				n++
			}
		}
		return n
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := load(candidates[i]), load(candidates[j])
		if a != b {
			return a < b
		}
		return candidates[i].Snapshot.ID < candidates[j].Snapshot.ID
	})
	for _, v := range candidates {
		if v.Snapshot.ID != excluded && Eligible(v, t) {
			return v.Snapshot.ID, "Yetenek ve enerji uygun; en az yük, eşitlikte araç kimliği"
		}
	}
	return "", "Güncel ve uygun araç yok; görev bekletiliyor"
}

// Reserve happens BEFORE sending: a lost receipt never makes work free for another vehicle.
func Reserve(r *model.TaskRecord, owner string) error {
	if r.Owner != "" || owner == "" {
		return fmt.Errorf("task already reserved or owner empty")
	}
	r.Owner = owner
	r.Generation++
	r.Status = "offered"
	return nil
}

// Release only accepts an explicit application receipt for this exact owner and generation.
func Release(r *model.TaskRecord, owner string, receipt model.Receipt) error {
	if r.Owner != owner || !receipt.Accepted || receipt.TaskID != r.Task.ID || receipt.Generation != r.Generation || receipt.Reason != "released" {
		return fmt.Errorf("release not confirmed")
	}
	r.Owner = ""
	r.Status = "pending"
	r.Progress = 0
	return nil
}

func DemoTasks() []model.Task {
	return []model.Task{
		{ID: "S-01", Name: "Batı sırtı taraması", Capability: "survey", MinBattery: 25, Duration: 110, Route: []model.Point{{X: 12, Y: 22}, {X: 39, Y: 22}, {X: 39, Y: 37}, {X: 12, Y: 37}, {X: 12, Y: 52}, {X: 39, Y: 52}}},
		{ID: "S-02", Name: "Merkez havza taraması", Capability: "survey", MinBattery: 25, Duration: 125, Route: []model.Point{{X: 43, Y: 25}, {X: 65, Y: 25}, {X: 65, Y: 40}, {X: 43, Y: 40}, {X: 43, Y: 56}, {X: 65, Y: 56}}},
		{ID: "S-03", Name: "Doğu yamaç taraması", Capability: "survey", MinBattery: 25, Duration: 140, Route: []model.Point{{X: 70, Y: 28}, {X: 89, Y: 28}, {X: 89, Y: 46}, {X: 70, Y: 46}, {X: 70, Y: 63}, {X: 89, Y: 63}}},
		{ID: "S-04", Name: "Derin kesit doğrulaması", Capability: "survey", MinBattery: 80, Duration: 55, Route: []model.Point{{X: 30, Y: 72}, {X: 58, Y: 72}, {X: 58, Y: 85}, {X: 30, Y: 85}}},
	}
}
