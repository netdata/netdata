// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

const (
	passwordVariable = "WMI_FIXTURE_PASSWORD"
	// resolvedMarker is the resolved password and part of every raw cause, so it
	// must never reach a job's output.
	resolvedMarker = "fixture-resolved-secret"
)

// A job whose password is a secret reference gets the Agent's redaction. Its
// output keeps exactly the collector's public messages.
func TestCollector_SecretReferenceJobKeepsPublicMessages(t *testing.T) {
	classified := fmt.Errorf("dial %s: %w", resolvedMarker, context.DeadlineExceeded)
	unclassified := errors.New("login as " + resolvedMarker + " rejected")
	sessionWith := func(results map[string]queryResult) func(context.Context) (querySession, error) {
		return func(context.Context) (querySession, error) {
			return &fakeSession{
				results: results,
			}, nil
		}
	}
	tests := map[string]struct {
		options    map[string]any
		newSession func(context.Context) (querySession, error)
		pingErr    error
		// collect runs a collection after the test, with the module's logger.
		collect bool
		wantErr string
		wantLog []string
	}{
		"invalid option": {
			options: map[string]any{"auth": "kerberos"},
			wantErr: "auth must be ntlm",
		},
		"invalid transport": {
			options: map[string]any{"transport": resolvedMarker},
			wantErr: "transport must be dcom or winrm",
		},
		"unreadable tls_ca": {
			options: map[string]any{"transport": "winrm", "tls_ca": "/nonexistent/" + resolvedMarker + ".pem"},
			wantErr: "could not load tls_ca",
		},
		"classified session failure": {
			newSession: func(context.Context) (querySession, error) { return nil, classified },
			wantErr:    "failed to open WMI session: WMI collection timed out",
		},
		"unclassified session failure": {
			newSession: func(context.Context) (querySession, error) { return nil, unclassified },
			wantErr:    "failed to open WMI session",
		},
		"no group succeeded": {
			options: map[string]any{
				"collectors": map[string]any{
					"system":  false,
					"network": false,
					"disk":    false,
					"smb":     false,
					"time":    false,
				},
			},
			newSession: sessionWith(map[string]queryResult{
				queryCPU:    {err: classified},
				queryMemory: {},
			}),
			wantErr: "no enabled WMI query group succeeded: CPU WMI query failed: WMI collection timed out\n" +
				"memory WMI query returned no usable observations\n" +
				"memory counters WMI query failed\n" +
				"paging file WMI query failed\n" +
				"NUMA node WMI query failed",
		},
		"collection timeout": {
			options:    map[string]any{"timeout": "50ms"},
			newSession: func(context.Context) (querySession, error) { return &stalledSession{}, nil },
			wantErr:    "WMI collection did not finish within the timeout",
		},
		"failed groups are logged": {
			newSession: sessionWith(map[string]queryResult{
				queryCPU:               {err: classified},
				queryMemory:            {err: unclassified},
				querySystem:            validQueryResults()[querySystem],
				queryNetworkInterfaces: validQueryResults()[queryNetworkInterfaces],
				queryNetworkAdapters:   validQueryResults()[queryNetworkAdapters],
				queryDisk:              validQueryResults()[queryDisk],
			}),
			wantLog: []string{"CPU WMI query failed: WMI collection timed out", "memory WMI query failed"},
		},
		"failed adapter query is logged": {
			newSession: sessionWith(func() map[string]queryResult {
				results := validQueryResults()
				results[queryNetworkAdapters] = queryResult{
					err: classified,
				}
				return results
			}()),
			collect: true,
			wantLog: []string{"network adapter WMI query failed: WMI collection timed out"},
		},
		"failed inventory queries are logged": {
			newSession: sessionWith(func() map[string]queryResult {
				results := validQueryResults()
				results[queryOSInfo] = queryResult{
					err: classified,
				}
				results[querySystemInfo] = queryResult{
					err: unclassified,
				}
				return results
			}()),
			collect: true,
			wantLog: []string{
				"operating system WMI inventory query failed: WMI collection timed out",
				"computer system WMI inventory query failed",
			},
		},
		"missing ping privilege is logged": {
			pingErr: fmt.Errorf("%w: %s", notPermitted(syscall.EPERM), resolvedMarker),
			collect: true,
			wantLog: []string{pingDisabledMessage},
		},
		"failed ping is logged": {
			pingErr: &pinger.ProbeError{
				Host:  resolvedMarker,
				Stage: "resolve",
				Err: &net.DNSError{
					Err:  "no such host",
					Name: resolvedMarker,
				},
			},
			collect: true,
			wantLog: []string{"ICMP ping failed: could not resolve the host"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv(passwordVariable, resolvedMarker)
			newSession := tc.newSession
			if newSession == nil {
				newSession = sessionWith(validQueryResults())
			}
			ping := &fakePinger{
				sample: answeredPing(),
				err:    tc.pingErr,
			}
			factory, moduleLog, created := referenceJobFactory(t, newSession, ping.factory)

			err := factory.Test(t.Context(), referenceJob(tc.options))
			if tc.collect {
				require.NoError(t, err)
				// The tested module keeps the Job Manager's redacting logger.
				_, err = collecttest.CollectScalarSeries(created())
			}

			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				message, ok := dyncfg.PublicMessage(err)
				require.True(t, ok, err.Error())
				assert.Equal(t, tc.wantErr, message)
				assert.NotContains(t, err.Error(), resolvedMarker)
				assert.NotContains(t, err.Error(), "50ms", "the timeout is a configuration value")
				// The redactor replaced the collector's private causes.
				assertErrorTreeOmits(t, err, resolvedMarker)
			}
			logged, err := os.ReadFile(moduleLog)
			require.NoError(t, err)
			for _, line := range tc.wantLog {
				assert.Contains(t, string(logged), line)
			}
			assert.NotContains(t, string(logged), resolvedMarker)
			assert.NotContains(t, string(logged), "redacted")
		})
	}
}

