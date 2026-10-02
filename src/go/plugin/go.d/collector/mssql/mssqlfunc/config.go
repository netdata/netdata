// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type FunctionsConfig struct {
	TopQueries   TopQueriesConfig   `yaml:"top_queries,omitempty" json:"top_queries"`
	DeadlockInfo DeadlockInfoConfig `yaml:"deadlock_info,omitempty" json:"deadlock_info"`
	ErrorInfo    ErrorInfoConfig    `yaml:"error_info,omitempty" json:"error_info"`
}

type TopQueriesConfig struct {
	Disabled       bool             `yaml:"disabled" json:"disabled"`
	Timeout        confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit          int              `yaml:"limit,omitempty" json:"limit"`
	TimeWindowDays int              `yaml:"time_window_days,omitempty" json:"time_window_days"`
}

type DeadlockInfoConfig struct {
	Disabled      bool             `yaml:"disabled" json:"disabled"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	UseRingBuffer bool             `yaml:"use_ring_buffer" json:"use_ring_buffer"`
}

type ErrorInfoConfig struct {
	Disabled      bool             `yaml:"disabled" json:"disabled"`
	Timeout       confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	SessionName   string           `yaml:"session_name,omitempty" json:"session_name,omitempty"`
	UseRingBuffer bool             `yaml:"use_ring_buffer" json:"use_ring_buffer"`
}

// Function timeouts default independently of the metrics timeout: a slow diagnostic query
// must not inherit a budget tuned for lightweight metric collection.
const defaultMSSQLFunctionTimeout = 30 * time.Second

func newMSSQLFunctionTimeout(option string, configured confopt.Duration) mssqlFunctionTimeout {
	if configured == 0 {
		return mssqlFunctionTimeout{
			option: option,
			value:  defaultMSSQLFunctionTimeout,
		}
	}
	return mssqlFunctionTimeout{
		option: option,
		value:  configured.Duration(),
	}
}

func (c FunctionsConfig) topQueriesTimeout() mssqlFunctionTimeout {
	return newMSSQLFunctionTimeout("top_queries", c.TopQueries.Timeout)
}

func (c FunctionsConfig) topQueriesLimit() int {
	if c.TopQueries.Limit <= 0 {
		return 500
	}
	return c.TopQueries.Limit
}

func (c FunctionsConfig) topQueriesTimeWindowDays() int {
	if c.TopQueries.TimeWindowDays == -1 {
		return 0 // -1 means "query all history"
	}
	if c.TopQueries.TimeWindowDays <= 0 {
		return 7
	}
	return c.TopQueries.TimeWindowDays
}

func (c FunctionsConfig) deadlockInfoTimeout() mssqlFunctionTimeout {
	return newMSSQLFunctionTimeout("deadlock_info", c.DeadlockInfo.Timeout)
}

func (c FunctionsConfig) errorInfoTimeout() mssqlFunctionTimeout {
	return newMSSQLFunctionTimeout("error_info", c.ErrorInfo.Timeout)
}

func (c FunctionsConfig) errorInfoSessionName() string {
	if strings.TrimSpace(c.ErrorInfo.SessionName) == "" {
		return "netdata_errors"
	}
	return c.ErrorInfo.SessionName
}
