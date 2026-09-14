// SPDX-License-Identifier: GPL-3.0-or-later

package cephfunc

import (
	"context"
	"fmt"
)

const (
	MethodHealth          = "health"
	MethodOSDs            = "osds"
	MethodPools           = "pools"
	MethodDaemons         = "daemons"
	DefaultInventoryLimit = 500
	MaxInventoryLimit     = 5000
)

type Deps interface {
	Health(context.Context, int) (HealthResult, error)
	OSDs(context.Context, int) (OSDResult, error)
	Pools(context.Context, int) (PoolResult, error)
	Daemons(context.Context, int) (DaemonResult, error)
}

type SourceError struct {
	Status  int
	Message string
}

func (e *SourceError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Ceph Dashboard request failed with status %d", e.Status)
}

type InventoryLimitError struct {
	Resource string
	Total    int
	Limit    int
	Hard     bool
}

type IncompleteInventoryError struct {
	Resource string
	Rows     int
	Total    int
}

func (e *IncompleteInventoryError) Error() string {
	return fmt.Sprintf("Ceph %s inventory is incomplete: received %d of %d rows", e.Resource, e.Rows, e.Total)
}

func (e *InventoryLimitError) Error() string {
	if e.Hard {
		return fmt.Sprintf(
			"Ceph %s inventory contains %d rows, exceeding the internal ceiling of %d",
			e.Resource,
			e.Total,
			e.Limit,
		)
	}
	return fmt.Sprintf(
		"Ceph %s inventory contains %d rows, exceeding the selected limit of %d; choose a larger limit",
		e.Resource,
		e.Total,
		e.Limit,
	)
}

type HealthResult struct {
	Rows  []HealthRow
	Total int
}

type HealthRow struct {
	ID       string
	Code     string
	Severity string
	Muted    bool
	Summary  string
	Count    int64
	Detail   string
}

type OSDResult struct {
	Rows  []OSDRow
	Total int
}

type OSDRow struct {
	ID                int64
	Name              string
	UUID              string
	Host              string
	DeviceClass       string
	Up                bool
	In                bool
	OperationalStatus string
	TotalBytes        int64
	UsedBytes         int64
	AvailableBytes    int64
	Utilization       float64
	ReadBytesPerSec   float64
	WriteBytesPerSec  float64
	ReadOpsPerSec     float64
	WriteOpsPerSec    float64
	CommitLatencyMS   float64
	ApplyLatencyMS    float64
}

type PoolResult struct {
	Rows  []PoolRow
	Total int
}

type PoolRow struct {
	Name            string
	Type            string
	Size            *int64
	MinSize         *int64
	PGNum           *int64
	PGPNum          *int64
	PGAutoscaleMode string
	CrushRule       string
	CrushRoot       string
	FailureDomain   string
	DeviceClass     string
	Applications    string
	ErasureProfile  string
	QuotaMaxBytes   *int64
	QuotaMaxObjects *int64
	Flags           string
}

type DaemonResult struct {
	Rows  []DaemonRow
	Total int
}

type DaemonRow struct {
	ID          string
	Type        string
	Name        string
	Host        string
	Status      string
	Active      *bool
	Version     string
	Image       string
	LastRefresh string
	Placement   string
}
