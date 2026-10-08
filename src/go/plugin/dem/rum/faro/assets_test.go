// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinnedAssets(t *testing.T) {
	var manifest struct {
		Bundles []struct {
			File   string `json:"file"`
			SHA256 string `json:"sha256"`
			Bytes  int    `json:"bytes"`
		} `json:"bundles"`
		NoticesSHA256 string `json:"notices_sha256"`
	}
	raw, err := os.ReadFile("assets/manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Len(t, manifest.Bundles, 2)
	for i, path := range []string{SDKPath, TracingPath, NoticesPath} {
		t.Run(path, func(t *testing.T) {
			asset, ok := LookupAsset(strings.TrimPrefix(path, "assets/"))
			require.True(t, ok)
			sum := sha256.Sum256([]byte(asset.Content))
			digest := hex.EncodeToString(sum[:])
			assert.Contains(t, path, digest)
			assert.Equal(t, `"`+digest+`"`, asset.ETag)
			if i < 2 {
				assert.Equal(t, manifest.Bundles[i].SHA256, digest)
				assert.Len(t, asset.Content, manifest.Bundles[i].Bytes)
				published, err := os.ReadFile("assets/" + manifest.Bundles[i].File)
				require.NoError(t, err)
				assert.Equal(t, string(published), asset.Content)
				assert.Equal(t, "application/javascript; charset=utf-8", asset.ContentType)
			} else {
				assert.Equal(t, manifest.NoticesSHA256, digest)
				assert.Equal(t, "text/plain; charset=utf-8", asset.ContentType)
			}
		})
	}
}

func TestAssetLookupRejectsUnpublishedNames(t *testing.T) {
	for _, name := range []string{"", SDKPath, "faro-web-sdk.iife.js", "NOTICES.txt", "../assets.go", "faro-web-sdk.iife.js.map", strings.Replace(strings.TrimPrefix(SDKPath, "assets/"), sdkSHA256, strings.Repeat("0", 64), 1)} {
		_, ok := LookupAsset(name)
		assert.False(t, ok, name)
	}
}
