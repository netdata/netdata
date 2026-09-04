// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratorNext(t *testing.T) {
	g := Generator{
		Prefix:  "netdata-s3check/",
		OwnerID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Now:     func() time.Time { return time.Unix(123, 456) },
		Random:  bytes.NewReader(make([]byte, 16+PayloadBytes)),
	}
	namespace, err := g.Namespace()
	require.NoError(t, err)
	assert.Equal(t, "netdata-s3check/0123456789abcdef/", namespace)
	keyPrefix, err := g.KeyPrefix()
	require.NoError(t, err)
	assert.Equal(t, namespace+"probe-", keyPrefix)

	object, err := g.Next()
	require.NoError(t, err)
	assert.Equal(
		t,
		"netdata-s3check/0123456789abcdef/probe-123000000456-00000000000000000000000000000000.bin",
		object.Key,
	)
	assert.Len(t, object.Payload, PayloadBytes)
	assert.Equal(t, Digest(object.Payload), object.Digest)
}
