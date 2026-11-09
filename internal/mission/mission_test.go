package mission

import (
	"github.com/StarkTech-coder/sima/internal/model"
	"testing"
)

func TestOwnershipRequiresReleaseReceipt(t *testing.T) {
	r := model.TaskRecord{Task: DemoTasks()[3]}
	if err := Reserve(&r, "AUV-02"); err != nil {
		t.Fatal(err)
	}
	if Reserve(&r, "AUV-01") == nil {
		t.Fatal("duplicated uncertain work")
	}
	if Release(&r, "AUV-02", model.Receipt{}) == nil {
		t.Fatal("unconfirmed release")
	}
	if err := Release(&r, "AUV-02", model.Receipt{Accepted: true, TaskID: r.Task.ID, Generation: 1, Reason: "released"}); err != nil {
		t.Fatal(err)
	}
	if err := Reserve(&r, "AUV-01"); err != nil || r.Generation != 2 {
		t.Fatal("generation not advanced")
	}
}

func TestAllocationDeterministicAndFresh(t *testing.T) {
	task := DemoTasks()[3]
	a := model.Vehicle{Snapshot: model.Snapshot{ID: "AUV-01", Battery: 90, Capabilities: []string{"survey"}}, Health: "healthy", Synchronized: true}
	b := a
	b.Snapshot.ID = "AUV-03"
	for _, vs := range [][]model.Vehicle{{b, a}, {a, b}} {
		if id, _ := Select(vs, task, ""); id != "AUV-01" {
			t.Fatal(id)
		}
	}
	a.Synchronized = false
	b.Snapshot.Battery = 50
	if id, _ := Select([]model.Vehicle{a, b}, task, ""); id != "" {
		t.Fatal("ineligible allocated")
	}
}
