// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const defaultTopQueriesLimit = 500

// FunctionsConfig holds MySQL function-specific settings.
//
// Timeout is an internal fallback timeout inherited from collector config.
// It is intentionally excluded from YAML/JSON.
type FunctionsConfig struct {
	Timeout confopt.Duration `yaml:"-" json:"-"`

	TopQueries   TopQueriesConfig   `yaml:"top_queries,omitempty" json:"top_queries"`
	DeadlockInfo DeadlockInfoConfig `yaml:"deadlock_info,omitempty" json:"deadlock_info"`
	ErrorInfo    ErrorInfoConfig    `yaml:"error_info,omitempty" json:"error_info"`
}

type TopQueriesConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty" json:"limit"`
}

type DeadlockInfoConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type ErrorInfoConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

func (c FunctionsConfig) topQueriesDisabled() bool {
	return c.TopQueries.Disabled
}

func (c FunctionsConfig) deadlockInfoDisabled() bool {
	return c.DeadlockInfo.Disabled
}

func (c FunctionsConfig) errorInfoDisabled() bool {
	return c.ErrorInfo.Disabled
}

func (c FunctionsConfig) topQueriesTimeout() time.Duration {
	if c.TopQueries.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.TopQueries.Timeout.Duration()
}

func (c FunctionsConfig) deadlockInfoTimeout() time.Duration {
	if c.DeadlockInfo.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.DeadlockInfo.Timeout.Duration()
}

func (c FunctionsConfig) errorInfoTimeout() time.Duration {
	if c.ErrorInfo.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.ErrorInfo.Timeout.Duration()
}

func (c FunctionsConfig) collectorTimeout() time.Duration {
	return c.Timeout.Duration()
}

func (c FunctionsConfig) topQueriesLimit() int {
	if c.TopQueries.Limit <= 0 {
		return defaultTopQueriesLimit
	}
	return c.TopQueries.Limit
}
