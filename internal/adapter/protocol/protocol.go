// Package protocol defines a versioned transport-neutral vehicle message contract.
package protocol

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/StarkTech-coder/sima/internal/model"
)

const MessageVersion = 1

var vehicleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,62}[A-Za-z0-9]$`)

type CommandState string

const (
	CommandSent      CommandState = "sent"
	CommandReceived  CommandState = "received"
	CommandAccepted  CommandState = "accepted"
	CommandRejected  CommandState = "rejected"
	CommandExecuting CommandState = "executing"
	CommandCompleted CommandState = "completed"
	CommandFailed    CommandState = "failed"
)

type Header struct {
	VehicleID      string    `json:"vehicle_id"`
	MessageID      string    `json:"message_id"`
	MessageVersion int       `json:"message_version"`
	Timestamp      time.Time `json:"timestamp"`
}

type TelemetryMessage struct {
	Header
	Epoch       string             `json:"epoch"`
	Sequence    uint64             `json:"sequence"`
	Position    model.Point        `json:"position"`
	Depth       float64            `json:"depth"`
	Battery     float64            `json:"battery"`
	Speed       float64            `json:"speed"`
	Mode        string             `json:"mode"`
	Assignments []model.Assignment `json:"assignments"`
	Connected   bool               `json:"connected"`
}

type CommandMessage struct {
	Header
	CommandID  string         `json:"command_id"`
	TaskID     string         `json:"task_id"`
	Generation uint64         `json:"generation"`
	Kind       string         `json:"kind"`
	Start      bool           `json:"start"`
	Task       model.Task     `json:"task"`
	TimeoutMS  int            `json:"timeout_ms"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type AckMessage struct {
	Header
	CommandID  string       `json:"command_id"`
	TaskID     string       `json:"task_id"`
	Generation uint64       `json:"generation"`
	State      CommandState `json:"state"`
	Accepted   bool         `json:"accepted"`
	Reason     string       `json:"reason,omitempty"`
}

func ValidateVehicleID(id string) error {
	if !vehicleIDPattern.MatchString(id) {
		return fmt.Errorf("invalid vehicle_id %q", id)
	}
	return nil
}

func ValidateHeader(h Header) error {
	if err := ValidateVehicleID(h.VehicleID); err != nil {
		return err
	}
	if h.MessageVersion != MessageVersion {
		return fmt.Errorf("unsupported message_version %d", h.MessageVersion)
	}
	if h.MessageID == "" || len(h.MessageID) > 160 {
		return errors.New("invalid message_id")
	}
	if h.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

func ValidateTelemetry(t TelemetryMessage) error {
	if err := ValidateHeader(t.Header); err != nil {
		return err
	}
	if t.Epoch == "" || len(t.Epoch) > 160 {
		return errors.New("invalid epoch")
	}
	if t.Battery < 0 || t.Battery > 100 {
		return errors.New("battery out of range")
	}
	if len(t.Assignments) > model.MaxTasks {
		return errors.New("too many assignments")
	}
	return nil
}

func ValidateCommand(c CommandMessage) error {
	if err := ValidateHeader(c.Header); err != nil {
		return err
	}
	if c.CommandID == "" || len(c.CommandID) > 160 {
		return errors.New("invalid command_id")
	}
	if c.TaskID == "" || len(c.TaskID) > 160 {
		return errors.New("invalid task_id")
	}
	if c.Generation == 0 {
		return errors.New("generation required")
	}
	if c.Kind == "" {
		return errors.New("kind required")
	}
	if c.TimeoutMS < 0 || c.TimeoutMS > 120000 {
		return errors.New("timeout_ms out of range")
	}
	return nil
}

func ValidateAck(a AckMessage) error {
	if err := ValidateHeader(a.Header); err != nil {
		return err
	}
	if a.CommandID == "" || a.TaskID == "" || a.Generation == 0 {
		return errors.New("invalid command reference")
	}
	switch a.State {
	case CommandSent, CommandReceived, CommandAccepted, CommandRejected, CommandExecuting, CommandCompleted, CommandFailed:
	default:
		return fmt.Errorf("invalid state %q", a.State)
	}
	return nil
}
