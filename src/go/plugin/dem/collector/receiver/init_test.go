// SPDX-License-Identifier: GPL-3.0-or-later
package receiver

import (
	"context"
	"testing"

	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitValidatesListenPort(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:65536", "127.0.0.1:-1", "127.0.0.1:not-a-tcp-service"} {
		t.Run(listen, func(t *testing.T) {
			c := New(rumregistry.New())
			c.Listen = listen
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			require.ErrorContains(t, c.Init(context.Background()), "listen")
		})
	}
	for _, listen := range []string{"127.0.0.1:0", "127.0.0.1:65535", "127.0.0.1:http", "[::1]:19938"} {
		t.Run(listen, func(t *testing.T) {
			c := New(rumregistry.New())
			c.Listen = listen
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			require.NoError(t, c.Init(context.Background()))
		})
	}
}

func TestInitRejectsPublicURLWithoutHostname(t *testing.T) {
	c := New(rumregistry.New())
	c.PublicURL = "http://:19938"
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	require.ErrorContains(t, c.Init(context.Background()), "public_url")
}

func TestCancelledPlaintextPreparationAndRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(rumregistry.New())
	c.Listen = "127.0.0.1:0"
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	assert.ErrorIs(t, c.Init(ctx), context.Canceled)
	require.NoError(t, c.Init(context.Background()))
	ready := false
	require.ErrorIs(t, c.Run(ctx, func() { ready = true }), context.Canceled)
	assert.False(t, ready)
	assert.False(t, c.registry.Availability().Serving)
}
