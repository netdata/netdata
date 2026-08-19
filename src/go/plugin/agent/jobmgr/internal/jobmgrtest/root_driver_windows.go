//go:build windows

package jobmgrtest

import (
	"context"
	"errors"
)

func (srd ShippedRootDriver) RunMatrixAvailable(ctx context.Context) ([]string, error) {
	if ctx == nil {
		return nil, errors.New("jobmgr test: nil shipped-root matrix context")
	}
	if err := srd.validateConfigs(); err != nil {
		return nil, err
	}
	roots := srd.roots()
	missing := make([]string, 0, len(roots))
	for _, root := range roots {
		missing = append(missing, root.name)
	}
	return missing, nil
}
