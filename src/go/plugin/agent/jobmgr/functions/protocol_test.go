// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestFunctionPublicationFrame(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	port, err := NewFramePublicationPort(7, owner)
	require.NoError(t, err)
	record := PublicationRecord{
		Name: "module:method", Generation: 3, Timeout: 60,
		Help: "method help", Tags: "top", Access: "0x0013",
		Priority: 100, Version: 3,
	}
	handle, err := port.Publish(record)
	require.NoError(t, err)
	require.EqualValues(t, PublicationHandle{
		ID: 1, Epoch: 7, Generation: 3, Name: "module:method",
	}, handle)

	require.NoError(t, port.Withdraw(handle))

	want := "" +
		"FUNCTION GLOBAL \"module:method\" 60 \"method help\" \"top\" 0x0013 100 3\n\n" +
		"FUNCTION_DEL GLOBAL \"module:method\"\n\n"
	require.EqualValues(t, want, output.String())

	require.Error(t, port.Withdraw(handle))
}

func TestFunctionWithdrawalUsesRegistrationNameBytes(t *testing.T) {
	name := "module:\u0085method"
	registration, err := encodeFunctionRegistration(PublicationRecord{
		Name: name, Generation: 1, Timeout: 1,
		Help: "help", Tags: "top", Access: "0x0000",
	})
	require.NoError(t, err)
	withdrawal, err := encodeFunctionWithdrawal(name)
	require.NoError(t, err)

	require.Contains(t, string(registration), `"`+name+`"`)
	require.Equal(t, "FUNCTION_DEL GLOBAL \""+name+"\"\n\n", string(withdrawal))
}

func TestFunctionPublicationFrameRejectsInjection(t *testing.T) {
	tests := map[string]struct {
		mutate func(*PublicationRecord)
	}{
		"quote in name": {
			mutate: func(record *PublicationRecord) { record.Name = `module:"method` },
		},
		"space in name": {
			mutate: func(record *PublicationRecord) { record.Name = "module: method" },
		},
		"backslash in name": {
			mutate: func(record *PublicationRecord) { record.Name = `module:\method` },
		},
		"newline in help": {
			mutate: func(record *PublicationRecord) { record.Help = "help\nFUNCTION_DEL" },
		},
		"nul in tags": {
			mutate: func(record *PublicationRecord) { record.Tags = "top\x00hidden" },
		},
		"uppercase access": {
			mutate: func(record *PublicationRecord) { record.Access = "0x00AF" },
		},
		"wide access": {
			mutate: func(record *PublicationRecord) { record.Access = "0x00000" },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			owner, err := lifecycle.NewFrameOwner(&output)
			require.NoError(t, err)
			port, err := NewFramePublicationPort(1, owner)
			require.NoError(t, err)
			record := PublicationRecord{
				Name: "module:method", Generation: 1, Timeout: 1,
				Help: "help", Tags: "top", Access: "0x0000",
			}
			test.mutate(&record)

			_, publishErr := port.Publish(record)
			require.Error(t, publishErr)

			require.EqualValues(t, 0, output.Len())
		})
	}
}

func TestFunctionPublicationFrameNoHandleBeforeCommit(t *testing.T) {
	writer := &failingFunctionFrameWriter{err: errors.New("write failed")}
	owner, err := lifecycle.NewFrameOwner(writer)
	require.NoError(t, err)
	port, err := NewFramePublicationPort(1, owner)
	require.NoError(t, err)
	record := PublicationRecord{
		Name: "module:method", Generation: 1, Timeout: 1,
		Help: "help", Tags: "top", Access: "0x0000",
	}
	handle, err := port.Publish(record)
	require.False(t, err == nil || handle != (PublicationHandle{}))

	census := owner.Census()
	require.False(t, !census.Poisoned || census.Commits != 0)
}

type failingFunctionFrameWriter struct {
	err error
}

func (fffw *failingFunctionFrameWriter) Write(_ []byte) (int, error) {
	return 0, fffw.err
}
