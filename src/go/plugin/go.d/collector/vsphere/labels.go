// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func resourceEnrichmentLabels(labels map[string]string) []metrix.Label {
	if len(labels) == 0 {
		return nil
	}

	keys := make([]string, 0, len(labels))
	for key, value := range labels {
		if key != "" && value != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}

	sort.Strings(keys)

	out := make([]metrix.Label, 0, len(keys))
	for _, key := range keys {
		out = append(out, metrix.Label{Key: key, Value: labels[key]})
	}
	return out
}

func getVMClusterName(vm *rs.VM) string {
	if isStandaloneHostClusterID(vm.Hier.Cluster.ID) {
		return ""
	}
	return vm.Hier.Cluster.Name
}

func getHostClusterName(host *rs.Host) string {
	if isStandaloneHostClusterID(host.Hier.Cluster.ID) {
		return ""
	}
	return host.Hier.Cluster.Name
}

func isStandaloneHostClusterID(id string) bool {
	return strings.HasPrefix(id, "domain-s")
}
