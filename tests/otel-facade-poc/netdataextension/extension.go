// SPDX-License-Identifier: GPL-3.0-or-later

package netdataextension

import (
	"context"

	"github.com/netdata/netdata/otel-facade-poc/internal/control"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
)

type Config struct {
	Revision uint64 `mapstructure:"revision"`
}

type statusExtension struct {
	revision   uint64
	controller *control.Controller
}

var _ extensioncapabilities.PipelineWatcher = (*statusExtension)(nil)

func NewFactory() extension.Factory {
	return extension.NewFactory(component.MustNewType("netdata"), func() component.Config { return &Config{} },
		func(_ context.Context, _ extension.Settings, cfg component.Config) (extension.Extension, error) {
			return &statusExtension{revision: cfg.(*Config).Revision}, nil
		}, component.StabilityLevelDevelopment)
}

func (e *statusExtension) Start(context.Context, component.Host) error {
	var err error
	e.controller, err = control.Process()
	return err
}

func (*statusExtension) Shutdown(context.Context) error { return nil }

func (e *statusExtension) Ready() error { return e.controller.Ready(e.revision, true) }

func (e *statusExtension) NotReady() error { return e.controller.Ready(e.revision, false) }