// assertErrorTreeOmits checks every error in the tree, including joined ones.
func assertErrorTreeOmits(t *testing.T, err error, marker string) {
	t.Helper()
	assert.NotContains(t, err.Error(), marker)
	switch wrapped := err.(type) {
	case interface{ Unwrap() error }:
		if inner := wrapped.Unwrap(); inner != nil {
			assertErrorTreeOmits(t, inner, marker)
		}
	case interface{ Unwrap() []error }:
		for _, inner := range wrapped.Unwrap() {
			assertErrorTreeOmits(t, inner, marker)
		}
	}
}

// referenceJobFactory builds the Agent's DynCfg test path for the module: the
// shipped env resolver and the Job Manager's error and log redaction. It
// returns the file that receives the module's log lines and the last module
// the factory created.
func referenceJobFactory(
	t *testing.T,
	newSession func(context.Context) (querySession, error),
	newPinger func(pinger.Config, *logger.Logger) (pinger.Client, error),
) (*joboutput.ConfigModuleFactory, string, func() *Collector) {
	t.Helper()
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	require.NoError(t, err)
	configs, err := secrets.NewConfigResolver(resolver, func([]string) (secretresolver.AtomicScope, error) {
		return nil, errors.New("no store references in this fixture")
	})
	require.NoError(t, err)

	logFile, err := os.CreateTemp(t.TempDir(), "module-log")
	require.NoError(t, err)
	t.Cleanup(func() { _ = logFile.Close() })
	// The factory's logger, which module loggers derive from, keeps the
	// standard error it sees when created.
	stderr := os.Stderr
	os.Stderr = logFile
	defer func() { os.Stderr = stderr }()

	var last *Collector
	factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
		Modules: collectorapi.Registry{
			"windows": collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					last = New()
					last.newSession = newSession
					last.newPinger = newPinger
					return last
				},
				Config: func() any { return &Config{} },
			},
		},
		Configs: configs,
	})
	require.NoError(t, err)
	return factory, logFile.Name(), func() *Collector { return last }
}

func referenceJob(options map[string]any) confgroup.Config {
	cfg := confgroup.Config{
		"module":   "windows",
		"name":     "fixture",
		"host":     "windows.example.com",
		"auth":     "ntlm",
		"username": `EXAMPLE\monitor`,
		"password": "${env:" + passwordVariable + "}",
	}
	maps.Copy(cfg, options)
	return cfg.SetSourceType(confgroup.TypeUser)
}
