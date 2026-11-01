// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha1"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/stretchr/testify/require"
)

func TestReleaseRequiresCurrentSignerValidity(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	atPast := func() time.Time { return past }
	for _, kind := range []string{"primary-expiry", "primary-revocation", "subkey-expiry", "subkey-revocation", "signature-expiry", "future-signature"} {
		t.Run(kind, func(t *testing.T) {
			config := &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA, DefaultHash: crypto.SHA256, Time: atPast}
			if kind == "primary-expiry" {
				config.KeyLifetimeSecs = 60
			}
			key, err := openpgp.NewEntity("Test", "", "test@example.invalid", config)
			require.NoError(t, err)
			if strings.HasPrefix(kind, "subkey-") {
				config.KeyLifetimeSecs = 0
				if kind == "subkey-expiry" {
					config.KeyLifetimeSecs = 60
				}
				require.NoError(t, key.AddSigningSubkey(config))
			}
			config.KeyLifetimeSecs = 0
			if kind == "signature-expiry" {
				config.SigLifetimeSecs = 60
			}
			if kind == "future-signature" {
				config.Time = func() time.Time { return time.Now().Add(time.Hour) }
			}
			release, _ := releaseFixture(t)
			payload, err := ReleaseSigningPayload(release)
			require.NoError(t, err)
			signature := pgptest.SignWithConfig(t, key, payload, config)
			raw, err := EncodeRelease(payload, signature)
			require.NoError(t, err)
			if kind == "primary-revocation" {
				require.NoError(t, key.Revoke(packet.KeyCompromised, "test", nil))
			}
			if kind == "subkey-revocation" {
				require.NoError(t, key.Subkeys[len(key.Subkeys)-1].Revoke(packet.KeyCompromised, "test", nil))
			}
			_, err = VerifyRelease(raw, testTrust(t, key))
			require.Error(t, err)
		})
	}
}

func TestHistoricalInvalidOtherKeysDoNotRejectValidSigner(t *testing.T) {
	release, key := releaseFixture(t)
	signed, err := pgptest.Release(release, key)
	require.NoError(t, err)
	past := time.Now().Add(-time.Hour)
	expired, err := openpgp.NewEntity("Old", "", "old@example.invalid", &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA, Time: func() time.Time { return past }, KeyLifetimeSecs: 1})
	require.NoError(t, err)
	revoked, err := openpgp.NewEntity("Revoked", "", "revoked@example.invalid", &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA})
	require.NoError(t, err)
	require.NoError(t, revoked.Revoke(packet.KeyCompromised, "test", nil))
	ring := append(pgptest.Public(t, expired), pgptest.Public(t, revoked)...)
	ring = append(ring, pgptest.Public(t, key)...)
	trust, err := ParseReleaseKeyring(ring)
	require.NoError(t, err)
	got, err := VerifyRelease(signed, trust)
	require.NoError(t, err)
	require.Equal(t, release, got)
}

func TestReleaseSignaturePolicyAndPacketBounds(t *testing.T) {
	release, key := releaseFixture(t)
	payload, err := ReleaseSigningPayload(release)
	require.NoError(t, err)
	signature := pgptest.Sign(t, key, payload)
	trust := testTrust(t, key)
	headerSize := 2
	if signature[1] >= 192 && signature[1] < 224 {
		headerSize = 3
	}
	if signature[1] == 255 {
		headerSize = 6
	}
	// The library accepts this declared length with a complete but much shorter
	// signature body; release framing must reject it before invoking the parser.
	oversizedDeclaration := append([]byte{0xc2, 0xff, 0xff, 0xff, 0xff, 0xff}, signature[headerSize:]...)
	hiddenBody := append(bytes.Clone(signature[headerSize:]), 0xca, 3, 'P', 'G', 'P')
	hiddenPacket := append([]byte{0xc2, 0xff, 0, 0, byte(len(hiddenBody) >> 8), byte(len(hiddenBody))}, hiddenBody...)
	var text bytes.Buffer
	require.NoError(t, openpgp.DetachSignWithParams(&text, []*openpgp.Entity{key}, bytes.NewReader(payload), &openpgp.SignParams{TextSig: true, Config: &packet.Config{DefaultHash: crypto.SHA256}}))
	// Use the packet signer directly: the high-level signer deliberately upgrades
	// a weak requested digest to a preferred strong digest.
	weakPacket := &packet.Signature{SigType: packet.SigTypeBinary, PubKeyAlgo: key.PrimaryKey.PubKeyAlgo, Hash: crypto.SHA1, CreationTime: time.Now(), IssuerKeyId: &key.PrimaryKey.KeyId}
	weakHash := sha1.New()
	_, err = weakHash.Write(payload)
	require.NoError(t, err)
	randomize := false
	require.NoError(t, weakPacket.Sign(weakHash, key.PrivateKey, &packet.Config{NonDeterministicSignaturesViaNotation: &randomize}))
	var weakBuffer bytes.Buffer
	require.NoError(t, weakPacket.Serialize(&weakBuffer))
	weak := weakBuffer.Bytes()
	for name, sig := range map[string][]byte{
		"text": text.Bytes(), "weak": weak, "empty": nil, "oversized": bytes.Repeat([]byte{0}, ReleaseSignatureLimit+1),
		"multiple": append(bytes.Clone(signature), signature...), "trailing": append(bytes.Clone(signature), 0),
		"unknown-packet": append(bytes.Clone(signature), 0xff, 0), "truncated": signature[:len(signature)-1],
		"oversized-declaration": oversizedDeclaration, "hidden-packet": hiddenPacket,
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := EncodeRelease(payload, sig)
			require.NoError(t, err)
			_, err = VerifyRelease(raw, trust)
			require.Error(t, err)
		})
	}
	// Framing must reject length declarations shorter/longer than the signature,
	// including a valid extra packet hidden inside a larger declared packet body.
	malformed := append([]byte{0xc2, 0xff, 0, 0, 0, 0}, signature...)
	_, err = readReleasePacket(bytes.NewReader(malformed))
	require.Error(t, err)
}

