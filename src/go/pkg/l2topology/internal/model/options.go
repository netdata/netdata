// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

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
	LookupVendorByMAC         func(mac string) (vendor string, prefix string)
	CollapseActorsByIP        bool
	EliminateNonIPInferred    bool
	ProbabilisticConnectivity bool
	InferenceStrategy         string
	WorkLimiter               worklimit.Limiter
}
