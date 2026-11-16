// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_extractServerVersion(t *testing.T) {
	tests := map[string]struct {
		info    []byte
		wantSrv string
		wantVer string
	}{
		"redis 6.0.9":         {info: dataVer609InfoAll, wantSrv: "redis", wantVer: "6.0.9"},
		"valkey 9.1.2":        {info: dataValkeyInfoAll, wantSrv: "redis", wantVer: "7.2.4"},
		"dragonfly df-v2.0.0": {info: dataDragonflyInfoAll, wantSrv: "redis", wantVer: "7.4.0"},
		"keydb 6.3.4":         {info: dataKeydbInfoAll, wantSrv: "redis", wantVer: "6.3.4"},
		"kvrocks 2.17.0":      {info: dataKvrocksInfoAll, wantSrv: "kvrocks", wantVer: "2.17.0"},
		"garnet 2.2.0":        {info: dataGarnetInfoAll, wantSrv: "garnet", wantVer: "2.2.0"},
		"garnet with redis_version first": {
			info:    withRedisVersionFirst(dataGarnetInfoAll),
			wantSrv: "garnet",
			wantVer: "2.2.0",
		},
		"pika 3.4.0": {info: dataPikaInfoAll, wantSrv: "pika", wantVer: "3.4.0"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server, version, err := extractServerVersion(string(test.info))
			require.NoError(t, err)
			assert.Equal(t, test.wantSrv, server)
			assert.Equal(t, test.wantVer, version)
		})
	}
}

func Test_extractServerVersion_errors(t *testing.T) {
	tests := map[string]string{
		"no version property":         "# Server\nos:Linux\n",
		"unparseable version":         "# Server\nredis_version:unknown\n",
		"empty response":              "",
		"first version is unparsable": "# Server\nredis_version:7\ngcc_version:9.3.0\n",
	}

	for name, info := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := extractServerVersion(info)
			assert.Error(t, err)
		})
	}
}

// withRedisVersionFirst moves the redis_version line before the garnet_version
// line, as if a Garnet server reported its redis compatibility version first.
func withRedisVersionFirst(info []byte) []byte {
	lines := strings.Split(string(info), "\n")
	redisLine := ""
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "redis_version:") {
			redisLine = line
			continue
		}
		kept = append(kept, line)
	}
	if redisLine == "" {
		return info
	}
	for i, line := range kept {
		if strings.HasPrefix(line, "garnet_version:") {
			kept = append(kept[:i], append([]string{redisLine}, kept[i:]...)...)
			break
		}
	}
	return []byte(strings.Join(kept, "\n"))
}
