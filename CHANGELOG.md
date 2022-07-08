# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.1...HEAD)

**Merged pull requests:**

- Silence compile warnings on external source [\#13332](https://github.com/netdata/netdata/pull/13332) ([MrZammler](https://github.com/MrZammler))
- UpdateNodeCollectors message [\#13330](https://github.com/netdata/netdata/pull/13330) ([MrZammler](https://github.com/MrZammler))
- Cid 379238 379238 [\#13328](https://github.com/netdata/netdata/pull/13328) ([stelfrag](https://github.com/stelfrag))
- Fix two helgrind reports [\#13325](https://github.com/netdata/netdata/pull/13325) ([vkalintiris](https://github.com/vkalintiris))
- fix\(cgroups.plugin\): adjust kubepods patterns to filter pods when using Kind cluster [\#13324](https://github.com/netdata/netdata/pull/13324) ([ilyam8](https://github.com/ilyam8))
- Add link to docker config section [\#13323](https://github.com/netdata/netdata/pull/13323) ([cakrit](https://github.com/cakrit))
- fix\(apps.plugin\): adjust `zmstat*` pattern to exclude zoneminder scripts [\#13314](https://github.com/netdata/netdata/pull/13314) ([ilyam8](https://github.com/ilyam8))
- array allocator for dbengine page descriptors [\#13312](https://github.com/netdata/netdata/pull/13312) ([ktsaou](https://github.com/ktsaou))
- fix\(health\): disable go.d last collected alarm for prometheus module [\#13309](https://github.com/netdata/netdata/pull/13309) ([ilyam8](https://github.com/ilyam8))
- Explicitly skip uploads and notifications in third-party repositories. [\#13308](https://github.com/netdata/netdata/pull/13308) ([Ferroin](https://github.com/Ferroin))
- Protect shared variables with log lock. [\#13306](https://github.com/netdata/netdata/pull/13306) ([vkalintiris](https://github.com/vkalintiris))
- Keep rc before freeing it during labels unlink alarms [\#13305](https://github.com/netdata/netdata/pull/13305) ([MrZammler](https://github.com/MrZammler))
- fix\(cgroups.plugin\): adjust kubepods regex to fix name resolution in a kind cluster [\#13302](https://github.com/netdata/netdata/pull/13302) ([ilyam8](https://github.com/ilyam8))
- Null terminate string if file read was not successful [\#13299](https://github.com/netdata/netdata/pull/13299) ([stelfrag](https://github.com/stelfrag))
- fix\(health\): fix incorrect Redis dimension names [\#13296](https://github.com/netdata/netdata/pull/13296) ([ilyam8](https://github.com/ilyam8))
- chore: remove python-mysql from install-required-packages.sh [\#13288](https://github.com/netdata/netdata/pull/13288) ([ilyam8](https://github.com/ilyam8))
- Use new MQTT as default \(revert \#13258\)" [\#13287](https://github.com/netdata/netdata/pull/13287) ([underhood](https://github.com/underhood))
- query engine fixes for alarms and dashboards [\#13282](https://github.com/netdata/netdata/pull/13282) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin version to v0.33.0 [\#13280](https://github.com/netdata/netdata/pull/13280) ([ilyam8](https://github.com/ilyam8))
- Fixes vbi parser in mqtt5 implementation [\#13277](https://github.com/netdata/netdata/pull/13277) ([underhood](https://github.com/underhood))
- Fix alignment in charts endpoint [\#13275](https://github.com/netdata/netdata/pull/13275) ([thiagoftsm](https://github.com/thiagoftsm))
- Dont print io errors for cgroups [\#13274](https://github.com/netdata/netdata/pull/13274) ([vlvkobal](https://github.com/vlvkobal))
- Pluginsd doc [\#13273](https://github.com/netdata/netdata/pull/13273) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove obsolete --use-system-lws option from netdata-installer.sh help [\#13272](https://github.com/netdata/netdata/pull/13272) ([Dim-P](https://github.com/Dim-P))
- Rename the chart of real memory usage in FreeBSD [\#13271](https://github.com/netdata/netdata/pull/13271) ([vlvkobal](https://github.com/vlvkobal))
- Update documentation about our REST API documentation. [\#13269](https://github.com/netdata/netdata/pull/13269) ([Ferroin](https://github.com/Ferroin))
- Fix package build filtering on PRs. [\#13267](https://github.com/netdata/netdata/pull/13267) ([Ferroin](https://github.com/Ferroin))
- chore\(python.d\): remove deprecated modules from python.d.conf [\#13264](https://github.com/netdata/netdata/pull/13264) ([ilyam8](https://github.com/ilyam8))
- Multi-Tier database backend for long term metrics storage [\#13263](https://github.com/netdata/netdata/pull/13263) ([stelfrag](https://github.com/stelfrag))
- Get rid of extra semicolon in Graphite exporting [\#13261](https://github.com/netdata/netdata/pull/13261) ([vlvkobal](https://github.com/vlvkobal))
- fix RAM calculation on macOS in system-info [\#13260](https://github.com/netdata/netdata/pull/13260) ([ilyam8](https://github.com/ilyam8))
- Ebpf issues [\#13259](https://github.com/netdata/netdata/pull/13259) ([thiagoftsm](https://github.com/thiagoftsm))
- Use old mqtt implementation as default [\#13258](https://github.com/netdata/netdata/pull/13258) ([MrZammler](https://github.com/MrZammler))
- Remove warnings while compiling ML on FreeBSD [\#13255](https://github.com/netdata/netdata/pull/13255) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix issues with DEB postinstall script. [\#13252](https://github.com/netdata/netdata/pull/13252) ([Ferroin](https://github.com/Ferroin))
- Remove strftime from statements and use unixepoch instead [\#13250](https://github.com/netdata/netdata/pull/13250) ([stelfrag](https://github.com/stelfrag))
- Query engine with natural and virtual points [\#13248](https://github.com/netdata/netdata/pull/13248) ([ktsaou](https://github.com/ktsaou))
- Add fstype labels to disk charts [\#13245](https://github.com/netdata/netdata/pull/13245) ([vlvkobal](https://github.com/vlvkobal))
- Don’t pull in GCC for build if Clang is already present. [\#13244](https://github.com/netdata/netdata/pull/13244) ([Ferroin](https://github.com/Ferroin))
- Delay health until obsoletions check is complete [\#13239](https://github.com/netdata/netdata/pull/13239) ([MrZammler](https://github.com/MrZammler))
- Improve anomaly detection guide [\#13238](https://github.com/netdata/netdata/pull/13238) ([andrewm4894](https://github.com/andrewm4894))
- Implement PackageCloud cleanup [\#13236](https://github.com/netdata/netdata/pull/13236) ([maneamarius](https://github.com/maneamarius))
- Bump repoconfig package version used in kickstart.sh [\#13235](https://github.com/netdata/netdata/pull/13235) ([Ferroin](https://github.com/Ferroin))
- Updates the sqlite version in the agent [\#13233](https://github.com/netdata/netdata/pull/13233) ([stelfrag](https://github.com/stelfrag))
- Migrate data when machine GUID changes [\#13232](https://github.com/netdata/netdata/pull/13232) ([stelfrag](https://github.com/stelfrag))
- Add more sqlite unittests [\#13227](https://github.com/netdata/netdata/pull/13227) ([stelfrag](https://github.com/stelfrag))
- ci: add issues to the Agent Board project workflow [\#13225](https://github.com/netdata/netdata/pull/13225) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): fix qemu VMs and LXC containers name resolution [\#13220](https://github.com/netdata/netdata/pull/13220) ([ilyam8](https://github.com/ilyam8))
- netdata doubles [\#13217](https://github.com/netdata/netdata/pull/13217) ([ktsaou](https://github.com/ktsaou))
- deduplicate mountinfo based on mount point [\#13215](https://github.com/netdata/netdata/pull/13215) ([ktsaou](https://github.com/ktsaou))
- feat\(python.d\): load modules from user plugin directories \(NETDATA\_USER\_PLUGINS\_DIRS\) [\#13214](https://github.com/netdata/netdata/pull/13214) ([ilyam8](https://github.com/ilyam8))
- Properly handle interactivity in the updater code. [\#13209](https://github.com/netdata/netdata/pull/13209) ([Ferroin](https://github.com/Ferroin))
- Don’t use realpath to find kickstart source path. [\#13208](https://github.com/netdata/netdata/pull/13208) ([Ferroin](https://github.com/Ferroin))
- Print INTERNAL BUG messages only when NETDATA\_INTERNAL\_CHECKS is enabled [\#13207](https://github.com/netdata/netdata/pull/13207) ([MrZammler](https://github.com/MrZammler))
- Ensure tmpdir is set for every function that uses it. [\#13206](https://github.com/netdata/netdata/pull/13206) ([Ferroin](https://github.com/Ferroin))
- Add user plugin dirs to environment [\#13203](https://github.com/netdata/netdata/pull/13203) ([vlvkobal](https://github.com/vlvkobal))
- Fix cgroups netdev chart labels [\#13200](https://github.com/netdata/netdata/pull/13200) ([vlvkobal](https://github.com/vlvkobal))
- Add hostname in the worker structure to avoid constant lookups [\#13199](https://github.com/netdata/netdata/pull/13199) ([stelfrag](https://github.com/stelfrag))
- Rpm group creation [\#13197](https://github.com/netdata/netdata/pull/13197) ([iigorkarpov](https://github.com/iigorkarpov))
- Allow for an easy way to do metadata migrations [\#13196](https://github.com/netdata/netdata/pull/13196) ([stelfrag](https://github.com/stelfrag))
- Dictionaries with reference counters and full deletion support during traversal [\#13195](https://github.com/netdata/netdata/pull/13195) ([ktsaou](https://github.com/ktsaou))
- Add configuration for dbengine page fetch timeout and retry count [\#13194](https://github.com/netdata/netdata/pull/13194) ([stelfrag](https://github.com/stelfrag))
- Clean sqlite prepared statements on thread shutdown [\#13193](https://github.com/netdata/netdata/pull/13193) ([stelfrag](https://github.com/stelfrag))
- Update dashboard to version v2.26.5. [\#13192](https://github.com/netdata/netdata/pull/13192) ([netdatabot](https://github.com/netdatabot))
- chore\(netdata-installer\): remove a call to 'cleanup\_old\_netdata\_updater\(\)' because it is no longer exists [\#13189](https://github.com/netdata/netdata/pull/13189) ([ilyam8](https://github.com/ilyam8))
- feat\(python.d/smartd\_log\): add 2nd job that tries to read from '/var/lib/smartmontools/' [\#13188](https://github.com/netdata/netdata/pull/13188) ([ilyam8](https://github.com/ilyam8))
- Add type label for network interfaces [\#13187](https://github.com/netdata/netdata/pull/13187) ([vlvkobal](https://github.com/vlvkobal))
- fix\(freebsd.plugin\): fix wired/cached/avail memory calculation on FreeBSD with ZFS [\#13183](https://github.com/netdata/netdata/pull/13183) ([ilyam8](https://github.com/ilyam8))
- make configuration example clearer [\#13182](https://github.com/netdata/netdata/pull/13182) ([andrewm4894](https://github.com/andrewm4894))
- add k8s\_state dashboard\_info [\#13181](https://github.com/netdata/netdata/pull/13181) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.26.2. [\#13177](https://github.com/netdata/netdata/pull/13177) ([netdatabot](https://github.com/netdatabot))
- feat\(proc/proc\_net\_dev\): add dim per phys link state to the "Interface Physical Link State" chart [\#13176](https://github.com/netdata/netdata/pull/13176) ([ilyam8](https://github.com/ilyam8))
- set default for `minimum num samples to train` to `900` [\#13174](https://github.com/netdata/netdata/pull/13174) ([andrewm4894](https://github.com/andrewm4894))
- Add ml alerts examples [\#13173](https://github.com/netdata/netdata/pull/13173) ([andrewm4894](https://github.com/andrewm4894))
- Revert "Configurable storage engine for Netdata agents: step 3 \(\#12892\)" [\#13171](https://github.com/netdata/netdata/pull/13171) ([vkalintiris](https://github.com/vkalintiris))
- Remove warnings when openssl 3 is used. [\#13170](https://github.com/netdata/netdata/pull/13170) ([thiagoftsm](https://github.com/thiagoftsm))
- Don’t manipulate positional parameters in DEB postinst script. [\#13169](https://github.com/netdata/netdata/pull/13169) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues [\#13168](https://github.com/netdata/netdata/pull/13168) ([stelfrag](https://github.com/stelfrag))
- feat\(proc/proc\_net\_dev\): add dim per operstate to the "Interface Operational State" chart [\#13167](https://github.com/netdata/netdata/pull/13167) ([ilyam8](https://github.com/ilyam8))
- feat\(proc/proc\_net\_dev\): add dim per duplex state to the "Interface Duplex State" chart [\#13165](https://github.com/netdata/netdata/pull/13165) ([ilyam8](https://github.com/ilyam8))
- allow traversing null-value dictionaries [\#13162](https://github.com/netdata/netdata/pull/13162) ([ktsaou](https://github.com/ktsaou))
- Use memset to mark the empty words in the quoted\_strings\_splitter function [\#13161](https://github.com/netdata/netdata/pull/13161) ([stelfrag](https://github.com/stelfrag))
- Fix data query on stale chart [\#13159](https://github.com/netdata/netdata/pull/13159) ([stelfrag](https://github.com/stelfrag))
- enable ml by default [\#13158](https://github.com/netdata/netdata/pull/13158) ([andrewm4894](https://github.com/andrewm4894))
- Fix labels unit test [\#13156](https://github.com/netdata/netdata/pull/13156) ([stelfrag](https://github.com/stelfrag))
- Query Engine multi-granularity support \(and MC improvements\) [\#13155](https://github.com/netdata/netdata/pull/13155) ([ktsaou](https://github.com/ktsaou))
- add CAP\_SYS\_RAWIO to Netdata's systemd unit CapabilityBoundingSet [\#13154](https://github.com/netdata/netdata/pull/13154) ([ilyam8](https://github.com/ilyam8))
- Update docs on what to do if collector not there [\#13152](https://github.com/netdata/netdata/pull/13152) ([cakrit](https://github.com/cakrit))
- revert to default of `host anomaly rate threshold=0.01` [\#13150](https://github.com/netdata/netdata/pull/13150) ([andrewm4894](https://github.com/andrewm4894))
- fix conditions for nightly build triggers [\#13145](https://github.com/netdata/netdata/pull/13145) ([maneamarius](https://github.com/maneamarius))
- Add cargo/rustc/bazel/buck to apps\_groups.conf [\#13143](https://github.com/netdata/netdata/pull/13143) ([vkalintiris](https://github.com/vkalintiris))
- Add an option to use malloc for page cache instead of mmap [\#13142](https://github.com/netdata/netdata/pull/13142) ([stelfrag](https://github.com/stelfrag))
- Add mem.available chart to FreeBSD [\#13140](https://github.com/netdata/netdata/pull/13140) ([MrZammler](https://github.com/MrZammler))
- fix crashes due to misaligned allocations [\#13137](https://github.com/netdata/netdata/pull/13137) ([ktsaou](https://github.com/ktsaou))
- fix\(python.d\): urllib3 import collection for py3.10+ [\#13136](https://github.com/netdata/netdata/pull/13136) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d/mongodb\): set `serverSelectionTimeoutMS` for pymongo4+ [\#13135](https://github.com/netdata/netdata/pull/13135) ([ilyam8](https://github.com/ilyam8))
- Use new mqtt implementation as default [\#13132](https://github.com/netdata/netdata/pull/13132) ([underhood](https://github.com/underhood))
- use ks2 as MC default [\#13131](https://github.com/netdata/netdata/pull/13131) ([andrewm4894](https://github.com/andrewm4894))
- allow label names to have slashes [\#13125](https://github.com/netdata/netdata/pull/13125) ([ktsaou](https://github.com/ktsaou))
- fixed coveriry 379136 379135 379134 379133 [\#13123](https://github.com/netdata/netdata/pull/13123) ([ktsaou](https://github.com/ktsaou))
- buffer overflow detected by the compiler [\#13120](https://github.com/netdata/netdata/pull/13120) ([ktsaou](https://github.com/ktsaou))
- Ci coverage [\#13118](https://github.com/netdata/netdata/pull/13118) ([maneamarius](https://github.com/maneamarius))
- Add missing control to streaming [\#13112](https://github.com/netdata/netdata/pull/13112) ([thiagoftsm](https://github.com/thiagoftsm))
- Removes Legacy JSON Cloud Protocol Support In Agent [\#13111](https://github.com/netdata/netdata/pull/13111) ([underhood](https://github.com/underhood))
- Re-enable updates for systems using static builds. [\#13110](https://github.com/netdata/netdata/pull/13110) ([Ferroin](https://github.com/Ferroin))
- Add user netdata to secondary group in DEB package [\#13109](https://github.com/netdata/netdata/pull/13109) ([iigorkarpov](https://github.com/iigorkarpov))
- Remove pinned page reference [\#13108](https://github.com/netdata/netdata/pull/13108) ([stelfrag](https://github.com/stelfrag))
- 73x times faster metrics correlations at the agent [\#13107](https://github.com/netdata/netdata/pull/13107) ([ktsaou](https://github.com/ktsaou))
- fix\(updater\): fix updating when using `--force-update` and new version of the updater script is available [\#13104](https://github.com/netdata/netdata/pull/13104) ([ilyam8](https://github.com/ilyam8))
- Remove unnescesary ‘cleanup’ code. [\#13103](https://github.com/netdata/netdata/pull/13103) ([Ferroin](https://github.com/Ferroin))
- Temporarily disable updates for static builds. [\#13100](https://github.com/netdata/netdata/pull/13100) ([Ferroin](https://github.com/Ferroin))
- docs\(statsd.plugin\): fix indentation [\#13096](https://github.com/netdata/netdata/pull/13096) ([ilyam8](https://github.com/ilyam8))
- Statistics on bytes recvd and sent [\#13091](https://github.com/netdata/netdata/pull/13091) ([underhood](https://github.com/underhood))
- fix virtualization detection on FreeBSD [\#13087](https://github.com/netdata/netdata/pull/13087) ([ilyam8](https://github.com/ilyam8))
- Update netdata commands [\#13080](https://github.com/netdata/netdata/pull/13080) ([tkatsoulas](https://github.com/tkatsoulas))
- fix: fix a base64\_encode bug [\#13074](https://github.com/netdata/netdata/pull/13074) ([kklionz](https://github.com/kklionz))
- Labels with dictionary [\#13070](https://github.com/netdata/netdata/pull/13070) ([ktsaou](https://github.com/ktsaou))
- Use a separate thread for slow mountpoints in the diskspace plugin [\#13067](https://github.com/netdata/netdata/pull/13067) ([vlvkobal](https://github.com/vlvkobal))
- Remove official support for Debian 9. [\#13065](https://github.com/netdata/netdata/pull/13065) ([Ferroin](https://github.com/Ferroin))
- Fix coverity 378587 [\#13024](https://github.com/netdata/netdata/pull/13024) ([MrZammler](https://github.com/MrZammler))
- Configurable storage engine for Netdata agents: step 3 [\#12892](https://github.com/netdata/netdata/pull/12892) ([aberaud](https://github.com/aberaud))

## [v1.35.1](https://github.com/netdata/netdata/tree/v1.35.1) (2022-06-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.0...v1.35.1)

## [v1.35.0](https://github.com/netdata/netdata/tree/v1.35.0) (2022-06-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.1...v1.35.0)

**Merged pull requests:**

- Update README.md [\#13089](https://github.com/netdata/netdata/pull/13089) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#13088](https://github.com/netdata/netdata/pull/13088) ([ktsaou](https://github.com/ktsaou))
- fix\(updater\): return 0 on successful update for native packages when running interactively [\#13083](https://github.com/netdata/netdata/pull/13083) ([ilyam8](https://github.com/ilyam8))
- Fix Coverity errors in mqtt\_websockets submodule [\#13082](https://github.com/netdata/netdata/pull/13082) ([underhood](https://github.com/underhood))
- fix\(updater\): don't produce any output if binpkg update completed successfully [\#13081](https://github.com/netdata/netdata/pull/13081) ([ilyam8](https://github.com/ilyam8))
- Fix handling of DEB package naming in CI. [\#13076](https://github.com/netdata/netdata/pull/13076) ([Ferroin](https://github.com/Ferroin))
- Update default value for "host anomaly rate threshold" [\#13075](https://github.com/netdata/netdata/pull/13075) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix locking access to chart labels [\#13064](https://github.com/netdata/netdata/pull/13064) ([stelfrag](https://github.com/stelfrag))
- fix\(cgroup.plugin\): read k8s\_cluster\_name label from the correct file [\#13062](https://github.com/netdata/netdata/pull/13062) ([ilyam8](https://github.com/ilyam8))
- Initialize chart label key parameter correctly [\#13061](https://github.com/netdata/netdata/pull/13061) ([stelfrag](https://github.com/stelfrag))
- Added Alma Linux 9 and RHEL 9 support to CI and packaging. [\#13058](https://github.com/netdata/netdata/pull/13058) ([Ferroin](https://github.com/Ferroin))
- Fix handling of temp directory in kickstart when uninstalling. [\#13056](https://github.com/netdata/netdata/pull/13056) ([Ferroin](https://github.com/Ferroin))
- Fix coverity 378625 [\#13055](https://github.com/netdata/netdata/pull/13055) ([MrZammler](https://github.com/MrZammler))
- add the ability to merge dictionary items [\#13054](https://github.com/netdata/netdata/pull/13054) ([ktsaou](https://github.com/ktsaou))
- Check for host labels when linking alerts for children [\#13053](https://github.com/netdata/netdata/pull/13053) ([MrZammler](https://github.com/MrZammler))
- dictionary improvements [\#13052](https://github.com/netdata/netdata/pull/13052) ([ktsaou](https://github.com/ktsaou))
- Fix dictionary crash walkthrough empty [\#13051](https://github.com/netdata/netdata/pull/13051) ([ktsaou](https://github.com/ktsaou))
- coverity fixes about statsd; removal of strsame [\#13049](https://github.com/netdata/netdata/pull/13049) ([ktsaou](https://github.com/ktsaou))
- Add improved reinstall documentation. [\#13047](https://github.com/netdata/netdata/pull/13047) ([Ferroin](https://github.com/Ferroin))
- Fix disabled apps \(ebpf.plugin\) [\#13044](https://github.com/netdata/netdata/pull/13044) ([thiagoftsm](https://github.com/thiagoftsm))
- add note about anomaly advisor [\#13042](https://github.com/netdata/netdata/pull/13042) ([andrewm4894](https://github.com/andrewm4894))
- replace `history` with relevant `dbengine` params [\#13041](https://github.com/netdata/netdata/pull/13041) ([andrewm4894](https://github.com/andrewm4894))
- Fix the retry count and netdata\_exit check when running an sqlite3\_step command [\#13040](https://github.com/netdata/netdata/pull/13040) ([stelfrag](https://github.com/stelfrag))
- Schedule retention message calculation to a worker thread [\#13039](https://github.com/netdata/netdata/pull/13039) ([stelfrag](https://github.com/stelfrag))
- Check return value and log an error on failure [\#13037](https://github.com/netdata/netdata/pull/13037) ([stelfrag](https://github.com/stelfrag))
- Add additional metadata to the data response [\#13036](https://github.com/netdata/netdata/pull/13036) ([stelfrag](https://github.com/stelfrag))
- When sending a dimension for the first time, make sure there is a non zero created\_at timestamp [\#13035](https://github.com/netdata/netdata/pull/13035) ([stelfrag](https://github.com/stelfrag))
- Update apps\_groups.conf [\#13033](https://github.com/netdata/netdata/pull/13033) ([fqx](https://github.com/fqx))
- Dictionary with JudyHS and double linked list [\#13032](https://github.com/netdata/netdata/pull/13032) ([ktsaou](https://github.com/ktsaou))
- add hostname to mirrored hosts [\#13030](https://github.com/netdata/netdata/pull/13030) ([ktsaou](https://github.com/ktsaou))
- Update dashboard to version v2.25.6. [\#13028](https://github.com/netdata/netdata/pull/13028) ([netdatabot](https://github.com/netdatabot))
- prevent gap filling on dbengine gaps [\#13027](https://github.com/netdata/netdata/pull/13027) ([ktsaou](https://github.com/ktsaou))
- Initialize a pointer and add a check for it [\#13023](https://github.com/netdata/netdata/pull/13023) ([vlvkobal](https://github.com/vlvkobal))
- Fix coverity issue 378598 [\#13022](https://github.com/netdata/netdata/pull/13022) ([MrZammler](https://github.com/MrZammler))
- Skip collecting network interface speed and duplex if carrier is down [\#13019](https://github.com/netdata/netdata/pull/13019) ([vlvkobal](https://github.com/vlvkobal))
- fix COVERITY\_PATH added with INSTALL\_DIR into PATH [\#13014](https://github.com/netdata/netdata/pull/13014) ([maneamarius](https://github.com/maneamarius))
- Only try to update repo metadata in updater script if needed. [\#13009](https://github.com/netdata/netdata/pull/13009) ([Ferroin](https://github.com/Ferroin))
- Treat dimensions as normal when we don't have enough/valid data. [\#13005](https://github.com/netdata/netdata/pull/13005) ([vkalintiris](https://github.com/vkalintiris))
- Use printf instead of echo for printing collected warnings in kickstart.sh. [\#13002](https://github.com/netdata/netdata/pull/13002) ([Ferroin](https://github.com/Ferroin))
- Update dashboard to version v2.25.4. [\#13000](https://github.com/netdata/netdata/pull/13000) ([netdatabot](https://github.com/netdatabot))
- Run the /net/dev module of the proc plugin in a separate thread [\#12996](https://github.com/netdata/netdata/pull/12996) ([vlvkobal](https://github.com/vlvkobal))
- Autodetect coverity install path to increase robustness [\#12995](https://github.com/netdata/netdata/pull/12995) ([maneamarius](https://github.com/maneamarius))
- Fix compilation warnings [\#12993](https://github.com/netdata/netdata/pull/12993) ([vlvkobal](https://github.com/vlvkobal))
- Delay children chart obsoletion check [\#12992](https://github.com/netdata/netdata/pull/12992) ([MrZammler](https://github.com/MrZammler))
- Fix nanosleep on platforms other than Linux [\#12991](https://github.com/netdata/netdata/pull/12991) ([vlvkobal](https://github.com/vlvkobal))
- Don't expose the chart definition to streaming if there is no metadata change [\#12990](https://github.com/netdata/netdata/pull/12990) ([stelfrag](https://github.com/stelfrag))
- Faster queries [\#12988](https://github.com/netdata/netdata/pull/12988) ([ktsaou](https://github.com/ktsaou))
- Improve reconnect node instructions [\#12987](https://github.com/netdata/netdata/pull/12987) ([cakrit](https://github.com/cakrit))
- Make heartbeat a static chart [\#12986](https://github.com/netdata/netdata/pull/12986) ([MrZammler](https://github.com/MrZammler))
- chore\(apps.plugin\): change cpu\_guest chart context [\#12983](https://github.com/netdata/netdata/pull/12983) ([ilyam8](https://github.com/ilyam8))
- fix: don't kill Netdata PIDs if successfully stopped Netdata [\#12982](https://github.com/netdata/netdata/pull/12982) ([ilyam8](https://github.com/ilyam8))
- add dictionary support to statsd [\#12980](https://github.com/netdata/netdata/pull/12980) ([ktsaou](https://github.com/ktsaou))
- fix\(kickstart.sh\): handle the case when `tput colors` doesn't return a number [\#12979](https://github.com/netdata/netdata/pull/12979) ([ilyam8](https://github.com/ilyam8))
- query engine optimizations and cleanup [\#12978](https://github.com/netdata/netdata/pull/12978) ([ktsaou](https://github.com/ktsaou))
- optimize poll\_events\(\) to spread the work over the threads more evenly [\#12975](https://github.com/netdata/netdata/pull/12975) ([ktsaou](https://github.com/ktsaou))
- chore: check link local address before querying cloud instance metadata [\#12973](https://github.com/netdata/netdata/pull/12973) ([ilyam8](https://github.com/ilyam8))
- Alarms py collector add filtering [\#12972](https://github.com/netdata/netdata/pull/12972) ([andrewm4894](https://github.com/andrewm4894))
- Don't permanetly disable a destination because of denied access [\#12971](https://github.com/netdata/netdata/pull/12971) ([MrZammler](https://github.com/MrZammler))
- modify code to resolve compile warning issue [\#12969](https://github.com/netdata/netdata/pull/12969) ([kklionz](https://github.com/kklionz))
- Return rc-\>last\_update from alarms\_values api [\#12968](https://github.com/netdata/netdata/pull/12968) ([MrZammler](https://github.com/MrZammler))
- cleanup and optimize rrdeng\_load\_metric\_next\(\) [\#12966](https://github.com/netdata/netdata/pull/12966) ([ktsaou](https://github.com/ktsaou))
- feat\(charts.d/apcupds\): add load usage chart \(Watts\) [\#12965](https://github.com/netdata/netdata/pull/12965) ([ilyam8](https://github.com/ilyam8))
- fix: keep virtualization unknown if all used commands are not available [\#12964](https://github.com/netdata/netdata/pull/12964) ([ilyam8](https://github.com/ilyam8))
- statsd sets should count unique values [\#12963](https://github.com/netdata/netdata/pull/12963) ([ktsaou](https://github.com/ktsaou))
- Add automatic retries fo static builds during nightly and release builds. [\#12961](https://github.com/netdata/netdata/pull/12961) ([Ferroin](https://github.com/Ferroin))
- Cleanup chart hash and map tables on startup [\#12956](https://github.com/netdata/netdata/pull/12956) ([stelfrag](https://github.com/stelfrag))
- Suppress warning when freeing a NULL pointer in onewayalloc\_freez [\#12955](https://github.com/netdata/netdata/pull/12955) ([stelfrag](https://github.com/stelfrag))
- Trigger queue removed alerts on health log exchange with cloud [\#12954](https://github.com/netdata/netdata/pull/12954) ([MrZammler](https://github.com/MrZammler))
- Optimize the dimensions option store to the metadata database [\#12952](https://github.com/netdata/netdata/pull/12952) ([stelfrag](https://github.com/stelfrag))
- Defer the dimension payload check to the ACLK sync thread [\#12951](https://github.com/netdata/netdata/pull/12951) ([stelfrag](https://github.com/stelfrag))
- detailed dbengine stats [\#12948](https://github.com/netdata/netdata/pull/12948) ([ktsaou](https://github.com/ktsaou))
- Prevent command\_to\_be\_logged from overflowing [\#12947](https://github.com/netdata/netdata/pull/12947) ([MrZammler](https://github.com/MrZammler))
- Update libbpf version [\#12945](https://github.com/netdata/netdata/pull/12945) ([thiagoftsm](https://github.com/thiagoftsm))
- Reduce timeout to 1 second for getting cloud instance info [\#12941](https://github.com/netdata/netdata/pull/12941) ([MrZammler](https://github.com/MrZammler))
- Stream and advertise metric correlations to the cloud [\#12940](https://github.com/netdata/netdata/pull/12940) ([MrZammler](https://github.com/MrZammler))
- feat: move dirs, logs, and env vars config options to separate sections [\#12935](https://github.com/netdata/netdata/pull/12935) ([ilyam8](https://github.com/ilyam8))
- Adjust the dimension liveness status check [\#12933](https://github.com/netdata/netdata/pull/12933) ([stelfrag](https://github.com/stelfrag))
- chore\(fping.plugin\): bump default fping version to 5.1 [\#12930](https://github.com/netdata/netdata/pull/12930) ([ilyam8](https://github.com/ilyam8))
- Restore a broken symbolic link [\#12923](https://github.com/netdata/netdata/pull/12923) ([vlvkobal](https://github.com/vlvkobal))
- collectors: apps.plugin: apps\_groups: update net, aws, ha groups [\#12921](https://github.com/netdata/netdata/pull/12921) ([k0ste](https://github.com/k0ste))
- Remove Alpine 3.12 from CI. [\#12919](https://github.com/netdata/netdata/pull/12919) ([Ferroin](https://github.com/Ferroin))
- user configurable sqlite PRAGMAs [\#12917](https://github.com/netdata/netdata/pull/12917) ([ktsaou](https://github.com/ktsaou))
- fix `[global statistics]` section in netdata.conf [\#12916](https://github.com/netdata/netdata/pull/12916) ([ilyam8](https://github.com/ilyam8))
- chore\(streaming\): bump default "buffer size bytes" to 10MB [\#12913](https://github.com/netdata/netdata/pull/12913) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): improve check for uninitialized containers in k8s [\#12912](https://github.com/netdata/netdata/pull/12912) ([ilyam8](https://github.com/ilyam8))
- fix virtualization detection when `systemd-detect-virt` is not available [\#12911](https://github.com/netdata/netdata/pull/12911) ([ilyam8](https://github.com/ilyam8))
- added worker jobs for cgroup-rename, cgroup-network and cgroup-first-time [\#12910](https://github.com/netdata/netdata/pull/12910) ([ktsaou](https://github.com/ktsaou))
- Fix the log entry for incoming cloud start streaming commands [\#12908](https://github.com/netdata/netdata/pull/12908) ([stelfrag](https://github.com/stelfrag))
- chore\(cgroups.plugin\): remove "enable new cgroups detected at run time" config option [\#12906](https://github.com/netdata/netdata/pull/12906) ([ilyam8](https://github.com/ilyam8))
- Fix release channel in the node info message [\#12905](https://github.com/netdata/netdata/pull/12905) ([stelfrag](https://github.com/stelfrag))
- chore\(worker\_utilization\): log an error when re-registering an already registered job [\#12903](https://github.com/netdata/netdata/pull/12903) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): use correct identifier when registering the main thread "chart" worker job [\#12902](https://github.com/netdata/netdata/pull/12902) ([ilyam8](https://github.com/ilyam8))
- Remove CPU-specific info from cpuidle dimensions [\#12898](https://github.com/netdata/netdata/pull/12898) ([vlvkobal](https://github.com/vlvkobal))
- Adjust alarms count [\#12896](https://github.com/netdata/netdata/pull/12896) ([MrZammler](https://github.com/MrZammler))
- Return stable or nightly based on version if the file check fails [\#12894](https://github.com/netdata/netdata/pull/12894) ([stelfrag](https://github.com/stelfrag))
- Update reconnect node with kickstart info [\#12891](https://github.com/netdata/netdata/pull/12891) ([cakrit](https://github.com/cakrit))
- Fix compilation warnings in FreeBSD [\#12887](https://github.com/netdata/netdata/pull/12887) ([vlvkobal](https://github.com/vlvkobal))
- Fix compilation warnings [\#12886](https://github.com/netdata/netdata/pull/12886) ([vlvkobal](https://github.com/vlvkobal))
- Take into account the in queue wait time when executing a data query [\#12885](https://github.com/netdata/netdata/pull/12885) ([stelfrag](https://github.com/stelfrag))
- Update dashboard to version v2.25.2. [\#12884](https://github.com/netdata/netdata/pull/12884) ([netdatabot](https://github.com/netdatabot))
- Consider ZFS ARC shrinkable as cache on FreeBSD [\#12879](https://github.com/netdata/netdata/pull/12879) ([vlvkobal](https://github.com/vlvkobal))
- Remove Fedora 34 from CI and package builds. [\#12875](https://github.com/netdata/netdata/pull/12875) ([Ferroin](https://github.com/Ferroin))
- fix\(health\): change duplicate health template message logging level to 'info' [\#12873](https://github.com/netdata/netdata/pull/12873) ([ilyam8](https://github.com/ilyam8))
- docs: fix unresolved file references [\#12872](https://github.com/netdata/netdata/pull/12872) ([ilyam8](https://github.com/ilyam8))
- Set trust durations to have data from children properly aligned [\#12870](https://github.com/netdata/netdata/pull/12870) ([stelfrag](https://github.com/stelfrag))
- feat\(proc/cgroups.plugin\): add PSI stall time charts [\#12869](https://github.com/netdata/netdata/pull/12869) ([ilyam8](https://github.com/ilyam8))
- Update README.md [\#12868](https://github.com/netdata/netdata/pull/12868) ([tkatsoulas](https://github.com/tkatsoulas))
- fix for negative per job busy time [\#12867](https://github.com/netdata/netdata/pull/12867) ([ktsaou](https://github.com/ktsaou))
- Apply some logic to possible streaming destinations [\#12866](https://github.com/netdata/netdata/pull/12866) ([MrZammler](https://github.com/MrZammler))
- fix\(cgroups.plugin\): do not disable K8s pod/container cgroups if can't rename them [\#12865](https://github.com/netdata/netdata/pull/12865) ([ilyam8](https://github.com/ilyam8))
- workers fixes and improvements [\#12863](https://github.com/netdata/netdata/pull/12863) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin version to v0.32.3 [\#12862](https://github.com/netdata/netdata/pull/12862) ([ilyam8](https://github.com/ilyam8))
- Initialize the metadata database when performing dbengine stress test [\#12861](https://github.com/netdata/netdata/pull/12861) ([stelfrag](https://github.com/stelfrag))
- Add a SQLite database checkpoint command [\#12859](https://github.com/netdata/netdata/pull/12859) ([stelfrag](https://github.com/stelfrag))
- feat\(cgroups.plugin\): add k8s cluster name label \(GKE only\) [\#12858](https://github.com/netdata/netdata/pull/12858) ([ilyam8](https://github.com/ilyam8))
- Autodetect channel for specific version [\#12856](https://github.com/netdata/netdata/pull/12856) ([maneamarius](https://github.com/maneamarius))
- Pause alert pushes to the cloud [\#12852](https://github.com/netdata/netdata/pull/12852) ([MrZammler](https://github.com/MrZammler))
- fix\(proc.plugin\): consider ZFS ARC as cache when collecting memory usage on Linux [\#12847](https://github.com/netdata/netdata/pull/12847) ([ilyam8](https://github.com/ilyam8))
- Resolve coverity related to memory and structure dereference [\#12846](https://github.com/netdata/netdata/pull/12846) ([stelfrag](https://github.com/stelfrag))
- fix memory leaks and mismatches of the use of the z functions for allocations [\#12841](https://github.com/netdata/netdata/pull/12841) ([ktsaou](https://github.com/ktsaou))
- Allow usage of new MQTT 5 implementation [\#12838](https://github.com/netdata/netdata/pull/12838) ([underhood](https://github.com/underhood))
- Set a page wait timeout and retry count [\#12836](https://github.com/netdata/netdata/pull/12836) ([stelfrag](https://github.com/stelfrag))
- Expose anomaly-bit option to health. [\#12835](https://github.com/netdata/netdata/pull/12835) ([vkalintiris](https://github.com/vkalintiris))
- feat\(plugins.d\): allow external plugins to create chart labels [\#12834](https://github.com/netdata/netdata/pull/12834) ([ilyam8](https://github.com/ilyam8))
- Ignore obsolete charts/dims in prediction thread. [\#12833](https://github.com/netdata/netdata/pull/12833) ([vkalintiris](https://github.com/vkalintiris))
- fix\(exporting\)" make 'send charts matching' behave the same as 'filter' for prometheus format [\#12832](https://github.com/netdata/netdata/pull/12832) ([ilyam8](https://github.com/ilyam8))
- Remove sync warning [\#12831](https://github.com/netdata/netdata/pull/12831) ([thiagoftsm](https://github.com/thiagoftsm))
- Reduce the number of messages written in the error log due to out of bound timestamps [\#12829](https://github.com/netdata/netdata/pull/12829) ([stelfrag](https://github.com/stelfrag))
- Bug fix in netdata-uninstaller.sh [\#12828](https://github.com/netdata/netdata/pull/12828) ([maneamarius](https://github.com/maneamarius))
- Cleanup the node instance table on startup [\#12825](https://github.com/netdata/netdata/pull/12825) ([stelfrag](https://github.com/stelfrag))
- Accept a data query timeout parameter from the cloud [\#12823](https://github.com/netdata/netdata/pull/12823) ([stelfrag](https://github.com/stelfrag))
- Broadcast completion before unlocking condition variable's mutex [\#12822](https://github.com/netdata/netdata/pull/12822) ([vkalintiris](https://github.com/vkalintiris))
- Add chart filtering parameter to the allmetrics API query [\#12820](https://github.com/netdata/netdata/pull/12820) ([vlvkobal](https://github.com/vlvkobal))
- Write the entire request with parameters in the access.log file [\#12815](https://github.com/netdata/netdata/pull/12815) ([stelfrag](https://github.com/stelfrag))
- Add a parameter for how many worker threads the libuv library needs to pre-initialize [\#12814](https://github.com/netdata/netdata/pull/12814) ([stelfrag](https://github.com/stelfrag))
- Optimize linking of foreach alarms to dimensions. [\#12813](https://github.com/netdata/netdata/pull/12813) ([vkalintiris](https://github.com/vkalintiris))
- fix!: do not replace a hyphen in the chart name with an underscore [\#12812](https://github.com/netdata/netdata/pull/12812) ([ilyam8](https://github.com/ilyam8))
- speedup queries by providing optimization in the main loop [\#12811](https://github.com/netdata/netdata/pull/12811) ([ktsaou](https://github.com/ktsaou))
- onewayallocator to use mallocz\(\) instead of mmap\(\) [\#12810](https://github.com/netdata/netdata/pull/12810) ([ktsaou](https://github.com/ktsaou))
- Add support for installing static builds on systems without usable internet connections. [\#12809](https://github.com/netdata/netdata/pull/12809) ([Ferroin](https://github.com/Ferroin))
- Configurable storage engine for Netdata agents: step 2 [\#12808](https://github.com/netdata/netdata/pull/12808) ([aberaud](https://github.com/aberaud))
- Workers utilization charts [\#12807](https://github.com/netdata/netdata/pull/12807) ([ktsaou](https://github.com/ktsaou))
- add --repositories-only option [\#12806](https://github.com/netdata/netdata/pull/12806) ([maneamarius](https://github.com/maneamarius))
- Move kickstart argument parsing code to a function. [\#12805](https://github.com/netdata/netdata/pull/12805) ([Ferroin](https://github.com/Ferroin))
- Fill missing removed events after a crash [\#12803](https://github.com/netdata/netdata/pull/12803) ([MrZammler](https://github.com/MrZammler))
- Switch to Alma Linux for RHEL compatible support. [\#12799](https://github.com/netdata/netdata/pull/12799) ([Ferroin](https://github.com/Ferroin))
- Rename --install option for kickstart.sh [\#12798](https://github.com/netdata/netdata/pull/12798) ([maneamarius](https://github.com/maneamarius))
- chore\(python.d\): remove python.d/\* announced in v1.34.0 deprecation notice [\#12796](https://github.com/netdata/netdata/pull/12796) ([ilyam8](https://github.com/ilyam8))
- Don't use MADV\_DONTDUMP on non-linux builds [\#12795](https://github.com/netdata/netdata/pull/12795) ([vkalintiris](https://github.com/vkalintiris))
- Speed up BUFFER increases \(minimize reallocs\) [\#12792](https://github.com/netdata/netdata/pull/12792) ([ktsaou](https://github.com/ktsaou))
- procfile: more comfortable initial settings and faster/fewer reallocs [\#12791](https://github.com/netdata/netdata/pull/12791) ([ktsaou](https://github.com/ktsaou))
- just a simple fix to avoid recompiling protobuf all the time [\#12790](https://github.com/netdata/netdata/pull/12790) ([ktsaou](https://github.com/ktsaou))
- fix\(proc/net/dev\): exclude Proxmox bridge interfaces  [\#12789](https://github.com/netdata/netdata/pull/12789) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): do not add network devices if cgroup proc is in the host net ns [\#12788](https://github.com/netdata/netdata/pull/12788) ([ilyam8](https://github.com/ilyam8))
- One way allocator to double the speed of parallel context queries [\#12787](https://github.com/netdata/netdata/pull/12787) ([ktsaou](https://github.com/ktsaou))
- fix\(installer\): non interpreted new lines when printing deferred errors [\#12786](https://github.com/netdata/netdata/pull/12786) ([ilyam8](https://github.com/ilyam8))
- Trace rwlocks of netdata [\#12785](https://github.com/netdata/netdata/pull/12785) ([ktsaou](https://github.com/ktsaou))
- update ml defaults in docs [\#12782](https://github.com/netdata/netdata/pull/12782) ([andrewm4894](https://github.com/andrewm4894))
- fix: printing a warning msg in installer [\#12781](https://github.com/netdata/netdata/pull/12781) ([ilyam8](https://github.com/ilyam8))
- feat\(cgroups.plugin\): add filtering by cgroups names and improve renaming in k8s [\#12778](https://github.com/netdata/netdata/pull/12778) ([ilyam8](https://github.com/ilyam8))
- Skip ACLK dimension update when dimension is freed [\#12777](https://github.com/netdata/netdata/pull/12777) ([stelfrag](https://github.com/stelfrag))
- Configurable storage engine for Netdata agents: step 1 [\#12776](https://github.com/netdata/netdata/pull/12776) ([aberaud](https://github.com/aberaud))
- Fix coverity on receiver setsockopt [\#12772](https://github.com/netdata/netdata/pull/12772) ([MrZammler](https://github.com/MrZammler))
- some config updates for ml [\#12771](https://github.com/netdata/netdata/pull/12771) ([andrewm4894](https://github.com/andrewm4894))
- Remove node.d.plugin and relevant files [\#12769](https://github.com/netdata/netdata/pull/12769) ([surajnpn](https://github.com/surajnpn))
- Fix checking of enviornment file in updater. [\#12768](https://github.com/netdata/netdata/pull/12768) ([Ferroin](https://github.com/Ferroin))
- use aclk\_parse\_otp\_error on /env error [\#12767](https://github.com/netdata/netdata/pull/12767) ([underhood](https://github.com/underhood))
- feat\(dbengine\): make dbengine page cache undumpable and dedupuble [\#12765](https://github.com/netdata/netdata/pull/12765) ([ilyam8](https://github.com/ilyam8))
- fix: use 'diskutil info` to calculate the disk size on macOS [\#12764](https://github.com/netdata/netdata/pull/12764) ([ilyam8](https://github.com/ilyam8))
- faster execution of external programs [\#12759](https://github.com/netdata/netdata/pull/12759) ([ktsaou](https://github.com/ktsaou))
- Fix and improve netdata-updater.sh script [\#12757](https://github.com/netdata/netdata/pull/12757) ([MarianSavchuk](https://github.com/MarianSavchuk))
- fix implicit declaration of function 'appconfig\_section\_option\_destroy\_non\_loaded' [\#12756](https://github.com/netdata/netdata/pull/12756) ([ilyam8](https://github.com/ilyam8))
- Update netdata-installer.sh [\#12755](https://github.com/netdata/netdata/pull/12755) ([petecooper](https://github.com/petecooper))
- Tag Gotify health notifications for the Gotify phone app [\#12753](https://github.com/netdata/netdata/pull/12753) ([JaphethLim](https://github.com/JaphethLim))
- fix\(cgroups.plugin\): remove "search for cgroups under PATH" conf option to fix memory leak [\#12752](https://github.com/netdata/netdata/pull/12752) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): run renaming script only for containers in k8s [\#12747](https://github.com/netdata/netdata/pull/12747) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): remove "enable cgroup X" config option on cgroup deletion [\#12746](https://github.com/netdata/netdata/pull/12746) ([ilyam8](https://github.com/ilyam8))
- chore\(cgroup.plugin\): remove undocumented feature reading cgroups-names.sh when renaming cgroups [\#12745](https://github.com/netdata/netdata/pull/12745) ([ilyam8](https://github.com/ilyam8))
- feat\(cgroups.plugin\): add "CPU Time Relative Share" chart [\#12741](https://github.com/netdata/netdata/pull/12741) ([ilyam8](https://github.com/ilyam8))
- chore: reduce logging in rrdset [\#12739](https://github.com/netdata/netdata/pull/12739) ([ilyam8](https://github.com/ilyam8))
- feat\(cgroups.plugin\): add k8s\_qos\_class label [\#12737](https://github.com/netdata/netdata/pull/12737) ([ilyam8](https://github.com/ilyam8))
- expand on the various parent-child config options [\#12734](https://github.com/netdata/netdata/pull/12734) ([andrewm4894](https://github.com/andrewm4894))
- Mention serial numbers in chart names in the plugins.d API documentation [\#12733](https://github.com/netdata/netdata/pull/12733) ([vlvkobal](https://github.com/vlvkobal))
- Make atomics a hard-dep. [\#12730](https://github.com/netdata/netdata/pull/12730) ([vkalintiris](https://github.com/vkalintiris))
- add --install-version flag for installing specific version of Netdata [\#12729](https://github.com/netdata/netdata/pull/12729) ([maneamarius](https://github.com/maneamarius))
- Remove per chart configuration. [\#12728](https://github.com/netdata/netdata/pull/12728) ([vkalintiris](https://github.com/vkalintiris))
- Avoid clearing already unset flags. [\#12727](https://github.com/netdata/netdata/pull/12727) ([vkalintiris](https://github.com/vkalintiris))
- Remove commented code. [\#12726](https://github.com/netdata/netdata/pull/12726) ([vkalintiris](https://github.com/vkalintiris))
- chore\(kickstart.sh\): remove unused `--auto-update` option when using static/build install method [\#12725](https://github.com/netdata/netdata/pull/12725) ([ilyam8](https://github.com/ilyam8))
- \[Chore\]: Small typo in macos document [\#12724](https://github.com/netdata/netdata/pull/12724) ([MrZammler](https://github.com/MrZammler))
- fix upgrading all currently installed packages when updating Netdata on Debian [\#12716](https://github.com/netdata/netdata/pull/12716) ([iigorkarpov](https://github.com/iigorkarpov))
- chore\(cgroups.plugin\): reduce the CPU time required for cgroup-network-helper.sh [\#12711](https://github.com/netdata/netdata/pull/12711) ([ilyam8](https://github.com/ilyam8))
- Add `-pipe` to CFLAGS in most cases for builds. [\#12709](https://github.com/netdata/netdata/pull/12709) ([Ferroin](https://github.com/Ferroin))
- Tweak static build process to improve build speed and debuggability. [\#12708](https://github.com/netdata/netdata/pull/12708) ([Ferroin](https://github.com/Ferroin))
- Check for chart obsoletion on children re-connections [\#12707](https://github.com/netdata/netdata/pull/12707) ([MrZammler](https://github.com/MrZammler))
- feat\(apps.plugin\): add proxmox-ve processes to apps\_groups.conf [\#12704](https://github.com/netdata/netdata/pull/12704) ([ilyam8](https://github.com/ilyam8))
- chore\(ebpf.plugin\): re-enable socket module by default [\#12702](https://github.com/netdata/netdata/pull/12702) ([ilyam8](https://github.com/ilyam8))
- Disable automake dependency tracking in our various one-time builds. [\#12701](https://github.com/netdata/netdata/pull/12701) ([Ferroin](https://github.com/Ferroin))
- Add missing values to algorithm vector \(eBPF\) [\#12698](https://github.com/netdata/netdata/pull/12698) ([thiagoftsm](https://github.com/thiagoftsm))

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
