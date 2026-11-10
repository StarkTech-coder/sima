package adaptercontract

import (
	"testing"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
)

func TestValidateTelemetryAndAck(t *testing.T) {
	h := Header{VehicleID: "AUV-01", MessageID: "m-1", MessageVersion: MessageVersion, Timestamp: time.Now().UTC()}
	telemetry := TelemetryMessage{Header: h, Epoch: "epoch-1", Sequence: 1, Battery: 88, Connected: true}
	if err := ValidateTelemetry(telemetry); err != nil {
		t.Fatal(err)
	}
	ack := AckMessage{Header: h, CommandID: "c-1", TaskID: "S-01", Generation: 1, State: CommandAccepted, Accepted: true}
	if err := ValidateAck(ack); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCommandRejectsBrokenInput(t *testing.T) {
	h := Header{VehicleID: "!bad-id", MessageID: "", MessageVersion: 99, Timestamp: time.Now().UTC()}
	if err := ValidateHeader(h); err == nil {
		t.Fatal("expected invalid header")
	}
	validHeader := Header{VehicleID: "AUV-01", MessageID: "m-1", MessageVersion: MessageVersion, Timestamp: time.Now().UTC()}
	cmd := CommandMessage{Header: validHeader, CommandID: "", TaskID: "S-01", Generation: 0, Kind: "assign", Task: model.Task{ID: "S-01"}}
	if err := ValidateCommand(cmd); err == nil {
		t.Fatal("expected invalid command")
	}
}
