// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"maps"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// DeviceMetadataSource acquires profile metadata using caller-owned SNMP state.
type DeviceMetadataSource interface {
	CollectDeviceMetadata() (map[string]MetaTag, error)
}

// DeviceIdentityOptions supplies the address, identity overrides, and label policy.
type DeviceIdentityOptions struct {
	Address  string
	GUID     string
	Hostname string
	// BaseLabels supplies policy defaults before system and profile metadata.
	BaseLabels map[string]string
	// Labels supplies final overrides, including explicitly empty values.
	Labels map[string]string
}

// DeviceIdentity is an acquired device identity independent of vnode publication.
type DeviceIdentity struct {
	GUID     string
	Hostname string
	Labels   map[string]string
}

// AcquireDeviceIdentity acquires metadata once and combines it with system identity.
// A nil source uses system identity alone. The caller owns transport, profile
// selection, retries, and publication. Inputs are borrowed and are not modified.
func AcquireDeviceIdentity(
	si *snmputils.SysInfo,
	source DeviceMetadataSource,
	opts DeviceIdentityOptions,
) (*DeviceIdentity, error) {
	if si == nil {
		return nil, errors.New("SNMP system identity is required")
	}
	var metadata map[string]MetaTag
	if source != nil {
		var err error
		metadata, err = source.CollectDeviceMetadata()
		if err != nil {
			return nil, err
		}
	}

	guid := opts.GUID
	if guid == "" {
		guid = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(opts.Address)).String()
	}
	hostname := opts.Hostname
	if hostname == "" {
		hostname = si.Name
	}
	if hostname == "" {
		hostname = "snmp-device"
	}

	labels := make(map[string]string, len(opts.BaseLabels)+11)
	maps.Copy(labels, opts.BaseLabels)
	labels["_vnode_type"] = "snmp"
	labels["_net_default_iface_ip"] = opts.Address
	labels["address"] = opts.Address
	labels["sys_object_id"] = si.SysObjectID
	labels["name"] = si.Name
	labels["description"] = si.Descr
	labels["contact"] = si.Contact
	labels["location"] = si.Location
	if si.Vendor != "" {
		labels["vendor"] = si.Vendor
	} else if si.Organization != "" {
		labels["vendor"] = si.Organization
	}
	if si.Category != "" {
		labels["type"] = si.Category
	}
	if si.Model != "" {
		labels["model"] = si.Model
	}

	return &DeviceIdentity{
		GUID:     guid,
		Hostname: hostname,
		Labels:   ResolveDeviceMetadata(labels, metadata, opts.Labels),
	}, nil
}
