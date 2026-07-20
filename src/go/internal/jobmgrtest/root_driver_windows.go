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

func (ShippedRootDriver) RunMatrixAvailable(
	context.Context,
) ([]string, error) {
	return []string{
		"godplugin",
		"ibmdplugin",
		"scriptsdplugin",
	}, nil
}
