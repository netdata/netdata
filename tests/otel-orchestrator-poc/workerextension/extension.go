// SPDX-License-Identifier: GPL-3.0-or-later
package workerextension

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
)

type statusExtension struct{}

var _ extensioncapabilities.PipelineWatcher = (*statusExtension)(nil)

func NewFactory() extension.Factory {
	return extension.NewFactory(component.MustNewType("netdata_worker"), func() component.Config { return &struct{}{} },
		func(context.Context, extension.Settings, component.Config) (extension.Extension, error) {
			return &statusExtension{}, nil
		}, component.StabilityLevelDevelopment)
}
func (*statusExtension) Start(context.Context, component.Host) error { return nil }
func (*statusExtension) Shutdown(context.Context) error              { return nil }
func (*statusExtension) Ready() error {
	_, err := fmt.Fprintf(os.Stdout, "NETDATA_WORKER_READY %d\n", os.Getpid())
	return err
}
func (*statusExtension) NotReady() error { return nil }
