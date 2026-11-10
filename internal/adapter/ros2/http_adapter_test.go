package ros2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/adaptercontract"
	"github.com/StarkTech-coder/sima/internal/model"
)

func TestAdapterTelemetryAndExecute(t *testing.T) {
	var mu sync.Mutex
	acks := 0
	telemetry := adaptercontract.TelemetryMessage{
		Header: adaptercontract.Header{VehicleID: "AUV-01", MessageID: "tm-1", MessageVersion: adaptercontract.MessageVersion, Timestamp: time.Now().UTC()},
		Epoch:  "e-1", Sequence: 2, Battery: 85, Connected: true,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/telemetry/AUV-01":
			_ = json.NewEncoder(w).Encode(telemetry)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/command":
			var cmd adaptercontract.CommandMessage
			_ = json.NewDecoder(r.Body).Decode(&cmd)
			mu.Lock()
			acks++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(adaptercontract.AckMessage{
				Header:     adaptercontract.Header{VehicleID: cmd.VehicleID, MessageID: "ack-1", MessageVersion: adaptercontract.MessageVersion, Timestamp: time.Now().UTC()},
				CommandID:  cmd.CommandID,
				TaskID:     cmd.TaskID,
				Generation: cmd.Generation,
				State:      adaptercontract.CommandAccepted,
				Accepted:   true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	a, err := New(Config{BaseURL: server.URL, VehicleIDs: []string{"AUV-01"}, PollInterval: 30 * time.Millisecond, CommandTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err = a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	deadline := time.Now().Add(time.Second)
	for {
		s, snapErr := a.Snapshot(ctx, "AUV-01")
		if snapErr == nil && s.Epoch == "e-1" && a.Health("AUV-01") == "healthy" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("telemetry not ingested: %v", snapErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	r, err := a.Execute(ctx, "AUV-01", model.Command{ID: "cmd-1", Kind: "assign", Task: model.Task{ID: "S-01"}, Generation: 1})
	if err != nil || !r.Accepted {
		t.Fatalf("execute failed: %+v %v", r, err)
	}
	if _, err = a.Execute(ctx, "AUV-01", model.Command{ID: "cmd-1", Kind: "assign", Task: model.Task{ID: "S-01"}, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := acks
	mu.Unlock()
	if count != 1 {
		t.Fatalf("duplicate protection failed, acks=%d", count)
	}
}

func TestAdapterRejectsInvalidAck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/telemetry/AUV-01" {
			_ = json.NewEncoder(w).Encode(adaptercontract.TelemetryMessage{Header: adaptercontract.Header{VehicleID: "AUV-01", MessageID: "tm", MessageVersion: adaptercontract.MessageVersion, Timestamp: time.Now().UTC()}, Epoch: "e", Sequence: 1, Battery: 90, Connected: true})
			return
		}
		if r.URL.Path == "/v1/command" {
			_ = json.NewEncoder(w).Encode(adaptercontract.AckMessage{Header: adaptercontract.Header{VehicleID: "AUV-01", MessageID: "ack", MessageVersion: adaptercontract.MessageVersion, Timestamp: time.Now().UTC()}, CommandID: "c", TaskID: "S-01", Generation: 1, State: "broken"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	a, err := New(Config{BaseURL: server.URL, VehicleIDs: []string{"AUV-01"}, PollInterval: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err = a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	time.Sleep(40 * time.Millisecond)
	if _, err = a.Execute(ctx, "AUV-01", model.Command{ID: "c", Kind: "assign", Task: model.Task{ID: "S-01"}, Generation: 1}); err == nil {
		t.Fatal("expected invalid ack error")
	}
}
