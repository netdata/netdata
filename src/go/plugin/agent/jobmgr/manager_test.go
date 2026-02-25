// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestManager_Run(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"stock => ok: add and remove": {
			createSim: func() *runSim {
				cfg := prepareStockCfg("success", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))

						sendConfGroup(in, cfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:success:name create accepted job /collectors/test/Jobs stock 'type=stock,module=success,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:name status running

CONFIG test:collector:success:name delete
`,
				}
			},
		},
		"stock => nok: add": {
			createSim: func() *runSim {
				cfg := prepareStockCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: []confgroup.Config{cfg},
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: nil,
					wantRunning: nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs stock 'type=stock,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name delete
`,
				}
			},
		},
		"stock => nok: add and remove": {
			createSim: func() *runSim {
				cfg := prepareStockCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))

						sendConfGroup(in, cfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs stock 'type=stock,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name delete
`,
				}
			},
		},
		"user => ok: add and remove": {
			createSim: func() *runSim {
				cfg := prepareUserCfg("success", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))

						sendConfGroup(in, cfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:success:name create accepted job /collectors/test/Jobs user 'type=user,module=success,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:name status running

CONFIG test:collector:success:name delete
		`,
				}
			},
		},
		"user => nok: add and remove": {
			createSim: func() *runSim {
				cfg := prepareUserCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))

						sendConfGroup(in, cfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs user 'type=user,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed

CONFIG test:collector:fail:name delete
		`,
				}
			},
		},
		"disc => ok: add and remove": {
			createSim: func() *runSim {
				cfg := prepareDiscoveredCfg("success", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))

						sendConfGroup(in, cfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:success:name create accepted job /collectors/test/Jobs discovered 'type=discovered,module=success,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:name status running

CONFIG test:collector:success:name delete
		`,
				}
			},
		},
		"disc => nok: add and remove": {
			createSim: func() *runSim {
				cfg := prepareDiscoveredCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))

						sendConfGroup(in, cfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs discovered 'type=discovered,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed

CONFIG test:collector:fail:name delete
		`,
				}
			},
		},
		"non-dyncfg => nok: diff src, diff name: add": {
			createSim: func() *runSim {
				stockCfg := prepareStockCfg("fail", "stock")
				discCfg := prepareDiscoveredCfg("fail", "discovered")
				userCfg := prepareUserCfg("fail", "user")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, stockCfg.Source(), stockCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(stockCfg), "enable"},
						}))

						sendConfGroup(in, discCfg.Source(), discCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(discCfg), "enable"},
						}))

						sendConfGroup(in, userCfg.Source(), userCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-enable",
							Args: []string{mgr.dyncfgJobID(userCfg), "enable"},
						}))
					},
					wantDiscovered: []confgroup.Config{
						stockCfg,
						userCfg,
						discCfg,
					},
					wantSeen: []confgroup.Config{
						stockCfg,
						discCfg,
						userCfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: discCfg, status: dyncfg.StatusFailed},
						{cfg: userCfg, status: dyncfg.StatusFailed},
					},
					wantRunning: nil,
					wantDyncfg: `
CONFIG test:collector:fail:stock create accepted job /collectors/test/Jobs stock 'type=stock,module=fail,job=stock' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:stock delete

CONFIG test:collector:fail:discovered create accepted job /collectors/test/Jobs discovered 'type=discovered,module=fail,job=discovered' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:discovered status failed

CONFIG test:collector:fail:user create accepted job /collectors/test/Jobs user 'type=user,module=fail,job=user' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:user status failed
		`,
				}
			},
		},
		"non-dyncfg => nok: diff src,src prio asc,same name: add": {
			createSim: func() *runSim {
				stockCfg := prepareStockCfg("fail", "name")
				discCfg := prepareDiscoveredCfg("fail", "name")
				userCfg := prepareUserCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, stockCfg.Source(), stockCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(stockCfg), "enable"},
						}))

						sendConfGroup(in, discCfg.Source(), discCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(discCfg), "enable"},
						}))

						sendConfGroup(in, userCfg.Source(), userCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-enable",
							Args: []string{mgr.dyncfgJobID(userCfg), "enable"},
						}))
					},
					wantDiscovered: []confgroup.Config{
						stockCfg,
						userCfg,
						discCfg,
					},
					wantSeen: []confgroup.Config{
						stockCfg,
						discCfg,
						userCfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: userCfg, status: dyncfg.StatusFailed},
					},
					wantRunning: nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs stock 'type=stock,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name delete

CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs discovered 'type=discovered,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed

CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs user 'type=user,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed
		`,
				}
			},
		},
		"non-dyncfg => nok: diff src,src prio asc,same name: add and remove": {
			createSim: func() *runSim {
				stockCfg := prepareStockCfg("fail", "name")
				discCfg := prepareDiscoveredCfg("fail", "name")
				userCfg := prepareUserCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, stockCfg.Source(), stockCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(stockCfg), "enable"},
						}))

						sendConfGroup(in, discCfg.Source(), discCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(discCfg), "enable"},
						}))

						sendConfGroup(in, userCfg.Source(), userCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-enable",
							Args: []string{mgr.dyncfgJobID(userCfg), "enable"},
						}))

						sendConfGroup(in, stockCfg.Source())
						sendConfGroup(in, discCfg.Source())
						sendConfGroup(in, userCfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs stock 'type=stock,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name delete

CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs discovered 'type=discovered,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed

CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs user 'type=user,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed

CONFIG test:collector:fail:name delete
		`,
				}
			},
		},
		"non-dyncfg => nok: diff src,src prio desc,same name: add": {
			createSim: func() *runSim {
				userCfg := prepareUserCfg("fail", "name")
				discCfg := prepareDiscoveredCfg("fail", "name")
				stockCfg := prepareStockCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, userCfg.Source(), userCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(userCfg), "enable"},
						}))

						sendConfGroup(in, discCfg.Source(), discCfg)
						sendConfGroup(in, stockCfg.Source(), stockCfg)
					},
					wantDiscovered: []confgroup.Config{
						stockCfg,
						userCfg,
						discCfg,
					},
					wantSeen: []confgroup.Config{
						userCfg,
						discCfg,
						stockCfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: userCfg, status: dyncfg.StatusFailed},
					},
					wantRunning: nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs user 'type=user,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed
		`,
				}
			},
		},
		"non-dyncfg => nok: diff src,src prio desc,same name: add and remove": {
			createSim: func() *runSim {
				userCfg := prepareUserCfg("fail", "name")
				discCfg := prepareDiscoveredCfg("fail", "name")
				stockCfg := prepareStockCfg("fail", "name")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, userCfg.Source(), userCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(userCfg), "enable"},
						}))

						sendConfGroup(in, discCfg.Source(), discCfg)
						sendConfGroup(in, stockCfg.Source(), stockCfg)

						sendConfGroup(in, userCfg.Source())
						sendConfGroup(in, discCfg.Source())
						sendConfGroup(in, stockCfg.Source())
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
CONFIG test:collector:fail:name create accepted job /collectors/test/Jobs user 'type=user,module=fail,job=name' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:name status failed

