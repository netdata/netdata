// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "time"

// GraphOptions controls conversion from Result to the internal graph projection.
type GraphOptions struct {
	SchemaVersion             string
	Source                    string
	Layer                     string
	View                      string
	AgentID                   string
	LocalDeviceID             string
	CollectedAt               time.Time
	ResolveDNSName            func(ip string) string
	CollapseActorsByIP        bool
	EliminateNonIPInferred    bool
	ProbabilisticConnectivity bool
	InferenceStrategy         string
}
