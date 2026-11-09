package aegis

import (
	"context"
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/mission"
	"github.com/StarkTech-coder/sima/internal/model"
	"github.com/StarkTech-coder/sima/internal/simulation"
)

func TestActualUDPLostCommandReceiptAndHeartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	v := simulation.New("AUV-02", 86, model.Point{})
	e, err := StartEndpoint(ctx, "AUV-02", v)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	g, err := NewGateway(map[string]string{"AUV-02": e.Address()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.Snapshot(ctx, "AUV-02"); err != nil {
		t.Fatal(err)
	}
	m, err := g.StartMonitor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	c := model.Command{ID: "lost-receipt", Kind: "assign", Task: mission.DemoTasks()[1], Generation: 1, Start: true}
	e.DropNextReceipt()
	if _, err = g.Execute(ctx, "AUV-02", c); err == nil {
		t.Fatal("expected lost receipt")
	}
	v.Step(10)
	r, err := g.Execute(ctx, "AUV-02", c)
	if err != nil || !r.Accepted {
		t.Fatalf("retry: %+v %v", r, err)
	}
	s, err := g.Snapshot(ctx, "AUV-02")
	if err != nil || len(s.Assignments) != 1 || s.Assignments[0].Progress == 0 {
		t.Fatal("replay restarted task", err)
	}
	wait := func(want string) {
		t.Helper()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for m.Health("AUV-02") != want {
			select {
			case <-ctx.Done():
				t.Fatalf("heartbeat wanted %s got %s", want, m.Health("AUV-02"))
			case <-tick.C:
			}
		}
	}
	wait("healthy")
	e.SetConnected(false)
	v.Link(false)
	wait("unreachable")
	if _, err = g.Snapshot(ctx, "AUV-02"); err == nil {
		t.Fatal("partition leaked telemetry")
	}
	v.Step(5)
	e.SetConnected(true)
	v.Link(true)
	wait("healthy")
	s2, err := g.Snapshot(ctx, "AUV-02")
	if err != nil || s2.Assignments[0].Progress <= s.Assignments[0].Progress {
		t.Fatal("local continuation not visible", err)
	}
}

func TestRejectMalformedAndMismatchedEnvelope(t *testing.T) {
	e := &Endpoint{id: "AUV-01", vehicle: simulation.New("AUV-01", 90, model.Point{}), ready: make(chan struct{})}
	for _, data := range []string{`{`, `{"version":2,"id":"x","vehicle":"AUV-01","kind":"ping"}`, `{"version":1,"id":"x","vehicle":"other","kind":"ping"}`, `{"version":1,"id":"x","vehicle":"AUV-01","kind":"ping","unexpected":true}`} {
		if _, err := e.HandleUDPPacket([]byte(data), nil); err == nil {
			t.Fatal("accepted malformed envelope", data)
		}
	}
}