CONFIG test:collector:fail:name delete
		`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Get(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[get] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-get",
							Args: []string{mgr.dyncfgJobID(cfg), "get"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-get 404 application/json
{"status":404,"errorMessage":"The specified module 'success' job 'test' is not registered."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[get] existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test").
					Set("option_str", "1").
					Set("option_int", 1)
				bs, _ := json.Marshal(cfg)

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: bs,
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-get",
							Args: []string{mgr.dyncfgJobID(cfg), "get"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusAccepted},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-get 200 application/json
{"option_str":"1","option_int":1}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Userconfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[userconfig] existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-userconfig",
							Args: []string{mgr.dyncfgJobID(cfg), "userconfig"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-userconfig 200 application/yaml
jobs:
- name: test
  option_one: one
  option_two: 2

FUNCTION_RESULT_END
`,
				}
			},
		},
		"[userconfig] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success!", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-userconfig",
							Args: []string{mgr.dyncfgJobID(cfg), "userconfig"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-userconfig 404 application/json
{"status":404,"errorMessage":"The specified module 'success!' is not registered."}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Add(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[add] dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusAccepted},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
		"[add] dyncfg:nok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("fail", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusAccepted},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
		"[add] dyncfg:ok twice": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "2-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusAccepted},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Enable(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[enable] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-enable 404 application/json
{"status":404,"errorMessage":"job not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[enable] dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{cfg.FullName()},
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running
`,
				}
			},
		},
		"[enable] dyncfg:ok twice": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{cfg.FullName()},
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running
`,
				}
			},
		},
		"[enable] dyncfg:nok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("fail", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusFailed},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test status failed
