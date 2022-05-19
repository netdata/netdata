# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.1...HEAD)

**Merged pull requests:**

- Optimize the dimensions option store to the metadata database [\#12952](https://github.com/netdata/netdata/pull/12952) ([stelfrag](https://github.com/stelfrag))
- Defer the dimension payload check to the ACLK sync thread [\#12951](https://github.com/netdata/netdata/pull/12951) ([stelfrag](https://github.com/stelfrag))
- detailed dbengine stats [\#12948](https://github.com/netdata/netdata/pull/12948) ([ktsaou](https://github.com/ktsaou))
- Prevent command\_to\_be\_logged from overflowing [\#12947](https://github.com/netdata/netdata/pull/12947) ([MrZammler](https://github.com/MrZammler))
- Update libbpf version [\#12945](https://github.com/netdata/netdata/pull/12945) ([thiagoftsm](https://github.com/thiagoftsm))
- Reduce timeout to 1 second for getting cloud instance info [\#12941](https://github.com/netdata/netdata/pull/12941) ([MrZammler](https://github.com/MrZammler))
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
- fix\(health\): change duplicate health template message logging level to 'info' [\#12873](https://github.com/netdata/netdata/pull/12873) ([ilyam8](https://github.com/ilyam8))
- docs: fix unresolved file references [\#12872](https://github.com/netdata/netdata/pull/12872) ([ilyam8](https://github.com/ilyam8))
- Set trust durations to have data from children properly aligned [\#12870](https://github.com/netdata/netdata/pull/12870) ([stelfrag](https://github.com/stelfrag))
- feat\(proc/cgroups.plugin\): add PSI stall time charts [\#12869](https://github.com/netdata/netdata/pull/12869) ([ilyam8](https://github.com/ilyam8))
- Update README.md [\#12868](https://github.com/netdata/netdata/pull/12868) ([tkatsoulas](https://github.com/tkatsoulas))
- fix for negative per job busy time [\#12867](https://github.com/netdata/netdata/pull/12867) ([ktsaou](https://github.com/ktsaou))
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
- Extend aclk-state [\#12458](https://github.com/netdata/netdata/pull/12458) ([underhood](https://github.com/underhood))
- Add support for passing extra claiming options when claiming with Docker. [\#12457](https://github.com/netdata/netdata/pull/12457) ([Ferroin](https://github.com/Ferroin))
- Update dashboard to version v2.21.8. [\#12455](https://github.com/netdata/netdata/pull/12455) ([netdatabot](https://github.com/netdatabot))
- fix\(collectors/cgroups\): use different context for cgroup network charts [\#12454](https://github.com/netdata/netdata/pull/12454) ([ilyam8](https://github.com/ilyam8))
- Initialize foreach alarms of dimensions in health thread. [\#12452](https://github.com/netdata/netdata/pull/12452) ([vkalintiris](https://github.com/vkalintiris))
- Fix issue with charts not properly synchronized with the cloud [\#12451](https://github.com/netdata/netdata/pull/12451) ([stelfrag](https://github.com/stelfrag))
- Add delay on missing priv\_key [\#12450](https://github.com/netdata/netdata/pull/12450) ([underhood](https://github.com/underhood))
- fix unclaimed agents [\#12449](https://github.com/netdata/netdata/pull/12449) ([underhood](https://github.com/underhood))
- apps.plugin: fix for plugin sending unnecessary data in freebsd [\#12446](https://github.com/netdata/netdata/pull/12446) ([surajnpn](https://github.com/surajnpn))
- fix\(cups.plugin\): add `cups` prefix to chart context [\#12444](https://github.com/netdata/netdata/pull/12444) ([ilyam8](https://github.com/ilyam8))
- Skip `foreach` alarms for dimensions of anomaly rate chart. [\#12441](https://github.com/netdata/netdata/pull/12441) ([vkalintiris](https://github.com/vkalintiris))
- fix: CPU frequency detection of FreeBSD [\#12440](https://github.com/netdata/netdata/pull/12440) ([ilyam8](https://github.com/ilyam8))
- fix install type in netdata-uninstaller.sh [\#12438](https://github.com/netdata/netdata/pull/12438) ([maneamarius](https://github.com/maneamarius))
- fix\(kickstart.sh\): prefer shasum over sha256sum [\#12429](https://github.com/netdata/netdata/pull/12429) ([ilyam8](https://github.com/ilyam8))
- Docs: Fix broken link [\#12428](https://github.com/netdata/netdata/pull/12428) ([kickoke](https://github.com/kickoke))
- Fix OS name for older agent versions streaming to a parent [\#12425](https://github.com/netdata/netdata/pull/12425) ([stelfrag](https://github.com/stelfrag))
- fix: ensure claim\_id is always sent lowercase as string [\#12423](https://github.com/netdata/netdata/pull/12423) ([underhood](https://github.com/underhood))
- fix: lowercase uuidgen [\#12422](https://github.com/netdata/netdata/pull/12422) ([ilyam8](https://github.com/ilyam8))
- fix: add a delay between starting Netdata and checking pids [\#12420](https://github.com/netdata/netdata/pull/12420) ([ilyam8](https://github.com/ilyam8))
- fix\(charts.d.plugin\): fix recursion in apcupsd\_check and enable it again [\#12418](https://github.com/netdata/netdata/pull/12418) ([ilyam8](https://github.com/ilyam8))
- Add content for eBPF documentation [\#12417](https://github.com/netdata/netdata/pull/12417) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: temporarily disable charts.d apcupsd collectors [\#12415](https://github.com/netdata/netdata/pull/12415) ([ilyam8](https://github.com/ilyam8))
- Add additional link to badges doc [\#12412](https://github.com/netdata/netdata/pull/12412) ([Steve8291](https://github.com/Steve8291))
- chore: remove contrib/sles11 [\#12410](https://github.com/netdata/netdata/pull/12410) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.21.3. [\#12407](https://github.com/netdata/netdata/pull/12407) ([netdatabot](https://github.com/netdatabot))
- chore: remove "web files" options leftovers [\#12403](https://github.com/netdata/netdata/pull/12403) ([ilyam8](https://github.com/ilyam8))
- Allow updates without environment files in some cases. [\#12400](https://github.com/netdata/netdata/pull/12400) ([Ferroin](https://github.com/Ferroin))
- Reorder functions properly in updater script. [\#12399](https://github.com/netdata/netdata/pull/12399) ([Ferroin](https://github.com/Ferroin))
- Fix crash when netdatacli command output too long [\#12393](https://github.com/netdata/netdata/pull/12393) ([underhood](https://github.com/underhood))
- Dont check host health enabled if host is null [\#12392](https://github.com/netdata/netdata/pull/12392) ([MrZammler](https://github.com/MrZammler))
- Update build/m4/ax\_pthread.m4 [\#12390](https://github.com/netdata/netdata/pull/12390) ([vkalintiris](https://github.com/vkalintiris))
- Delay removed event for 60 seconds after the chart's last collected time [\#12388](https://github.com/netdata/netdata/pull/12388) ([MrZammler](https://github.com/MrZammler))
- Remove unecessary error report for proc and sys files [\#12385](https://github.com/netdata/netdata/pull/12385) ([thiagoftsm](https://github.com/thiagoftsm))
- feat\(statsd.plugin\): add Asterisk configuration file with synthetic charts [\#12381](https://github.com/netdata/netdata/pull/12381) ([ilyam8](https://github.com/ilyam8))
- fix: handle double host prefix when Netdata running in a podman container [\#12380](https://github.com/netdata/netdata/pull/12380) ([ilyam8](https://github.com/ilyam8))
- fix\(ebpf.plugin\): remove pid file on exit [\#12379](https://github.com/netdata/netdata/pull/12379) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: shellcheck warnings in docker run.sh [\#12377](https://github.com/netdata/netdata/pull/12377) ([ilyam8](https://github.com/ilyam8))
- Fix version handling issues in release workflow. [\#12375](https://github.com/netdata/netdata/pull/12375) ([Ferroin](https://github.com/Ferroin))
- Update Agent version in the Swagger API [\#12374](https://github.com/netdata/netdata/pull/12374) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix handling of checks for newer updater script on update. [\#12367](https://github.com/netdata/netdata/pull/12367) ([Ferroin](https://github.com/Ferroin))
- configure.ac: Unconditionally link against libatomic [\#12366](https://github.com/netdata/netdata/pull/12366) ([AlexGhiti](https://github.com/AlexGhiti))
- Fix handling of pushing commits for release process. [\#12359](https://github.com/netdata/netdata/pull/12359) ([Ferroin](https://github.com/Ferroin))
- Fix chart synchronization with the cloud [\#12356](https://github.com/netdata/netdata/pull/12356) ([stelfrag](https://github.com/stelfrag))
- minor - fix analytics\_build\_info [\#12354](https://github.com/netdata/netdata/pull/12354) ([underhood](https://github.com/underhood))
- Docs fix: Add missing frontmatter [\#12353](https://github.com/netdata/netdata/pull/12353) ([kickoke](https://github.com/kickoke))
- fix\(health\): make ioping\_disk\_latency alarm less sensitive [\#12351](https://github.com/netdata/netdata/pull/12351) ([ilyam8](https://github.com/ilyam8))
- Trigger cloud intergration workflow for PRs and pushes against master [\#12349](https://github.com/netdata/netdata/pull/12349) ([dimko](https://github.com/dimko))
- Improve agent to cloud synchronization performance [\#12348](https://github.com/netdata/netdata/pull/12348) ([stelfrag](https://github.com/stelfrag))
- Prepend context in anomaly rate dimension id. [\#12342](https://github.com/netdata/netdata/pull/12342) ([vkalintiris](https://github.com/vkalintiris))
- Redirect dependency handling script output to logfile when running from the updater. [\#12341](https://github.com/netdata/netdata/pull/12341) ([Ferroin](https://github.com/Ferroin))
- Remove owner check from webserver [\#12339](https://github.com/netdata/netdata/pull/12339) ([thiagoftsm](https://github.com/thiagoftsm))
- fix: container virtualization detection with systemd-detect-virt [\#12338](https://github.com/netdata/netdata/pull/12338) ([ilyam8](https://github.com/ilyam8))
- fix: use default "bind to" in native packages [\#12336](https://github.com/netdata/netdata/pull/12336) ([ilyam8](https://github.com/ilyam8))
- Use the built agent version for Netdata static build archive name. [\#12335](https://github.com/netdata/netdata/pull/12335) ([Ferroin](https://github.com/Ferroin))
- Set repo priority in YUM/DNF repository configuration. [\#12332](https://github.com/netdata/netdata/pull/12332) ([Ferroin](https://github.com/Ferroin))
- Add latency dimension [\#12329](https://github.com/netdata/netdata/pull/12329) ([Steve8291](https://github.com/Steve8291))
- Remove check for config file in stock conf dir [\#12327](https://github.com/netdata/netdata/pull/12327) ([Steve8291](https://github.com/Steve8291))
- fix underscore in libnetfilter-acct-dev package [\#12326](https://github.com/netdata/netdata/pull/12326) ([Steve8291](https://github.com/Steve8291))
- fix: returning 0 for CPU frequency when unknown [\#12323](https://github.com/netdata/netdata/pull/12323) ([ilyam8](https://github.com/ilyam8))
- Add a dry run mode to the kickstart script. [\#12322](https://github.com/netdata/netdata/pull/12322) ([Ferroin](https://github.com/Ferroin))
- fix\(health\): adjust 10s\_ipv4\_tcp\_resets\_sent warn trigger [\#12320](https://github.com/netdata/netdata/pull/12320) ([ilyam8](https://github.com/ilyam8))
- CO-RE and syscalls [\#12318](https://github.com/netdata/netdata/pull/12318) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix 'connect' typo anomaly-detection-python.md [\#12317](https://github.com/netdata/netdata/pull/12317) ([DanTheMediocre](https://github.com/DanTheMediocre))
- Add ml notebooks [\#12313](https://github.com/netdata/netdata/pull/12313) ([andrewm4894](https://github.com/andrewm4894))
- Provide better handling of config files in Docker containers. [\#12310](https://github.com/netdata/netdata/pull/12310) ([Ferroin](https://github.com/Ferroin))
- Replace write with read locks [\#12309](https://github.com/netdata/netdata/pull/12309) ([MrZammler](https://github.com/MrZammler))
- adds node\_id into mirrored\_hosts list [\#12307](https://github.com/netdata/netdata/pull/12307) ([underhood](https://github.com/underhood))

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
