# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.1...HEAD)

**Merged pull requests:**

- Fix crash when netdatacli command output too long [\#12393](https://github.com/netdata/netdata/pull/12393) ([underhood](https://github.com/underhood))
- Dont check host health enabled if host is null [\#12392](https://github.com/netdata/netdata/pull/12392) ([MrZammler](https://github.com/MrZammler))
- Delay removed event for 60 seconds after the chart's last collected time [\#12388](https://github.com/netdata/netdata/pull/12388) ([MrZammler](https://github.com/MrZammler))
- Remove unecessary error report for proc and sys files [\#12385](https://github.com/netdata/netdata/pull/12385) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: handle double host prefix when Netdata running in a podman container [\#12380](https://github.com/netdata/netdata/pull/12380) ([ilyam8](https://github.com/ilyam8))
- fix\(ebpf.plugin\): remove pid file on exit [\#12379](https://github.com/netdata/netdata/pull/12379) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: shellcheck warnings in docker run.sh [\#12377](https://github.com/netdata/netdata/pull/12377) ([ilyam8](https://github.com/ilyam8))
- Fix version handling issues in release workflow. [\#12375](https://github.com/netdata/netdata/pull/12375) ([Ferroin](https://github.com/Ferroin))
- Fix handling of pushing commits for release process. [\#12359](https://github.com/netdata/netdata/pull/12359) ([Ferroin](https://github.com/Ferroin))
- Fix chart synchronization with the cloud [\#12356](https://github.com/netdata/netdata/pull/12356) ([stelfrag](https://github.com/stelfrag))
- minor - fix analytics\_build\_info [\#12354](https://github.com/netdata/netdata/pull/12354) ([underhood](https://github.com/underhood))
- Docs fix: Add missing frontmatter [\#12353](https://github.com/netdata/netdata/pull/12353) ([kickoke](https://github.com/kickoke))
- fix\(health\): make ioping\_disk\_latency alarm less sensitive [\#12351](https://github.com/netdata/netdata/pull/12351) ([ilyam8](https://github.com/ilyam8))
- Improve agent to cloud synchronization performance [\#12348](https://github.com/netdata/netdata/pull/12348) ([stelfrag](https://github.com/stelfrag))
- Prepend context in anomaly rate dimension id. [\#12342](https://github.com/netdata/netdata/pull/12342) ([vkalintiris](https://github.com/vkalintiris))
- Redirect dependency handling script output to logfile when running from the updater. [\#12341](https://github.com/netdata/netdata/pull/12341) ([Ferroin](https://github.com/Ferroin))
- Remove owner check from webserver [\#12339](https://github.com/netdata/netdata/pull/12339) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: container virtualization detection with systemd-detect-virt [\#12338](https://github.com/netdata/netdata/pull/12338) ([ilyam8](https://github.com/ilyam8))
- fix: use default "bind to" in native packages [\#12336](https://github.com/netdata/netdata/pull/12336) ([ilyam8](https://github.com/ilyam8))
- Add latency dimension [\#12329](https://github.com/netdata/netdata/pull/12329) ([Steve8291](https://github.com/Steve8291))
- Remove check for config file in stock conf dir [\#12327](https://github.com/netdata/netdata/pull/12327) ([Steve8291](https://github.com/Steve8291))
- fix underscore in libnetfilter-acct-dev package [\#12326](https://github.com/netdata/netdata/pull/12326) ([Steve8291](https://github.com/Steve8291))
- fix: returning 0 for CPU frequency when unknown [\#12323](https://github.com/netdata/netdata/pull/12323) ([ilyam8](https://github.com/ilyam8))
- fix\(health\): adjust 10s\_ipv4\_tcp\_resets\_sent warn trigger [\#12320](https://github.com/netdata/netdata/pull/12320) ([ilyam8](https://github.com/ilyam8))
- CO-RE and syscalls [\#12318](https://github.com/netdata/netdata/pull/12318) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix 'connect' typo anomaly-detection-python.md [\#12317](https://github.com/netdata/netdata/pull/12317) ([DanTheMediocre](https://github.com/DanTheMediocre))
- Replace write with read locks [\#12309](https://github.com/netdata/netdata/pull/12309) ([MrZammler](https://github.com/MrZammler))
- adds node\_id into mirrored\_hosts list [\#12307](https://github.com/netdata/netdata/pull/12307) ([underhood](https://github.com/underhood))
- fix: CPU frequency detection for some containers [\#12306](https://github.com/netdata/netdata/pull/12306) ([ilyam8](https://github.com/ilyam8))
- introduce new chart for process states metrics [\#12305](https://github.com/netdata/netdata/pull/12305) ([codeguru1](https://github.com/codeguru1))
- fix uninstall using kickstart flag [\#12304](https://github.com/netdata/netdata/pull/12304) ([maneamarius](https://github.com/maneamarius))
- Workflow to trigger cloud regression e2e tests [\#12299](https://github.com/netdata/netdata/pull/12299) ([dimko](https://github.com/dimko))
- Fixing stderr output when testing tmpdir [\#12298](https://github.com/netdata/netdata/pull/12298) ([godismyjudge95](https://github.com/godismyjudge95))
- chore: remove unused variable in the system-info script [\#12297](https://github.com/netdata/netdata/pull/12297) ([ilyam8](https://github.com/ilyam8))
- Pull in build dependencies when updating a locally built install. [\#12294](https://github.com/netdata/netdata/pull/12294) ([Ferroin](https://github.com/Ferroin))
- fix: cpu system info detection on macOS [\#12293](https://github.com/netdata/netdata/pull/12293) ([ilyam8](https://github.com/ilyam8))
- Only store alert hashes when iterated from localhost [\#12292](https://github.com/netdata/netdata/pull/12292) ([MrZammler](https://github.com/MrZammler))
- fix\(kickstart\): use correct syntax for claiming extra parameters [\#12289](https://github.com/netdata/netdata/pull/12289) ([ilyam8](https://github.com/ilyam8))
- feat\(health\): add charts.d/nut alarms [\#12285](https://github.com/netdata/netdata/pull/12285) ([ilyam8](https://github.com/ilyam8))
- Adjust cloud dimension update frequency  [\#12284](https://github.com/netdata/netdata/pull/12284) ([stelfrag](https://github.com/stelfrag))
- Fix incorrect install type on some older nightly installs. [\#12282](https://github.com/netdata/netdata/pull/12282) ([Ferroin](https://github.com/Ferroin))
- Timex: make offset rrd independently configurable [\#12281](https://github.com/netdata/netdata/pull/12281) ([d--j](https://github.com/d--j))
- add host labels \_aclk\_ng\_new\_cloud\_protocol [\#12278](https://github.com/netdata/netdata/pull/12278) ([underhood](https://github.com/underhood))
- Use the new error mechanism in case host not found [\#12277](https://github.com/netdata/netdata/pull/12277) ([underhood](https://github.com/underhood))
- Add proper handling for legacy kickstart install detection. [\#12273](https://github.com/netdata/netdata/pull/12273) ([Ferroin](https://github.com/Ferroin))
- Fixed typos in docs/Running-behind-haproxy.md [\#12272](https://github.com/netdata/netdata/pull/12272) ([RatishT](https://github.com/RatishT))
- Change default OOM score and scheduling policy to behave more sanely. [\#12271](https://github.com/netdata/netdata/pull/12271) ([Ferroin](https://github.com/Ferroin))
- Add Ubuntu 22.04 to CI and package builds. [\#12269](https://github.com/netdata/netdata/pull/12269) ([Ferroin](https://github.com/Ferroin))
- Add Fedora 36 to CI and package builds. [\#12268](https://github.com/netdata/netdata/pull/12268) ([Ferroin](https://github.com/Ferroin))
- Null terminate decoded\_query\_string if there are no url parameters. [\#12266](https://github.com/netdata/netdata/pull/12266) ([MrZammler](https://github.com/MrZammler))
- delete package.json [\#12265](https://github.com/netdata/netdata/pull/12265) ([ilyam8](https://github.com/ilyam8))
- Docs: Fix typo in step-10.md [\#12263](https://github.com/netdata/netdata/pull/12263) ([tnagorran](https://github.com/tnagorran))
- Set a version number for the metadata database to better handle future data migrations [\#12249](https://github.com/netdata/netdata/pull/12249) ([stelfrag](https://github.com/stelfrag))
- Add a check to make sure internal chart state is initialized [\#12244](https://github.com/netdata/netdata/pull/12244) ([stelfrag](https://github.com/stelfrag))
- eBPF installation fixes [\#12242](https://github.com/netdata/netdata/pull/12242) ([thiagoftsm](https://github.com/thiagoftsm))
- Add a fix to correctly register child nodes to the cloud via a parent [\#12241](https://github.com/netdata/netdata/pull/12241) ([stelfrag](https://github.com/stelfrag))
- Fix builds where HAVE\_C\_\_\_ATOMIC is not defined. [\#12240](https://github.com/netdata/netdata/pull/12240) ([vkalintiris](https://github.com/vkalintiris))
- Adds more info to aclk-state API call [\#12231](https://github.com/netdata/netdata/pull/12231) ([underhood](https://github.com/underhood))
- minor - remove dead code [\#12230](https://github.com/netdata/netdata/pull/12230) ([underhood](https://github.com/underhood))
- Fix node information send to the cloud for older agent versions [\#12223](https://github.com/netdata/netdata/pull/12223) ([stelfrag](https://github.com/stelfrag))
- Fixed typo in docs/guides/monitor/anomaly-detection-python.md file [\#12220](https://github.com/netdata/netdata/pull/12220) ([MariosMarinos](https://github.com/MariosMarinos))
- \[makeself\] Fix license URL [\#12219](https://github.com/netdata/netdata/pull/12219) ([Daniel15](https://github.com/Daniel15))
- Update github's code owners configuration. [\#12213](https://github.com/netdata/netdata/pull/12213) ([vkalintiris](https://github.com/vkalintiris))
- Skip training of constant metrics. [\#12212](https://github.com/netdata/netdata/pull/12212) ([vkalintiris](https://github.com/vkalintiris))
- Add -W keepopenfds option. [\#12211](https://github.com/netdata/netdata/pull/12211) ([vkalintiris](https://github.com/vkalintiris))
- Skip info field in protobuf alerts messages if it doesn't exist. [\#12210](https://github.com/netdata/netdata/pull/12210) ([MrZammler](https://github.com/MrZammler))
- Remove chart specific configuration from netdata.conf except enabled [\#12209](https://github.com/netdata/netdata/pull/12209) ([stelfrag](https://github.com/stelfrag))
- Fix two small typos in documentation [\#12208](https://github.com/netdata/netdata/pull/12208) ([xrgman](https://github.com/xrgman))
- Fix `hpssa` parse error [\#12206](https://github.com/netdata/netdata/pull/12206) ([wooyey](https://github.com/wooyey))
- Add support to the updater to toggle auto-updates on and off. [\#12202](https://github.com/netdata/netdata/pull/12202) ([Ferroin](https://github.com/Ferroin))
- Improve cleaning up of orphan hosts [\#12201](https://github.com/netdata/netdata/pull/12201) ([stelfrag](https://github.com/stelfrag))
- Fix detection of existing installs. [\#12199](https://github.com/netdata/netdata/pull/12199) ([Ferroin](https://github.com/Ferroin))
- Store dimension hidden option in the metadata db [\#12196](https://github.com/netdata/netdata/pull/12196) ([stelfrag](https://github.com/stelfrag))
- make netdata-uninstaller.sh POSIX compatibility and add --uninstall flag… [\#12195](https://github.com/netdata/netdata/pull/12195) ([maneamarius](https://github.com/maneamarius))
- docs: document the issue with seccomp and claiming [\#12192](https://github.com/netdata/netdata/pull/12192) ([ilyam8](https://github.com/ilyam8))
- fix\(docs\): unresolved file references [\#12191](https://github.com/netdata/netdata/pull/12191) ([ilyam8](https://github.com/ilyam8))
- Update libs code [\#12190](https://github.com/netdata/netdata/pull/12190) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(python.d/nvidia\_smi\): use uid when can't find the username [\#12184](https://github.com/netdata/netdata/pull/12184) ([ilyam8](https://github.com/ilyam8))
- Fix typos [\#12183](https://github.com/netdata/netdata/pull/12183) ([rex4539](https://github.com/rex4539))
- Revert "Overhaul handling of auto-updates in the installer code. \(\#12076 [\#12182](https://github.com/netdata/netdata/pull/12182) ([Ferroin](https://github.com/Ferroin))
- Add warning about broken Docker hosts in container entrypoint. [\#12175](https://github.com/netdata/netdata/pull/12175) ([Ferroin](https://github.com/Ferroin))
- tidy up the installer script usage message [\#12171](https://github.com/netdata/netdata/pull/12171) ([petecooper](https://github.com/petecooper))
- Remove check for ACLK\_NG and PROMETHEUS\_WRITE in order to assume PROTOBUF [\#12168](https://github.com/netdata/netdata/pull/12168) ([MrZammler](https://github.com/MrZammler))
- Bundle protobuf on CentOS 7 and earlier. [\#12167](https://github.com/netdata/netdata/pull/12167) ([Ferroin](https://github.com/Ferroin))
- add `stress-ng` and `gremlin` to apps\_groups.conf [\#12165](https://github.com/netdata/netdata/pull/12165) ([andrewm4894](https://github.com/andrewm4894))
- Fix alerts to raise correctly when the delay and repeat parameters are used together. [\#12164](https://github.com/netdata/netdata/pull/12164) ([erdem2000](https://github.com/erdem2000))
- fix: claiming with wget [\#12163](https://github.com/netdata/netdata/pull/12163) ([ilyam8](https://github.com/ilyam8))
- fix: CPU frequency calculation in system-info.sh [\#12162](https://github.com/netdata/netdata/pull/12162) ([ilyam8](https://github.com/ilyam8))
- Docs fix: Claim nodes in the kickstart script [\#12161](https://github.com/netdata/netdata/pull/12161) ([kickoke](https://github.com/kickoke))
- Update packaging CI to only run a limited set of jobs on PRs. [\#12156](https://github.com/netdata/netdata/pull/12156) ([Ferroin](https://github.com/Ferroin))
- kickstart.sh: fix quoting for globbing [\#12148](https://github.com/netdata/netdata/pull/12148) ([fayak](https://github.com/fayak))
- Removed Google Analytics from the docs [\#12145](https://github.com/netdata/netdata/pull/12145) ([kickoke](https://github.com/kickoke))
- Docs: Improved kickstart's cloud installation docs [\#12143](https://github.com/netdata/netdata/pull/12143) ([kickoke](https://github.com/kickoke))
- Documentation: Fixed broken links [\#12142](https://github.com/netdata/netdata/pull/12142) ([kickoke](https://github.com/kickoke))
- Fix typo in ZFS ARC Cache size info [\#12138](https://github.com/netdata/netdata/pull/12138) ([dvdmuckle](https://github.com/dvdmuckle))
- Fix data query option allow\_past to correctly work in memory mode ram and save [\#12136](https://github.com/netdata/netdata/pull/12136) ([stelfrag](https://github.com/stelfrag))
- Improve messaging around unknown install handling in kickstart script. [\#12134](https://github.com/netdata/netdata/pull/12134) ([Ferroin](https://github.com/Ferroin))
- Fix the format=array output in context queries [\#12129](https://github.com/netdata/netdata/pull/12129) ([stelfrag](https://github.com/stelfrag))
- rename DO\_NOT\_TRACK to DISABLE\_TELEMETRY [\#12126](https://github.com/netdata/netdata/pull/12126) ([ilyam8](https://github.com/ilyam8))
- Bump follow-redirects from 1.14.7 to 1.14.8 [\#12124](https://github.com/netdata/netdata/pull/12124) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump karma from 6.0.2 to 6.3.14 [\#12116](https://github.com/netdata/netdata/pull/12116) ([dependabot[bot]](https://github.com/apps/dependabot))
- Track anomaly rates with DBEngine. [\#12083](https://github.com/netdata/netdata/pull/12083) ([vkalintiris](https://github.com/vkalintiris))
- Fix compilation warnings on macOS [\#12082](https://github.com/netdata/netdata/pull/12082) ([vlvkobal](https://github.com/vlvkobal))
- feat\(apps.plugin\): group Apple Filing Protocol daemons into `afp` group [\#12078](https://github.com/netdata/netdata/pull/12078) ([ilyam8](https://github.com/ilyam8))
- Overhaul handling of auto-updates in the installer code. [\#12076](https://github.com/netdata/netdata/pull/12076) ([Ferroin](https://github.com/Ferroin))
- Add serial numbers to chart names [\#12067](https://github.com/netdata/netdata/pull/12067) ([vlvkobal](https://github.com/vlvkobal))
- remove deprecated node.d modules [\#12047](https://github.com/netdata/netdata/pull/12047) ([ilyam8](https://github.com/ilyam8))
- Remove SIZEOF\_VOIDP and ENVIRONMENT{32,64} macros. [\#12046](https://github.com/netdata/netdata/pull/12046) ([vkalintiris](https://github.com/vkalintiris))
- Remove unused NETDATA\_NO\_ATOMIC\_INSTRUCTIONS macro [\#12045](https://github.com/netdata/netdata/pull/12045) ([vkalintiris](https://github.com/vkalintiris))
- Remove NETDATA\_WITH\_UUID def because it's not used anywhere. [\#12044](https://github.com/netdata/netdata/pull/12044) ([vkalintiris](https://github.com/vkalintiris))
- inform cloud about inability to satisfy request [\#12041](https://github.com/netdata/netdata/pull/12041) ([underhood](https://github.com/underhood))
- adds install method to /api/v1/info as label [\#12040](https://github.com/netdata/netdata/pull/12040) ([underhood](https://github.com/underhood))
- Adds all query types to aclk\_processed\_query\_type [\#12036](https://github.com/netdata/netdata/pull/12036) ([underhood](https://github.com/underhood))
- Create a removed alert event if chart goes obsolete [\#12021](https://github.com/netdata/netdata/pull/12021) ([MrZammler](https://github.com/MrZammler))
- minor - remove ACLK\_NEWARCH\_DEVMODE [\#12018](https://github.com/netdata/netdata/pull/12018) ([underhood](https://github.com/underhood))
- Adds chart for incoming proto msgs in new cloud protocol [\#11969](https://github.com/netdata/netdata/pull/11969) ([underhood](https://github.com/underhood))
- Update AWS SNS README.md [\#11946](https://github.com/netdata/netdata/pull/11946) ([kickoke](https://github.com/kickoke))
- Add `--no-same-owner` to `tar xf` in installer [\#11940](https://github.com/netdata/netdata/pull/11940) ([cimnine](https://github.com/cimnine))
- Show the number of processes/threads for empty apps groups [\#11834](https://github.com/netdata/netdata/pull/11834) ([vlvkobal](https://github.com/vlvkobal))

## [v1.33.1](https://github.com/netdata/netdata/tree/v1.33.1) (2022-02-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.0...v1.33.1)

**Merged pull requests:**

- docs: rename kickstart install badges units [\#12131](https://github.com/netdata/netdata/pull/12131) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.20.18. [\#12117](https://github.com/netdata/netdata/pull/12117) ([netdatabot](https://github.com/netdatabot))
- Docs: Fix paths to install boxes [\#12109](https://github.com/netdata/netdata/pull/12109) ([kickoke](https://github.com/kickoke))
- Docs fix: Match the new install box component name [\#12106](https://github.com/netdata/netdata/pull/12106) ([kickoke](https://github.com/kickoke))
- Add proper support for Oracle Linux native packages to installer. [\#12101](https://github.com/netdata/netdata/pull/12101) ([Ferroin](https://github.com/Ferroin))
- Docs improvement: Added interactive kickstart scripts where possible [\#12098](https://github.com/netdata/netdata/pull/12098) ([kickoke](https://github.com/kickoke))
- Update syntax for Caddy v2 [\#12092](https://github.com/netdata/netdata/pull/12092) ([mohammed90](https://github.com/mohammed90))
- Properly handle non-interactive installs as non-root users. [\#12089](https://github.com/netdata/netdata/pull/12089) ([Ferroin](https://github.com/Ferroin))
- Add info about installer interactivity to anonymous installer telemetry events. [\#12088](https://github.com/netdata/netdata/pull/12088) ([Ferroin](https://github.com/Ferroin))
- Make a lack of an os-release file non-fatal on install. [\#12087](https://github.com/netdata/netdata/pull/12087) ([Ferroin](https://github.com/Ferroin))
- docs: add a note that the "Install Netdata on Synology" is maintained by community [\#12086](https://github.com/netdata/netdata/pull/12086) ([ilyam8](https://github.com/ilyam8))
- disable\_ebpf\_socket: Disable thread while race condition is fixed [\#12085](https://github.com/netdata/netdata/pull/12085) ([thiagoftsm](https://github.com/thiagoftsm))
- add native installation for rockylinux [\#12081](https://github.com/netdata/netdata/pull/12081) ([maneamarius](https://github.com/maneamarius))
- Remove mention of libJudy in installation documentation for macOS [\#12080](https://github.com/netdata/netdata/pull/12080) ([vlvkobal](https://github.com/vlvkobal))
- docs: improve "Docker container names resolution" section [\#12079](https://github.com/netdata/netdata/pull/12079) ([ilyam8](https://github.com/ilyam8))
- Fix aclk\_kill\_link reconnect endless loop [\#12074](https://github.com/netdata/netdata/pull/12074) ([underhood](https://github.com/underhood))
- Disable hashes for charts and alerts if openssl is not available [\#12071](https://github.com/netdata/netdata/pull/12071) ([MrZammler](https://github.com/MrZammler))
- Adds legacy protocol deprecation banner to agent log [\#12065](https://github.com/netdata/netdata/pull/12065) ([underhood](https://github.com/underhood))
- fix typo, tidy up sentence [\#12062](https://github.com/netdata/netdata/pull/12062) ([petecooper](https://github.com/petecooper))
- Docs install cleanup [\#12057](https://github.com/netdata/netdata/pull/12057) ([kickoke](https://github.com/kickoke))
- Fix handling of non-x86 static builds in updater. [\#12055](https://github.com/netdata/netdata/pull/12055) ([Ferroin](https://github.com/Ferroin))
- fix\(docs\): unresolved file references [\#12053](https://github.com/netdata/netdata/pull/12053) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.20.16. [\#12052](https://github.com/netdata/netdata/pull/12052) ([netdatabot](https://github.com/netdatabot))
- \[Stream Compression\] - Bug fix \#12043 - lz4.h compilation error - compile from source [\#12049](https://github.com/netdata/netdata/pull/12049) ([odynik](https://github.com/odynik))
- Fix compilation errors for OpenSSL on macOS [\#12048](https://github.com/netdata/netdata/pull/12048) ([vlvkobal](https://github.com/vlvkobal))
- Updated the docs to match new install script [\#12042](https://github.com/netdata/netdata/pull/12042) ([kickoke](https://github.com/kickoke))
- Fix handling of removed packages with leftover config files in package check. [\#12033](https://github.com/netdata/netdata/pull/12033) ([Ferroin](https://github.com/Ferroin))
- update existing OS dependencies scripts and add scripts for fedora an… [\#11963](https://github.com/netdata/netdata/pull/11963) ([maneamarius](https://github.com/maneamarius))
- Posix [\#11961](https://github.com/netdata/netdata/pull/11961) ([maneamarius](https://github.com/maneamarius))
- Updated formatting issues and copy [\#11944](https://github.com/netdata/netdata/pull/11944) ([kickoke](https://github.com/kickoke))
- Replace CentOS 8 with RockyLinux 8 in CI and package builds. [\#11801](https://github.com/netdata/netdata/pull/11801) ([Ferroin](https://github.com/Ferroin))

## [v1.33.0](https://github.com/netdata/netdata/tree/v1.33.0) (2022-01-26)

[Full Changelog](https://github.com/netdata/netdata/compare/1.32.1...v1.33.0)

**Merged pull requests:**

- Re-instate plugins\_action for clabels [\#12039](https://github.com/netdata/netdata/pull/12039) ([MrZammler](https://github.com/MrZammler))
- Have cURL properly fail on non-2xx status codes in the installer. [\#12038](https://github.com/netdata/netdata/pull/12038) ([Ferroin](https://github.com/Ferroin))
- Docs Bugfix: Fixed Markdown formatting [\#12026](https://github.com/netdata/netdata/pull/12026) ([kickoke](https://github.com/kickoke))
- update README [\#12024](https://github.com/netdata/netdata/pull/12024) ([cboydstun](https://github.com/cboydstun))
- \[Stream Compression\] - Compressor buffer overflow causes stream corruption. [\#12019](https://github.com/netdata/netdata/pull/12019) ([odynik](https://github.com/odynik))
- mqtt\_websockets submodule to latest master \(fix \#12011\) [\#12015](https://github.com/netdata/netdata/pull/12015) ([underhood](https://github.com/underhood))
- Remove uncessary call [\#12014](https://github.com/netdata/netdata/pull/12014) ([thiagoftsm](https://github.com/thiagoftsm))
- Updated idlejitter-plugin docs [\#12012](https://github.com/netdata/netdata/pull/12012) ([kickoke](https://github.com/kickoke))
- Add install type info to `-W buildinfo` output. [\#12010](https://github.com/netdata/netdata/pull/12010) ([Ferroin](https://github.com/Ferroin))
- Remove internal dbengine header from spawn/spawn\_client.c [\#12009](https://github.com/netdata/netdata/pull/12009) ([vkalintiris](https://github.com/vkalintiris))
- Fix typo in the dashboard\_info.js spigot part [\#12008](https://github.com/netdata/netdata/pull/12008) ([lokerhp](https://github.com/lokerhp))
- Add support for NVME disks with blkext driver [\#12007](https://github.com/netdata/netdata/pull/12007) ([ralphm](https://github.com/ralphm))
- Fix cleanup from a failed DEB install. [\#12006](https://github.com/netdata/netdata/pull/12006) ([Ferroin](https://github.com/Ferroin))
- update go.d.plugin version to v0.31.2 [\#12005](https://github.com/netdata/netdata/pull/12005) ([ilyam8](https://github.com/ilyam8))
- Fix handling of static archive selection for installs. [\#12004](https://github.com/netdata/netdata/pull/12004) ([Ferroin](https://github.com/Ferroin))
- fix\(python.d.plugin\): prefer python3 if available [\#12001](https://github.com/netdata/netdata/pull/12001) ([ilyam8](https://github.com/ilyam8))
- Fixing redirects [\#12000](https://github.com/netdata/netdata/pull/12000) ([kickoke](https://github.com/kickoke))
- Fix install prefix handling for claiming code in new kickstart script. [\#11999](https://github.com/netdata/netdata/pull/11999) ([Ferroin](https://github.com/Ferroin))
- Add alternative install command for macOS. [\#11997](https://github.com/netdata/netdata/pull/11997) ([Ferroin](https://github.com/Ferroin))
- Fix queue removed alerts [\#11996](https://github.com/netdata/netdata/pull/11996) ([MrZammler](https://github.com/MrZammler))
- update go.d.plugin version to v0.31.1 [\#11995](https://github.com/netdata/netdata/pull/11995) ([ilyam8](https://github.com/ilyam8))
- Fix ib counters [\#11994](https://github.com/netdata/netdata/pull/11994) ([Saruspete](https://github.com/Saruspete))
- eBPF plugin CO-RE and monitoring [\#11992](https://github.com/netdata/netdata/pull/11992) ([thiagoftsm](https://github.com/thiagoftsm))
- Included link to charts.d example [\#11990](https://github.com/netdata/netdata/pull/11990) ([kickoke](https://github.com/kickoke))
- Refined the python example for clarity [\#11989](https://github.com/netdata/netdata/pull/11989) ([kickoke](https://github.com/kickoke))
- fix\(updater\): checksum validation for static build [\#11986](https://github.com/netdata/netdata/pull/11986) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d\): ignore decoding errors in ExecutableService [\#11979](https://github.com/netdata/netdata/pull/11979) ([ilyam8](https://github.com/ilyam8))
- Deleted duplicate getting started doc [\#11978](https://github.com/netdata/netdata/pull/11978) ([kickoke](https://github.com/kickoke))
- Bump lodash from 4.17.19 to 4.17.21 [\#11976](https://github.com/netdata/netdata/pull/11976) ([dependabot[bot]](https://github.com/apps/dependabot))
- Better handle creation of UUID for claiming. [\#11974](https://github.com/netdata/netdata/pull/11974) ([Ferroin](https://github.com/Ferroin))
- Fixes coverity 374746 [\#11973](https://github.com/netdata/netdata/pull/11973) ([MrZammler](https://github.com/MrZammler))
- Bump follow-redirects from 1.13.2 to 1.14.7 [\#11972](https://github.com/netdata/netdata/pull/11972) ([dependabot[bot]](https://github.com/apps/dependabot))
- Use libnetdata/required\_dummies.h in collectors. [\#11971](https://github.com/netdata/netdata/pull/11971) ([vkalintiris](https://github.com/vkalintiris))
- Fixes Wrong Chart Description [\#11970](https://github.com/netdata/netdata/pull/11970) ([underhood](https://github.com/underhood))
- Bump engine.io from 4.1.0 to 4.1.2 [\#11968](https://github.com/netdata/netdata/pull/11968) ([dependabot[bot]](https://github.com/apps/dependabot))
- Do not use dbengine headers when dbengine is disabled. [\#11967](https://github.com/netdata/netdata/pull/11967) ([vkalintiris](https://github.com/vkalintiris))
- Perform a host metadata update on child reconnection [\#11965](https://github.com/netdata/netdata/pull/11965) ([stelfrag](https://github.com/stelfrag))
- Remove bitfields from rrdhost. [\#11964](https://github.com/netdata/netdata/pull/11964) ([vkalintiris](https://github.com/vkalintiris))
- Update libmongoc CMake config [\#11962](https://github.com/netdata/netdata/pull/11962) ([vlvkobal](https://github.com/vlvkobal))
- Find host and pass host-\>health\_enabled to cloud AlarmLogHealth message [\#11960](https://github.com/netdata/netdata/pull/11960) ([MrZammler](https://github.com/MrZammler))
- Updated SNMP v3 documentation [\#11959](https://github.com/netdata/netdata/pull/11959) ([kickoke](https://github.com/kickoke))
- Add a missing capability for the perf plugin [\#11958](https://github.com/netdata/netdata/pull/11958) ([vlvkobal](https://github.com/vlvkobal))
- python.d/nvidia\_smi: add bar1 chart [\#11956](https://github.com/netdata/netdata/pull/11956) ([pbouchez](https://github.com/pbouchez))
- Compute platform-specific list of static\_threads at runtime. [\#11955](https://github.com/netdata/netdata/pull/11955) ([vkalintiris](https://github.com/vkalintiris))
- fix\(nfacct.plugin\): Netfilter accounting charts priority [\#11952](https://github.com/netdata/netdata/pull/11952) ([ilyam8](https://github.com/ilyam8))
- fix\(nfacct.plugin\): Netfilter accounting data collection [\#11951](https://github.com/netdata/netdata/pull/11951) ([ilyam8](https://github.com/ilyam8))
- fix: add a note that netfilter's `new` and `ignore` counters are removed in the latest kernel [\#11950](https://github.com/netdata/netdata/pull/11950) ([ilyam8](https://github.com/ilyam8))
- Fix a broken link in dashboard\_info.js [\#11948](https://github.com/netdata/netdata/pull/11948) ([Ancairon](https://github.com/Ancairon))
- fix retrieving service commands without failure [\#11947](https://github.com/netdata/netdata/pull/11947) ([maneamarius](https://github.com/maneamarius))
- Fix yum config-manager check [\#11945](https://github.com/netdata/netdata/pull/11945) ([lgrn](https://github.com/lgrn))
- Fixed formatting [\#11943](https://github.com/netdata/netdata/pull/11943) ([kickoke](https://github.com/kickoke))
- Fix error in configure.ac [\#11937](https://github.com/netdata/netdata/pull/11937) ([underhood](https://github.com/underhood))
- Update dashboard to version v2.20.15. [\#11934](https://github.com/netdata/netdata/pull/11934) ([netdatabot](https://github.com/netdatabot))
- Blocking publish and in flight buffer regrowth [\#11932](https://github.com/netdata/netdata/pull/11932) ([underhood](https://github.com/underhood))
- Try to find worker config thread from inactive threads for new architecture [\#11928](https://github.com/netdata/netdata/pull/11928) ([MrZammler](https://github.com/MrZammler))
- Handle re-claim while the agent is running in new architecture [\#11924](https://github.com/netdata/netdata/pull/11924) ([MrZammler](https://github.com/MrZammler))
- fix\(claim\): set URL\_BASE only if `-url` parameter value is not null [\#11919](https://github.com/netdata/netdata/pull/11919) ([ilyam8](https://github.com/ilyam8))
- Include libatomic again to allow protobuf to resolve [\#11917](https://github.com/netdata/netdata/pull/11917) ([MrZammler](https://github.com/MrZammler))
- Send ML feature information with UpdateNodeInfo. [\#11913](https://github.com/netdata/netdata/pull/11913) ([vkalintiris](https://github.com/vkalintiris))
- Don’t verify optional dependencies in build test environments in CI. [\#11910](https://github.com/netdata/netdata/pull/11910) ([Ferroin](https://github.com/Ferroin))
- fix getting latest release tag [\#11908](https://github.com/netdata/netdata/pull/11908) ([maneamarius](https://github.com/maneamarius))
- Added "==" to the list of expression operators [\#11905](https://github.com/netdata/netdata/pull/11905) ([laned130](https://github.com/laned130))
- fix\(docs\): unresolved file references [\#11903](https://github.com/netdata/netdata/pull/11903) ([ilyam8](https://github.com/ilyam8))
- Fix slight errors [\#11902](https://github.com/netdata/netdata/pull/11902) ([ardabbour](https://github.com/ardabbour))
- Fix time\_t format [\#11897](https://github.com/netdata/netdata/pull/11897) ([vlvkobal](https://github.com/vlvkobal))
- Fix handling of agent restart on update. [\#11887](https://github.com/netdata/netdata/pull/11887) ([Ferroin](https://github.com/Ferroin))
- Provide runtime ml info from a new endpoint. [\#11886](https://github.com/netdata/netdata/pull/11886) ([vkalintiris](https://github.com/vkalintiris))
- fix permissions of plugins that may be built [\#11877](https://github.com/netdata/netdata/pull/11877) ([boxjan](https://github.com/boxjan))
- Use absolute features when doing training/prediction. [\#11876](https://github.com/netdata/netdata/pull/11876) ([vkalintiris](https://github.com/vkalintiris))
- Update dependencies for the pubsub exporting connector [\#11872](https://github.com/netdata/netdata/pull/11872) ([vlvkobal](https://github.com/vlvkobal))
- Fix the code that checks for available updates. [\#11870](https://github.com/netdata/netdata/pull/11870) ([Ferroin](https://github.com/Ferroin))
- Don't check for symbols in libaws-cpp-sdk-core [\#11867](https://github.com/netdata/netdata/pull/11867) ([vlvkobal](https://github.com/vlvkobal))
- ACLK-NG remove 'cmd' switch by message type [\#11866](https://github.com/netdata/netdata/pull/11866) ([underhood](https://github.com/underhood))
- Update libbpf [\#11865](https://github.com/netdata/netdata/pull/11865) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix cmake build [\#11862](https://github.com/netdata/netdata/pull/11862) ([vlvkobal](https://github.com/vlvkobal))
- chore: update community link of alert notifications [\#11860](https://github.com/netdata/netdata/pull/11860) ([burbuli8ra](https://github.com/burbuli8ra))
- nvidia\_smi\_chart.py : fixed username for not-local users [\#11858](https://github.com/netdata/netdata/pull/11858) ([scatenag](https://github.com/scatenag))
- Remove Ubuntu 21.04 from CI and packaging. [\#11851](https://github.com/netdata/netdata/pull/11851) ([Ferroin](https://github.com/Ferroin))
- Allow PushBullet notifications to be sent to PushBullet channels [\#11850](https://github.com/netdata/netdata/pull/11850) ([sourcecodes2](https://github.com/sourcecodes2))
- Fix compilation warnings [\#11846](https://github.com/netdata/netdata/pull/11846) ([vlvkobal](https://github.com/vlvkobal))
- Send the cloud protocol used to posthog [\#11842](https://github.com/netdata/netdata/pull/11842) ([MrZammler](https://github.com/MrZammler))
- Removes ACLK Legacy [\#11841](https://github.com/netdata/netdata/pull/11841) ([underhood](https://github.com/underhood))
- Fix cachestat on kernel 5.15.x \(eBPF\) [\#11833](https://github.com/netdata/netdata/pull/11833) ([thiagoftsm](https://github.com/thiagoftsm))
- feat\(python.d/fail2ban\): add "Failed attempts" chart, cleanup [\#11825](https://github.com/netdata/netdata/pull/11825) ([ilyam8](https://github.com/ilyam8))
- Add code for LZ4 streaming data compression [\#11821](https://github.com/netdata/netdata/pull/11821) ([avstrakhov](https://github.com/avstrakhov))
- Postgres: mat. views considered as tables in table size/count chart [\#11816](https://github.com/netdata/netdata/pull/11816) ([NikolayS](https://github.com/NikolayS))
- Postgres: use block\_size instead of 8\*1024 [\#11815](https://github.com/netdata/netdata/pull/11815) ([NikolayS](https://github.com/NikolayS))
- Optimize rx msg name resolution [\#11811](https://github.com/netdata/netdata/pull/11811) ([underhood](https://github.com/underhood))
- Ignore clangd cache directory. [\#11803](https://github.com/netdata/netdata/pull/11803) ([vkalintiris](https://github.com/vkalintiris))
- fix tps decode, add memory usage chart [\#11797](https://github.com/netdata/netdata/pull/11797) ([neotf](https://github.com/neotf))
- Add localhost hostname to the edit\_command [\#11793](https://github.com/netdata/netdata/pull/11793) ([MrZammler](https://github.com/MrZammler))
- Initial release of new kickstart script. [\#11764](https://github.com/netdata/netdata/pull/11764) ([Ferroin](https://github.com/Ferroin))
- Add support to updater for updating native DEB/RPM installs with our official packages. [\#11753](https://github.com/netdata/netdata/pull/11753) ([Ferroin](https://github.com/Ferroin))

## [1.32.1](https://github.com/netdata/netdata/tree/1.32.1) (2021-12-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.32.1...1.32.1)

## [v1.32.1](https://github.com/netdata/netdata/tree/v1.32.1) (2021-12-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.32.0...v1.32.1)

**Merged pull requests:**

- Clean up anomaly-detection guide docs [\#11901](https://github.com/netdata/netdata/pull/11901) ([andrewm4894](https://github.com/andrewm4894))
- Use the chart id instead of chart name in response to incoming cloud context queries [\#11898](https://github.com/netdata/netdata/pull/11898) ([stelfrag](https://github.com/stelfrag))
- Moved data privacy section into a separate topic [\#11889](https://github.com/netdata/netdata/pull/11889) ([kickoke](https://github.com/kickoke))
- Fixed formatting issues. [\#11888](https://github.com/netdata/netdata/pull/11888) ([kickoke](https://github.com/kickoke))
- Fix postdrop handling for systemd systems. [\#11885](https://github.com/netdata/netdata/pull/11885) ([Ferroin](https://github.com/Ferroin))
- Minor ACLK docu updates [\#11882](https://github.com/netdata/netdata/pull/11882) ([underhood](https://github.com/underhood))
- Adds Swagger docs for new `/api/v1/aclk` endpoint [\#11881](https://github.com/netdata/netdata/pull/11881) ([underhood](https://github.com/underhood))
- fix\(updater\): don't produce output when static update succeeded [\#11879](https://github.com/netdata/netdata/pull/11879) ([ilyam8](https://github.com/ilyam8))
- fix\(updater\): fix exit code when updating static install && updater script [\#11873](https://github.com/netdata/netdata/pull/11873) ([ilyam8](https://github.com/ilyam8))
- add z score alarm example [\#11871](https://github.com/netdata/netdata/pull/11871) ([andrewm4894](https://github.com/andrewm4894))
- fix\(health\): used\_swap alarm calc [\#11868](https://github.com/netdata/netdata/pull/11868) ([ilyam8](https://github.com/ilyam8))
- Initialize enabled parameter to 1 in AlarmLogHealth message [\#11856](https://github.com/netdata/netdata/pull/11856) ([MrZammler](https://github.com/MrZammler))
- Explicitly conflict with distro netdata DEB packages. [\#11855](https://github.com/netdata/netdata/pull/11855) ([Ferroin](https://github.com/Ferroin))
- fixed username for not-local users [\#11854](https://github.com/netdata/netdata/pull/11854) ([scatenag](https://github.com/scatenag))
- fix static build, curl will be staict binary; extra args can be transfer [\#11852](https://github.com/netdata/netdata/pull/11852) ([boxjan](https://github.com/boxjan))
- Create ML README.md [\#11848](https://github.com/netdata/netdata/pull/11848) ([andrewm4894](https://github.com/andrewm4894))
- Fix token name in release draft workflow. [\#11847](https://github.com/netdata/netdata/pull/11847) ([Ferroin](https://github.com/Ferroin))
- Bump static builds to use Alpine 3.15 as a base. [\#11836](https://github.com/netdata/netdata/pull/11836) ([Ferroin](https://github.com/Ferroin))
- Detect whether libatomic should be linked in when using CXX linker. [\#11818](https://github.com/netdata/netdata/pull/11818) ([vkalintiris](https://github.com/vkalintiris))
- Make netdata-updater.sh POSIX compliant. [\#11755](https://github.com/netdata/netdata/pull/11755) ([Ferroin](https://github.com/Ferroin))

## [v1.32.0](https://github.com/netdata/netdata/tree/v1.32.0) (2021-11-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.31.0...v1.32.0)

**Merged pull requests:**

- fix\(health\): `pihole_blocklist_gravity_file` and `pihole_status` info lines [\#11844](https://github.com/netdata/netdata/pull/11844) ([ilyam8](https://github.com/ilyam8))
- Optional proto support fix [\#11840](https://github.com/netdata/netdata/pull/11840) ([underhood](https://github.com/underhood))
- feat\(apps.plugin\): add consul to apps\_groups.conf [\#11839](https://github.com/netdata/netdata/pull/11839) ([ilyam8](https://github.com/ilyam8))
- Add a note about pkg-config file location for freeipmi [\#11831](https://github.com/netdata/netdata/pull/11831) ([vlvkobal](https://github.com/vlvkobal))
- Remove pihole\_blocked\_queries alert [\#11829](https://github.com/netdata/netdata/pull/11829) ([Ancairon](https://github.com/Ancairon))
- Add commands to check and fix database corruption [\#11828](https://github.com/netdata/netdata/pull/11828) ([stelfrag](https://github.com/stelfrag))
- Set NETDATA\_CONTAINER\_OS\_DETECTION properly [\#11827](https://github.com/netdata/netdata/pull/11827) ([MrZammler](https://github.com/MrZammler))
- feat\(apps.plugin\): add aws to apps\_groups.conf [\#11826](https://github.com/netdata/netdata/pull/11826) ([ilyam8](https://github.com/ilyam8))
- Updating ansible steps for clarity [\#11823](https://github.com/netdata/netdata/pull/11823) ([kickoke](https://github.com/kickoke))
- Don't use wc struct if it might not exist [\#11820](https://github.com/netdata/netdata/pull/11820) ([MrZammler](https://github.com/MrZammler))
- specify pip3 when installing git-semver package [\#11817](https://github.com/netdata/netdata/pull/11817) ([maneamarius](https://github.com/maneamarius))
- Cleanup compilation warnings [\#11810](https://github.com/netdata/netdata/pull/11810) ([stelfrag](https://github.com/stelfrag))
- Fix coverity issues  [\#11809](https://github.com/netdata/netdata/pull/11809) ([stelfrag](https://github.com/stelfrag))
- Fix broken link in charts.mdx [\#11808](https://github.com/netdata/netdata/pull/11808) ([DShreve2](https://github.com/DShreve2))
- Always queue alerts to aclk\_alert [\#11806](https://github.com/netdata/netdata/pull/11806) ([MrZammler](https://github.com/MrZammler))
- Use two digits after the decimal point for the anomaly rate. [\#11804](https://github.com/netdata/netdata/pull/11804) ([vkalintiris](https://github.com/vkalintiris))
- Add POWER8+ static builds. [\#11802](https://github.com/netdata/netdata/pull/11802) ([Ferroin](https://github.com/Ferroin))
- Update libbpf [\#11800](https://github.com/netdata/netdata/pull/11800) ([thiagoftsm](https://github.com/thiagoftsm))
- Assorted cleanups to static builds. [\#11798](https://github.com/netdata/netdata/pull/11798) ([Ferroin](https://github.com/Ferroin))
- Use the proper format specifier when logging configuration options. [\#11795](https://github.com/netdata/netdata/pull/11795) ([vkalintiris](https://github.com/vkalintiris))
- Verify checksums of makeself deps. [\#11791](https://github.com/netdata/netdata/pull/11791) ([vkalintiris](https://github.com/vkalintiris))
- packaging: update go.d.plugin version to v0.31.0 [\#11789](https://github.com/netdata/netdata/pull/11789) ([ilyam8](https://github.com/ilyam8))
- Add some logging for cloud new architecture to access.log [\#11788](https://github.com/netdata/netdata/pull/11788) ([MrZammler](https://github.com/MrZammler))
- Simple fix for the data API query [\#11787](https://github.com/netdata/netdata/pull/11787) ([vlvkobal](https://github.com/vlvkobal))
- Use correct hop count if host is already in memory [\#11785](https://github.com/netdata/netdata/pull/11785) ([stelfrag](https://github.com/stelfrag))
- Fix proc/interrupts parser [\#11783](https://github.com/netdata/netdata/pull/11783) ([maximethebault](https://github.com/maximethebault))
- Fix typos [\#11782](https://github.com/netdata/netdata/pull/11782) ([rex4539](https://github.com/rex4539))
- add nightly release version to readme [\#11780](https://github.com/netdata/netdata/pull/11780) ([andrewm4894](https://github.com/andrewm4894))
- Delete from aclk alerts table if ack'ed from cloud one day ago [\#11779](https://github.com/netdata/netdata/pull/11779) ([MrZammler](https://github.com/MrZammler))
- Add Oracle Linux 8 to CI and package builds. [\#11776](https://github.com/netdata/netdata/pull/11776) ([Ferroin](https://github.com/Ferroin))
- Temporary fix for cgroup renaming [\#11775](https://github.com/netdata/netdata/pull/11775) ([vlvkobal](https://github.com/vlvkobal))
- Remove feature flag for ACLK new cloud architecture [\#11774](https://github.com/netdata/netdata/pull/11774) ([stelfrag](https://github.com/stelfrag))
- Fix link to new charts. [\#11773](https://github.com/netdata/netdata/pull/11773) ([DShreve2](https://github.com/DShreve2))
- Update netdata-security.md [\#11772](https://github.com/netdata/netdata/pull/11772) ([jlbriston](https://github.com/jlbriston))
- Skip sending hidden dimensions via ACLK [\#11770](https://github.com/netdata/netdata/pull/11770) ([stelfrag](https://github.com/stelfrag))
- Insert alert into aclk\_alert directly instead of queuing it [\#11769](https://github.com/netdata/netdata/pull/11769) ([MrZammler](https://github.com/MrZammler))
- Fix host hop count reported to the cloud [\#11768](https://github.com/netdata/netdata/pull/11768) ([stelfrag](https://github.com/stelfrag))
- Show stats for protected mount points in diskspace plugin [\#11767](https://github.com/netdata/netdata/pull/11767) ([vlvkobal](https://github.com/vlvkobal))
- Adding parenthesis [\#11766](https://github.com/netdata/netdata/pull/11766) ([ShimonOhayon](https://github.com/ShimonOhayon))
- fix log if D\_ACLK is used [\#11763](https://github.com/netdata/netdata/pull/11763) ([underhood](https://github.com/underhood))
- Don't interrupt popcorn timer for children [\#11758](https://github.com/netdata/netdata/pull/11758) ([underhood](https://github.com/underhood))
- fix \(cgroups.plugin\): containers name resolution for crio/containerd cri [\#11756](https://github.com/netdata/netdata/pull/11756) ([ilyam8](https://github.com/ilyam8))
- Add SSL\_MODE\_ENABLE\_PARTIAL\_WRITE to netdata\_srv\_ctx [\#11754](https://github.com/netdata/netdata/pull/11754) ([MrZammler](https://github.com/MrZammler))
- Update eBPF documenation \(Filesystem and HardIRQ\) [\#11752](https://github.com/netdata/netdata/pull/11752) ([UmanShahzad](https://github.com/UmanShahzad))
- Adds exit points between env and OTP [\#11751](https://github.com/netdata/netdata/pull/11751) ([underhood](https://github.com/underhood))
- Teach GH about ML label and its code owners. [\#11750](https://github.com/netdata/netdata/pull/11750) ([vkalintiris](https://github.com/vkalintiris))
- Update enable-streaming.mdx [\#11747](https://github.com/netdata/netdata/pull/11747) ([caleno](https://github.com/caleno))
- Minor improvement to CPU number function regarding macOS. [\#11746](https://github.com/netdata/netdata/pull/11746) ([iigorkarpov](https://github.com/iigorkarpov))
- minor - popocorn no more [\#11745](https://github.com/netdata/netdata/pull/11745) ([underhood](https://github.com/underhood))
- Update dashboard to version v2.20.11. [\#11743](https://github.com/netdata/netdata/pull/11743) ([netdatabot](https://github.com/netdatabot))
- Update eBPF documentation [\#11741](https://github.com/netdata/netdata/pull/11741) ([thiagoftsm](https://github.com/thiagoftsm))
- Change comma possition in v1/info if ml-info is missing [\#11739](https://github.com/netdata/netdata/pull/11739) ([MrZammler](https://github.com/MrZammler))

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

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.1...v1.23.2)

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
