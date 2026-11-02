// SPDX-License-Identifier: GPL-3.0-or-later

// Package merakiapi implements Meraki Dashboard API v1 discovery and availability
// reads. It returns provider data without inventing metric semantics.
package merakiapi

// Organization is the minimal projection needed to identify an organization.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Network describes a network visible to the authenticated identity.
type Network struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	ProductTypes   []string `json:"productTypes"`
	Tags           []string `json:"tags"`
}

// Device describes assigned inventory, excluding unused organization inventory.
type Device struct {
	Serial      string   `json:"serial"`
	NetworkID   string   `json:"networkId"`
	Name        string   `json:"name"`
	Model       string   `json:"model"`
	ProductType string   `json:"productType"`
	Firmware    string   `json:"firmware"`
	MAC         string   `json:"mac"`
	Tags        []string `json:"tags"`
}

// DeviceAvailability preserves the provider's status string, including unknown
// or absent values. Meraki refreshes this endpoint every five minutes and does
// not supply a sample timestamp.
type DeviceAvailability struct {
	Serial      string   `json:"serial"`
	Name        string   `json:"name"`
	ProductType string   `json:"productType"`
	MAC         string   `json:"mac"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	Network     struct {
		ID string `json:"id"`
	} `json:"network"`
}
