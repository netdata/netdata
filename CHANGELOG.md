# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.31.0...HEAD)

**Merged pull requests:**

- fix various python modules charts contexts [\#11310](https://github.com/netdata/netdata/pull/11310) ([ilyam8](https://github.com/ilyam8))
- \[docs\] fix prometheus node cpu alert rule [\#11309](https://github.com/netdata/netdata/pull/11309) ([ilyam8](https://github.com/ilyam8))
- health: remove pythond modules specific last\_collected alarms [\#11307](https://github.com/netdata/netdata/pull/11307) ([ilyam8](https://github.com/ilyam8))
- fix `mdstat` current operation charts context/title [\#11289](https://github.com/netdata/netdata/pull/11289) ([ilyam8](https://github.com/ilyam8))
- Remove access check for install-type file [\#11288](https://github.com/netdata/netdata/pull/11288) ([MrZammler](https://github.com/MrZammler))
- eBPF plugin remove parallel plugins [\#11287](https://github.com/netdata/netdata/pull/11287) ([thiagoftsm](https://github.com/thiagoftsm))
- suport TLS SNI in ACLK-NG [\#11285](https://github.com/netdata/netdata/pull/11285) ([underhood](https://github.com/underhood))
- fixes confusing error in ACLK Legacy [\#11278](https://github.com/netdata/netdata/pull/11278) ([underhood](https://github.com/underhood))
- Ebpf disk latency [\#11276](https://github.com/netdata/netdata/pull/11276) ([thiagoftsm](https://github.com/thiagoftsm))
- makes ACLK-NG the default if available [\#11272](https://github.com/netdata/netdata/pull/11272) ([underhood](https://github.com/underhood))
- Added new postgres charts and updated standby charts to include slot\_… [\#11267](https://github.com/netdata/netdata/pull/11267) ([mjtice](https://github.com/mjtice))
- Explicitly update libarchive on CentOS 8 when installing dependencies. [\#11264](https://github.com/netdata/netdata/pull/11264) ([Ferroin](https://github.com/Ferroin))
- update old log to new one [\#11263](https://github.com/netdata/netdata/pull/11263) ([OdysLam](https://github.com/OdysLam))
- fix kickstart-static64.sh install script fail when trying to access `.install-type` before it is created [\#11262](https://github.com/netdata/netdata/pull/11262) ([ilyam8](https://github.com/ilyam8))
- Add openSUSE 15.3 package builds. [\#11259](https://github.com/netdata/netdata/pull/11259) ([Ferroin](https://github.com/Ferroin))
- slabinfo: Handle slabs added after discovery [\#11257](https://github.com/netdata/netdata/pull/11257) ([Saruspete](https://github.com/Saruspete))
- Ebpf apps memory usage [\#11256](https://github.com/netdata/netdata/pull/11256) ([thiagoftsm](https://github.com/thiagoftsm))
- eBPF keep values from `ebpf.d.conf` [\#11253](https://github.com/netdata/netdata/pull/11253) ([thiagoftsm](https://github.com/thiagoftsm))
- Add more nics to FreeBSD plugin [\#11251](https://github.com/netdata/netdata/pull/11251) ([diizzyy](https://github.com/diizzyy))
- Fix libjudy installation on CentOS 8. [\#11248](https://github.com/netdata/netdata/pull/11248) ([Ferroin](https://github.com/Ferroin))
- Send correct aclk implementation used by agent to posthog. [\#11247](https://github.com/netdata/netdata/pull/11247) ([MrZammler](https://github.com/MrZammler))
- Fixes error on --disable-cloud [\#11244](https://github.com/netdata/netdata/pull/11244) ([underhood](https://github.com/underhood))
- Swap class and type attributes in stock alarm configurations [\#11240](https://github.com/netdata/netdata/pull/11240) ([MrZammler](https://github.com/MrZammler))
- packaging: update go.d.plugin version to v0.29.0 [\#11239](https://github.com/netdata/netdata/pull/11239) ([ilyam8](https://github.com/ilyam8))
- Adds xfs filesystem monitoring to eBPF [\#11238](https://github.com/netdata/netdata/pull/11238) ([thiagoftsm](https://github.com/thiagoftsm))
- Extra posthog attributes [\#11237](https://github.com/netdata/netdata/pull/11237) ([MrZammler](https://github.com/MrZammler))
- health: update cockroachdb alarms [\#11235](https://github.com/netdata/netdata/pull/11235) ([ilyam8](https://github.com/ilyam8))
- Ebpf arrays [\#11230](https://github.com/netdata/netdata/pull/11230) ([thiagoftsm](https://github.com/thiagoftsm))
- Add links to data privacy page [\#11226](https://github.com/netdata/netdata/pull/11226) ([joelhans](https://github.com/joelhans))
- Allows ACLK NG and Legacy to coexist [\#11225](https://github.com/netdata/netdata/pull/11225) ([underhood](https://github.com/underhood))
- eBPF ext4 \(new thread for collector\) [\#11224](https://github.com/netdata/netdata/pull/11224) ([thiagoftsm](https://github.com/thiagoftsm))
- Add vkalintiris as a code owner for more components [\#11221](https://github.com/netdata/netdata/pull/11221) ([vkalintiris](https://github.com/vkalintiris))
- Decentralized [\#11220](https://github.com/netdata/netdata/pull/11220) ([OdysLam](https://github.com/OdysLam))
- New email notification template [\#11219](https://github.com/netdata/netdata/pull/11219) ([MrZammler](https://github.com/MrZammler))
- python.d: merge user/stock plugin configuration files [\#11217](https://github.com/netdata/netdata/pull/11217) ([ilyam8](https://github.com/ilyam8))
- Only report the exit code when anonymous statistics script fails [\#11215](https://github.com/netdata/netdata/pull/11215) ([MrZammler](https://github.com/MrZammler))
- Reduce memory needed per dimension [\#11212](https://github.com/netdata/netdata/pull/11212) ([stelfrag](https://github.com/stelfrag))
- Ignore dbengine journal files that can not be read [\#11210](https://github.com/netdata/netdata/pull/11210) ([stelfrag](https://github.com/stelfrag))
- Use memory mode RAM if memory mode dbengine is specified but not available [\#11207](https://github.com/netdata/netdata/pull/11207) ([stelfrag](https://github.com/stelfrag))
- Add Microsoft Teams to supported notification endpoints [\#11205](https://github.com/netdata/netdata/pull/11205) ([zanechua](https://github.com/zanechua))
- health: fix alarm-line-charts matching [\#11204](https://github.com/netdata/netdata/pull/11204) ([ilyam8](https://github.com/ilyam8))
- fix ebpf.plugin segfault when ebpf\_load\_program return null pointer [\#11203](https://github.com/netdata/netdata/pull/11203) ([wangpei-nice](https://github.com/wangpei-nice))
- fix `install\_type` detection during update [\#11199](https://github.com/netdata/netdata/pull/11199) ([ilyam8](https://github.com/ilyam8))
- claiming: exit 0 when daemon not running and the claim was successful [\#11195](https://github.com/netdata/netdata/pull/11195) ([ilyam8](https://github.com/ilyam8))
- Load class, component and type from health log when sufficient fields are detected. [\#11193](https://github.com/netdata/netdata/pull/11193) ([MrZammler](https://github.com/MrZammler))
- Check return status of execution of anonymous statistics script [\#11188](https://github.com/netdata/netdata/pull/11188) ([MrZammler](https://github.com/MrZammler))
- VFS new thread [\#11187](https://github.com/netdata/netdata/pull/11187) ([thiagoftsm](https://github.com/thiagoftsm))
- add link to example conf [\#11182](https://github.com/netdata/netdata/pull/11182) ([gotjoshua](https://github.com/gotjoshua))
- rename default from job 'local' to 'anomalies' [\#11178](https://github.com/netdata/netdata/pull/11178) ([andrewm4894](https://github.com/andrewm4894))
- health: add system clock synchronization state alarm [\#11177](https://github.com/netdata/netdata/pull/11177) ([ilyam8](https://github.com/ilyam8))
- Fix broken links [\#11175](https://github.com/netdata/netdata/pull/11175) ([joelhans](https://github.com/joelhans))
- mqtt\_websockets FreeBSD fix [\#11172](https://github.com/netdata/netdata/pull/11172) ([underhood](https://github.com/underhood))
- Fix typo in aclk.c [\#11170](https://github.com/netdata/netdata/pull/11170) ([eltociear](https://github.com/eltociear))
- Adding more postgres metrics [\#11169](https://github.com/netdata/netdata/pull/11169) ([filip-plata](https://github.com/filip-plata))
- health: add python.d/go.d jobs last\_collected\_secs alarms [\#11168](https://github.com/netdata/netdata/pull/11168) ([ilyam8](https://github.com/ilyam8))
- Update news in main README for latest release [\#11165](https://github.com/netdata/netdata/pull/11165) ([joelhans](https://github.com/joelhans))
- Query the size of the hw.intrnames mib instead of using of a fixed va… [\#11159](https://github.com/netdata/netdata/pull/11159) ([MikaelUrankar](https://github.com/MikaelUrankar))
- Store info about the installation type for later retrieval. [\#11157](https://github.com/netdata/netdata/pull/11157) ([Ferroin](https://github.com/Ferroin))
- health: make stocks alarms less sensitive \(2\) [\#11153](https://github.com/netdata/netdata/pull/11153) ([ilyam8](https://github.com/ilyam8))
- Move parser from children to main thread [\#11152](https://github.com/netdata/netdata/pull/11152) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove deprecated options. [\#11149](https://github.com/netdata/netdata/pull/11149) ([vkalintiris](https://github.com/vkalintiris))
- Fixes mqtt\_websockets on MacOS [\#11145](https://github.com/netdata/netdata/pull/11145) ([underhood](https://github.com/underhood))
- Removed Fedora 32 from CI. [\#11143](https://github.com/netdata/netdata/pull/11143) ([Ferroin](https://github.com/Ferroin))
- Remove an unnecessary check for cgroup v1 [\#11137](https://github.com/netdata/netdata/pull/11137) ([vlvkobal](https://github.com/vlvkobal))
- Compile/Link with absolute paths for bundled/vendored deps. [\#11129](https://github.com/netdata/netdata/pull/11129) ([vkalintiris](https://github.com/vkalintiris))
- Remove unecessary relative paths when including headers. [\#11124](https://github.com/netdata/netdata/pull/11124) ([vkalintiris](https://github.com/vkalintiris))
- Move mdstat charts near to Disks [\#11119](https://github.com/netdata/netdata/pull/11119) ([thiagoftsm](https://github.com/thiagoftsm))
- Ebpf swap [\#11090](https://github.com/netdata/netdata/pull/11090) ([thiagoftsm](https://github.com/thiagoftsm))
- Provide UTC offset in seconds and edit health config command [\#11051](https://github.com/netdata/netdata/pull/11051) ([MrZammler](https://github.com/MrZammler))

## [v1.31.0](https://github.com/netdata/netdata/tree/v1.31.0) (2021-05-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.1...v1.31.0)

**Merged pull requests:**

- Fix broken link in dimensions/contexts/families doc [\#11148](https://github.com/netdata/netdata/pull/11148) ([joelhans](https://github.com/joelhans))
- Add info on other memory modes to performance.md [\#11144](https://github.com/netdata/netdata/pull/11144) ([cakrit](https://github.com/cakrit))
- Use size\_t instead of int for vfs\_bufspace\_count in FreeBSD plugin [\#11142](https://github.com/netdata/netdata/pull/11142) ([diizzyy](https://github.com/diizzyy))
- Bundle the react dashboard code into the agent repo directly. [\#11139](https://github.com/netdata/netdata/pull/11139) ([Ferroin](https://github.com/Ferroin))
- Reduce the number of ACLK chart updates during chart obsoletion [\#11133](https://github.com/netdata/netdata/pull/11133) ([stelfrag](https://github.com/stelfrag))
- Update k6.md [\#11127](https://github.com/netdata/netdata/pull/11127) ([OdysLam](https://github.com/OdysLam))
- Fix broken link in doc [\#11122](https://github.com/netdata/netdata/pull/11122) ([forest0](https://github.com/forest0))
- analytics: reduce alarms notifications dump logging [\#11116](https://github.com/netdata/netdata/pull/11116) ([ilyam8](https://github.com/ilyam8))
- Check configuration for CUSTOM and MSTEAM [\#11113](https://github.com/netdata/netdata/pull/11113) ([MrZammler](https://github.com/MrZammler))
- Fix broken links in various docs [\#11109](https://github.com/netdata/netdata/pull/11109) ([joelhans](https://github.com/joelhans))
- minor - fixes typo in ACLK-NG log [\#11107](https://github.com/netdata/netdata/pull/11107) ([underhood](https://github.com/underhood))
- Update mqtt\_websockets [\#11105](https://github.com/netdata/netdata/pull/11105) ([underhood](https://github.com/underhood))
- packaging: update go.d.plugin version to v0.28.2 [\#11104](https://github.com/netdata/netdata/pull/11104) ([ilyam8](https://github.com/ilyam8))
- aclk/legacy: change aclk statistics charts units from kB/s to KiB/s [\#11103](https://github.com/netdata/netdata/pull/11103) ([ilyam8](https://github.com/ilyam8))
- Check the version of the default cgroup mountpoint [\#11102](https://github.com/netdata/netdata/pull/11102) ([vlvkobal](https://github.com/vlvkobal))
- Don't repeat the cgroup discovery cleanup info message [\#11101](https://github.com/netdata/netdata/pull/11101) ([vlvkobal](https://github.com/vlvkobal))
- Add host\_cloud\_enabled attribute to analytics [\#11100](https://github.com/netdata/netdata/pull/11100) ([MrZammler](https://github.com/MrZammler))
- Improve dashboard documentation \(part 3\) [\#11099](https://github.com/netdata/netdata/pull/11099) ([joelhans](https://github.com/joelhans))
- cgroups: fix network interfaces detection when using `virsh` [\#11096](https://github.com/netdata/netdata/pull/11096) ([ilyam8](https://github.com/ilyam8))
- Reduce send statistics logging [\#11091](https://github.com/netdata/netdata/pull/11091) ([MrZammler](https://github.com/MrZammler))
- fix SSL random failures when using multithreaded web server with OpenSSL \< 1.1.0 [\#11089](https://github.com/netdata/netdata/pull/11089) ([thiagoftsm](https://github.com/thiagoftsm))
- health: clarify which health configuration entities are required / optional [\#11086](https://github.com/netdata/netdata/pull/11086) ([ilyam8](https://github.com/ilyam8))
- build mqtt\_websockets with netdata autotools [\#11083](https://github.com/netdata/netdata/pull/11083) ([underhood](https://github.com/underhood))
- Fixed a single typo in documentation [\#11082](https://github.com/netdata/netdata/pull/11082) ([yavin87](https://github.com/yavin87))
- netdata-installer.sh: Enable IPv6 support in libwebsockets [\#11080](https://github.com/netdata/netdata/pull/11080) ([pjakuszew](https://github.com/pjakuszew))
- Add an event when an incomplete agent shutdown is detected [\#11078](https://github.com/netdata/netdata/pull/11078) ([stelfrag](https://github.com/stelfrag))
- Remove dash-example, place in community repo [\#11077](https://github.com/netdata/netdata/pull/11077) ([tnyeanderson](https://github.com/tnyeanderson))
- Change eBPF chart type [\#11074](https://github.com/netdata/netdata/pull/11074) ([thiagoftsm](https://github.com/thiagoftsm))
- Add a module for ZFS pool state [\#11071](https://github.com/netdata/netdata/pull/11071) ([vlvkobal](https://github.com/vlvkobal))
- Improve dashboard documentation \(part 2\) [\#11065](https://github.com/netdata/netdata/pull/11065) ([joelhans](https://github.com/joelhans))
- Fix coverity issue \(CID 370510\) [\#11060](https://github.com/netdata/netdata/pull/11060) ([stelfrag](https://github.com/stelfrag))
- Add functionality to store node\_id for a host [\#11059](https://github.com/netdata/netdata/pull/11059) ([stelfrag](https://github.com/stelfrag))
- Add `charts` to templates [\#11054](https://github.com/netdata/netdata/pull/11054) ([thiagoftsm](https://github.com/thiagoftsm))
- Add documentation for claiming during kickstart installation [\#11052](https://github.com/netdata/netdata/pull/11052) ([joelhans](https://github.com/joelhans))
- Remove dots in cgroup ids [\#11050](https://github.com/netdata/netdata/pull/11050) ([vlvkobal](https://github.com/vlvkobal))
- health/vernemq: use `average` instead of  `sum` [\#11037](https://github.com/netdata/netdata/pull/11037) ([ilyam8](https://github.com/ilyam8))
- Fix storing an NULL claim id on a parent node [\#11036](https://github.com/netdata/netdata/pull/11036) ([stelfrag](https://github.com/stelfrag))
- Improve installation method for Alpine [\#11035](https://github.com/netdata/netdata/pull/11035) ([tiramiseb](https://github.com/tiramiseb))
- Load names [\#11034](https://github.com/netdata/netdata/pull/11034) ([thiagoftsm](https://github.com/thiagoftsm))
- Add Third-party collector: nextcloud plugin [\#11032](https://github.com/netdata/netdata/pull/11032) ([tknobi](https://github.com/tknobi))
- Create ebpf.d directory in PLUGINDIR for debian and rpm package\(netdata\#11017\) [\#11031](https://github.com/netdata/netdata/pull/11031) ([wangpei-nice](https://github.com/wangpei-nice))
- proc/mdstat: add raid level to the family [\#11024](https://github.com/netdata/netdata/pull/11024) ([ilyam8](https://github.com/ilyam8))
- bump to netdata-pandas==0.0.38 [\#11022](https://github.com/netdata/netdata/pull/11022) ([andrewm4894](https://github.com/andrewm4894))
- Provide more agent analytics to posthog [\#11020](https://github.com/netdata/netdata/pull/11020) ([MrZammler](https://github.com/MrZammler))
- Rename struct fields from class to classification. [\#11019](https://github.com/netdata/netdata/pull/11019) ([vkalintiris](https://github.com/vkalintiris))
- Improve dashboard documentation \(part 1\) [\#11015](https://github.com/netdata/netdata/pull/11015) ([joelhans](https://github.com/joelhans))
- Remove links to old install doc [\#11014](https://github.com/netdata/netdata/pull/11014) ([joelhans](https://github.com/joelhans))
- Revert "Provide more agent analytics to posthog" [\#11011](https://github.com/netdata/netdata/pull/11011) ([MrZammler](https://github.com/MrZammler))
- anonymous-statistics: add a timeout when using `curl` [\#11010](https://github.com/netdata/netdata/pull/11010) ([ilyam8](https://github.com/ilyam8))
- python.d: add plugin and module names to the runtime charts [\#11007](https://github.com/netdata/netdata/pull/11007) ([ilyam8](https://github.com/ilyam8))
- \[area/collectors\] Added support for libvirtd LXC containers to the `cgroup-name.sh` cgroup name normalization script [\#11006](https://github.com/netdata/netdata/pull/11006) ([endreszabo](https://github.com/endreszabo))
- Allow the remote write configuration have multiple destinations [\#11005](https://github.com/netdata/netdata/pull/11005) ([vlvkobal](https://github.com/vlvkobal))
- improvements to anomalies collector following dogfooding [\#11003](https://github.com/netdata/netdata/pull/11003) ([andrewm4894](https://github.com/andrewm4894))
- Backend chart filtering backward compatibility fix [\#11002](https://github.com/netdata/netdata/pull/11002) ([vlvkobal](https://github.com/vlvkobal))
- Add a chart with netdata uptime [\#10997](https://github.com/netdata/netdata/pull/10997) ([vlvkobal](https://github.com/vlvkobal))
- Improve get started/installation docs [\#10995](https://github.com/netdata/netdata/pull/10995) ([joelhans](https://github.com/joelhans))
- Persist claim ids in local database for parent and children [\#10993](https://github.com/netdata/netdata/pull/10993) ([stelfrag](https://github.com/stelfrag))
- ci: fix aws-kinesis builds [\#10992](https://github.com/netdata/netdata/pull/10992) ([ilyam8](https://github.com/ilyam8))
- Move global stats to a separate thread [\#10991](https://github.com/netdata/netdata/pull/10991) ([vlvkobal](https://github.com/vlvkobal))
- adds missing SPDX license info into ACLK-NG [\#10990](https://github.com/netdata/netdata/pull/10990) ([underhood](https://github.com/underhood))
- K6 quality of life updates [\#10985](https://github.com/netdata/netdata/pull/10985) ([OdysLam](https://github.com/OdysLam))
- Add sections for class, component and type. [\#10984](https://github.com/netdata/netdata/pull/10984) ([MrZammler](https://github.com/MrZammler))
- Update eBPF documentation [\#10982](https://github.com/netdata/netdata/pull/10982) ([thiagoftsm](https://github.com/thiagoftsm))
- remove vneg from ACLK-NG [\#10980](https://github.com/netdata/netdata/pull/10980) ([underhood](https://github.com/underhood))
- Remove outdated privacy policy and terms of use [\#10979](https://github.com/netdata/netdata/pull/10979) ([joelhans](https://github.com/joelhans))
- collectors/charts.d/opensips: fix detection of `opensipsctl` executable  [\#10978](https://github.com/netdata/netdata/pull/10978) ([ilyam8](https://github.com/ilyam8))
- Update fping version [\#10977](https://github.com/netdata/netdata/pull/10977) ([Habetdin](https://github.com/Habetdin))
- fix uil in statsd guide [\#10975](https://github.com/netdata/netdata/pull/10975) ([OdysLam](https://github.com/OdysLam))
- health: fix alarm line options syntax in the docs [\#10974](https://github.com/netdata/netdata/pull/10974) ([ilyam8](https://github.com/ilyam8))
- Upgrade OKay repository RPM for RHEL8 [\#10973](https://github.com/netdata/netdata/pull/10973) ([BastienBalaud](https://github.com/BastienBalaud))
- Remove condition that was creating gaps [\#10972](https://github.com/netdata/netdata/pull/10972) ([thiagoftsm](https://github.com/thiagoftsm))
- Add a metric for percpu memory [\#10964](https://github.com/netdata/netdata/pull/10964) ([vlvkobal](https://github.com/vlvkobal))
- Bring flexible adjust for eBPF hash tables [\#10962](https://github.com/netdata/netdata/pull/10962) ([thiagoftsm](https://github.com/thiagoftsm))
- Provide new attributes in health conf files [\#10961](https://github.com/netdata/netdata/pull/10961) ([MrZammler](https://github.com/MrZammler))
- Fix epbf crash when process exit [\#10957](https://github.com/netdata/netdata/pull/10957) ([thiagoftsm](https://github.com/thiagoftsm))
- Contributing revamp, take 2 [\#10956](https://github.com/netdata/netdata/pull/10956) ([OdysLam](https://github.com/OdysLam))
- health: add Inconsistent state to the mysql\_galera\_cluster\_state alarm [\#10945](https://github.com/netdata/netdata/pull/10945) ([ilyam8](https://github.com/ilyam8))
- Update cloud-providers.md [\#10942](https://github.com/netdata/netdata/pull/10942) ([Avre](https://github.com/Avre))
- ACLK new cloud architecture new TBEB [\#10941](https://github.com/netdata/netdata/pull/10941) ([underhood](https://github.com/underhood))
- Add new charts for extended disk metrics [\#10939](https://github.com/netdata/netdata/pull/10939) ([vlvkobal](https://github.com/vlvkobal))
- Adds --recursive to docu git clones [\#10932](https://github.com/netdata/netdata/pull/10932) ([underhood](https://github.com/underhood))
- Add lists of monitored metrics to the cgroups plugin documentation [\#10924](https://github.com/netdata/netdata/pull/10924) ([vlvkobal](https://github.com/vlvkobal))
- Spelling web gui [\#10922](https://github.com/netdata/netdata/pull/10922) ([jsoref](https://github.com/jsoref))
- Spelling web api server [\#10921](https://github.com/netdata/netdata/pull/10921) ([jsoref](https://github.com/jsoref))
- Spelling tests [\#10920](https://github.com/netdata/netdata/pull/10920) ([jsoref](https://github.com/jsoref))
- Spelling streaming [\#10919](https://github.com/netdata/netdata/pull/10919) ([jsoref](https://github.com/jsoref))
- spelling: bidirectional [\#10918](https://github.com/netdata/netdata/pull/10918) ([jsoref](https://github.com/jsoref))
- Spelling libnetdata [\#10917](https://github.com/netdata/netdata/pull/10917) ([jsoref](https://github.com/jsoref))
- Spelling health [\#10916](https://github.com/netdata/netdata/pull/10916) ([jsoref](https://github.com/jsoref))
- Spelling exporting [\#10915](https://github.com/netdata/netdata/pull/10915) ([jsoref](https://github.com/jsoref))
- Spelling database [\#10914](https://github.com/netdata/netdata/pull/10914) ([jsoref](https://github.com/jsoref))
- Spelling daemon [\#10913](https://github.com/netdata/netdata/pull/10913) ([jsoref](https://github.com/jsoref))
- Spelling collectors [\#10912](https://github.com/netdata/netdata/pull/10912) ([jsoref](https://github.com/jsoref))
- spelling: backend [\#10911](https://github.com/netdata/netdata/pull/10911) ([jsoref](https://github.com/jsoref))
- Spelling aclk [\#10910](https://github.com/netdata/netdata/pull/10910) ([jsoref](https://github.com/jsoref))
- Spelling build [\#10909](https://github.com/netdata/netdata/pull/10909) ([jsoref](https://github.com/jsoref))
- health: add synchronization.conf to the Makefile [\#10907](https://github.com/netdata/netdata/pull/10907) ([ilyam8](https://github.com/ilyam8))
- health: add systemdunits alarms [\#10906](https://github.com/netdata/netdata/pull/10906) ([ilyam8](https://github.com/ilyam8))
- web/gui: add systemdunits info to the dashboard\_info.js [\#10904](https://github.com/netdata/netdata/pull/10904) ([ilyam8](https://github.com/ilyam8))
- Add a plugin for the system clock synchronization state [\#10895](https://github.com/netdata/netdata/pull/10895) ([vlvkobal](https://github.com/vlvkobal))
- Provide more agent analytics to posthog [\#10887](https://github.com/netdata/netdata/pull/10887) ([MrZammler](https://github.com/MrZammler))
- Add a chart for out of memory kills [\#10880](https://github.com/netdata/netdata/pull/10880) ([vlvkobal](https://github.com/vlvkobal))
- Remove RewriteEngine for dedicated vHost [\#10873](https://github.com/netdata/netdata/pull/10873) ([Steve8291](https://github.com/Steve8291))
- python.d\(smartd\_log\): collect attribute 249 -- NAND Writes 1GiB [\#10872](https://github.com/netdata/netdata/pull/10872) ([RaitoBezarius](https://github.com/RaitoBezarius))
- Improvements to dash-example.html [\#10870](https://github.com/netdata/netdata/pull/10870) ([tnyeanderson](https://github.com/tnyeanderson))
- Replace references to Google Analytics with Posthog where relevant [\#10868](https://github.com/netdata/netdata/pull/10868) ([andrewm4894](https://github.com/andrewm4894))
- ACLK Passwd endpoint update [\#10859](https://github.com/netdata/netdata/pull/10859) ([underhood](https://github.com/underhood))
- Dashboard version 2.17.0 [\#10856](https://github.com/netdata/netdata/pull/10856) ([allelos](https://github.com/allelos))
- Ebpf directory cache [\#10855](https://github.com/netdata/netdata/pull/10855) ([thiagoftsm](https://github.com/thiagoftsm))
- prevents mqtt connection attempt on OTP failure [\#10839](https://github.com/netdata/netdata/pull/10839) ([underhood](https://github.com/underhood))
- implements ACLK env endpoint [\#10833](https://github.com/netdata/netdata/pull/10833) ([underhood](https://github.com/underhood))
- implements new https client for ACLK [\#10805](https://github.com/netdata/netdata/pull/10805) ([underhood](https://github.com/underhood))
- Support mulitple jobs in make\(1\) when building LWS. [\#10799](https://github.com/netdata/netdata/pull/10799) ([vkalintiris](https://github.com/vkalintiris))
- Overhaul streaming documentation [\#10709](https://github.com/netdata/netdata/pull/10709) ([joelhans](https://github.com/joelhans))

## [v1.30.1](https://github.com/netdata/netdata/tree/v1.30.1) (2021-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.0...v1.30.1)

**Merged pull requests:**

- Don’t use glob expansion in argument to `cd` in updater. [\#10936](https://github.com/netdata/netdata/pull/10936) ([Ferroin](https://github.com/Ferroin))
- Fix memory corruption issue when executing context queries in RAM/SAVE memory mode [\#10933](https://github.com/netdata/netdata/pull/10933) ([stelfrag](https://github.com/stelfrag))
- Update CODEOWNERS [\#10928](https://github.com/netdata/netdata/pull/10928) ([knatsakis](https://github.com/knatsakis))
- Update news and GIF in README, fix typo [\#10900](https://github.com/netdata/netdata/pull/10900) ([joelhans](https://github.com/joelhans))
- Update README.md [\#10898](https://github.com/netdata/netdata/pull/10898) ([slimanio](https://github.com/slimanio))
- Fixed bundling of ACLK-NG components in dist tarballs. [\#10894](https://github.com/netdata/netdata/pull/10894) ([Ferroin](https://github.com/Ferroin))
- Add a CRASH event when the agent fails to properly shutdown [\#10893](https://github.com/netdata/netdata/pull/10893) ([stelfrag](https://github.com/stelfrag))
- Bumped version of OpenSSL bundled in static builds to 1.1.1k. [\#10884](https://github.com/netdata/netdata/pull/10884) ([Ferroin](https://github.com/Ferroin))
- Fix incorrect health log entries [\#10822](https://github.com/netdata/netdata/pull/10822) ([stelfrag](https://github.com/stelfrag))

## [v1.30.0](https://github.com/netdata/netdata/tree/v1.30.0) (2021-03-31)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.3...v1.30.0)

**Merged pull requests:**

- Properly handle different netcat command names in binary package test code. [\#10883](https://github.com/netdata/netdata/pull/10883) ([Ferroin](https://github.com/Ferroin))
- Add carrier and mtu charts for network interfaces [\#10866](https://github.com/netdata/netdata/pull/10866) ([vlvkobal](https://github.com/vlvkobal))
- Fix typo in main.h [\#10858](https://github.com/netdata/netdata/pull/10858) ([eltociear](https://github.com/eltociear))
- health: improve alarms infos [\#10853](https://github.com/netdata/netdata/pull/10853) ([ilyam8](https://github.com/ilyam8))
- minor - add info about --aclk-ng into netdata-installer [\#10852](https://github.com/netdata/netdata/pull/10852) ([underhood](https://github.com/underhood))
- mqtt-c coverity fix [\#10851](https://github.com/netdata/netdata/pull/10851) ([underhood](https://github.com/underhood))
- web/gui: make network state map sytanx consistent in the dashboard info [\#10849](https://github.com/netdata/netdata/pull/10849) ([ilyam8](https://github.com/ilyam8))
- fix\_repeat: Update repeat\_every and avoid unecessary test [\#10846](https://github.com/netdata/netdata/pull/10846) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix agent crash when executing data query with context and non-existing chart\_label\_key [\#10844](https://github.com/netdata/netdata/pull/10844) ([stelfrag](https://github.com/stelfrag))
- Check device names in diskstats plugin [\#10843](https://github.com/netdata/netdata/pull/10843) ([vlvkobal](https://github.com/vlvkobal))
- Fix memory leak when archived data is requested [\#10837](https://github.com/netdata/netdata/pull/10837) ([stelfrag](https://github.com/stelfrag))
- add Installation method to the bug template [\#10836](https://github.com/netdata/netdata/pull/10836) ([ilyam8](https://github.com/ilyam8))
- Add lock check to avoid shutdown when compiled with internal and locking checks [\#10835](https://github.com/netdata/netdata/pull/10835) ([stelfrag](https://github.com/stelfrag))
- health: apply megacli alarms for all adapters/physical disks [\#10834](https://github.com/netdata/netdata/pull/10834) ([ilyam8](https://github.com/ilyam8))
- Fix broken link in StatsD guide [\#10831](https://github.com/netdata/netdata/pull/10831) ([joelhans](https://github.com/joelhans))
- health: add collector prefix to the external collectors alarms/templates [\#10830](https://github.com/netdata/netdata/pull/10830) ([ilyam8](https://github.com/ilyam8))
- health: remove exporting\_metrics\_lost template [\#10829](https://github.com/netdata/netdata/pull/10829) ([ilyam8](https://github.com/ilyam8))
- Fix name of PackageCLoud API token secret in workflows. [\#10828](https://github.com/netdata/netdata/pull/10828) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.28.1 [\#10826](https://github.com/netdata/netdata/pull/10826) ([ilyam8](https://github.com/ilyam8))
- alarm\(irc\): add support to change IRC\_PORT [\#10824](https://github.com/netdata/netdata/pull/10824) ([RaitoBezarius](https://github.com/RaitoBezarius))
- Update syntax for Caddy v2 [\#10823](https://github.com/netdata/netdata/pull/10823) ([salazarp](https://github.com/salazarp))
- health: apply adapter\_raid alarms for every logical/physical device [\#10820](https://github.com/netdata/netdata/pull/10820) ([ilyam8](https://github.com/ilyam8))
- Fix handling of nightly and release packages in GHA workflows. [\#10819](https://github.com/netdata/netdata/pull/10819) ([Ferroin](https://github.com/Ferroin))
- health: log an error if any when send email notification [\#10818](https://github.com/netdata/netdata/pull/10818) ([ilyam8](https://github.com/ilyam8))
- Ebpf extend sync [\#10814](https://github.com/netdata/netdata/pull/10814) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix coverity issue \(CID 367566\) [\#10813](https://github.com/netdata/netdata/pull/10813) ([stelfrag](https://github.com/stelfrag))
- fix claiming via env vars in docker container [\#10811](https://github.com/netdata/netdata/pull/10811) ([ilyam8](https://github.com/ilyam8))
- Fix eBPF compilation [\#10810](https://github.com/netdata/netdata/pull/10810) ([thiagoftsm](https://github.com/thiagoftsm))
- update bug report template [\#10807](https://github.com/netdata/netdata/pull/10807) ([underhood](https://github.com/underhood))
- health: exclude cgroups net ifaces from packets dropped alarms [\#10806](https://github.com/netdata/netdata/pull/10806) ([ilyam8](https://github.com/ilyam8))
- Don't show alarms for charts without data [\#10804](https://github.com/netdata/netdata/pull/10804) ([vlvkobal](https://github.com/vlvkobal))
- claiming: increase curl connect-timeout and decrease number of claim attempts [\#10800](https://github.com/netdata/netdata/pull/10800) ([ilyam8](https://github.com/ilyam8))
- Added Ubuntu 21.04 and Fedora 34 to our CI checks and binary package builds. [\#10791](https://github.com/netdata/netdata/pull/10791) ([Ferroin](https://github.com/Ferroin))
- health: remove ram\_in\_swap alarm [\#10789](https://github.com/netdata/netdata/pull/10789) ([ilyam8](https://github.com/ilyam8))
- Add a new parameter 'chart' to the /api/v1/alarm\_log. [\#10788](https://github.com/netdata/netdata/pull/10788) ([MrZammler](https://github.com/MrZammler))
- Add check for children connecting to a parent agent with unsupported memory mode [\#10787](https://github.com/netdata/netdata/pull/10787) ([stelfrag](https://github.com/stelfrag))
- health: use separate packets\_dropped\_ratio alarms for wifi network interfaces [\#10785](https://github.com/netdata/netdata/pull/10785) ([ilyam8](https://github.com/ilyam8))
- ACLK separate https client [\#10784](https://github.com/netdata/netdata/pull/10784) ([underhood](https://github.com/underhood))
- health: add `wmi\_` prefix to the wmi collector network alarms [\#10782](https://github.com/netdata/netdata/pull/10782) ([ilyam8](https://github.com/ilyam8))
- web/gui: add max value to the nvidia\_smi.fan\_speed gauge [\#10780](https://github.com/netdata/netdata/pull/10780) ([ilyam8](https://github.com/ilyam8))
- health/: fix various alarms critical and warning thresholds hysteresis [\#10779](https://github.com/netdata/netdata/pull/10779) ([ilyam8](https://github.com/ilyam8))
- Adds \_aclk\_impl label [\#10778](https://github.com/netdata/netdata/pull/10778) ([underhood](https://github.com/underhood))
- adding a default job with some params and example of additional job. [\#10777](https://github.com/netdata/netdata/pull/10777) ([andrewm4894](https://github.com/andrewm4894))
- Fix typo in dashboard\_info.js [\#10775](https://github.com/netdata/netdata/pull/10775) ([eltociear](https://github.com/eltociear))
- Fixed Travis config issues related to new packaging workflows. [\#10774](https://github.com/netdata/netdata/pull/10774) ([Ferroin](https://github.com/Ferroin))
- add a dump\_methods parameter to alarm-notify.sh.in [\#10772](https://github.com/netdata/netdata/pull/10772) ([MrZammler](https://github.com/MrZammler))
- Add data query support for archived charts [\#10771](https://github.com/netdata/netdata/pull/10771) ([stelfrag](https://github.com/stelfrag))
- health: make vernemq alarms less sensitive [\#10770](https://github.com/netdata/netdata/pull/10770) ([ilyam8](https://github.com/ilyam8))
- Fixed handling of perf.plugin capabilities. [\#10766](https://github.com/netdata/netdata/pull/10766) ([Ferroin](https://github.com/Ferroin))
- dashboard@v2.13.28 [\#10761](https://github.com/netdata/netdata/pull/10761) ([jacekkolasa](https://github.com/jacekkolasa))
- collectors/cgroups: fix cpuset.cpus count [\#10757](https://github.com/netdata/netdata/pull/10757) ([ilyam8](https://github.com/ilyam8))
- eBPF plugin \(fixes 10727\) [\#10756](https://github.com/netdata/netdata/pull/10756) ([thiagoftsm](https://github.com/thiagoftsm))
- web/gui: add supervisord to the dashboard\_info.js [\#10754](https://github.com/netdata/netdata/pull/10754) ([ilyam8](https://github.com/ilyam8))
- Add state map to duplex and operstate charts [\#10752](https://github.com/netdata/netdata/pull/10752) ([vlvkobal](https://github.com/vlvkobal))
- comment out memory mode mention in example [\#10751](https://github.com/netdata/netdata/pull/10751) ([OdysLam](https://github.com/OdysLam))
- collectors/apps.plugin: Add wireguard to vpn [\#10743](https://github.com/netdata/netdata/pull/10743) ([liepumartins](https://github.com/liepumartins))
- Enable metadata persistence in all memory modes [\#10742](https://github.com/netdata/netdata/pull/10742) ([stelfrag](https://github.com/stelfrag))
- Move network interface speed, duplex, and operstate variables to charts [\#10740](https://github.com/netdata/netdata/pull/10740) ([vlvkobal](https://github.com/vlvkobal))
- Use of out-of-line struct definitions. [\#10739](https://github.com/netdata/netdata/pull/10739) ([vkalintiris](https://github.com/vkalintiris))
- Use a parameter name that is not a reserved keyword in C++ [\#10738](https://github.com/netdata/netdata/pull/10738) ([vkalintiris](https://github.com/vkalintiris))
- Skip C++ incompatible header in main libnetdata header [\#10737](https://github.com/netdata/netdata/pull/10737) ([vkalintiris](https://github.com/vkalintiris))
- Rename struct avl to avl\_element and the typedef to avl\_t [\#10735](https://github.com/netdata/netdata/pull/10735) ([vkalintiris](https://github.com/vkalintiris))
- Fix claim behind squid proxy [\#10734](https://github.com/netdata/netdata/pull/10734) ([underhood](https://github.com/underhood))
- add k6.conf [\#10733](https://github.com/netdata/netdata/pull/10733) ([OdysLam](https://github.com/OdysLam))
- Always configure multihost database context [\#10732](https://github.com/netdata/netdata/pull/10732) ([stelfrag](https://github.com/stelfrag))
- Removes unused fnc warning in ACLK Legacy [\#10731](https://github.com/netdata/netdata/pull/10731) ([underhood](https://github.com/underhood))
- Update chart's metadata in database when it already exists during creation [\#10728](https://github.com/netdata/netdata/pull/10728) ([stelfrag](https://github.com/stelfrag))
- New thread for ebpf.plugin [\#10726](https://github.com/netdata/netdata/pull/10726) ([thiagoftsm](https://github.com/thiagoftsm))
- Support VS Code container devenv [\#10723](https://github.com/netdata/netdata/pull/10723) ([OdysLam](https://github.com/OdysLam))
- Fixed detection of already claimed node in Docker images. [\#10720](https://github.com/netdata/netdata/pull/10720) ([Ferroin](https://github.com/Ferroin))
- Add statsd guide [\#10719](https://github.com/netdata/netdata/pull/10719) ([OdysLam](https://github.com/OdysLam))
- Add the ability to store chart labels in the database [\#10718](https://github.com/netdata/netdata/pull/10718) ([stelfrag](https://github.com/stelfrag))
- Fix a parameter binding issue when storing chart names in the database [\#10717](https://github.com/netdata/netdata/pull/10717) ([stelfrag](https://github.com/stelfrag))
- Fix typo in backend\_prometheus.c [\#10716](https://github.com/netdata/netdata/pull/10716) ([eltociear](https://github.com/eltociear))
- Add guide: Unsupervised anomaly detection for Raspberry Pi monitoring [\#10713](https://github.com/netdata/netdata/pull/10713) ([joelhans](https://github.com/joelhans))
- Add Working Set charts to the cgroups plugin [\#10712](https://github.com/netdata/netdata/pull/10712) ([vlvkobal](https://github.com/vlvkobal))
- python.d/smartd\_log: collect attribute 233 \(Media Wearout Indicator \(SSD\)\). [\#10711](https://github.com/netdata/netdata/pull/10711) ([aazedo](https://github.com/aazedo))
- Add guide: Develop a custom data collector for Netdata in Python [\#10710](https://github.com/netdata/netdata/pull/10710) ([joelhans](https://github.com/joelhans))
- New version eBPF programs. [\#10707](https://github.com/netdata/netdata/pull/10707) ([thiagoftsm](https://github.com/thiagoftsm))
- Add JSON output option for buildinfo. [\#10706](https://github.com/netdata/netdata/pull/10706) ([Ferroin](https://github.com/Ferroin))
- Fix disk utilization and backlog charts [\#10705](https://github.com/netdata/netdata/pull/10705) ([vlvkobal](https://github.com/vlvkobal))
- update\_kernel\_version: Fix overflow on Centos and probably Ubuntu [\#10704](https://github.com/netdata/netdata/pull/10704) ([thiagoftsm](https://github.com/thiagoftsm))
- Docs: Convert references to `service` to `systemctl` [\#10703](https://github.com/netdata/netdata/pull/10703) ([joelhans](https://github.com/joelhans))
- Add noauthcodecheck workaround flag to the freeipmi plugin [\#10701](https://github.com/netdata/netdata/pull/10701) ([vlvkobal](https://github.com/vlvkobal))
- Add guide: LAMP stack monitoring [\#10698](https://github.com/netdata/netdata/pull/10698) ([joelhans](https://github.com/joelhans))
- Log ACLK cloud commands to access.log [\#10697](https://github.com/netdata/netdata/pull/10697) ([stelfrag](https://github.com/stelfrag))
- Add Linux page cache metrics to eBPF [\#10693](https://github.com/netdata/netdata/pull/10693) ([thiagoftsm](https://github.com/thiagoftsm))
- Update guide: Kubernetes monitoring with Netdata: Overview and visualizations [\#10691](https://github.com/netdata/netdata/pull/10691) ([joelhans](https://github.com/joelhans))
- health: make alarms less sensitive [\#10688](https://github.com/netdata/netdata/pull/10688) ([ilyam8](https://github.com/ilyam8))

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
