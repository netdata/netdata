# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.41.0...HEAD)

**Merged pull requests:**

- Improve the update of the alert chart name in the database [\#15490](https://github.com/netdata/netdata/pull/15490) ([stelfrag](https://github.com/stelfrag))
- Add basic slabinfo metadata. [\#15484](https://github.com/netdata/netdata/pull/15484) ([Ferroin](https://github.com/Ferroin))
- docs: note that health foreach works only with template [\#15478](https://github.com/netdata/netdata/pull/15478) ([ilyam8](https://github.com/ilyam8))
- Yaml file updates [\#15477](https://github.com/netdata/netdata/pull/15477) ([Ancairon](https://github.com/Ancairon))
- Rename most-popular to most\_popular in categories.yaml [\#15476](https://github.com/netdata/netdata/pull/15476) ([Ancairon](https://github.com/Ancairon))
- Fix coverity issue [\#15475](https://github.com/netdata/netdata/pull/15475) ([stelfrag](https://github.com/stelfrag))
- Memory Controller \(MC\) and DIMM Error Detection And Correction \(EDAC\) [\#15473](https://github.com/netdata/netdata/pull/15473) ([ktsaou](https://github.com/ktsaou))
- meta schema change multi-instance to multi\_instance [\#15470](https://github.com/netdata/netdata/pull/15470) ([ilyam8](https://github.com/ilyam8))
- fix anchors [\#15469](https://github.com/netdata/netdata/pull/15469) ([Ancairon](https://github.com/Ancairon))
- fix the calculation of incremental-sum [\#15468](https://github.com/netdata/netdata/pull/15468) ([ktsaou](https://github.com/ktsaou))
- apps.plugin fds limits improvements [\#15467](https://github.com/netdata/netdata/pull/15467) ([ktsaou](https://github.com/ktsaou))
- Add community key in schema [\#15465](https://github.com/netdata/netdata/pull/15465) ([Ancairon](https://github.com/Ancairon))
- Overhaul deployment strategies documentation [\#15464](https://github.com/netdata/netdata/pull/15464) ([ralphm](https://github.com/ralphm))
- Update debugfs plugin metadata. [\#15463](https://github.com/netdata/netdata/pull/15463) ([Ferroin](https://github.com/Ferroin))
- Update proc plugin yaml [\#15460](https://github.com/netdata/netdata/pull/15460) ([Ancairon](https://github.com/Ancairon))
- Macos yaml updates [\#15459](https://github.com/netdata/netdata/pull/15459) ([Ancairon](https://github.com/Ancairon))
- Freeipmi yaml updates [\#15458](https://github.com/netdata/netdata/pull/15458) ([Ancairon](https://github.com/Ancairon))
- Add short descriptions to cgroups yaml [\#15457](https://github.com/netdata/netdata/pull/15457) ([Ancairon](https://github.com/Ancairon))
- Store and transmit chart\_name to cloud in alert events [\#15441](https://github.com/netdata/netdata/pull/15441) ([MrZammler](https://github.com/MrZammler))
- Add linux powercap metrics collector [\#15364](https://github.com/netdata/netdata/pull/15364) ([fhriley](https://github.com/fhriley))
- Hash table charts [\#15323](https://github.com/netdata/netdata/pull/15323) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix non-interactive options for apt-get and zypper. [\#15288](https://github.com/netdata/netdata/pull/15288) ([zeylos](https://github.com/zeylos))

## [v1.41.0](https://github.com/netdata/netdata/tree/v1.41.0) (2023-07-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.40.1...v1.41.0)

**Merged pull requests:**

- Include license for web v2 [\#15453](https://github.com/netdata/netdata/pull/15453) ([tkatsoulas](https://github.com/tkatsoulas))
- Updates to metadata.yaml [\#15452](https://github.com/netdata/netdata/pull/15452) ([shyamvalsan](https://github.com/shyamvalsan))
- Add apps yaml [\#15451](https://github.com/netdata/netdata/pull/15451) ([Ancairon](https://github.com/Ancairon))
- Add cgroups yaml [\#15450](https://github.com/netdata/netdata/pull/15450) ([Ancairon](https://github.com/Ancairon))
- Fix multiline [\#15449](https://github.com/netdata/netdata/pull/15449) ([Ancairon](https://github.com/Ancairon))
- bump v2 dashboard to v6.21.3 [\#15448](https://github.com/netdata/netdata/pull/15448) ([ilyam8](https://github.com/ilyam8))
- fix alerts transitions search when something specific is asked for [\#15447](https://github.com/netdata/netdata/pull/15447) ([ktsaou](https://github.com/ktsaou))
- collector meta: remove meta.alternative\_monitored\_instances [\#15445](https://github.com/netdata/netdata/pull/15445) ([ilyam8](https://github.com/ilyam8))
- added missing fields to alerts instances [\#15442](https://github.com/netdata/netdata/pull/15442) ([ktsaou](https://github.com/ktsaou))
- removed dup categories [\#15440](https://github.com/netdata/netdata/pull/15440) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Create netdata-assistant docs [\#15438](https://github.com/netdata/netdata/pull/15438) ([shyamvalsan](https://github.com/shyamvalsan))
- apps.plugin fds limits improvements [\#15437](https://github.com/netdata/netdata/pull/15437) ([ktsaou](https://github.com/ktsaou))
- disable apps\_group\_file\_descriptors\_utilization alarm [\#15435](https://github.com/netdata/netdata/pull/15435) ([ilyam8](https://github.com/ilyam8))
- Add catch-all category entry in categories.yaml [\#15434](https://github.com/netdata/netdata/pull/15434) ([Ancairon](https://github.com/Ancairon))
- Update CODEOWNERS [\#15433](https://github.com/netdata/netdata/pull/15433) ([andrewm4894](https://github.com/andrewm4894))
- Remove duplicate category from categories.yaml [\#15432](https://github.com/netdata/netdata/pull/15432) ([Ancairon](https://github.com/Ancairon))
- readme: add link for netdata cloud and sign-in cta [\#15431](https://github.com/netdata/netdata/pull/15431) ([andrewm4894](https://github.com/andrewm4894))
- add chart id and name to alert instances and transitions [\#15430](https://github.com/netdata/netdata/pull/15430) ([ktsaou](https://github.com/ktsaou))
- update v2 dashboard [\#15427](https://github.com/netdata/netdata/pull/15427) ([ilyam8](https://github.com/ilyam8))
- fix unlocked registry access and add hostname to search response [\#15426](https://github.com/netdata/netdata/pull/15426) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#15424](https://github.com/netdata/netdata/pull/15424) ([christophidesp](https://github.com/christophidesp))
- Decode url before checking for question mark [\#15422](https://github.com/netdata/netdata/pull/15422) ([MrZammler](https://github.com/MrZammler))
- use real-time clock for http response headers [\#15421](https://github.com/netdata/netdata/pull/15421) ([ktsaou](https://github.com/ktsaou))
- Bugfix on alerts generation for yamls [\#15420](https://github.com/netdata/netdata/pull/15420) ([Ancairon](https://github.com/Ancairon))
- Minor typo fix on consul.conf [\#15419](https://github.com/netdata/netdata/pull/15419) ([Ancairon](https://github.com/Ancairon))
- monitor applications file descriptor limits [\#15417](https://github.com/netdata/netdata/pull/15417) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#15416](https://github.com/netdata/netdata/pull/15416) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#15414](https://github.com/netdata/netdata/pull/15414) ([ktsaou](https://github.com/ktsaou))
- collector meta: restrict chart\_type to known values [\#15413](https://github.com/netdata/netdata/pull/15413) ([ilyam8](https://github.com/ilyam8))
- Update README.md [\#15412](https://github.com/netdata/netdata/pull/15412) ([tkatsoulas](https://github.com/tkatsoulas))
- add reference to cncf [\#15408](https://github.com/netdata/netdata/pull/15408) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Make skipped CI run even faster. [\#15407](https://github.com/netdata/netdata/pull/15407) ([Ferroin](https://github.com/Ferroin))
- Pre release fixes [\#15405](https://github.com/netdata/netdata/pull/15405) ([ktsaou](https://github.com/ktsaou))
- Hide eBPF functions [\#15404](https://github.com/netdata/netdata/pull/15404) ([thiagoftsm](https://github.com/thiagoftsm))
- remove collector meta definitions [\#15403](https://github.com/netdata/netdata/pull/15403) ([ilyam8](https://github.com/ilyam8))
- bump v2 dashboard to latest prod [\#15402](https://github.com/netdata/netdata/pull/15402) ([ilyam8](https://github.com/ilyam8))
- Make yamls pass the schema, and use decided temporary naming scheme [\#15401](https://github.com/netdata/netdata/pull/15401) ([Ancairon](https://github.com/Ancairon))
- collector meta schema: global config examples folding + per example [\#15398](https://github.com/netdata/netdata/pull/15398) ([ilyam8](https://github.com/ilyam8))
- packaging: fix arch detection in update\_static [\#15396](https://github.com/netdata/netdata/pull/15396) ([ilyam8](https://github.com/ilyam8))
- add expiration to bearer token response [\#15392](https://github.com/netdata/netdata/pull/15392) ([ktsaou](https://github.com/ktsaou))
- dont add all nodes to registry action hello [\#15390](https://github.com/netdata/netdata/pull/15390) ([ktsaou](https://github.com/ktsaou))
- Revert "dont add all nodes to registry action hello" [\#15389](https://github.com/netdata/netdata/pull/15389) ([ktsaou](https://github.com/ktsaou))
- dont add all nodes to registry action hello [\#15388](https://github.com/netdata/netdata/pull/15388) ([ktsaou](https://github.com/ktsaou))
- update bundled v2 dashboard; make v2 the default dashboard [\#15386](https://github.com/netdata/netdata/pull/15386) ([ilyam8](https://github.com/ilyam8))
- Create categories.yaml [\#15385](https://github.com/netdata/netdata/pull/15385) ([Ancairon](https://github.com/Ancairon))
- Fix CodeQL alert  [\#15384](https://github.com/netdata/netdata/pull/15384) ([stelfrag](https://github.com/stelfrag))
- Add missing files to web/gui/Makefile.am. [\#15383](https://github.com/netdata/netdata/pull/15383) ([Ferroin](https://github.com/Ferroin))
- Updates on JSON schemas [\#15382](https://github.com/netdata/netdata/pull/15382) ([Ancairon](https://github.com/Ancairon))
- Build optimizations [\#15381](https://github.com/netdata/netdata/pull/15381) ([tkatsoulas](https://github.com/tkatsoulas))
- update http response code descriptions [\#15379](https://github.com/netdata/netdata/pull/15379) ([ktsaou](https://github.com/ktsaou))
- Suppress H2O compilation warnings [\#15378](https://github.com/netdata/netdata/pull/15378) ([stelfrag](https://github.com/stelfrag))
- update bundled v2 dashboard [\#15377](https://github.com/netdata/netdata/pull/15377) ([ilyam8](https://github.com/ilyam8))
- health: fix windows alarms for vnodes [\#15376](https://github.com/netdata/netdata/pull/15376) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#15375](https://github.com/netdata/netdata/pull/15375) ([stelfrag](https://github.com/stelfrag))
- Update bundled v2 dashboard. [\#15374](https://github.com/netdata/netdata/pull/15374) ([Ferroin](https://github.com/Ferroin))
- Update libbpf version \(1.2.2\) [\#15373](https://github.com/netdata/netdata/pull/15373) ([thiagoftsm](https://github.com/thiagoftsm))
- simplify collector schema by moving some props under meta [\#15372](https://github.com/netdata/netdata/pull/15372) ([ilyam8](https://github.com/ilyam8))
- dont log error on opening .environment [\#15371](https://github.com/netdata/netdata/pull/15371) ([ilyam8](https://github.com/ilyam8))
- Add most-popular entry in oneOf of categories in definitions.json [\#15370](https://github.com/netdata/netdata/pull/15370) ([Ancairon](https://github.com/Ancairon))
- Rename log\_access and log\_health [\#15368](https://github.com/netdata/netdata/pull/15368) ([MrZammler](https://github.com/MrZammler))
- move not really related props single-module.json -\> definitions.json [\#15366](https://github.com/netdata/netdata/pull/15366) ([ilyam8](https://github.com/ilyam8))
- Add keys to integrations schema, categories, icon path, plus some fixes [\#15365](https://github.com/netdata/netdata/pull/15365) ([Ancairon](https://github.com/Ancairon))
- format the sdr cache filenames [\#15361](https://github.com/netdata/netdata/pull/15361) ([ktsaou](https://github.com/ktsaou))
- fix\(freeipmi\): set sensor state on every reading [\#15360](https://github.com/netdata/netdata/pull/15360) ([ilyam8](https://github.com/ilyam8))
- documentation update for the release of the new UI [\#15359](https://github.com/netdata/netdata/pull/15359) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Rename multi module yamls to same name but wuth prefix [\#15356](https://github.com/netdata/netdata/pull/15356) ([Ancairon](https://github.com/Ancairon))
- Update dashboard to version v3.0.1. [\#15352](https://github.com/netdata/netdata/pull/15352) ([netdatabot](https://github.com/netdatabot))
- Fix installation type command [\#15351](https://github.com/netdata/netdata/pull/15351) ([hugovalente-pm](https://github.com/hugovalente-pm))
- agent alert notifications redirect [\#15350](https://github.com/netdata/netdata/pull/15350) ([ktsaou](https://github.com/ktsaou))
- bearer protection - additions [\#15349](https://github.com/netdata/netdata/pull/15349) ([ktsaou](https://github.com/ktsaou))
- health: fix evaluating expression with `nan` [\#15348](https://github.com/netdata/netdata/pull/15348) ([ilyam8](https://github.com/ilyam8))
- add missing labels to freeipmi metrics csv [\#15347](https://github.com/netdata/netdata/pull/15347) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#15345](https://github.com/netdata/netdata/pull/15345) ([stelfrag](https://github.com/stelfrag))
- Update libbpf on netdata repo [\#15343](https://github.com/netdata/netdata/pull/15343) ([thiagoftsm](https://github.com/thiagoftsm))
- bearer improvements [\#15342](https://github.com/netdata/netdata/pull/15342) ([ktsaou](https://github.com/ktsaou))
- Attempt to more aggressively skip CI jobs on PRs if those jobs are irrelevant to the PR. [\#15341](https://github.com/netdata/netdata/pull/15341) ([Ferroin](https://github.com/Ferroin))
- Remove availability from required fields on metric level [\#15340](https://github.com/netdata/netdata/pull/15340) ([Ancairon](https://github.com/Ancairon))
- docs: make the default Docker installation provide the full feature set [\#15339](https://github.com/netdata/netdata/pull/15339) ([ilyam8](https://github.com/ilyam8))
- add internal stats metrics csv [\#15337](https://github.com/netdata/netdata/pull/15337) ([ilyam8](https://github.com/ilyam8))
- Add missing required field in schema [\#15335](https://github.com/netdata/netdata/pull/15335) ([Ancairon](https://github.com/Ancairon))
- Fix compilation on BSD [\#15331](https://github.com/netdata/netdata/pull/15331) ([thiagoftsm](https://github.com/thiagoftsm))
- alerts\_transitions outputs hostnames and items statistics [\#15329](https://github.com/netdata/netdata/pull/15329) ([ktsaou](https://github.com/ktsaou))
- Use spinlock in host and chart [\#15328](https://github.com/netdata/netdata/pull/15328) ([stelfrag](https://github.com/stelfrag))
- multi-threaded version of freeipmi.plugin [\#15327](https://github.com/netdata/netdata/pull/15327) ([ktsaou](https://github.com/ktsaou))
- Single module schema, add required properties [\#15326](https://github.com/netdata/netdata/pull/15326) ([Ancairon](https://github.com/Ancairon))
- Fix coverity issue 394862 - Argument cannot be negative [\#15324](https://github.com/netdata/netdata/pull/15324) ([stelfrag](https://github.com/stelfrag))
- Rename log Macros \(debug\) [\#15322](https://github.com/netdata/netdata/pull/15322) ([thiagoftsm](https://github.com/thiagoftsm))
- bearer authorization API [\#15321](https://github.com/netdata/netdata/pull/15321) ([ktsaou](https://github.com/ktsaou))
- local-listeners: use host prefix in read\_cmdline [\#15320](https://github.com/netdata/netdata/pull/15320) ([ilyam8](https://github.com/ilyam8))
- local-listener using libnetdata [\#15319](https://github.com/netdata/netdata/pull/15319) ([ktsaou](https://github.com/ktsaou))
- avoid memory allocations for alert transitions facets processing [\#15318](https://github.com/netdata/netdata/pull/15318) ([ktsaou](https://github.com/ktsaou))
- add add summary linking to alert instances \(ati\) when options=summary,values is requested [\#15317](https://github.com/netdata/netdata/pull/15317) ([ktsaou](https://github.com/ktsaou))
- fix alerts transitions sorting [\#15315](https://github.com/netdata/netdata/pull/15315) ([ktsaou](https://github.com/ktsaou))
- Keep health log history in seconds [\#15314](https://github.com/netdata/netdata/pull/15314) ([MrZammler](https://github.com/MrZammler))
- stale vitual hosts [\#15313](https://github.com/netdata/netdata/pull/15313) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin to v0.54.0 [\#15312](https://github.com/netdata/netdata/pull/15312) ([ilyam8](https://github.com/ilyam8))
- health: respect overriding nc binary for IRC notifications [\#15310](https://github.com/netdata/netdata/pull/15310) ([ilyam8](https://github.com/ilyam8))
- hide not available for viewers charts when exporting in shell format [\#15309](https://github.com/netdata/netdata/pull/15309) ([ilyam8](https://github.com/ilyam8))
- move collectors meta to metadata/ [\#15308](https://github.com/netdata/netdata/pull/15308) ([ilyam8](https://github.com/ilyam8))
- Release acquired dimensions [\#15307](https://github.com/netdata/netdata/pull/15307) ([stelfrag](https://github.com/stelfrag))
- Check for source field when requesting /api/v1/alarm\_log [\#15306](https://github.com/netdata/netdata/pull/15306) ([MrZammler](https://github.com/MrZammler))
- ci: disable clang format [\#15305](https://github.com/netdata/netdata/pull/15305) ([ilyam8](https://github.com/ilyam8))
- Change info to netdata\_log\_info in sqlite\_db\_migration.c [\#15303](https://github.com/netdata/netdata/pull/15303) ([MrZammler](https://github.com/MrZammler))
- Create integrations JSON schema [\#15302](https://github.com/netdata/netdata/pull/15302) ([Ancairon](https://github.com/Ancairon))
- Change query to store host system info values [\#15300](https://github.com/netdata/netdata/pull/15300) ([MrZammler](https://github.com/MrZammler))
- s/info/netdata\_log\_info/ [\#15299](https://github.com/netdata/netdata/pull/15299) ([vkalintiris](https://github.com/vkalintiris))
- Fix wording issue in Docker README [\#15298](https://github.com/netdata/netdata/pull/15298) ([Ancairon](https://github.com/Ancairon))
- Update metrics-streaming-and-replication.md [\#15297](https://github.com/netdata/netdata/pull/15297) ([Ancairon](https://github.com/Ancairon))
- Rename generic `error` function [\#15296](https://github.com/netdata/netdata/pull/15296) ([thiagoftsm](https://github.com/thiagoftsm))
- Code reorg and cleanup - enrichment of /api/v2 [\#15294](https://github.com/netdata/netdata/pull/15294) ([ktsaou](https://github.com/ktsaou))
- Optimizations part 3 [\#15293](https://github.com/netdata/netdata/pull/15293) ([ktsaou](https://github.com/ktsaou))
- docs: update stream.conf "health enabled by default" description [\#15291](https://github.com/netdata/netdata/pull/15291) ([ilyam8](https://github.com/ilyam8))
- Remove extra parenthesis from doc [\#15290](https://github.com/netdata/netdata/pull/15290) ([Ancairon](https://github.com/Ancairon))
- merged spaces, war rooms and invite your team to one place [\#15289](https://github.com/netdata/netdata/pull/15289) ([hugovalente-pm](https://github.com/hugovalente-pm))
- use stat\(\) instead of lstat\(\) [\#15287](https://github.com/netdata/netdata/pull/15287) ([ktsaou](https://github.com/ktsaou))
- Only try to enable \_FORTIFY\_SOURCE if the user has not disabled optimizations [\#15284](https://github.com/netdata/netdata/pull/15284) ([Ferroin](https://github.com/Ferroin))
- Send alert chart labels config key to cloud [\#15283](https://github.com/netdata/netdata/pull/15283) ([MrZammler](https://github.com/MrZammler))
- Fixed mistype for 'send automatic labels' Prometheus option [\#15282](https://github.com/netdata/netdata/pull/15282) ([k0ste](https://github.com/k0ste))
- Optimizations part 2 [\#15280](https://github.com/netdata/netdata/pull/15280) ([ktsaou](https://github.com/ktsaou))
- Revert "Optimizations Part 2" [\#15279](https://github.com/netdata/netdata/pull/15279) ([ktsaou](https://github.com/ktsaou))
- exporting: change priority to synchronous when calculating value [\#15276](https://github.com/netdata/netdata/pull/15276) ([ilyam8](https://github.com/ilyam8))
- expose CmdLine in apps function [\#15275](https://github.com/netdata/netdata/pull/15275) ([ilyam8](https://github.com/ilyam8))
- Misc alert fixes [\#15274](https://github.com/netdata/netdata/pull/15274) ([MrZammler](https://github.com/MrZammler))
- Small readme improvements [\#15270](https://github.com/netdata/netdata/pull/15270) ([andrewm4894](https://github.com/andrewm4894))
- Optimizations Part 2 [\#15267](https://github.com/netdata/netdata/pull/15267) ([ktsaou](https://github.com/ktsaou))
- Replace `info` macro with a less generic name [\#15266](https://github.com/netdata/netdata/pull/15266) ([carlocab](https://github.com/carlocab))
- Yaml template finalization [\#15265](https://github.com/netdata/netdata/pull/15265) ([Ancairon](https://github.com/Ancairon))
- fix tc.plugin charts labels [\#15262](https://github.com/netdata/netdata/pull/15262) ([ilyam8](https://github.com/ilyam8))
- Update libbpf version [\#15258](https://github.com/netdata/netdata/pull/15258) ([thiagoftsm](https://github.com/thiagoftsm))
- rewrite /api/v2/alerts [\#15257](https://github.com/netdata/netdata/pull/15257) ([ktsaou](https://github.com/ktsaou))
- Fix $\(libh2o\_dir\) not expanded properly sometimes. [\#15253](https://github.com/netdata/netdata/pull/15253) ([Dim-P](https://github.com/Dim-P))
- use gperf for the pluginsd/streaming parser hashtable [\#15251](https://github.com/netdata/netdata/pull/15251) ([ktsaou](https://github.com/ktsaou))
- Update pfsense.md package install instructions [\#15250](https://github.com/netdata/netdata/pull/15250) ([MYanello](https://github.com/MYanello))
- URL rewrite at the agent web server to support multiple dashboard versions [\#15247](https://github.com/netdata/netdata/pull/15247) ([ktsaou](https://github.com/ktsaou))
- delay collecting virtual network interfaces [\#15244](https://github.com/netdata/netdata/pull/15244) ([ilyam8](https://github.com/ilyam8))
- Assorted kickstart script improvements. [\#15243](https://github.com/netdata/netdata/pull/15243) ([Ferroin](https://github.com/Ferroin))
- Install the correct systemd unit file on older RPM systems. [\#15240](https://github.com/netdata/netdata/pull/15240) ([Ferroin](https://github.com/Ferroin))
- Add yaml metadata for metrics.csv files [\#15238](https://github.com/netdata/netdata/pull/15238) ([Ancairon](https://github.com/Ancairon))
- Add module column to apps.plugin csv [\#15235](https://github.com/netdata/netdata/pull/15235) ([Ancairon](https://github.com/Ancairon))
- Fix coverity 393183 & 393182 [\#15234](https://github.com/netdata/netdata/pull/15234) ([MrZammler](https://github.com/MrZammler))
- Create index for health log migration [\#15233](https://github.com/netdata/netdata/pull/15233) ([stelfrag](https://github.com/stelfrag))
- New alerts endpoint [\#15232](https://github.com/netdata/netdata/pull/15232) ([stelfrag](https://github.com/stelfrag))
- fix not handling N/A value in python.d/nvidia\_smi [\#15231](https://github.com/netdata/netdata/pull/15231) ([ilyam8](https://github.com/ilyam8))
- Fix handling of plugin ownership in static builds. [\#15230](https://github.com/netdata/netdata/pull/15230) ([Ferroin](https://github.com/Ferroin))
- /api/v2 improvements [\#15227](https://github.com/netdata/netdata/pull/15227) ([ktsaou](https://github.com/ktsaou))
- Remove erroneous space for unit [\#15226](https://github.com/netdata/netdata/pull/15226) ([ralphm](https://github.com/ralphm))
- Relax jnfv2 caching [\#15224](https://github.com/netdata/netdata/pull/15224) ([ktsaou](https://github.com/ktsaou))
- Fix /api/v2/contexts,nodes,nodes\_instances,q before match [\#15223](https://github.com/netdata/netdata/pull/15223) ([ktsaou](https://github.com/ktsaou))
- Fix SSL non-blocking retry handling in the web server [\#15222](https://github.com/netdata/netdata/pull/15222) ([ktsaou](https://github.com/ktsaou))
- Update dashboard to version v3.0.0. [\#15219](https://github.com/netdata/netdata/pull/15219) ([netdatabot](https://github.com/netdatabot))
- fix arch detection on i386 \(native packages\) [\#15218](https://github.com/netdata/netdata/pull/15218) ([ilyam8](https://github.com/ilyam8))
- RW\_SPINLOCK: recursive readers support [\#15217](https://github.com/netdata/netdata/pull/15217) ([ktsaou](https://github.com/ktsaou))
- cgroups: remove pod\_uid and container\_id labels in k8s [\#15216](https://github.com/netdata/netdata/pull/15216) ([ilyam8](https://github.com/ilyam8))
- Allow overriding pipename from env [\#15215](https://github.com/netdata/netdata/pull/15215) ([vkalintiris](https://github.com/vkalintiris))
- eBPF Functions \(enable/disable threads\) [\#15214](https://github.com/netdata/netdata/pull/15214) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix health crash [\#15209](https://github.com/netdata/netdata/pull/15209) ([stelfrag](https://github.com/stelfrag))
- Fix file permissions under directory [\#15208](https://github.com/netdata/netdata/pull/15208) ([stelfrag](https://github.com/stelfrag))
- RocketChat cloud integration docs [\#15205](https://github.com/netdata/netdata/pull/15205) ([car12o](https://github.com/car12o))
- Obvious memory reductions [\#15204](https://github.com/netdata/netdata/pull/15204) ([ktsaou](https://github.com/ktsaou))
- Agent dashboard reorganization. [\#15200](https://github.com/netdata/netdata/pull/15200) ([Ferroin](https://github.com/Ferroin))
- sqlite\_health.c: remove `uuid.h` include [\#15195](https://github.com/netdata/netdata/pull/15195) ([nandahkrishna](https://github.com/nandahkrishna))
- RPM: Added elfutils-libelf-devel for build with eBPF \(again\) [\#15192](https://github.com/netdata/netdata/pull/15192) ([k0ste](https://github.com/k0ste))
- Speed up eBPF exit before to bring functions [\#15187](https://github.com/netdata/netdata/pull/15187) ([thiagoftsm](https://github.com/thiagoftsm))
- Add two functions that allow someone to start/stop ML. [\#15185](https://github.com/netdata/netdata/pull/15185) ([vkalintiris](https://github.com/vkalintiris))
- Fix issues in sync thread \(eBPF plugin\) [\#15174](https://github.com/netdata/netdata/pull/15174) ([thiagoftsm](https://github.com/thiagoftsm))
- /api/v2/nodes and streaming function [\#15168](https://github.com/netdata/netdata/pull/15168) ([ktsaou](https://github.com/ktsaou))
- Use a single health log table [\#15157](https://github.com/netdata/netdata/pull/15157) ([MrZammler](https://github.com/MrZammler))
- Add configuration file for netdata-updater.sh. [\#15149](https://github.com/netdata/netdata/pull/15149) ([Ferroin](https://github.com/Ferroin))
- Redirect to index.html when a file is not found by web server [\#15143](https://github.com/netdata/netdata/pull/15143) ([MrZammler](https://github.com/MrZammler))
- fix\(alerting\): removing some of criticals [\#15124](https://github.com/netdata/netdata/pull/15124) ([M4itee](https://github.com/M4itee))
- Add hardening options to CFLAGS by default if they are available. [\#15087](https://github.com/netdata/netdata/pull/15087) ([Ferroin](https://github.com/Ferroin))
- Additional CO-RE code \(eBPF.plugin\) [\#15078](https://github.com/netdata/netdata/pull/15078) ([thiagoftsm](https://github.com/thiagoftsm))
- Update README.md [\#15044](https://github.com/netdata/netdata/pull/15044) ([ktsaou](https://github.com/ktsaou))
- Consistently start the agent as root and rely on it to drop privileges properly. [\#14890](https://github.com/netdata/netdata/pull/14890) ([Ferroin](https://github.com/Ferroin))

## [v1.40.1](https://github.com/netdata/netdata/tree/v1.40.1) (2023-06-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.40.0...v1.40.1)

## [v1.40.0](https://github.com/netdata/netdata/tree/v1.40.0) (2023-06-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.39.1...v1.40.0)

**Merged pull requests:**

- ebpf: disable sync by default [\#15190](https://github.com/netdata/netdata/pull/15190) ([ilyam8](https://github.com/ilyam8))
- Add support for SUSE 15.5 [\#15189](https://github.com/netdata/netdata/pull/15189) ([tkatsoulas](https://github.com/tkatsoulas))
- bump go.d.plugin to v0.53.2 [\#15184](https://github.com/netdata/netdata/pull/15184) ([ilyam8](https://github.com/ilyam8))
- Do strdupz on empty string [\#15183](https://github.com/netdata/netdata/pull/15183) ([MrZammler](https://github.com/MrZammler))
- set setuid for go.d.plugin in container [\#15180](https://github.com/netdata/netdata/pull/15180) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.53.1 [\#15179](https://github.com/netdata/netdata/pull/15179) ([ilyam8](https://github.com/ilyam8))
- Update smartd\_log.conf [\#15171](https://github.com/netdata/netdata/pull/15171) ([TougeAI](https://github.com/TougeAI))
- Change package conflicts policy on deb based packages [\#15170](https://github.com/netdata/netdata/pull/15170) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix coverity issues [\#15169](https://github.com/netdata/netdata/pull/15169) ([stelfrag](https://github.com/stelfrag))
- Fix user and group handling in DEB packages. [\#15166](https://github.com/netdata/netdata/pull/15166) ([Ferroin](https://github.com/Ferroin))
- change mandatory packages for RPMs [\#15165](https://github.com/netdata/netdata/pull/15165) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix CID 385073 -- Uninitialized scalar variable [\#15163](https://github.com/netdata/netdata/pull/15163) ([stelfrag](https://github.com/stelfrag))
- api v2 nodes for streaming statuses [\#15162](https://github.com/netdata/netdata/pull/15162) ([ktsaou](https://github.com/ktsaou))
- Restrict ebpf dep in DEB package to amd64 only. [\#15161](https://github.com/netdata/netdata/pull/15161) ([Ferroin](https://github.com/Ferroin))
- Make plugin packages hard dependencies. [\#15160](https://github.com/netdata/netdata/pull/15160) ([Ferroin](https://github.com/Ferroin))
- freeipmi: add availability status chart and alarm [\#15151](https://github.com/netdata/netdata/pull/15151) ([ilyam8](https://github.com/ilyam8))
- Check null transition id and config hash [\#15147](https://github.com/netdata/netdata/pull/15147) ([stelfrag](https://github.com/stelfrag))
- eBPF unittest + bug fix [\#15146](https://github.com/netdata/netdata/pull/15146) ([thiagoftsm](https://github.com/thiagoftsm))
- Mattermost cloud integration docs [\#15141](https://github.com/netdata/netdata/pull/15141) ([car12o](https://github.com/car12o))
- send EXIT before exiting in freeipmi and debugfs plugins [\#15140](https://github.com/netdata/netdata/pull/15140) ([ilyam8](https://github.com/ilyam8))
- minor - fix syntax in config.ac [\#15139](https://github.com/netdata/netdata/pull/15139) ([underhood](https://github.com/underhood))
- fix a typo in `libnetdata/simple_pattern/README.md` [\#15135](https://github.com/netdata/netdata/pull/15135) ([n0099](https://github.com/n0099))
- updated events docs and minor fix on silecing rules table [\#15134](https://github.com/netdata/netdata/pull/15134) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Provide necessary permission for the kickstart to run the netdata-updater script [\#15132](https://github.com/netdata/netdata/pull/15132) ([tkatsoulas](https://github.com/tkatsoulas))
- fix: allow square brackets in label value [\#15131](https://github.com/netdata/netdata/pull/15131) ([ilyam8](https://github.com/ilyam8))
- Add library to encode/decode Gorilla compressed buffers. [\#15128](https://github.com/netdata/netdata/pull/15128) ([vkalintiris](https://github.com/vkalintiris))
- Fix bundling of eBPF legacy code for DEB packages. [\#15127](https://github.com/netdata/netdata/pull/15127) ([Ferroin](https://github.com/Ferroin))
- Percentage of group aggregatable at cloud - fixed for backwards compatibility [\#15126](https://github.com/netdata/netdata/pull/15126) ([ktsaou](https://github.com/ktsaou))
- Fix package versioning issues. [\#15125](https://github.com/netdata/netdata/pull/15125) ([Ferroin](https://github.com/Ferroin))
- Revert "percentage of group is now aggregatable at cloud across multiple nodes" [\#15122](https://github.com/netdata/netdata/pull/15122) ([ktsaou](https://github.com/ktsaou))
- add netdata demo rooms to the list of demo urls [\#15120](https://github.com/netdata/netdata/pull/15120) ([andrewm4894](https://github.com/andrewm4894))
- Fix handling of eBPF plugin for DEB packages. [\#15117](https://github.com/netdata/netdata/pull/15117) ([Ferroin](https://github.com/Ferroin))
- Re-write of SSL support in Netdata; restoration of SIGCHLD; detection of stale plugins; streaming improvements [\#15113](https://github.com/netdata/netdata/pull/15113) ([ktsaou](https://github.com/ktsaou))
- initial draft for the silencing docs [\#15112](https://github.com/netdata/netdata/pull/15112) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Generate, store and transmit a unique alert event\_hash\_id [\#15111](https://github.com/netdata/netdata/pull/15111) ([MrZammler](https://github.com/MrZammler))
- Only queue an alert to the cloud when it's inserted [\#15110](https://github.com/netdata/netdata/pull/15110) ([MrZammler](https://github.com/MrZammler))
- percentage of group is now aggregatable at cloud across multiple nodes [\#15109](https://github.com/netdata/netdata/pull/15109) ([ktsaou](https://github.com/ktsaou))
- percentage-of-group: fix uninitialized array vh [\#15106](https://github.com/netdata/netdata/pull/15106) ([ktsaou](https://github.com/ktsaou))
- fix the units when returning percentage of a group [\#15105](https://github.com/netdata/netdata/pull/15105) ([ktsaou](https://github.com/ktsaou))
- oracledb: make conn protocol configurable [\#15104](https://github.com/netdata/netdata/pull/15104) ([ilyam8](https://github.com/ilyam8))
- /api/v2/data percentage calculation on grouped queries [\#15100](https://github.com/netdata/netdata/pull/15100) ([ktsaou](https://github.com/ktsaou))
- Add chart labels to Prometheus. [\#15099](https://github.com/netdata/netdata/pull/15099) ([thiagoftsm](https://github.com/thiagoftsm))
- Invert order in remote write [\#15097](https://github.com/netdata/netdata/pull/15097) ([thiagoftsm](https://github.com/thiagoftsm))
- fix cockroachdb alarms [\#15095](https://github.com/netdata/netdata/pull/15095) ([ilyam8](https://github.com/ilyam8))
- Address issue with Thanos Receiver [\#15094](https://github.com/netdata/netdata/pull/15094) ([thiagoftsm](https://github.com/thiagoftsm))
- update ml defaults to 24h [\#15093](https://github.com/netdata/netdata/pull/15093) ([andrewm4894](https://github.com/andrewm4894))
- Create category overview pages for learn's restructure [\#15091](https://github.com/netdata/netdata/pull/15091) ([Ancairon](https://github.com/Ancairon))
- Release buffer in case of error -- CID 385075 [\#15090](https://github.com/netdata/netdata/pull/15090) ([stelfrag](https://github.com/stelfrag))
- health: remove "families" from alarms config [\#15086](https://github.com/netdata/netdata/pull/15086) ([ilyam8](https://github.com/ilyam8))
- update agent telemetry url to be cloud function instead of posthog [\#15085](https://github.com/netdata/netdata/pull/15085) ([andrewm4894](https://github.com/andrewm4894))
- mentioned waive off of space subscription price [\#15082](https://github.com/netdata/netdata/pull/15082) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Python Dependency Migration - OracleDB Python Module [\#15074](https://github.com/netdata/netdata/pull/15074) ([EricAndrechek](https://github.com/EricAndrechek))
- Free context when establishing ACLK connection [\#15073](https://github.com/netdata/netdata/pull/15073) ([stelfrag](https://github.com/stelfrag))
- Update Security doc [\#15072](https://github.com/netdata/netdata/pull/15072) ([tkatsoulas](https://github.com/tkatsoulas))
- Update netdata-security.md [\#15068](https://github.com/netdata/netdata/pull/15068) ([cakrit](https://github.com/cakrit))
- Update netdata-security.md [\#15067](https://github.com/netdata/netdata/pull/15067) ([cakrit](https://github.com/cakrit))
- Simplify loop in alert checkpoint [\#15065](https://github.com/netdata/netdata/pull/15065) ([MrZammler](https://github.com/MrZammler))
- Update CODEOWNERS [\#15064](https://github.com/netdata/netdata/pull/15064) ([cakrit](https://github.com/cakrit))
- Update netdata-security.md [\#15063](https://github.com/netdata/netdata/pull/15063) ([sashwathn](https://github.com/sashwathn))
- Fix CodeQL warning  [\#15062](https://github.com/netdata/netdata/pull/15062) ([stelfrag](https://github.com/stelfrag))
- Improve some of the error messages in the kickstart script. [\#15061](https://github.com/netdata/netdata/pull/15061) ([Ferroin](https://github.com/Ferroin))
- Fix memory leak when sending alerts checkoint [\#15060](https://github.com/netdata/netdata/pull/15060) ([stelfrag](https://github.com/stelfrag))
- bump go.d.plugin to v0.53.0 [\#15059](https://github.com/netdata/netdata/pull/15059) ([ilyam8](https://github.com/ilyam8))
- Fix ACLK memleak [\#15055](https://github.com/netdata/netdata/pull/15055) ([underhood](https://github.com/underhood))
- fix\(debugfs/zswap\): don't collect metrics if Zswap is disabled [\#15054](https://github.com/netdata/netdata/pull/15054) ([ilyam8](https://github.com/ilyam8))
- Comment out default `role_recipients_*` values [\#15047](https://github.com/netdata/netdata/pull/15047) ([jamgregory](https://github.com/jamgregory))
- Small update ml defaults [\#15046](https://github.com/netdata/netdata/pull/15046) ([andrewm4894](https://github.com/andrewm4894))
- Better cleanup of health log table [\#15045](https://github.com/netdata/netdata/pull/15045) ([MrZammler](https://github.com/MrZammler))
- Fix handling of permissions in static installs. [\#15042](https://github.com/netdata/netdata/pull/15042) ([Ferroin](https://github.com/Ferroin))
- Update tor.chart.py [\#15041](https://github.com/netdata/netdata/pull/15041) ([jmphilippe](https://github.com/jmphilippe))
- Wording fix in interact with charts doc [\#15040](https://github.com/netdata/netdata/pull/15040) ([Ancairon](https://github.com/Ancairon))
- fatal in claim\(\) only if --claim-only is used [\#15039](https://github.com/netdata/netdata/pull/15039) ([ilyam8](https://github.com/ilyam8))
- Update libbpf [\#15038](https://github.com/netdata/netdata/pull/15038) ([thiagoftsm](https://github.com/thiagoftsm))
- Slight wording fix on the database readme [\#15034](https://github.com/netdata/netdata/pull/15034) ([Ancairon](https://github.com/Ancairon))
- Update SQLITE to version 3.41.2 [\#15031](https://github.com/netdata/netdata/pull/15031) ([stelfrag](https://github.com/stelfrag))
- Update troubleshooting-agent-with-cloud-connection.md [\#15029](https://github.com/netdata/netdata/pull/15029) ([cakrit](https://github.com/cakrit))
- Adjust buffers to prevent overflow [\#15025](https://github.com/netdata/netdata/pull/15025) ([stelfrag](https://github.com/stelfrag))
- Reduce netdatacli size [\#15024](https://github.com/netdata/netdata/pull/15024) ([stelfrag](https://github.com/stelfrag))
- Debugfs collector [\#15017](https://github.com/netdata/netdata/pull/15017) ([thiagoftsm](https://github.com/thiagoftsm))
- review the billing docs for the flow [\#15014](https://github.com/netdata/netdata/pull/15014) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Rollback ML transaction on failure. [\#15013](https://github.com/netdata/netdata/pull/15013) ([vkalintiris](https://github.com/vkalintiris))
- Silence dimensions with noisy ML models [\#15011](https://github.com/netdata/netdata/pull/15011) ([vkalintiris](https://github.com/vkalintiris))
- Update chart documentation [\#15010](https://github.com/netdata/netdata/pull/15010) ([Ancairon](https://github.com/Ancairon))
- Honor maximum message size limit of MQTT server [\#15009](https://github.com/netdata/netdata/pull/15009) ([underhood](https://github.com/underhood))
- libjudy: remove JudyLTablesGen [\#14984](https://github.com/netdata/netdata/pull/14984) ([mochaaP](https://github.com/mochaaP))
- Use chart labels to filter alerts [\#14982](https://github.com/netdata/netdata/pull/14982) ([MrZammler](https://github.com/MrZammler))
- Remove Fedora 36 from CI and platform support. [\#14938](https://github.com/netdata/netdata/pull/14938) ([Ferroin](https://github.com/Ferroin))
- make zlib compulsory dep [\#14928](https://github.com/netdata/netdata/pull/14928) ([underhood](https://github.com/underhood))

## [v1.39.1](https://github.com/netdata/netdata/tree/v1.39.1) (2023-05-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.39.0...v1.39.1)

## [v1.39.0](https://github.com/netdata/netdata/tree/v1.39.0) (2023-05-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.38.1...v1.39.0)

**Merged pull requests:**

- Fix typo in file capabilities settings in static installer. [\#15023](https://github.com/netdata/netdata/pull/15023) ([Ferroin](https://github.com/Ferroin))
- On data and weight queries now instances filter matches also instance\_id@node\_id [\#15021](https://github.com/netdata/netdata/pull/15021) ([ktsaou](https://github.com/ktsaou))
- add `grafana` to `apps_groups.conf` [\#15020](https://github.com/netdata/netdata/pull/15020) ([andrewm4894](https://github.com/andrewm4894))
- Set file capabilities correctly on static installs. [\#15018](https://github.com/netdata/netdata/pull/15018) ([Ferroin](https://github.com/Ferroin))
- Differentiate error codes better when claiming from kickstart script. [\#15015](https://github.com/netdata/netdata/pull/15015) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues  [\#15005](https://github.com/netdata/netdata/pull/15005) ([stelfrag](https://github.com/stelfrag))
- weights endpoint: volume diff of anomaly rates [\#15004](https://github.com/netdata/netdata/pull/15004) ([ktsaou](https://github.com/ktsaou))
- Bump Coverity scan tool version. [\#15003](https://github.com/netdata/netdata/pull/15003) ([Ferroin](https://github.com/Ferroin))
- feat\(apps.plugin\): collect context switches  [\#15002](https://github.com/netdata/netdata/pull/15002) ([ilyam8](https://github.com/ilyam8))
- add support for monitoring thp, ballooning, zswap, ksm cow [\#15000](https://github.com/netdata/netdata/pull/15000) ([ktsaou](https://github.com/ktsaou))
- Fix cmake errors [\#14998](https://github.com/netdata/netdata/pull/14998) ([stelfrag](https://github.com/stelfrag))
- Remove lighttpd2 from docs [\#14997](https://github.com/netdata/netdata/pull/14997) ([Ancairon](https://github.com/Ancairon))
- Update netdata-security.md [\#14995](https://github.com/netdata/netdata/pull/14995) ([cakrit](https://github.com/cakrit))
- feat: add OpsGenie alert levels to payload [\#14992](https://github.com/netdata/netdata/pull/14992) ([OliverNChalk](https://github.com/OliverNChalk))
- disable CPU full pressure at the system level [\#14991](https://github.com/netdata/netdata/pull/14991) ([ilyam8](https://github.com/ilyam8))
- fix config generation for plugins [\#14990](https://github.com/netdata/netdata/pull/14990) ([ilyam8](https://github.com/ilyam8))
- Load/Store ML models [\#14981](https://github.com/netdata/netdata/pull/14981) ([vkalintiris](https://github.com/vkalintiris))
- Fix TYPO in README.md [\#14980](https://github.com/netdata/netdata/pull/14980) ([DevinNorgarb](https://github.com/DevinNorgarb))
- add metrics.csv to proc.plugin [\#14979](https://github.com/netdata/netdata/pull/14979) ([thiagoftsm](https://github.com/thiagoftsm))
- interrupt callback on api/v1/data [\#14978](https://github.com/netdata/netdata/pull/14978) ([ktsaou](https://github.com/ktsaou))
- add metrics.csv to macos, freebsd and cgroups plugins [\#14977](https://github.com/netdata/netdata/pull/14977) ([ilyam8](https://github.com/ilyam8))
- fix adding chart labels in tc.plugin [\#14976](https://github.com/netdata/netdata/pull/14976) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to some c collectors [\#14974](https://github.com/netdata/netdata/pull/14974) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to perf.plugin [\#14973](https://github.com/netdata/netdata/pull/14973) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to xenstat.plugin [\#14972](https://github.com/netdata/netdata/pull/14972) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to slabinfo.plugin [\#14971](https://github.com/netdata/netdata/pull/14971) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to timex.plugin [\#14970](https://github.com/netdata/netdata/pull/14970) ([ilyam8](https://github.com/ilyam8))
- do not convert to percentage, when the raw option is given [\#14969](https://github.com/netdata/netdata/pull/14969) ([ktsaou](https://github.com/ktsaou))
- add metrics.csv to apps.plugin [\#14968](https://github.com/netdata/netdata/pull/14968) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to charts.d [\#14966](https://github.com/netdata/netdata/pull/14966) ([ilyam8](https://github.com/ilyam8))
- Add metrics.csv for ebpf [\#14965](https://github.com/netdata/netdata/pull/14965) ([thiagoftsm](https://github.com/thiagoftsm))
- Update ML README.md [\#14964](https://github.com/netdata/netdata/pull/14964) ([ktsaou](https://github.com/ktsaou))
- Document netdatacli dumpconfig option [\#14963](https://github.com/netdata/netdata/pull/14963) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14962](https://github.com/netdata/netdata/pull/14962) ([cakrit](https://github.com/cakrit))
- Fix handling of users and groups on install. [\#14961](https://github.com/netdata/netdata/pull/14961) ([Ferroin](https://github.com/Ferroin))
- Reject child when context is loading [\#14960](https://github.com/netdata/netdata/pull/14960) ([stelfrag](https://github.com/stelfrag))
- Add metadata.csv to python.d.plugin [\#14959](https://github.com/netdata/netdata/pull/14959) ([Ancairon](https://github.com/Ancairon))
- Address log issue [\#14958](https://github.com/netdata/netdata/pull/14958) ([thiagoftsm](https://github.com/thiagoftsm))
- Add adaptec\_raid metrics.csv [\#14955](https://github.com/netdata/netdata/pull/14955) ([Ancairon](https://github.com/Ancairon))
- Set api v2 version 2 [\#14954](https://github.com/netdata/netdata/pull/14954) ([stelfrag](https://github.com/stelfrag))
- Cancel Pending Request [\#14953](https://github.com/netdata/netdata/pull/14953) ([underhood](https://github.com/underhood))
- Terminate JSX element in doc file [\#14952](https://github.com/netdata/netdata/pull/14952) ([Ancairon](https://github.com/Ancairon))
- Update dbengine README.md [\#14951](https://github.com/netdata/netdata/pull/14951) ([ktsaou](https://github.com/ktsaou))
- Prevent pager from preventing non-interactive install [\#14950](https://github.com/netdata/netdata/pull/14950) ([bompus](https://github.com/bompus))
- Update README.md [\#14948](https://github.com/netdata/netdata/pull/14948) ([cakrit](https://github.com/cakrit))
- Disable SQL operations in training thread [\#14947](https://github.com/netdata/netdata/pull/14947) ([vkalintiris](https://github.com/vkalintiris))
- Add support for acquire/release operations on RRDSETs [\#14945](https://github.com/netdata/netdata/pull/14945) ([vkalintiris](https://github.com/vkalintiris))
- fix 32bit segv [\#14940](https://github.com/netdata/netdata/pull/14940) ([ktsaou](https://github.com/ktsaou))
- Update using-host-labels.md [\#14939](https://github.com/netdata/netdata/pull/14939) ([cakrit](https://github.com/cakrit))
- Fix broken image, in database/README.md [\#14936](https://github.com/netdata/netdata/pull/14936) ([Ancairon](https://github.com/Ancairon))
- Add a description to proc.plugin/README.md [\#14935](https://github.com/netdata/netdata/pull/14935) ([Ancairon](https://github.com/Ancairon))
- zfspool: add suspended state [\#14934](https://github.com/netdata/netdata/pull/14934) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin v0.52.2 [\#14933](https://github.com/netdata/netdata/pull/14933) ([ilyam8](https://github.com/ilyam8))
- Make the document more generic [\#14932](https://github.com/netdata/netdata/pull/14932) ([cakrit](https://github.com/cakrit))
- Replace "XYZ view" with "XYZ tab" in documentation files [\#14930](https://github.com/netdata/netdata/pull/14930) ([Ancairon](https://github.com/Ancairon))
- Add windows MSI installer start stop restart instructions to docs [\#14929](https://github.com/netdata/netdata/pull/14929) ([Ancairon](https://github.com/Ancairon))
- Add Docker instructions to enable Nvidia GPUs [\#14924](https://github.com/netdata/netdata/pull/14924) ([D34DC3N73R](https://github.com/D34DC3N73R))
- Initialize machine GUID earlier in the agent startup sequence [\#14922](https://github.com/netdata/netdata/pull/14922) ([stelfrag](https://github.com/stelfrag))
- bump go.d.plugin to v0.52.1 [\#14921](https://github.com/netdata/netdata/pull/14921) ([ilyam8](https://github.com/ilyam8))
- Skip ML initialization when it's been disabled in netdata.conf [\#14920](https://github.com/netdata/netdata/pull/14920) ([vkalintiris](https://github.com/vkalintiris))
- Fix warnings and error when compiling with --disable-dbengine [\#14919](https://github.com/netdata/netdata/pull/14919) ([stelfrag](https://github.com/stelfrag))
- minor - remove RX\_MSGLEN\_MAX [\#14918](https://github.com/netdata/netdata/pull/14918) ([underhood](https://github.com/underhood))
- change docusaurus admonitions to our style of admonitions [\#14917](https://github.com/netdata/netdata/pull/14917) ([Ancairon](https://github.com/Ancairon))
- Add windows diagram [\#14916](https://github.com/netdata/netdata/pull/14916) ([cakrit](https://github.com/cakrit))
- Add section for scaling parent nodes [\#14915](https://github.com/netdata/netdata/pull/14915) ([cakrit](https://github.com/cakrit))
- Update suggested replication setups [\#14914](https://github.com/netdata/netdata/pull/14914) ([cakrit](https://github.com/cakrit))
- Revert ML changes. [\#14908](https://github.com/netdata/netdata/pull/14908) ([vkalintiris](https://github.com/vkalintiris))
- Remove netdatacli response size limitation [\#14906](https://github.com/netdata/netdata/pull/14906) ([stelfrag](https://github.com/stelfrag))
- Update change-metrics-storage.md [\#14905](https://github.com/netdata/netdata/pull/14905) ([cakrit](https://github.com/cakrit))
- /api/v2 part 10 [\#14904](https://github.com/netdata/netdata/pull/14904) ([ktsaou](https://github.com/ktsaou))
- Optimize the cheat sheet to be in a printable form factor  [\#14903](https://github.com/netdata/netdata/pull/14903) ([Ancairon](https://github.com/Ancairon))
- Address issues on `EC2` \(eBPF\). [\#14902](https://github.com/netdata/netdata/pull/14902) ([thiagoftsm](https://github.com/thiagoftsm))
- Update REFERENCE.md [\#14900](https://github.com/netdata/netdata/pull/14900) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14899](https://github.com/netdata/netdata/pull/14899) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14898](https://github.com/netdata/netdata/pull/14898) ([cakrit](https://github.com/cakrit))
- Disable threads while we are investigating [\#14897](https://github.com/netdata/netdata/pull/14897) ([thiagoftsm](https://github.com/thiagoftsm))
- add opsgenie as a business level notificaiton method [\#14895](https://github.com/netdata/netdata/pull/14895) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Remove dry run option from uninstall documentation [\#14894](https://github.com/netdata/netdata/pull/14894) ([Ancairon](https://github.com/Ancairon))
- cgroups: add option to use Kubelet for pods metadata [\#14891](https://github.com/netdata/netdata/pull/14891) ([ilyam8](https://github.com/ilyam8))
- /api/v2 part 9 [\#14888](https://github.com/netdata/netdata/pull/14888) ([ktsaou](https://github.com/ktsaou))
- Add example configuration to w1sensor collector [\#14886](https://github.com/netdata/netdata/pull/14886) ([Ancairon](https://github.com/Ancairon))
- /api/v2 part 8 [\#14885](https://github.com/netdata/netdata/pull/14885) ([ktsaou](https://github.com/ktsaou))
- Fix/introduce links inside charts.d.plugin documentation  [\#14884](https://github.com/netdata/netdata/pull/14884) ([Ancairon](https://github.com/Ancairon))
- Add support for alert notifications to ntfy.sh [\#14875](https://github.com/netdata/netdata/pull/14875) ([Dim-P](https://github.com/Dim-P))
- WEBRTC for communication between agents and browsers [\#14874](https://github.com/netdata/netdata/pull/14874) ([ktsaou](https://github.com/ktsaou))
- Remove alpine 3.14 from the ci [\#14873](https://github.com/netdata/netdata/pull/14873) ([tkatsoulas](https://github.com/tkatsoulas))
- cgroups.plugin: add image label [\#14872](https://github.com/netdata/netdata/pull/14872) ([ilyam8](https://github.com/ilyam8))
- Fix regex syntax for clang-format checks. [\#14871](https://github.com/netdata/netdata/pull/14871) ([Ferroin](https://github.com/Ferroin))
- bump go.d.plugin v0.52.0 [\#14870](https://github.com/netdata/netdata/pull/14870) ([ilyam8](https://github.com/ilyam8))
- eBPF bug fixes [\#14869](https://github.com/netdata/netdata/pull/14869) ([thiagoftsm](https://github.com/thiagoftsm))
- Update link from http to https [\#14864](https://github.com/netdata/netdata/pull/14864) ([Ancairon](https://github.com/Ancairon))
- Fix js tag in documentation [\#14862](https://github.com/netdata/netdata/pull/14862) ([Ancairon](https://github.com/Ancairon))
- Set a default registry unique id when there is none for statistics script [\#14861](https://github.com/netdata/netdata/pull/14861) ([MrZammler](https://github.com/MrZammler))
- review usage of you to say user instead [\#14858](https://github.com/netdata/netdata/pull/14858) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Add labels for cgroup name [\#14856](https://github.com/netdata/netdata/pull/14856) ([thiagoftsm](https://github.com/thiagoftsm))
- fix typo alerms -\> alarms [\#14854](https://github.com/netdata/netdata/pull/14854) ([slavox](https://github.com/slavox))
- Add a checkpoint message to alerts stream [\#14847](https://github.com/netdata/netdata/pull/14847) ([MrZammler](https://github.com/MrZammler))
- fix  \#14841 Exception funktion call Rados.mon\_command\(\) [\#14844](https://github.com/netdata/netdata/pull/14844) ([farax4de](https://github.com/farax4de))
- Update parent child examples [\#14842](https://github.com/netdata/netdata/pull/14842) ([cakrit](https://github.com/cakrit))
- fix double host prefix when reading ZFS pools state [\#14840](https://github.com/netdata/netdata/pull/14840) ([ilyam8](https://github.com/ilyam8))
- Update enable-notifications.md [\#14838](https://github.com/netdata/netdata/pull/14838) ([cakrit](https://github.com/cakrit))

## [v1.38.1](https://github.com/netdata/netdata/tree/v1.38.1) (2023-02-13)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.38.0...v1.38.1)

## [v1.38.0](https://github.com/netdata/netdata/tree/v1.38.0) (2023-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.37.1...v1.38.0)

## [v1.37.1](https://github.com/netdata/netdata/tree/v1.37.1) (2022-12-05)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.37.0...v1.37.1)

## [v1.37.0](https://github.com/netdata/netdata/tree/v1.37.0) (2022-11-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.36.1...v1.37.0)

## [v1.36.1](https://github.com/netdata/netdata/tree/v1.36.1) (2022-08-15)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.36.0...v1.36.1)

## [v1.36.0](https://github.com/netdata/netdata/tree/v1.36.0) (2022-08-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.1...v1.36.0)

## [v1.35.1](https://github.com/netdata/netdata/tree/v1.35.1) (2022-06-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.0...v1.35.1)

## [v1.35.0](https://github.com/netdata/netdata/tree/v1.35.0) (2022-06-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.1...v1.35.0)

## [v1.34.1](https://github.com/netdata/netdata/tree/v1.34.1) (2022-04-15)

[Full Changelog](https://github.com/netdata/netdata/compare/1.34.0...v1.34.1)

## [1.34.0](https://github.com/netdata/netdata/tree/1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.0...1.34.0)

## [v1.34.0](https://github.com/netdata/netdata/tree/v1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.1...v1.34.0)

## [v1.33.1](https://github.com/netdata/netdata/tree/v1.33.1) (2022-02-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.0...v1.33.1)

## [v1.33.0](https://github.com/netdata/netdata/tree/v1.33.0) (2022-01-26)

[Full Changelog](https://github.com/netdata/netdata/compare/1.32.1...v1.33.0)

## [1.32.1](https://github.com/netdata/netdata/tree/1.32.1) (2021-12-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.32.1...1.32.1)

## [v1.32.1](https://github.com/netdata/netdata/tree/v1.32.1) (2021-12-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.32.0...v1.32.1)

## [v1.32.0](https://github.com/netdata/netdata/tree/v1.32.0) (2021-11-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.31.0...v1.32.0)

## [v1.31.0](https://github.com/netdata/netdata/tree/v1.31.0) (2021-05-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.1...v1.31.0)

## [v1.30.1](https://github.com/netdata/netdata/tree/v1.30.1) (2021-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.0...v1.30.1)

## [v1.30.0](https://github.com/netdata/netdata/tree/v1.30.0) (2021-03-31)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.3...v1.30.0)

## [v1.29.3](https://github.com/netdata/netdata/tree/v1.29.3) (2021-02-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.2...v1.29.3)

## [v1.29.2](https://github.com/netdata/netdata/tree/v1.29.2) (2021-02-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.1...v1.29.2)

## [v1.29.1](https://github.com/netdata/netdata/tree/v1.29.1) (2021-02-09)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.0...v1.29.1)

## [v1.29.0](https://github.com/netdata/netdata/tree/v1.29.0) (2021-02-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.27.0_0104103941...v1.29.0)

## [v1.27.0_0104103941](https://github.com/netdata/netdata/tree/v1.27.0_0104103941) (2021-01-04)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.28.0...v1.27.0_0104103941)

## [v1.28.0](https://github.com/netdata/netdata/tree/v1.28.0) (2020-12-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.27.0...v1.28.0)

## [v1.27.0](https://github.com/netdata/netdata/tree/v1.27.0) (2020-12-17)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.26.0...v1.27.0)

## [v1.26.0](https://github.com/netdata/netdata/tree/v1.26.0) (2020-10-14)

[Full Changelog](https://github.com/netdata/netdata/compare/before_rebase...v1.26.0)

## [before_rebase](https://github.com/netdata/netdata/tree/before_rebase) (2020-09-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.25.0...before_rebase)

## [v1.25.0](https://github.com/netdata/netdata/tree/v1.25.0) (2020-09-15)

[Full Changelog](https://github.com/netdata/netdata/compare/poc2...v1.25.0)

## [poc2](https://github.com/netdata/netdata/tree/poc2) (2020-08-25)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.24.0...poc2)

## [v1.24.0](https://github.com/netdata/netdata/tree/v1.24.0) (2020-08-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.2...v1.24.0)

## [v1.23.2](https://github.com/netdata/netdata/tree/v1.23.2) (2020-07-16)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.1_infiniband...v1.23.2)

## [v1.23.1_infiniband](https://github.com/netdata/netdata/tree/v1.23.1_infiniband) (2020-07-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.1...v1.23.1_infiniband)

## [v1.23.1](https://github.com/netdata/netdata/tree/v1.23.1) (2020-07-01)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.0...v1.23.1)

## [v1.23.0](https://github.com/netdata/netdata/tree/v1.23.0) (2020-06-25)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.22.1...v1.23.0)

## [v1.22.1](https://github.com/netdata/netdata/tree/v1.22.1) (2020-05-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.22.0...v1.22.1)

## [v1.22.0](https://github.com/netdata/netdata/tree/v1.22.0) (2020-05-11)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.21.1...v1.22.0)

## [v1.21.1](https://github.com/netdata/netdata/tree/v1.21.1) (2020-04-13)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.21.0...v1.21.1)

## [v1.21.0](https://github.com/netdata/netdata/tree/v1.21.0) (2020-04-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.20.0...v1.21.0)

## [v1.20.0](https://github.com/netdata/netdata/tree/v1.20.0) (2020-02-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.19.0...v1.20.0)

## [v1.19.0](https://github.com/netdata/netdata/tree/v1.19.0) (2019-11-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.18.1...v1.19.0)

## [v1.18.1](https://github.com/netdata/netdata/tree/v1.18.1) (2019-10-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.18.0...v1.18.1)

## [v1.18.0](https://github.com/netdata/netdata/tree/v1.18.0) (2019-10-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.17.1...v1.18.0)

## [v1.17.1](https://github.com/netdata/netdata/tree/v1.17.1) (2019-09-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.17.0...v1.17.1)

## [v1.17.0](https://github.com/netdata/netdata/tree/v1.17.0) (2019-09-03)

[Full Changelog](https://github.com/netdata/netdata/compare/issue_4934...v1.17.0)

## [issue_4934](https://github.com/netdata/netdata/tree/issue_4934) (2019-08-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.16.1...issue_4934)

## [v1.16.1](https://github.com/netdata/netdata/tree/v1.16.1) (2019-07-31)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.16.0...v1.16.1)

## [v1.16.0](https://github.com/netdata/netdata/tree/v1.16.0) (2019-07-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.15.0...v1.16.0)

## [v1.15.0](https://github.com/netdata/netdata/tree/v1.15.0) (2019-05-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.14.0...v1.15.0)

## [v1.14.0](https://github.com/netdata/netdata/tree/v1.14.0) (2019-04-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.14.0-rc0...v1.14.0)

## [v1.14.0-rc0](https://github.com/netdata/netdata/tree/v1.14.0-rc0) (2019-03-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.13.0...v1.14.0-rc0)

## [v1.13.0](https://github.com/netdata/netdata/tree/v1.13.0) (2019-03-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.2...v1.13.0)

## [v1.12.2](https://github.com/netdata/netdata/tree/v1.12.2) (2019-02-28)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.1...v1.12.2)

## [v1.12.1](https://github.com/netdata/netdata/tree/v1.12.1) (2019-02-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.0...v1.12.1)

## [v1.12.0](https://github.com/netdata/netdata/tree/v1.12.0) (2019-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.0-rc3...v1.12.0)

## [v1.12.0-rc3](https://github.com/netdata/netdata/tree/v1.12.0-rc3) (2019-01-17)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.0-rc2...v1.12.0-rc3)

## [v1.12.0-rc2](https://github.com/netdata/netdata/tree/v1.12.0-rc2) (2019-01-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.0-rc1...v1.12.0-rc2)

## [v1.12.0-rc1](https://github.com/netdata/netdata/tree/v1.12.0-rc1) (2018-12-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.12.0-rc0...v1.12.0-rc1)

## [v1.12.0-rc0](https://github.com/netdata/netdata/tree/v1.12.0-rc0) (2018-12-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.11.1...v1.12.0-rc0)

## [v1.11.1](https://github.com/netdata/netdata/tree/v1.11.1) (2018-11-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.11.0...v1.11.1)

## [v1.11.0](https://github.com/netdata/netdata/tree/v1.11.0) (2018-11-02)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.10.0...v1.11.0)



\* *This Changelog was automatically generated by [github_changelog_generator](https://github.com/github-changelog-generator/github-changelog-generator)*
