// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
)

// ReleaseEnvelopeLimit bounds untrusted release metadata before JSON/base64
// decoding. Descriptors contain binding slots and identities, never file bodies
// or the complete source inventory. Publishers enforce the same protocol bound.
const ReleaseEnvelopeLimit = 1 << 20

const releaseSignatureDomain = "netdata-dem-runtime-release-v1\x00"

// Release is the complete authenticated preparation contract. Sequence is
// publisher order across architectures and execution-contract changes, not a
// component version or a claim that the components are more secure.
type Release struct {
	Format   int           `json:"format"`
	Sequence uint64        `json:"sequence"`
	Manifest Manifest      `json:"manifest"`
	Sources  ReleaseSource `json:"sources"`
}

type ReleaseSource struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type releaseEnvelope struct {
	Format    int    `json:"format"`
	KeyID     string `json:"key_id"`
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

func (r Release) Validate() error {
	if r.Format != 1 || r.Sequence == 0 {
		return errors.New("invalid runtime release format or publication sequence")
	}
	if err := r.Manifest.Validate(); err != nil {
		return err
	}
	if r.Manifest.Archive.URL == "" {
		return errors.New("published runtime release requires an archive URL")
	}
	u, err := url.Parse(r.Sources.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" ||
		!validID(r.Sources.SHA256) || r.Sources.Bytes <= 0 {
		return errors.New("invalid runtime release source archive")
	}
	return nil
}

// ReleaseKeyID identifies a locally trusted public key. The envelope cannot
// introduce a key or change the administrator's trust configuration.
func ReleaseKeyID(key ed25519.PublicKey) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

func releaseSignedBytes(payload []byte) []byte {
	return append([]byte(releaseSignatureDomain), payload...)
}

// SignRelease is used by release tooling. Source publication and platform
// qualification remain publisher obligations; a signature cannot prove either.
func SignRelease(release Release, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid runtime release signing key")
	}
	if err := release.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(release)
	if err != nil {
		return nil, err
	}
	envelope, err := json.Marshal(releaseEnvelope{
		Format:    1,
		KeyID:     ReleaseKeyID(key.Public().(ed25519.PublicKey)),
		Payload:   payload,
		Signature: ed25519.Sign(key, releaseSignedBytes(payload)),
	})
	if err == nil && len(envelope) > ReleaseEnvelopeLimit {
		return nil, errors.New("runtime release descriptor exceeds protocol size limit")
	}
	return envelope, err
}

// VerifyRelease authenticates all preparation fields before exposing them to
// acquisition. It provides no freshness, expiry or automatic key-rotation claim.
func VerifyRelease(data []byte, trusted []ed25519.PublicKey) (Release, error) {
	var result Release
	if len(data) > ReleaseEnvelopeLimit {
		return result, errors.New("runtime release descriptor exceeds protocol size limit")
	}
	var envelope releaseEnvelope
	if err := decodeReleaseJSON(data, &envelope); err != nil {
		return result, fmt.Errorf("runtime release envelope: %w", err)
	}
	if envelope.Format != 1 || !validID(envelope.KeyID) || len(envelope.Signature) != ed25519.SignatureSize {
		return result, errors.New("invalid runtime release envelope")
	}
	var key ed25519.PublicKey
	for _, candidate := range trusted {
		if len(candidate) != ed25519.PublicKeySize {
			return result, errors.New("invalid configured runtime release verification key")
		}
		if ReleaseKeyID(candidate) == envelope.KeyID {
			key = candidate
		}
	}
	if key == nil || !ed25519.Verify(key, releaseSignedBytes(envelope.Payload), envelope.Signature) {
		return result, errors.New("runtime release is not signed by a trusted key")
	}
	if err := decodeReleaseJSON(envelope.Payload, &result); err != nil {
		return Release{}, fmt.Errorf("signed runtime release: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Release{}, err
	}
	return result, nil
}

func decodeReleaseJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("release metadata has trailing content")
	}
	return nil
}

// ReleaseUpdate checks normal latest-based updates, including explicitly
// requested latest checks. Choosing an older authenticated offline release is a
// separate administrator recovery operation. Compare with selected state only;
// this is not a historical high-water mark or proof of newest availability.
func ReleaseUpdate(current, next Release) (bool, error) {
	if err := current.Validate(); err != nil {
		return false, fmt.Errorf("installed runtime release: %w", err)
	}
	if err := next.Validate(); err != nil {
		return false, err
	}
	if current.Manifest.Platform != next.Manifest.Platform {
		return false, errors.New("runtime update platform differs from the installation")
	}
	if next.Sequence < current.Sequence {
		return false, errors.New("runtime latest would regress the installed publication sequence")
	}
	if next.Sequence == current.Sequence {
		before, _ := json.Marshal(current)
		after, _ := json.Marshal(next)
		if !bytes.Equal(before, after) {
			return false, errors.New("runtime release publication sequence was reused for different content")
		}
		return false, nil
	}
	return true, nil
}
