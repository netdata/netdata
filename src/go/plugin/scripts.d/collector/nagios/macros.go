// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type macroSet struct {
	CommandArgs []string
	Env         map[string]string
}

func buildMacroSet(job JobConfig, vnode vnodeInfo, state macroState, now time.Time) macroSet {
	env := make(map[string]string)
	set := func(key, value string) {
		if value != "" {
			env[key] = value
		}
	}

	set("NAGIOS_PLUGIN", job.Plugin)
	set("NAGIOS_JOB", job.Name)
	set("NAGIOS_HOSTNAME", firstNonEmpty(job.Vnode, vnode.Hostname))
	set("NAGIOS_HOSTADDRESS", vnode.Labels["_address"])
	set("NAGIOS_HOSTALIAS", vnode.Labels["_alias"])
	set("NAGIOS_SERVICEDESC", job.Name)
	set("NAGIOS_SERVICESTATE", state.ServiceState)
	set("NAGIOS_SERVICESTATEID", stateID(state.ServiceState))
	if state.ServiceAttempt > 0 {
		set("NAGIOS_SERVICEATTEMPT", fmt.Sprintf("%d", state.ServiceAttempt))
	}
	if state.ServiceMaxAttempts > 0 {
		set("NAGIOS_MAXSERVICEATTEMPTS", fmt.Sprintf("%d", state.ServiceMaxAttempts))
	}
	set("NAGIOS_HOSTSTATE", nagiosHostStateUp)
	set("NAGIOS_HOSTSTATEID", nagiosHostStateUpID)
	set("NAGIOS_LONGDATETIME", now.Format(time.RFC1123))
	set("NAGIOS_SHORTDATETIME", now.Format("2006-01-02 15:04"))
	set("NAGIOS_DATE", now.Format("2006-01-02"))
	set("NAGIOS_TIME", now.Format("15:04:05"))
	set("NAGIOS_TIMET", fmt.Sprintf("%d", now.Unix()))

	for k, v := range vnode.Labels {
		if strings.HasPrefix(k, "_") && k != "_address" && k != "_alias" {
			env[fmt.Sprintf("NAGIOS__HOST%s", strings.ToUpper(k[1:]))] = v
		} else if !strings.HasPrefix(k, "_") {
			env[fmt.Sprintf("NAGIOS__HOSTLABEL_%s", strings.ToUpper(k))] = v
		}
	}
	for k, v := range job.CustomVars {
		key := fmt.Sprintf("NAGIOS__SERVICE%s", strings.ToUpper(k))
		env[key] = v
	}
	for idx := 0; idx < len(job.ArgValues) && idx < maxArgMacros; idx++ {
		env[fmt.Sprintf("NAGIOS_ARG%d", idx+1)] = job.ArgValues[idx]
	}

	cmdArgs := make([]string, len(job.Args))
	for i, arg := range job.Args {
		cmdArgs[i] = replaceMacro(arg, env)
	}
	return macroSet{CommandArgs: cmdArgs, Env: env}
}

func vnodeInfoFromVirtualNode(vn *vnodes.VirtualNode, fallbackHostname string) vnodeInfo {
	info := vnodeInfo{
		Hostname: fallbackHostname,
		Labels:   make(map[string]string),
	}
	if vn == nil {
		return info
	}
	if vn.Hostname != "" {
		info.Hostname = vn.Hostname
	}
	maps.Copy(info.Labels, vn.Labels)
	return info
}

func replaceMacro(value string, env map[string]string) string {
	return replaceMacroWithStack(value, env, nil)
}

func replaceMacroWithStack(value string, env map[string]string, stack map[string]struct{}) string {
	if value == "" || len(env) == 0 {
		return value
	}

	var b strings.Builder
	b.Grow(len(value))

	for i := 0; i < len(value); {
		if value[i] != '$' {
			b.WriteByte(value[i])
			i++
			continue
		}

		end := strings.IndexByte(value[i+1:], '$')
		if end < 0 {
			b.WriteByte(value[i])
			i++
			continue
		}

		token := value[i+1 : i+1+end]
		if token == "" {
			b.WriteString("$$")
			i += 2
			continue
		}

		macroKey := "NAGIOS_" + token
		macroValue, ok := env[macroKey]
		if !ok {
			b.WriteString(value[i : i+end+2])
			i += end + 2
			continue
		}

		b.WriteString(resolveMacroValue(macroKey, macroValue, env, stack))
		i += end + 2
	}

	return b.String()
}

func resolveMacroValue(key, raw string, env map[string]string, stack map[string]struct{}) string {
	if stack == nil {
		stack = make(map[string]struct{})
	}
	if _, ok := stack[key]; ok {
		return macroTokenForKey(key)
	}

	stack[key] = struct{}{}
	resolved := replaceMacroWithStack(raw, env, stack)
	delete(stack, key)
	return resolved
}

func macroTokenForKey(key string) string {
	return "$" + strings.TrimPrefix(key, "NAGIOS_") + "$"
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
	case nagiosStateOK:
		return nagiosStateIDOK
	case nagiosStateWarning:
		return nagiosStateIDWarning
	case nagiosStateCritical:
		return nagiosStateIDCritical
	case nagiosStateUnknown:
		return nagiosStateIDUnknown
	default:
		return nagiosStateIDUnknown
	}
}
