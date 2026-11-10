package aegis

import (
	"context"
	"errors"

	"github.com/StarkTech-coder/sima/internal/adaptercore"
	"github.com/StarkTech-coder/sima/internal/model"
)

var _ adaptercore.Adapter = (*VehicleAdapter)(nil)

type VehicleAdapter struct {
	addresses map[string]string
	gateway   *Gateway
	monitor   *Monitor
}

func NewVehicleAdapter(addresses map[string]string) *VehicleAdapter {
	copyAddresses := make(map[string]string, len(addresses))
	for k, v := range addresses {
		copyAddresses[k] = v
	}
	return &VehicleAdapter{addresses: copyAddresses}
}

func (a *VehicleAdapter) Name() string { return "aegis-udp" }

func (a *VehicleAdapter) VehicleIDs() []string {
	ids := make([]string, 0, len(a.addresses))
	for id := range a.addresses {
		ids = append(ids, id)
	}
	return ids
}

func (a *VehicleAdapter) Start(ctx context.Context) error {
	g, err := NewGateway(a.addresses)
	if err != nil {
		return err
	}
	m, err := g.StartMonitor(ctx)
	if err != nil {
		return err
	}
	a.gateway = g
	a.monitor = m
	return nil
}

func (a *VehicleAdapter) Close() error {
	if a.monitor != nil {
		return a.monitor.Close()
	}
	return nil
}

func (a *VehicleAdapter) Snapshot(ctx context.Context, id string) (model.Snapshot, error) {
	if a.gateway == nil {
		return model.Snapshot{}, errors.New("adapter not started")
	}
	return a.gateway.Snapshot(ctx, id)
}

func (a *VehicleAdapter) Execute(ctx context.Context, id string, c model.Command) (model.Receipt, error) {
	if a.gateway == nil {
		return model.Receipt{}, errors.New("adapter not started")
	}
	return a.gateway.Execute(ctx, id, c)
}

func (a *VehicleAdapter) Health(id string) string {
	if a.monitor == nil {
		return "unknown"
	}
	return a.monitor.Health(id)
}
