// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
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

// ReleaseSigningPayload returns the exact domain-separated bytes signed by GPG.
func ReleaseSigningPayload(release Release) ([]byte, error) {
	if err := release.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(release)
	if err != nil {
		return nil, err
	}
	return releaseSignedBytes(payload), nil
}

func releaseSignedBytes(payload []byte) []byte {
	return append([]byte(releaseSignatureDomain), payload...)
}

// EncodeRelease wraps the original payload and binary detached OpenPGP signature.
// The caller must verify the resulting descriptor before publishing it.
func EncodeRelease(signedPayload, signature []byte) ([]byte, error) {
	if !bytes.HasPrefix(signedPayload, []byte(releaseSignatureDomain)) {
		return nil, errors.New("invalid release signing domain")
	}
	envelope, err := json.Marshal(releaseEnvelope{Format: 1, Payload: signedPayload[len(releaseSignatureDomain):], Signature: signature})
	if err == nil && len(envelope) > ReleaseEnvelopeLimit {
		return nil, errors.New("runtime release descriptor exceeds protocol size limit")
	}
	return envelope, err
}

// VerifyRelease authenticates every preparation field and checks current signer validity.
func VerifyRelease(data []byte, trusted *ReleaseTrust) (Release, error) {
	var result Release
	if len(data) > ReleaseEnvelopeLimit {
		return result, errors.New("runtime release descriptor exceeds protocol size limit")
	}
	var envelope releaseEnvelope
	if err := decodeReleaseJSON(data, &envelope); err != nil {
		return result, fmt.Errorf("runtime release envelope: %w", err)
	}
	if envelope.Format != 1 {
		return result, errors.New("invalid runtime release envelope")
	}
	if err := verifyReleaseSignature(releaseSignedBytes(envelope.Payload), envelope.Signature, trusted); err != nil {
		return result, err
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

// releaseUpdate checks normal latest-based updates, including explicitly
// requested latest checks. Choosing an older authenticated offline release is a
// separate administrator recovery operation. Compare with selected state only;
// this is not a historical high-water mark or proof of newest availability.
func releaseUpdate(current, next Release) (bool, error) {
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

// FetchRelease authenticates bounded metadata before any archive acquisition.
// Trust is installed independently; a downloaded descriptor cannot supply keys.
func FetchRelease(ctx context.Context, address string, trusted *ReleaseTrust) (Release, error) {
	if err := validateReleaseTrust(trusted); err != nil {
		return Release{}, err
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return Release{}, errors.New("runtime release discovery requires HTTPS without credentials or a fragment")
	}
	body, err := openDownload(ctx, address, downloadIdleTimeout)
	if err != nil {
		return Release{}, err
	}
	defer body.Close()
	return readRelease(body, trusted)
}

// ReadRelease uses the same publisher trust for explicit offline provisioning.
func ReadRelease(path string, trusted *ReleaseTrust) (Release, error) {
	if err := validateReleaseTrust(trusted); err != nil {
		return Release{}, err
	}
	if !filepath.IsAbs(path) {
		return Release{}, errors.New("runtime release descriptor requires an absolute path")
	}
	file, err := safefile.Open(path)
	if err != nil {
		return Release{}, err
	}
	defer file.Close()
	return readRelease(file, trusted)
}

func readRelease(reader io.Reader, trusted *ReleaseTrust) (Release, error) {
	data, err := io.ReadAll(io.LimitReader(reader, ReleaseEnvelopeLimit+1))
	if err != nil {
		return Release{}, err
	}
	return VerifyRelease(data, trusted)
}
