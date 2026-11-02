// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"errors"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

type deviceSnapshot struct {
	serial, name, model, productType string
	networkID, networkName           string
	status                           string
}

type availabilityRecord struct {
	status, networkID string
	ambiguous         bool
}

func normalize(
	networks []merakiapi.Network,
	devices []merakiapi.Device,
	availability []merakiapi.DeviceAvailability,
	organizationID string,
) ([]deviceSnapshot, error) {
	networkNames := make(map[string]string, len(networks))
	for _, n := range networks {
		if n.ID == "" {
			continue // Optional metadata cannot establish or remove device membership.
		}
		if n.OrganizationID != "" && n.OrganizationID != organizationID {
			return nil, errors.New("Meraki network belongs to a different organization")
		}
		if name, ok := networkNames[n.ID]; ok && name != n.Name {
			return nil, errors.New("conflicting Meraki network records")
		}
		networkNames[n.ID] = n.Name
	}
	statuses := make(map[string]availabilityRecord, len(availability))
	for _, a := range availability {
		if a.Serial == "" {
			continue
		}
		record := availabilityRecord{
			status:    a.Status,
			networkID: a.Network.ID,
		}
		if previous, ok := statuses[a.Serial]; ok && previous != record {
			record.ambiguous = true
		}
		statuses[a.Serial] = record
	}
	assigned := make(map[string]deviceSnapshot, len(devices))
	for _, d := range devices {
		if d.Serial == "" {
			return nil, errors.New("Meraki assigned device has no serial")
		}
		device := deviceSnapshot{
			serial:      d.Serial,
			name:        d.Name,
			model:       d.Model,
			productType: d.ProductType,
			networkID:   d.NetworkID,
			networkName: networkNames[d.NetworkID],
		}
		if previous, ok := assigned[d.Serial]; ok && previous != device {
			return nil, errors.New("conflicting Meraki assigned-device records")
		}
		assigned[d.Serial] = device
	}
	result := make([]deviceSnapshot, 0, len(assigned))
	for _, d := range assigned {
		if a, ok := statuses[d.serial]; ok && !a.ambiguous && knownAvailability(a.status) &&
			(d.networkID == "" || a.networkID == "" || d.networkID == a.networkID) {
			d.status = a.status
		}
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].serial < result[j].serial })
	return result, nil
}

func knownAvailability(status string) bool {
	switch status {
	case "online", "alerting", "offline", "dormant":
		return true
	default:
		return false
	}
}
