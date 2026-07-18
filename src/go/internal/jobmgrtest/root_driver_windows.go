//go:build windows

package jobmgrtest

import (
	"context"
)

type ShippedRoot struct {
	Executable string
	Module     string
	ConfigDir  string
}

type ShippedRootDriver struct {
	Roots map[string]ShippedRoot
}

func (ShippedRootDriver) Available(string) bool {
	return false
}

func (ShippedRootDriver) Run(context.Context, string) error {
	return context.Canceled
}
