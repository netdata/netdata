# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.37.1...HEAD)

**Merged pull requests:**

- Create ML charts on child hosts. [\#14207](https://github.com/netdata/netdata/pull/14207) ([vkalintiris](https://github.com/vkalintiris))
- Use brackets around info variables [\#14206](https://github.com/netdata/netdata/pull/14206) ([MrZammler](https://github.com/MrZammler))
- Dont call worker\_utilization\_finish\(\) twice [\#14204](https://github.com/netdata/netdata/pull/14204) ([MrZammler](https://github.com/MrZammler))
- Switch to actions/labeler@v4 for labeling PRs. [\#14203](https://github.com/netdata/netdata/pull/14203) ([Ferroin](https://github.com/Ferroin))
- Refactor ML code and add support for multiple KMeans models [\#14198](https://github.com/netdata/netdata/pull/14198) ([vkalintiris](https://github.com/vkalintiris))
- bump go.d.plugin to v0.48.0 [\#14195](https://github.com/netdata/netdata/pull/14195) ([ilyam8](https://github.com/ilyam8))
- Fix typos [\#14194](https://github.com/netdata/netdata/pull/14194) ([rex4539](https://github.com/rex4539))
- add `telegraf` to `apps_groups.conf` monitoring section [\#14188](https://github.com/netdata/netdata/pull/14188) ([andrewm4894](https://github.com/andrewm4894))
- bump go.d.plugin to v0.47.0 [\#14182](https://github.com/netdata/netdata/pull/14182) ([ilyam8](https://github.com/ilyam8))
- fix logrotate postrotate [\#14180](https://github.com/netdata/netdata/pull/14180) ([ilyam8](https://github.com/ilyam8))
- docs: explicitly set the `nofile` limit for Netdata container and document the reason for this [\#14178](https://github.com/netdata/netdata/pull/14178) ([ilyam8](https://github.com/ilyam8))
- remove interface name from cgroup net family [\#14174](https://github.com/netdata/netdata/pull/14174) ([ilyam8](https://github.com/ilyam8))
- use specific charts labels instead of family in alarms [\#14173](https://github.com/netdata/netdata/pull/14173) ([ilyam8](https://github.com/ilyam8))
- Revert "Refactor ML code and add support for multiple KMeans models. … [\#14172](https://github.com/netdata/netdata/pull/14172) ([vkalintiris](https://github.com/vkalintiris))
- fix a typo in debian postinst [\#14171](https://github.com/netdata/netdata/pull/14171) ([ilyam8](https://github.com/ilyam8))
- feat\(packaging\): add netdata to www-data group on Proxmox [\#14168](https://github.com/netdata/netdata/pull/14168) ([ilyam8](https://github.com/ilyam8))
- minor - fix localhost nodeinstance fnc caps [\#14166](https://github.com/netdata/netdata/pull/14166) ([underhood](https://github.com/underhood))
- minor - Adds query type "function\[s\]" for aclk chart [\#14165](https://github.com/netdata/netdata/pull/14165) ([underhood](https://github.com/underhood))
- Fix race on query thread startup [\#14164](https://github.com/netdata/netdata/pull/14164) ([underhood](https://github.com/underhood))
- add alarms and dashboard info for Consul [\#14163](https://github.com/netdata/netdata/pull/14163) ([ilyam8](https://github.com/ilyam8))
- Ensure --claim-url for the claim script is a URL [\#14160](https://github.com/netdata/netdata/pull/14160) ([ralphm](https://github.com/ralphm))
- Finish switch to self-hosted RPM repositories. [\#14158](https://github.com/netdata/netdata/pull/14158) ([Ferroin](https://github.com/Ferroin))
- fix nodejs app detection [\#14156](https://github.com/netdata/netdata/pull/14156) ([ilyam8](https://github.com/ilyam8))
- Populate field values in send\_slack\(\) for Mattermost [\#14153](https://github.com/netdata/netdata/pull/14153) ([je2555](https://github.com/je2555))
- bump go.d.plugin to v0.46.1 [\#14151](https://github.com/netdata/netdata/pull/14151) ([ilyam8](https://github.com/ilyam8))
- Add a health configuration option of which alarms to load [\#14150](https://github.com/netdata/netdata/pull/14150) ([MrZammler](https://github.com/MrZammler))
- Disable integration by default \(eBPF \<-\> APPS\) [\#14147](https://github.com/netdata/netdata/pull/14147) ([thiagoftsm](https://github.com/thiagoftsm))
- Revert "MQTT 5 publish topic alias support" [\#14145](https://github.com/netdata/netdata/pull/14145) ([MrZammler](https://github.com/MrZammler))
- Document memory mode alloc [\#14142](https://github.com/netdata/netdata/pull/14142) ([cakrit](https://github.com/cakrit))
- fix\(packaging\): add setuid for cgroup-network and ebpf.plugin in RPM [\#14140](https://github.com/netdata/netdata/pull/14140) ([ilyam8](https://github.com/ilyam8))
- use chart labels in portcheck alarms [\#14137](https://github.com/netdata/netdata/pull/14137) ([ilyam8](https://github.com/ilyam8))
- Remove Fedora 35 from the list of supported platforms. [\#14136](https://github.com/netdata/netdata/pull/14136) ([Ferroin](https://github.com/Ferroin))
- Fix conditions for uploading repoconfig packages to new infra. [\#14134](https://github.com/netdata/netdata/pull/14134) ([Ferroin](https://github.com/Ferroin))
- fix httpcheck alarms [\#14133](https://github.com/netdata/netdata/pull/14133) ([ilyam8](https://github.com/ilyam8))
- eBPF \(memory, NV, basis for functions\) [\#14131](https://github.com/netdata/netdata/pull/14131) ([thiagoftsm](https://github.com/thiagoftsm))
- bump go.d.plugin to v0.46.0 [\#14124](https://github.com/netdata/netdata/pull/14124) ([ilyam8](https://github.com/ilyam8))
- heartbeat: don't log every discrepancy [\#14122](https://github.com/netdata/netdata/pull/14122) ([ktsaou](https://github.com/ktsaou))
- ARAL: add destroy function and optimize ifdefs [\#14121](https://github.com/netdata/netdata/pull/14121) ([ktsaou](https://github.com/ktsaou))
- ci: fix cgroup-parent name in packaging [\#14118](https://github.com/netdata/netdata/pull/14118) ([ilyam8](https://github.com/ilyam8))
- don't log too much about streaming connections [\#14117](https://github.com/netdata/netdata/pull/14117) ([ktsaou](https://github.com/ktsaou))
- fix get\_system\_cpus\(\) [\#14116](https://github.com/netdata/netdata/pull/14116) ([ktsaou](https://github.com/ktsaou))
- Fix minor typo. [\#14111](https://github.com/netdata/netdata/pull/14111) ([tkatsoulas](https://github.com/tkatsoulas))
- expose ACLK SSL KeyLog interface for developers [\#14109](https://github.com/netdata/netdata/pull/14109) ([underhood](https://github.com/underhood))
- add filtering options to functions table output [\#14108](https://github.com/netdata/netdata/pull/14108) ([ktsaou](https://github.com/ktsaou))
- fix health emphemerality labels src [\#14105](https://github.com/netdata/netdata/pull/14105) ([ilyam8](https://github.com/ilyam8))
- fix docker host editable config [\#14104](https://github.com/netdata/netdata/pull/14104) ([ilyam8](https://github.com/ilyam8))
- Switch to self-hosted infrastructure for RPM package distribution. [\#14100](https://github.com/netdata/netdata/pull/14100) ([Ferroin](https://github.com/Ferroin))
- Fix missing required package install of tar on FreeBSD [\#14095](https://github.com/netdata/netdata/pull/14095) ([Dim-P](https://github.com/Dim-P))
- Add version to netdatacli [\#14094](https://github.com/netdata/netdata/pull/14094) ([MrZammler](https://github.com/MrZammler))
- docs: add a note to set container nofile ulimit for Fedora users [\#14092](https://github.com/netdata/netdata/pull/14092) ([ilyam8](https://github.com/ilyam8))
- Fix eBPF load on RH 8.x family and improve code. [\#14090](https://github.com/netdata/netdata/pull/14090) ([thiagoftsm](https://github.com/thiagoftsm))
- fix v1.37 dbengine page alignment crashes [\#14086](https://github.com/netdata/netdata/pull/14086) ([ktsaou](https://github.com/ktsaou))
- Fix \_\_atomic\_compare\_exchange\_n\(\) atomics [\#14085](https://github.com/netdata/netdata/pull/14085) ([ktsaou](https://github.com/ktsaou))
- Fix 1.37 crashes [\#14081](https://github.com/netdata/netdata/pull/14081) ([stelfrag](https://github.com/stelfrag))
- add basic dashboard info for NGINX Plus [\#14080](https://github.com/netdata/netdata/pull/14080) ([ilyam8](https://github.com/ilyam8))
- replication fixes 9 [\#14079](https://github.com/netdata/netdata/pull/14079) ([ktsaou](https://github.com/ktsaou))
- optimize workers statistics performance [\#14077](https://github.com/netdata/netdata/pull/14077) ([ktsaou](https://github.com/ktsaou))
- fix SSL related crashes [\#14076](https://github.com/netdata/netdata/pull/14076) ([ktsaou](https://github.com/ktsaou))
- remove python.d/springboot [\#14075](https://github.com/netdata/netdata/pull/14075) ([ilyam8](https://github.com/ilyam8))
- fix backfilling statistics [\#14074](https://github.com/netdata/netdata/pull/14074) ([ktsaou](https://github.com/ktsaou))
- remove deprecated fping.plugin in accordance with v1.37.0 deprecation notice  [\#14073](https://github.com/netdata/netdata/pull/14073) ([ilyam8](https://github.com/ilyam8))
- remove deprecated python.d collectors announced in v1.37.0 deprecation notice [\#14072](https://github.com/netdata/netdata/pull/14072) ([ilyam8](https://github.com/ilyam8))
- Add workflow dispatch trigger for parent/child with cloud integration smoke tests [\#14070](https://github.com/netdata/netdata/pull/14070) ([dimko](https://github.com/dimko))
- MQTT 5 publish topic alias support [\#14067](https://github.com/netdata/netdata/pull/14067) ([underhood](https://github.com/underhood))
- Refactor ML code and add support for multiple KMeans models. [\#14065](https://github.com/netdata/netdata/pull/14065) ([vkalintiris](https://github.com/vkalintiris))
- add clickhouse third party collector and install instructions [\#14021](https://github.com/netdata/netdata/pull/14021) ([andrewm4894](https://github.com/andrewm4894))
- Wmi descriptions [\#14001](https://github.com/netdata/netdata/pull/14001) ([thiagoftsm](https://github.com/thiagoftsm))
- Finish renaming the `--install` option to `--install-prefix`. [\#13881](https://github.com/netdata/netdata/pull/13881) ([Ferroin](https://github.com/Ferroin))
- Support HTTP proxy Basic auth [\#13762](https://github.com/netdata/netdata/pull/13762) ([underhood](https://github.com/underhood))
- Cleanly reimplement system/edit-config.in. [\#13702](https://github.com/netdata/netdata/pull/13702) ([Ferroin](https://github.com/Ferroin))

## [v1.37.1](https://github.com/netdata/netdata/tree/v1.37.1) (2022-12-05)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.37.0...v1.37.1)

## [v1.37.0](https://github.com/netdata/netdata/tree/v1.37.0) (2022-11-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.36.1...v1.37.0)

**Merged pull requests:**

- fix spinlock [\#14068](https://github.com/netdata/netdata/pull/14068) ([ktsaou](https://github.com/ktsaou))
- Sanitize command arguments. [\#14064](https://github.com/netdata/netdata/pull/14064) ([vkalintiris](https://github.com/vkalintiris))
- Strict control of streaming API keys and MACHINE GUIDs in stream.conf [\#14063](https://github.com/netdata/netdata/pull/14063) ([ktsaou](https://github.com/ktsaou))
- replication fixes No 8 [\#14061](https://github.com/netdata/netdata/pull/14061) ([ktsaou](https://github.com/ktsaou))
- fix\(updater\): don't produce any output if static update completed successfully [\#14058](https://github.com/netdata/netdata/pull/14058) ([ilyam8](https://github.com/ilyam8))
- Add Alpine 3.17 to supported distros. [\#14056](https://github.com/netdata/netdata/pull/14056) ([Ferroin](https://github.com/Ferroin))
- replication fixes No 7 [\#14053](https://github.com/netdata/netdata/pull/14053) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin to v0.45.0 [\#14052](https://github.com/netdata/netdata/pull/14052) ([ilyam8](https://github.com/ilyam8))
- Remove eBPF plugin warning [\#14047](https://github.com/netdata/netdata/pull/14047) ([thiagoftsm](https://github.com/thiagoftsm))
- replication fixes \#6 [\#14046](https://github.com/netdata/netdata/pull/14046) ([ktsaou](https://github.com/ktsaou))
- fix build on old openssl versions on centos [\#14045](https://github.com/netdata/netdata/pull/14045) ([underhood](https://github.com/underhood))
- Don't let slow disk plugin thread delay shutdown [\#14044](https://github.com/netdata/netdata/pull/14044) ([MrZammler](https://github.com/MrZammler))
- minor - wss to point to master instead of branch [\#14043](https://github.com/netdata/netdata/pull/14043) ([underhood](https://github.com/underhood))
- fix dictionaries unittest [\#14042](https://github.com/netdata/netdata/pull/14042) ([ktsaou](https://github.com/ktsaou))
- minor - Adds better information in case of SSL error [\#14041](https://github.com/netdata/netdata/pull/14041) ([underhood](https://github.com/underhood))
- replication fixes \#5 [\#14038](https://github.com/netdata/netdata/pull/14038) ([ktsaou](https://github.com/ktsaou))
- do not merge duplicate replication requests  [\#14037](https://github.com/netdata/netdata/pull/14037) ([ktsaou](https://github.com/ktsaou))
- Replication fixes \#3 [\#14035](https://github.com/netdata/netdata/pull/14035) ([ktsaou](https://github.com/ktsaou))
- improve performance of worker utilization statistics [\#14034](https://github.com/netdata/netdata/pull/14034) ([ktsaou](https://github.com/ktsaou))
- use 2 levels of judy arrays to speed up replication on very busy parents [\#14031](https://github.com/netdata/netdata/pull/14031) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin v0.44.0 [\#14030](https://github.com/netdata/netdata/pull/14030) ([ilyam8](https://github.com/ilyam8))
- remove retries from SSL [\#14026](https://github.com/netdata/netdata/pull/14026) ([ktsaou](https://github.com/ktsaou))
- Fix documentation TLS streaming [\#14024](https://github.com/netdata/netdata/pull/14024) ([thiagoftsm](https://github.com/thiagoftsm))
- streaming compression, query planner and replication fixes [\#14023](https://github.com/netdata/netdata/pull/14023) ([ktsaou](https://github.com/ktsaou))
- Change relative links to absolute for learn components [\#14015](https://github.com/netdata/netdata/pull/14015) ([tkatsoulas](https://github.com/tkatsoulas))
- allow statsd tags to modify chart metadata on the fly [\#14014](https://github.com/netdata/netdata/pull/14014) ([ktsaou](https://github.com/ktsaou))
- minor - silence misleading error [\#14013](https://github.com/netdata/netdata/pull/14013) ([underhood](https://github.com/underhood))
- Improve eBPF exit [\#14012](https://github.com/netdata/netdata/pull/14012) ([thiagoftsm](https://github.com/thiagoftsm))
- Change static image urls to app.netdata.cloud in alarm-notify.sh [\#14007](https://github.com/netdata/netdata/pull/14007) ([MrZammler](https://github.com/MrZammler))
- Fix connection resets on big parents [\#14004](https://github.com/netdata/netdata/pull/14004) ([underhood](https://github.com/underhood))
- minor typo in the uninstall command [\#14002](https://github.com/netdata/netdata/pull/14002) ([tkatsoulas](https://github.com/tkatsoulas))
- Revert "New journal disk based indexing for agent memory reduction" [\#14000](https://github.com/netdata/netdata/pull/14000) ([ktsaou](https://github.com/ktsaou))
- Revert "enable dbengine tiering by default" [\#13999](https://github.com/netdata/netdata/pull/13999) ([ktsaou](https://github.com/ktsaou))
- fixes MQTT-NG QoS0 [\#13997](https://github.com/netdata/netdata/pull/13997) ([underhood](https://github.com/underhood))
- remove python.d/nginx\_plus [\#13995](https://github.com/netdata/netdata/pull/13995) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.43.1 [\#13993](https://github.com/netdata/netdata/pull/13993) ([ilyam8](https://github.com/ilyam8))
- Add 'funcs' capability [\#13992](https://github.com/netdata/netdata/pull/13992) ([underhood](https://github.com/underhood))
- added debug info on left-over query targets [\#13990](https://github.com/netdata/netdata/pull/13990) ([ktsaou](https://github.com/ktsaou))
- replication improvements [\#13989](https://github.com/netdata/netdata/pull/13989) ([ktsaou](https://github.com/ktsaou))
- use calculator app instead of spreadsheet [\#13981](https://github.com/netdata/netdata/pull/13981) ([andrewm4894](https://github.com/andrewm4894))
- apps.plugin function processes: keys should not have spaces [\#13980](https://github.com/netdata/netdata/pull/13980) ([ktsaou](https://github.com/ktsaou))
- dont crash when netdata cannot execute its external plugins [\#13978](https://github.com/netdata/netdata/pull/13978) ([ktsaou](https://github.com/ktsaou))
- Add \_total suffix to raw increment metrics for remote write [\#13977](https://github.com/netdata/netdata/pull/13977) ([vlvkobal](https://github.com/vlvkobal))
- add Cassandra icon to dashboard info [\#13975](https://github.com/netdata/netdata/pull/13975) ([ilyam8](https://github.com/ilyam8))
- Change the db-engine calculator to a read only gsheet [\#13974](https://github.com/netdata/netdata/pull/13974) ([tkatsoulas](https://github.com/tkatsoulas))
- enable collecting ECC memory errors by default [\#13970](https://github.com/netdata/netdata/pull/13970) ([ilyam8](https://github.com/ilyam8))
- break active-active loop from replicating non-existing child to each other [\#13968](https://github.com/netdata/netdata/pull/13968) ([ktsaou](https://github.com/ktsaou))
- document password param for tor collector [\#13966](https://github.com/netdata/netdata/pull/13966) ([andrewm4894](https://github.com/andrewm4894))
- Fallback to ar and ranlib if llvm-ar and llvm-ranlib are not there [\#13959](https://github.com/netdata/netdata/pull/13959) ([MrZammler](https://github.com/MrZammler))
- require -DENABLE\_DLSYM=1 to use dlsym\(\) [\#13958](https://github.com/netdata/netdata/pull/13958) ([ktsaou](https://github.com/ktsaou))
- health/ping: use 'host' label in alerts info [\#13955](https://github.com/netdata/netdata/pull/13955) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.43.0 [\#13954](https://github.com/netdata/netdata/pull/13954) ([ilyam8](https://github.com/ilyam8))
- Fix local dashboard cloud links [\#13953](https://github.com/netdata/netdata/pull/13953) ([underhood](https://github.com/underhood))
- Remove health\_thread\_stop [\#13948](https://github.com/netdata/netdata/pull/13948) ([MrZammler](https://github.com/MrZammler))
- Provide improved messaging in the kickstart script for existing installs managed by the system package manager. [\#13947](https://github.com/netdata/netdata/pull/13947) ([Ferroin](https://github.com/Ferroin))
- do not resend charts upstream when chart variables are being updated [\#13946](https://github.com/netdata/netdata/pull/13946) ([ktsaou](https://github.com/ktsaou))
- recalculate last\_collected\_total [\#13945](https://github.com/netdata/netdata/pull/13945) ([ktsaou](https://github.com/ktsaou))
- fix chart definition end time\_t printing and parsing [\#13942](https://github.com/netdata/netdata/pull/13942) ([ktsaou](https://github.com/ktsaou))
- Setup default certificates path [\#13941](https://github.com/netdata/netdata/pull/13941) ([MrZammler](https://github.com/MrZammler))
- update link to point at demo spaces [\#13939](https://github.com/netdata/netdata/pull/13939) ([andrewm4894](https://github.com/andrewm4894))
- Statsd dictionaries should be multi-threaded [\#13938](https://github.com/netdata/netdata/pull/13938) ([ktsaou](https://github.com/ktsaou))
- Fix oracle linux \(eBPF plugin\) [\#13935](https://github.com/netdata/netdata/pull/13935) ([thiagoftsm](https://github.com/thiagoftsm))
- Update print message on startup [\#13934](https://github.com/netdata/netdata/pull/13934) ([andrewm4894](https://github.com/andrewm4894))
- Rrddim acquire on replay set [\#13932](https://github.com/netdata/netdata/pull/13932) ([ktsaou](https://github.com/ktsaou))
- fix compiling without dbengine [\#13931](https://github.com/netdata/netdata/pull/13931) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.42.1 [\#13930](https://github.com/netdata/netdata/pull/13930) ([ilyam8](https://github.com/ilyam8))
- Remove pluginsd action param & dead code. [\#13928](https://github.com/netdata/netdata/pull/13928) ([vkalintiris](https://github.com/vkalintiris))
- Do not force internal collectors to call rrdset\_next. [\#13926](https://github.com/netdata/netdata/pull/13926) ([vkalintiris](https://github.com/vkalintiris))
- Return accidentaly removed 32bit RPi keep alive fix [\#13925](https://github.com/netdata/netdata/pull/13925) ([underhood](https://github.com/underhood))
- error\_limit\(\) function to limit number of error lines per instance [\#13924](https://github.com/netdata/netdata/pull/13924) ([ktsaou](https://github.com/ktsaou))
- fix crash on query plan switch [\#13920](https://github.com/netdata/netdata/pull/13920) ([ktsaou](https://github.com/ktsaou))
- Enable aclk conversation log even without NETDATA\_INTERNAL CHECKS [\#13917](https://github.com/netdata/netdata/pull/13917) ([MrZammler](https://github.com/MrZammler))
- add ping dashboard info and alarms [\#13916](https://github.com/netdata/netdata/pull/13916) ([ilyam8](https://github.com/ilyam8))
- enable dbengine tiering by default [\#13914](https://github.com/netdata/netdata/pull/13914) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin to v0.42.0 [\#13913](https://github.com/netdata/netdata/pull/13913) ([ilyam8](https://github.com/ilyam8))
- do not free hosts if a change on db mode is not needed [\#13912](https://github.com/netdata/netdata/pull/13912) ([ktsaou](https://github.com/ktsaou))
- timeframe matching should take into account the update frequency of the chart [\#13911](https://github.com/netdata/netdata/pull/13911) ([ktsaou](https://github.com/ktsaou))
- WMI Process \(Dashboard, Documentation\) [\#13910](https://github.com/netdata/netdata/pull/13910) ([thiagoftsm](https://github.com/thiagoftsm))
- feat\(packaging\): add CAP\_NET\_RAW to go.d.plugin [\#13909](https://github.com/netdata/netdata/pull/13909) ([ilyam8](https://github.com/ilyam8))
- Reference the bash collector for RPi [\#13907](https://github.com/netdata/netdata/pull/13907) ([cakrit](https://github.com/cakrit))
- Improve intro paragraph [\#13906](https://github.com/netdata/netdata/pull/13906) ([cakrit](https://github.com/cakrit))
- bump go.d.plugin v0.41.2 [\#13903](https://github.com/netdata/netdata/pull/13903) ([ilyam8](https://github.com/ilyam8))
- apps.plugin function add max value on all value columns [\#13899](https://github.com/netdata/netdata/pull/13899) ([ktsaou](https://github.com/ktsaou))
- Reduce unnecessary alert events to the cloud [\#13897](https://github.com/netdata/netdata/pull/13897) ([MrZammler](https://github.com/MrZammler))
- add pandas collector to collectors.md [\#13895](https://github.com/netdata/netdata/pull/13895) ([andrewm4894](https://github.com/andrewm4894))
- Fix reading health "enable" from the configuration [\#13894](https://github.com/netdata/netdata/pull/13894) ([stelfrag](https://github.com/stelfrag))
- fix\(proc.plugin\): fix read retry logic when reading interface speed [\#13893](https://github.com/netdata/netdata/pull/13893) ([ilyam8](https://github.com/ilyam8))
- Record installation command in telemetry events. [\#13892](https://github.com/netdata/netdata/pull/13892) ([Ferroin](https://github.com/Ferroin))
- Prompt users about updates/claiming on unknown install types. [\#13890](https://github.com/netdata/netdata/pull/13890) ([Ferroin](https://github.com/Ferroin))
- tune rrdcontext timings [\#13889](https://github.com/netdata/netdata/pull/13889) ([ktsaou](https://github.com/ktsaou))
- filtering out charts in context queries, includes them in full\_xxx variables [\#13886](https://github.com/netdata/netdata/pull/13886) ([ktsaou](https://github.com/ktsaou))
- New journal disk based indexing for agent memory reduction [\#13885](https://github.com/netdata/netdata/pull/13885) ([stelfrag](https://github.com/stelfrag))
- Fix systemd chart update \(eBPF\) [\#13884](https://github.com/netdata/netdata/pull/13884) ([thiagoftsm](https://github.com/thiagoftsm))
- apps.plugin function processes cosmetic changes [\#13880](https://github.com/netdata/netdata/pull/13880) ([ktsaou](https://github.com/ktsaou))
- allow single chart to be filtered in context queries [\#13879](https://github.com/netdata/netdata/pull/13879) ([ktsaou](https://github.com/ktsaou))
- Add description for TCP WMI [\#13878](https://github.com/netdata/netdata/pull/13878) ([thiagoftsm](https://github.com/thiagoftsm))
- Use print macros [\#13876](https://github.com/netdata/netdata/pull/13876) ([MrZammler](https://github.com/MrZammler))
- Suppress ML and dlib ABI warnings [\#13875](https://github.com/netdata/netdata/pull/13875) ([Dim-P](https://github.com/Dim-P))
- bump go.d.plugin v0.41.1 [\#13874](https://github.com/netdata/netdata/pull/13874) ([ilyam8](https://github.com/ilyam8))
- Replication of metrics \(gaps filling\) during streaming [\#13873](https://github.com/netdata/netdata/pull/13873) ([vkalintiris](https://github.com/vkalintiris))
- Don't create a REMOVED alert event after a REMOVED. [\#13871](https://github.com/netdata/netdata/pull/13871) ([MrZammler](https://github.com/MrZammler))
- Store hidden status when creating / updating dimension metadata [\#13869](https://github.com/netdata/netdata/pull/13869) ([stelfrag](https://github.com/stelfrag))
- Find the chart and dimension UUID from the context [\#13868](https://github.com/netdata/netdata/pull/13868) ([stelfrag](https://github.com/stelfrag))
- fix\(cgroup.plugin\): handle qemu-1- prefix when extracting virsh domain [\#13866](https://github.com/netdata/netdata/pull/13866) ([ilyam8](https://github.com/ilyam8))
- Update step-09 for dbmode update.md [\#13864](https://github.com/netdata/netdata/pull/13864) ([DShreve2](https://github.com/DShreve2))
- add ACLK access to ml\_info \(fix anomalies tab in cloud\) [\#13863](https://github.com/netdata/netdata/pull/13863) ([underhood](https://github.com/underhood))
- bump go.d.plugin to v0.41.0 [\#13861](https://github.com/netdata/netdata/pull/13861) ([ilyam8](https://github.com/ilyam8))
- app to api netdata cloud [\#13856](https://github.com/netdata/netdata/pull/13856) ([underhood](https://github.com/underhood))
- Use llvm's ar and ranlib when compiling with clang [\#13854](https://github.com/netdata/netdata/pull/13854) ([MrZammler](https://github.com/MrZammler))
- fix typo [\#13853](https://github.com/netdata/netdata/pull/13853) ([andrewm4894](https://github.com/andrewm4894))
- Retry reading carrier, duplex, and speed files periodically [\#13850](https://github.com/netdata/netdata/pull/13850) ([vlvkobal](https://github.com/vlvkobal))
- Properly guard commands when installing services for offline service managers. [\#13848](https://github.com/netdata/netdata/pull/13848) ([Ferroin](https://github.com/Ferroin))
- fix tiers update frequency [\#13844](https://github.com/netdata/netdata/pull/13844) ([ktsaou](https://github.com/ktsaou))
- Fix service installation on FreeBSD. [\#13842](https://github.com/netdata/netdata/pull/13842) ([Ferroin](https://github.com/Ferroin))
- Cassandra dashboard description [\#13835](https://github.com/netdata/netdata/pull/13835) ([thiagoftsm](https://github.com/thiagoftsm))
- Use mmap to read an extent from a datafile [\#13834](https://github.com/netdata/netdata/pull/13834) ([stelfrag](https://github.com/stelfrag))
- chore\(health\): rm pihole\_blocklist\_gravity\_file\_existence\_state [\#13826](https://github.com/netdata/netdata/pull/13826) ([ilyam8](https://github.com/ilyam8))
- Improve error and warning messages in the kickstart script. [\#13825](https://github.com/netdata/netdata/pull/13825) ([Ferroin](https://github.com/Ferroin))
- Remove option to use MQTT 3 [\#13824](https://github.com/netdata/netdata/pull/13824) ([underhood](https://github.com/underhood))
- extended processes function info from apps.plugin [\#13822](https://github.com/netdata/netdata/pull/13822) ([ktsaou](https://github.com/ktsaou))
- Fix crash on child reconnect and lost metrics [\#13821](https://github.com/netdata/netdata/pull/13821) ([stelfrag](https://github.com/stelfrag))
- Remove NFS readahead histogram [\#13819](https://github.com/netdata/netdata/pull/13819) ([vlvkobal](https://github.com/vlvkobal))
- minor - add trace alloc to buildinfo [\#13817](https://github.com/netdata/netdata/pull/13817) ([underhood](https://github.com/underhood))
- Fix exporting unit tests [\#13816](https://github.com/netdata/netdata/pull/13816) ([vlvkobal](https://github.com/vlvkobal))
- Inject costallocz to mqtt\_websockets library and its children [\#13813](https://github.com/netdata/netdata/pull/13813) ([underhood](https://github.com/underhood))
- overload libc memory allocators with custom ones to trace all allocations [\#13810](https://github.com/netdata/netdata/pull/13810) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin v0.40.4 [\#13808](https://github.com/netdata/netdata/pull/13808) ([ilyam8](https://github.com/ilyam8))
- fix post-processing of contexts [\#13807](https://github.com/netdata/netdata/pull/13807) ([ktsaou](https://github.com/ktsaou))
- Merge netstat, snmp, and snmp6 modules [\#13806](https://github.com/netdata/netdata/pull/13806) ([vlvkobal](https://github.com/vlvkobal))
- fix warning when -Wfree-nonheap-object is used [\#13805](https://github.com/netdata/netdata/pull/13805) ([underhood](https://github.com/underhood))
- ARAL optimal alloc size [\#13804](https://github.com/netdata/netdata/pull/13804) ([ktsaou](https://github.com/ktsaou))
- internal log error, when passing NULL dictionary [\#13803](https://github.com/netdata/netdata/pull/13803) ([ktsaou](https://github.com/ktsaou))
- Properly propagate errors from installer/updater to kickstart script. [\#13802](https://github.com/netdata/netdata/pull/13802) ([Ferroin](https://github.com/Ferroin))
- Add up to date info on improving performance [\#13801](https://github.com/netdata/netdata/pull/13801) ([cakrit](https://github.com/cakrit))
- Return memory freed properly [\#13799](https://github.com/netdata/netdata/pull/13799) ([stelfrag](https://github.com/stelfrag))
- Use string\_freez instead of freez in rrdhost\_init\_timezone [\#13798](https://github.com/netdata/netdata/pull/13798) ([MrZammler](https://github.com/MrZammler))
- Fix runtime directory ownership when installed as non-root user. [\#13797](https://github.com/netdata/netdata/pull/13797) ([Ferroin](https://github.com/Ferroin))
- Fix minor typo in systemdunits.conf alert [\#13796](https://github.com/netdata/netdata/pull/13796) ([tkatsoulas](https://github.com/tkatsoulas))
- Also enable rrdvars from health [\#13795](https://github.com/netdata/netdata/pull/13795) ([MrZammler](https://github.com/MrZammler))
- feat\(python.d\): respect NETDATA\_INTERNALS\_MONITORING [\#13793](https://github.com/netdata/netdata/pull/13793) ([ilyam8](https://github.com/ilyam8))
- Array Allocator Memory Leak Fix [\#13792](https://github.com/netdata/netdata/pull/13792) ([ktsaou](https://github.com/ktsaou))
- Add variants of functions allowing callers to specify the time to use. [\#13791](https://github.com/netdata/netdata/pull/13791) ([vkalintiris](https://github.com/vkalintiris))
- Remove extern from function declared in headers. [\#13790](https://github.com/netdata/netdata/pull/13790) ([vkalintiris](https://github.com/vkalintiris))
- full memory tracking and profiling of Netdata Agent [\#13789](https://github.com/netdata/netdata/pull/13789) ([ktsaou](https://github.com/ktsaou))
- allow disabling netdata monitoring section of the dashboard [\#13788](https://github.com/netdata/netdata/pull/13788) ([ktsaou](https://github.com/ktsaou))
- Stop pulling in netcat as a mandatory dependency. [\#13787](https://github.com/netdata/netdata/pull/13787) ([Ferroin](https://github.com/Ferroin))
- Initialize st-\>rrdvars from rrdset insert callback [\#13786](https://github.com/netdata/netdata/pull/13786) ([MrZammler](https://github.com/MrZammler))
- Add Ubuntu 22.10 to supported distros, CI, and package builds. [\#13785](https://github.com/netdata/netdata/pull/13785) ([Ferroin](https://github.com/Ferroin))
- minor - add host labels for ephemerality and nodes with unstable connections [\#13784](https://github.com/netdata/netdata/pull/13784) ([underhood](https://github.com/underhood))
- Add a thread to asynchronously process metadata updates [\#13783](https://github.com/netdata/netdata/pull/13783) ([stelfrag](https://github.com/stelfrag))
- Parser cleanup  [\#13782](https://github.com/netdata/netdata/pull/13782) ([stelfrag](https://github.com/stelfrag))
- allow netdata installer to install and run netdata as any user [\#13780](https://github.com/netdata/netdata/pull/13780) ([ktsaou](https://github.com/ktsaou))
- Update libbpf 1.0.1 [\#13778](https://github.com/netdata/netdata/pull/13778) ([thiagoftsm](https://github.com/thiagoftsm))
- Bump websockets submodule [\#13776](https://github.com/netdata/netdata/pull/13776) ([underhood](https://github.com/underhood))
- Rename variable for old CentOS version [\#13775](https://github.com/netdata/netdata/pull/13775) ([thiagoftsm](https://github.com/thiagoftsm))
- Further improvements to the new service installation code. [\#13774](https://github.com/netdata/netdata/pull/13774) ([Ferroin](https://github.com/Ferroin))
- Pandas collector [\#13773](https://github.com/netdata/netdata/pull/13773) ([andrewm4894](https://github.com/andrewm4894))
- dbengine free from RRDSET and RRDDIM [\#13772](https://github.com/netdata/netdata/pull/13772) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin v0.40.3 [\#13771](https://github.com/netdata/netdata/pull/13771) ([ilyam8](https://github.com/ilyam8))
- Update fping plugin documentation with better details about the required version. [\#13765](https://github.com/netdata/netdata/pull/13765) ([Ferroin](https://github.com/Ferroin))
- fix bad merge [\#13764](https://github.com/netdata/netdata/pull/13764) ([ktsaou](https://github.com/ktsaou))
- Remove anomaly rates chart. [\#13763](https://github.com/netdata/netdata/pull/13763) ([vkalintiris](https://github.com/vkalintiris))
- add 1m delay for tcp reset alarms [\#13761](https://github.com/netdata/netdata/pull/13761) ([ilyam8](https://github.com/ilyam8))
- Use /bin/sh instead of ls to detect glibc [\#13758](https://github.com/netdata/netdata/pull/13758) ([MrZammler](https://github.com/MrZammler))
- Add ZFS rate charts [\#13757](https://github.com/netdata/netdata/pull/13757) ([vlvkobal](https://github.com/vlvkobal))
- Count currently streaming senders on the localhost [\#13755](https://github.com/netdata/netdata/pull/13755) ([MrZammler](https://github.com/MrZammler))
- Fix streaming crash when child reconnects and is archived on the parent [\#13754](https://github.com/netdata/netdata/pull/13754) ([stelfrag](https://github.com/stelfrag))
- Add CloudLinux OS detection to the updater script [\#13752](https://github.com/netdata/netdata/pull/13752) ([Pulseeey](https://github.com/Pulseeey))
- Add CloudLinux OS detection to kickstart [\#13750](https://github.com/netdata/netdata/pull/13750) ([Pulseeey](https://github.com/Pulseeey))
- bump go.d v0.40.2 [\#13747](https://github.com/netdata/netdata/pull/13747) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d\): set correct label source for \_collect\_job label [\#13746](https://github.com/netdata/netdata/pull/13746) ([ilyam8](https://github.com/ilyam8))
- Provide Details on Label Filtering/Custom Labels [\#13745](https://github.com/netdata/netdata/pull/13745) ([DShreve2](https://github.com/DShreve2))
- Fix handling of temporary directories in kickstart code. [\#13744](https://github.com/netdata/netdata/pull/13744) ([Ferroin](https://github.com/Ferroin))
- Dont send NodeInfo during first database cleanup [\#13740](https://github.com/netdata/netdata/pull/13740) ([MrZammler](https://github.com/MrZammler))
- CMake - add possibility to build without ACLK [\#13736](https://github.com/netdata/netdata/pull/13736) ([underhood](https://github.com/underhood))
- Change cast to remove coverity warnings [\#13735](https://github.com/netdata/netdata/pull/13735) ([thiagoftsm](https://github.com/thiagoftsm))
- Do not try to start an archived host in dbengine if dbengine is not compiled [\#13724](https://github.com/netdata/netdata/pull/13724) ([stelfrag](https://github.com/stelfrag))
- Allow netdata plugins to expose functions for querying more information about specific charts [\#13720](https://github.com/netdata/netdata/pull/13720) ([ktsaou](https://github.com/ktsaou))
- feat\(health\): add new Redis alarms [\#13715](https://github.com/netdata/netdata/pull/13715) ([ilyam8](https://github.com/ilyam8))
- Health thread per host [\#13712](https://github.com/netdata/netdata/pull/13712) ([MrZammler](https://github.com/MrZammler))
- Faster streaming by 25% on the child [\#13708](https://github.com/netdata/netdata/pull/13708) ([ktsaou](https://github.com/ktsaou))
- Do not create train/predict dimensions meant for tracking anomaly rates. [\#13707](https://github.com/netdata/netdata/pull/13707) ([vkalintiris](https://github.com/vkalintiris))
- Update exporting unit tests [\#13706](https://github.com/netdata/netdata/pull/13706) ([vlvkobal](https://github.com/vlvkobal))
- bump go.d.plugin to v0.40.1 [\#13704](https://github.com/netdata/netdata/pull/13704) ([ilyam8](https://github.com/ilyam8))
- Build judy even without dbengine [\#13703](https://github.com/netdata/netdata/pull/13703) ([underhood](https://github.com/underhood))
- alarms collector: ability to exclude certain alarms via config [\#13701](https://github.com/netdata/netdata/pull/13701) ([andrewm4894](https://github.com/andrewm4894))
- Fix inconsistent alert class names [\#13699](https://github.com/netdata/netdata/pull/13699) ([ralphm](https://github.com/ralphm))
- disable Postgres last vacuum/analyze alarms [\#13698](https://github.com/netdata/netdata/pull/13698) ([ilyam8](https://github.com/ilyam8))
- QUERY\_TARGET: new query engine for Netdata Agent [\#13697](https://github.com/netdata/netdata/pull/13697) ([ktsaou](https://github.com/ktsaou))
- Update dashboard to version v2.29.1. [\#13696](https://github.com/netdata/netdata/pull/13696) ([netdatabot](https://github.com/netdatabot))
- docs: nvidia-smi in a container limitation note [\#13695](https://github.com/netdata/netdata/pull/13695) ([ilyam8](https://github.com/ilyam8))
- Use CMake generated config.h also in out of tree CMake build [\#13692](https://github.com/netdata/netdata/pull/13692) ([underhood](https://github.com/underhood))
- Add info for Docker containers about using hostname from host. [\#13685](https://github.com/netdata/netdata/pull/13685) ([Ferroin](https://github.com/Ferroin))
- add node level AR based example [\#13684](https://github.com/netdata/netdata/pull/13684) ([andrewm4894](https://github.com/andrewm4894))
- Store nulls instead of empty strings in health tables [\#13683](https://github.com/netdata/netdata/pull/13683) ([MrZammler](https://github.com/MrZammler))
- Fix warnings during compilation time on ARM \(32 bits\) [\#13681](https://github.com/netdata/netdata/pull/13681) ([thiagoftsm](https://github.com/thiagoftsm))
- dictionary updated documentation and cosmetics [\#13679](https://github.com/netdata/netdata/pull/13679) ([ktsaou](https://github.com/ktsaou))
- disable internal log [\#13678](https://github.com/netdata/netdata/pull/13678) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin v0.40.0 [\#13675](https://github.com/netdata/netdata/pull/13675) ([ilyam8](https://github.com/ilyam8))
- remove \_instance\_family label [\#13674](https://github.com/netdata/netdata/pull/13674) ([ilyam8](https://github.com/ilyam8))
- fix typo not deleting collected flag; force removing collected flag on child disconnect [\#13672](https://github.com/netdata/netdata/pull/13672) ([ktsaou](https://github.com/ktsaou))
- feat\(health\): add Postgres alarms [\#13671](https://github.com/netdata/netdata/pull/13671) ([ilyam8](https://github.com/ilyam8))
- add proxysql dashboard info [\#13669](https://github.com/netdata/netdata/pull/13669) ([ilyam8](https://github.com/ilyam8))
- Additional sqlite statistics [\#13668](https://github.com/netdata/netdata/pull/13668) ([stelfrag](https://github.com/stelfrag))
- Advance the buffer properly to scan the journal file [\#13666](https://github.com/netdata/netdata/pull/13666) ([stelfrag](https://github.com/stelfrag))
- Add sqlite page cache hit and miss statistics [\#13665](https://github.com/netdata/netdata/pull/13665) ([stelfrag](https://github.com/stelfrag))
- bump go.d.plugin to v0.39.0 [\#13662](https://github.com/netdata/netdata/pull/13662) ([ilyam8](https://github.com/ilyam8))
- update Postgres dashboard info [\#13661](https://github.com/netdata/netdata/pull/13661) ([ilyam8](https://github.com/ilyam8))
- Use mmap if possible during startup for journal replay [\#13660](https://github.com/netdata/netdata/pull/13660) ([stelfrag](https://github.com/stelfrag))
- Remove anomaly detector [\#13657](https://github.com/netdata/netdata/pull/13657) ([vkalintiris](https://github.com/vkalintiris))
- Update dashboard to version v2.29.0. [\#13654](https://github.com/netdata/netdata/pull/13654) ([netdatabot](https://github.com/netdatabot))
- Fix container virtualization info [\#13653](https://github.com/netdata/netdata/pull/13653) ([vlvkobal](https://github.com/vlvkobal))
- Do not free AR dimensions from within ML. [\#13651](https://github.com/netdata/netdata/pull/13651) ([vkalintiris](https://github.com/vkalintiris))
- Remove Chart/Dim based communication [\#13650](https://github.com/netdata/netdata/pull/13650) ([underhood](https://github.com/underhood))
- Improve agent shutdown time [\#13649](https://github.com/netdata/netdata/pull/13649) ([stelfrag](https://github.com/stelfrag))
- add \_collect\_job label to python.d/\* charts [\#13648](https://github.com/netdata/netdata/pull/13648) ([ilyam8](https://github.com/ilyam8))
- RRD structures managed by dictionaries [\#13646](https://github.com/netdata/netdata/pull/13646) ([ktsaou](https://github.com/ktsaou))
- fix rrdcontexts left in the post-processing queue from the garbage collector [\#13645](https://github.com/netdata/netdata/pull/13645) ([ktsaou](https://github.com/ktsaou))
- apps.plugin: Re-add `chrome` to the `webbrowser` group. [\#13642](https://github.com/netdata/netdata/pull/13642) ([Ferroin](https://github.com/Ferroin))
- Fix a memory leak on archived host creation [\#13641](https://github.com/netdata/netdata/pull/13641) ([stelfrag](https://github.com/stelfrag))
- fix compile issues [\#13640](https://github.com/netdata/netdata/pull/13640) ([ktsaou](https://github.com/ktsaou))
- Obsolete RRDSET state [\#13635](https://github.com/netdata/netdata/pull/13635) ([ktsaou](https://github.com/ktsaou))
- Updated tc.plugin \(linux bandwidth QoS\) [\#13634](https://github.com/netdata/netdata/pull/13634) ([ktsaou](https://github.com/ktsaou))
- Fix worker utilization cleanup [\#13633](https://github.com/netdata/netdata/pull/13633) ([stelfrag](https://github.com/stelfrag))
- remove forgotten avl structure from rrdcalc [\#13632](https://github.com/netdata/netdata/pull/13632) ([ktsaou](https://github.com/ktsaou))
- Deaggregate the `gui` and `email` app groupx and improve GUI coverage. [\#13631](https://github.com/netdata/netdata/pull/13631) ([Ferroin](https://github.com/Ferroin))
- Faster rrdcontext [\#13629](https://github.com/netdata/netdata/pull/13629) ([ktsaou](https://github.com/ktsaou))
- Update uninstaller documentation. [\#13627](https://github.com/netdata/netdata/pull/13627) ([Ferroin](https://github.com/Ferroin))
- eBPF different improvements [\#13624](https://github.com/netdata/netdata/pull/13624) ([thiagoftsm](https://github.com/thiagoftsm))
- adjust systemdunits alarms [\#13623](https://github.com/netdata/netdata/pull/13623) ([ilyam8](https://github.com/ilyam8))
- fix apps plugin users charts descriptipon [\#13621](https://github.com/netdata/netdata/pull/13621) ([ilyam8](https://github.com/ilyam8))
- add Postgres total connection utilization alarm [\#13620](https://github.com/netdata/netdata/pull/13620) ([ilyam8](https://github.com/ilyam8))
- update Postgres "connections" dashboard info [\#13619](https://github.com/netdata/netdata/pull/13619) ([ilyam8](https://github.com/ilyam8))
- Assorted updates for apps\_groups.conf. [\#13618](https://github.com/netdata/netdata/pull/13618) ([Ferroin](https://github.com/Ferroin))
- add spiceproxy to proxmox group [\#13615](https://github.com/netdata/netdata/pull/13615) ([ilyam8](https://github.com/ilyam8))
- Improve coverage of Linux kernel threads in apps\_groups.conf [\#13612](https://github.com/netdata/netdata/pull/13612) ([Ferroin](https://github.com/Ferroin))
- Clean chart hash map  [\#13611](https://github.com/netdata/netdata/pull/13611) ([stelfrag](https://github.com/stelfrag))
- Update dashboard to version v2.28.8. [\#13609](https://github.com/netdata/netdata/pull/13609) ([netdatabot](https://github.com/netdatabot))
- Don't try to load db rows when chart\_id or dim\_id is null [\#13608](https://github.com/netdata/netdata/pull/13608) ([MrZammler](https://github.com/MrZammler))
- Add info text for wal and update for checkpoints [\#13607](https://github.com/netdata/netdata/pull/13607) ([shyamvalsan](https://github.com/shyamvalsan))
- bump go.d.plugin to v0.38.0 [\#13603](https://github.com/netdata/netdata/pull/13603) ([ilyam8](https://github.com/ilyam8))
- Use prepared statements for context related queries [\#13602](https://github.com/netdata/netdata/pull/13602) ([stelfrag](https://github.com/stelfrag))
- fix\(cgroups.plugin\): fix chart id length check [\#13601](https://github.com/netdata/netdata/pull/13601) ([ilyam8](https://github.com/ilyam8))
- Temporary fix for command injection vulnerability in GHA workflow. [\#13600](https://github.com/netdata/netdata/pull/13600) ([Ferroin](https://github.com/Ferroin))
- update logind dashboard info [\#13597](https://github.com/netdata/netdata/pull/13597) ([ilyam8](https://github.com/ilyam8))
- Add link to the performance optimization guide [\#13595](https://github.com/netdata/netdata/pull/13595) ([cakrit](https://github.com/cakrit))
- sqlite3 global statistics [\#13594](https://github.com/netdata/netdata/pull/13594) ([ktsaou](https://github.com/ktsaou))
- feat\(python.d/nvidia\_smi\): collect power state [\#13580](https://github.com/netdata/netdata/pull/13580) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d/nvidia\_smi\): repsect update\_every for polling [\#13579](https://github.com/netdata/netdata/pull/13579) ([ilyam8](https://github.com/ilyam8))
- prevent crash on rrdcontext apis when rrdcontexts is not initialized [\#13578](https://github.com/netdata/netdata/pull/13578) ([ktsaou](https://github.com/ktsaou))
- CMake improvements part 1 [\#13575](https://github.com/netdata/netdata/pull/13575) ([underhood](https://github.com/underhood))
- bump go.d.plugin to v0.37.2 [\#13574](https://github.com/netdata/netdata/pull/13574) ([ilyam8](https://github.com/ilyam8))
- Updating info for postgreqsql metrics [\#13573](https://github.com/netdata/netdata/pull/13573) ([shyamvalsan](https://github.com/shyamvalsan))
- add `apt` to `apps_groups.conf` [\#13571](https://github.com/netdata/netdata/pull/13571) ([andrewm4894](https://github.com/andrewm4894))
- Deduplicate all netdata strings [\#13570](https://github.com/netdata/netdata/pull/13570) ([ktsaou](https://github.com/ktsaou))
- eBPF cmake missing include dir [\#13568](https://github.com/netdata/netdata/pull/13568) ([underhood](https://github.com/underhood))
- chore: removing logging that a chart collection in the same interpolation point [\#13567](https://github.com/netdata/netdata/pull/13567) ([ilyam8](https://github.com/ilyam8))
- add more monitoring tools to `apps_groups.conf` [\#13566](https://github.com/netdata/netdata/pull/13566) ([andrewm4894](https://github.com/andrewm4894))
- fix\(health.d/mysql\): adjust `mysql_galera_cluster_size_max_2m` lookup to make time in warn/crit predictable [\#13563](https://github.com/netdata/netdata/pull/13563) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.28.6. [\#13562](https://github.com/netdata/netdata/pull/13562) ([netdatabot](https://github.com/netdatabot))
- Prefer context attributes from non archived charts [\#13559](https://github.com/netdata/netdata/pull/13559) ([MrZammler](https://github.com/MrZammler))
- bump go.d.plugin to v0.37.1 [\#13555](https://github.com/netdata/netdata/pull/13555) ([ilyam8](https://github.com/ilyam8))
- Fix coverity 380387 [\#13551](https://github.com/netdata/netdata/pull/13551) ([MrZammler](https://github.com/MrZammler))
- add docker dashboard info [\#13547](https://github.com/netdata/netdata/pull/13547) ([ilyam8](https://github.com/ilyam8))

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
