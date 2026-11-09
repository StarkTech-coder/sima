// Package model contains transport-independent simulation and mission contracts.
package model

import "time"

const (
	MaxTasks    = 16
	MaxCommands = 128
	MaxEvents   = 160
	Freshness   = 3 * time.Second
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Task struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Capability string  `json:"capability"`
	MinBattery float64 `json:"minBattery"`
	Duration   float64 `json:"duration"`
	Route      []Point `json:"route"`
}
type Assignment struct {
	Task        Task    `json:"task"`
	Generation  uint64  `json:"generation"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	ResultAcked bool    `json:"resultAcked"`
}
type Snapshot struct {
	ID           string       `json:"id"`
	Epoch        string       `json:"epoch"`
	Sequence     uint64       `json:"sequence"`
	Capabilities []string     `json:"capabilities"`
	Position     Point        `json:"position"`
	Depth        float64      `json:"depth"`
	Battery      float64      `json:"battery"`
	Speed        float64      `json:"speed"`
	Mode         string       `json:"mode"`
	Assignments  []Assignment `json:"assignments"`
}
type Command struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Task       Task   `json:"task"`
	Generation uint64 `json:"generation"`
	Start      bool   `json:"start"`
}
type Receipt struct {
	CommandID  string `json:"commandID"`
	TaskID     string `json:"taskID"`
	Generation uint64 `json:"generation"`
	Accepted   bool   `json:"accepted"`
	Reason     string `json:"reason"`
}
type Vehicle struct {
	Snapshot     Snapshot  `json:"snapshot"`
	Health       string    `json:"health"`
	LastSeen     time.Time `json:"lastSeen"`
	Synchronized bool      `json:"synchronized"`
}
type TaskRecord struct {
	Task       Task    `json:"task"`
	Owner      string  `json:"owner"`
	Generation uint64  `json:"generation"`
	Status     string  `json:"status"`
	Progress   float64 `json:"progress"`
}
type Event struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
}
type View struct {
	Vehicles     []Vehicle    `json:"vehicles"`
	Tasks        []TaskRecord `json:"tasks"`
	Events       []Event      `json:"events"`
	Disconnected bool         `json:"disconnected"`
	Ready        bool         `json:"ready"`
	Now          time.Time    `json:"now"`
}
