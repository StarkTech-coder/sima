// Package coordination owns central mission state. It sees vehicles only through Gateway.
package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/StarkTech-coder/sima/internal/fleet"
	"github.com/StarkTech-coder/sima/internal/mission"
	"github.com/StarkTech-coder/sima/internal/model"
)

var IDs = []string{"AUV-01", "AUV-02", "AUV-03"}

type Gateway interface {
	Snapshot(context.Context, string) (model.Snapshot, error)
	Execute(context.Context, string, model.Command) (model.Receipt, error)
}
type Health interface{ Health(string) string }
type pending struct {
	command  model.Command
	attempts int
}
type Coordinator struct {
	mu          sync.Mutex
	gateway     Gateway
	health      Health
	fleet       fleet.Registry
	tasks       []model.TaskRecord
	events      []model.Event
	pending     map[string]*pending
	initialized bool
	ready       bool
	lastMessage map[string]string
}

func New(g Gateway, h Health) *Coordinator {
	c := &Coordinator{gateway: g, health: h, fleet: make(fleet.Registry), pending: make(map[string]*pending), lastMessage: make(map[string]string)}
	for _, id := range IDs {
		c.fleet[id] = model.Vehicle{Snapshot: model.Snapshot{ID: id}, Health: "unknown"}
	}
	for _, t := range mission.DemoTasks() {
		c.tasks = append(c.tasks, model.TaskRecord{Task: t, Status: "pending"})
	}
	return c
}

func (c *Coordinator) log(now time.Time, key, kind, message string) {
	if c.lastMessage[key] == message {
		return
	}
	c.lastMessage[key] = message
	c.events = append(c.events, model.Event{Time: now, Kind: kind, Message: message})
	if len(c.events) > model.MaxEvents {
		c.events = append([]model.Event(nil), c.events[len(c.events)-model.MaxEvents:]...)
	}
}

