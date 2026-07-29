// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/require"
)

func TestMaterializationIdentityStructurallyEncodesValues(t *testing.T) {
	require.NotEqual(
		t,
		materializationIdentity("userconfig", []byte("fixture"), []byte("a\x00b"), []byte("c")),
		materializationIdentity("userconfig", []byte("fixture"), []byte("a"), []byte("b\x00c")),
	)
}

func TestQuarantinedMaterializationIsAConfigLocalFailure(t *testing.T) {
	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptServiceDiscovery,
		Key:       "pipeline",
		Resource:  "service discovery configuration",
	}
	err := classifyMaterializationError(jobmgr.ErrProcessAttemptQuarantined, identity)

	var contained *materializationError
	require.True(t, errors.As(err, &contained))
	require.Equal(t, identity, contained.identity)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.Equal(t, 503, contained.DyncfgCode())
}
