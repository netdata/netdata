// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo
// +build !cgo

package as400

import (
	"context"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Stub implementation for when CGO is disabled

type AS400 struct {
	module.Base
}

func New() *AS400 {
	return &AS400{}
}

func (a *AS400) Configuration() any {
	return nil
}

func (a *AS400) Init(ctx context.Context) error {
	return errors.New("AS400 collector requires CGO support")
}

func (a *AS400) Check(ctx context.Context) error {
	return errors.New("AS400 collector requires CGO support")
}

func (a *AS400) Charts() *module.Charts {
	return nil
}

func (a *AS400) Collect(ctx context.Context) map[string]int64 {
	return nil
}

func (a *AS400) Cleanup(context.Context) {}