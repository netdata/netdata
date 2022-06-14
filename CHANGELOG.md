# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.1...HEAD)

**Merged pull requests:**

- Re-enable updates for systems using static builds. [\#13110](https://github.com/netdata/netdata/pull/13110) ([Ferroin](https://github.com/Ferroin))
- Add user netdata to secondary group in DEB package [\#13109](https://github.com/netdata/netdata/pull/13109) ([iigorkarpov](https://github.com/iigorkarpov))
- 73x times faster metrics correlations at the agent [\#13107](https://github.com/netdata/netdata/pull/13107) ([ktsaou](https://github.com/ktsaou))
- Remove unnescesary ‘cleanup’ code. [\#13103](https://github.com/netdata/netdata/pull/13103) ([Ferroin](https://github.com/Ferroin))
- Temporarily disable updates for static builds. [\#13100](https://github.com/netdata/netdata/pull/13100) ([Ferroin](https://github.com/Ferroin))
- docs\(statsd.plugin\): fix indentation [\#13096](https://github.com/netdata/netdata/pull/13096) ([ilyam8](https://github.com/ilyam8))
- fix virtualization detection on FreeBSD [\#13087](https://github.com/netdata/netdata/pull/13087) ([ilyam8](https://github.com/ilyam8))
- Labels with dictionary [\#13070](https://github.com/netdata/netdata/pull/13070) ([ktsaou](https://github.com/ktsaou))

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
- Allocate buffer memory for uv\_write and release in the callback function [\#12688](https://github.com/netdata/netdata/pull/12688) ([stelfrag](https://github.com/stelfrag))
- \[Uninstall Netdata\] - Add description in the docs to use uninstaller script with force arg [\#12687](https://github.com/netdata/netdata/pull/12687) ([odynik](https://github.com/odynik))
- Correctly propagate errors and warnings up to the kickstart script from scripts it calls. [\#12686](https://github.com/netdata/netdata/pull/12686) ([Ferroin](https://github.com/Ferroin))
- Memory CO-RE [\#12684](https://github.com/netdata/netdata/pull/12684) ([thiagoftsm](https://github.com/thiagoftsm))
- Docs: fix GitHub format [\#12682](https://github.com/netdata/netdata/pull/12682) ([eltociear](https://github.com/eltociear))
- feat\(apps.plugin\): add caddy to apps\_groups.conf [\#12678](https://github.com/netdata/netdata/pull/12678) ([simon300000](https://github.com/simon300000))
- fix: use NETDATA\_LISTENER\_PORT in docker healtcheck [\#12676](https://github.com/netdata/netdata/pull/12676) ([ilyam8](https://github.com/ilyam8))
- Add a 2 minute timeout to stream receiver socket [\#12673](https://github.com/netdata/netdata/pull/12673) ([MrZammler](https://github.com/MrZammler))
- Add options to kickstart.sh for explicitly passing options to installer code. [\#12658](https://github.com/netdata/netdata/pull/12658) ([Ferroin](https://github.com/Ferroin))
- Improve agent cloud chart synchronization [\#12655](https://github.com/netdata/netdata/pull/12655) ([stelfrag](https://github.com/stelfrag))
- Add the ability to perform a data query using an offline node id [\#12650](https://github.com/netdata/netdata/pull/12650) ([stelfrag](https://github.com/stelfrag))
- Gotify notifications [\#12639](https://github.com/netdata/netdata/pull/12639) ([coffeegrind123](https://github.com/coffeegrind123))
- Improve handling of release channel selection in kickstart.sh. [\#12635](https://github.com/netdata/netdata/pull/12635) ([Ferroin](https://github.com/Ferroin))
- Fix Valgrind errors [\#12619](https://github.com/netdata/netdata/pull/12619) ([vlvkobal](https://github.com/vlvkobal))
- Pass the child machine's guid to the goto\_url link [\#12609](https://github.com/netdata/netdata/pull/12609) ([MrZammler](https://github.com/MrZammler))
- Implements new capability fields in aclk\_schemas [\#12602](https://github.com/netdata/netdata/pull/12602) ([underhood](https://github.com/underhood))
- Metric correlations [\#12582](https://github.com/netdata/netdata/pull/12582) ([MrZammler](https://github.com/MrZammler))
- Reduce alert events sent to the cloud. [\#12544](https://github.com/netdata/netdata/pull/12544) ([MrZammler](https://github.com/MrZammler))
- include proper package dependency [\#12518](https://github.com/netdata/netdata/pull/12518) ([atriwidada](https://github.com/atriwidada))
- Docs templates [\#12466](https://github.com/netdata/netdata/pull/12466) ([kickoke](https://github.com/kickoke))

## [v1.34.1](https://github.com/netdata/netdata/tree/v1.34.1) (2022-04-15)

[Full Changelog](https://github.com/netdata/netdata/compare/1.34.0...v1.34.1)

## [1.34.0](https://github.com/netdata/netdata/tree/1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.0...1.34.0)

## [v1.34.0](https://github.com/netdata/netdata/tree/v1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.1...v1.34.0)

**Merged pull requests:**

- Cancel anomaly detection threads before joining. [\#12681](https://github.com/netdata/netdata/pull/12681) ([vkalintiris](https://github.com/vkalintiris))
- Update dashboard to version v2.25.0. [\#12680](https://github.com/netdata/netdata/pull/12680) ([netdatabot](https://github.com/netdatabot))
- Delete ML-related data of a host in the proper order. [\#12672](https://github.com/netdata/netdata/pull/12672) ([vkalintiris](https://github.com/vkalintiris))
- fix\(ebpf.plugin\): add missing chart context for cgroups charts [\#12671](https://github.com/netdata/netdata/pull/12671) ([ilyam8](https://github.com/ilyam8))
- Remove on pull\_request trigger [\#12670](https://github.com/netdata/netdata/pull/12670) ([dimko](https://github.com/dimko))
- \[Stream compression Docs\] - Enabled by default [\#12669](https://github.com/netdata/netdata/pull/12669) ([odynik](https://github.com/odynik))
- Update dashboard to version v2.24.0. [\#12668](https://github.com/netdata/netdata/pull/12668) ([netdatabot](https://github.com/netdatabot))
- Show error when clock synchronization state is unavailable [\#12667](https://github.com/netdata/netdata/pull/12667) ([vlvkobal](https://github.com/vlvkobal))
- Dashboard network title [\#12665](https://github.com/netdata/netdata/pull/12665) ([thiagoftsm](https://github.com/thiagoftsm))
- bump go.d.plugin version to v0.32.2 [\#12663](https://github.com/netdata/netdata/pull/12663) ([ilyam8](https://github.com/ilyam8))
- Properly limit repository configuration dependencies. [\#12661](https://github.com/netdata/netdata/pull/12661) ([Ferroin](https://github.com/Ferroin))
- Show --build-only instead of --only-build [\#12657](https://github.com/netdata/netdata/pull/12657) ([MrZammler](https://github.com/MrZammler))
- Update dashboard to version v2.22.6. [\#12653](https://github.com/netdata/netdata/pull/12653) ([netdatabot](https://github.com/netdatabot))
- Add a chart label filter parameter in context data queries [\#12652](https://github.com/netdata/netdata/pull/12652) ([stelfrag](https://github.com/stelfrag))
- Add a timeout parameter to data queries [\#12649](https://github.com/netdata/netdata/pull/12649) ([stelfrag](https://github.com/stelfrag))
- fix: remove instance-specific information from chart titles [\#12644](https://github.com/netdata/netdata/pull/12644) ([ilyam8](https://github.com/ilyam8))
- feat: add k8s\_cluster\_name host tag \(GKE only\) [\#12638](https://github.com/netdata/netdata/pull/12638) ([ilyam8](https://github.com/ilyam8))
- Summarize encountered errors and warnings at end of kickstart script run. [\#12636](https://github.com/netdata/netdata/pull/12636) ([Ferroin](https://github.com/Ferroin))
- Add eBPF CO-RE version and checksum files to distfile list. [\#12627](https://github.com/netdata/netdata/pull/12627) ([Ferroin](https://github.com/Ferroin))
- Fix ACLK shutdown [\#12625](https://github.com/netdata/netdata/pull/12625) ([underhood](https://github.com/underhood))
- Don't do fatal on error writing the health api management key. [\#12623](https://github.com/netdata/netdata/pull/12623) ([MrZammler](https://github.com/MrZammler))
- fix\(cgroups.plugin\): set CPU prev usage before first usage. [\#12622](https://github.com/netdata/netdata/pull/12622) ([ilyam8](https://github.com/ilyam8))
- eBPF update dashboard [\#12617](https://github.com/netdata/netdata/pull/12617) ([thiagoftsm](https://github.com/thiagoftsm))
- fix print: command not found issue [\#12615](https://github.com/netdata/netdata/pull/12615) ([maneamarius](https://github.com/maneamarius))
- feat: add support for cloud providers info to /api/v1/info [\#12613](https://github.com/netdata/netdata/pull/12613) ([ilyam8](https://github.com/ilyam8))
- Fix training/prediction stats charts context. [\#12610](https://github.com/netdata/netdata/pull/12610) ([vkalintiris](https://github.com/vkalintiris))
- Fix a compilation warning [\#12608](https://github.com/netdata/netdata/pull/12608) ([vlvkobal](https://github.com/vlvkobal))
- Update dashboard to version v2.22.3. [\#12607](https://github.com/netdata/netdata/pull/12607) ([netdatabot](https://github.com/netdatabot))
- Enable streaming of anomaly\_detection.\* charts [\#12606](https://github.com/netdata/netdata/pull/12606) ([vkalintiris](https://github.com/vkalintiris))
- Better check for IOMainPort on MacOS [\#12600](https://github.com/netdata/netdata/pull/12600) ([vlvkobal](https://github.com/vlvkobal))
- Fix coverity issues [\#12598](https://github.com/netdata/netdata/pull/12598) ([vkalintiris](https://github.com/vkalintiris))
- chore: make logs less noisy on child reconnect [\#12594](https://github.com/netdata/netdata/pull/12594) ([ilyam8](https://github.com/ilyam8))
- feat\(cgroups.plugin\): add CPU throttling charts [\#12591](https://github.com/netdata/netdata/pull/12591) ([ilyam8](https://github.com/ilyam8))
- Fix ebpf exit [\#12590](https://github.com/netdata/netdata/pull/12590) ([thiagoftsm](https://github.com/thiagoftsm))
- feat\(collectors\): update go.d.plugin version to v0.32.1 [\#12586](https://github.com/netdata/netdata/pull/12586) ([ilyam8](https://github.com/ilyam8))
- Check if libatomic can be linked [\#12583](https://github.com/netdata/netdata/pull/12583) ([MrZammler](https://github.com/MrZammler))
- Update links to documentation \(eBPF\) [\#12581](https://github.com/netdata/netdata/pull/12581) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: re-add setuid bit to ioping after installing Debian package [\#12580](https://github.com/netdata/netdata/pull/12580) ([ilyam8](https://github.com/ilyam8))
- Kickstart improved messaging [\#12577](https://github.com/netdata/netdata/pull/12577) ([Ferroin](https://github.com/Ferroin))
- add some new ml params to README [\#12575](https://github.com/netdata/netdata/pull/12575) ([andrewm4894](https://github.com/andrewm4894))
- Update ML-related charts [\#12574](https://github.com/netdata/netdata/pull/12574) ([vkalintiris](https://github.com/vkalintiris))
- update anonymous-statistics readme for PH Cloud [\#12571](https://github.com/netdata/netdata/pull/12571) ([andrewm4894](https://github.com/andrewm4894))
- Respect dimension hidden option when executing a query  [\#12570](https://github.com/netdata/netdata/pull/12570) ([stelfrag](https://github.com/stelfrag))
- \[Agent crash on api/v1/info call\] - fixes \#12559 [\#12565](https://github.com/netdata/netdata/pull/12565) ([erdem2000](https://github.com/erdem2000))
- Fix temporary directory handling for dependency handling script in updater. [\#12562](https://github.com/netdata/netdata/pull/12562) ([Ferroin](https://github.com/Ferroin))
- feat\(netdata-updater\): add the script name when logging [\#12557](https://github.com/netdata/netdata/pull/12557) ([ilyam8](https://github.com/ilyam8))
- Fix Build on MacOS [\#12554](https://github.com/netdata/netdata/pull/12554) ([underhood](https://github.com/underhood))
- Unblock cgroup version detection with systemd [\#12553](https://github.com/netdata/netdata/pull/12553) ([vlvkobal](https://github.com/vlvkobal))
- fix FreeBSD bundled protobuf build if system one is present [\#12552](https://github.com/netdata/netdata/pull/12552) ([underhood](https://github.com/underhood))
- fix: use `/proc/cpuinfo` for CPU freq detection as a last resort [\#12550](https://github.com/netdata/netdata/pull/12550) ([ilyam8](https://github.com/ilyam8))
- add --reinstall-clean flag for kickstart.sh and update documentation [\#12548](https://github.com/netdata/netdata/pull/12548) ([maneamarius](https://github.com/maneamarius))
- Don't send alert events without wc-\>host [\#12547](https://github.com/netdata/netdata/pull/12547) ([MrZammler](https://github.com/MrZammler))
- reduce min `dbengine anomaly rate every` 60s-\>30s [\#12543](https://github.com/netdata/netdata/pull/12543) ([andrewm4894](https://github.com/andrewm4894))
- Explicitly use debhelper to enable systemd service [\#12542](https://github.com/netdata/netdata/pull/12542) ([ralphm](https://github.com/ralphm))
- Allocate buffer and release on callback when executing agent CLI commands [\#12540](https://github.com/netdata/netdata/pull/12540) ([stelfrag](https://github.com/stelfrag))
- Make sure registered static threads are unique. [\#12538](https://github.com/netdata/netdata/pull/12538) ([vkalintiris](https://github.com/vkalintiris))
- packaging: upgrage protocol buffer version to 3.19.4 [\#12537](https://github.com/netdata/netdata/pull/12537) ([surajnpn](https://github.com/surajnpn))
- feat\(collectors\): update go.d.plugin version to v0.32.0 [\#12536](https://github.com/netdata/netdata/pull/12536) ([ilyam8](https://github.com/ilyam8))
- Improve ACLK sync logging  [\#12534](https://github.com/netdata/netdata/pull/12534) ([stelfrag](https://github.com/stelfrag))
- Socket connections \(eBPF\) and bug fix [\#12532](https://github.com/netdata/netdata/pull/12532) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: use internal defaults for sched policy/oom score in native packages [\#12529](https://github.com/netdata/netdata/pull/12529) ([ilyam8](https://github.com/ilyam8))
- docs: fix unresolved file references [\#12528](https://github.com/netdata/netdata/pull/12528) ([ilyam8](https://github.com/ilyam8))
- fix\(kickstart.sh\): use `$ROOTCMD` when setting auto updates [\#12526](https://github.com/netdata/netdata/pull/12526) ([ilyam8](https://github.com/ilyam8))
- fix\(netdata-updater\): properly handle update for debian packages [\#12524](https://github.com/netdata/netdata/pull/12524) ([ilyam8](https://github.com/ilyam8))
- fix centos gpg key issue [\#12519](https://github.com/netdata/netdata/pull/12519) ([maneamarius](https://github.com/maneamarius))
- fix: Netdata segfault because of 2 timex.plugin threads [\#12512](https://github.com/netdata/netdata/pull/12512) ([ilyam8](https://github.com/ilyam8))
- Fix memory leaks on Netdata exit [\#12511](https://github.com/netdata/netdata/pull/12511) ([vlvkobal](https://github.com/vlvkobal))
- fix centos7 gpg key issue [\#12506](https://github.com/netdata/netdata/pull/12506) ([maneamarius](https://github.com/maneamarius))
- Use live charts to count the total number of dimensions. [\#12504](https://github.com/netdata/netdata/pull/12504) ([vkalintiris](https://github.com/vkalintiris))
- Update ebpf doc [\#12503](https://github.com/netdata/netdata/pull/12503) ([thiagoftsm](https://github.com/thiagoftsm))
- feat\(collectors/timex.plugin\): add clock status chart [\#12501](https://github.com/netdata/netdata/pull/12501) ([ilyam8](https://github.com/ilyam8))
- PR template: Include user information section [\#12499](https://github.com/netdata/netdata/pull/12499) ([kickoke](https://github.com/kickoke))
- add `locust` to `apps_groups.conf` [\#12498](https://github.com/netdata/netdata/pull/12498) ([andrewm4894](https://github.com/andrewm4894))
- Properly skip running the updater in kickstart dry-run mode. [\#12497](https://github.com/netdata/netdata/pull/12497) ([Ferroin](https://github.com/Ferroin))
- Adjust timex.plugin information to be less cryptic [\#12495](https://github.com/netdata/netdata/pull/12495) ([DanTheMediocre](https://github.com/DanTheMediocre))
- ML-related changes to address issue/discussion comments. [\#12494](https://github.com/netdata/netdata/pull/12494) ([vkalintiris](https://github.com/vkalintiris))
- fix: open fd 3 before first use in the netdata-updater.sh script [\#12491](https://github.com/netdata/netdata/pull/12491) ([ilyam8](https://github.com/ilyam8))
- timex: this plugin enables timex plugin for non-linux systems [\#12489](https://github.com/netdata/netdata/pull/12489) ([surajnpn](https://github.com/surajnpn))
- Bump the debhelper compat level to 10 in our DEB packaging code. [\#12488](https://github.com/netdata/netdata/pull/12488) ([Ferroin](https://github.com/Ferroin))
- Properly recognize Almalinux as an RHEL clone. [\#12487](https://github.com/netdata/netdata/pull/12487) ([Ferroin](https://github.com/Ferroin))
- minor - fix configure output of eBPF [\#12471](https://github.com/netdata/netdata/pull/12471) ([underhood](https://github.com/underhood))
- Don't send an alert snapshot with snapshot\_id 0 [\#12469](https://github.com/netdata/netdata/pull/12469) ([MrZammler](https://github.com/MrZammler))
- Update ebpf dashboard [\#12467](https://github.com/netdata/netdata/pull/12467) ([thiagoftsm](https://github.com/thiagoftsm))
- docs\(collectors/python.d\): remove mention of compatibility with py2/py3 [\#12465](https://github.com/netdata/netdata/pull/12465) ([ilyam8](https://github.com/ilyam8))
- feat\(collectors/cgroups\): prefer `blkio.*_recursive` when available [\#12462](https://github.com/netdata/netdata/pull/12462) ([ilyam8](https://github.com/ilyam8))
- Updated static build components to latest versions. [\#12461](https://github.com/netdata/netdata/pull/12461) ([ktsaou](https://github.com/ktsaou))
- Implement fine-grained errors to cloud queries [\#12460](https://github.com/netdata/netdata/pull/12460) ([underhood](https://github.com/underhood))

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
