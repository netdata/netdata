// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"testing"

	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/require"
)

func TestCancelledTLSPreparationHonorsCaller(t *testing.T) {
	c := New(rumregistry.New())
	c.TLSCert, c.TLSKey = "synthetic-certificate.pem", "synthetic-key.pem"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Init(ctx), context.Canceled)
}
