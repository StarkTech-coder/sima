package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/StarkTech-coder/sima/internal/adapter/ros2"
	"github.com/StarkTech-coder/sima/internal/adaptercore"
	"github.com/StarkTech-coder/sima/internal/coordination"
	"github.com/StarkTech-coder/sima/internal/model"
)

type ROS2Config struct {
	BridgeURL  string
	VehicleIDs []string
}

type ROS2App struct {
	mu          sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	hub         *adaptercore.Hub
	coordinator *coordination.Coordinator
	closed      bool
}

func NewROS2(parent context.Context, cfg ROS2Config) (*ROS2App, error) {
	ctx, cancel := context.WithCancel(parent)
	if len(cfg.VehicleIDs) == 0 {
		cfg.VehicleIDs = append([]string(nil), coordination.IDs...)
	}
	r, err := ros2.New(ros2.Config{BaseURL: cfg.BridgeURL, VehicleIDs: cfg.VehicleIDs})
	if err != nil {
		cancel()
		return nil, err
	}
	h, err := adaptercore.NewHub(r)
	if err != nil {
		cancel()
		return nil, err
	}
	if err = h.Start(ctx); err != nil {
		cancel()
		return nil, err
	}
	a := &ROS2App{cancel: cancel, hub: h, coordinator: coordination.New(h, h)}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.coordinator.Run(ctx)
	}()
	return a, nil
}

func (a *ROS2App) View() model.View {
	return a.coordinator.View(time.Now())
}

func (a *ROS2App) SetConnected(bool) error {
	return errors.New("manual disconnect toggle is only available in local simulator mode")
}

func (a *ROS2App) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()
	a.cancel()
	a.wg.Wait()
	return a.hub.Close()
}
