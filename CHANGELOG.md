# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.0...HEAD)

**Merged pull requests:**

- improvement\(go.d.plugin\): add data collection status chart [\#18981](https://github.com/netdata/netdata/pull/18981) ([ilyam8](https://github.com/ilyam8))
- ci: fix win jobs [\#18979](https://github.com/netdata/netdata/pull/18979) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18977](https://github.com/netdata/netdata/pull/18977) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/rabbitmq\): add queue status and net partitions [\#18976](https://github.com/netdata/netdata/pull/18976) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18973](https://github.com/netdata/netdata/pull/18973) ([netdatabot](https://github.com/netdatabot))
- add rabbitmq alerts [\#18972](https://github.com/netdata/netdata/pull/18972) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18971](https://github.com/netdata/netdata/pull/18971) ([netdatabot](https://github.com/netdatabot))
- fix\(go.d/snmp\): don't return error if no sysName [\#18970](https://github.com/netdata/netdata/pull/18970) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump golang.org/x/text from 0.19.0 to 0.20.0 in /src/go [\#18968](https://github.com/netdata/netdata/pull/18968) ([dependabot[bot]](https://github.com/apps/dependabot))
- go mod tidy [\#18967](https://github.com/netdata/netdata/pull/18967) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18966](https://github.com/netdata/netdata/pull/18966) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/rabbitmq\): add cluster support [\#18965](https://github.com/netdata/netdata/pull/18965) ([ilyam8](https://github.com/ilyam8))
- added /api/v3/stream\_path [\#18943](https://github.com/netdata/netdata/pull/18943) ([ktsaou](https://github.com/ktsaou))
- Update Windows Documentation [\#18928](https://github.com/netdata/netdata/pull/18928) ([thiagoftsm](https://github.com/thiagoftsm))
- Bump github.com/Wing924/ltsv from 0.3.1 to 0.4.0 in /src/go [\#18636](https://github.com/netdata/netdata/pull/18636) ([dependabot[bot]](https://github.com/apps/dependabot))

## [v2.0.0](https://github.com/netdata/netdata/tree/v2.0.0) (2024-11-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.5...v2.0.0)

**Merged pull requests:**

- build\(deps\): update go toolchain to v1.23.3 [\#18961](https://github.com/netdata/netdata/pull/18961) ([ilyam8](https://github.com/ilyam8))
- Adjust max possible extent size [\#18960](https://github.com/netdata/netdata/pull/18960) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/vmware/govmomi from 0.45.1 to 0.46.0 in /src/go [\#18959](https://github.com/netdata/netdata/pull/18959) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore\(go.d.plugin\): remove duplicate logging in init/check [\#18955](https://github.com/netdata/netdata/pull/18955) ([ilyam8](https://github.com/ilyam8))
- Update README.md [\#18954](https://github.com/netdata/netdata/pull/18954) ([Ancairon](https://github.com/Ancairon))
- Fix br elements [\#18952](https://github.com/netdata/netdata/pull/18952) ([Ancairon](https://github.com/Ancairon))
- Precompile Python code on Windows. [\#18951](https://github.com/netdata/netdata/pull/18951) ([Ferroin](https://github.com/Ferroin))
- docs: simplify go.d.plugin readme [\#18949](https://github.com/netdata/netdata/pull/18949) ([ilyam8](https://github.com/ilyam8))
- fix memory leak when using libcurl [\#18947](https://github.com/netdata/netdata/pull/18947) ([ktsaou](https://github.com/ktsaou))
- docs: add "Plugin Privileges" section [\#18946](https://github.com/netdata/netdata/pull/18946) ([ilyam8](https://github.com/ilyam8))
- docs: fix Caddy docker compose example [\#18944](https://github.com/netdata/netdata/pull/18944) ([ilyam8](https://github.com/ilyam8))
- docs: grammar/format fixes to `docs/netdata-agent/` [\#18942](https://github.com/netdata/netdata/pull/18942) ([ilyam8](https://github.com/ilyam8))
- Streaming re-organization [\#18941](https://github.com/netdata/netdata/pull/18941) ([ktsaou](https://github.com/ktsaou))
- random numbers No 3 [\#18940](https://github.com/netdata/netdata/pull/18940) ([ktsaou](https://github.com/ktsaou))
- Random numbers improvements [\#18939](https://github.com/netdata/netdata/pull/18939) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/prometheus\): correct unsupported protocol scheme "file" error [\#18938](https://github.com/netdata/netdata/pull/18938) ([ilyam8](https://github.com/ilyam8))
- Improve ACLK sync CPU usage [\#18935](https://github.com/netdata/netdata/pull/18935) ([stelfrag](https://github.com/stelfrag))
- Hyper collector fixes [\#18934](https://github.com/netdata/netdata/pull/18934) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18932](https://github.com/netdata/netdata/pull/18932) ([netdatabot](https://github.com/netdatabot))
- better randomness for heartbeat [\#18930](https://github.com/netdata/netdata/pull/18930) ([ktsaou](https://github.com/ktsaou))
- add randomness per thread to heartbeat [\#18929](https://github.com/netdata/netdata/pull/18929) ([ktsaou](https://github.com/ktsaou))
- Improve the documentation on removing stale nodes [\#18927](https://github.com/netdata/netdata/pull/18927) ([ralphm](https://github.com/ralphm))
- Docs: Changes to title and CPU requirements [\#18925](https://github.com/netdata/netdata/pull/18925) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/nvidia\_smi\): remove use\_csv\_format \(deprecated\) from config [\#18924](https://github.com/netdata/netdata/pull/18924) ([ilyam8](https://github.com/ilyam8))
- Docs: small fixes and pass on sizing Agents [\#18923](https://github.com/netdata/netdata/pull/18923) ([Ancairon](https://github.com/Ancairon))
- go.d/portcheck: separate tabs for tcp/upd ports [\#18922](https://github.com/netdata/netdata/pull/18922) ([ilyam8](https://github.com/ilyam8))
- Update Libbpf [\#18921](https://github.com/netdata/netdata/pull/18921) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/fsnotify/fsnotify from 1.7.0 to 1.8.0 in /src/go [\#18920](https://github.com/netdata/netdata/pull/18920) ([dependabot[bot]](https://github.com/apps/dependabot))
- log2journal now uses libnetdata [\#18919](https://github.com/netdata/netdata/pull/18919) ([ktsaou](https://github.com/ktsaou))
- docs: fix ui license link [\#18918](https://github.com/netdata/netdata/pull/18918) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18917](https://github.com/netdata/netdata/pull/18917) ([netdatabot](https://github.com/netdatabot))
- Switch DEB/RPM repositories to new subdomain. [\#18916](https://github.com/netdata/netdata/pull/18916) ([Ferroin](https://github.com/Ferroin))
- docs: fix broken links in metadata [\#18915](https://github.com/netdata/netdata/pull/18915) ([ilyam8](https://github.com/ilyam8))
- Update CI to generate MSI installer for Windows using WiX. [\#18914](https://github.com/netdata/netdata/pull/18914) ([Ferroin](https://github.com/Ferroin))
- Fix potential wait forever in mqtt loop [\#18913](https://github.com/netdata/netdata/pull/18913) ([stelfrag](https://github.com/stelfrag))
- add `dagster` to apps\_groups.conf [\#18912](https://github.com/netdata/netdata/pull/18912) ([andrewm4894](https://github.com/andrewm4894))
- Installation section simplification [\#18911](https://github.com/netdata/netdata/pull/18911) ([Ancairon](https://github.com/Ancairon))
- fix\(debugfs/extfrag\): add zone label [\#18910](https://github.com/netdata/netdata/pull/18910) ([ilyam8](https://github.com/ilyam8))
- proc.plugin: log as info if a dir not exists [\#18909](https://github.com/netdata/netdata/pull/18909) ([ilyam8](https://github.com/ilyam8))
- uninstall docs edits [\#18908](https://github.com/netdata/netdata/pull/18908) ([Ancairon](https://github.com/Ancairon))
- Update uninstallation docs and remove reinstallation page [\#18907](https://github.com/netdata/netdata/pull/18907) ([Ancairon](https://github.com/Ancairon))
- Adjust API version [\#18906](https://github.com/netdata/netdata/pull/18906) ([stelfrag](https://github.com/stelfrag))
- Fix a potential invalid double free memory [\#18905](https://github.com/netdata/netdata/pull/18905) ([stelfrag](https://github.com/stelfrag))
- MSI Improvements [\#18903](https://github.com/netdata/netdata/pull/18903) ([thiagoftsm](https://github.com/thiagoftsm))
- versioning for functions [\#18902](https://github.com/netdata/netdata/pull/18902) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18901](https://github.com/netdata/netdata/pull/18901) ([netdatabot](https://github.com/netdatabot))
- chore\(go.d.plugin\): add build tags to modules [\#18900](https://github.com/netdata/netdata/pull/18900) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18899](https://github.com/netdata/netdata/pull/18899) ([netdatabot](https://github.com/netdatabot))
- Updating Netdata docs [\#18898](https://github.com/netdata/netdata/pull/18898) ([Ancairon](https://github.com/Ancairon))
- remove python.d/zscores [\#18897](https://github.com/netdata/netdata/pull/18897) ([ilyam8](https://github.com/ilyam8))
- Coverity fixes [\#18896](https://github.com/netdata/netdata/pull/18896) ([stelfrag](https://github.com/stelfrag))
- docs edit [\#18895](https://github.com/netdata/netdata/pull/18895) ([Ancairon](https://github.com/Ancairon))
- Start-stop-restart for windows, plus move info to its own file [\#18894](https://github.com/netdata/netdata/pull/18894) ([Ancairon](https://github.com/Ancairon))
- log2journal: fix config parsing memory leaks [\#18893](https://github.com/netdata/netdata/pull/18893) ([ktsaou](https://github.com/ktsaou))
- Fix coverity issues [\#18892](https://github.com/netdata/netdata/pull/18892) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18891](https://github.com/netdata/netdata/pull/18891) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d.plugin\): add spigotmc collector [\#18890](https://github.com/netdata/netdata/pull/18890) ([ilyam8](https://github.com/ilyam8))
- remove python.d/spigotmc [\#18889](https://github.com/netdata/netdata/pull/18889) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/k8sstate\): collect pod status reason [\#18887](https://github.com/netdata/netdata/pull/18887) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18886](https://github.com/netdata/netdata/pull/18886) ([netdatabot](https://github.com/netdatabot))
- fix\(go.d/k8sstate\): use static list of warning/terminated reasons [\#18885](https://github.com/netdata/netdata/pull/18885) ([ilyam8](https://github.com/ilyam8))
- properly sanitize prometheus names and values [\#18884](https://github.com/netdata/netdata/pull/18884) ([ktsaou](https://github.com/ktsaou))
- Windows storage fixes [\#18880](https://github.com/netdata/netdata/pull/18880) ([ktsaou](https://github.com/ktsaou))
- include windows.h globally in libnetdata [\#18878](https://github.com/netdata/netdata/pull/18878) ([ktsaou](https://github.com/ktsaou))
- fix: correct go.d.plugin permission for source builds [\#18876](https://github.com/netdata/netdata/pull/18876) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/prometheus/common from 0.60.0 to 0.60.1 in /src/go [\#18874](https://github.com/netdata/netdata/pull/18874) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump k8s.io/client-go from 0.31.1 to 0.31.2 in /src/go [\#18873](https://github.com/netdata/netdata/pull/18873) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.45.0 to 0.45.1 in /src/go [\#18872](https://github.com/netdata/netdata/pull/18872) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix: correct health schema typo preventing Action alert rendering. [\#18871](https://github.com/netdata/netdata/pull/18871) ([ilyam8](https://github.com/ilyam8))
- Adjust text\_sanitizer to accept the default value [\#18870](https://github.com/netdata/netdata/pull/18870) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18869](https://github.com/netdata/netdata/pull/18869) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/ping\): clarify permissions [\#18868](https://github.com/netdata/netdata/pull/18868) ([ilyam8](https://github.com/ilyam8))
- Fix corruption in expression value replacement [\#18865](https://github.com/netdata/netdata/pull/18865) ([stelfrag](https://github.com/stelfrag))
- Prevent memory corruption during ACLK OTP decode [\#18863](https://github.com/netdata/netdata/pull/18863) ([stelfrag](https://github.com/stelfrag))
- Do not build H2O by default. [\#18861](https://github.com/netdata/netdata/pull/18861) ([vkalintiris](https://github.com/vkalintiris))
- Regenerate integrations.js [\#18860](https://github.com/netdata/netdata/pull/18860) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d.plugin\): add MaxScale collector [\#18859](https://github.com/netdata/netdata/pull/18859) ([ilyam8](https://github.com/ilyam8))
- fix\(apps.plugin\): add tini to Linux managers [\#18856](https://github.com/netdata/netdata/pull/18856) ([ilyam8](https://github.com/ilyam8))
- feat\(proc/numa\): add numa node mem activity [\#18855](https://github.com/netdata/netdata/pull/18855) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/vmware/govmomi from 0.44.1 to 0.45.0 in /src/go [\#18854](https://github.com/netdata/netdata/pull/18854) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore\(ci\): print versions in check\_successful\_update [\#18853](https://github.com/netdata/netdata/pull/18853) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18852](https://github.com/netdata/netdata/pull/18852) ([netdatabot](https://github.com/netdatabot))
- Make integration links absolute [\#18851](https://github.com/netdata/netdata/pull/18851) ([Ancairon](https://github.com/Ancairon))
- fix\(packaging\): check for sys/capability.h only on Linux [\#18849](https://github.com/netdata/netdata/pull/18849) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/sd/nl\): make timeout and interval configurable [\#18847](https://github.com/netdata/netdata/pull/18847) ([ilyam8](https://github.com/ilyam8))
- fix\(packaging\): fix installing libcurl\_dev on FreeBSD [\#18845](https://github.com/netdata/netdata/pull/18845) ([ilyam8](https://github.com/ilyam8))
- Silence up-to-date installation targets. [\#18842](https://github.com/netdata/netdata/pull/18842) ([vkalintiris](https://github.com/vkalintiris))
- docs\(web/gui\): remove legacy dashboard description [\#18841](https://github.com/netdata/netdata/pull/18841) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18839](https://github.com/netdata/netdata/pull/18839) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/vernemq\): add "Queued PUBLISH Messages" chart [\#18838](https://github.com/netdata/netdata/pull/18838) ([ilyam8](https://github.com/ilyam8))
- Remove RRDSET\_FLAG\_DETAIL. [\#18837](https://github.com/netdata/netdata/pull/18837) ([vkalintiris](https://github.com/vkalintiris))
- Update enterprise SSO docs [\#18836](https://github.com/netdata/netdata/pull/18836) ([car12o](https://github.com/car12o))
- chore\(go.d/vernemq\): remove unused file [\#18835](https://github.com/netdata/netdata/pull/18835) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18834](https://github.com/netdata/netdata/pull/18834) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/nvidia\_smi\): add "index" label to GPU charts [\#18833](https://github.com/netdata/netdata/pull/18833) ([ilyam8](https://github.com/ilyam8))
- spawn-server-nofork: invalid magic [\#18831](https://github.com/netdata/netdata/pull/18831) ([ktsaou](https://github.com/ktsaou))
- remove old obsolete check for excess data in request [\#18830](https://github.com/netdata/netdata/pull/18830) ([ktsaou](https://github.com/ktsaou))
- Add the Windows event logs integration to the meta [\#18829](https://github.com/netdata/netdata/pull/18829) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.6.2 to 9.7.0 in /src/go [\#18828](https://github.com/netdata/netdata/pull/18828) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#18826](https://github.com/netdata/netdata/pull/18826) ([netdatabot](https://github.com/netdatabot))
- Common O/S Caching Layer for users and groups [\#18825](https://github.com/netdata/netdata/pull/18825) ([ktsaou](https://github.com/ktsaou))
- More windows metrics [\#18824](https://github.com/netdata/netdata/pull/18824) ([ktsaou](https://github.com/ktsaou))
- fix compilation on windows [\#18823](https://github.com/netdata/netdata/pull/18823) ([ktsaou](https://github.com/ktsaou))
- numa basic meminfo [\#18822](https://github.com/netdata/netdata/pull/18822) ([ktsaou](https://github.com/ktsaou))
- fixes last PR merge [\#18821](https://github.com/netdata/netdata/pull/18821) ([ktsaou](https://github.com/ktsaou))
- optimizations for servers with vast amounts of sockets [\#18820](https://github.com/netdata/netdata/pull/18820) ([ktsaou](https://github.com/ktsaou))
- claiming should wait for node id and status ONLINE only [\#18816](https://github.com/netdata/netdata/pull/18816) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/vernemq\)!: support prometheus namespace added in v2.0 [\#18815](https://github.com/netdata/netdata/pull/18815) ([ilyam8](https://github.com/ilyam8))
- Comment out dictionary with hashtable code for now [\#18814](https://github.com/netdata/netdata/pull/18814) ([stelfrag](https://github.com/stelfrag))
- Fix variable scope to prevent invalid memory access [\#18813](https://github.com/netdata/netdata/pull/18813) ([stelfrag](https://github.com/stelfrag))
- fix\(proc/proc\_net\_dev\): delay collecting all virtual interfaces [\#18812](https://github.com/netdata/netdata/pull/18812) ([ilyam8](https://github.com/ilyam8))
- Revert "Fix atomic builtins test that currently fails for llvm+compiler\_rt when gcc is not present" [\#18811](https://github.com/netdata/netdata/pull/18811) ([stelfrag](https://github.com/stelfrag))
- Windows storage metrics [\#18810](https://github.com/netdata/netdata/pull/18810) ([ktsaou](https://github.com/ktsaou))
- aesthetic changes in the code [\#18808](https://github.com/netdata/netdata/pull/18808) ([ktsaou](https://github.com/ktsaou))
- allow local-listeners to associate container sockets with pids [\#18807](https://github.com/netdata/netdata/pull/18807) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/sensors\): ignore 'unknown' values [\#18806](https://github.com/netdata/netdata/pull/18806) ([ilyam8](https://github.com/ilyam8))
- Calculate currently collected metrics [\#18803](https://github.com/netdata/netdata/pull/18803) ([stelfrag](https://github.com/stelfrag))
- feat\(apps.plugin\): add vernemq to apps\_groups.conf [\#18802](https://github.com/netdata/netdata/pull/18802) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18801](https://github.com/netdata/netdata/pull/18801) ([netdatabot](https://github.com/netdatabot))
- bugfix for logs integrations [\#18800](https://github.com/netdata/netdata/pull/18800) ([Ancairon](https://github.com/Ancairon))
- docs: fix grammar in readme [\#18799](https://github.com/netdata/netdata/pull/18799) ([ilyam8](https://github.com/ilyam8))
- local-listeners improvements [\#18798](https://github.com/netdata/netdata/pull/18798) ([ktsaou](https://github.com/ktsaou))
- Remove macOS 12 from CI, and add macOS 15. [\#18797](https://github.com/netdata/netdata/pull/18797) ([Ferroin](https://github.com/Ferroin))
- Windows fixes \(chart labels and warnings\) [\#18796](https://github.com/netdata/netdata/pull/18796) ([ktsaou](https://github.com/ktsaou))
- Schedule a node state update after context load [\#18795](https://github.com/netdata/netdata/pull/18795) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18794](https://github.com/netdata/netdata/pull/18794) ([netdatabot](https://github.com/netdatabot))
- Add ref to dyncfg [\#18793](https://github.com/netdata/netdata/pull/18793) ([Ancairon](https://github.com/Ancairon))
- systemd-journal; support querying archived files [\#18792](https://github.com/netdata/netdata/pull/18792) ([ktsaou](https://github.com/ktsaou))
- Ιmplementation to add logs integrations [\#18791](https://github.com/netdata/netdata/pull/18791) ([Ancairon](https://github.com/Ancairon))
- Do not load/save context data in RAM mode [\#18790](https://github.com/netdata/netdata/pull/18790) ([stelfrag](https://github.com/stelfrag))
- Fix broken claiming via kickstart on some systems. [\#18789](https://github.com/netdata/netdata/pull/18789) ([Ferroin](https://github.com/Ferroin))
- Fix atomic builtins test that currently fails for llvm+compiler\_rt when gcc is not present [\#18788](https://github.com/netdata/netdata/pull/18788) ([StormBytePP](https://github.com/StormBytePP))
- Add basis for MSI installer. [\#18787](https://github.com/netdata/netdata/pull/18787) ([vkalintiris](https://github.com/vkalintiris))
- fix\(netdata-updater.sh\): ensure `--non-interactive` flag is passed during self-update [\#18786](https://github.com/netdata/netdata/pull/18786) ([ilyam8](https://github.com/ilyam8))
- Windows Network Interfaces Charts and Alerts [\#18785](https://github.com/netdata/netdata/pull/18785) ([ktsaou](https://github.com/ktsaou))
- Document ML enabled `auto` [\#18784](https://github.com/netdata/netdata/pull/18784) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/redis/go-redis/v9 from 9.6.1 to 9.6.2 in /src/go [\#18783](https://github.com/netdata/netdata/pull/18783) ([dependabot[bot]](https://github.com/apps/dependabot))
- Update README.md, fix a typo [\#18781](https://github.com/netdata/netdata/pull/18781) ([BobConanDev](https://github.com/BobConanDev))
- fix\(go.d/apcupsd\): fix ups\_load value divided by 100 [\#18780](https://github.com/netdata/netdata/pull/18780) ([ilyam8](https://github.com/ilyam8))
- unify claiming response json [\#18777](https://github.com/netdata/netdata/pull/18777) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/sd/netlisteners\): fix exec deadline exceeded check [\#18774](https://github.com/netdata/netdata/pull/18774) ([ilyam8](https://github.com/ilyam8))
- Sqlite upgrade to version 3.46.1 [\#18772](https://github.com/netdata/netdata/pull/18772) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18771](https://github.com/netdata/netdata/pull/18771) ([netdatabot](https://github.com/netdatabot))
- Bump github.com/bmatcuk/doublestar/v4 from 4.6.1 to 4.7.1 in /src/go [\#18768](https://github.com/netdata/netdata/pull/18768) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/sijms/go-ora/v2 from 2.8.20 to 2.8.22 in /src/go [\#18767](https://github.com/netdata/netdata/pull/18767) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.43.0 to 0.44.1 in /src/go [\#18766](https://github.com/netdata/netdata/pull/18766) ([dependabot[bot]](https://github.com/apps/dependabot))
- SPAWN SERVER: close all open fds on callback [\#18764](https://github.com/netdata/netdata/pull/18764) ([ktsaou](https://github.com/ktsaou))
- Adjust option \(Windows claim\) [\#18763](https://github.com/netdata/netdata/pull/18763) ([thiagoftsm](https://github.com/thiagoftsm))
- NetFramework \(Part I\) [\#18762](https://github.com/netdata/netdata/pull/18762) ([thiagoftsm](https://github.com/thiagoftsm))
- Expand ml enabled option [\#18761](https://github.com/netdata/netdata/pull/18761) ([stelfrag](https://github.com/stelfrag))
- Fix storing of repeat field [\#18760](https://github.com/netdata/netdata/pull/18760) ([stelfrag](https://github.com/stelfrag))
- local-listeners without libmnl [\#18759](https://github.com/netdata/netdata/pull/18759) ([ktsaou](https://github.com/ktsaou))
- fix\(proc.plugin/zfs\): fix arcstats.pm [\#18758](https://github.com/netdata/netdata/pull/18758) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/sd/net\_listeners\): exit if local-listeners constantly times out [\#18757](https://github.com/netdata/netdata/pull/18757) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18756](https://github.com/netdata/netdata/pull/18756) ([netdatabot](https://github.com/netdatabot))
- Update metadata.yaml [\#18755](https://github.com/netdata/netdata/pull/18755) ([Ancairon](https://github.com/Ancairon))
-  Remove the overview section from cloud notif. integrations [\#18754](https://github.com/netdata/netdata/pull/18754) ([Ancairon](https://github.com/Ancairon))
- Add Ubuntu 24.10 and Fedora 41 to CI. [\#18753](https://github.com/netdata/netdata/pull/18753) ([Ferroin](https://github.com/Ferroin))
- Simplify sentence on cloud notification integrations [\#18750](https://github.com/netdata/netdata/pull/18750) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#18749](https://github.com/netdata/netdata/pull/18749) ([netdatabot](https://github.com/netdatabot))
- fix\(freebsd.plugin\): fix sysctl arcstats.p fails on FreeBSD 14 [\#18748](https://github.com/netdata/netdata/pull/18748) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d.plugin\): fix plugin exit if no python found [\#18747](https://github.com/netdata/netdata/pull/18747) ([ilyam8](https://github.com/ilyam8))
- Fix crash on agent initialization [\#18746](https://github.com/netdata/netdata/pull/18746) ([stelfrag](https://github.com/stelfrag))
- Fix issues with Cloud Notification Integrations metadata [\#18745](https://github.com/netdata/netdata/pull/18745) ([Ancairon](https://github.com/Ancairon))
- fix\(apps.plugin\): fix debug msg spam on macOS/freeBSD [\#18743](https://github.com/netdata/netdata/pull/18743) ([ilyam8](https://github.com/ilyam8))
- docs\(apps.plugin\): fix prefix/suffix pattern example [\#18742](https://github.com/netdata/netdata/pull/18742) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/nvme\): add model\_number label [\#18741](https://github.com/netdata/netdata/pull/18741) ([ilyam8](https://github.com/ilyam8))
- sanitizers should not remove trailing underscores [\#18738](https://github.com/netdata/netdata/pull/18738) ([ktsaou](https://github.com/ktsaou))
- Remove CR \(windows.plugin\) [\#18737](https://github.com/netdata/netdata/pull/18737) ([thiagoftsm](https://github.com/thiagoftsm))
- Add `ilert` cloud notification integration [\#18736](https://github.com/netdata/netdata/pull/18736) ([car12o](https://github.com/car12o))
- fix\(go.d/sensors\): fix parsing power accuracy [\#18735](https://github.com/netdata/netdata/pull/18735) ([ilyam8](https://github.com/ilyam8))
- apps.plugin; allow parents to identify the children [\#18734](https://github.com/netdata/netdata/pull/18734) ([ktsaou](https://github.com/ktsaou))
- Windows deploy metadata [\#18733](https://github.com/netdata/netdata/pull/18733) ([Ancairon](https://github.com/Ancairon))
- \[storcli\] Support for controller ROC temperature. [\#18732](https://github.com/netdata/netdata/pull/18732) ([eatnumber1](https://github.com/eatnumber1))
- systemd-cat-native negative timeout [\#18729](https://github.com/netdata/netdata/pull/18729) ([ktsaou](https://github.com/ktsaou))
- fix\(perf.plugin\): disable if all events disabled during init [\#18728](https://github.com/netdata/netdata/pull/18728) ([ilyam8](https://github.com/ilyam8))
- apps.plugin: print also the original comm [\#18727](https://github.com/netdata/netdata/pull/18727) ([ktsaou](https://github.com/ktsaou))
- Fix handling of workflow artifacts. [\#18726](https://github.com/netdata/netdata/pull/18726) ([Ferroin](https://github.com/Ferroin))
- reset the log sources to apply user selection [\#18725](https://github.com/netdata/netdata/pull/18725) ([ktsaou](https://github.com/ktsaou))
- fix logs POST query payload parsing [\#18722](https://github.com/netdata/netdata/pull/18722) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/portcheck\): stop checking UDP ports on ICMP listen error [\#18721](https://github.com/netdata/netdata/pull/18721) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18720](https://github.com/netdata/netdata/pull/18720) ([netdatabot](https://github.com/netdatabot))
- static install: bump openssl and curl to latest stable versions [\#18719](https://github.com/netdata/netdata/pull/18719) ([ilyam8](https://github.com/ilyam8))
- go.d: use lib function to check if stderr connected to journal [\#18718](https://github.com/netdata/netdata/pull/18718) ([ilyam8](https://github.com/ilyam8))
- Pass correct GOOS and GOARCH on to package builders in CI. [\#18717](https://github.com/netdata/netdata/pull/18717) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18715](https://github.com/netdata/netdata/pull/18715) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#18714](https://github.com/netdata/netdata/pull/18714) ([netdatabot](https://github.com/netdatabot))
- Add link to meta section on integrations template [\#18713](https://github.com/netdata/netdata/pull/18713) ([Ancairon](https://github.com/Ancairon))
- Delay child disconnect update [\#18712](https://github.com/netdata/netdata/pull/18712) ([stelfrag](https://github.com/stelfrag))
- Windows installer \(Change descriptions add helping\) [\#18711](https://github.com/netdata/netdata/pull/18711) ([thiagoftsm](https://github.com/thiagoftsm))
- add instructions to configure SCIM integration in Okta [\#18710](https://github.com/netdata/netdata/pull/18710) ([juacker](https://github.com/juacker))
- fix wrong config file name in go.d/oracledb meta [\#18709](https://github.com/netdata/netdata/pull/18709) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18708](https://github.com/netdata/netdata/pull/18708) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/sensors\): add a config option to update/add sensor label value [\#18707](https://github.com/netdata/netdata/pull/18707) ([ilyam8](https://github.com/ilyam8))
- improve apps.plugin readme [\#18705](https://github.com/netdata/netdata/pull/18705) ([ilyam8](https://github.com/ilyam8))
- Update windows documentation [\#18703](https://github.com/netdata/netdata/pull/18703) ([Ancairon](https://github.com/Ancairon))
- Detect when swap is disabled when agent is running [\#18702](https://github.com/netdata/netdata/pull/18702) ([stelfrag](https://github.com/stelfrag))
- Bump golang.org/x/net from 0.29.0 to 0.30.0 in /src/go [\#18701](https://github.com/netdata/netdata/pull/18701) ([dependabot[bot]](https://github.com/apps/dependabot))
- Load chart labels on demand [\#18699](https://github.com/netdata/netdata/pull/18699) ([stelfrag](https://github.com/stelfrag))
- Add hyper-v metrics [\#18697](https://github.com/netdata/netdata/pull/18697) ([stelfrag](https://github.com/stelfrag))
- fix system-info disk space in LXC [\#18696](https://github.com/netdata/netdata/pull/18696) ([ilyam8](https://github.com/ilyam8))
- fix ram usage calculation in LXC [\#18695](https://github.com/netdata/netdata/pull/18695) ([ilyam8](https://github.com/ilyam8))
- cgroups.plugin: call `setresuid` before spawn server init [\#18694](https://github.com/netdata/netdata/pull/18694) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18693](https://github.com/netdata/netdata/pull/18693) ([netdatabot](https://github.com/netdatabot))
- go.d/nvidia\_smi: use configured "timeout" in loop mode [\#18692](https://github.com/netdata/netdata/pull/18692) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): handle containers no env vars [\#18691](https://github.com/netdata/netdata/pull/18691) ([daniel-sampliner](https://github.com/daniel-sampliner))
- MSSQL Metrics \(Part II\). [\#18689](https://github.com/netdata/netdata/pull/18689) ([thiagoftsm](https://github.com/thiagoftsm))
- Log to windows [\#18688](https://github.com/netdata/netdata/pull/18688) ([ktsaou](https://github.com/ktsaou))
- fix sanitization issues [\#18687](https://github.com/netdata/netdata/pull/18687) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18686](https://github.com/netdata/netdata/pull/18686) ([netdatabot](https://github.com/netdatabot))
- go.d/chrony: collect serverstats using chronyc [\#18685](https://github.com/netdata/netdata/pull/18685) ([ilyam8](https://github.com/ilyam8))
- UTF8 support for chart ids, names and other metadata [\#18684](https://github.com/netdata/netdata/pull/18684) ([ktsaou](https://github.com/ktsaou))
- Send node info update after ACLK connection timeout [\#18683](https://github.com/netdata/netdata/pull/18683) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18682](https://github.com/netdata/netdata/pull/18682) ([netdatabot](https://github.com/netdatabot))
- Bump github.com/tidwall/gjson from 1.17.3 to 1.18.0 in /src/go [\#18681](https://github.com/netdata/netdata/pull/18681) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.59.1 to 0.60.0 in /src/go [\#18680](https://github.com/netdata/netdata/pull/18680) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump go.mongodb.org/mongo-driver from 1.17.0 to 1.17.1 in /src/go [\#18679](https://github.com/netdata/netdata/pull/18679) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d downgrade go-ora to v2.8.20 [\#18677](https://github.com/netdata/netdata/pull/18677) ([ilyam8](https://github.com/ilyam8))
- Docs fixes [\#18676](https://github.com/netdata/netdata/pull/18676) ([Ancairon](https://github.com/Ancairon))
- cgroup-network now uses its own spawn server [\#18674](https://github.com/netdata/netdata/pull/18674) ([ktsaou](https://github.com/ktsaou))
- Apps plugin improvements2 [\#18673](https://github.com/netdata/netdata/pull/18673) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18672](https://github.com/netdata/netdata/pull/18672) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#18671](https://github.com/netdata/netdata/pull/18671) ([netdatabot](https://github.com/netdatabot))
- src dir docs pass [\#18670](https://github.com/netdata/netdata/pull/18670) ([Ancairon](https://github.com/Ancairon))
- Remove section in python plugin readme [\#18669](https://github.com/netdata/netdata/pull/18669) ([Ancairon](https://github.com/Ancairon))
- Properly set start/shutdown times to parent/child [\#18668](https://github.com/netdata/netdata/pull/18668) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18667](https://github.com/netdata/netdata/pull/18667) ([netdatabot](https://github.com/netdatabot))
- apps\_groups.conf: add oracledb [\#18666](https://github.com/netdata/netdata/pull/18666) ([ilyam8](https://github.com/ilyam8))
- Docs lint on `packaging/` dir [\#18665](https://github.com/netdata/netdata/pull/18665) ([Ancairon](https://github.com/Ancairon))
- Add FAQ to SCIM integration doc [\#18664](https://github.com/netdata/netdata/pull/18664) ([juacker](https://github.com/juacker))
- Fix win apps uptime [\#18662](https://github.com/netdata/netdata/pull/18662) ([ktsaou](https://github.com/ktsaou))
- Embed CPU architecture info in Windows installer filename. [\#18661](https://github.com/netdata/netdata/pull/18661) ([Ferroin](https://github.com/Ferroin))
- Docs directory lint documentation and fix issues [\#18660](https://github.com/netdata/netdata/pull/18660) ([Ancairon](https://github.com/Ancairon))
- bump go toolchain v1.22.8 [\#18659](https://github.com/netdata/netdata/pull/18659) ([ilyam8](https://github.com/ilyam8))
- go.d sd fix sprig funcmap [\#18658](https://github.com/netdata/netdata/pull/18658) ([ilyam8](https://github.com/ilyam8))
- Adjust content api/v1/info \(Windows\) [\#18656](https://github.com/netdata/netdata/pull/18656) ([thiagoftsm](https://github.com/thiagoftsm))
- add go.d/oracle [\#18654](https://github.com/netdata/netdata/pull/18654) ([ilyam8](https://github.com/ilyam8))
- Handle mqtt ping timeouts [\#18653](https://github.com/netdata/netdata/pull/18653) ([stelfrag](https://github.com/stelfrag))
- apps.plugin improvements [\#18652](https://github.com/netdata/netdata/pull/18652) ([ktsaou](https://github.com/ktsaou))
- remove python implementation of oracledb [\#18651](https://github.com/netdata/netdata/pull/18651) ([Ancairon](https://github.com/Ancairon))
- go.d remove duplicate chart check in tests [\#18650](https://github.com/netdata/netdata/pull/18650) ([ilyam8](https://github.com/ilyam8))
- Improve windows installer [\#18649](https://github.com/netdata/netdata/pull/18649) ([thiagoftsm](https://github.com/thiagoftsm))
- fixed freebsd cpu calculation [\#18648](https://github.com/netdata/netdata/pull/18648) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18647](https://github.com/netdata/netdata/pull/18647) ([netdatabot](https://github.com/netdatabot))
- Use temporary file for commit date check. [\#18646](https://github.com/netdata/netdata/pull/18646) ([Ferroin](https://github.com/Ferroin))
- Reorganize top-level headers in libnetdata. [\#18643](https://github.com/netdata/netdata/pull/18643) ([vkalintiris](https://github.com/vkalintiris))
- New wording about edit-config script in docs [\#18639](https://github.com/netdata/netdata/pull/18639) ([Ancairon](https://github.com/Ancairon))
- Update file names. [\#18638](https://github.com/netdata/netdata/pull/18638) ([vkalintiris](https://github.com/vkalintiris))
- Move plugins.d directory outside of collectors [\#18637](https://github.com/netdata/netdata/pull/18637) ([vkalintiris](https://github.com/vkalintiris))
- go.d/smartctl: fix exit status check in scan [\#18635](https://github.com/netdata/netdata/pull/18635) ([ilyam8](https://github.com/ilyam8))
- go.d pkg/socket: keep only one timeout option [\#18633](https://github.com/netdata/netdata/pull/18633) ([ilyam8](https://github.com/ilyam8))
- Log  agent start / stop timing events [\#18632](https://github.com/netdata/netdata/pull/18632) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18630](https://github.com/netdata/netdata/pull/18630) ([netdatabot](https://github.com/netdatabot))
- go.d/postgres: fix checkpoints query for postgres 17 [\#18629](https://github.com/netdata/netdata/pull/18629) ([ilyam8](https://github.com/ilyam8))
- go.d/ceph: fix leftovers after \#18582 [\#18628](https://github.com/netdata/netdata/pull/18628) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18627](https://github.com/netdata/netdata/pull/18627) ([netdatabot](https://github.com/netdatabot))
- Remove Python OpenLDAP implementation [\#18626](https://github.com/netdata/netdata/pull/18626) ([Ancairon](https://github.com/Ancairon))
- Port the OpenLDAP collector from Python to Go [\#18625](https://github.com/netdata/netdata/pull/18625) ([Ancairon](https://github.com/Ancairon))
- Change default pages per extent [\#18623](https://github.com/netdata/netdata/pull/18623) ([stelfrag](https://github.com/stelfrag))
- Misc mqtt related code cleanup [\#18622](https://github.com/netdata/netdata/pull/18622) ([stelfrag](https://github.com/stelfrag))
- Revert "Add ceph commands to ndsudo" [\#18620](https://github.com/netdata/netdata/pull/18620) ([ilyam8](https://github.com/ilyam8))
- go.d/hddtemp: connect and read [\#18619](https://github.com/netdata/netdata/pull/18619) ([ilyam8](https://github.com/ilyam8))
- go.d/uwsgi: don't write just connect and read [\#18618](https://github.com/netdata/netdata/pull/18618) ([ilyam8](https://github.com/ilyam8))
- Windows Installer \(Silent mode\) [\#18613](https://github.com/netdata/netdata/pull/18613) ([thiagoftsm](https://github.com/thiagoftsm))
- POST Functions [\#18611](https://github.com/netdata/netdata/pull/18611) ([ktsaou](https://github.com/ktsaou))
- Correctly include Windows installer in release creation. [\#18609](https://github.com/netdata/netdata/pull/18609) ([Ferroin](https://github.com/Ferroin))
- feat: HW req for onprem installation. [\#18608](https://github.com/netdata/netdata/pull/18608) ([M4itee](https://github.com/M4itee))
- WEB SERVER: retry sending data when errno is EAGAIN [\#18607](https://github.com/netdata/netdata/pull/18607) ([ktsaou](https://github.com/ktsaou))
- Publish Windows installers on nightly builds. [\#18603](https://github.com/netdata/netdata/pull/18603) ([Ferroin](https://github.com/Ferroin))
- Bump github.com/docker/docker from 27.3.0+incompatible to 27.3.1+incompatible in /src/go [\#18600](https://github.com/netdata/netdata/pull/18600) ([dependabot[bot]](https://github.com/apps/dependabot))
- Windows Plugin metadata [\#18599](https://github.com/netdata/netdata/pull/18599) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18598](https://github.com/netdata/netdata/pull/18598) ([netdatabot](https://github.com/netdatabot))
- go.d/sensors fix meta [\#18597](https://github.com/netdata/netdata/pull/18597) ([ilyam8](https://github.com/ilyam8))
- go.d/sensors update meta [\#18595](https://github.com/netdata/netdata/pull/18595) ([ilyam8](https://github.com/ilyam8))
- apps.plugin for windows [\#18594](https://github.com/netdata/netdata/pull/18594) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18592](https://github.com/netdata/netdata/pull/18592) ([netdatabot](https://github.com/netdatabot))
- Add MSSQL metrics \(Part I\). [\#18591](https://github.com/netdata/netdata/pull/18591) ([thiagoftsm](https://github.com/thiagoftsm))
- Add DLLs to CmakeLists.txt [\#18590](https://github.com/netdata/netdata/pull/18590) ([thiagoftsm](https://github.com/thiagoftsm))
- Bump go.mongodb.org/mongo-driver from 1.16.1 to 1.17.0 in /src/go [\#18589](https://github.com/netdata/netdata/pull/18589) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.2.1+incompatible to 27.3.0+incompatible in /src/go [\#18588](https://github.com/netdata/netdata/pull/18588) ([dependabot[bot]](https://github.com/apps/dependabot))
- Update kickstart.sh [\#18587](https://github.com/netdata/netdata/pull/18587) ([eya46](https://github.com/eya46))
- Remove python ceph collector implementation [\#18584](https://github.com/netdata/netdata/pull/18584) ([Ancairon](https://github.com/Ancairon))
- Add ceph commands to ndsudo [\#18583](https://github.com/netdata/netdata/pull/18583) ([Ancairon](https://github.com/Ancairon))
- Port Ceph collector to Go [\#18582](https://github.com/netdata/netdata/pull/18582) ([Ancairon](https://github.com/Ancairon))
- go.d/sensors refactor [\#18581](https://github.com/netdata/netdata/pull/18581) ([ilyam8](https://github.com/ilyam8))
- go.d move packages [\#18580](https://github.com/netdata/netdata/pull/18580) ([ilyam8](https://github.com/ilyam8))
- WEIGHTS: use node\_id when available, otherwise host\_id [\#18579](https://github.com/netdata/netdata/pull/18579) ([ktsaou](https://github.com/ktsaou))
- go.d/portcheck: update status duration calculation [\#18577](https://github.com/netdata/netdata/pull/18577) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18576](https://github.com/netdata/netdata/pull/18576) ([netdatabot](https://github.com/netdatabot))
- go.d/portcheck schema add tabs [\#18575](https://github.com/netdata/netdata/pull/18575) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18574](https://github.com/netdata/netdata/pull/18574) ([netdatabot](https://github.com/netdatabot))
- go.d portcheck update meta [\#18573](https://github.com/netdata/netdata/pull/18573) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18571](https://github.com/netdata/netdata/pull/18571) ([netdatabot](https://github.com/netdatabot))
- go.d sd docker: remove unnecessary info message [\#18570](https://github.com/netdata/netdata/pull/18570) ([ilyam8](https://github.com/ilyam8))
- go.d/portcheck: add UDP support [\#18569](https://github.com/netdata/netdata/pull/18569) ([ilyam8](https://github.com/ilyam8))
- Reduce connection timeout and fallback to IPV4 for ACLK connections [\#18568](https://github.com/netdata/netdata/pull/18568) ([stelfrag](https://github.com/stelfrag))
- Windows Events Log improvements 4 [\#18567](https://github.com/netdata/netdata/pull/18567) ([ktsaou](https://github.com/ktsaou))
- windows.plugin \(IIS\) [\#18566](https://github.com/netdata/netdata/pull/18566) ([thiagoftsm](https://github.com/thiagoftsm))
- Add check for 64bit builtin atomics [\#18565](https://github.com/netdata/netdata/pull/18565) ([kraj](https://github.com/kraj))
- Windows Events Log Explorer improvements 3 [\#18564](https://github.com/netdata/netdata/pull/18564) ([ktsaou](https://github.com/ktsaou))
- Windows Events Improvements 2 [\#18563](https://github.com/netdata/netdata/pull/18563) ([ktsaou](https://github.com/ktsaou))
- add cpu model to host labels [\#18562](https://github.com/netdata/netdata/pull/18562) ([ilyam8](https://github.com/ilyam8))
- go.d rename example =\> testrandom [\#18561](https://github.com/netdata/netdata/pull/18561) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18560](https://github.com/netdata/netdata/pull/18560) ([netdatabot](https://github.com/netdatabot))
- go.d/prometheus: add label\_prefix config option [\#18559](https://github.com/netdata/netdata/pull/18559) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18558](https://github.com/netdata/netdata/pull/18558) ([netdatabot](https://github.com/netdatabot))
- add nginx unit to apps\_groups.conf [\#18557](https://github.com/netdata/netdata/pull/18557) ([ilyam8](https://github.com/ilyam8))
- go.d fix typesense/nginxunit meta [\#18556](https://github.com/netdata/netdata/pull/18556) ([ilyam8](https://github.com/ilyam8))
- add go.d/nginxunit [\#18554](https://github.com/netdata/netdata/pull/18554) ([ilyam8](https://github.com/ilyam8))
- fix some docs issues [\#18553](https://github.com/netdata/netdata/pull/18553) ([ilyam8](https://github.com/ilyam8))
- go.d fix Goland code inspection warnings [\#18552](https://github.com/netdata/netdata/pull/18552) ([ilyam8](https://github.com/ilyam8))
- Bump k8s.io/client-go from 0.31.0 to 0.31.1 in /src/go [\#18549](https://github.com/netdata/netdata/pull/18549) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d move doing http req logic to web [\#18546](https://github.com/netdata/netdata/pull/18546) ([ilyam8](https://github.com/ilyam8))
- go.d pkg web renames [\#18545](https://github.com/netdata/netdata/pull/18545) ([ilyam8](https://github.com/ilyam8))
- go.d fix duplicate closeBody func [\#18544](https://github.com/netdata/netdata/pull/18544) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18543](https://github.com/netdata/netdata/pull/18543) ([netdatabot](https://github.com/netdatabot))
- go.d typesense: fix name in meta [\#18542](https://github.com/netdata/netdata/pull/18542) ([ilyam8](https://github.com/ilyam8))
- Misc code cleanup [\#18540](https://github.com/netdata/netdata/pull/18540) ([stelfrag](https://github.com/stelfrag))
- go.d add typesense collector [\#18538](https://github.com/netdata/netdata/pull/18538) ([ilyam8](https://github.com/ilyam8))
- add typesense to apps\_groups.conf [\#18537](https://github.com/netdata/netdata/pull/18537) ([ilyam8](https://github.com/ilyam8))
- Fetch metadata by hash for DEB repos. [\#18536](https://github.com/netdata/netdata/pull/18536) ([Ferroin](https://github.com/Ferroin))
- go.d snmp change label name organization-\>vendor [\#18535](https://github.com/netdata/netdata/pull/18535) ([ilyam8](https://github.com/ilyam8))
- go.d snmp fix vnode host labels [\#18534](https://github.com/netdata/netdata/pull/18534) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/vmware/govmomi from 0.42.0 to 0.43.0 in /src/go [\#18532](https://github.com/netdata/netdata/pull/18532) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d add vnode guid validation [\#18531](https://github.com/netdata/netdata/pull/18531) ([ilyam8](https://github.com/ilyam8))
- go.d snmp handle multiline sysDescr [\#18530](https://github.com/netdata/netdata/pull/18530) ([ilyam8](https://github.com/ilyam8))
- go.d/snmp: add "organization" label \(vnode\) [\#18529](https://github.com/netdata/netdata/pull/18529) ([ilyam8](https://github.com/ilyam8))
- Windows Events Improvements 1 [\#18528](https://github.com/netdata/netdata/pull/18528) ([ktsaou](https://github.com/ktsaou))
- go.d snmp: add sys descr, contact and loc as host labels for vnode [\#18527](https://github.com/netdata/netdata/pull/18527) ([ilyam8](https://github.com/ilyam8))
- Add charts for TCPv4/TCPV6/ICMP errors in windows [\#18526](https://github.com/netdata/netdata/pull/18526) ([stelfrag](https://github.com/stelfrag))
- Windows Events: recalculate the length of unicode strings returned every time [\#18525](https://github.com/netdata/netdata/pull/18525) ([ktsaou](https://github.com/ktsaou))
- go.d snmp add private enterprise numbers mapping [\#18523](https://github.com/netdata/netdata/pull/18523) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18522](https://github.com/netdata/netdata/pull/18522) ([netdatabot](https://github.com/netdatabot))
- go.d/snmp: add an option to automatically create vnode [\#18520](https://github.com/netdata/netdata/pull/18520) ([ilyam8](https://github.com/ilyam8))
- remove save-database from netdatacli usage [\#18519](https://github.com/netdata/netdata/pull/18519) ([ilyam8](https://github.com/ilyam8))
- improve netdatacli docs [\#18518](https://github.com/netdata/netdata/pull/18518) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18517](https://github.com/netdata/netdata/pull/18517) ([netdatabot](https://github.com/netdatabot))
- go.d/varnish update meta [\#18516](https://github.com/netdata/netdata/pull/18516) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/jackc/pgx/v5 from 5.7.0 to 5.7.1 in /src/go [\#18515](https://github.com/netdata/netdata/pull/18515) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d update redis lib to v9 [\#18513](https://github.com/netdata/netdata/pull/18513) ([ilyam8](https://github.com/ilyam8))
- go.d/varnish: add docker support [\#18512](https://github.com/netdata/netdata/pull/18512) ([ilyam8](https://github.com/ilyam8))
- go.d add function to execute a command inside a Docker container [\#18509](https://github.com/netdata/netdata/pull/18509) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18508](https://github.com/netdata/netdata/pull/18508) ([netdatabot](https://github.com/netdatabot))
- server dashboard v3 static files, when available [\#18507](https://github.com/netdata/netdata/pull/18507) ([ktsaou](https://github.com/ktsaou))
- add varnishstat and varnishadm to ndsudo [\#18503](https://github.com/netdata/netdata/pull/18503) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/docker/docker from 27.2.0+incompatible to 27.2.1+incompatible in /src/go [\#18502](https://github.com/netdata/netdata/pull/18502) ([dependabot[bot]](https://github.com/apps/dependabot))
- Assorted build cleanup for external data collection plugins. [\#18501](https://github.com/netdata/netdata/pull/18501) ([Ferroin](https://github.com/Ferroin))
- remove python.d/varnish [\#18499](https://github.com/netdata/netdata/pull/18499) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/jackc/pgx/v5 from 5.6.0 to 5.7.0 in /src/go [\#18498](https://github.com/netdata/netdata/pull/18498) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.58.0 to 0.59.1 in /src/go [\#18497](https://github.com/netdata/netdata/pull/18497) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/net from 0.28.0 to 0.29.0 in /src/go [\#18496](https://github.com/netdata/netdata/pull/18496) ([dependabot[bot]](https://github.com/apps/dependabot))
- Windows Plugin Metrics \(Thermal and Memory\) [\#18494](https://github.com/netdata/netdata/pull/18494) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18493](https://github.com/netdata/netdata/pull/18493) ([netdatabot](https://github.com/netdatabot))
- varnish collector Go implementation [\#18491](https://github.com/netdata/netdata/pull/18491) ([Ancairon](https://github.com/Ancairon))
- add go.d/apcupsd [\#18489](https://github.com/netdata/netdata/pull/18489) ([ilyam8](https://github.com/ilyam8))
- Improve processing on removed alerts after agent restart [\#18488](https://github.com/netdata/netdata/pull/18488) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/prometheus/common from 0.57.0 to 0.58.0 in /src/go [\#18487](https://github.com/netdata/netdata/pull/18487) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/text from 0.17.0 to 0.18.0 in /src/go [\#18486](https://github.com/netdata/netdata/pull/18486) ([dependabot[bot]](https://github.com/apps/dependabot))
- Remove Warnings \(ebpf\) [\#18484](https://github.com/netdata/netdata/pull/18484) ([thiagoftsm](https://github.com/thiagoftsm))
- \[WIP\] Windows-Events Logs Explorer [\#18483](https://github.com/netdata/netdata/pull/18483) ([ktsaou](https://github.com/ktsaou))
- fix win sysinfo installed ram calculation [\#18482](https://github.com/netdata/netdata/pull/18482) ([ilyam8](https://github.com/ilyam8))
- remove charts.d/apcupsd [\#18481](https://github.com/netdata/netdata/pull/18481) ([ilyam8](https://github.com/ilyam8))
- Update LIbbpf [\#18480](https://github.com/netdata/netdata/pull/18480) ([thiagoftsm](https://github.com/thiagoftsm))
- added missing comma in Access-Control-Allow-Headers [\#18479](https://github.com/netdata/netdata/pull/18479) ([ktsaou](https://github.com/ktsaou))
- add Access-Control-Allow-Headers: x-transaction-id [\#18478](https://github.com/netdata/netdata/pull/18478) ([ktsaou](https://github.com/ktsaou))
- add Access-Control-Allow-Headers: x-netdata-auth [\#18477](https://github.com/netdata/netdata/pull/18477) ([ktsaou](https://github.com/ktsaou))
- prevent sigsegv in config-parsers [\#18476](https://github.com/netdata/netdata/pull/18476) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18475](https://github.com/netdata/netdata/pull/18475) ([netdatabot](https://github.com/netdatabot))
- added version to systemd-journal info response [\#18474](https://github.com/netdata/netdata/pull/18474) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18473](https://github.com/netdata/netdata/pull/18473) ([netdatabot](https://github.com/netdatabot))
- Remove w1sensor in favor of Go implementation [\#18471](https://github.com/netdata/netdata/pull/18471) ([Ancairon](https://github.com/Ancairon))
- Improve processing of pending alerts [\#18470](https://github.com/netdata/netdata/pull/18470) ([stelfrag](https://github.com/stelfrag))
- Fix node index in alerts [\#18469](https://github.com/netdata/netdata/pull/18469) ([stelfrag](https://github.com/stelfrag))
- go.d storcli: fix unmarshal driveInfo [\#18466](https://github.com/netdata/netdata/pull/18466) ([ilyam8](https://github.com/ilyam8))
- w1sensor collector Go implementation [\#18464](https://github.com/netdata/netdata/pull/18464) ([Ancairon](https://github.com/Ancairon))
- Check correct number of bits for LZC of XOR value. [\#18463](https://github.com/netdata/netdata/pull/18463) ([vkalintiris](https://github.com/vkalintiris))
- netdata-claim.sh: fix parsing url arg [\#18460](https://github.com/netdata/netdata/pull/18460) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/likexian/whois from 1.15.4 to 1.15.5 in /src/go [\#18457](https://github.com/netdata/netdata/pull/18457) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/likexian/whois-parser from 1.24.19 to 1.24.20 in /src/go [\#18456](https://github.com/netdata/netdata/pull/18456) ([dependabot[bot]](https://github.com/apps/dependabot))
- Cleanup, rename and packaging fix \(Windows Codes\) [\#18455](https://github.com/netdata/netdata/pull/18455) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18454](https://github.com/netdata/netdata/pull/18454) ([netdatabot](https://github.com/netdatabot))
- Bump github.com/Masterminds/sprig/v3 from 3.2.3 to 3.3.0 in /src/go [\#18453](https://github.com/netdata/netdata/pull/18453) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.55.0 to 0.57.0 in /src/go [\#18452](https://github.com/netdata/netdata/pull/18452) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.1.2+incompatible to 27.2.0+incompatible in /src/go [\#18451](https://github.com/netdata/netdata/pull/18451) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#18450](https://github.com/netdata/netdata/pull/18450) ([netdatabot](https://github.com/netdatabot))
- go.d sensors add parsing intrusion to exec method [\#18449](https://github.com/netdata/netdata/pull/18449) ([ilyam8](https://github.com/ilyam8))
- Exit slabinfo.plugin on EPIPE [\#18448](https://github.com/netdata/netdata/pull/18448) ([teqwve](https://github.com/teqwve))
- ilert Integration [\#18447](https://github.com/netdata/netdata/pull/18447) ([DaTiMy](https://github.com/DaTiMy))
- go.d remove vnode disable [\#18446](https://github.com/netdata/netdata/pull/18446) ([ilyam8](https://github.com/ilyam8))
- go.d add support for symlinked vnode config files [\#18445](https://github.com/netdata/netdata/pull/18445) ([ilyam8](https://github.com/ilyam8))
- Proper precedence when calculating time\_to\_evict [\#18444](https://github.com/netdata/netdata/pull/18444) ([stelfrag](https://github.com/stelfrag))
- Windows Permissions [\#18443](https://github.com/netdata/netdata/pull/18443) ([thiagoftsm](https://github.com/thiagoftsm))
- do not free the sender when the sender thread exits [\#18441](https://github.com/netdata/netdata/pull/18441) ([ktsaou](https://github.com/ktsaou))
- fix receiver deadlock [\#18440](https://github.com/netdata/netdata/pull/18440) ([ktsaou](https://github.com/ktsaou))
- fix charts.d/sensors leftovers [\#18439](https://github.com/netdata/netdata/pull/18439) ([ilyam8](https://github.com/ilyam8))
- remove deadlock from sender [\#18438](https://github.com/netdata/netdata/pull/18438) ([ktsaou](https://github.com/ktsaou))
- Un-vendor proprietary dashboard code. [\#18437](https://github.com/netdata/netdata/pull/18437) ([Ferroin](https://github.com/Ferroin))
- go.d remove duplicates in testing [\#18435](https://github.com/netdata/netdata/pull/18435) ([ilyam8](https://github.com/ilyam8))
- Improve agent shutdown time [\#18434](https://github.com/netdata/netdata/pull/18434) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18432](https://github.com/netdata/netdata/pull/18432) ([netdatabot](https://github.com/netdatabot))
- go.d/sensors: add sysfs scan method to collect metrics [\#18431](https://github.com/netdata/netdata/pull/18431) ([ilyam8](https://github.com/ilyam8))
- stream paths propagated to children and parents [\#18430](https://github.com/netdata/netdata/pull/18430) ([ktsaou](https://github.com/ktsaou))
- go.d lmsensors improve performance [\#18429](https://github.com/netdata/netdata/pull/18429) ([ilyam8](https://github.com/ilyam8))
- ci fix InvalidDefaultArgInFrom warn [\#18428](https://github.com/netdata/netdata/pull/18428) ([ilyam8](https://github.com/ilyam8))
- vendor https://github.com/mdlayher/lmsensors [\#18427](https://github.com/netdata/netdata/pull/18427) ([ilyam8](https://github.com/ilyam8))
- remove charts.d/sensors [\#18426](https://github.com/netdata/netdata/pull/18426) ([ilyam8](https://github.com/ilyam8))

## [v1.47.5](https://github.com/netdata/netdata/tree/v1.47.5) (2024-10-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.4...v1.47.5)

## [v1.47.4](https://github.com/netdata/netdata/tree/v1.47.4) (2024-10-09)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.3...v1.47.4)

## [v1.47.3](https://github.com/netdata/netdata/tree/v1.47.3) (2024-10-02)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.2...v1.47.3)

## [v1.47.2](https://github.com/netdata/netdata/tree/v1.47.2) (2024-09-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.1...v1.47.2)

## [v1.47.1](https://github.com/netdata/netdata/tree/v1.47.1) (2024-09-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.99.0...v1.47.1)

## [v1.99.0](https://github.com/netdata/netdata/tree/v1.99.0) (2024-08-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.0...v1.99.0)

## [v1.47.0](https://github.com/netdata/netdata/tree/v1.47.0) (2024-08-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.3...v1.47.0)

## [v1.46.3](https://github.com/netdata/netdata/tree/v1.46.3) (2024-07-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.2...v1.46.3)

## [v1.46.2](https://github.com/netdata/netdata/tree/v1.46.2) (2024-07-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.1...v1.46.2)

## [v1.46.1](https://github.com/netdata/netdata/tree/v1.46.1) (2024-06-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.0...v1.46.1)

## [v1.46.0](https://github.com/netdata/netdata/tree/v1.46.0) (2024-06-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.6...v1.46.0)

## [v1.45.6](https://github.com/netdata/netdata/tree/v1.45.6) (2024-06-05)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.5...v1.45.6)

## [v1.45.5](https://github.com/netdata/netdata/tree/v1.45.5) (2024-05-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.4...v1.45.5)

## [v1.45.4](https://github.com/netdata/netdata/tree/v1.45.4) (2024-05-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.3...v1.45.4)

## [v1.45.3](https://github.com/netdata/netdata/tree/v1.45.3) (2024-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.2...v1.45.3)

## [v1.45.2](https://github.com/netdata/netdata/tree/v1.45.2) (2024-04-01)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.1...v1.45.2)

## [v1.45.1](https://github.com/netdata/netdata/tree/v1.45.1) (2024-03-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.0...v1.45.1)

## [v1.45.0](https://github.com/netdata/netdata/tree/v1.45.0) (2024-03-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.3...v1.45.0)

## [v1.44.3](https://github.com/netdata/netdata/tree/v1.44.3) (2024-02-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.2...v1.44.3)

## [v1.44.2](https://github.com/netdata/netdata/tree/v1.44.2) (2024-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.1...v1.44.2)

## [v1.44.1](https://github.com/netdata/netdata/tree/v1.44.1) (2023-12-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.0...v1.44.1)

## [v1.44.0](https://github.com/netdata/netdata/tree/v1.44.0) (2023-12-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.2...v1.44.0)

## [v1.43.2](https://github.com/netdata/netdata/tree/v1.43.2) (2023-10-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.1...v1.43.2)

## [v1.43.1](https://github.com/netdata/netdata/tree/v1.43.1) (2023-10-26)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.0...v1.43.1)

## [v1.43.0](https://github.com/netdata/netdata/tree/v1.43.0) (2023-10-16)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.4...v1.43.0)

## [v1.42.4](https://github.com/netdata/netdata/tree/v1.42.4) (2023-09-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.3...v1.42.4)

## [v1.42.3](https://github.com/netdata/netdata/tree/v1.42.3) (2023-09-11)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.2...v1.42.3)

## [v1.42.2](https://github.com/netdata/netdata/tree/v1.42.2) (2023-08-28)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.1...v1.42.2)

## [v1.42.1](https://github.com/netdata/netdata/tree/v1.42.1) (2023-08-16)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.0...v1.42.1)

## [v1.42.0](https://github.com/netdata/netdata/tree/v1.42.0) (2023-08-09)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.41.0...v1.42.0)

## [v1.41.0](https://github.com/netdata/netdata/tree/v1.41.0) (2023-07-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.40.1...v1.41.0)

## [v1.40.1](https://github.com/netdata/netdata/tree/v1.40.1) (2023-06-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.40.0...v1.40.1)

## [v1.40.0](https://github.com/netdata/netdata/tree/v1.40.0) (2023-06-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.39.1...v1.40.0)

## [v1.39.1](https://github.com/netdata/netdata/tree/v1.39.1) (2023-05-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.39.0...v1.39.1)

## [v1.39.0](https://github.com/netdata/netdata/tree/v1.39.0) (2023-05-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.38.1...v1.39.0)

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
