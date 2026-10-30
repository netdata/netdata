// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/url"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// FetchRelease authenticates bounded metadata before any archive acquisition.
// Trust is installed independently; a downloaded descriptor cannot supply keys.
func FetchRelease(ctx context.Context, address string, trusted []ed25519.PublicKey) (Release, error) {
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
func ReadRelease(path string, trusted []ed25519.PublicKey) (Release, error) {
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

func readRelease(reader io.Reader, trusted []ed25519.PublicKey) (Release, error) {
	data, err := io.ReadAll(io.LimitReader(reader, ReleaseEnvelopeLimit+1))
	if err != nil {
		return Release{}, err
	}
	return VerifyRelease(data, trusted)
}

func validateReleaseTrust(trusted []ed25519.PublicKey) error {
	if len(trusted) == 0 {
		return errors.New("no runtime release verification key is configured")
	}
	for _, key := range trusted {
		if len(key) != ed25519.PublicKeySize {
			return errors.New("invalid configured runtime release verification key")
		}
	}
	return nil
}

// ReadReleaseKey loads one administrator-installed PKIX Ed25519 public PEM.
func ReadReleaseKey(path string) (ed25519.PublicKey, error) {
	data, err := safefile.Read(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 ||
		!bytes.HasPrefix(bytes.TrimSpace(data), []byte("-----BEGIN PUBLIC KEY-----")) {
		return nil, errors.New("expected one unencrypted PUBLIC KEY PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid PKIX public key")
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("verification key must be Ed25519")
	}
	return key, nil
}
