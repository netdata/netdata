// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessAttemptIdentityKeyIsBoundedAndStructurallyFramed(t *testing.T) {
	require.Empty(t, ProcessAttemptIdentityKey(""))
	key := ProcessAttemptIdentityKey("collector-job", "module_job")
	require.Equal(t, key, ProcessAttemptIdentityKey("collector-job", "module_job"))
	require.Len(t, key, sha256.Size)
	require.NotEqual(t, key, ProcessAttemptIdentityKey("collector-job-test", "module_job"))
	require.NotEqual(
		t,
		ProcessAttemptIdentityKey("domain", "ab", "c"),
		ProcessAttemptIdentityKey("domain", "a", "bc"),
	)
}

func TestProcessAttemptDiagnosticResourceUsesBoundedFallback(t *testing.T) {
	require.Equal(
		t,
		"module_job",
		ProcessAttemptDiagnosticResource("module_job", "collector job"),
	)
	for name, resource := range map[string]string{
		"empty":        "",
		"long":         strings.Repeat("a", MaximumProcessAttemptDiagnosticResourceBytes+1),
		"invalid UTF8": string([]byte{0xff}),
		"control":      "module\njob",
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t,
				"collector job",
				ProcessAttemptDiagnosticResource(resource, "collector job"),
			)
		})
	}
	require.Equal(
		t,
		"job manager resource",
		ProcessAttemptDiagnosticResource("", ""),
	)
}

func TestProcessAttemptIdentityValidationUsesSharedBounds(t *testing.T) {
	identity := ProcessAttemptIdentity{
		Namespace: ProcessAttemptJob,
		Key:       ProcessAttemptIdentityKey("collector-job", "module_job"),
		Resource:  "module_job",
	}
	require.True(t, identity.Valid())

	identity.Key = strings.Repeat("k", MaximumProcessAttemptKeyBytes+1)
	require.False(t, identity.Valid())
	identity.Key = "key"
	identity.Resource = strings.Repeat(
		"r",
		MaximumProcessAttemptDiagnosticResourceBytes+1,
	)
	require.False(t, identity.Valid())
}
