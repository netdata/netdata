// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo

package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var configSchema string

func init() {
	module.Register("db2", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return nil },
	})
}
