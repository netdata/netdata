// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// https://github.com/Qihoo360/pika/blob/master/src/pika_admin.cc
// https://github.com/Qihoo360/pika/blob/a0dbdcf5897dd7800ba8a4d1eafce1595619ddc8/src/pika_admin.cc#L694-L710

const (
	infoSectionServer           = "# Server"
	infoSectionData             = "# Data"
	infoSectionClients          = "# Clients"
	infoSectionStats            = "# Stats"
	infoSectionCommandExecCount = "# Command_Exec_Count"
	infoSectionCPU              = "# CPU"
	infoSectionReplMaster       = "# Replication(MASTER)"
	infoSectionReplSlave        = "# Replication(SLAVE)"
	infoSectionReplMasterSlave  = "# Replication(Master && SLAVE)"
	infoSectionKeyspace         = "# Keyspace"
)

var infoSections = map[string]struct{}{
	infoSectionServer:           {},
	infoSectionData:             {},
	infoSectionClients:          {},
	infoSectionStats:            {},
	infoSectionCommandExecCount: {},
	infoSectionCPU:              {},
	infoSectionReplMaster:       {},
	infoSectionReplSlave:        {},
	infoSectionReplMasterSlave:  {},
	infoSectionKeyspace:         {},
}

func isInfoSection(line string) bool { _, ok := infoSections[line]; return ok }

func (p *Pika) collectInfo(ms map[string]int64, info string) {
	var curSection string

	sc := bufio.NewScanner(strings.NewReader(info))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) == 0 {
			curSection = ""
			continue
		}
		if strings.HasPrefix(line, "#") {
			if isInfoSection(line) {
				curSection = line
			}
			continue
		}

		field, value, ok := parseProperty(line)
		if !ok {
			continue
		}

		switch curSection {
		case infoSectionCommandExecCount:
			p.collectInfoCommandExecCountProperty(ms, field, value)
		case infoSectionKeyspace:
			p.collectInfoKeyspaceProperty(ms, field, value)
		default:
			collectNumericValue(ms, field, value)
		}
	}
}

var reKeyspaceValue = regexp.MustCompile(`^(.+)_keys=(\d+), expires=(\d+), invalid_keys=(\d+)`)

func (p *Pika) collectInfoKeyspaceProperty(ms map[string]int64, field, value string) {
	match := reKeyspaceValue.FindStringSubmatch(value)
	if match == nil {
		return
	}

	dataType, keys, expires, invalid := strings.ToLower(match[1]), match[2], match[3], match[4]
	collectNumericValue(ms, field+"_"+dataType+"_keys", keys)
	collectNumericValue(ms, field+"_"+dataType+"_expires_keys", expires)
	collectNumericValue(ms, field+"_"+dataType+"_invalid_keys", invalid)

	if !p.collectedDbs[field] {
		p.collectedDbs[field] = true
		p.addDbToKeyspaceCharts(field)
	}
}

func (p *Pika) collectInfoCommandExecCountProperty(ms map[string]int64, field, value string) {
	collectNumericValue(ms, "cmd_"+field+"_calls", value)

	if !p.collectedCommands[field] {
		p.collectedCommands[field] = true
		p.addCmdToCommandsCharts(field)
	}
}

func (p *Pika) addCmdToCommandsCharts(cmd string) {
	p.addDimToChart(chartCommandsCalls.ID, &module.Dim{
		ID:   "cmd_" + cmd + "_calls",
		Name: cmd,
		Algo: module.Incremental,
	})
}

func (p *Pika) addDbToKeyspaceCharts(db string) {
	p.addDimToChart(chartDbStringsKeys.ID, &module.Dim{
		ID:   db + "_strings_keys",
		Name: db,
	})
	p.addDimToChart(chartDbStringsExpiresKeys.ID, &module.Dim{
		ID:   db + "_strings_expires_keys",
		Name: db,
	})
	p.addDimToChart(chartDbStringsInvalidKeys.ID, &module.Dim{
		ID:   db + "_strings_invalid_keys",
		Name: db,
	})

	p.addDimToChart(chartDbHashesKeys.ID, &module.Dim{
		ID:   db + "_hashes_keys",
		Name: db,
	})
	p.addDimToChart(chartDbHashesExpiresKeys.ID, &module.Dim{
		ID:   db + "_hashes_expires_keys",
		Name: db,
	})
	p.addDimToChart(chartDbHashesInvalidKeys.ID, &module.Dim{
		ID:   db + "_hashes_invalid_keys",
		Name: db,
	})

	p.addDimToChart(chartDbListsKeys.ID, &module.Dim{
		ID:   db + "_lists_keys",
		Name: db,
	})
	p.addDimToChart(chartDbListsExpiresKeys.ID, &module.Dim{
		ID:   db + "_lists_expires_keys",
		Name: db,
	})
	p.addDimToChart(chartDbListsInvalidKeys.ID, &module.Dim{
		ID:   db + "_lists_invalid_keys",
		Name: db,
	})

	p.addDimToChart(chartDbZsetsKeys.ID, &module.Dim{
		ID:   db + "_zsets_keys",
		Name: db,
	})
	p.addDimToChart(chartDbZsetsExpiresKeys.ID, &module.Dim{
		ID:   db + "_zsets_expires_keys",
		Name: db,
	})
	p.addDimToChart(chartDbZsetsInvalidKeys.ID, &module.Dim{
		ID:   db + "_zsets_invalid_keys",
		Name: db,
	})

	p.addDimToChart(chartDbSetsKeys.ID, &module.Dim{
		ID:   db + "_sets_keys",
		Name: db,
	})
	p.addDimToChart(chartDbSetsExpiresKeys.ID, &module.Dim{
		ID:   db + "_sets_expires_keys",
		Name: db,
	})
	p.addDimToChart(chartDbSetsInvalidKeys.ID, &module.Dim{
		ID:   db + "_sets_invalid_keys",
		Name: db,
	})
}

func (p *Pika) addDimToChart(chartID string, dim *module.Dim) {
	chart := p.Charts().Get(chartID)
	if chart == nil {
		p.Warningf("error on adding '%s' dimension: can not find '%s' chart", dim.ID, chartID)
		return
	}
	if err := chart.AddDim(dim); err != nil {
		p.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func parseProperty(prop string) (field, value string, ok bool) {
	var sep byte
	if strings.HasPrefix(prop, "db") {
		sep = ' '
	} else {
		sep = ':'
	}
	i := strings.IndexByte(prop, sep)
	if i == -1 {
		return "", "", false
	}
	field, value = prop[:i], prop[i+1:]
	return field, value, field != "" && value != ""
}

func collectNumericValue(ms map[string]int64, field, value string) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return
	}
	if strings.IndexByte(value, '.') == -1 {
		ms[field] = int64(v)
	} else {
		ms[field] = int64(v * precision)
	}
}
