// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo

package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var configSchema string

func init() {
	collectorapi.Register("db2", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return nil },
	})
}
