package adaptercore

// Package adaptercore defines transport-independent vehicle adapter interfaces.

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/StarkTech-coder/sima/internal/model"
)

type Adapter interface {
	Name() string
	VehicleIDs() []string
	Start(context.Context) error
	Close() error
	Snapshot(context.Context, string) (model.Snapshot, error)
	Execute(context.Context, string, model.Command) (model.Receipt, error)
	Health(string) string
}

type Hub struct {
	mu       sync.RWMutex
	routes   map[string]Adapter
	adapters []Adapter
}

func NewHub(adapters ...Adapter) (*Hub, error) {
	h := &Hub{routes: make(map[string]Adapter), adapters: append([]Adapter(nil), adapters...)}
	for _, a := range adapters {
		for _, id := range a.VehicleIDs() {
			if _, ok := h.routes[id]; ok {
				return nil, errors.New("duplicate vehicle route: " + id)
			}
			h.routes[id] = a
		}
	}
	return h, nil
}

func (h *Hub) Start(ctx context.Context) error {
	for _, a := range h.adapters {
		if err := a.Start(ctx); err != nil {
			_ = h.Close()
			return err
		}
	}
	return nil
}

func (h *Hub) Close() error {
	var err error
	for i := len(h.adapters) - 1; i >= 0; i-- {
		if e := h.adapters[i].Close(); e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

func (h *Hub) Snapshot(ctx context.Context, id string) (model.Snapshot, error) {
	a, err := h.route(id)
	if err != nil {
		return model.Snapshot{}, err
	}
	return a.Snapshot(ctx, id)
}

func (h *Hub) Execute(ctx context.Context, id string, c model.Command) (model.Receipt, error) {
	a, err := h.route(id)
	if err != nil {
		return model.Receipt{}, err
	}
	return a.Execute(ctx, id, c)
}

func (h *Hub) Health(id string) string {
	a, err := h.route(id)
	if err != nil {
		return "unknown"
	}
	return a.Health(id)
}

func (h *Hub) KnownVehicleIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.routes))
	for id := range h.routes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (h *Hub) route(id string) (Adapter, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	a, ok := h.routes[id]
	if !ok {
		return nil, errors.New("unknown vehicle: " + id)
	}
	return a, nil
}
