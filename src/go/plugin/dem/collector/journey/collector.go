// SPDX-License-Identifier: GPL-3.0-or-later

package journey

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	shared "github.com/netdata/netdata/go/plugins/plugin/dem/collector/synthetic"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Secret struct {
	Name  string `yaml:"name"  json:"name"`
	Value string `yaml:"value" json:"value"`
}

type Config struct {
	Name                string           `yaml:"name"                  json:"name"`
	UpdateEvery         int              `yaml:"update_every"          json:"update_every"`
	Timeout             confopt.Duration `yaml:"timeout"               json:"timeout"`
	Script              string           `yaml:"script,omitempty"      json:"script,omitempty"`
	ScriptPath          string           `yaml:"script_path,omitempty" json:"script_path,omitempty"`
	Secrets             []Secret         `yaml:"secrets,omitempty"     json:"secrets,omitempty"`
	ScreenshotOnFailure bool             `yaml:"screenshot_on_failure" json:"screenshot_on_failure"`
}

type Dependencies = shared.Dependencies

type Collector struct {
	collectorapi.Base
	Config  `yaml:",inline" json:""`
	runtime *shared.Runtime
}

func Creator(deps Dependencies) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 900,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New(deps) },
		Config:          func() any { return &Config{} },
		JobConfigSchema: schema,
		StoreFirst:      true,
	}
}
func New(deps Dependencies) *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery: 900,
			Timeout:     confopt.Duration(2 * time.Minute),
		},
		runtime: shared.New(deps),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.runtime.MetricStore() }
func (c *Collector) ChartTemplateYAML() string          { return charts }

var secretName = regexp.MustCompile(`^DEM_SECRET_[A-Za-z0-9_]+$`)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if (c.Script == "") == (c.ScriptPath == "") || (c.Script != "" && strings.TrimSpace(c.Script) == "") {
		return errors.New("configure exactly one nonempty script or script_path")
	}
	if c.ScriptPath != "" {
		if !filepath.IsAbs(c.ScriptPath) {
			return errors.New("script_path must be absolute")
		}
		switch filepath.Ext(c.ScriptPath) {
		case ".js", ".ts", ".mjs", ".mts", ".cjs", ".cts":
		default:
			return errors.New("script_path must reference a JavaScript or TypeScript entry file")
		}
		info, err := os.Stat(c.ScriptPath)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("script_path must reference a readable regular file")
		}
		f, err := os.Open(c.ScriptPath)
		if err != nil {
			return errors.New("script_path must be readable")
		}
		info, err = f.Stat()
		_ = f.Close()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("script_path must reference a regular file")
		}
	}
	names := make(map[string]struct{}, len(c.Secrets))
	for _, secret := range c.Secrets {
		if !secretName.MatchString(secret.Name) {
			return errors.New(
				"secret names must start with DEM_SECRET_ and contain only letters, digits or underscores",
			)
		}
		if _, exists := names[secret.Name]; exists {
			return errors.New("secret names must be unique")
		}
		if strings.ContainsRune(secret.Value, 0) {
			return errors.New("secret values must not contain NUL")
		}
		names[secret.Name] = struct{}{}
	}
	return c.runtime.Init(c.Name, c.UpdateEvery, time.Duration(c.Timeout))
}
func (c *Collector) Check(ctx context.Context) error { return c.runtime.Check(ctx, model.Journey) }
func (c *Collector) Run(ctx context.Context, ready func()) error {
	return c.runtime.Run(
		ctx,
		ready,
		model.Job{
			JobID:          "journey:" + c.Name,
			Kind:           model.Journey,
			Name:           c.Name,
			CadenceSeconds: c.UpdateEvery,
			TimeoutSeconds: time.Duration(c.Timeout).Seconds(),
		},
	)
}
func (c *Collector) Collect(ctx context.Context) error {
	secrets := make(map[string]string, len(c.Secrets))
	for _, secret := range c.Secrets {
		secrets[secret.Name] = secret.Value
	}
	return c.runtime.Collect(
		ctx,
		model.Request{
			Kind:       model.Journey,
			Name:       c.Name,
			Script:     c.Script,
			ScriptPath: c.ScriptPath,
			Secrets:    secrets,
			Timeout:    time.Duration(c.Timeout),
			Capture:    c.ScreenshotOnFailure,
		},
	)
}
func (c *Collector) Cleanup(context.Context) {}
