// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUsername = "s3cr3t-user"
	testPassword = "s3cr3t-password"
)

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

	err := client.authenticate(testUsername, testPassword)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUpsdCommand))
	assert.NotContains(t, err.Error(), testPassword)
	assert.Contains(t, err.Error(), "PASSWORD")
}

func TestUpsdClient_authenticateDoesNotLeakUsername(t *testing.T) {
	client := &upsdClient{conn: &fakeSocket{responses: map[string][]string{
		"USERNAME": {"ERR ACCESS-DENIED"},
	}}}

	err := client.authenticate(testUsername, testPassword)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUpsdCommand))
	assert.NotContains(t, err.Error(), testUsername)
	assert.Contains(t, err.Error(), "USERNAME")
}

func TestUpsdClient_errorKeepsNonCredentialCommandArguments(t *testing.T) {
	client := &upsdClient{conn: &fakeSocket{responses: map[string][]string{
		"LIST": {"ERR VAR-NOT-SUPPORTED"},
	}}}

	_, err := client.sendCommand(fmt.Sprintf(commandListVar, "ups1"))

	require.Error(t, err)
	// Only USERNAME/PASSWORD arguments are redacted: the UPS name is what
	// makes this error diagnosable.
	assert.Contains(t, err.Error(), "LIST VAR ups1")
}

func TestUpsdClient_emptyResponseIsAnError(t *testing.T) {
	client := &upsdClient{conn: &fakeSocket{responses: map[string][]string{}}}

	_, err := client.sendCommand(commandListUPS)

	require.Error(t, err)
	// The peer closed the connection; the caller must drop it, so this must
	// not be reported as a protocol-level upsd command error.
	assert.False(t, errors.Is(err, errUpsdCommand))
	assert.Contains(t, err.Error(), commandListUPS)
}
