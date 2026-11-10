package ros2

// Package ros2 integrates S.I.M.A. with ROS 2 through a local bridge process.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/StarkTech-coder/sima/internal/adaptercontract"
	"github.com/StarkTech-coder/sima/internal/adaptercore"
	"github.com/StarkTech-coder/sima/internal/model"
)

var _ adaptercore.Adapter = (*Adapter)(nil)

type Config struct {
	BaseURL        string
	VehicleIDs     []string
	PollInterval   time.Duration
	CommandTimeout time.Duration
	RetryLimit     int
}

type Adapter struct {
	cfg     Config
	client  *http.Client
	cancel  context.CancelFunc
	closeCh chan struct{}

	mu        sync.RWMutex
	telemetry map[string]model.Snapshot
	lastSeq   map[string]uint64
	lastSeen  map[string]time.Time
	health    map[string]string
	lastAck   map[string]model.Receipt
}

func New(cfg Config) (*Adapter, error) {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 600 * time.Millisecond
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 2 * time.Second
	}
	if cfg.RetryLimit < 0 || cfg.RetryLimit > 10 {
		return nil, errors.New("invalid retry limit")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("bridge base URL required")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(cfg.VehicleIDs))
	seen := make(map[string]struct{}, len(cfg.VehicleIDs))
	for _, id := range cfg.VehicleIDs {
		if err := adaptercontract.ValidateVehicleID(id); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			return nil, errors.New("duplicate configured vehicle_id: " + id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	cfg.VehicleIDs = ids
	a := &Adapter{
		cfg:       cfg,
		client:    &http.Client{Timeout: 3 * time.Second},
		closeCh:   make(chan struct{}),
		telemetry: make(map[string]model.Snapshot, len(ids)),
		lastSeq:   make(map[string]uint64, len(ids)),
		lastSeen:  make(map[string]time.Time, len(ids)),
		health:    make(map[string]string, len(ids)),
		lastAck:   make(map[string]model.Receipt),
	}
	for _, id := range ids {
		a.health[id] = "unknown"
	}
	return a, nil
}

func (a *Adapter) Name() string { return "ros2-http-bridge" }

func (a *Adapter) VehicleIDs() []string { return append([]string(nil), a.cfg.VehicleIDs...) }

func (a *Adapter) Start(ctx context.Context) error {
	run, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	go func() {
		defer close(a.closeCh)
		t := time.NewTicker(a.cfg.PollInterval)
		defer t.Stop()
		a.pollAll(run)
		for {
			select {
			case <-run.Done():
				return
			case <-t.C:
				a.pollAll(run)
			}
		}
	}()
	return nil
}

func (a *Adapter) Close() error {
	if a.cancel != nil {
		a.cancel()
		<-a.closeCh
	}
	return nil
}

func (a *Adapter) Snapshot(_ context.Context, id string) (model.Snapshot, error) {
	a.mu.RLock()
	s, ok := a.telemetry[id]
	a.mu.RUnlock()
	if !ok {
		return model.Snapshot{}, errors.New("telemetry unavailable")
	}
	return s, nil
}

func (a *Adapter) Execute(ctx context.Context, id string, c model.Command) (model.Receipt, error) {
	if err := adaptercontract.ValidateVehicleID(id); err != nil {
		return model.Receipt{}, err
	}
	key := id + ":" + c.ID
	a.mu.RLock()
	if old, ok := a.lastAck[key]; ok {
		a.mu.RUnlock()
		return old, nil
	}
	a.mu.RUnlock()
	if c.ID == "" || c.Task.ID == "" || c.Generation == 0 {
		return model.Receipt{}, errors.New("invalid command identity")
	}
	msg := adaptercontract.CommandMessage{
		Header: adaptercontract.Header{
			VehicleID:      id,
			MessageID:      fmt.Sprintf("%s-%d", c.ID, time.Now().UnixNano()),
			MessageVersion: adaptercontract.MessageVersion,
			Timestamp:      time.Now().UTC(),
		},
		CommandID:  c.ID,
		TaskID:     c.Task.ID,
		Generation: c.Generation,
		Kind:       c.Kind,
		Start:      c.Start,
		Task:       c.Task,
		TimeoutMS:  int(a.cfg.CommandTimeout / time.Millisecond),
	}
	if err := adaptercontract.ValidateCommand(msg); err != nil {
		return model.Receipt{}, err
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return model.Receipt{}, err
	}
	tries := a.cfg.RetryLimit + 1
	var lastErr error
	for i := 0; i < tries; i++ {
		requestCtx, cancel := context.WithTimeout(ctx, a.cfg.CommandTimeout)
		ack, err := a.postCommand(requestCtx, payload)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		receipt := toReceipt(ack)
		a.mu.Lock()
		a.lastAck[key] = receipt
		a.mu.Unlock()
		if ack.State == adaptercontract.CommandRejected || ack.State == adaptercontract.CommandFailed {
			return receipt, nil
		}
		if ack.State == adaptercontract.CommandAccepted || ack.State == adaptercontract.CommandExecuting || ack.State == adaptercontract.CommandCompleted || ack.State == adaptercontract.CommandReceived {
			return receipt, nil
		}
		lastErr = errors.New("unexpected ack state")
	}
	if lastErr != nil {
		return model.Receipt{}, lastErr
	}
	return model.Receipt{}, errors.New("command not acknowledged")
}

func (a *Adapter) Health(id string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	h, ok := a.health[id]
	if !ok {
		return "unknown"
	}
	if seen := a.lastSeen[id]; !seen.IsZero() && time.Since(seen) > 3*time.Second && h == "healthy" {
		return "unreachable"
	}
	return h
}

func (a *Adapter) pollAll(ctx context.Context) {
	for _, id := range a.cfg.VehicleIDs {
		s, connected, err := a.fetchTelemetry(ctx, id)
		a.mu.Lock()
		if err != nil {
			a.health[id] = "unreachable"
			a.mu.Unlock()
			continue
		}
		if prev, ok := a.lastSeq[id]; !ok || s.Sequence > prev {
			a.lastSeen[id] = time.Now()
		}
		a.lastSeq[id] = s.Sequence
		a.telemetry[id] = s
		if connected {
			a.health[id] = "healthy"
		} else {
			a.health[id] = "suspect"
		}
		a.mu.Unlock()
	}
}

func (a *Adapter) fetchTelemetry(ctx context.Context, id string) (model.Snapshot, bool, error) {
	u := strings.TrimRight(a.cfg.BaseURL, "/") + "/v1/telemetry/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return model.Snapshot{}, false, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return model.Snapshot{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.Snapshot{}, false, errors.New("telemetry request failed")
	}
	var m adaptercontract.TelemetryMessage
	if err = json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return model.Snapshot{}, false, err
	}
	if err = adaptercontract.ValidateTelemetry(m); err != nil {
		return model.Snapshot{}, false, err
	}
	s := model.Snapshot{ID: m.VehicleID, Epoch: m.Epoch, Sequence: m.Sequence, Position: m.Position, Depth: m.Depth, Battery: m.Battery, Speed: m.Speed, Mode: m.Mode, Assignments: m.Assignments, Capabilities: []string{"survey"}}
	return s, m.Connected, nil
}

func (a *Adapter) postCommand(ctx context.Context, payload []byte) (adaptercontract.AckMessage, error) {
	u := strings.TrimRight(a.cfg.BaseURL, "/") + "/v1/command"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return adaptercontract.AckMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return adaptercontract.AckMessage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return adaptercontract.AckMessage{}, fmt.Errorf("command request failed: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var ack adaptercontract.AckMessage
	if err = json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return adaptercontract.AckMessage{}, err
	}
	if err = adaptercontract.ValidateAck(ack); err != nil {
		return adaptercontract.AckMessage{}, err
	}
	return ack, nil
}

func toReceipt(ack adaptercontract.AckMessage) model.Receipt {
	accepted := ack.Accepted || ack.State == adaptercontract.CommandAccepted || ack.State == adaptercontract.CommandExecuting || ack.State == adaptercontract.CommandCompleted || ack.State == adaptercontract.CommandReceived
	return model.Receipt{CommandID: ack.CommandID, TaskID: ack.TaskID, Generation: ack.Generation, Accepted: accepted, Reason: ack.Reason}
}
