// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package systemdunits

import (
	"fmt"
	"regexp"
	"strconv"
)

func (c *Collector) collect() (map[string]int64, error) {
	conn, err := c.getConnection()
	if err != nil {
		return nil, err
	}

	if c.systemdVersion == 0 {
		ver, err := c.getSystemdVersion(conn)
		if err != nil {
			c.closeConnection()
			return nil, err
		}
		c.systemdVersion = ver
	}

	mx := make(map[string]int64)

	if err := c.collectUnits(mx, conn); err != nil {
		c.closeConnection()
		return nil, err
	}

	if c.CollectUnitFiles && len(c.IncludeUnitFiles) > 0 {
		if err := c.collectUnitFiles(mx, conn); err != nil {
			c.closeConnection()
			return mx, err
		}
	}

	return mx, nil
}

func (c *Collector) getConnection() (systemdConnection, error) {
	if c.conn == nil {
		conn, err := c.client.connect()
		if err != nil {
			return nil, fmt.Errorf("error on creating a connection: %v", err)
		}
		c.conn = conn
	}
	return c.conn, nil
}

func (c *Collector) closeConnection() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

var reVersion = regexp.MustCompile(`[0-9][0-9][0-9]`)

const versionProperty = "Version"

func (c *Collector) getSystemdVersion(conn systemdConnection) (int, error) {
	c.Debugf("calling function 'GetManagerProperty'")
	version, err := conn.GetManagerProperty(versionProperty)
	if err != nil {
		return 0, fmt.Errorf("error on getting '%s' manager property: %v", versionProperty, err)
	}

	c.Debugf("systemd version: %s", version)

	major := reVersion.FindString(version)
	if major == "" {
		return 0, fmt.Errorf("couldn't parse systemd version string '%s'", version)
	}

	ver, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("couldn't parse systemd version string '%s': %v", version, err)
	}

	return ver, nil
}