`,
				}
			},
		},
		"[enable] dyncfg:nok twice": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("fail", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusFailed},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test status failed

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":"job enable failed: mock failed init"}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test status failed
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Disable(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[disable] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-disable 404 application/json
{"status":404,"errorMessage":"job not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[disable] dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusDisabled},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled
`,
				}
			},
		},
		"[disable] dyncfg:ok twice": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusDisabled},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled

FUNCTION_RESULT_BEGIN 3-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled
`,
				}
			},
		},
		"[disable] dyncfg:nok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("fail", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusDisabled},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test status disabled
`,
				}
			},
		},
		"[disable] dyncfg:nok twice": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("fail", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusDisabled},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test status disabled

FUNCTION_RESULT_BEGIN 3-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:fail:test status disabled
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Restart(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[restart] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-restart",
							Args: []string{mgr.dyncfgJobID(cfg), "restart"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-restart 404 application/json
{"status":404,"errorMessage":"job not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[restart] not enabled dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-restart",
							Args: []string{mgr.dyncfgJobID(cfg), "restart"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusAccepted},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-restart 405 application/json
{"status":405,"errorMessage":"restarting is not allowed in 'accepted' state."}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status accepted
`,
				}
			},
		},
		"[restart] enabled dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-restart",
							Args: []string{mgr.dyncfgJobID(cfg), "restart"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{cfg.FullName()},
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running

FUNCTION_RESULT_BEGIN 3-restart 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running
`,
				}
			},
		},
		"[restart] disabled dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-disable",
							Args: []string{mgr.dyncfgJobID(cfg), "disable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-restart",
							Args: []string{mgr.dyncfgJobID(cfg), "restart"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusDisabled},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled

FUNCTION_RESULT_BEGIN 3-restart 405 application/json
{"status":405,"errorMessage":"restarting is not allowed in 'disabled' state."}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled
`,
				}
			},
		},
		"[restart] enabled dyncfg:ok multiple times": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-restart",
							Args: []string{mgr.dyncfgJobID(cfg), "restart"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "4-restart",
							Args: []string{mgr.dyncfgJobID(cfg), "restart"},
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{cfg.FullName()},
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running

FUNCTION_RESULT_BEGIN 3-restart 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running

FUNCTION_RESULT_BEGIN 4-restart 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Remove(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[remove] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-remove",
							Args: []string{mgr.dyncfgJobID(cfg), "remove"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-remove 404 application/json
{"status":404,"errorMessage":"job not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[remove] non-dyncfg": {
			createSim: func() *runSim {
				stockCfg := prepareStockCfg("success", "stock")
				userCfg := prepareUserCfg("success", "user")
				discCfg := prepareDiscoveredCfg("success", "discovered")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, stockCfg.Source(), stockCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(stockCfg), "enable"},
						}))

						sendConfGroup(in, userCfg.Source(), userCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(userCfg), "enable"},
						}))

						sendConfGroup(in, discCfg.Source(), discCfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-enable",
							Args: []string{mgr.dyncfgJobID(discCfg), "enable"},
						}))

						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-remove",
							Args: []string{mgr.dyncfgJobID(stockCfg), "remove"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-remove",
							Args: []string{mgr.dyncfgJobID(userCfg), "remove"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-remove",
							Args: []string{mgr.dyncfgJobID(discCfg), "remove"},
						}))
					},
					wantDiscovered: []confgroup.Config{
						stockCfg,
						userCfg,
						discCfg,
					},
					wantSeen: []confgroup.Config{
						stockCfg,
						userCfg,
						discCfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: stockCfg, status: dyncfg.StatusRunning},
						{cfg: userCfg, status: dyncfg.StatusRunning},
						{cfg: discCfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{stockCfg.FullName(), userCfg.FullName(), discCfg.FullName()},
					wantDyncfg: `
CONFIG test:collector:success:stock create accepted job /collectors/test/Jobs stock 'type=stock,module=success,job=stock' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:stock status running

CONFIG test:collector:success:user create accepted job /collectors/test/Jobs user 'type=user,module=success,job=user' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:user status running

CONFIG test:collector:success:discovered create accepted job /collectors/test/Jobs discovered 'type=discovered,module=success,job=discovered' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:discovered status running

FUNCTION_RESULT_BEGIN 1-remove 405 application/json
{"status":405,"errorMessage":"removing jobs of type 'stock' is not supported, only 'dyncfg' jobs can be removed."}
FUNCTION_RESULT_END

FUNCTION_RESULT_BEGIN 2-remove 405 application/json
{"status":405,"errorMessage":"removing jobs of type 'user' is not supported, only 'dyncfg' jobs can be removed."}
FUNCTION_RESULT_END

FUNCTION_RESULT_BEGIN 3-remove 405 application/json
{"status":405,"errorMessage":"removing jobs of type 'discovered' is not supported, only 'dyncfg' jobs can be removed."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[remove] not enabled dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-remove",
							Args: []string{mgr.dyncfgJobID(cfg), "remove"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test delete
`,
				}
			},
		},
		"[remove] enabled dyncfg:ok": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(cfg.Module()), "add", cfg.Name()},
							Payload: []byte("{}"),
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "3-remove",
							Args: []string{mgr.dyncfgJobID(cfg), "remove"},
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running

FUNCTION_RESULT_BEGIN 3-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test delete
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func TestManager_Run_Dyncfg_Update(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"[update] non-existing": {
			createSim: func() *runSim {
				cfg := prepareDyncfgCfg("success", "test")

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-update",
							Args:    []string{mgr.dyncfgJobID(cfg), "update"},
							Payload: []byte("{}"),
						}))
					},
					wantDiscovered: nil,
					wantSeen:       nil,
					wantExposed:    nil,
					wantRunning:    nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-update 404 application/json
{"status":404,"errorMessage":"job not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"[update] enabled dyncfg:ok with dyncfg:ok": {
			createSim: func() *runSim {
				origCfg := prepareDyncfgCfg("success", "test").
					Set("option_str", "1")
				updCfg := prepareDyncfgCfg("success", "test").
					Set("option_str", "2")
				origBs, _ := json.Marshal(origCfg)
				updBs, _ := json.Marshal(updCfg)

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(origCfg.Module()), "add", origCfg.Name()},
							Payload: origBs,
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-enable",
							Args: []string{mgr.dyncfgJobID(origCfg), "enable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "3-update",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgJobID(origCfg), "update"},
							Payload: updBs,
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						updCfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: updCfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{updCfg.FullName()},
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running

FUNCTION_RESULT_BEGIN 3-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status running
`,
				}
			},
		},
		"[update] disabled dyncfg:ok with dyncfg:ok": {
			createSim: func() *runSim {
				origCfg := prepareDyncfgCfg("success", "test").
					Set("option_str", "1")
				updCfg := prepareDyncfgCfg("success", "test").
					Set("option_str", "2")
				origBs, _ := json.Marshal(origCfg)
				updBs, _ := json.Marshal(updCfg)

				return &runSim{
					do: func(mgr *Manager, _ chan []*confgroup.Group) {
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "1-add",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgModID(origCfg.Module()), "add", origCfg.Name()},
							Payload: origBs,
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "2-disable",
							Args: []string{mgr.dyncfgJobID(origCfg), "disable"},
						}))
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:     "3-update",
							Source:  "type=dyncfg",
							Args:    []string{mgr.dyncfgJobID(origCfg), "update"},
							Payload: updBs,
						}))
					},
					wantDiscovered: nil,
					wantSeen: []confgroup.Config{
						updCfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: updCfg, status: dyncfg.StatusDisabled},
					},
					wantRunning: nil,
					wantDyncfg: `

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test create accepted job /collectors/test/Jobs dyncfg 'type=dyncfg' 'schema get enable disable update restart test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled

FUNCTION_RESULT_BEGIN 3-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:success:test status disabled
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func sendConfGroup(in chan []*confgroup.Group, src string, configs ...confgroup.Config) {
	in <- prepareCfgGroups(src, configs...)
	in <- prepareCfgGroups("_")
}

func prepareCfgGroups(src string, configs ...confgroup.Config) []*confgroup.Group {
	return []*confgroup.Group{{Configs: configs, Source: src}}
}

func prepareStockCfg(module, job string) confgroup.Config {
	return confgroup.Config{}.
		SetSourceType(confgroup.TypeStock).
		SetProvider("test").
		SetSource(fmt.Sprintf("type=stock,module=%s,job=%s", module, job)).
		SetModule(module).
		SetName(job)
}

func prepareUserCfg(module, job string) confgroup.Config {
	return confgroup.Config{}.
		SetSourceType(confgroup.TypeUser).
		SetProvider("test").
		SetSource(fmt.Sprintf("type=user,module=%s,job=%s", module, job)).
		SetModule(module).
		SetName(job)
}

func prepareDiscoveredCfg(module, job string) confgroup.Config {
	return confgroup.Config{}.
		SetSourceType(confgroup.TypeDiscovered).
		SetProvider("test").
		SetSource(fmt.Sprintf("type=discovered,module=%s,job=%s", module, job)).
		SetModule(module).
		SetName(job)
}

func prepareDyncfgCfg(module, job string) confgroup.Config {
	return confgroup.Config{}.
		SetSourceType(confgroup.TypeDyncfg).
		SetProvider("dyncfg").
		SetSource("type=dyncfg").
		SetModule(module).
		SetName(job)
}

func prepareFunctionOnlyCfg(module, job string) confgroup.Config {
	return confgroup.Config{}.
		SetSourceType(confgroup.TypeUser).
		SetProvider("test").
		SetSource(fmt.Sprintf("type=user,module=%s,job=%s", module, job)).
		SetModule(module).
		SetName(job).
		Set("function_only", true)
}

func TestManager_Run_FunctionOnly(t *testing.T) {
	tests := map[string]struct {
		createSim func() *runSim
	}{
		"function_only config for module without methods => error": {
			createSim: func() *runSim {
				cfg := prepareFunctionOnlyCfg("nofuncs", "test")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: []confgroup.Config{cfg},
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusFailed},
					},
					wantRunning: nil,
					wantDyncfg: `
