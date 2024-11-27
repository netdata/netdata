// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"errors"
)

func (c *Collector) verifyConfig() error {
	if c.URI == "" {
		return errors.New("connection URI is empty")
	}

	return nil
}

func (c *Collector) initDatabaseSelector() error {
	if c.Databases.Empty() {
		return nil
	}

	sr, err := c.Databases.Parse()
	if err != nil {
		return err
	}
	c.dbSelector = sr

	return nil
}
