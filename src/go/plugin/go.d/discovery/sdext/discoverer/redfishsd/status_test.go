// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStatusRoundTripAndHashInvalidation(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "status.json")
	status := newDiscoveryStatus("hash-a")
	status.Endpoints["https://192.0.2.1/redfish/v1/"] = statusEntry{
		ValidatedAt: time.Unix(1234, 0).UTC(),
	}
	require.NoError(t, saveStatus(filename, status))
	info, err := os.Stat(filename)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	loaded, err := loadStatus(filename, "hash-a")
	require.NoError(t, err)
	require.Equal(t, status.Endpoints, loaded.Endpoints)

	invalidated, err := loadStatus(filename, "hash-b")
	require.NoError(t, err)
	require.Empty(t, invalidated.Endpoints)
	require.Equal(t, "hash-b", invalidated.ConfigHash)
}

func TestMarshalStatusRejectsOversizeBeforeBuildingPayload(t *testing.T) {
	status := newDiscoveryStatus("hash")
	for index := range 2_000 {
		status.Endpoints[fmt.Sprintf("https://%04d.%s/redfish/v1/", index, string(make([]byte, 4<<10)))] = statusEntry{
			ValidatedAt: time.Unix(int64(index), 0).UTC(),
		}
	}

	payload, err := marshalStatus(status)
	require.Nil(t, payload)
	require.ErrorContains(t, err, fmt.Sprintf("exceeds %d bytes", maxStatusBytes))
}

func TestJSONStringEncodedSizeMatchesStandardEncoder(t *testing.T) {
	values := []string{
		"",
		"plain ASCII",
		"quotes \\\" and controls\n\t",
		"html <>& and separators \u2028\u2029",
		"invalid UTF-8 " + string([]byte{0xff, 0xfe}),
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, len(encoded), jsonStringEncodedSize(value), "%q", value)
	}
}

func TestStatusRejectsTrailingDataAndSymlink(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "status.json")
	require.NoError(t, os.WriteFile(filename, []byte(`{"version":1,"config_hash":"x","endpoints":{}} {}`), 0o600))
	_, err := loadStatus(filename, "x")
	require.Error(t, err)
	if runtime.GOOS == "windows" {
		return
	}

	target := filepath.Join(dir, "target.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"version":1,"config_hash":"x","endpoints":{}}`), 0o600))
	link := filepath.Join(dir, "link.json")
	require.NoError(t, os.Symlink(target, link))
	_, err = loadStatus(link, "x")
	require.ErrorContains(t, err, "not a regular file")
}

func TestStatusRejectsBroadPermissionsAndChangedFileIdentity(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	payload := []byte(`{"version":1,"config_hash":"x","endpoints":{}}`)
	require.NoError(t, os.WriteFile(first, payload, 0o600))
	require.NoError(t, os.WriteFile(second, payload, 0o600))
	firstInfo, err := os.Lstat(first)
	require.NoError(t, err)
	secondInfo, err := os.Stat(second)
	require.NoError(t, err)
	require.ErrorContains(t, validateOpenedStatusFile(firstInfo, secondInfo), "changed during validation")

	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(first, 0o644))
		_, err = loadStatus(first, "x")
		require.ErrorContains(t, err, "permissions 0644 are too broad")
	}
}

func TestStatusPathIsIsolatedByCredentialFreePipelineHash(t *testing.T) {
	t.Setenv("NETDATA_LIB_DIR", t.TempDir())
	base := decodeDiscoveryConfig(t, `{
		"profiles":[{
			"name":"p",
			"scheme":"https",
			"auth_method":"basic",
			"username":"operator",
			"password":"first",
			"proxy_url":"http://proxy-user:first@proxy.example:8080"
		}],
		"networks":[{"subnet":"192.0.2.1","ports":[443],"profile":"p"}]
	}`)
	base.Source = "pipeline=one"
	first, err := NewDiscoverer(base)
	require.NoError(t, err)

	base.Profiles[0].JobConfig["password"] = "second"
	base.Profiles[0].JobConfig["proxy_url"] = "http://proxy-user:second@proxy.example:8080"
	second, err := NewDiscoverer(base)
	require.NoError(t, err)
	require.Equal(t, first.statusPath, second.statusPath)

	base.Source = "pipeline=two"
	third, err := NewDiscoverer(base)
	require.NoError(t, err)
	require.NotEqual(t, first.statusPath, third.statusPath)

	base.Source = "pipeline=one"
	base.Profiles[0].JobConfig["proxy_url"] = "http://other-proxy.example:8080"
	fourth, err := NewDiscoverer(base)
	require.NoError(t, err)
	require.NotEqual(t, first.statusPath, fourth.statusPath)
}

func TestDiscoveryConfigHashUsesSanitizedResolvedProxyRoute(t *testing.T) {
	profile := ProfileConfig{
		Name: "p", Scheme: "https",
		JobConfig:   map[string]any{"proxy_url": "${env:PROXY_URL}"},
		ProbeConfig: map[string]any{"proxy_url": "http://first:secret@proxy-a.example:8080"},
	}
	config := parsedConfig{
		profiles: map[string]ProfileConfig{"p": profile},
		networks: []network{{normalized: "192.0.2.1/32", ports: []int{443}, profile: profile}},
	}
	first, err := discoveryConfigHash("pipeline", config)
	require.NoError(t, err)

	profile.ProbeConfig = map[string]any{"proxy_url": "http://second:other@proxy-a.example:8080"}
	config.profiles["p"] = profile
	config.networks[0].profile = profile
	sameRoute, err := discoveryConfigHash("pipeline", config)
	require.NoError(t, err)
	require.Equal(t, first, sameRoute, "proxy credential changes must not enter cache identity")

	profile.ProbeConfig = map[string]any{"proxy_url": "http://third:private@proxy-b.example:8080"}
	config.profiles["p"] = profile
	config.networks[0].profile = profile
	differentRoute, err := discoveryConfigHash("pipeline", config)
	require.NoError(t, err)
	require.NotEqual(t, first, differentRoute, "resolved proxy routing changes must invalidate cache")
}
