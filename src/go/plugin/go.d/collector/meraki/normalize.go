// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"errors"
	"iter"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

// device is one assigned device of the organization, joined with its network
// name and, when trustworthy, its availability status.
type device struct {
	serial      string
	name        string
	model       string
	productType string
	networkID   string
	networkName string
	status      string // one of availabilityStates, or "" when unknown
}

// findOrganization returns the configured organization from the visible list.
func findOrganization(organizations []merakiapi.Organization, id string) (merakiapi.Organization, error) {
	var org merakiapi.Organization
	found := false
	for _, o := range organizations {
		if o.ID != id {
			continue
		}
		if found && o != org {
			return merakiapi.Organization{}, errors.New("conflicting Meraki organization records")
		}
		org, found = o, true
	}
	if !found {
		return merakiapi.Organization{}, errors.New(
			"configured organization_id is not visible to the Meraki credential",
		)
	}
	return org, nil
}

// buildInventory joins the devices with their network names and availability.
// The assigned-device list alone decides membership. Availability is attached
// only when it is a known state for the same serial, consistent across duplicate
// rows, and not reported for a different network. Otherwise the status stays
// unknown: the provider lists are not one atomic snapshot, and a status that
// cannot be trusted must not become a reported outage.
func buildInventory(
	networkNames map[string]string,
	devices []merakiapi.Device,
	availabilities []merakiapi.DeviceAvailability,
) ([]device, error) {
	statuses := statusesBySerial(availabilities)

	assigned := make(map[string]device, len(devices))
	for _, d := range devices {
		if d.Serial == "" {
			return nil, errors.New("Meraki assigned device has no serial")
		}
		dev := device{
			serial:      d.Serial,
			name:        d.Name,
			model:       d.Model,
			productType: d.ProductType,
			networkID:   d.NetworkID,
			networkName: networkNames[d.NetworkID],
		}
		if prev, ok := assigned[d.Serial]; ok && prev != dev {
			return nil, errors.New("conflicting Meraki assigned-device records")
		}
		assigned[d.Serial] = dev
	}

	inventory := make([]device, 0, len(assigned))
	for _, dev := range assigned {
		if s, ok := statuses[dev.serial]; ok && s.trusted(dev.networkID) {
			dev.status = s.status
		}
		inventory = append(inventory, dev)
	}
	return inventory, nil
}

// networkNamesByID maps the organization's network IDs to their names. A network
// of another organization, or two records of one network with different names,
// fails the cycle.
func networkNamesByID(organizationID string, networks []merakiapi.Network) (map[string]string, error) {
	names := make(map[string]string, len(networks))
	for _, n := range networks {
		if n.ID == "" {
			continue // a network without an ID cannot be joined to devices
		}
		if n.OrganizationID != "" && n.OrganizationID != organizationID {
			return nil, errors.New("Meraki network belongs to a different organization")
		}
		if name, ok := names[n.ID]; ok && name != n.Name {
			return nil, errors.New("conflicting Meraki network records")
		}
		names[n.ID] = n.Name
	}
	return names, nil
}

// availabilityStatus is the availability reported for one serial.
type availabilityStatus struct {
	status    string
	networkID string
	ambiguous bool // duplicate rows for the serial disagree
}

func statusesBySerial(availabilities []merakiapi.DeviceAvailability) map[string]availabilityStatus {
	statuses := make(map[string]availabilityStatus, len(availabilities))
	for _, a := range availabilities {
		if a.Serial == "" {
			continue
		}
		s := availabilityStatus{
			status:    a.Status,
			networkID: a.Network.ID,
		}
		if prev, ok := statuses[a.Serial]; ok && prev != s {
			s.ambiguous = true
		}
		statuses[a.Serial] = s
	}
	return statuses
}

// trusted reports whether the status can be attached to a device on networkID.
// A blank network ID on either side does not count as a mismatch.
func (s availabilityStatus) trusted(networkID string) bool {
	return !s.ambiguous && slices.Contains(availabilityStates, s.status) &&
		(networkID == "" || s.networkID == "" || networkID == s.networkID)
}

// trustedSerials reports, for each assigned device the rows mention, whether
// the rows can describe it. A device with any row naming a network other than
// its assigned one gets nothing from these rows; a blank network ID on either
// side does not count as a mismatch. Other serials are absent, so they read as
// untrusted.
func trustedSerials[R any](
	rows []R,
	devices map[string]device,
	ids func(R) (serial, networkID string),
) map[string]bool {
	trusted := make(map[string]bool)
	for _, row := range rows {
		serial, networkID := ids(row)
		dev, ok := devices[serial]
		if !ok {
			continue
		}
		if prev, seen := trusted[serial]; !seen || prev {
			trusted[serial] = dev.networkID == "" || networkID == "" || dev.networkID == networkID
		}
	}
	return trusted
}

// observable is a value that can hold nothing worth publishing.
type observable interface {
	comparable
	present() bool
}

// observations collects the values seen for each key. Identical duplicates
// collapse into one; a key whose duplicates disagree has no value, because
// summing them would double count and picking one would be a guess.
type observations[K comparable, V observable] map[K]observation[V]

type observation[V observable] struct {
	value     V
	disagreed bool
}

func (o observations[K, V]) add(key K, value V) {
	prev, seen := o[key]
	o[key] = observation[V]{
		value:     value,
		disagreed: seen && (prev.disagreed || prev.value != value),
	}
}

// agreed yields the value of every key whose observations agree and hold
// something to publish.
func (o observations[K, V]) agreed() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for key, obs := range o {
			if !obs.disagreed && obs.value.present() && !yield(key, obs.value) {
				return
			}
		}
	}
}
