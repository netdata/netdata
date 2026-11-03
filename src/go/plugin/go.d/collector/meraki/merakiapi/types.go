// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

// The response types keep only the fields the collector reads. IDs stay opaque
// strings, and missing fields decode as empty values: the provider does not mark
// them required, so callers validate what they rely on.

// Organization is an organization visible to the credential.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Network is a network of an organization.
type Network struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
}

// Device is a device assigned to a network. Unused organization inventory is
// not listed.
type Device struct {
	Serial      string `json:"serial"`
	NetworkID   string `json:"networkId"`
	Name        string `json:"name"`
	Model       string `json:"model"`
	ProductType string `json:"productType"`
}

// DeviceAvailability is the provider-reported status of a device. Status keeps
// the provider string as-is, including unknown or empty values. Meraki refreshes
// availability every five minutes and supplies no sample timestamp.
type DeviceAvailability struct {
	Serial  string     `json:"serial"`
	Status  string     `json:"status"`
	Network NetworkRef `json:"network"`
}

// NetworkRef references a network by ID.
type NetworkRef struct {
	ID string `json:"id"`
}
