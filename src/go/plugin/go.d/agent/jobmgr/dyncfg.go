// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
)

func (m *Manager) dyncfgConfig(fn functions.Function) {
	if len(fn.Args) < 2 {
		m.Warningf("dyncfg: %s: missing required arguments, want 3 got %d", fn.Name, len(fn.Args))
		m.dyncfgApi.SendCodef(fn, 400, "Missing required arguments. Need at least 2, but got %d.", len(fn.Args))
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgApi.SendCodef(fn, 503, "Job manager is shutting down.")
	default:
	}

	//m.Infof("QQ FN: '%s'", fn)

	m.dyncfgQueuedExec(fn)
}

func (m *Manager) dyncfgQueuedExec(fn functions.Function) {
	id := fn.Args[0]

	switch {
	case strings.HasPrefix(id, m.dyncfgCollectorPrefixValue()):
		m.dyncfgCollectorExec(fn)
	case strings.HasPrefix(id, m.dyncfgVnodePrefixValue()):
		m.dyncfgVnodeExec(fn)
	default:
		m.dyncfgApi.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Name, id)
	}
}

func (m *Manager) dyncfgSeqExec(fn functions.Function) {
	id := fn.Args[0]

	switch {
	case strings.HasPrefix(id, m.dyncfgCollectorPrefixValue()):
		m.dyncfgCollectorSeqExec(fn)
	case strings.HasPrefix(id, m.dyncfgVnodePrefixValue()):
		m.dyncfgVnodeSeqExec(fn)
	default:
		m.dyncfgApi.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Name, id)
	}
}

func unmarshalPayload(dst any, fn functions.Function) error {
	if v := reflect.ValueOf(dst); v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("invalid config: expected a pointer to a struct, got a %s", v.Type())
	}
	if fn.ContentType == "application/json" {
		return json.Unmarshal(fn.Payload, dst)
	}
	return yaml.Unmarshal(fn.Payload, dst)
}

func getFnSourceValue(fn functions.Function, key string) string {
	prefix := key + "="
	for _, part := range strings.Split(fn.Source, ",") {
		if v, ok := strings.CutPrefix(part, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func getDyncfgCommand(fn functions.Function) dyncfg.Command {
	if len(fn.Args) < 2 {
		return ""
	}
	return dyncfg.Command(strings.ToLower(fn.Args[1]))
}
