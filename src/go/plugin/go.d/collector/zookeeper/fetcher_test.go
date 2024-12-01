// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"

	"github.com/stretchr/testify/assert"
)

func Test_clientFetch(t *testing.T) {
	c := &zookeeperFetcher{Client: &mockSocket{rowsNumResp: 10}}

	rows, err := c.fetch("whatever\n")
	assert.NoError(t, err)
	assert.Len(t, rows, 10)

	rows, err = c.fetch("whatever\n")
	assert.NoError(t, err)
	assert.Len(t, rows, 10)
}

type mockSocket struct {
	rowsNumResp int
}

func (m *mockSocket) Connect() error {
	return nil
}

func (m *mockSocket) Disconnect() error {
	return nil
}

func (m *mockSocket) Command(command string, process socket.Processor) error {
	for i := 0; i < m.rowsNumResp; i++ {
		if _, err := process([]byte(command)); err != nil {
			return err
		}
	}
	return nil
}
