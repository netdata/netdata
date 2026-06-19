// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"gopkg.in/yaml.v2"
)

type fileRootConfig struct {
	Jobs []map[string]any `yaml:"jobs"`
}

func LoadFileConfigs(roots []string) ([]Config, []error) {
	var cfgs []Config
	var errs []error

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		dir := filepath.Join(root, "ss")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}

			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".conf" && ext != ".yaml" && ext != ".yml" {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			stem := strings.TrimSpace(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))

			jobs, jobErrs := loadFileConfigJobs(path)
			errs = append(errs, jobErrs...)
			if len(jobs) == 0 {
				continue
			}

			sourceType := fileConfigSourceType(path)
			source := "file=" + path
			for i, job := range jobs {
				cfg := Config(job)
				if strings.TrimSpace(cfg.Name()) == "" {
					errs = append(errs, fmt.Errorf("secretstore file config '%s' job %d: store name is required", path, i+1))
					continue
				}
				if cfg.Kind() == "" {
					cfg.SetKind(StoreKind(stem))
				}
				cfg.SetSource(source)
				cfg.SetSourceType(sourceType)
				if err := validateFileConfig(cfg); err != nil {
					errs = append(errs, fmt.Errorf("secretstore file config '%s' job %d: %w", path, i+1, err))
					continue
				}
				cfgs = append(cfgs, cfg)
			}
		}
	}

	return cfgs, errs
}

func loadFileConfigJobs(path string) ([]map[string]any, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []error{fmt.Errorf("secretstore file config '%s': %w", path, err)}
	}

	var root fileRootConfig
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, []error{fmt.Errorf("secretstore file config '%s': %w", path, err)}
	}

	return root.Jobs, nil
}

func fileConfigSourceType(path string) string {
	if pluginconfig.IsStock(path) {
		return confgroup.TypeStock
	}
	return confgroup.TypeUser
}

func validateFileConfig(cfg Config) error {
	if cfg == nil {
		return fmt.Errorf("store config is nil")
	}
	if strings.TrimSpace(cfg.Name()) == "" {
		return fmt.Errorf("store name is required")
	}
	if err := validateStoreName(cfg.Name()); err != nil {
		return fmt.Errorf("invalid store name '%s': %w", cfg.Name(), err)
	}
	if strings.TrimSpace(string(cfg.Kind())) == "" {
		return fmt.Errorf("store kind is required")
	}
	if strings.TrimSpace(cfg.Source()) == "" {
		return fmt.Errorf("store source is required")
	}
	switch cfg.SourceType() {
	case confgroup.TypeUser, confgroup.TypeStock:
		return nil
	default:
		return fmt.Errorf("invalid store source type '%s'", cfg.SourceType())
	}
}
