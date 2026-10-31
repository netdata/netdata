// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func releaseFixture(t *testing.T) (Release, *openpgp.Entity) {
	t.Helper()
	m, _ := bundleFixture(t, "signed-release")
	m.Platform = "linux/arm64"
	m.Archive.URL = "https://artifacts.example.invalid/dem-runtime/1/arm64/" + m.Archive.SHA256 + ".tar.zst"
	return Release{
		Format:   1,
		Sequence: 23,
		Manifest: m,
		Sources: ReleaseSource{
			URL:    "https://artifacts.example.invalid/dem-runtime/1/sources.tar.zst",
			SHA256: strings.Repeat("a", 64),
			Bytes:  100,
		},
	}, pgptest.Key(t, 1)
}

func TestReleaseAuthenticatesCompletePreparation(t *testing.T) {
	release, key := releaseFixture(t)
	public := key
	data, err := pgptest.Release(release, key)
	require.NoError(t, err)
	verified, err := VerifyRelease(data, testTrust(t, public))
	require.NoError(t, err)
	assert.Equal(t, release, verified)
	for _, field := range []string{"binding", "layout", "bounds", "sequence", "sources"} {
		t.Run(field, func(t *testing.T) {
			var envelope releaseEnvelope
			require.NoError(t, json.Unmarshal(data, &envelope))
			var changed Release
			require.NoError(t, json.Unmarshal(envelope.Payload, &changed))
			switch field {
			case "binding":
				changed.Manifest.Bindings[0].Offset++
			case "layout":
				changed.Manifest.Layout.EntryPoint = "runner/other.mjs"
			case "bounds":
				changed.Manifest.Archive.Entries++
			case "sequence":
				changed.Sequence++
			case "sources":
				changed.Sources.URL += ".changed"
			}
			envelope.Payload, err = json.Marshal(changed)
			require.NoError(t, err)
			tampered, err := json.Marshal(envelope)
			require.NoError(t, err)
			_, err = VerifyRelease(tampered, testTrust(t, public))
			assert.ErrorContains(t, err, "not signed by a trusted key")
		})
	}
}

func TestReleaseVerificationRequiresLocalTrustAndBoundedMetadata(t *testing.T) {
	release, key := releaseFixture(t)
	data, err := pgptest.Release(release, key)
	require.NoError(t, err)
	other := pgptest.Key(t, 2)
	for _, keys := range []*ReleaseTrust{nil, testTrust(t, other), {}} {
		_, err := VerifyRelease(data, keys)
		assert.Error(t, err)
	}
	public := key
	for _, invalid := range [][]byte{append(bytes.Clone(data), []byte(" {}")...), []byte("{}"), bytes.Repeat([]byte(" "), ReleaseEnvelopeLimit+1)} {
		_, err := VerifyRelease(invalid, testTrust(t, public))
		assert.Error(t, err)
	}
	var envelope releaseEnvelope
	require.NoError(t, json.Unmarshal(data, &envelope))
	envelope.Payload = append(envelope.Payload[:len(envelope.Payload)-1], []byte(`,"unknown":true}`)...)
	envelope.Signature = pgptest.Sign(t, key, releaseSignedBytes(envelope.Payload))
	unknown, err := json.Marshal(envelope)
	require.NoError(t, err)
	_, err = VerifyRelease(unknown, testTrust(t, public))
	assert.ErrorContains(t, err, "unknown field")
	// A signature for the raw JSON is not a signature for this release protocol.
	require.NoError(t, json.Unmarshal(data, &envelope))
	envelope.Signature = pgptest.Sign(t, key, envelope.Payload)
	otherPurpose, err := json.Marshal(envelope)
	require.NoError(t, err)
	_, err = VerifyRelease(otherPurpose, testTrust(t, public))
	assert.ErrorContains(t, err, "not signed by a trusted key")
	// The byte bound applies before parsing, including otherwise valid whitespace.
	atLimit := append(bytes.Clone(data), bytes.Repeat([]byte(" "), ReleaseEnvelopeLimit-len(data))...)
	_, err = VerifyRelease(atLimit, testTrust(t, public))
	require.NoError(t, err)
	_, err = VerifyRelease(append(atLimit, ' '), testTrust(t, public))
	assert.ErrorContains(t, err, "size limit")
}

func TestReleaseUpdatePreservesPublicationOrder(t *testing.T) {
	current, _ := releaseFixture(t)
	for _, scenario := range []struct {
		name   string
		change func(*Release)
		update bool
		err    string
	}{
		{name: "same release"},
		{name: "new publication", change: func(r *Release) { r.Sequence++ }, update: true},
		{name: "replayed older release", change: func(r *Release) { r.Sequence-- }, err: "regress"},
		{name: "reused sequence", change: func(r *Release) { r.Manifest.Version += ".changed" }, err: "reused"},
		{name: "changed binding with same archive", change: func(r *Release) { r.Manifest.Bindings[0].Offset++ }, err: "reused"},
		{name: "other architecture", change: func(r *Release) { r.Manifest.Platform = "linux/amd64" }, err: "platform differs"},
		{name: "missing publication order", change: func(r *Release) { r.Sequence = 0 }, err: "sequence"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			data, err := json.Marshal(current)
			require.NoError(t, err)
			var next Release
			require.NoError(t, json.Unmarshal(data, &next))
			if scenario.change != nil {
				scenario.change(&next)
			}
			update, err := ReleaseUpdate(current, next)
			if scenario.err != "" {
				assert.ErrorContains(t, err, scenario.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, scenario.update, update)
		})
	}
}

func testTrust(t *testing.T, key *openpgp.Entity) *ReleaseTrust {
	t.Helper()
	trust, err := ParseReleaseKeyring(pgptest.Public(t, key))
	require.NoError(t, err)
	return trust
}
