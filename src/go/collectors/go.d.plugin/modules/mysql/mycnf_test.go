// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dsnFromFile(t *testing.T) {
	user := getUser()
	tests := map[string]struct {
		config      string
		expectedDSN string
		wantErr     bool
	}{
		"socket": {
			config: `
[client]
socket=/opt/bitnami/mariadb/tmp/mysql.sock
`,
			expectedDSN: user + "@unix(/opt/bitnami/mariadb/tmp/mysql.sock)/",
		},
		"socket, host, port": {
			config: `
[client]
host=10.0.0.0
port=3307
socket=/opt/bitnami/mariadb/tmp/mysql.sock
`,
			expectedDSN: user + "@unix(/opt/bitnami/mariadb/tmp/mysql.sock)/",
		},
		"host, port": {
			config: `
[client]
host=10.0.0.0
port=3307
`,
			expectedDSN: user + "@tcp(10.0.0.0:3307)/",
		},
		"only host": {
			config: `
[client]
host=10.0.0.0
`,
			expectedDSN: user + "@tcp(10.0.0.0:3306)/",
		},
		"only port": {
			config: `
[client]
port=3307
`,
			expectedDSN: user + "@tcp(localhost:3307)/",
		},
		"user, password": {
			config: `
[client]
user=user
password=password
`,
			expectedDSN: "user:password@/",
		},
		"empty": {
			config: `
[client]
`,
			expectedDSN: user + "@/",
		},
		"no client section": {
			config: `
[no_client]
`,
			wantErr: true,
		},
	}
	pattern := "netdata-godplugin-mysql-dsnFromFile-*"
	dir, err := os.MkdirTemp(os.TempDir(), pattern)
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }()

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := os.CreateTemp(dir, name)
			require.NoError(t, err)
			_ = f.Close()
			defer func() { _ = os.Remove(f.Name()) }()
			_ = os.WriteFile(f.Name(), []byte(test.config), 0644)

			if dsn, err := dsnFromFile(f.Name()); test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedDSN, dsn)
			}
		})
	}
}
