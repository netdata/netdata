// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"slices"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// Stock state mappings become dimension IDs and names on the plugins.d wire.
// Every state of the F5 pool-member session status must reach the Agent as one
// intact DIMENSION and a numeric SET, whichever state is currently active.
func TestStockF5PoolMemberSessionStatus_WireTokens(t *testing.T) {
	const (
		profileName = "f5-big-ip.yaml"
		tableOID    = "1.3.6.1.4.1.3375.2.2.5.3.2"
		metric      = "ltmPoolMemberSessionStatus"
		suffix      = "5.112.111.111.108.49.1.4.10.0.0.1.80"
	)
	wantStates := []string{"enabled", "addrdisabled", "servdisabled", "disabled", "forceddisabled"}

	for name, active := range map[string]struct {
		value int
		state string
	}{
		"servdisabled active": {value: 3, state: "servdisabled"},
		"enabled active":      {value: 1, state: "enabled"},
	} {
		t.Run(name, func(t *testing.T) {
			profile, err := ddsnmp.LoadProfileByName(profileName)
			require.NoError(t, err)
			profile = stockIdentitySelectRow(t, profile, stockIdentityFixture{TableOID: tableOID, Metric: metric})
			row := &profile.Definition.Metrics[0]
			row.Symbols = slices.DeleteFunc(row.Symbols, func(s ddprofiledefinition.SymbolConfig) bool { return s.Name != metric })
			require.Len(t, row.Symbols, 1)

			pdus := []gosnmp.SnmpPDU{
				{Name: row.Symbols[0].OID + "." + suffix, Type: gosnmp.Integer, Value: active.value},
				{Name: "1.3.6.1.4.1.3375.2.2.5.3.2.1.1." + suffix, Type: gosnmp.OctetString, Value: []byte("pool1")},
				{Name: "1.3.6.1.4.1.3375.2.2.5.3.2.1.19." + suffix, Type: gosnmp.OctetString, Value: []byte("node1")},
			}
			metrics := stockIdentityAcquire(t, profile, pdus)
			require.Len(t, metrics, 1)
			require.Len(t, metrics[0].MultiValue, len(wantStates))
			assert.Equal(t, int64(1), metrics[0].MultiValue[active.state])

			wire := renderIdentityMetrics(t, metrics)

			var charts, dims []string
			sets := make(map[string]string)
			inChart := false
			for _, line := range strings.Split(wire, "\n") {
				words := splitAgentQuotedWords(line)
				if len(words) == 0 {
					continue
				}
				switch words[0] {
				case "CHART", "BEGIN":
					require.GreaterOrEqual(t, len(words), 2, "chart command: %q", line)
					inChart = strings.Contains(words[1], ".snmp_device_prof_"+metric+"_")
					if inChart && words[0] == "CHART" {
						charts = append(charts, words[1])
					}
				case "END":
					inChart = false
				}
				if !inChart {
					continue
				}
				switch words[0] {
				case "DIMENSION":
					require.Len(t, words, 7, "DIMENSION fields must not shift: %q", line)
					assert.Equal(t, words[1], words[2], "dimension ID and name: %q", line)
					assert.Equal(t, "absolute", words[3], "dimension algorithm: %q", line)
					dims = append(dims, words[1])
				case "SET":
					require.Len(t, words, 3, "SET fields must not shift: %q", line)
					sets[words[1]] = words[2]
				}
			}

			require.Len(t, charts, 1, "the session status chart must be registered")
			assert.ElementsMatch(t, wantStates, dims, "every state must be one intact dimension")
			for _, state := range wantStates {
				want := "0"
				if state == active.state {
					want = "1"
				}
				assert.Equal(t, want, sets[state], "SET value for state %s", state)
			}
		})
	}
}

// splitAgentQuotedWords mirrors quoted_strings_splitter_pluginsd() in
// src/libnetdata/line_splitter/line_splitter.h, which the Agent uses to parse
// plugins.d commands, so the test sees the same tokens the Agent does. Like
// isspace_pluginsd(), it treats '=' as a separator.
func splitAgentQuotedWords(line string) []string {
	isSpace := func(c byte) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f' || c == '='
	}
	s := []byte(line)
	var words []string
	var quote byte
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	if i == len(s) {
		return nil
	}
	if s[i] == '\'' || s[i] == '"' {
		quote = s[i]
		i++
	}
	start := i
	for i < len(s) {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i += 2
		case quote != 0 && s[i] == quote:
			quote = 0
			s[i] = ' '
		case quote == 0 && isSpace(s[i]):
			words = append(words, string(s[start:i]))
			for i < len(s) && isSpace(s[i]) {
				i++
			}
			if i < len(s) && (s[i] == '\'' || s[i] == '"') {
				quote = s[i]
				i++
			}
			start = i
			if i == len(s) {
				return words
			}
		default:
			i++
		}
	}
	return append(words, string(s[start:]))
}
