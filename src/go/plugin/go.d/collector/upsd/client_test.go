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

func TestUpsdClient_commandErrorsDoNotLeakCredentials(t *testing.T) {
	tests := map[string]struct {
		command        string
		responses      map[string][]string
		wantUpsdError  bool
		wantContains   string
		wantNotContain string
	}{
		"username protocol error is redacted": {
			command: fmt.Sprintf(commandUsername, testUsername),
			responses: map[string][]string{
				"USERNAME": {"ERR ACCESS-DENIED"},
			},
			wantUpsdError:  true,
			wantContains:   "USERNAME",
			wantNotContain: testUsername,
		},
		"password protocol error is redacted": {
			command: fmt.Sprintf(commandPassword, testPassword),
			responses: map[string][]string{
				"PASSWORD": {"ERR ACCESS-DENIED"},
			},
			wantUpsdError:  true,
			wantContains:   "PASSWORD",
			wantNotContain: testPassword,
		},
		"username empty response is redacted": {
			command:        fmt.Sprintf(commandUsername, testUsername),
			responses:      map[string][]string{},
			wantContains:   "USERNAME",
			wantNotContain: testUsername,
		},
		"password empty response is redacted": {
			command:        fmt.Sprintf(commandPassword, testPassword),
			responses:      map[string][]string{},
			wantContains:   "PASSWORD",
			wantNotContain: testPassword,
		},
		"list ups protocol error is unchanged": {
			command: commandListUPS,
			responses: map[string][]string{
				"LIST": {"ERR UNKNOWN-COMMAND"},
			},
			wantUpsdError: true,
			wantContains:  commandListUPS,
		},
		"list var protocol error keeps ups name": {
			command: fmt.Sprintf(commandListVar, "ups1"),
			responses: map[string][]string{
				"LIST": {"ERR VAR-NOT-SUPPORTED"},
			},
			wantUpsdError: true,
			wantContains:  "LIST VAR ups1",
		},
		"list ups empty response is unchanged": {
			command:      commandListUPS,
			responses:    map[string][]string{},
			wantContains: commandListUPS,
		},
		"logout empty response is unchanged": {
			command:      commandLogout,
			responses:    map[string][]string{},
			wantContains: commandLogout,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := &upsdClient{conn: &fakeSocket{responses: tt.responses}}

			_, err := client.sendCommand(tt.command)

			require.Error(t, err)
			// An empty response means the peer closed the connection: the
			// caller must drop it, so it must not be an upsd command error.
			assert.Equal(t, tt.wantUpsdError, errors.Is(err, errUpsdCommand))
			assert.Contains(t, err.Error(), tt.wantContains)

			if tt.wantNotContain != "" {
				assert.NotContains(t, err.Error(), tt.wantNotContain)
			}
		})
	}
}
