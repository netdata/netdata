// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

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

func TestReleaseAuthenticationDoesNotRequireExecutionCompatibility(t *testing.T) {
	release, key := releaseFixture(t)
	release.Manifest.Contract = 1
	data, err := pgptest.Release(release, key)
	require.NoError(t, err)
	verified, err := VerifyRelease(data, testTrust(t, key))
	require.NoError(t, err, "authentication must permit inspection of another Agent's release")
	require.Equal(t, release, verified.Release())
	require.ErrorContains(t, verified.Release().Manifest.Validate(), "unsupported runtime execution contract 1")
	require.ErrorIs(t, verified.Release().Manifest.Validate(), ErrIncompatible)
}

func TestIncompatibleManifestStillRequiresValidMetadata(t *testing.T) {
	release, _ := releaseFixture(t)
	for _, contract := range []int{-1, 0, 1, 3} {
		m := release.Manifest.clone()
		m.Contract = contract
		if contract > 0 {
			require.NoError(t, m.ValidateMetadata())
			require.ErrorIs(t, m.Validate(), ErrIncompatible)
		} else {
			require.Error(t, m.ValidateMetadata())
			require.NotErrorIs(t, m.Validate(), ErrIncompatible)
		}
		m.Layout.Node = "../outside"
		require.Error(t, m.ValidateMetadata())
		require.NotErrorIs(t, m.Validate(), ErrIncompatible, "malformed metadata is not a healthy mismatch")
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

func TestExplicitLegacyProvisioningDoesNotAdvanceBySequence(t *testing.T) {
	current, _ := releaseFixture(t)
	scenarios := map[string]struct {
		change func(*Release)
		update bool
		err    string
	}{
		"same release":          {},
		"different metadata":    {change: func(r *Release) { r.Sequence++ }, update: true},
		"explicit older target": {change: func(r *Release) { r.Sequence-- }, update: true},
		"reused sequence": {
			change: func(r *Release) { r.Manifest.Version += ".changed" },
			update: true,
		},
		"changed binding with same archive": {
			change: func(r *Release) { r.Manifest.Bindings[0].Offset++ },
			update: true,
		},
		"other architecture": {
			change: func(r *Release) { r.Manifest.Platform = "linux/amd64" },
			err:    "platform differs",
		},
		"missing publication order": {change: func(r *Release) { r.Sequence = 0 }, err: "sequence"},
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
			update, err := releaseChanged(current, next)
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

func pairedReleaseFixture(t *testing.T) Release {
	t.Helper()
	release, _ := releaseFixture(t)
	agent := testAgentRelease()
	release.Format, release.Sequence, release.Agent = 2, 0, &agent
	release.Manifest.Version = agent.Version
	return release
}

func TestPairedReleaseRequiresExactSignedAgent(t *testing.T) {
	paired := pairedReleaseFixture(t)
	expected := *paired.Agent
	_, key := releaseFixture(t)
	for name, change := range map[string]func(*Release){
		"matching": func(*Release) {},
		"wrong version": func(r *Release) {
			r.Agent.Version = "v2.11.0"
			r.Manifest.Version = r.Agent.Version
		},
		"wrong channel": func(r *Release) { r.Agent.Channel = "stable" },
		"wrong source":  func(r *Release) { r.Agent.Source = strings.Repeat("b", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			release := pairedReleaseFixture(t)
			change(&release)
			data, err := pgptest.Release(release, key)
			require.NoError(t, err)
			verified, err := VerifyRelease(data, testTrust(t, key))
			require.NoError(t, err, "the signer is trusted even when the release names another pair")
			err = verified.Release().ValidateForAgent(expected)
			if name == "matching" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrIncompatible)
				require.ErrorContains(t, err, "matching Agent/runtime pair")
			}
		})
	}
	legacy, _ := releaseFixture(t)
	require.NoError(t, legacy.ValidateMetadata())
	require.ErrorIs(t, legacy.ValidateForAgent(expected), ErrIncompatible)
	invalid := expected
	invalid.Source = "master"
	err := paired.ValidateForAgent(invalid)
	require.ErrorContains(t, err, "expected Agent release")
	require.NotErrorIs(t, err, ErrIncompatible)
	paired.Manifest.Contract = 1
	require.NoError(t, paired.ValidateMetadata())
	require.ErrorIs(t, paired.ValidateForAgent(expected), ErrIncompatible)
}

func TestPairedReleaseAuthenticationAndCloning(t *testing.T) {
	release := pairedReleaseFixture(t)
	_, key := releaseFixture(t)
	data, err := pgptest.Release(release, key)
	require.NoError(t, err)
	verified, err := VerifyRelease(data, testTrust(t, key))
	require.NoError(t, err)
	changed := verified.Release()
	changed.Agent.Version = "different"
	changed.Agent.Channel = "devel"
	changed.Agent.Source = strings.Repeat("b", 40)
	changed.Manifest.Bindings[0].Expected[0] = '!'
	require.Equal(t, release, verified.Release())
	for _, field := range []string{"version", "channel", "source"} {
		t.Run(field, func(t *testing.T) {
			var envelope releaseEnvelope
			require.NoError(t, json.Unmarshal(data, &envelope))
			var payload map[string]any
			require.NoError(t, json.Unmarshal(envelope.Payload, &payload))
			payload["agent"].(map[string]any)[field] = "changed"
			envelope.Payload, err = json.Marshal(payload)
			require.NoError(t, err)
			tampered, err := json.Marshal(envelope)
			require.NoError(t, err)
			_, err = VerifyRelease(tampered, testTrust(t, key))
			require.ErrorContains(t, err, "not signed by a trusted key")
		})
	}
}

func TestReleaseFormatConstraints(t *testing.T) {
	_, key := releaseFixture(t)
	for name, change := range map[string]func(*Release){
		"legacy Agent tuple": func(r *Release) { r.Format, r.Sequence = 1, 1 },
		"paired sequence":    func(r *Release) { r.Sequence = 1 },
		"missing Agent":      func(r *Release) { r.Agent = nil },
		"invalid Agent":      func(r *Release) { r.Agent.Source = "master" },
		"different version":  func(r *Release) { r.Manifest.Version += ".changed" },
		"unknown format":     func(r *Release) { r.Format = 3 },
	} {
		t.Run(name, func(t *testing.T) {
			release := pairedReleaseFixture(t)
			change(&release)
			require.Error(t, release.ValidateMetadata())
			_, err := ReleaseSigningPayload(release)
			require.Error(t, err)
			// An authentic signer does not bypass structural format constraints.
			payload, err := json.Marshal(release)
			require.NoError(t, err)
			data, err := json.Marshal(releaseEnvelope{
				Format:    1,
				Payload:   payload,
				Signature: pgptest.Sign(t, key, releaseSignedBytes(payload)),
			})
			require.NoError(t, err)
			_, err = VerifyRelease(data, testTrust(t, key))
			require.Error(t, err)
		})
	}
	legacy, _ := releaseFixture(t)
	legacy.Sequence = 0
	require.Error(t, legacy.ValidateMetadata())
	paired := pairedReleaseFixture(t)
	data, err := json.Marshal(paired)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"sequence"`)
	require.Contains(t, string(data), `"agent"`)
	legacy.Sequence = 1
	data, err = json.Marshal(legacy)
	require.NoError(t, err)
	require.Contains(t, string(data), `"sequence":1`)
	require.NotContains(t, string(data), `"agent"`)
	// New pair data is outside Manifest, preserving older generation readers.
	data, err = json.Marshal(paired.Manifest)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"agent"`)
}

func TestPairedReleaseUpdatesUseExactIdentityRatherThanSequence(t *testing.T) {
	current := pairedReleaseFixture(t)
	for name, change := range map[string]func(*Release){
		"same pair": func(*Release) {},
		"next release": func(r *Release) {
			r.Agent.Version = "v2.13.0"
			r.Manifest.Version = r.Agent.Version
		},
		"explicit earlier target": func(r *Release) {
			r.Agent.Version = "v2.11.0"
			r.Manifest.Version = r.Agent.Version
		},
		"other channel": func(r *Release) { r.Agent.Channel = "stable" },
		"other source":  func(r *Release) { r.Agent.Source = strings.Repeat("b", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			next := pairedReleaseFixture(t)
			change(&next)
			update, err := releaseChanged(current, next)
			require.NoError(t, err)
			require.Equal(t, name != "same pair", update)
		})
	}
	for name, change := range map[string]func(*Release){
		"archive": func(r *Release) { r.Manifest.Archive.SHA256 = strings.Repeat("b", 64) },
		"layout":  func(r *Release) { r.Manifest.Layout.EntryPoint = "runner/other.mjs" },
		"sources": func(r *Release) { r.Sources.URL += ".changed" },
	} {
		t.Run("same pair different "+name, func(t *testing.T) {
			next := pairedReleaseFixture(t)
			change(&next)
			update, err := releaseChanged(current, next)
			require.ErrorContains(t, err, "identity was reused")
			require.False(t, update)
		})
	}
	legacy, _ := releaseFixture(t)
	legacy.Sequence = ^uint64(0)
	update, err := releaseChanged(legacy, current)
	require.NoError(t, err, "legacy sequence does not constrain the exact paired target")
	require.True(t, update)
	update, err = releaseChanged(current, legacy)
	require.ErrorIs(t, err, ErrIncompatible)
	require.False(t, update)
}
