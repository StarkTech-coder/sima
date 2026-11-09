package coordination

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
	"github.com/StarkTech-coder/sima/internal/simulation"
)

type fakeGateway struct {
	vehicles    map[string]*simulation.Vehicle
	offline     bool
	lostRelease bool
}

func (g *fakeGateway) Health(id string) string {
	if id == "AUV-02" && g.offline {
		return "unreachable"
	}
	return "healthy"
}
func (g *fakeGateway) Snapshot(_ context.Context, id string) (model.Snapshot, error) {
	if g.Health(id) != "healthy" {
		return model.Snapshot{}, fmt.Errorf("offline")
	}
	return g.vehicles[id].Snapshot(), nil
}
func (g *fakeGateway) Execute(_ context.Context, id string, c model.Command) (model.Receipt, error) {
	if g.Health(id) != "healthy" {
		return model.Receipt{}, fmt.Errorf("offline")
	}
	r := g.vehicles[id].Apply(c)
	if c.Kind == "release" && g.lostRelease {
		g.lostRelease = false
		return model.Receipt{}, fmt.Errorf("lost receipt")
	}
	return r, nil
}

func TestReconciliationRequiresReleaseThenNewGeneration(t *testing.T) {
	g := &fakeGateway{vehicles: make(map[string]*simulation.Vehicle)}
	for _, id := range IDs {
		g.vehicles[id] = simulation.New(id, 90, model.Point{})
	}
	c := New(g, g)
	now := time.Unix(100, 0)
	ctx := context.Background()
	c.Step(ctx, now)
	if !c.View(now).Ready {
		t.Fatal("not ready")
	}
	g.offline = true
	g.vehicles["AUV-02"].Link(false)
	for _, v := range g.vehicles {
		v.Step(20)
	}
	c.Step(ctx, now.Add(time.Second))
	v := c.View(now.Add(time.Second))
	if v.Tasks[1].Owner != "AUV-02" || v.Tasks[3].Owner != "AUV-02" {
		t.Fatal("uncertain work duplicated")
	}
	if v.Vehicles[0].Snapshot.Assignments[0].Progress == 0 || v.Vehicles[2].Snapshot.Assignments[0].Progress == 0 {
		t.Fatal("other vehicles did not continue")
	}
	g.offline = false
	g.vehicles["AUV-02"].Link(true)
	g.lostRelease = true
	c.Step(ctx, now.Add(2*time.Second))
	if c.View(now).Tasks[3].Owner != "AUV-02" {
		t.Fatal("released without receipt")
	}
	c.Step(ctx, now.Add(3*time.Second))
	v = c.View(now.Add(3 * time.Second))
	r := v.Tasks[3]
	if r.Owner != "AUV-01" || r.Generation != 2 {
		t.Fatalf("bad reallocation %+v", r)
	}
	if v.Tasks[1].Owner != "AUV-02" || v.Tasks[1].Progress == 0 {
		t.Fatal("running assignment corrupted")
	}
	c.Step(ctx, now.Add(4*time.Second))
	for _, s := range g.vehicles {
		s.Step(200)
	}
	for i := 0; i < 5; i++ {
		c.Step(ctx, now.Add(time.Duration(5+i)*time.Second))
	}
	g.vehicles["AUV-01"].Step(60)
	c.Step(ctx, now.Add(11*time.Second))
	c.Step(ctx, now.Add(12*time.Second))
	for _, r := range c.View(now.Add(12 * time.Second)).Tasks {
		if r.Status != "completed" {
			t.Fatalf("task not complete: %+v", r)
		}
	}
}
