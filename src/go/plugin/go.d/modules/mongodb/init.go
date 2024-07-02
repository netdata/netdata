// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"errors"
)

func (m *Mongo) verifyConfig() error {
	if m.URI == "" {
		return errors.New("connection URI is empty")
	}

	return nil
}

func (m *Mongo) initDatabaseSelector() error {
	if m.Databases.Empty() {
		return nil
	}

	sr, err := m.Databases.Parse()
	if err != nil {
		return err
	}
	m.dbSelector = sr

	return nil
}
