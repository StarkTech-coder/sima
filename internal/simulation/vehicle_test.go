package simulation

import (
	"github.com/StarkTech-coder/sima/internal/mission"
	"github.com/StarkTech-coder/sima/internal/model"
	"testing"
)

func TestLostReceiptRetryDoesNotRestart(t *testing.T) {
	v := New("AUV-02", 86, model.Point{})
	task := mission.DemoTasks()[1]
	c := model.Command{ID: "assign-1", Kind: "assign", Task: task, Generation: 1, Start: true}
	first := v.Apply(c) // Applied, but imagine this receipt was lost.
	v.Step(10)
	before := v.Snapshot().Assignments[0].Progress
	if retry := v.Apply(c); retry != first || !retry.Accepted {
		t.Fatalf("bad retry: %+v", retry)
	}
	if after := v.Snapshot().Assignments[0].Progress; after != before {
		t.Fatal("retry restarted work")
	}
	c.Start = false
	if v.Apply(c).Accepted {
		t.Fatal("ID collision accepted")
	}
}

func TestDisconnectedSegmentAndReleaseFencing(t *testing.T) {
	v := New("AUV-02", 86, model.Point{})
	tasks := mission.DemoTasks()
	if !v.Apply(model.Command{ID: "primary", Kind: "assign", Task: tasks[1], Generation: 1, Start: true}).Accepted {
		t.Fatal("assign")
	}
	if !v.Apply(model.Command{ID: "queued", Kind: "assign", Task: tasks[3], Generation: 1}).Accepted {
		t.Fatal("queue")
	}
	v.Link(false)
	v.Step(200)
	s := v.Snapshot()
	if s.Assignments[0].Status != "completed" || s.Assignments[1].Status != "queued" {
		t.Fatalf("unsafe local policy: %+v", s)
	}
	v.Link(true)
	if v.Apply(model.Command{ID: "start", Kind: "start", Task: tasks[3], Generation: 1}).Accepted {
		t.Fatal("low-energy start")
	}
	if !v.Apply(model.Command{ID: "release", Kind: "release", Task: tasks[3], Generation: 1}).Accepted {
		t.Fatal("release")
	}
	if v.Apply(model.Command{ID: "old-replay", Kind: "assign", Task: tasks[3], Generation: 1}).Accepted {
		t.Fatal("old generation resurrected")
	}
}
