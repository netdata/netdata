// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "context"

// collect reads the four organization lists completely and joins them, then
// reads the enabled telemetry, and only then writes metrics. A failed list or an
// invalid inventory fails the whole cycle, so a partial read never looks like
// removed devices; a failed optional read only leaves gaps in its charts.
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
	networkNames, err := networkNamesByID(c.OrganizationID, networks)
	if err != nil {
		return err
	}
	inventory, err := buildInventory(networkNames, devices, availabilities)
	if err != nil {
		return err
	}
	telemetry := c.collectTelemetry(ctx, inventory, networkNames)

	if err := ctx.Err(); err != nil {
		return err
	}
	c.writeMetrics(org.Name, inventory, telemetry)
	return ctx.Err()
}
