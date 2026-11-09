// Package app composes the demo. Only this simulation environment can operate the fault gate.
package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/StarkTech-coder/sima/internal/adapter/aegis"
	"github.com/StarkTech-coder/sima/internal/coordination"
	"github.com/StarkTech-coder/sima/internal/model"
	"github.com/StarkTech-coder/sima/internal/simulation"
)

type App struct {
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	closeOnce    sync.Once
	closeErr     error
	linkMu       sync.Mutex
	disconnected bool
	closed       bool
	vehicles     map[string]*simulation.Vehicle
	endpoints    map[string]*aegis.Endpoint
	monitor      *aegis.Monitor
	coordinator  *coordination.Coordinator
}

func New(parent context.Context) (*App, error) {
	ctx, cancel := context.WithCancel(parent)
	a := &App{cancel: cancel, vehicles: make(map[string]*simulation.Vehicle), endpoints: make(map[string]*aegis.Endpoint)}
	addresses := make(map[string]string)
	for i, id := range coordination.IDs {
		battery := 96.0
		if id == "AUV-02" {
			battery = 86
		}
		v := simulation.New(id, battery, model.Point{X: 12 + float64(i)*30, Y: 18})
		a.vehicles[id] = v
		a.wg.Add(1)
		go func() { defer a.wg.Done(); v.Run(ctx) }()
		e, err := aegis.StartEndpoint(ctx, id, v)
		if err != nil {
			_ = a.Close()
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		a.endpoints[id] = e
		addresses[id] = e.Address()
	}
	g, err := aegis.NewGateway(addresses)
	if err != nil {
		_ = a.Close()
		return nil, err
	}
	a.monitor, err = g.StartMonitor(ctx)
	if err != nil {
		_ = a.Close()
		return nil, err
	}
	a.coordinator = coordination.New(g, a.monitor)
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.coordinator.Run(ctx) }()
	return a, nil
}

func (a *App) View() model.View {
	a.linkMu.Lock()
	disconnected := a.disconnected
	a.linkMu.Unlock()
	v := a.coordinator.View(time.Now())
	v.Disconnected = disconnected
	return v
}

func (a *App) SetConnected(connected bool) error {
	if !a.coordinator.View(time.Now()).Ready {
		return errors.New("initial assignment receipts not ready")
	}
	a.linkMu.Lock()
	defer a.linkMu.Unlock()
	if a.closed {
		return errors.New("simulation closed")
	}
	if a.disconnected == !connected {
		return nil
	}
	// Close the communication gate before changing the environment; restore it last.
	if !connected {
		a.endpoints["AUV-02"].SetConnected(false)
	}
	a.vehicles["AUV-02"].Link(connected)
	if connected {
		a.endpoints["AUV-02"].SetConnected(true)
	}
	a.disconnected = !connected
	return nil
}

func (a *App) Close() error {
	a.closeOnce.Do(func() {
		a.linkMu.Lock()
		a.closed = true
		a.linkMu.Unlock()
		a.cancel()
		a.wg.Wait()
		if a.monitor != nil {
			a.closeErr = errors.Join(a.closeErr, a.monitor.Close())
		}
		for _, id := range coordination.IDs {
			if e := a.endpoints[id]; e != nil {
				a.closeErr = errors.Join(a.closeErr, e.Close())
			}
		}
	})
	return a.closeErr
}
