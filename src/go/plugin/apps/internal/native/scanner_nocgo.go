//go:build !cgo

// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

type Scanner struct{}

var errUnavailable = errors.New("apps native scanner requires a build with CGO_ENABLED=1")

func New(Options) (*Scanner, error) { return nil, errUnavailable }
func (*Scanner) Scan(context.Context) (model.Snapshot, error) {
	return model.Snapshot{}, errUnavailable
}
func (*Scanner) Finalize(uint64, []model.Assignment) ([]model.GroupFD, error) {
	return nil, errUnavailable
}
func (*Scanner) Close() {}
