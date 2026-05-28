// SPDX-License-Identifier: GPL-3.0-or-later

package dovecot

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioSessions = collectorapi.Priority + iota
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

var charts = collectorapi.Charts{
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
	sessionsChart = collectorapi.Chart{
		ID:       "sessions",
		Title:    "Dovecot Active Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "dovecot.sessions",
		Priority: prioSessions,
		Dims: collectorapi.Dims{
			{ID: "num_connected_sessions", Name: "active"},
		},
	}
	loginsChart = collectorapi.Chart{
		ID:       "logins",
		Title:    "Dovecot Logins",
		Units:    "logins",
		Fam:      "logins",
		Ctx:      "dovecot.logins",
		Priority: prioLogins,
		Dims: collectorapi.Dims{
			{ID: "num_logins", Name: "logins"},
		},
	}
	authAttemptsChart = collectorapi.Chart{
		ID:       "auth",
		Title:    "Dovecot Authentications",
		Units:    "attempts/s",
		Fam:      "logins",
		Ctx:      "dovecot.auth",
		Priority: prioAuthenticationAttempts,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "auth_successes", Name: "ok", Algo: collectorapi.Incremental},
			{ID: "auth_failures", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	commandsChart = collectorapi.Chart{
		ID:       "commands",
		Title:    "Dovecot Commands",
		Units:    "commands",
		Fam:      "commands",
		Ctx:      "dovecot.commands",
		Priority: prioCommands,
		Dims: collectorapi.Dims{
			{ID: "num_cmds", Name: "commands"},
		},
	}
	pageFaultsChart = collectorapi.Chart{
		ID:       "faults",
		Title:    "Dovecot Page Faults",
		Units:    "faults/s",
		Fam:      "page faults",
		Ctx:      "dovecot.faults",
		Priority: prioPageFaults,
		Dims: collectorapi.Dims{
			{ID: "min_faults", Name: "minor", Algo: collectorapi.Incremental},
			{ID: "maj_faults", Name: "major", Algo: collectorapi.Incremental},
		},
	}
	contextSwitchesChart = collectorapi.Chart{
		ID:       "context_switches",
		Title:    "Dovecot Context Switches",
		Units:    "switches/s",
		Fam:      "context switches",
		Ctx:      "dovecot.context_switches",
		Priority: prioContextSwitches,
		Dims: collectorapi.Dims{
			{ID: "vol_cs", Name: "voluntary", Algo: collectorapi.Incremental},
			{ID: "invol_cs", Name: "involuntary", Algo: collectorapi.Incremental},
		},
	}
	diskIOChart = collectorapi.Chart{
		ID:       "io",
		Title:    "Dovecot Disk I/O",
		Units:    "KiB/s",
		Fam:      "disk",
		Ctx:      "dovecot.io",
		Priority: prioDiskIO,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "disk_input", Name: "read", Div: 1024, Algo: collectorapi.Incremental},
			{ID: "disk_output", Name: "write", Mul: -1, Div: 1024, Algo: collectorapi.Incremental},
		},
	}
	netTrafficChart = collectorapi.Chart{
		ID:       "net",
		Title:    "Dovecot Network Bandwidth",
		Units:    "kilobits/s",
		Fam:      "network",
		Ctx:      "dovecot.net",
		Priority: prioNetTraffic,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "read_bytes", Name: "read", Mul: 8, Div: 1000, Algo: collectorapi.Incremental},
			{ID: "write_bytes", Name: "write", Mul: -8, Div: 1000, Algo: collectorapi.Incremental},
		},
	}
	sysCallsChart = collectorapi.Chart{
		ID:       "syscalls",
		Title:    "Dovecot Number of SysCalls",
		Units:    "syscalls/s",
		Fam:      "system",
		Ctx:      "dovecot.syscalls",
		Priority: prioSysCalls,
		Dims: collectorapi.Dims{
			{ID: "read_count", Name: "read", Algo: collectorapi.Incremental},
			{ID: "write_count", Name: "write", Algo: collectorapi.Incremental},
		},
	}
	lookupsChart = collectorapi.Chart{
		ID:       "lookup",
		Title:    "Dovecot Lookups",
		Units:    "lookups/s",
		Fam:      "lookups",
		Ctx:      "dovecot.lookup",
		Priority: prioLookups,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "mail_lookup_path", Name: "path", Algo: collectorapi.Incremental},
			{ID: "mail_lookup_attr", Name: "attr", Algo: collectorapi.Incremental},
		},
	}
	cacheChart = collectorapi.Chart{
		ID:       "cache",
		Title:    "Dovecot Cache Hits",
		Units:    "hits/s",
		Fam:      "cache",
		Ctx:      "dovecot.cache",
		Priority: prioCachePerformance,
		Dims: collectorapi.Dims{
			{ID: "mail_cache_hits", Name: "hits", Algo: collectorapi.Incremental},
		},
	}
	authCacheChart = collectorapi.Chart{
		ID:       "auth_cache",
		Title:    "Dovecot Authentication Cache",
		Units:    "requests/s",
		Fam:      "cache",
		Ctx:      "dovecot.auth_cache",
		Priority: prioAuthCachePerformance,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "auth_cache_hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "auth_cache_misses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
)
