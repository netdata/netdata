// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "context"

// collect reads the four organization lists completely, joins them, and only
// then writes metrics. Any failed request or invalid inventory fails the whole
// cycle, so a partial read never looks like removed devices.
func (c *Collector) collect(ctx context.Context) error {
	organizations, err := c.client.ListOrganizations(ctx)
	if err != nil {
		return err
	}
	org, err := findOrganization(organizations, c.OrganizationID)
	if err != nil {
		return err
	}
	networks, err := c.client.ListNetworks(ctx, c.OrganizationID)
	if err != nil {
		return err
	}
	devices, err := c.client.ListDevices(ctx, c.OrganizationID)
	if err != nil {
		return err
	}
	availabilities, err := c.client.ListDeviceAvailabilities(ctx, c.OrganizationID)
	if err != nil {
		return err
	}
	inventory, err := buildInventory(c.OrganizationID, networks, devices, availabilities)
	if err != nil {
		return err
	}

	c.writeMetrics(org.Name, inventory)
	return nil
}
