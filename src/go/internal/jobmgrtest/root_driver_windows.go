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

func (ShippedRootDriver) RunAvailable(
	context.Context,
	string,
) ([]string, error) {
	return []string{
		"godplugin",
		"ibmdplugin",
		"scriptsdplugin",
	}, nil
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
