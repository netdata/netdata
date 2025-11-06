package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactDSN(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"simple user:pass@host": {
			input:    "user:password@localhost",
			expected: "user:****@localhost",
		},
		"simple user:pass@host:port": {
			input:    "user:password@localhost:5432",
			expected: "user:****@localhost:5432",
		},
		"no credentials just host": {
			input:    "localhost:5432",
			expected: "localhost:5432",
		},
		"URL without credentials": {
			input:    "postgresql://localhost/dbname",
			expected: "postgresql://localhost/dbname",
		},
		"colon in host but no password": {
			input:    "localhost:5432/db",
			expected: "localhost:5432/db",
		},
		"empty string": {
			input:    "",
			expected: "",
		},
		"just scheme": {
			input:    "postgresql://",
			expected: "postgresql://",
		},
		"simple user@host (no password)": {
			input:    "user@localhost",
			expected: "****@localhost",
		},
		"postgresql URL with password": {
			input:    "postgresql://user:password@localhost/dbname",
			expected: "postgresql://user:****@localhost/dbname",
		},
		"postgresql URL with password and port": {
			input:    "postgresql://user:password@localhost:5432/dbname",
			expected: "postgresql://user:****@localhost:5432/dbname",
		},
		"postgresql URL without password": {
			input:    "postgresql://user@localhost/dbname",
			expected: "postgresql://****@localhost/dbname",
		},
		"mysql URL with password": {
			input:    "mysql://root:secret@localhost:3306/mydb",
			expected: "mysql://root:****@localhost:3306/mydb",
		},
		"postgres URL with complex password": {
			input:    "postgres://admin:p@ss:w0rd@localhost/db",
			expected: "postgres://admin:****@localhost/db",
		},
		"URL with query params": {
			input:    "postgresql://user:pass@localhost/db?sslmode=disable",
			expected: "postgresql://user:****@localhost/db?sslmode=disable",
		},
		"user with special chars in password": {
			input:    "user:p@ssw0rd!@localhost",
			expected: "user:****@localhost",
		},
		"redis URL": {
			input:    "redis://user:password@localhost:6379/0",
			expected: "redis://user:****@localhost:6379/0",
		},
		"mongodb URL": {
			input:    "mongodb://admin:secret@localhost:27017/mydb",
			expected: "mongodb://admin:****@localhost:27017/mydb",
		},
		"URL with IP address": {
			input:    "postgresql://user:pass@192.168.1.1:5432/db",
			expected: "postgresql://user:****@192.168.1.1:5432/db",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := redactDSN(tc.input)

			assert.Equal(t, tc.expected, got)
		})
	}
}