CONFIG test:collector:nofuncs:test create accepted job /collectors/test/Jobs user 'type=user,module=nofuncs,job=test' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 400 application/json
{"status":400,"errorMessage":"invalid configuration: failed to apply configuration: function_only is set but nofuncs module has no methods defined"}
FUNCTION_RESULT_END

CONFIG test:collector:nofuncs:test status failed
`,
				}
			},
		},
		"function_only config for module with methods => ok": {
			createSim: func() *runSim {
				cfg := prepareFunctionOnlyCfg("withfuncs", "test")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: []confgroup.Config{cfg},
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{cfg.FullName()},
					wantDyncfg: `
CONFIG test:collector:withfuncs:test create accepted job /collectors/test/Jobs user 'type=user,module=withfuncs,job=test' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:withfuncs:test status running
`,
				}
			},
		},
		"FunctionOnly module => ok": {
			createSim: func() *runSim {
				cfg := prepareUserCfg("funconly", "test")

				return &runSim{
					do: func(mgr *Manager, in chan []*confgroup.Group) {
						sendConfGroup(in, cfg.Source(), cfg)
						mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
							UID:  "1-enable",
							Args: []string{mgr.dyncfgJobID(cfg), "enable"},
						}))
					},
					wantDiscovered: []confgroup.Config{cfg},
					wantSeen: []confgroup.Config{
						cfg,
					},
					wantExposed: []wantExposedEntry{
						{cfg: cfg, status: dyncfg.StatusRunning},
					},
					wantRunning: []string{cfg.FullName()},
					wantDyncfg: `
CONFIG test:collector:funconly:test create accepted job /collectors/test/Jobs user 'type=user,module=funconly,job=test' 'schema get enable disable update restart test userconfig' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:collector:funconly:test status running
`,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}
