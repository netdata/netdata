// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"errors"
)

func (c *Collector) collect(ctx context.Context) error {
	organizations, err := c.client.ListOrganizations(ctx)
	if err != nil {
		return err
	}
	var organizationName string
	found := false
	for _, org := range organizations {
		if org.ID == c.OrganizationID {
			if found && organizationName != org.Name {
				return errors.New("conflicting Meraki organization records")
			}
			organizationName, found = org.Name, true
		}
	}
	if !found {
		return errors.New("configured organization_id is not visible to the Meraki credential")
	}
	networks, err := c.client.ListNetworks(ctx, c.OrganizationID)
	if err != nil {
		return err
	}
	devices, err := c.client.ListDevices(ctx, c.OrganizationID)
	if err != nil {
		return err
	}
	availability, err := c.client.ListDeviceAvailabilities(ctx, c.OrganizationID)
	if err != nil {
		return err
	}
	snapshot, err := normalize(networks, devices, availability, c.OrganizationID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.writeMetrics(organizationName, snapshot)
	return ctx.Err()
}
