// Package aegis is the only S.I.M.A. package importing the external AEGIS SDK.
// It supplies application envelopes, not a second transport implementation.
package aegis

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/StarkTech-coder/aegis/pkg/aegis"
	"github.com/StarkTech-coder/sima/internal/model"
)

const MaxMessage = 8192
const ExchangeTimeout = 250 * time.Millisecond

type Vehicle interface {
	Snapshot() model.Snapshot
	Apply(model.Command) model.Receipt
}

type envelope struct {
	Version  int             `json:"version"`
	ID       string          `json:"id"`
	Vehicle  string          `json:"vehicle"`
	Kind     string          `json:"kind"`
	Epoch    string          `json:"epoch,omitempty"`
	Command  *model.Command  `json:"command,omitempty"`
	Receipt  *model.Receipt  `json:"receipt,omitempty"`
	Snapshot *model.Snapshot `json:"snapshot,omitempty"`
}

func decode(data []byte, out any) error {
	if len(data) > MaxMessage {
		return errors.New("message too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func config(address string) *sdk.Config {
	c := sdk.DefaultConfig()
	c.Protocol = sdk.ProtocolUDP
	c.Address = address
	c.MaxPayloadSize = MaxMessage
	c.MaxConnections = 8
	c.ReadTimeout = time.Second
	c.WriteTimeout = time.Second
	return c
}

type Endpoint struct {
	id          string
	vehicle     Vehicle
	address     string
	server      *sdk.Server
	done        chan struct{}
	err         error // published by closing done
	ready       chan struct{}
	readyOnce   sync.Once
	blocked     atomic.Bool
	dropReceipt atomic.Bool
}

// StartEndpoint uses an OS-selected loopback port. AEGIS owns the real listener.
// The temporary socket only selects a candidate port; readiness is verified by an
// application round trip, never an arbitrary startup sleep.
func StartEndpoint(ctx context.Context, id string, vehicle Vehicle) (*Endpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reservation, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	address := reservation.LocalAddr().String()
	if err = reservation.Close(); err != nil {
		return nil, err
	}
	s, err := sdk.NewServer(config(address))
	if err != nil {
		return nil, err
	}
	e := &Endpoint{id: id, vehicle: vehicle, address: address, server: s, done: make(chan struct{}), ready: make(chan struct{})}
	s.SetHandler(e)
	go func() { e.err = s.Start(); close(e.done) }()
	probe, err := NewGateway(map[string]string{id: address})
	if err != nil {
		return nil, err
	}
	startup, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for {
		select {
		case <-e.done:
			return nil, fmt.Errorf("AEGIS bind: %w", e.err)
		default:
		}
		_, err = probe.exchange(startup, id, "ping", nil, "")
		if err == nil {
			return e, nil
		}
		if startup.Err() != nil {
			_ = e.Close()
			return nil, fmt.Errorf("AEGIS readiness: %w", startup.Err())
		}
	}
}

func (e *Endpoint) Address() string             { return e.address }
func (e *Endpoint) SetConnected(connected bool) { e.blocked.Store(!connected) }

// DropNextReceipt is a test-only fault: apply a command, then lose its response.
func (e *Endpoint) DropNextReceipt() { e.dropReceipt.Store(true) }
func (e *Endpoint) Close() error {
	if err := e.server.Stop(); err != nil {
		return err
	}
	<-e.done
	return e.err
}
func (e *Endpoint) HandlePacket([]byte) error { return errors.New("UDP response handler required") }
func (e *Endpoint) HandleUDPPacket(payload []byte, _ *net.UDPAddr) ([]byte, error) {
	e.readyOnce.Do(func() { close(e.ready) })
	if e.blocked.Load() {
		return nil, nil
	}
	var request envelope
	if err := decode(payload, &request); err != nil {
		return nil, err
	}
	if request.Version != 1 || request.Vehicle != e.id || request.ID == "" || len(request.ID) > 128 {
		return nil, errors.New("invalid envelope")
	}
	r := envelope{Version: 1, ID: request.ID, Vehicle: e.id, Kind: request.Kind}
	switch request.Kind {
	case "ping":
	case "snapshot":
		s := e.vehicle.Snapshot()
		r.Snapshot = &s
	case "command":
		if request.Command == nil {
			return nil, errors.New("missing command")
		}
		if request.Epoch != e.vehicle.Snapshot().Epoch {
			return nil, errors.New("vehicle epoch changed; reconciliation required")
		}
		receipt := e.vehicle.Apply(*request.Command)
		r.Receipt = &receipt
		if e.dropReceipt.Swap(false) {
			return nil, nil
		}
	default:
		return nil, errors.New("unknown message kind")
	}
	return json.Marshal(r)
}

type Gateway struct {
	client    *sdk.Client
	addresses map[string]string
	epochs    sync.Map
}

func NewGateway(addresses map[string]string) (*Gateway, error) {
	c, err := sdk.NewClient(config("127.0.0.1:1"))
	if err != nil {
		return nil, err
	}
	copyAddresses := make(map[string]string, len(addresses))
	for k, v := range addresses {
		copyAddresses[k] = v
	}
	return &Gateway{client: c, addresses: copyAddresses}, nil
}

func (g *Gateway) exchange(ctx context.Context, id, kind string, command *model.Command, epoch string) (envelope, error) {
	address, ok := g.addresses[id]
	if !ok {
		return envelope{}, errors.New("unknown vehicle")
	}
	ctx, cancel := context.WithTimeout(ctx, ExchangeTimeout)
	defer cancel()
	conn, err := g.client.ConnectContext(ctx, address)
	if err != nil {
		return envelope{}, err
	}
	defer conn.Close()
	request := envelope{Version: 1, ID: rand.Text(), Vehicle: id, Kind: kind, Command: command, Epoch: epoch}
	data, err := json.Marshal(request)
	if err != nil {
		return envelope{}, err
	}
	if len(data) > MaxMessage {
		return envelope{}, errors.New("message too large")
	}
	if err = g.client.SendPacket(ctx, conn, data); err != nil {
		return envelope{}, err
	}
	data, err = g.client.ReceivePacket(ctx, conn)
	if err != nil {
		return envelope{}, err
	}
	var response envelope
	if err = decode(data, &response); err != nil {
		return envelope{}, err
	}
	if response.Version != 1 || response.ID != request.ID || response.Vehicle != id || response.Kind != kind {
		return envelope{}, errors.New("response correlation mismatch")
	}
	return response, nil
}
func (g *Gateway) Snapshot(ctx context.Context, id string) (model.Snapshot, error) {
	r, err := g.exchange(ctx, id, "snapshot", nil, "")
	if err != nil {
		return model.Snapshot{}, err
	}
	if r.Snapshot == nil || r.Snapshot.ID != id || r.Snapshot.Epoch == "" || len(r.Snapshot.Assignments) > model.MaxTasks {
		return model.Snapshot{}, errors.New("invalid snapshot")
	}
	g.epochs.LoadOrStore(id, r.Snapshot.Epoch) // never silently trust a new execution epoch
	return *r.Snapshot, nil
}
func (g *Gateway) Execute(ctx context.Context, id string, c model.Command) (model.Receipt, error) {
	epoch, ok := g.epochs.Load(id)
	if !ok {
		return model.Receipt{}, errors.New("snapshot required before commands")
	}
	r, err := g.exchange(ctx, id, "command", &c, epoch.(string))
	if err != nil {
		return model.Receipt{}, err
	}
	if r.Receipt == nil || r.Receipt.CommandID != c.ID || r.Receipt.TaskID != c.Task.ID || r.Receipt.Generation != c.Generation {
		return model.Receipt{}, errors.New("invalid command receipt")
	}
	return *r.Receipt, nil
}

type Monitor struct{ monitor *sdk.HeartbeatMonitor }

func (g *Gateway) StartMonitor(ctx context.Context) (*Monitor, error) {
	cfg := sdk.DefaultHeartbeatConfig("SIMA-CENTRAL")
	cfg.Interval = 500 * time.Millisecond
	cfg.Timeout = ExchangeTimeout
	cfg.UnhealthyThreshold = 2
	probe := sdk.HeartbeatProbeFunc(func(ctx context.Context, target sdk.NodeEndpoint, msg sdk.HeartbeatMessage) (sdk.HeartbeatResponse, error) {
		_, err := g.exchange(ctx, target.NodeID, "ping", nil, "")
		return sdk.HeartbeatResponse{SourceNodeID: target.NodeID, TargetNodeID: msg.SourceNodeID, ReceivedAt: time.Now()}, err
	})
	h, err := sdk.NewHeartbeatMonitor(cfg, probe, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return nil, err
	}
	for id, address := range g.addresses {
		if err = h.AddNode(sdk.NodeEndpoint{NodeID: id, Address: address}); err != nil {
			return nil, err
		}
	}
	if err = h.Start(ctx); err != nil {
		return nil, err
	}
	return &Monitor{h}, nil
}
func (m *Monitor) Health(id string) string {
	h, ok := m.monitor.GetNodeHealth(id)
	if !ok {
		return "unknown"
	}
	if h.State == sdk.NodeHealthUnhealthy {
		return "unreachable"
	}
	return string(h.State)
}
func (m *Monitor) Close() error { return m.monitor.Stop() }
