package zabbixpreproc

import "time"

// Limits defines configurable constraints for preprocessing operations.
// Default values match Zabbix 7.x behavior for maximum compatibility.
type Limits struct {
	// JavaScript preprocessing limits
	JavaScript JSLimits

	// Regular expression preprocessing limits
	Regex RegexLimits
}

// JSLimits configures JavaScript preprocessing constraints.
type JSLimits struct {
	// Timeout is the maximum execution time for a single JavaScript step.
	// Zabbix default: 10 seconds. Scripts exceeding this are terminated.
	Timeout time.Duration

	// MaxCallStackSize limits recursion depth to prevent stack exhaustion.
	// Zabbix default: 1000. Matches ZBX_ES_STACK_LIMIT.
	MaxCallStackSize int

	// MaxHttpRequests limits HttpRequest objects per script execution.
	// Zabbix default: 10. Prevents resource exhaustion.
	MaxHttpRequests int
}

// RegexLimits configures regular expression preprocessing constraints.
type RegexLimits struct {
	// MaxCaptureGroups is the maximum number of capture groups (\1-\N).
	// Zabbix default: 10 (groups \0-\9). Matches ZBX_REGEXP_GROUPS_MAX.
	MaxCaptureGroups int

	// MatchTimeout is the maximum time for a single regex match operation.
	// Prevents ReDoS attacks. Recommended: 1s for safety.
	MatchTimeout time.Duration

	// ReplaceTimeout is the maximum time for regex replacement operations.
	// Zabbix default: 3 seconds. Matches ZBX_REGEX_REPL_TIMEOUT.
	ReplaceTimeout time.Duration

	// MaxPatternLength limits the size of regex patterns.
	// Zabbix default: unlimited. Set to 0 for no limit.
	MaxPatternLength int

	// UsePCREFeatures enables lookahead/lookbehind via regexp2.
	// If false, uses stdlib regexp (RE2) which lacks these features.
	// Default: true for Zabbix compatibility.
	UsePCREFeatures bool
}

// DefaultLimits returns limits matching Zabbix 7.x defaults.
func DefaultLimits() Limits {
	return Limits{
		JavaScript: JSLimits{
			Timeout:          10 * time.Second,
			MaxCallStackSize: 1000,
			MaxHttpRequests:  10,
		},
		Regex: RegexLimits{
			MaxCaptureGroups: 10,
			MatchTimeout:     time.Second,
			ReplaceTimeout:   3 * time.Second,
			MaxPatternLength: 0, // No limit (matches Zabbix)
			UsePCREFeatures:  true,
		},
	}
}
