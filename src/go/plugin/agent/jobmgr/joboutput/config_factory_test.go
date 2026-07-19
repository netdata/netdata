// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func TestConfigModuleFactoryCleansEveryAttemptAndPrefersV2(t *testing.T) {
	tests := map[string]struct {
		operation   string
		checkErr    error
		wantErr     bool
		wantCreates int
	}{
		"configuration success": {
			operation: "configuration", wantCreates: 1,
		},
		"test success": {
			operation: "test", wantCreates: 1,
		},
		"test failure": {
			operation: "test", checkErr: errors.New("check failed"),
			wantErr: true, wantCreates: 1,
		},
		"validation success": {
			operation: "validate", wantCreates: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := &factoryTestState{}
			v1Creates := 0
			v2Creates := 0
			resolver, err := secretresolver.NewAtomicResolver(nil)
			if err != nil {
				t.Fatal(err)
			}
			factory, err := NewConfigModuleFactory(
				ConfigModuleFactoryConfig{
					Modules: collectorapi.Registry{
						"module": {
							Create: func() collectorapi.CollectorV1 {
								v1Creates++
								return state.module(nil, false)
							},
							CreateV2: func() collectorapi.CollectorV2 {
								v2Creates++
								return &factoryTestV2{
									state: state, checkErr: test.checkErr,
								}
							},
						},
					},
					Resolver:   resolver,
					StoreScope: unavailableStoreScope,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			config := factoryTestConfig(false)
			switch test.operation {
			case "configuration":
				payload, runErr := factory.Configuration(
					context.Background(),
					config,
				)
				err = runErr
				if runErr == nil && !json.Valid(payload) {
					t.Fatalf("configuration payload is not JSON: %q", payload)
				}
			case "test":
				err = factory.Test(context.Background(), config)
			case "validate":
				err = factory.Validate(context.Background(), config)
			default:
				t.Fatalf("unknown operation %q", test.operation)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v, want error=%v", err, test.wantErr)
			}
			if v1Creates != 0 ||
				v2Creates != test.wantCreates ||
				state.collectorCleanup != test.wantCreates {
				t.Fatalf(
					"creates v1/v2=%d/%d cleanup=%d want=0/%d/%d",
					v1Creates,
					v2Creates,
					state.collectorCleanup,
					test.wantCreates,
					test.wantCreates,
				)
			}
		})
	}
}
