// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"

	"github.com/stretchr/testify/assert"
)

var (
	testLoadStatsData, _     = os.ReadFile("testdata/load-stats.txt")
	testVersionData, _       = os.ReadFile("testdata/version.txt")
	testStatus3Data, _       = os.ReadFile("testdata/status3.txt")
	testMaxLinesExceededData = strings.Repeat(">CLIENT:ESTABLISHED,0\n", 501)
)

func TestNew(t *testing.T) { assert.IsType(t, (*Client)(nil), New(socket.Config{})) }

func TestClient_GetVersion(t *testing.T) {
	client := Client{Client: &mockSocketClient{}}
	ver, err := client.Version()
	assert.NoError(t, err)
	expected := &Version{Major: 2, Minor: 3, Patch: 4, Management: 1}
	assert.Equal(t, expected, ver)
}

func TestClient_GetLoadStats(t *testing.T) {
	client := Client{Client: &mockSocketClient{}}
	stats, err := client.LoadStats()
	assert.NoError(t, err)
	expected := &LoadStats{NumOfClients: 1, BytesIn: 7811, BytesOut: 7667}
	assert.Equal(t, expected, stats)
}

func TestClient_GetUsers(t *testing.T) {
	client := Client{
		Client: &mockSocketClient{},
	}
	users, err := client.Users()
	assert.NoError(t, err)
	expected := Users{{
		CommonName:     "pepehome",
		RealAddress:    "1.2.3.4:44347",
		VirtualAddress: "10.9.0.5",
		BytesReceived:  6043,
		BytesSent:      5661,
		ConnectedSince: 1555439465,
		Username:       "pepe",
	}}
	assert.Equal(t, expected, users)
}

func TestClient_MaxLineExceeded(t *testing.T) {
	client := Client{
		Client: &mockSocketClient{maxLineExceeded: true},
	}
	_, err := client.Users()
	assert.Error(t, err)
}

type mockSocketClient struct {
	maxLineExceeded bool
}

func (m *mockSocketClient) Connect() error { return nil }

func (m *mockSocketClient) Disconnect() error { return nil }

func (m *mockSocketClient) Command(command string, process socket.Processor) error {
	var s *bufio.Scanner

	switch command {
	default:
		return fmt.Errorf("unknown command : %s", command)
	case commandExit:
	case commandVersion:
		s = bufio.NewScanner(bytes.NewReader(testVersionData))
	case commandStatus3:
		if m.maxLineExceeded {
			s = bufio.NewScanner(strings.NewReader(testMaxLinesExceededData))
			break
		}
		s = bufio.NewScanner(bytes.NewReader(testStatus3Data))
	case commandLoadStats:
		s = bufio.NewScanner(bytes.NewReader(testLoadStatsData))
	}

	if s == nil {
		return nil
	}

	for s.Scan() {
		if _, err := process(s.Bytes()); err != nil {
			return err
		}
	}
	return nil
}
