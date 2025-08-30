// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo
// +build !cgo

package db2

import (
	"context"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Stub implementation for when CGO is disabled

type DB2 struct {
	module.Base
}

func New() *DB2 {
	return &DB2{}
}

func (d *DB2) Configuration() any {
	return nil
}

func (d *DB2) Init(ctx context.Context) error {
	return errors.New("DB2 collector requires CGO support")
}

func (d *DB2) Check(ctx context.Context) error {
	return errors.New("DB2 collector requires CGO support")
}

func (d *DB2) Charts() *module.Charts {
	return nil
}

func (d *DB2) Collect(ctx context.Context) map[string]int64 {
	return nil
}

func (d *DB2) Cleanup(context.Context) {}