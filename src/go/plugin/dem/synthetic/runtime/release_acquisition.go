// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"io"
	"net/url"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

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
