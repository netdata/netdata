// SPDX-License-Identifier: GPL-3.0-or-later

package filestatus

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"

	"github.com/stretchr/testify/assert"
)

// TODO: tech debt
func TestLoadStore(t *testing.T) {

}

// TODO: tech debt
func TestStore_Contains(t *testing.T) {

}

func TestStore_add(t *testing.T) {
	tests := map[string]struct {
		prepare      func() *Store
		input        confgroup.Config
		wantItemsNum int
	}{
		"add cfg to the empty store": {
			prepare: func() *Store {
				return &Store{}
			},
			input: prepareConfig(
				"module", "modName",
				"name", "jobName",
			),
			wantItemsNum: 1,
		},
		"add cfg that already in the store": {
			prepare: func() *Store {
				return &Store{
					items: map[string]map[string]string{
						"modName": {"jobName:14246646561040761746": "state"},
					},
				}
			},
			input: prepareConfig(
				"module", "modName",
				"name", "jobName",
			),
			wantItemsNum: 1,
		},
		"add cfg with same module, same name, but specific options": {
			prepare: func() *Store {
				return &Store{
					items: map[string]map[string]string{
						"modName": {"jobName:18299273693089411682": "state"},
					},
				}
			},
			input: prepareConfig(
				"module", "modName",
				"name", "jobName",
				"opt", "val",
			),
			wantItemsNum: 2,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := test.prepare()
			s.add(test.input, "state")
			assert.Equal(t, test.wantItemsNum, calcStoreItems(s))
		})
	}
}

func TestStore_remove(t *testing.T) {
	tests := map[string]struct {
		prepare      func() *Store
		input        confgroup.Config
		wantItemsNum int
	}{
		"remove cfg from the empty store": {
			prepare: func() *Store {
				return &Store{}
			},
			input: prepareConfig(
				"module", "modName",
				"name", "jobName",
			),
			wantItemsNum: 0,
		},
		"remove cfg from the store": {
			prepare: func() *Store {
				return &Store{
					items: map[string]map[string]string{
						"modName": {
							"jobName:14246646561040761746": "state",
							"jobName:14246646561040761747": "state",
						},
					},
				}
			},
			input: prepareConfig(
				"module", "modName",
				"name", "jobName",
			),
			wantItemsNum: 1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := test.prepare()
			s.remove(test.input)
			assert.Equal(t, test.wantItemsNum, calcStoreItems(s))
		})
	}
}

func calcStoreItems(s *Store) (num int) {
	for _, v := range s.items {
		for range v {
			num++
		}
	}
	return num
}

func prepareConfig(values ...string) confgroup.Config {
	cfg := confgroup.Config{}
	for i := 1; i < len(values); i += 2 {
		cfg[values[i-1]] = values[i]
	}
	return cfg
}
