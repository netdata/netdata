// SPDX-License-Identifier: GPL-3.0-or-later

package dovecot

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioSessions = module.Priority + iota
	prioLogins
	prioAuthenticationAttempts
	prioCommands
	prioPageFaults
	prioContextSwitches
	prioDiskIO
	prioNetTraffic
	prioSysCalls
	prioLookups
	prioCachePerformance
	prioAuthCachePerformance
)

var charts = module.Charts{
	sessionsChart.Copy(),
	loginsChart.Copy(),
	authAttemptsChart.Copy(),
	commandsChart.Copy(),
	pageFaultsChart.Copy(),
	contextSwitchesChart.Copy(),
	diskIOChart.Copy(),
	netTrafficChart.Copy(),
	sysCallsChart.Copy(),
	lookupsChart.Copy(),
	cacheChart.Copy(),
	authCacheChart.Copy(),
}

var (
	sessionsChart = module.Chart{
		ID:       "sessions",
		Title:    "Dovecot Active Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "dovecot.sessions",
		Priority: prioSessions,
		Dims: module.Dims{
			{ID: "num_connected_sessions", Name: "active"},
		},
	}
	loginsChart = module.Chart{
		ID:       "logins",
		Title:    "Dovecot Logins",
		Units:    "logins",
		Fam:      "logins",
		Ctx:      "dovecot.logins",
		Priority: prioLogins,
		Dims: module.Dims{
			{ID: "num_logins", Name: "logins"},
		},
	}
	authAttemptsChart = module.Chart{
		ID:       "auth",
		Title:    "Dovecot Authentications",
		Units:    "attempts/s",
		Fam:      "logins",
		Ctx:      "dovecot.auth",
		Priority: prioAuthenticationAttempts,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "auth_successes", Name: "ok", Algo: module.Incremental},
			{ID: "auth_failures", Name: "failed", Algo: module.Incremental},
		},
	}
	commandsChart = module.Chart{
		ID:       "commands",
		Title:    "Dovecot Commands",
		Units:    "commands",
		Fam:      "commands",
		Ctx:      "dovecot.commands",
		Priority: prioCommands,
		Dims: module.Dims{
			{ID: "num_cmds", Name: "commands"},
		},
	}
	pageFaultsChart = module.Chart{
		ID:       "faults",
		Title:    "Dovecot Page Faults",
		Units:    "faults/s",
		Fam:      "page faults",
		Ctx:      "dovecot.faults",
		Priority: prioPageFaults,
		Dims: module.Dims{
			{ID: "min_faults", Name: "minor", Algo: module.Incremental},
			{ID: "maj_faults", Name: "major", Algo: module.Incremental},
		},
	}
	contextSwitchesChart = module.Chart{
		ID:       "context_switches",
		Title:    "Dovecot Context Switches",
		Units:    "switches/s",
		Fam:      "context switches",
		Ctx:      "dovecot.context_switches",
		Priority: prioContextSwitches,
		Dims: module.Dims{
			{ID: "vol_cs", Name: "voluntary", Algo: module.Incremental},
			{ID: "invol_cs", Name: "involuntary", Algo: module.Incremental},
		},
	}
	diskIOChart = module.Chart{
		ID:       "io",
		Title:    "Dovecot Disk I/O",
		Units:    "KiB/s",
		Fam:      "disk",
		Ctx:      "dovecot.io",
		Priority: prioDiskIO,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "disk_input", Name: "read", Div: 1024, Algo: module.Incremental},
			{ID: "disk_output", Name: "write", Mul: -1, Div: 1024, Algo: module.Incremental},
		},
	}
	netTrafficChart = module.Chart{
		ID:       "net",
		Title:    "Dovecot Network Bandwidth",
		Units:    "kilobits/s",
		Fam:      "network",
		Ctx:      "dovecot.net",
		Priority: prioNetTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "read_bytes", Name: "read", Mul: 8, Div: 1000, Algo: module.Incremental},
			{ID: "write_bytes", Name: "write", Mul: -8, Div: 1000, Algo: module.Incremental},
		},
	}
	sysCallsChart = module.Chart{
		ID:       "syscalls",
		Title:    "Dovecot Number of SysCalls",
		Units:    "syscalls/s",
		Fam:      "system",
		Ctx:      "dovecot.syscalls",
		Priority: prioSysCalls,
		Dims: module.Dims{
			{ID: "read_count", Name: "read", Algo: module.Incremental},
			{ID: "write_count", Name: "write", Algo: module.Incremental},
		},
	}
	lookupsChart = module.Chart{
		ID:       "lookup",
		Title:    "Dovecot Lookups",
		Units:    "lookups/s",
		Fam:      "lookups",
		Ctx:      "dovecot.lookup",
		Priority: prioLookups,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "mail_lookup_path", Name: "path", Algo: module.Incremental},
			{ID: "mail_lookup_attr", Name: "attr", Algo: module.Incremental},
		},
	}
	cacheChart = module.Chart{
		ID:       "cache",
		Title:    "Dovecot Cache Hits",
		Units:    "hits/s",
		Fam:      "cache",
		Ctx:      "dovecot.cache",
		Priority: prioCachePerformance,
		Dims: module.Dims{
			{ID: "mail_cache_hits", Name: "hits", Algo: module.Incremental},
		},
	}
	authCacheChart = module.Chart{
		ID:       "auth_cache",
		Title:    "Dovecot Authentication Cache",
		Units:    "requests/s",
		Fam:      "cache",
		Ctx:      "dovecot.auth_cache",
		Priority: prioAuthCachePerformance,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "auth_cache_hits", Name: "hits", Algo: module.Incremental},
			{ID: "auth_cache_misses", Name: "misses", Algo: module.Incremental},
		},
	}
)
