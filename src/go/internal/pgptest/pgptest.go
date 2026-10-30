// SPDX-License-Identifier: GPL-3.0-or-later

// Package pgptest supplies throwaway OpenPGP fixtures without runtime dependencies.
package pgptest

import (
	"bytes"
	"crypto"
	"encoding/json"
	"sync"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/stretchr/testify/require"
)

var keysMu sync.Mutex
var keys = map[int]*openpgp.Entity{}

func Key(t testing.TB, id int) *openpgp.Entity {
	t.Helper()
	keysMu.Lock()
	defer keysMu.Unlock()
	if key := keys[id]; key != nil {
		return key
	}
	key, err := openpgp.NewEntity("Release test", "", "release@example.invalid", &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA})
	require.NoError(t, err)
	keys[id] = key
	return key
}

func Public(t testing.TB, key *openpgp.Entity) []byte {
	t.Helper()
	keysMu.Lock()
	defer keysMu.Unlock()
	var buf bytes.Buffer
	require.NoError(t, key.Serialize(&buf))
	return buf.Bytes()
}

func Sign(t testing.TB, key *openpgp.Entity, payload []byte) []byte {
	t.Helper()
	return SignWithConfig(t, key, payload, &packet.Config{DefaultHash: crypto.SHA256})
}

func SignWithConfig(t testing.TB, key *openpgp.Entity, payload []byte, config *packet.Config) []byte {
	t.Helper()
	keysMu.Lock()
	defer keysMu.Unlock()
	var buf bytes.Buffer
	require.NoError(t, openpgp.DetachSign(&buf, []*openpgp.Entity{key}, bytes.NewReader(payload), config))
	return buf.Bytes()
}

func Release(release any, key *openpgp.Entity) ([]byte, error) {
	payload, err := json.Marshal(release)
	if err != nil {
		return nil, err
	}
	keysMu.Lock()
	defer keysMu.Unlock()
	var signature bytes.Buffer
	err = openpgp.DetachSign(&signature, []*openpgp.Entity{key}, bytes.NewReader(append([]byte("netdata-dem-runtime-release-v1\x00"), payload...)), &packet.Config{DefaultHash: crypto.SHA256})
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Format    int    `json:"format"`
		Payload   []byte `json:"payload"`
		Signature []byte `json:"signature"`
	}{1, payload, signature.Bytes()})
}
