package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
)

func TestThreeAUVActualUDPRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	wait := func(check func(model.View) bool) model.View {
		t.Helper()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			v := a.View()
			if check(v) {
				return v
			}
			select {
			case <-ctx.Done():
				t.Fatalf("scenario deadline; last view: %+v", v)
			case <-ticker.C:
			}
		}
	}
	initial := wait(func(v model.View) bool {
		return v.Ready && len(v.Vehicles[0].Snapshot.Assignments) > 0 && len(v.Vehicles[2].Snapshot.Assignments) > 0
	})
	if err = a.SetConnected(false); err != nil {
		t.Fatal(err)
	}
	cut := wait(func(v model.View) bool { return v.Vehicles[1].Health == "unreachable" })
	if cut.Tasks[1].Owner != "AUV-02" || cut.Tasks[3].Owner != "AUV-02" {
		t.Fatal("uncertain ownership changed")
	}
	// Test instrumentation advances onboard state, never feeds it to the coordinator.
	for _, v := range a.vehicles {
		v.Step(20)
	}
	advanced := wait(func(v model.View) bool {
		return v.Tasks[0].Progress > initial.Tasks[0].Progress+.1 && v.Tasks[2].Progress > initial.Tasks[2].Progress+.1
	})
	if advanced.Vehicles[1].Snapshot.Sequence != cut.Vehicles[1].Snapshot.Sequence {
		t.Fatal("telemetry bypassed partition")
	}
	if err = a.SetConnected(true); err != nil {
		t.Fatal(err)
	}
	recovered := wait(func(v model.View) bool {
		return v.Tasks[3].Generation == 2 && v.Tasks[3].Owner == "AUV-01" && v.Tasks[3].Status == "queued"
	})
	if recovered.Tasks[1].Progress <= cut.Tasks[1].Progress || recovered.Tasks[1].Owner != "AUV-02" {
		t.Fatal("local progress not reconciled")
	}
	old := a.vehicles["AUV-02"].Snapshot()
	if old.Assignments[1].Status != "released" {
		t.Fatal("new ownership before vehicle release")
	}
	for _, v := range a.vehicles {
		v.Step(200)
	}
	wait(func(v model.View) bool { return v.Tasks[3].Status == "running" })
	a.vehicles["AUV-01"].Step(60)
	wait(func(v model.View) bool {
		for _, r := range v.Tasks {
			if r.Status != "completed" {
				return false
			}
		}
		return true
	})
	wait(func(v model.View) bool {
		for _, vehicle := range v.Vehicles {
			for _, assignment := range vehicle.Snapshot.Assignments {
				if assignment.Status == "completed" && !assignment.ResultAcked {
					return false
				}
			}
		}
		return true
	})
	closed := make(chan error, 1)
	go func() { closed <- a.Close() }()
	select {
	case err = <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not join workers")
	}
}

func TestConcurrentViewAndSetConnectedNoDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for !a.View().Ready {
		select {
		case <-ctx.Done():
			t.Fatal("app not ready")
		case <-deadline.C:
			t.Fatal("app not ready before deadline")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	const viewers, togglers, iterations = 8, 4, 250
	start := make(chan struct{})
	done := make(chan struct{})
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	for i := 0; i < viewers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				_ = a.View()
			}
		}()
	}

	for i := 0; i < togglers; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				connected := (j+offset)%2 == 0
				if err := a.SetConnected(connected); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				_ = a.View()
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(done)
	}()
	close(start)

	select {
	case err := <-errCh:
		t.Fatalf("concurrent call failed: %v", err)
	case <-done:
	case <-ctx.Done():
		t.Fatal("possible deadlock between View and SetConnected")
	}
}
