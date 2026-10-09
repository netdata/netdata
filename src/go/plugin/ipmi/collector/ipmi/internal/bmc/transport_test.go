// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"reflect"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_OpenTransport(t *testing.T) {
	tr, err := New(Config{
		Device:  1,
		Timeout: time.Second,
	}).newTransport()
	require.NoError(t, err)
	open, ok := tr.(*openTransport)
	require.True(t, ok)
	assert.Equal(t, client.InterfaceOpen, open.sdk.Interface)

	// The pinned SDK has no public accessor for its backend state. Read (never
	// mutate) the field that ConnectOpen requires.
	backend := reflect.ValueOf(open.sdk).Elem().FieldByName("openipmi")
	require.True(t, backend.IsValid())
	assert.False(t, backend.IsNil(), "the local SDK client lacks OpenIPMI state")

	assert.NoError(t, open.Close(t.Context()), "closing a client that never connected")
}
