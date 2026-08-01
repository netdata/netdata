// SPDX-License-Identifier: GPL-3.0-or-later

package cephfunc

import (
	"context"
	"fmt"
	"time"
)

const (
	MethodHealth       = "health"
	MethodOSDs         = "osds"
	MethodPools        = "pools"
	MethodDaemons      = "daemons"
	MethodRGWMultisite = "rgw-multisite"
	MethodRGWQuotas    = "rgw-quotas"
)

type MethodConfig struct {
	Disabled bool
	Timeout  time.Duration
	Limit    int
}

type Config struct {
	Health       MethodConfig
	OSDs         MethodConfig
	Pools        MethodConfig
	Daemons      MethodConfig
	RGWMultisite MethodConfig
	RGWQuotas    MethodConfig
}

type Deps interface {
	Health(context.Context, int) (HealthResult, error)
	OSDs(context.Context, int) (OSDResult, error)
	Pools(context.Context, int) (PoolResult, error)
	Daemons(context.Context, int) (DaemonResult, error)
	RGWMultisite(context.Context, int) (RGWMultisiteResult, error)
	RGWQuotas(context.Context, int) (RGWQuotaResult, error)
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

type HealthResult struct {
	Rows  []HealthRow
	Total int
}

type HealthRow struct {
	ID              string
	Code            string
	Severity        string
	Muted           bool
	Summary         string
	Count           int64
	Detail          string
	DetailTruncated bool
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

type RGWMultisiteResult struct {
	Rows  []RGWMultisiteRow
	Total int
}

type RGWMultisiteRow struct {
	ID           string
	Kind         string
	Name         string
	Default      *bool
	Realm        string
	Zonegroup    string
	Master       *bool
	Endpoints    string
	SyncStatus   string
	SyncDetail   string
	ReleaseScope string
}

type RGWQuotaResult struct {
	Rows  []RGWQuotaRow
	Total int
}

type RGWQuotaRow struct {
	Key             string
	ID              string
	Kind            string
	Status          string
	Tenant          string
	Account         string
	Owner           string
	UsedBytes       *int64
	Objects         *int64
	QuotaEnabled    *bool
	QuotaMaxBytes   *int64
	QuotaMaxObjects *int64
	Utilization     *float64
	StatsFreshness  string
}
