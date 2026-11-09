// Package simulation models onboard execution only, not real navigation or safety control.
package simulation

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
)

type cached struct {
	command model.Command
	receipt model.Receipt
}

// Vehicle has a separate ticker and state lock. No central connection is required to Step.
type Vehicle struct {
	mu             sync.Mutex
	state          model.Snapshot
	tasks          map[string]model.Assignment
	commands       map[string]cached
	connected      bool
	reserveCharged bool
}

func New(id string, battery float64, position model.Point) *Vehicle {
	return &Vehicle{state: model.Snapshot{ID: id, Epoch: rand.Text(), Battery: battery, Position: position, Depth: 65, Mode: "ready", Capabilities: []string{"survey"}}, tasks: make(map[string]model.Assignment), commands: make(map[string]cached), connected: true}
}

func (v *Vehicle) Run(ctx context.Context) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			v.Step(now.Sub(last).Seconds())
			last = now
		}
	}
}

// Link is a simulation environment input, not a mission command. The first outage
// charges a documented 12-point local contingency reserve to make reallocation demonstrable.
func (v *Vehicle) Link(connected bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.connected = connected
	if !connected && !v.reserveCharged {
		v.state.Battery = math.Max(0, v.state.Battery-12)
		v.reserveCharged = true
	}
}

func (v *Vehicle) Step(seconds float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return
	}
	v.state.Speed = 0
	for id, a := range v.tasks {
		if a.Status != "running" {
			continue
		}
		if v.state.Battery <= 20 {
			a.Status = "held"
			v.state.Mode = "safe-hold"
			v.tasks[id] = a
			return
		}
		// Only the explicitly started segment runs; queued tasks never auto-start.
		delta := math.Min(seconds/a.Task.Duration, 1-a.Progress)
		delta = math.Min(delta, (v.state.Battery-20)/(a.Task.Duration*0.035))
		a.Progress += delta
		v.state.Battery = math.Max(0, v.state.Battery-delta*a.Task.Duration*0.035)
		v.state.Speed = 1.6
		v.state.Depth = 65 + 12*math.Sin(a.Progress*math.Pi)
		segments := len(a.Task.Route) - 1
		p := a.Progress * float64(segments)
		i := min(int(p), segments-1)
		f := p - float64(i)
		from, to := a.Task.Route[i], a.Task.Route[i+1]
		v.state.Position = model.Point{X: from.X + (to.X-from.X)*f, Y: from.Y + (to.Y-from.Y)*f}
		v.state.Mode = "surveying"
		if !v.connected {
			v.state.Mode = "local-policy"
		}
		if a.Progress >= 1 {
			a.Status = "completed"
			v.state.Speed = 0
			v.state.Mode = "segment-complete"
		}
		v.tasks[id] = a
		return
	}
}

func (v *Vehicle) Snapshot() model.Snapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.state.Sequence++
	s := v.state
	s.Capabilities = append([]string(nil), s.Capabilities...)
	s.Assignments = make([]model.Assignment, 0, len(v.tasks))
	for _, a := range v.tasks {
		a.Task.Route = append([]model.Point(nil), a.Task.Route...)
		s.Assignments = append(s.Assignments, a)
	}
	sort.Slice(s.Assignments, func(i, j int) bool { return s.Assignments[i].Task.ID < s.Assignments[j].Task.ID })
	return s
}

func (v *Vehicle) Apply(c model.Command) model.Receipt {
	v.mu.Lock()
	defer v.mu.Unlock()
	r := model.Receipt{CommandID: c.ID, TaskID: c.Task.ID, Generation: c.Generation}
	if old, ok := v.commands[c.ID]; ok {
		if reflect.DeepEqual(old.command, c) {
			return old.receipt
		}
		r.Reason = "command ID reused with different content"
		return r
	}
	if c.ID == "" || len(c.ID) > 160 || len(v.commands) >= model.MaxCommands {
		r.Reason = "invalid ID or command history full"
		return r
	}
	r.Reason = v.apply(c)
	r.Accepted = r.Reason == "accepted" || r.Reason == "released" || r.Reason == "result-recorded"
	c.Task.Route = append([]model.Point(nil), c.Task.Route...)
	v.commands[c.ID] = cached{c, r}
	return r
}

func (v *Vehicle) apply(c model.Command) string {
	a, exists := v.tasks[c.Task.ID]
	busy := func() bool {
		for _, x := range v.tasks {
			if x.Status == "running" {
				return true
			}
		}
		return false
	}
	switch c.Kind {
	case "assign":
		if c.Generation == 0 || c.Task.ID == "" || len(c.Task.Route) < 2 || len(c.Task.Route) > 16 || c.Task.Duration <= 0 || math.IsNaN(c.Task.Duration) || math.IsInf(c.Task.Duration, 0) {
			return "invalid task"
		}
		if exists && (c.Generation <= a.Generation || (a.Status != "released" && a.Status != "completed")) {
			return "stale generation or task still owned"
		}
		if !exists && len(v.tasks) >= model.MaxTasks {
			return "task capacity reached"
		}
		if !slices.Contains(v.state.Capabilities, c.Task.Capability) || v.state.Battery < c.Task.MinBattery {
			return "capability or energy insufficient"
		}
		if !v.connected || (c.Start && busy()) {
			return "not available to start"
		}
		status := "queued"
		if c.Start {
			status = "running"
		}
		c.Task.Route = append([]model.Point(nil), c.Task.Route...)
		v.tasks[c.Task.ID] = model.Assignment{Task: c.Task, Generation: c.Generation, Status: status}
		return "accepted"
	case "release", "start", "result-ack":
		if !exists || a.Generation != c.Generation {
			return "assignment generation mismatch"
		}
		switch c.Kind {
		case "release":
			if a.Status != "queued" && a.Status != "released" {
				return "only unstarted work can be released"
			}
			a.Status = "released"
		case "start":
			if a.Status != "queued" || !v.connected || busy() || v.state.Battery < a.Task.MinBattery {
				return "not safe to start"
			}
			a.Status = "running"
		case "result-ack":
			if a.Status != "completed" {
				return "no completed result"
			}
			a.ResultAcked = true
		}
		v.tasks[c.Task.ID] = a
		if c.Kind == "release" {
			return "released"
		}
		if c.Kind == "result-ack" {
			return "result-recorded"
		}
		return "accepted"
	default:
		return fmt.Sprintf("unknown command %q", c.Kind)
	}
}
