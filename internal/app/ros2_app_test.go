package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/adaptercontract"
)

func TestROS2AppViewAndLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/telemetry/AUV-01" {
			_ = json.NewEncoder(w).Encode(adaptercontract.TelemetryMessage{
				Header: adaptercontract.Header{VehicleID: "AUV-01", MessageID: "tm", MessageVersion: adaptercontract.MessageVersion, Timestamp: time.Now().UTC()},
				Epoch:  "e", Sequence: 1, Battery: 95, Connected: true,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	a, err := NewROS2(ctx, ROS2Config{BridgeURL: server.URL, VehicleIDs: []string{"AUV-01"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		view := a.View()
		if len(view.Vehicles) > 0 && view.Vehicles[0].Snapshot.Epoch != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ros2 app did not ingest telemetry")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err = a.SetConnected(false); err == nil {
		t.Fatal("expected ROS2 mode link toggle rejection")
	}
}
