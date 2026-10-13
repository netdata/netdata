// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func observeStateSet(instrument metrix.StateSetInstrument, active string) {
	if active == "" {
		return
	}
	instrument.Enable(active)
}

func observeStateSetVec(vec metrix.SnapshotStateSetVec, active string, labels ...string) {
	if active == "" {
		return
	}
	vec.WithLabelValues(labels...).Enable(active)
}

func boolState(ok bool, trueState, falseState string) string {
	if ok {
		return trueState
	}
	return falseState
}

func parsePANOSAffirmativeField(field, v string) (bool, error) {
	raw := strings.TrimSpace(v)
	switch strings.ToLower(raw) {
	case "yes", "true", "enabled", "enable", "up", "valid":
		return true, nil
	case "no", "false", "disabled", "disable", "down", "invalid", "off", "absent", "not present", "not-present":
		return false, nil
	default:
		if raw == "" {
			return false, fmt.Errorf("%s: missing status", field)
		}
		return false, fmt.Errorf("%s: invalid status %q", field, raw)
	}
}

func normalizeUpDownState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "":
		return ""
	case "up":
		return "up"
	case "down":
		return "down"
	default:
		return "unknown"
	}
}

func alarmState(alarm bool) string {
	return boolState(alarm, "alarm", "clear")
}

func parsePANOSAlarmField(field, v string) (bool, error) {
	raw := strings.TrimSpace(v)
	switch strings.ToLower(raw) {
	case "true", "yes", "on", "active", "alarm", "critical":
		return true, nil
	case "false", "no", "off", "inactive", "ok", "normal", "clear", "none":
		return false, nil
	default:
		if raw == "" {
			return false, fmt.Errorf("%s: missing status", field)
		}
		return false, fmt.Errorf("%s: invalid status %q", field, raw)
	}
}

func parsePANOSDecimalField(field, v string, scale int64) (int64, error) {
	raw := strings.TrimSpace(v)
	v = strings.ReplaceAll(raw, ",", "")
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("%s: invalid decimal %q", field, raw)
	}
	return int64(math.Round(f * float64(scale))), nil
}

func parseRequiredPANOSDecimalField(field, v string, scale int64) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, fmt.Errorf("%s: missing decimal", field)
	}
	return parsePANOSDecimalField(field, v, scale)
}

func panosCommandName(cmd string) string {
	switch cmd {
	case systemInfoCommand:
		return "system info query"
	case haStateCommand:
		return "HA state query"
	case environmentCommand:
		return "environmentals query"
	case licenseInfoCommand:
		return "license info query"
	case ipsecSACommand:
		return "IPsec SA query"
	default:
		return bgpCommandName(cmd)
	}
}
