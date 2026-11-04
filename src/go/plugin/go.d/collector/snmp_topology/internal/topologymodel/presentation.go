// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"slices"
	"strings"
)

// DevicePresentation is the effective current registration metadata for an older
// acquisition. It does not change collected identity, topology facts or clocks.
type DevicePresentation struct {
	SysName         string
	SysDescr        string
	SysContact      string
	SysLocation     string
	Vendor          string
	Model           string
	SerialNumber    *string
	SoftwareVersion *string
	FirmwareVersion *string
	HardwareVersion *string
	Labels          map[string]string
}

// WithDevicePresentation borrows immutable presentation labels and observation
// facts. Only the local observation vector is copied before replacing metadata.
func WithDevicePresentation(snapshot ObservationSnapshot, presentation *DevicePresentation) ObservationSnapshot {
	if presentation == nil {
		return snapshot
	}
	local := &snapshot.LocalDevice
	local.SysName = presentation.SysName
	local.SysDescr = presentation.SysDescr
	local.SysContact = presentation.SysContact
	local.SysLocation = presentation.SysLocation
	local.Vendor = presentation.Vendor
	local.Model = presentation.Model
	local.Labels = presentation.Labels
	// Older presentation records omit derived fields; keep acquired values then.
	if presentation.SerialNumber != nil {
		local.SerialNumber = *presentation.SerialNumber
	}
	if presentation.SoftwareVersion != nil {
		local.SoftwareVersion = *presentation.SoftwareVersion
	}
	if presentation.FirmwareVersion != nil {
		local.FirmwareVersion = *presentation.FirmwareVersion
	}
	if presentation.HardwareVersion != nil {
		local.HardwareVersion = *presentation.HardwareVersion
	}
	snapshot.L2Observations = slices.Clone(snapshot.L2Observations)
	for i := range snapshot.L2Observations {
		observation := &snapshot.L2Observations[i]
		if observation.DeviceID == snapshot.LocalDeviceID && !observation.Inferred {
			observation.Hostname = strings.TrimSpace(presentation.SysName)
			observation.Labels = presentation.Labels
		}
	}
	return snapshot
}
