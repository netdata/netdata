// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"errors"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPassword = "s3cr3t-password"

// fakeSocket replays a canned response per command verb.
type fakeSocket struct {
	responses map[string][]string
}

func (f *fakeSocket) Connect() error    { return nil }
func (f *fakeSocket) Disconnect() error { return nil }

func (f *fakeSocket) Command(command string, process socket.Processor) error {
	verb, _, _ := strings.Cut(strings.TrimSuffix(command, "\n"), " ")
	for _, line := range f.responses[verb] {
		if more, err := process([]byte(line)); err != nil || !more {
			return err
		}
	}
	return nil
}

func TestUpsdClient_authenticateDoesNotLeakPassword(t *testing.T) {
	client := &upsdClient{conn: &fakeSocket{responses: map[string][]string{
		"USERNAME": {"OK"},
		"PASSWORD": {"ERR ACCESS-DENIED"},
	}}}

	err := client.authenticate("user", testPassword)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUpsdCommand))
	assert.NotContains(t, err.Error(), testPassword)
	assert.Contains(t, err.Error(), "PASSWORD")
}

func TestUpsdClient_emptyResponseIsAnError(t *testing.T) {
	client := &upsdClient{conn: &fakeSocket{responses: map[string][]string{}}}

	_, err := client.sendCommand(commandListUPS)

	require.Error(t, err)
	// The peer closed the connection; the caller must drop it, so this must
	// not be reported as a protocol-level upsd command error.
	assert.False(t, errors.Is(err, errUpsdCommand))
}
