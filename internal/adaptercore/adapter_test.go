package adaptercore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
)

type fakeAdapter struct {
	name    string
	ids     []string
	health  map[string]string
	snaps   map[string]model.Snapshot
	receipt model.Receipt
}

func (f *fakeAdapter) Name() string                { return f.name }
func (f *fakeAdapter) VehicleIDs() []string        { return append([]string(nil), f.ids...) }
func (f *fakeAdapter) Start(context.Context) error { return nil }
func (f *fakeAdapter) Close() error                { return nil }
func (f *fakeAdapter) Snapshot(_ context.Context, id string) (model.Snapshot, error) {
	s, ok := f.snaps[id]
	if !ok {
		return model.Snapshot{}, errors.New("not found")
	}
	return s, nil
}
func (f *fakeAdapter) Execute(_ context.Context, _ string, _ model.Command) (model.Receipt, error) {
	return f.receipt, nil
}
func (f *fakeAdapter) Health(id string) string { return f.health[id] }

func TestHubRoutesGatewayAndHealth(t *testing.T) {
	a := &fakeAdapter{
		name:    "a",
		ids:     []string{"AUV-01"},
		health:  map[string]string{"AUV-01": "healthy"},
		snaps:   map[string]model.Snapshot{"AUV-01": {ID: "AUV-01", Epoch: "e", Sequence: 1}},
		receipt: model.Receipt{CommandID: "c", TaskID: "S-01", Generation: 1, Accepted: true},
	}
	h, err := NewHub(a)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s, err := h.Snapshot(context.Background(), "AUV-01")
	if err != nil || s.ID != "AUV-01" {
		t.Fatal(err)
	}
	r, err := h.Execute(context.Background(), "AUV-01", model.Command{ID: "c", Task: model.Task{ID: "S-01"}, Generation: 1})
	if err != nil || !r.Accepted {
		t.Fatal(err)
	}
	if h.Health("AUV-01") != "healthy" {
		t.Fatal("health route failed")
	}
	if _, err = h.Snapshot(context.Background(), "AUV-X"); err == nil {
		t.Fatal("expected unknown vehicle error")
	}
}

func TestHubRejectsDuplicateRoutes(t *testing.T) {
	a := &fakeAdapter{name: "a", ids: []string{"AUV-01"}}
	b := &fakeAdapter{name: "b", ids: []string{"AUV-01"}}
	if _, err := NewHub(a, b); err == nil {
		t.Fatal("expected duplicate route error")
	}
}

func TestKnownVehicleIDsSorted(t *testing.T) {
	a := &fakeAdapter{name: "a", ids: []string{"AUV-02", "AUV-01"}}
	h, err := NewHub(a)
	if err != nil {
		t.Fatal(err)
	}
	ids := h.KnownVehicleIDs()
	if len(ids) != 2 || ids[0] != "AUV-01" || ids[1] != "AUV-02" {
		t.Fatalf("unexpected order %v", ids)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = h.Start(ctx); err != nil {
		t.Fatal(err)
	}
}