func TestReleasePublicKeyringBoundsAndMaterial(t *testing.T) {
	key := pgptest.Key(t, 1)
	public := pgptest.Public(t, key)
	var armored bytes.Buffer
	writer, err := armor.Encode(&armored, openpgp.PublicKeyType, nil)
	require.NoError(t, err)
	_, err = writer.Write(public)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	_, err = ParseReleaseKeyring(armored.Bytes())
	require.NoError(t, err)
	// A valid keyring at the raw boundary remains usable.
	atLimit := append(bytes.Clone(armored.Bytes()), bytes.Repeat([]byte(" "), ReleaseKeyringLimit-armored.Len())...)
	_, err = ParseReleaseKeyring(atLimit)
	require.NoError(t, err)
	var secret bytes.Buffer
	require.NoError(t, key.SerializePrivateWithoutSigning(&secret, nil))
	for _, bad := range [][]byte{nil, []byte("not a keyring"), secret.Bytes(), append(bytes.Clone(public), 0xff, 0), append(bytes.Clone(public), pgptest.Sign(t, key, []byte("wrong-purpose"))...), append(atLimit, ' '), append(bytes.Clone(armored.Bytes()), []byte("trailing")...), append(bytes.Clone(armored.Bytes()), armored.Bytes()...)} {
		_, err = ParseReleaseKeyring(bad)
		require.Error(t, err)
	}
}

type trustTransport func(*http.Request) (*http.Response, error)

func (f trustTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCanonicalKeyFetchBoundedCanceledAndNoCacheFallback(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	calls := 0
	public := pgptest.Public(t, pgptest.Key(t, 1))
	status := http.StatusOK
	body := public
	http.DefaultTransport = trustTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, ReleaseKeyringURL, r.URL.String())
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})
	_, err := FetchReleaseKeyring(context.Background())
	require.NoError(t, err)
	status = http.StatusServiceUnavailable
	_, err = FetchReleaseKeyring(context.Background())
	require.ErrorContains(t, err, "503")
	require.Equal(t, 2, calls)
	status = http.StatusOK
	body = bytes.Repeat([]byte(" "), ReleaseKeyringLimit+1)
	_, err = FetchReleaseKeyring(context.Background())
	require.ErrorContains(t, err, "size limit")
	http.DefaultTransport = trustTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = FetchReleaseKeyring(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestEncodeReleaseRejectsWrongDomain(t *testing.T) {
	_, err := EncodeRelease([]byte(`{}`), nil)
	require.Error(t, err)
	release, key := releaseFixture(t)
	raw, err := pgptest.Release(release, key)
	require.NoError(t, err)
	var envelope releaseEnvelope
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.NotEmpty(t, envelope.Signature)
}

func TestCanonicalPublicKeyringFixture(t *testing.T) {
	path := os.Getenv("DEM_RUNTIME_TEST_CANONICAL_KEYRING")
	if path == "" {
		t.Skip("explicit canonical public keyring fixture not supplied")
	}
	trust, err := ReadReleaseKeyring(path)
	require.NoError(t, err)
	entities, err := openpgp.ReadKeyRing(bytes.NewReader(trust.binary))
	require.NoError(t, err)
	valid := false
	for _, entity := range entities {
		if _, ok := entity.SigningKey(time.Now(), nil); ok {
			valid = true
		}
	}
	require.True(t, valid, "canonical keyring must include a currently valid package signer")
}
