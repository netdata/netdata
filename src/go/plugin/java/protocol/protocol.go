// SPDX-License-Identifier: GPL-3.0-or-later

// Package protocol defines the fixed java.plugin privileged-helper interface.
package protocol

import "fmt"

type Process struct {
	PID         int    `json:"pid"`
	StartTime   uint64 `json:"start_time"`
	BootID      string `json:"boot_id"`
	UID         uint32 `json:"uid"`
	GID         uint32 `json:"gid"`
	Application string `json:"application"`
	Location    string `json:"location"`
	Eligible    bool   `json:"eligible"`
	Reason      string `json:"reason,omitempty"`
}

func (p Process) Instance() string { return fmt.Sprintf("%s-%d:%d", p.BootID, p.PID, p.StartTime) }

type Discovery struct {
	Processes []Process `json:"processes"`
}

type AttachRequest struct {
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time"`
	BootID    string `json:"boot_id"`
	Token     string `json:"token"`
	Port      uint16 `json:"port"`
}

type AttachResult struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

const (
	Attached = "Attached"
	Blocked  = "Blocked"
	Unknown  = "Unknown"
)
