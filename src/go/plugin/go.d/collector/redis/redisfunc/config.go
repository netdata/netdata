// SPDX-License-Identifier: GPL-3.0-or-later

package redisfunc

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

// DefaultTopQueriesLimit is the top-queries row limit used when none is configured.
const DefaultTopQueriesLimit = 500

// FunctionsConfig holds the Function settings of a redis job.
type FunctionsConfig struct {
	// Timeout is the collector timeout, the fallback for Function timeouts.
	// The collector sets it; it is not a config option.
	Timeout confopt.Duration `yaml:"-" json:"-"`

	TopQueries TopQueriesConfig `yaml:"top_queries,omitempty" json:"top_queries"`
}

type TopQueriesConfig struct {
	Disabled bool             `yaml:"disabled"          json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty"   json:"limit"`
}

func (c FunctionsConfig) topQueriesTimeout() time.Duration {
	if c.TopQueries.Timeout == 0 {
		return c.Timeout.Duration()
	}
	return c.TopQueries.Timeout.Duration()
}

func (c FunctionsConfig) topQueriesLimit() int {
	if c.TopQueries.Limit <= 0 {
		return DefaultTopQueriesLimit
	}
	return c.TopQueries.Limit
}
