# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.0...HEAD)

**Merged pull requests:**

- build\(deps\): bump github.com/redis/go-redis/v9 from 9.7.1 to 9.7.3 in /src/go [\#19926](https://github.com/netdata/netdata/pull/19926) ([dependabot[bot]](https://github.com/apps/dependabot))
- do not expose web server filenames [\#19925](https://github.com/netdata/netdata/pull/19925) ([ktsaou](https://github.com/ktsaou))
- Fix TOCTOU race in daemon status file handling. [\#19924](https://github.com/netdata/netdata/pull/19924) ([Ferroin](https://github.com/Ferroin))
- Exclude external code from CodeQL scanning. [\#19923](https://github.com/netdata/netdata/pull/19923) ([Ferroin](https://github.com/Ferroin))
- remove ilove endpoint [\#19919](https://github.com/netdata/netdata/pull/19919) ([ilyam8](https://github.com/ilyam8))
- Align cmsgbuf to size\_t to avoid unaligned memory access. [\#19917](https://github.com/netdata/netdata/pull/19917) ([vkalintiris](https://github.com/vkalintiris))
- Make sure ACLK sync thread completes initialization [\#19916](https://github.com/netdata/netdata/pull/19916) ([stelfrag](https://github.com/stelfrag))
- do not enqueue command if aclk is not initialized [\#19914](https://github.com/netdata/netdata/pull/19914) ([ktsaou](https://github.com/ktsaou))
- detect null datafile while finding datafiles in range [\#19913](https://github.com/netdata/netdata/pull/19913) ([ktsaou](https://github.com/ktsaou))
- post the first status when there is no last status [\#19912](https://github.com/netdata/netdata/pull/19912) ([ktsaou](https://github.com/ktsaou))
- fix reliability calculation [\#19909](https://github.com/netdata/netdata/pull/19909) ([ktsaou](https://github.com/ktsaou))
- new exit cause: shutdown timeout [\#19903](https://github.com/netdata/netdata/pull/19903) ([ktsaou](https://github.com/ktsaou))
- Store alert config asynchronously [\#19885](https://github.com/netdata/netdata/pull/19885) ([stelfrag](https://github.com/stelfrag))

## [v2.3.0](https://github.com/netdata/netdata/tree/v2.3.0) (2025-03-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.6...v2.3.0)

**Merged pull requests:**

- Remove auto-retry on changelog generation. [\#19908](https://github.com/netdata/netdata/pull/19908) ([Ferroin](https://github.com/Ferroin))
- Bump repoconfig version used in kickstart script to 5-1. [\#19906](https://github.com/netdata/netdata/pull/19906) ([Ferroin](https://github.com/Ferroin))
- Revert "Fix compile without dbengine" [\#19905](https://github.com/netdata/netdata/pull/19905) ([stelfrag](https://github.com/stelfrag))
- Fix compile without dbengine [\#19902](https://github.com/netdata/netdata/pull/19902) ([stelfrag](https://github.com/stelfrag))
- do not use errno when hashing status events [\#19900](https://github.com/netdata/netdata/pull/19900) ([ktsaou](https://github.com/ktsaou))
- more compilation flags for stack traces [\#19899](https://github.com/netdata/netdata/pull/19899) ([ktsaou](https://github.com/ktsaou))
- more strict checks on log-fw [\#19898](https://github.com/netdata/netdata/pull/19898) ([ktsaou](https://github.com/ktsaou))
- fix for system shutdown [\#19897](https://github.com/netdata/netdata/pull/19897) ([ktsaou](https://github.com/ktsaou))
- build: update otel deps to v0.122.0 [\#19895](https://github.com/netdata/netdata/pull/19895) ([ilyam8](https://github.com/ilyam8))
- do not recurse cleanup on shutdown [\#19894](https://github.com/netdata/netdata/pull/19894) ([ktsaou](https://github.com/ktsaou))
- make sure all rrdcalcs are unlinked the moment they are deleted [\#19893](https://github.com/netdata/netdata/pull/19893) ([ktsaou](https://github.com/ktsaou))
- Fix typo in README title [\#19891](https://github.com/netdata/netdata/pull/19891) ([felipecrs](https://github.com/felipecrs))
- remove deadlock from dyncfg health [\#19890](https://github.com/netdata/netdata/pull/19890) ([ktsaou](https://github.com/ktsaou))
- Update DEB/RPM package signing key info. [\#19888](https://github.com/netdata/netdata/pull/19888) ([Ferroin](https://github.com/Ferroin))
- fix\(go.d/snmp/ddsnmp\): correct profile directory path [\#19887](https://github.com/netdata/netdata/pull/19887) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp/ddsnmp\): use dd profile definition [\#19886](https://github.com/netdata/netdata/pull/19886) ([ilyam8](https://github.com/ilyam8))
- daemon status 18b [\#19884](https://github.com/netdata/netdata/pull/19884) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19883](https://github.com/netdata/netdata/pull/19883) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/snmp\): improve auto-detection section [\#19882](https://github.com/netdata/netdata/pull/19882) ([ilyam8](https://github.com/ilyam8))
- ci: use step-security/changed-files [\#19881](https://github.com/netdata/netdata/pull/19881) ([ilyam8](https://github.com/ilyam8))
- change log priorities on agent-events [\#19880](https://github.com/netdata/netdata/pull/19880) ([ktsaou](https://github.com/ktsaou))
- add stack trace information to the compiler and linker [\#19879](https://github.com/netdata/netdata/pull/19879) ([ktsaou](https://github.com/ktsaou))
- SIGABRT and already running are fatal conditions [\#19878](https://github.com/netdata/netdata/pull/19878) ([ktsaou](https://github.com/ktsaou))
- daemon-status-18 [\#19876](https://github.com/netdata/netdata/pull/19876) ([ktsaou](https://github.com/ktsaou))
- do not lose exit reasons [\#19875](https://github.com/netdata/netdata/pull/19875) ([ktsaou](https://github.com/ktsaou))
- make sure the daemon status hash does not depend on random bytes [\#19874](https://github.com/netdata/netdata/pull/19874) ([ktsaou](https://github.com/ktsaou))
- add the fatal to the exit reasons [\#19873](https://github.com/netdata/netdata/pull/19873) ([ktsaou](https://github.com/ktsaou))
- sentry events annotations [\#19872](https://github.com/netdata/netdata/pull/19872) ([ktsaou](https://github.com/ktsaou))
- Remove tj-actions/changed-files from CI jobs. [\#19870](https://github.com/netdata/netdata/pull/19870) ([Ferroin](https://github.com/Ferroin))
- daemon status file 17 [\#19869](https://github.com/netdata/netdata/pull/19869) ([ktsaou](https://github.com/ktsaou))
- fixed sentry version [\#19868](https://github.com/netdata/netdata/pull/19868) ([ktsaou](https://github.com/ktsaou))
- fixed sentry dedup [\#19867](https://github.com/netdata/netdata/pull/19867) ([ktsaou](https://github.com/ktsaou))
- fix\(freebsd.plugin\): correct disks/network devices charts [\#19866](https://github.com/netdata/netdata/pull/19866) ([ilyam8](https://github.com/ilyam8))
- improvement\(macos.plugin\): add options to filter net ifaces and mountpoints [\#19865](https://github.com/netdata/netdata/pull/19865) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/prometheus/common from 0.62.0 to 0.63.0 in /src/go [\#19864](https://github.com/netdata/netdata/pull/19864) ([dependabot[bot]](https://github.com/apps/dependabot))
- daemon status file 16 [\#19863](https://github.com/netdata/netdata/pull/19863) ([ktsaou](https://github.com/ktsaou))
- Release memory on shutdown - detect invalid extent in journal files [\#19861](https://github.com/netdata/netdata/pull/19861) ([stelfrag](https://github.com/stelfrag))
- restore needed variables for pluginsd [\#19860](https://github.com/netdata/netdata/pull/19860) ([ktsaou](https://github.com/ktsaou))
- fix\(macos.plugin\): correct disks/network devices charts [\#19859](https://github.com/netdata/netdata/pull/19859) ([ilyam8](https://github.com/ilyam8))
- disable UNW\_LOCAL\_ONLY on static builds [\#19858](https://github.com/netdata/netdata/pull/19858) ([ktsaou](https://github.com/ktsaou))
- daemon status 15 [\#19857](https://github.com/netdata/netdata/pull/19857) ([ktsaou](https://github.com/ktsaou))
- fix crashes identified by sentry [\#19856](https://github.com/netdata/netdata/pull/19856) ([ktsaou](https://github.com/ktsaou))
- netdata-uninstaller: improve input prompt with more descriptive guidance [\#19855](https://github.com/netdata/netdata/pull/19855) ([ilyam8](https://github.com/ilyam8))
- make sure alerts are concurrently altered by dyncfg [\#19854](https://github.com/netdata/netdata/pull/19854) ([ktsaou](https://github.com/ktsaou))
- fix contexts labels to avoid clearing the rrdlabels pointer [\#19853](https://github.com/netdata/netdata/pull/19853) ([ktsaou](https://github.com/ktsaou))
- fix updating on RPi2+ [\#19850](https://github.com/netdata/netdata/pull/19850) ([ilyam8](https://github.com/ilyam8))
- minor fixes [\#19849](https://github.com/netdata/netdata/pull/19849) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump k8s.io/client-go from 0.32.2 to 0.32.3 in /src/go [\#19848](https://github.com/netdata/netdata/pull/19848) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.48.1 to 0.49.0 in /src/go [\#19845](https://github.com/netdata/netdata/pull/19845) ([dependabot[bot]](https://github.com/apps/dependabot))
- docs: fix typos in nodes-ephemerality.md [\#19840](https://github.com/netdata/netdata/pull/19840) ([ilyam8](https://github.com/ilyam8))
- Add oci meta info [\#19839](https://github.com/netdata/netdata/pull/19839) ([Passific](https://github.com/Passific))
- fix rrdset name crash on cleanup [\#19838](https://github.com/netdata/netdata/pull/19838) ([ktsaou](https://github.com/ktsaou))
- when destroying pgc, check if the cache is null [\#19837](https://github.com/netdata/netdata/pull/19837) ([ktsaou](https://github.com/ktsaou))
- Fix for building with protobuf 30.0 [\#19835](https://github.com/netdata/netdata/pull/19835) ([vkalintiris](https://github.com/vkalintiris))
- Improve CI reliability by allowing for better retry behavior. [\#19834](https://github.com/netdata/netdata/pull/19834) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19833](https://github.com/netdata/netdata/pull/19833) ([netdatabot](https://github.com/netdatabot))
- Fix typo in otel collector build infra. [\#19832](https://github.com/netdata/netdata/pull/19832) ([Ferroin](https://github.com/Ferroin))
- store status file in /var/lib/netdata, not in /var/cache/netdata [\#19831](https://github.com/netdata/netdata/pull/19831) ([ktsaou](https://github.com/ktsaou))
- Fix RRDDIM\_MEM storage engine index [\#19830](https://github.com/netdata/netdata/pull/19830) ([ktsaou](https://github.com/ktsaou))
- improvement\(go.d/k8state\): add CronJob suspend status [\#19829](https://github.com/netdata/netdata/pull/19829) ([ilyam8](https://github.com/ilyam8))
- Revert "fix rrdset name crash on rrdset obsoletion" [\#19828](https://github.com/netdata/netdata/pull/19828) ([ktsaou](https://github.com/ktsaou))
- free strings judy arrays to show leaked strings [\#19827](https://github.com/netdata/netdata/pull/19827) ([ktsaou](https://github.com/ktsaou))
- rrdhost name fix heap-use-after-free [\#19826](https://github.com/netdata/netdata/pull/19826) ([ktsaou](https://github.com/ktsaou))
- use notice log level for "machine ID found" [\#19825](https://github.com/netdata/netdata/pull/19825) ([ilyam8](https://github.com/ilyam8))
- build\(otel-collector\): update to v0.121.0 [\#19824](https://github.com/netdata/netdata/pull/19824) ([ilyam8](https://github.com/ilyam8))
- Finding leaks No 2 [\#19823](https://github.com/netdata/netdata/pull/19823) ([ktsaou](https://github.com/ktsaou))
- Free all memory on exit [\#19821](https://github.com/netdata/netdata/pull/19821) ([ktsaou](https://github.com/ktsaou))
- Fix LSAN and memory leaks [\#19819](https://github.com/netdata/netdata/pull/19819) ([ktsaou](https://github.com/ktsaou))
- Include libucontext in static builds to vendor libunwind even on POWER. [\#19817](https://github.com/netdata/netdata/pull/19817) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19816](https://github.com/netdata/netdata/pull/19816) ([netdatabot](https://github.com/netdatabot))
- fix\(go.d/filecheck\): remove dyncfg path validation pattern  [\#19815](https://github.com/netdata/netdata/pull/19815) ([ilyam8](https://github.com/ilyam8))
- Initial commit with snmp profile code [\#19813](https://github.com/netdata/netdata/pull/19813) ([Ancairon](https://github.com/Ancairon))
- Acquire datafile for deletion before calculating retention [\#19812](https://github.com/netdata/netdata/pull/19812) ([stelfrag](https://github.com/stelfrag))
- Detect memory leaks [\#19811](https://github.com/netdata/netdata/pull/19811) ([ktsaou](https://github.com/ktsaou))
- Avoid zero timeout in libuv timers [\#19810](https://github.com/netdata/netdata/pull/19810) ([stelfrag](https://github.com/stelfrag))
- fix fsanitize ifdefs [\#19809](https://github.com/netdata/netdata/pull/19809) ([ktsaou](https://github.com/ktsaou))
- do not change the scheduling policy by default [\#19808](https://github.com/netdata/netdata/pull/19808) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/pihole\): switch to pihole6 api [\#19807](https://github.com/netdata/netdata/pull/19807) ([ilyam8](https://github.com/ilyam8))
- Help finding leaks and running valgrind [\#19806](https://github.com/netdata/netdata/pull/19806) ([ktsaou](https://github.com/ktsaou))
- fix memory corruption in streaming [\#19805](https://github.com/netdata/netdata/pull/19805) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19804](https://github.com/netdata/netdata/pull/19804) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations docs [\#19803](https://github.com/netdata/netdata/pull/19803) ([netdatabot](https://github.com/netdatabot))
- async-signal-safe stack traces [\#19802](https://github.com/netdata/netdata/pull/19802) ([ktsaou](https://github.com/ktsaou))
- add k8s\_state\_cronjob\_last\_execution\_failed alert [\#19801](https://github.com/netdata/netdata/pull/19801) ([ilyam8](https://github.com/ilyam8))
- bump dag jinja to 3.1.6 [\#19800](https://github.com/netdata/netdata/pull/19800) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump golang.org/x/net from 0.35.0 to 0.37.0 in /src/go [\#19799](https://github.com/netdata/netdata/pull/19799) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations docs [\#19797](https://github.com/netdata/netdata/pull/19797) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8s\_state\): add more CronJob metrics [\#19796](https://github.com/netdata/netdata/pull/19796) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19794](https://github.com/netdata/netdata/pull/19794) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8s\_state\): collect cronjobs [\#19793](https://github.com/netdata/netdata/pull/19793) ([ilyam8](https://github.com/ilyam8))
- status file improvements 12 [\#19792](https://github.com/netdata/netdata/pull/19792) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19791](https://github.com/netdata/netdata/pull/19791) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/snmp\): add snmp discovery information [\#19790](https://github.com/netdata/netdata/pull/19790) ([ilyam8](https://github.com/ilyam8))
- User configurable crash reporting [\#19789](https://github.com/netdata/netdata/pull/19789) ([ktsaou](https://github.com/ktsaou))
- detect when running in CI and disable posting status [\#19787](https://github.com/netdata/netdata/pull/19787) ([ktsaou](https://github.com/ktsaou))
- chore: rename snmp.profiles.d -\> snmp.profiles [\#19786](https://github.com/netdata/netdata/pull/19786) ([ilyam8](https://github.com/ilyam8))
- add datadog profiles for snmp collector [\#19785](https://github.com/netdata/netdata/pull/19785) ([Ancairon](https://github.com/Ancairon))
- Revert broken DEB priority configuration in repoconfig packages. [\#19783](https://github.com/netdata/netdata/pull/19783) ([Ferroin](https://github.com/Ferroin))
- Restructure shutdown logic used during updates. [\#19781](https://github.com/netdata/netdata/pull/19781) ([Ferroin](https://github.com/Ferroin))
- add unique machine id to status file [\#19778](https://github.com/netdata/netdata/pull/19778) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/sd\): fix logging cfg source when disabled [\#19777](https://github.com/netdata/netdata/pull/19777) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/sd\): add file path to k8s/snmp discovered job source [\#19776](https://github.com/netdata/netdata/pull/19776) ([ilyam8](https://github.com/ilyam8))
- Improve agent shutdown [\#19775](https://github.com/netdata/netdata/pull/19775) ([stelfrag](https://github.com/stelfrag))
- Fix SIGSEGV on static installs due to dengine log [\#19774](https://github.com/netdata/netdata/pull/19774) ([ktsaou](https://github.com/ktsaou))
- kickstart: install native pkg on RPi2+ [\#19773](https://github.com/netdata/netdata/pull/19773) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/sd\): rename discoverers pkgs [\#19772](https://github.com/netdata/netdata/pull/19772) ([ilyam8](https://github.com/ilyam8))
- block signals before curl [\#19771](https://github.com/netdata/netdata/pull/19771) ([ktsaou](https://github.com/ktsaou))
- block all signals before spawning any threads [\#19770](https://github.com/netdata/netdata/pull/19770) ([ktsaou](https://github.com/ktsaou))
- add handling for sigabrt in the status file [\#19769](https://github.com/netdata/netdata/pull/19769) ([ktsaou](https://github.com/ktsaou))
- copy fields only when the source is valid [\#19768](https://github.com/netdata/netdata/pull/19768) ([ktsaou](https://github.com/ktsaou))
- detect crashes during status file processing [\#19767](https://github.com/netdata/netdata/pull/19767) ([ktsaou](https://github.com/ktsaou))
- post status syncrhonously [\#19766](https://github.com/netdata/netdata/pull/19766) ([ktsaou](https://github.com/ktsaou))
- enable libunwind in static builds [\#19764](https://github.com/netdata/netdata/pull/19764) ([ktsaou](https://github.com/ktsaou))
- fix invalid free [\#19763](https://github.com/netdata/netdata/pull/19763) ([ktsaou](https://github.com/ktsaou))
- make status file use fixed size character arrays [\#19761](https://github.com/netdata/netdata/pull/19761) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/sd/snmp\): use rescan and cache ttl only when set [\#19760](https://github.com/netdata/netdata/pull/19760) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/nvidia\_smi\): handle xml gpu\_power\_readings change [\#19759](https://github.com/netdata/netdata/pull/19759) ([ilyam8](https://github.com/ilyam8))
- status file timings per step [\#19758](https://github.com/netdata/netdata/pull/19758) ([ktsaou](https://github.com/ktsaou))
- improvement\(go.d/sd/snmp\): support device cache ttl 0 [\#19756](https://github.com/netdata/netdata/pull/19756) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/sd/snmp\): comment out  defaults in snmp.conf [\#19755](https://github.com/netdata/netdata/pull/19755) ([ilyam8](https://github.com/ilyam8))
- Add documentation outlining how to use custom CA certificates with Netdata. [\#19754](https://github.com/netdata/netdata/pull/19754) ([Ferroin](https://github.com/Ferroin))
- status file version 8 [\#19753](https://github.com/netdata/netdata/pull/19753) ([ktsaou](https://github.com/ktsaou))
- status file improvements \(dedup and signal handler use\) [\#19751](https://github.com/netdata/netdata/pull/19751) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/axiomhq/hyperloglog from 0.2.3 to 0.2.5 in /src/go [\#19750](https://github.com/netdata/netdata/pull/19750) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/likexian/whois from 1.15.5 to 1.15.6 in /src/go [\#19749](https://github.com/netdata/netdata/pull/19749) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump go.mongodb.org/mongo-driver from 1.17.2 to 1.17.3 in /src/go [\#19748](https://github.com/netdata/netdata/pull/19748) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/gosnmp/gosnmp from 1.38.0 to 1.39.0 in /src/go [\#19747](https://github.com/netdata/netdata/pull/19747) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.0.0+incompatible to 28.0.1+incompatible in /src/go [\#19746](https://github.com/netdata/netdata/pull/19746) ([dependabot[bot]](https://github.com/apps/dependabot))
- more strict parsing of the output of system-info.sh [\#19745](https://github.com/netdata/netdata/pull/19745) ([ktsaou](https://github.com/ktsaou))
- pass NULL to sensors\_init\(\) when the standard files exist in /etc/ [\#19744](https://github.com/netdata/netdata/pull/19744) ([ktsaou](https://github.com/ktsaou))
- allow coredumps to be generated [\#19743](https://github.com/netdata/netdata/pull/19743) ([ktsaou](https://github.com/ktsaou))
- work on agent-events crashes [\#19741](https://github.com/netdata/netdata/pull/19741) ([ktsaou](https://github.com/ktsaou))
- zero mtime when a fallback check fails [\#19740](https://github.com/netdata/netdata/pull/19740) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d\): ignore sigpipe to exit gracefully [\#19739](https://github.com/netdata/netdata/pull/19739) ([ilyam8](https://github.com/ilyam8))
- Capture deadly signals [\#19737](https://github.com/netdata/netdata/pull/19737) ([ktsaou](https://github.com/ktsaou))
- allow insecure cloud connections [\#19736](https://github.com/netdata/netdata/pull/19736) ([ktsaou](https://github.com/ktsaou))
- add more information about claiming failures [\#19735](https://github.com/netdata/netdata/pull/19735) ([ktsaou](https://github.com/ktsaou))
- support https\_proxy too [\#19733](https://github.com/netdata/netdata/pull/19733) ([ktsaou](https://github.com/ktsaou))
- fix json generation of apps.plugin processes function info [\#19732](https://github.com/netdata/netdata/pull/19732) ([ktsaou](https://github.com/ktsaou))
- add another step when initializing web [\#19731](https://github.com/netdata/netdata/pull/19731) ([ktsaou](https://github.com/ktsaou))
- improved descriptions of exit reasons [\#19730](https://github.com/netdata/netdata/pull/19730) ([ktsaou](https://github.com/ktsaou))
- do not post empty reports [\#19729](https://github.com/netdata/netdata/pull/19729) ([ktsaou](https://github.com/ktsaou))
- docs: clarify Windows Agent limits on free plans [\#19727](https://github.com/netdata/netdata/pull/19727) ([ilyam8](https://github.com/ilyam8))
- improve status file deduplication [\#19726](https://github.com/netdata/netdata/pull/19726) ([ktsaou](https://github.com/ktsaou))
- handle flushing state during exit [\#19725](https://github.com/netdata/netdata/pull/19725) ([ktsaou](https://github.com/ktsaou))
- allow configuring journal v2 unmount time; turn it off for parents [\#19724](https://github.com/netdata/netdata/pull/19724) ([ktsaou](https://github.com/ktsaou))
- minor status file annotation fixes [\#19723](https://github.com/netdata/netdata/pull/19723) ([ktsaou](https://github.com/ktsaou))
- status has install type [\#19722](https://github.com/netdata/netdata/pull/19722) ([ktsaou](https://github.com/ktsaou))
- more status file annotations [\#19721](https://github.com/netdata/netdata/pull/19721) ([ktsaou](https://github.com/ktsaou))
- feat\(go.d\): add snmp devices discovery [\#19720](https://github.com/netdata/netdata/pull/19720) ([ilyam8](https://github.com/ilyam8))
- save status on out of memory event [\#19719](https://github.com/netdata/netdata/pull/19719) ([ktsaou](https://github.com/ktsaou))
- attempt to save status file from the signal handler [\#19718](https://github.com/netdata/netdata/pull/19718) ([ktsaou](https://github.com/ktsaou))
- unified out of memory handling [\#19717](https://github.com/netdata/netdata/pull/19717) ([ktsaou](https://github.com/ktsaou))
- chore\(go.d\): add file persister [\#19716](https://github.com/netdata/netdata/pull/19716) ([ilyam8](https://github.com/ilyam8))
- do not call cleanup and exit on fatal conditions during startup [\#19715](https://github.com/netdata/netdata/pull/19715) ([ktsaou](https://github.com/ktsaou))
- do not use mmap when the mmap limit is too low [\#19714](https://github.com/netdata/netdata/pull/19714) ([ktsaou](https://github.com/ktsaou))
- systemd-journal: allow almost all fields to be facets [\#19713](https://github.com/netdata/netdata/pull/19713) ([ktsaou](https://github.com/ktsaou))
- deduplicate all crash reports [\#19712](https://github.com/netdata/netdata/pull/19712) ([ktsaou](https://github.com/ktsaou))
- 4 malloc arenas for parents, not IoT [\#19711](https://github.com/netdata/netdata/pull/19711) ([ktsaou](https://github.com/ktsaou))
- Fix Fresh Installation on Microsoft [\#19710](https://github.com/netdata/netdata/pull/19710) ([thiagoftsm](https://github.com/thiagoftsm))
- Avoid post initialization errors repeateadly [\#19709](https://github.com/netdata/netdata/pull/19709) ([ktsaou](https://github.com/ktsaou))
- Check for final step [\#19708](https://github.com/netdata/netdata/pull/19708) ([stelfrag](https://github.com/stelfrag))
- daemon status improvements 3 [\#19707](https://github.com/netdata/netdata/pull/19707) ([ktsaou](https://github.com/ktsaou))
- fix runtime directory; annotate daemon status file [\#19706](https://github.com/netdata/netdata/pull/19706) ([ktsaou](https://github.com/ktsaou))
- Add repository priority configuration for DEB package repositories. [\#19705](https://github.com/netdata/netdata/pull/19705) ([Ferroin](https://github.com/Ferroin))
- add host/os fields to status file [\#19704](https://github.com/netdata/netdata/pull/19704) ([ktsaou](https://github.com/ktsaou))
- under MSYS2 use stat [\#19703](https://github.com/netdata/netdata/pull/19703) ([ktsaou](https://github.com/ktsaou))
- Integrate OpenTelemetry collector build into build system. [\#19702](https://github.com/netdata/netdata/pull/19702) ([Ferroin](https://github.com/Ferroin))
- Document journal v2 index file format. [\#19701](https://github.com/netdata/netdata/pull/19701) ([vkalintiris](https://github.com/vkalintiris))
- build\(deps\): update go.d packages [\#19700](https://github.com/netdata/netdata/pull/19700) ([ilyam8](https://github.com/ilyam8))
- ADFS \(windows.plugin\) [\#19699](https://github.com/netdata/netdata/pull/19699) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/sijms/go-ora/v2 from 2.8.23 to 2.8.24 in /src/go [\#19698](https://github.com/netdata/netdata/pull/19698) ([dependabot[bot]](https://github.com/apps/dependabot))
- change the moto and the description of netdata [\#19696](https://github.com/netdata/netdata/pull/19696) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.7.0 to 9.7.1 in /src/go [\#19693](https://github.com/netdata/netdata/pull/19693) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 27.5.1+incompatible to 28.0.0+incompatible in /src/go [\#19692](https://github.com/netdata/netdata/pull/19692) ([dependabot[bot]](https://github.com/apps/dependabot))
- load health config before creating localhost [\#19689](https://github.com/netdata/netdata/pull/19689) ([ktsaou](https://github.com/ktsaou))
- chore\(go.d/pkg/iprange\): add iterator [\#19688](https://github.com/netdata/netdata/pull/19688) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/mysql\): InnodbOSLogIO in MariaDB \>= 10.8 [\#19687](https://github.com/netdata/netdata/pull/19687) ([arkamar](https://github.com/arkamar))
- Switch back to x86 hosts for POWER8+ builds. [\#19686](https://github.com/netdata/netdata/pull/19686) ([Ferroin](https://github.com/Ferroin))
- allow parsing empty json arrays and objects [\#19685](https://github.com/netdata/netdata/pull/19685) ([ktsaou](https://github.com/ktsaou))
- improve dyncfg src type anon message [\#19684](https://github.com/netdata/netdata/pull/19684) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/mysql\): handle Cpu\_time in microseconds in v10.11.11+ [\#19683](https://github.com/netdata/netdata/pull/19683) ([ilyam8](https://github.com/ilyam8))
- build: change go.mod version to 1.23.4 to fix win ci builds [\#19681](https://github.com/netdata/netdata/pull/19681) ([ilyam8](https://github.com/ilyam8))
- build: change go.mod version to 1.23.6 [\#19680](https://github.com/netdata/netdata/pull/19680) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/go-sql-driver/mysql from 1.8.1 to 1.9.0 in /src/go [\#19679](https://github.com/netdata/netdata/pull/19679) ([dependabot[bot]](https://github.com/apps/dependabot))
- initial setup of custom OpenTelemetry Collector distribution [\#19678](https://github.com/netdata/netdata/pull/19678) ([ilyam8](https://github.com/ilyam8))
- Fix freebsd compilation [\#19677](https://github.com/netdata/netdata/pull/19677) ([stelfrag](https://github.com/stelfrag))
- test\(go.d dyncfg\): fix tests [\#19676](https://github.com/netdata/netdata/pull/19676) ([ilyam8](https://github.com/ilyam8))
- Dyncfg users actions log [\#19674](https://github.com/netdata/netdata/pull/19674) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d dyncfg\): don't overwrite source [\#19673](https://github.com/netdata/netdata/pull/19673) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d dyncfg\): log collector dyncfg actions [\#19672](https://github.com/netdata/netdata/pull/19672) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/k8sstate\): correct deployment conditions [\#19671](https://github.com/netdata/netdata/pull/19671) ([ilyam8](https://github.com/ilyam8))
- chore: remove netdata\_configured\_lock\_dir [\#19669](https://github.com/netdata/netdata/pull/19669) ([ilyam8](https://github.com/ilyam8))
- chore: remove lock files from go.d/python.d [\#19668](https://github.com/netdata/netdata/pull/19668) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/sensors\): disable by default [\#19667](https://github.com/netdata/netdata/pull/19667) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d dyncfg\): add user to source [\#19666](https://github.com/netdata/netdata/pull/19666) ([ilyam8](https://github.com/ilyam8))
- add k8s\_state\_deployment\_condition\_available alert [\#19664](https://github.com/netdata/netdata/pull/19664) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19663](https://github.com/netdata/netdata/pull/19663) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8sstate\): add deployment conditions [\#19662](https://github.com/netdata/netdata/pull/19662) ([ilyam8](https://github.com/ilyam8))
- avoid dbengine event loop starvation by running uv\_run periodically [\#19661](https://github.com/netdata/netdata/pull/19661) ([ktsaou](https://github.com/ktsaou))
- speed up aral when a single item is allocated and freed repeateadly [\#19660](https://github.com/netdata/netdata/pull/19660) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19658](https://github.com/netdata/netdata/pull/19658) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8sstate\): collect deployments [\#19657](https://github.com/netdata/netdata/pull/19657) ([ilyam8](https://github.com/ilyam8))
- add agent timezones as host labels [\#19656](https://github.com/netdata/netdata/pull/19656) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump k8s.io/client-go from 0.32.1 to 0.32.2 in /src/go [\#19652](https://github.com/netdata/netdata/pull/19652) ([dependabot[bot]](https://github.com/apps/dependabot))
- make onewayalloc fallback to malloc [\#19646](https://github.com/netdata/netdata/pull/19646) ([ktsaou](https://github.com/ktsaou))
- docs: move /run/dbus mount to Docker recommended way [\#19645](https://github.com/netdata/netdata/pull/19645) ([ilyam8](https://github.com/ilyam8))
- Fix native package installation on RHEL. [\#19643](https://github.com/netdata/netdata/pull/19643) ([Ferroin](https://github.com/Ferroin))
- ci: fix win build [\#19642](https://github.com/netdata/netdata/pull/19642) ([ilyam8](https://github.com/ilyam8))
- fix windows logs 2 - do not renumber - append fields [\#19640](https://github.com/netdata/netdata/pull/19640) ([ktsaou](https://github.com/ktsaou))
- Revert "fix windows logs" [\#19639](https://github.com/netdata/netdata/pull/19639) ([ktsaou](https://github.com/ktsaou))
- add Group=netdata to systemd unit file [\#19638](https://github.com/netdata/netdata/pull/19638) ([ilyam8](https://github.com/ilyam8))
- docs: add missing prop to graphite meta [\#19637](https://github.com/netdata/netdata/pull/19637) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19636](https://github.com/netdata/netdata/pull/19636) ([netdatabot](https://github.com/netdatabot))
- docs\(exporting\): clarify graphite exporters [\#19635](https://github.com/netdata/netdata/pull/19635) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19634](https://github.com/netdata/netdata/pull/19634) ([netdatabot](https://github.com/netdatabot))
- docs\(exporting\): remove influxdb \(via graphite\) exporter [\#19633](https://github.com/netdata/netdata/pull/19633) ([ilyam8](https://github.com/ilyam8))
- fix windows logs [\#19632](https://github.com/netdata/netdata/pull/19632) ([ktsaou](https://github.com/ktsaou))
- more perflib error checking [\#19631](https://github.com/netdata/netdata/pull/19631) ([ktsaou](https://github.com/ktsaou))
- Revert "HyperV Adjusts \(windows.plugin\)" [\#19630](https://github.com/netdata/netdata/pull/19630) ([ilyam8](https://github.com/ilyam8))
- do not send sentry reports on rrd\_init\(\) failures [\#19628](https://github.com/netdata/netdata/pull/19628) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump golang.org/x/net from 0.34.0 to 0.35.0 in /src/go [\#19626](https://github.com/netdata/netdata/pull/19626) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.48.0 to 0.48.1 in /src/go [\#19625](https://github.com/netdata/netdata/pull/19625) ([dependabot[bot]](https://github.com/apps/dependabot))
- feat\(health\): add system\_reboot\_detection alarm [\#19624](https://github.com/netdata/netdata/pull/19624) ([ilyam8](https://github.com/ilyam8))
- HyperV Adjusts \(windows.plugin\) [\#19623](https://github.com/netdata/netdata/pull/19623) ([thiagoftsm](https://github.com/thiagoftsm))
- detect the system ca bundle at runtime [\#19622](https://github.com/netdata/netdata/pull/19622) ([ktsaou](https://github.com/ktsaou))
- Switch to Ubuntu 22.04 runner images for CI build jobs. [\#19619](https://github.com/netdata/netdata/pull/19619) ([Ferroin](https://github.com/Ferroin))
- fix\(go.d/mysql\): handle Cpu\_time in microseconds in v11.4.5+ [\#19618](https://github.com/netdata/netdata/pull/19618) ([ilyam8](https://github.com/ilyam8))
- detect netdata exit reasons [\#19617](https://github.com/netdata/netdata/pull/19617) ([ktsaou](https://github.com/ktsaou))
- improvement\(health\): clarify clickhouse\_replicated\_readonly\_tables info [\#19616](https://github.com/netdata/netdata/pull/19616) ([ilyam8](https://github.com/ilyam8))
- fix: correct typo in NetdataCompilerFlags [\#19614](https://github.com/netdata/netdata/pull/19614) ([ilyam8](https://github.com/ilyam8))
- chore: remove fluentbit.log from Dockerfile [\#19613](https://github.com/netdata/netdata/pull/19613) ([ilyam8](https://github.com/ilyam8))
- Allow indirect access when agent is claimed, but offline \(indirect cloud connectivity\) [\#19611](https://github.com/netdata/netdata/pull/19611) ([ktsaou](https://github.com/ktsaou))
- silence new alerts [\#19610](https://github.com/netdata/netdata/pull/19610) ([ktsaou](https://github.com/ktsaou))
- Do not register removed node on agent restart [\#19609](https://github.com/netdata/netdata/pull/19609) ([stelfrag](https://github.com/stelfrag))
- Add sentry fatal message breadcrumb. [\#19608](https://github.com/netdata/netdata/pull/19608) ([vkalintiris](https://github.com/vkalintiris))
- Disable LTO for openSUSE package builds. [\#19607](https://github.com/netdata/netdata/pull/19607) ([Ferroin](https://github.com/Ferroin))
- add interpolation to median and percentile [\#19606](https://github.com/netdata/netdata/pull/19606) ([ktsaou](https://github.com/ktsaou))
- docs: reword nodes-ephemerality for clarity [\#19604](https://github.com/netdata/netdata/pull/19604) ([ilyam8](https://github.com/ilyam8))
- cleanup hosts - leftover code [\#19603](https://github.com/netdata/netdata/pull/19603) ([ktsaou](https://github.com/ktsaou))
- make remove-stale-node remove also ephemeral nodes [\#19602](https://github.com/netdata/netdata/pull/19602) ([ktsaou](https://github.com/ktsaou))
- Update manage-notification-methods.md [\#19601](https://github.com/netdata/netdata/pull/19601) ([Ancairon](https://github.com/Ancairon))
- Close database if we encounter error during startup [\#19600](https://github.com/netdata/netdata/pull/19600) ([stelfrag](https://github.com/stelfrag))
- dequeue from hub before deleting contexts [\#19599](https://github.com/netdata/netdata/pull/19599) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/gohugoio/hashstructure from 0.3.0 to 0.5.0 in /src/go [\#19598](https://github.com/netdata/netdata/pull/19598) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/text from 0.21.0 to 0.22.0 in /src/go [\#19597](https://github.com/netdata/netdata/pull/19597) ([dependabot[bot]](https://github.com/apps/dependabot))
- Cleanup code that writes extents to the database [\#19596](https://github.com/netdata/netdata/pull/19596) ([stelfrag](https://github.com/stelfrag))
- Add check for available active instances when checking for extreme cardinality [\#19594](https://github.com/netdata/netdata/pull/19594) ([stelfrag](https://github.com/stelfrag))
- Free resources where writing datafile extents [\#19593](https://github.com/netdata/netdata/pull/19593) ([stelfrag](https://github.com/stelfrag))
- fix incomplete implementation of journal watcher [\#19592](https://github.com/netdata/netdata/pull/19592) ([ktsaou](https://github.com/ktsaou))
- docs\(health\): clarify "special user of the cond operator" p2 [\#19590](https://github.com/netdata/netdata/pull/19590) ([ilyam8](https://github.com/ilyam8))
- docs\(health\): clarify "special user of the cond operator" [\#19589](https://github.com/netdata/netdata/pull/19589) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19588](https://github.com/netdata/netdata/pull/19588) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/zookeeper\): fix ZooKeeper server scope name [\#19587](https://github.com/netdata/netdata/pull/19587) ([ilyam8](https://github.com/ilyam8))
- Streaming alerts [\#19586](https://github.com/netdata/netdata/pull/19586) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19585](https://github.com/netdata/netdata/pull/19585) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/zookeeper\): add more metrics [\#19584](https://github.com/netdata/netdata/pull/19584) ([ilyam8](https://github.com/ilyam8))
- Add agent version during ACLK handshake [\#19583](https://github.com/netdata/netdata/pull/19583) ([stelfrag](https://github.com/stelfrag))
- Format missing file \(eBPF.plugin\) [\#19582](https://github.com/netdata/netdata/pull/19582) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(go.d/apache\): make ?auto param check non-fatal [\#19580](https://github.com/netdata/netdata/pull/19580) ([ilyam8](https://github.com/ilyam8))
- Fix static build conditions to run on release and nightly builds. [\#19579](https://github.com/netdata/netdata/pull/19579) ([Ferroin](https://github.com/Ferroin))
- build\(deps\): update go toolchain to v1.23.6 [\#19578](https://github.com/netdata/netdata/pull/19578) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/nvme\): add missing "/dev/" prefix to device path for v2.11 [\#19577](https://github.com/netdata/netdata/pull/19577) ([ilyam8](https://github.com/ilyam8))
- Generate protobuf source files in build dir. [\#19576](https://github.com/netdata/netdata/pull/19576) ([vkalintiris](https://github.com/vkalintiris))
- Switch from x86 to ARM build host for POWER8+ builds. [\#19575](https://github.com/netdata/netdata/pull/19575) ([Ferroin](https://github.com/Ferroin))
- fix\(go.d\): clean up charts for stopped and removed jobs [\#19573](https://github.com/netdata/netdata/pull/19573) ([ilyam8](https://github.com/ilyam8))
- Modify eBPF.plugin integration \(Part II, the sockets\) [\#19572](https://github.com/netdata/netdata/pull/19572) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix memory leak [\#19569](https://github.com/netdata/netdata/pull/19569) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/prometheus-community/pro-bing from 0.6.0 to 0.6.1 in /src/go [\#19567](https://github.com/netdata/netdata/pull/19567) ([dependabot[bot]](https://github.com/apps/dependabot))
- Code cleanup on ACLK messages [\#19566](https://github.com/netdata/netdata/pull/19566) ([stelfrag](https://github.com/stelfrag))
- Add a new agent status when connecting to the cloud [\#19564](https://github.com/netdata/netdata/pull/19564) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#19563](https://github.com/netdata/netdata/pull/19563) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/dnsquery\): support system DNS servers from /etc/resolv.conf [\#19562](https://github.com/netdata/netdata/pull/19562) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19561](https://github.com/netdata/netdata/pull/19561) ([netdatabot](https://github.com/netdatabot))
- MSSQL Multiple Instances \(windows.plugin\) [\#19559](https://github.com/netdata/netdata/pull/19559) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/lmittmann/tint from 1.0.6 to 1.0.7 in /src/go [\#19558](https://github.com/netdata/netdata/pull/19558) ([dependabot[bot]](https://github.com/apps/dependabot))
- Metadata \(AD and ADCS\), and small fixes [\#19557](https://github.com/netdata/netdata/pull/19557) ([thiagoftsm](https://github.com/thiagoftsm))
- docs\(start-stop-restart\): fix restart typo [\#19555](https://github.com/netdata/netdata/pull/19555) ([L-U-C-K-Y](https://github.com/L-U-C-K-Y))
- Format Windows.plugin [\#19554](https://github.com/netdata/netdata/pull/19554) ([thiagoftsm](https://github.com/thiagoftsm))
- Format ebpf [\#19553](https://github.com/netdata/netdata/pull/19553) ([thiagoftsm](https://github.com/thiagoftsm))
- Rename appconfig to inicfg and drop config\_\* function-like macros. [\#19552](https://github.com/netdata/netdata/pull/19552) ([vkalintiris](https://github.com/vkalintiris))
- fix\(go.d/mysql\): fix typo in test name [\#19550](https://github.com/netdata/netdata/pull/19550) ([arkamar](https://github.com/arkamar))
- fix\(go.d/mysql\): don't collect global variables on every iteration [\#19549](https://github.com/netdata/netdata/pull/19549) ([arkamar](https://github.com/arkamar))
- Regenerate integrations docs [\#19548](https://github.com/netdata/netdata/pull/19548) ([netdatabot](https://github.com/netdatabot))
- Fix cloud connect after claim [\#19547](https://github.com/netdata/netdata/pull/19547) ([stelfrag](https://github.com/stelfrag))
- virtual hosts now get hops = 1 [\#19546](https://github.com/netdata/netdata/pull/19546) ([ktsaou](https://github.com/ktsaou))
- chore: remove old dashboard leftovers [\#19545](https://github.com/netdata/netdata/pull/19545) ([ilyam8](https://github.com/ilyam8))
- chore\(windows.plugin\): format perflib ad and netframework [\#19544](https://github.com/netdata/netdata/pull/19544) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19541](https://github.com/netdata/netdata/pull/19541) ([netdatabot](https://github.com/netdatabot))
- Use database/rrd.h instead of daemon/common.h [\#19540](https://github.com/netdata/netdata/pull/19540) ([vkalintiris](https://github.com/vkalintiris))
- allow dbengine to read at offsets above 4GiB - again [\#19539](https://github.com/netdata/netdata/pull/19539) ([ktsaou](https://github.com/ktsaou))
- allow dbengine to read at offsets above 4GiB [\#19538](https://github.com/netdata/netdata/pull/19538) ([ktsaou](https://github.com/ktsaou))
- inline dbengine query critical path [\#19537](https://github.com/netdata/netdata/pull/19537) ([ktsaou](https://github.com/ktsaou))
- Fix contexts stay not-live when children reconnect [\#19536](https://github.com/netdata/netdata/pull/19536) ([ktsaou](https://github.com/ktsaou))
- Fix coverity issue [\#19535](https://github.com/netdata/netdata/pull/19535) ([stelfrag](https://github.com/stelfrag))
- Actually handle the `-fexceptions` requirement correctly in our build system. [\#19534](https://github.com/netdata/netdata/pull/19534) ([Ferroin](https://github.com/Ferroin))
- fix heap use after free [\#19532](https://github.com/netdata/netdata/pull/19532) ([ktsaou](https://github.com/ktsaou))
- docs\(web/gui\): remove info about old dashboard from readme [\#19531](https://github.com/netdata/netdata/pull/19531) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19530](https://github.com/netdata/netdata/pull/19530) ([netdatabot](https://github.com/netdatabot))
- chore\(go.d/snmp\): enable create\_vnode by default [\#19529](https://github.com/netdata/netdata/pull/19529) ([ilyam8](https://github.com/ilyam8))
- ci: bump static build timeout to 6hr [\#19528](https://github.com/netdata/netdata/pull/19528) ([ilyam8](https://github.com/ilyam8))
- Fix MSSQL Instance [\#19527](https://github.com/netdata/netdata/pull/19527) ([thiagoftsm](https://github.com/thiagoftsm))
- Improve data write [\#19525](https://github.com/netdata/netdata/pull/19525) ([stelfrag](https://github.com/stelfrag))
- inline functions related to metrics ingestion [\#19524](https://github.com/netdata/netdata/pull/19524) ([ktsaou](https://github.com/ktsaou))
- chore\(packaging\): remove old dashboard [\#19523](https://github.com/netdata/netdata/pull/19523) ([ilyam8](https://github.com/ilyam8))
- Format PGDs on fatal\(\) [\#19521](https://github.com/netdata/netdata/pull/19521) ([vkalintiris](https://github.com/vkalintiris))
- SMSEagle integration [\#19520](https://github.com/netdata/netdata/pull/19520) ([marcin-smseagle](https://github.com/marcin-smseagle))
- ci: increase static build timeout 180-\>300m [\#19519](https://github.com/netdata/netdata/pull/19519) ([ilyam8](https://github.com/ilyam8))
- Improve ACLK query processing [\#19518](https://github.com/netdata/netdata/pull/19518) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#19517](https://github.com/netdata/netdata/pull/19517) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/httpcheck\): add alerts to metadata [\#19516](https://github.com/netdata/netdata/pull/19516) ([ilyam8](https://github.com/ilyam8))
- Invert order of checks in pgd\_append\_point\(\). [\#19515](https://github.com/netdata/netdata/pull/19515) ([vkalintiris](https://github.com/vkalintiris))
- Link the ebpf plugin against libbpf directly instead of through libnetdata. [\#19514](https://github.com/netdata/netdata/pull/19514) ([Ferroin](https://github.com/Ferroin))
- compile time and runtime check of required compiler flags [\#19513](https://github.com/netdata/netdata/pull/19513) ([ktsaou](https://github.com/ktsaou))
- netdata.spec/plugin-go: remove dependency for lm\_sensors [\#19511](https://github.com/netdata/netdata/pull/19511) ([k0ste](https://github.com/k0ste))
- chore\(go.d/nvme\): fix :dog: warning [\#19510](https://github.com/netdata/netdata/pull/19510) ([ilyam8](https://github.com/ilyam8))
- Bundle cmake cache. [\#19509](https://github.com/netdata/netdata/pull/19509) ([vkalintiris](https://github.com/vkalintiris))
- ACLK: allow encoded proxy username and password to work [\#19508](https://github.com/netdata/netdata/pull/19508) ([ktsaou](https://github.com/ktsaou))
- Fix alert transition [\#19507](https://github.com/netdata/netdata/pull/19507) ([stelfrag](https://github.com/stelfrag))
- update buildinfo  [\#19506](https://github.com/netdata/netdata/pull/19506) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/nvme\): support v2.11 output format [\#19505](https://github.com/netdata/netdata/pull/19505) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/vmware/govmomi from 0.47.0 to 0.48.0 in /src/go [\#19504](https://github.com/netdata/netdata/pull/19504) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations docs [\#19502](https://github.com/netdata/netdata/pull/19502) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/postgres\): add config example with unix socket + custom port [\#19501](https://github.com/netdata/netdata/pull/19501) ([ilyam8](https://github.com/ilyam8))
- Create impact-on-resources.md [\#19499](https://github.com/netdata/netdata/pull/19499) ([ktsaou](https://github.com/ktsaou))
- Add worker for alert queue processing [\#19498](https://github.com/netdata/netdata/pull/19498) ([stelfrag](https://github.com/stelfrag))
- fix absolute injection again [\#19497](https://github.com/netdata/netdata/pull/19497) ([ktsaou](https://github.com/ktsaou))
- fix absolute injection [\#19496](https://github.com/netdata/netdata/pull/19496) ([ktsaou](https://github.com/ktsaou))
- max data file size [\#19495](https://github.com/netdata/netdata/pull/19495) ([ktsaou](https://github.com/ktsaou))
- proc.plugin: add `ifb4*` to excluded interface name patterns [\#19494](https://github.com/netdata/netdata/pull/19494) ([intelfx](https://github.com/intelfx))
- build\(deps\): bump github.com/bmatcuk/doublestar/v4 from 4.8.0 to 4.8.1 in /src/go [\#19493](https://github.com/netdata/netdata/pull/19493) ([dependabot[bot]](https://github.com/apps/dependabot))
- Active Directory Certification Service \(windows.plugin\) [\#19492](https://github.com/netdata/netdata/pull/19492) ([thiagoftsm](https://github.com/thiagoftsm))
- proc.plugin: remove traces of /proc/spl/kstat/zfs/pool/state [\#19491](https://github.com/netdata/netdata/pull/19491) ([intelfx](https://github.com/intelfx))
- cgroups.plugin: fixes to cgroup path validation [\#19490](https://github.com/netdata/netdata/pull/19490) ([intelfx](https://github.com/intelfx))
- Further improve alert processing [\#19489](https://github.com/netdata/netdata/pull/19489) ([stelfrag](https://github.com/stelfrag))
- LTO Benchmark [\#19488](https://github.com/netdata/netdata/pull/19488) ([ktsaou](https://github.com/ktsaou))
- Improve alert transition processing [\#19487](https://github.com/netdata/netdata/pull/19487) ([stelfrag](https://github.com/stelfrag))
- protection against extreme cardinality [\#19486](https://github.com/netdata/netdata/pull/19486) ([ktsaou](https://github.com/ktsaou))
- add agent name and version in streaming function [\#19485](https://github.com/netdata/netdata/pull/19485) ([ktsaou](https://github.com/ktsaou))
- Coverity fixes [\#19484](https://github.com/netdata/netdata/pull/19484) ([ktsaou](https://github.com/ktsaou))
- add system-info columns to streaming function [\#19482](https://github.com/netdata/netdata/pull/19482) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19481](https://github.com/netdata/netdata/pull/19481) ([netdatabot](https://github.com/netdatabot))
- chore\(go.d/ping\): set privileged by default for dyncfg jobs [\#19480](https://github.com/netdata/netdata/pull/19480) ([ilyam8](https://github.com/ilyam8))
- Improve metadata cleanup [\#19479](https://github.com/netdata/netdata/pull/19479) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/prometheus-community/pro-bing from 0.5.0 to 0.6.0 in /src/go [\#19477](https://github.com/netdata/netdata/pull/19477) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 27.5.0+incompatible to 27.5.1+incompatible in /src/go [\#19476](https://github.com/netdata/netdata/pull/19476) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/miekg/dns from 1.1.62 to 1.1.63 in /src/go [\#19475](https://github.com/netdata/netdata/pull/19475) ([dependabot[bot]](https://github.com/apps/dependabot))
- optimized rrdhost\_status [\#19472](https://github.com/netdata/netdata/pull/19472) ([ktsaou](https://github.com/ktsaou))
- Unregister node from the agent to run in a worker thread [\#19471](https://github.com/netdata/netdata/pull/19471) ([stelfrag](https://github.com/stelfrag))
- Make handling of cross-platform emulation for static builds smarter. [\#19470](https://github.com/netdata/netdata/pull/19470) ([Ferroin](https://github.com/Ferroin))
- Use QEMU from the runner environment instead of an external copy. [\#19468](https://github.com/netdata/netdata/pull/19468) ([Ferroin](https://github.com/Ferroin))
- fix crash when cleaning up virtual nodes [\#19467](https://github.com/netdata/netdata/pull/19467) ([ktsaou](https://github.com/ktsaou))
- streaming nodes accounting [\#19466](https://github.com/netdata/netdata/pull/19466) ([ktsaou](https://github.com/ktsaou))
- Don’t fail fast on static builds and Docker builds. [\#19465](https://github.com/netdata/netdata/pull/19465) ([Ferroin](https://github.com/Ferroin))
- Child should be online before initializing health [\#19463](https://github.com/netdata/netdata/pull/19463) ([stelfrag](https://github.com/stelfrag))
- Active Directory Metrics \(Windows.plugin\) [\#19461](https://github.com/netdata/netdata/pull/19461) ([thiagoftsm](https://github.com/thiagoftsm))
- Use aral in ACLK [\#19459](https://github.com/netdata/netdata/pull/19459) ([stelfrag](https://github.com/stelfrag))
- Enable libunwind in native packages and Docker images. [\#19452](https://github.com/netdata/netdata/pull/19452) ([Ferroin](https://github.com/Ferroin))
- fix rrdset name crash on rrdset obsoletion [\#19449](https://github.com/netdata/netdata/pull/19449) ([ktsaou](https://github.com/ktsaou))
- Pulse stream-parents [\#19445](https://github.com/netdata/netdata/pull/19445) ([ktsaou](https://github.com/ktsaou))
- Start using new GitHub hosted ARM runners for CI when appropriate. [\#19427](https://github.com/netdata/netdata/pull/19427) ([Ferroin](https://github.com/Ferroin))

## [v2.2.6](https://github.com/netdata/netdata/tree/v2.2.6) (2025-02-20)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.5...v2.2.6)

## [v2.2.5](https://github.com/netdata/netdata/tree/v2.2.5) (2025-02-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.4...v2.2.5)

## [v2.2.4](https://github.com/netdata/netdata/tree/v2.2.4) (2025-02-04)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.3...v2.2.4)

## [v2.2.3](https://github.com/netdata/netdata/tree/v2.2.3) (2025-01-31)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.2...v2.2.3)

## [v2.2.2](https://github.com/netdata/netdata/tree/v2.2.2) (2025-01-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.1...v2.2.2)

## [v2.2.1](https://github.com/netdata/netdata/tree/v2.2.1) (2025-01-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.0...v2.2.1)

## [v2.2.0](https://github.com/netdata/netdata/tree/v2.2.0) (2025-01-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.1.1...v2.2.0)

**Merged pull requests:**

- control stream-info requests rate [\#19458](https://github.com/netdata/netdata/pull/19458) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/upsd\): remove UPS load charts if UPS load not found [\#19457](https://github.com/netdata/netdata/pull/19457) ([ilyam8](https://github.com/ilyam8))
- Simplify the rrdhost\_ingestion\_status call [\#19456](https://github.com/netdata/netdata/pull/19456) ([stelfrag](https://github.com/stelfrag))
- Fix up handling of libunwind in CMake. [\#19451](https://github.com/netdata/netdata/pull/19451) ([Ferroin](https://github.com/Ferroin))
- Revert libunwind being enabled in Docker and DEB builds. [\#19450](https://github.com/netdata/netdata/pull/19450) ([Ferroin](https://github.com/Ferroin))
- Do not run queries synchronously in the event loop [\#19448](https://github.com/netdata/netdata/pull/19448) ([stelfrag](https://github.com/stelfrag))
- Cleanup metadata event loop [\#19447](https://github.com/netdata/netdata/pull/19447) ([stelfrag](https://github.com/stelfrag))
- Make sure ACLK synchronization event loop runs frequently [\#19446](https://github.com/netdata/netdata/pull/19446) ([stelfrag](https://github.com/stelfrag))
- move dbengine-retention chart to pulse [\#19444](https://github.com/netdata/netdata/pull/19444) ([ktsaou](https://github.com/ktsaou))
- Fix Child web remote access Config in Parent-Child Deployment Examples [\#19443](https://github.com/netdata/netdata/pull/19443) ([Destructio](https://github.com/Destructio))
- build\(deps\): update go toolchain to v1.23.5 [\#19442](https://github.com/netdata/netdata/pull/19442) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/prometheus/common from 0.61.0 to 0.62.0 in /src/go [\#19439](https://github.com/netdata/netdata/pull/19439) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/sijms/go-ora/v2 from 2.8.22 to 2.8.23 in /src/go [\#19438](https://github.com/netdata/netdata/pull/19438) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump k8s.io/client-go from 0.32.0 to 0.32.1 in /src/go [\#19437](https://github.com/netdata/netdata/pull/19437) ([dependabot[bot]](https://github.com/apps/dependabot))
- Handle incoming ACLK traffic asynchronously [\#19436](https://github.com/netdata/netdata/pull/19436) ([stelfrag](https://github.com/stelfrag))
- add more aclk worker jobs [\#19435](https://github.com/netdata/netdata/pull/19435) ([ktsaou](https://github.com/ktsaou))
- fix go.d/ethtool config schema [\#19434](https://github.com/netdata/netdata/pull/19434) ([ilyam8](https://github.com/ilyam8))
- Cleanup context check list on startup [\#19433](https://github.com/netdata/netdata/pull/19433) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#19432](https://github.com/netdata/netdata/pull/19432) ([netdatabot](https://github.com/netdatabot))
- Drop Fedora 39 from CI and package builds. [\#19431](https://github.com/netdata/netdata/pull/19431) ([Ferroin](https://github.com/Ferroin))
- docs: fix go.d/ethtool meta [\#19430](https://github.com/netdata/netdata/pull/19430) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ethtool\): use ndsudo for module info [\#19429](https://github.com/netdata/netdata/pull/19429) ([ilyam8](https://github.com/ilyam8))
- add 'ethtool -m' to ndsudo [\#19428](https://github.com/netdata/netdata/pull/19428) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/ethtool\): collect module ddm info using ethtool [\#19426](https://github.com/netdata/netdata/pull/19426) ([ilyam8](https://github.com/ilyam8))
- ACLK timeout [\#19425](https://github.com/netdata/netdata/pull/19425) ([ktsaou](https://github.com/ktsaou))
- log stream\_info payload when it cannot be parsed [\#19424](https://github.com/netdata/netdata/pull/19424) ([ktsaou](https://github.com/ktsaou))
- Add missing information in rule based membership document [\#19423](https://github.com/netdata/netdata/pull/19423) ([juacker](https://github.com/juacker))
- Fix coverity issues [\#19422](https://github.com/netdata/netdata/pull/19422) ([stelfrag](https://github.com/stelfrag))
- add 'type' to GH report forms [\#19421](https://github.com/netdata/netdata/pull/19421) ([ilyam8](https://github.com/ilyam8))
- fix mmaps accounting [\#19420](https://github.com/netdata/netdata/pull/19420) ([ktsaou](https://github.com/ktsaou))
- PULSE: network traffic [\#19419](https://github.com/netdata/netdata/pull/19419) ([ktsaou](https://github.com/ktsaou))
- hostnames: convert to utf8 and santitize [\#19418](https://github.com/netdata/netdata/pull/19418) ([ktsaou](https://github.com/ktsaou))
- Enable libunwind in DEB native packages. [\#19417](https://github.com/netdata/netdata/pull/19417) ([Ferroin](https://github.com/Ferroin))
- cleanup contexts during loading [\#19416](https://github.com/netdata/netdata/pull/19416) ([ktsaou](https://github.com/ktsaou))
- packaging\(windows\): use local copy of GPL-3 [\#19414](https://github.com/netdata/netdata/pull/19414) ([ilyam8](https://github.com/ilyam8))
- add "netdata-" prefix to streaming and metrics-cardinality functions [\#19413](https://github.com/netdata/netdata/pull/19413) ([ilyam8](https://github.com/ilyam8))
- REFCOUNT: use only compare-and-exchange [\#19411](https://github.com/netdata/netdata/pull/19411) ([ktsaou](https://github.com/ktsaou))
- Alert prototypes: use r/w spinlock instead of spinlock [\#19410](https://github.com/netdata/netdata/pull/19410) ([ktsaou](https://github.com/ktsaou))
- Enable libunwind in Docker images. [\#19409](https://github.com/netdata/netdata/pull/19409) ([Ferroin](https://github.com/Ferroin))
- build\(deps\): bump github.com/docker/docker from 27.4.1+incompatible to 27.5.0+incompatible in /src/go [\#19408](https://github.com/netdata/netdata/pull/19408) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/bmatcuk/doublestar/v4 from 4.7.1 to 4.8.0 in /src/go [\#19407](https://github.com/netdata/netdata/pull/19407) ([dependabot[bot]](https://github.com/apps/dependabot))
- fixed http clients accounting [\#19406](https://github.com/netdata/netdata/pull/19406) ([ktsaou](https://github.com/ktsaou))
- RRD files split, renames, cleanup Part 2 [\#19405](https://github.com/netdata/netdata/pull/19405) ([ktsaou](https://github.com/ktsaou))
- fix loading contexts [\#19404](https://github.com/netdata/netdata/pull/19404) ([ktsaou](https://github.com/ktsaou))
- Delay context cleanup checks after startup [\#19403](https://github.com/netdata/netdata/pull/19403) ([stelfrag](https://github.com/stelfrag))
- system memory calculation for cgroups v1 fix [\#19402](https://github.com/netdata/netdata/pull/19402) ([ktsaou](https://github.com/ktsaou))
- do not process contexts before they are loaded [\#19401](https://github.com/netdata/netdata/pull/19401) ([ktsaou](https://github.com/ktsaou))
- Revert "Update kickstart script to use new repository host." [\#19400](https://github.com/netdata/netdata/pull/19400) ([Ferroin](https://github.com/Ferroin))
- split rrdhost/rrdset/rrddim and rrd.h [\#19399](https://github.com/netdata/netdata/pull/19399) ([ktsaou](https://github.com/ktsaou))
- fix nodes staying in initializing status [\#19398](https://github.com/netdata/netdata/pull/19398) ([ktsaou](https://github.com/ktsaou))
- Use worker when dispatching alert transitions to the cloud [\#19397](https://github.com/netdata/netdata/pull/19397) ([stelfrag](https://github.com/stelfrag))
- Unified memory API [\#19396](https://github.com/netdata/netdata/pull/19396) ([ktsaou](https://github.com/ktsaou))
- build\(go.d\): switch to gohugoio/hashstructure [\#19395](https://github.com/netdata/netdata/pull/19395) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19394](https://github.com/netdata/netdata/pull/19394) ([netdatabot](https://github.com/netdatabot))
- Make libunwind opt-in at build time instead of auto-enabled. [\#19393](https://github.com/netdata/netdata/pull/19393) ([Ferroin](https://github.com/Ferroin))
- Remove openSUSE 15.5 from CI and package builds. [\#19392](https://github.com/netdata/netdata/pull/19392) ([Ferroin](https://github.com/Ferroin))
- Reduce glibc fragmentation Part 2 [\#19390](https://github.com/netdata/netdata/pull/19390) ([ktsaou](https://github.com/ktsaou))
- Verify and cleanup deleted contexts [\#19389](https://github.com/netdata/netdata/pull/19389) ([stelfrag](https://github.com/stelfrag))
- Reduce glibc memory fragmentation [\#19385](https://github.com/netdata/netdata/pull/19385) ([ktsaou](https://github.com/ktsaou))
- added mmap count charts [\#19384](https://github.com/netdata/netdata/pull/19384) ([ktsaou](https://github.com/ktsaou))
- used\_arena should exclude unused memory [\#19382](https://github.com/netdata/netdata/pull/19382) ([ktsaou](https://github.com/ktsaou))
- fix mallinfo2 [\#19381](https://github.com/netdata/netdata/pull/19381) ([ktsaou](https://github.com/ktsaou))
- limit the glibc unused memory [\#19380](https://github.com/netdata/netdata/pull/19380) ([ktsaou](https://github.com/ktsaou))
- Pulse extended memory statistics, now report glibc allocations [\#19379](https://github.com/netdata/netdata/pull/19379) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/axiomhq/hyperloglog from 0.2.2 to 0.2.3 in /src/go [\#19378](https://github.com/netdata/netdata/pull/19378) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump go.mongodb.org/mongo-driver from 1.17.1 to 1.17.2 in /src/go [\#19377](https://github.com/netdata/netdata/pull/19377) ([dependabot[bot]](https://github.com/apps/dependabot))
- ARAL: fast path to quickly allocate elements on a new page [\#19376](https://github.com/netdata/netdata/pull/19376) ([ktsaou](https://github.com/ktsaou))

## [v2.1.1](https://github.com/netdata/netdata/tree/v2.1.1) (2025-01-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.1.0...v2.1.1)

## [v2.1.0](https://github.com/netdata/netdata/tree/v2.1.0) (2024-12-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.3...v2.1.0)

## [v2.0.3](https://github.com/netdata/netdata/tree/v2.0.3) (2024-11-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.2...v2.0.3)

## [v2.0.2](https://github.com/netdata/netdata/tree/v2.0.2) (2024-11-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.1...v2.0.2)

## [v2.0.1](https://github.com/netdata/netdata/tree/v2.0.1) (2024-11-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.0...v2.0.1)

## [v2.0.0](https://github.com/netdata/netdata/tree/v2.0.0) (2024-11-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.5...v2.0.0)

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