func (c *Coordinator) Run(ctx context.Context) {
	t := time.NewTicker(600 * time.Millisecond)
	defer t.Stop()
	for {
		c.Step(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Step is serialized. Polling has exactly three bounded workers; decisions are
// processed in stable vehicle/task order, never goroutine arrival order.
func (c *Coordinator) Step(ctx context.Context, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	type observation struct {
		s   model.Snapshot
		err error
	}
	observed := make([]observation, len(IDs))
	var wg sync.WaitGroup
	for i, id := range IDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			poll, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
			defer cancel()
			observed[i].s, observed[i].err = c.gateway.Snapshot(poll, id)
		}(i, id)
	}
	wg.Wait()
	for i, id := range IDs {
		v := c.fleet[id]
		h := c.health.Health(id)
		if v.Health != h {
			c.log(now, "health-"+id, "link", fmt.Sprintf("%s iletişim: %s. Araç arızası varsayılmadı; görev sahipliği korunuyor.", id, h))
		}
		v.Health = h
		c.fleet[id] = v
		if observed[i].err != nil {
			v.Synchronized = false
			c.fleet[id] = v
			continue
		}
		if !c.fleet.Observe(observed[i].s, now) {
			v.Synchronized = false
			c.fleet[id] = v
			c.log(now, "snapshot-"+id, "warning", id+" eski sıra/yeni epoch: otomatik komutlar durduruldu")
			continue
		}
		if !v.Synchronized && c.initialized {
			c.log(now, "reconcile-"+id, "reconcile", id+" gerçek araç snapshot'ı alındı; görev nesilleri ve sonuçlar uzlaştırılıyor")
		}
	}
	if ctx.Err() != nil {
		return
	}
	if !c.initialized {
		for _, v := range c.fleet.List(now) {
			if !v.Synchronized || v.Health != "healthy" {
				return
			}
		}
		for i := 0; i < 3; i++ {
			c.reserve(now, i, IDs[i], true, "Ön tanımlı ayrı keşif bölgesi; yetenek ve enerji doğrulandı")
		}
		c.reserve(now, 3, "AUV-02", false, "Takip kesiti kuyrukta; ayrı başlatma yetkisi gerekiyor")
		c.initialized = true
	}
	for i := range c.tasks {
		r := &c.tasks[i]
		if r.Owner == "" {
			continue
		}
		v := c.fleet[r.Owner]
		if !v.Synchronized || v.Health != "healthy" {
			continue
		}
		for _, a := range v.Snapshot.Assignments {
			if a.Task.ID != r.Task.ID || a.Generation != r.Generation {
				continue
			}
			// Explicitly unresolved commands retain their reservations until receipts arrive.
			if p := c.pending[r.Task.ID]; p != nil {
				if (p.command.Kind == "assign" && a.Status != "released") || (p.command.Kind == "start" && (a.Status == "running" || a.Status == "completed")) || (p.command.Kind == "result-ack" && a.ResultAcked) {
					delete(c.pending, r.Task.ID)
					c.log(now, "resolved-"+r.Task.ID, "receipt", r.Task.ID+" önceki komutun uygulandığı snapshot ile doğrulandı")
				}
			}
			if a.Status != "released" {
				r.Status = a.Status
				r.Progress = a.Progress
			}
			if c.pending[r.Task.ID] != nil {
				continue
			}
			if a.Status == "completed" && !a.ResultAcked {
				c.queue(i, "result-ack", false)
			}
			if a.Status == "queued" && v.Snapshot.Battery < r.Task.MinBattery {
				c.queue(i, "release", false)
				c.log(now, "release-"+r.Task.ID, "decision", fmt.Sprintf("%s / %s: enerji %.1f%% < %.0f%%; başlamamış göreve açık serbest bırakma komutu", r.Owner, r.Task.ID, v.Snapshot.Battery, r.Task.MinBattery))
			}
			// Original S-04 is intentionally held for the outage scenario, never auto-started.
			if a.Status == "queued" && r.Generation > 1 && mission.Eligible(v, r.Task) && !busy(v) {
				c.queue(i, "start", true)
			}
		}
	}
	// Bounded retries reuse the immutable command ID. Exhaustion retains ownership.
	for i := range c.tasks {
		r := &c.tasks[i]
		p := c.pending[r.Task.ID]
		if p == nil || p.attempts >= 12 {
			continue
		}
		v := c.fleet[r.Owner]
		if !v.Synchronized || v.Health != "healthy" {
			continue
		}
		p.attempts++
		request, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		receipt, err := c.gateway.Execute(request, r.Owner, p.command)
		cancel()
		if err != nil {
			c.log(now, "unknown-"+r.Task.ID, "warning", r.Task.ID+" komut sonucu belirsiz; sahiplik korunuyor, aynı komut kimliğiyle sınırlı tekrar")
			continue
		}
		if receipt.CommandID != p.command.ID || receipt.TaskID != r.Task.ID || receipt.Generation != r.Generation {
			continue
		}
		if !receipt.Accepted {
			c.log(now, "rejected-"+r.Task.ID, "warning", r.Task.ID+" komut reddi: "+receipt.Reason+"; sahiplik korunuyor")
			p.attempts = 12
			continue
		}
		switch p.command.Kind {
		case "assign":
			r.Status = "queued"
			if p.command.Start {
				r.Status = "running"
			}
		case "start":
			r.Status = "running"
		case "release":
			old := r.Owner
			if mission.Release(r, old, receipt) != nil {
				continue
			}
			c.log(now, "released-"+r.Task.ID, "receipt", old+" / "+r.Task.ID+" serbest bırakma makbuzu doğrulandı; görev artık yeniden atanabilir")
		case "result-ack":
			c.log(now, "result-"+r.Task.ID, "receipt", r.Task.ID+" tamamlanma sonucu alındı ve araç makbuzu doğrulandı")
		}
		delete(c.pending, r.Task.ID)
	}
	for i := range c.tasks {
		r := &c.tasks[i]
		if r.Owner != "" {
			continue
		}
		if owner, reason := mission.Select(c.fleet.List(now), r.Task, "AUV-02"); owner != "" {
			c.reserve(now, i, owner, false, reason)
		}
	}
	if !c.ready {
		c.ready = true
		for _, r := range c.tasks {
			if r.Status == "offered" || r.Owner == "" {
				c.ready = false
			}
		}
		if c.ready {
			c.log(now, "ready", "system", "Üç araç görev makbuzları doğrulandı. AUV-02 kesinti senaryosu hazır.")
		}
	}
}

func busy(v model.Vehicle) bool {
	for _, a := range v.Snapshot.Assignments {
		if a.Status == "running" {
			return true
		}
	}
	return false
}
func (c *Coordinator) reserve(now time.Time, i int, owner string, start bool, reason string) {
	r := &c.tasks[i]
	if !mission.Eligible(c.fleet[owner], r.Task) {
		return
	}
	if mission.Reserve(r, owner) != nil {
		return
	}
	c.queue(i, "assign", start)
	c.log(now, "assignment-"+r.Task.ID, "decision", fmt.Sprintf("%s → %s / nesil %d. %s", r.Task.ID, owner, r.Generation, reason))
}
func (c *Coordinator) queue(i int, kind string, start bool) {
	r := c.tasks[i]
	c.pending[r.Task.ID] = &pending{command: model.Command{ID: fmt.Sprintf("%s/%d/%s", r.Task.ID, r.Generation, kind), Kind: kind, Task: r.Task, Generation: r.Generation, Start: start}}
}
func (c *Coordinator) View(now time.Time) model.View {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := model.View{Vehicles: c.fleet.List(now), Tasks: c.tasks, Events: c.events, Ready: c.ready, Now: now}
	// Deep copy prevents HTTP encoders from retaining mutable routes/slices.
	data, _ := json.Marshal(v)
	var copy model.View
	_ = json.Unmarshal(data, &copy)
	return copy
}
