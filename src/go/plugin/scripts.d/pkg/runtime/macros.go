// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

// MacroContext collects all sources needed to substitute Nagios macros and
// build the environment map for plugin execution.
type MacroContext struct {
	Job        spec.JobSpec
	UserMacros map[string]string
	Vnode      VnodeInfo
	State      StateInfo
}

// VnodeInfo mirrors the subset of data needed from Netdata virtual nodes.
type VnodeInfo struct {
	Hostname string
	Address  string
	Alias    string
	Labels   map[string]string
	Custom   map[string]string
}

// StateInfo contains runtime state variables for macros (SERVICESTATE, etc.).
type StateInfo struct {
	ServiceState       string
	ServiceAttempt     int
	ServiceMaxAttempts int
	HostState          string
	HostStateID        string
}

// MacroSet holds the resolved string replacements consumed by argument and
// environment builders.
type MacroSet struct {
	CommandArgs []string
	Env         map[string]string
}

// BuildMacroSet resolves macros and environment variables for a job.
func BuildMacroSet(ctx MacroContext) MacroSet {
	env := make(map[string]string)

	now := time.Now()
	set := func(key, value string) {
		if value != "" {
			env[key] = value
		}
	}

	set("NAGIOS_PLUGIN", ctx.Job.Plugin)
	set("NAGIOS_JOB", ctx.Job.Name)
	set("NAGIOS_HOSTNAME", firstNonEmpty(ctx.Job.Vnode, ctx.Vnode.Hostname))
	set("NAGIOS_HOSTADDRESS", ctx.Vnode.Address)
	set("NAGIOS_HOSTALIAS", ctx.Vnode.Alias)
	set("NAGIOS_SERVICEDESC", ctx.Job.Name)
	set("NAGIOS_SERVICESTATE", ctx.State.ServiceState)
	set("NAGIOS_SERVICESTATEID", stateID(ctx.State.ServiceState))
	if ctx.State.ServiceAttempt > 0 {
		set("NAGIOS_SERVICEATTEMPT", fmt.Sprintf("%d", ctx.State.ServiceAttempt))
	}
	if ctx.State.ServiceMaxAttempts > 0 {
		set("NAGIOS_MAXSERVICEATTEMPTS", fmt.Sprintf("%d", ctx.State.ServiceMaxAttempts))
	}
	set("NAGIOS_HOSTSTATE", ctx.State.HostState)
	set("NAGIOS_HOSTSTATEID", ctx.State.HostStateID)
	set("NAGIOS_LONGDATETIME", now.Format(time.RFC1123))
	set("NAGIOS_SHORTDATETIME", now.Format("2006-01-02 15:04"))
	set("NAGIOS_DATE", now.Format("2006-01-02"))
	set("NAGIOS_TIME", now.Format("15:04:05"))
	set("NAGIOS_TIMET", fmt.Sprintf("%d", now.Unix()))

	for k, v := range ctx.UserMacros {
		macro := strings.ToUpper(k)
		if strings.HasPrefix(macro, "USER") {
			env[fmt.Sprintf("NAGIOS_%s", macro)] = v
		}
	}

	for k, v := range ctx.Vnode.Custom {
		key := fmt.Sprintf("NAGIOS__HOST%s", strings.ToUpper(k))
		env[key] = v
	}
	for k, v := range ctx.Vnode.Labels {
		key := fmt.Sprintf("NAGIOS__HOSTLABEL_%s", strings.ToUpper(k))
		env[key] = v
	}
	for k, v := range ctx.Job.CustomVars {
		key := fmt.Sprintf("NAGIOS__SERVICE%s", strings.ToUpper(k))
		env[key] = v
	}
	for idx := 0; idx < len(ctx.Job.ArgValues) && idx < spec.MaxArgMacros; idx++ {
		macro := fmt.Sprintf("NAGIOS_ARG%d", idx+1)
		env[macro] = ctx.Job.ArgValues[idx]
	}

	cmdArgs := append([]string{}, ctx.Job.Args...)
	cmdArgs = substituteArgs(cmdArgs, env)

	return MacroSet{CommandArgs: cmdArgs, Env: env}
}

func substituteArgs(args []string, env map[string]string) []string {
	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = replaceMacro(arg, env)
	}
	return resolved
}

func replaceMacro(value string, env map[string]string) string {
	replaced := value
	for key, val := range env {
		macro := fmt.Sprintf("$%s$", strings.TrimPrefix(key, "NAGIOS_"))
		replaced = strings.ReplaceAll(replaced, macro, val)
	}
	return replaced
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func stateID(state string) string {
	switch strings.ToUpper(state) {
	case "OK":
		return "0"
	case "WARNING":
		return "1"
	case "CRITICAL":
		return "2"
	case "UNKNOWN":
		return "3"
	default:
		return "3"
	}
}

// Placeholder for future macro expansion functions.
