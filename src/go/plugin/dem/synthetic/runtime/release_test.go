// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseAuthenticatesCompletePreparation(t *testing.T) {
	release, key := releaseFixture(t)
	public := key
	data, err := pgptest.Release(release, key)
	require.NoError(t, err)
	verified, err := VerifyRelease(data, testTrust(t, public))
	require.NoError(t, err)
	assert.Equal(t, release, verified.Release())
	// The accessor returns a copy: changing it cannot change what is installed.
	changed := verified.Release()
	changed.Manifest.Bindings[0].Expected[0] = '!'
	changed.Manifest.Bindings[1].Offset++
	assert.Equal(t, release, verified.Release())
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
	for _, invalid := range [][]byte{
		append(bytes.Clone(data), []byte(" {}")...),
		[]byte("{}"),
		bytes.Repeat([]byte(" "), ReleaseEnvelopeLimit+1),
		// The unsigned envelope is as strict as the signed payload.
		bytes.Replace(data, []byte(`"format":1`), []byte(`"format":1,"download_override":"https://untrusted.invalid"`), 1),
	} {
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
	scenarios := map[string]struct {
		change func(*Release)
		update bool
		err    string
	}{
		"same release":                      {},
		"new publication":                   {change: func(r *Release) { r.Sequence++ }, update: true},
		"replayed older release":            {change: func(r *Release) { r.Sequence-- }, err: "regress"},
		"reused sequence":                   {change: func(r *Release) { r.Manifest.Version += ".changed" }, err: "reused"},
		"changed binding with same archive": {change: func(r *Release) { r.Manifest.Bindings[0].Offset++ }, err: "reused"},
		"other architecture":                {change: func(r *Release) { r.Manifest.Platform = "linux/amd64" }, err: "platform differs"},
		"missing publication order":         {change: func(r *Release) { r.Sequence = 0 }, err: "sequence"},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(current)
			require.NoError(t, err)
			var next Release
			require.NoError(t, json.Unmarshal(data, &next))
			if scenario.change != nil {
				scenario.change(&next)
			}
			update, err := releaseUpdate(current, next)
			if scenario.err != "" {
				assert.ErrorContains(t, err, scenario.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, scenario.update, update)
		})
	}
}

func TestReleaseAcquisitionAuthenticatesRemoteAndOffline(t *testing.T) {
	release, key := releaseFixture(t)
	keys := testTrust(t, key)
	signed, err := pgptest.Release(release, key)
	require.NoError(t, err)
	var calls atomic.Int32
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write(signed)
	})
	got, err := FetchRelease(context.Background(), server.URL, keys)
	require.NoError(t, err)
	require.Equal(t, release, got.Release())
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, signed, 0600))
	got, err = ReadRelease(path, keys)
	require.NoError(t, err)
	require.Equal(t, release, got.Release())
	_, err = FetchRelease(context.Background(), server.URL, nil)
	require.ErrorContains(t, err, "no runtime release verification key")
	require.Equal(t, int32(1), calls.Load(), "missing trust must fail before network access")
	wrong := pgptest.Key(t, 9)
	_, err = ReadRelease(path, testTrust(t, wrong))
	require.ErrorContains(t, err, "not signed by a trusted key")
}

func TestReleaseAcquisitionBoundsAndTransport(t *testing.T) {
	_, key := releaseFixture(t)
	keys := testTrust(t, key)
	for _, address := range []string{"http://example.invalid/release", "https://user:secret@example.invalid/release", "https://example.invalid/release#fragment"} {
		_, err := FetchRelease(context.Background(), address, keys)
		require.ErrorContains(t, err, "requires HTTPS")
		require.NotContains(t, err.Error(), "secret")
	}
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "http://example.invalid/release", http.StatusFound)
		case "/failure":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = io.WriteString(w, strings.Repeat(" ", ReleaseEnvelopeLimit+1))
		}
	})
	_, err := FetchRelease(context.Background(), server.URL, keys)
	require.ErrorContains(t, err, "exceeds protocol size limit")
	_, err = FetchRelease(context.Background(), server.URL+"/redirect", keys)
	require.ErrorIs(t, err, errDownloadRedirect)
	_, err = FetchRelease(context.Background(), server.URL+"/failure", keys)
	require.ErrorContains(t, err, "HTTP 503")
	path := filepath.Join(t.TempDir(), "oversized.json")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{' '}, ReleaseEnvelopeLimit+1), 0600))
	_, err = ReadRelease(path, keys)
	require.ErrorContains(t, err, "exceeds protocol size limit")
}
