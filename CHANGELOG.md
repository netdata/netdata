# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.31.0...HEAD)

**Merged pull requests:**

- docs\(health\): add a note about handling backslashes in configuration files [\#11527](https://github.com/netdata/netdata/pull/11527) ([ilyam8](https://github.com/ilyam8))
- add ipc semaphores charts info [\#11523](https://github.com/netdata/netdata/pull/11523) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.20.0. [\#11521](https://github.com/netdata/netdata/pull/11521) ([netdatabot](https://github.com/netdatabot))
- remove `reset\_netdata\_trace.sh` from netdata.service [\#11517](https://github.com/netdata/netdata/pull/11517) ([ilyam8](https://github.com/ilyam8))
- Remove unused script [\#11516](https://github.com/netdata/netdata/pull/11516) ([thiagoftsm](https://github.com/thiagoftsm))
- Clean up dependency handling for CentOS/RHEL [\#11515](https://github.com/netdata/netdata/pull/11515) ([Ferroin](https://github.com/Ferroin))
- Convert uppercase to lowercase for eBPF submenus [\#11511](https://github.com/netdata/netdata/pull/11511) ([thiagoftsm](https://github.com/thiagoftsm))
- Improved some wordings of the file README.md, only a lifecycle test! [\#11510](https://github.com/netdata/netdata/pull/11510) ([siamaktavakoli](https://github.com/siamaktavakoli))
- Install basic netdata deps by default. [\#11508](https://github.com/netdata/netdata/pull/11508) ([Ferroin](https://github.com/Ferroin))
- Better check for supported -F parameter in sendmail [\#11506](https://github.com/netdata/netdata/pull/11506) ([MrZammler](https://github.com/MrZammler))
- v2.19.9 [\#11505](https://github.com/netdata/netdata/pull/11505) ([allelos](https://github.com/allelos))
- Fix elasticsearch null values returned by \_cat/indices API [\#11501](https://github.com/netdata/netdata/pull/11501) ([vpiserchia](https://github.com/vpiserchia))
- Update claim README.md [\#11492](https://github.com/netdata/netdata/pull/11492) ([car12o](https://github.com/car12o))
- Fix issues in Alarm API [\#11491](https://github.com/netdata/netdata/pull/11491) ([underhood](https://github.com/underhood))
- Revert "add Travis ctrl file for checking if changes happened" [\#11486](https://github.com/netdata/netdata/pull/11486) ([Ferroin](https://github.com/Ferroin))
- Clean netdata naming [\#11484](https://github.com/netdata/netdata/pull/11484) ([andrewm4894](https://github.com/andrewm4894))
- remove broken link [\#11482](https://github.com/netdata/netdata/pull/11482) ([andrewm4894](https://github.com/andrewm4894))
- Allow arbitrary options to be passed to make from netdata-installer.sh. [\#11479](https://github.com/netdata/netdata/pull/11479) ([Ferroin](https://github.com/Ferroin))
- eBPF vsn bump to v0.7.9.1 [\#11471](https://github.com/netdata/netdata/pull/11471) ([UmanShahzad](https://github.com/UmanShahzad))
- eBPF OOM kill tracking [\#11470](https://github.com/netdata/netdata/pull/11470) ([UmanShahzad](https://github.com/UmanShahzad))
- Embed build architecture in static build archive names. [\#11463](https://github.com/netdata/netdata/pull/11463) ([Ferroin](https://github.com/Ferroin))
- Adds aclk/cloud state command to netdatacli [\#11462](https://github.com/netdata/netdata/pull/11462) ([underhood](https://github.com/underhood))
- add a note how to find web files directory for custom dashboards [\#11461](https://github.com/netdata/netdata/pull/11461) ([ilyam8](https://github.com/ilyam8))
- Use correct release codename for Debian 11 repoconfig packages. [\#11459](https://github.com/netdata/netdata/pull/11459) ([Ferroin](https://github.com/Ferroin))
- Fix edge repository configuration DEB packages. [\#11458](https://github.com/netdata/netdata/pull/11458) ([Ferroin](https://github.com/Ferroin))
- Fix postgres replication\_slot chart on standby [\#11455](https://github.com/netdata/netdata/pull/11455) ([anayrat](https://github.com/anayrat))
- Add custom e-mail headers [\#11454](https://github.com/netdata/netdata/pull/11454) ([MrZammler](https://github.com/MrZammler))
- Fix ram level alarms [\#11452](https://github.com/netdata/netdata/pull/11452) ([ilyam8](https://github.com/ilyam8))
- Check for failed protobuf configure or make [\#11450](https://github.com/netdata/netdata/pull/11450) ([MrZammler](https://github.com/MrZammler))
- update "Install Netdata on Synology" guide [\#11449](https://github.com/netdata/netdata/pull/11449) ([ilyam8](https://github.com/ilyam8))
- Don’t bail early if we fail to build cloud deps with required cloud. [\#11446](https://github.com/netdata/netdata/pull/11446) ([Ferroin](https://github.com/Ferroin))
- eBPF Soft IRQ latency [\#11445](https://github.com/netdata/netdata/pull/11445) ([UmanShahzad](https://github.com/UmanShahzad))
- Installation review [\#11442](https://github.com/netdata/netdata/pull/11442) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Update ebpf socket [\#11441](https://github.com/netdata/netdata/pull/11441) ([thiagoftsm](https://github.com/thiagoftsm))
- Update ebpf documentation [\#11440](https://github.com/netdata/netdata/pull/11440) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove ClearLinux from CI. [\#11438](https://github.com/netdata/netdata/pull/11438) ([Ferroin](https://github.com/Ferroin))
- Add terra to blockchains apps groups [\#11437](https://github.com/netdata/netdata/pull/11437) ([etienne-napoleone](https://github.com/etienne-napoleone))
- Fix issue \#11434 regarding inconsistent status check on  component. [\#11435](https://github.com/netdata/netdata/pull/11435) ([0x3333](https://github.com/0x3333))
- Force play timezone [\#11433](https://github.com/netdata/netdata/pull/11433) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Default to not using LTO for builds. [\#11432](https://github.com/netdata/netdata/pull/11432) ([Ferroin](https://github.com/Ferroin))
- Properly handle reuploads of packages to PackageCloud. [\#11427](https://github.com/netdata/netdata/pull/11427) ([Ferroin](https://github.com/Ferroin))
- Use DebHelper compat level 9 in repoconfig packages to support Ubuntu 16.04 [\#11426](https://github.com/netdata/netdata/pull/11426) ([Ferroin](https://github.com/Ferroin))
- Adds Alert Related API for new protocol [\#11424](https://github.com/netdata/netdata/pull/11424) ([underhood](https://github.com/underhood))
- Update SQLite version from v3.33.0 to 3.36.0 [\#11423](https://github.com/netdata/netdata/pull/11423) ([stelfrag](https://github.com/stelfrag))
- Add SQLite unit tests [\#11422](https://github.com/netdata/netdata/pull/11422) ([stelfrag](https://github.com/stelfrag))
- Remove Ubuntu 20.10 from repository config package builds. [\#11421](https://github.com/netdata/netdata/pull/11421) ([Ferroin](https://github.com/Ferroin))
- Adds NodeInstanceInfo API [\#11419](https://github.com/netdata/netdata/pull/11419) ([underhood](https://github.com/underhood))
- Custom dash broken links [\#11413](https://github.com/netdata/netdata/pull/11413) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Fix CID372233 to CID 372236 [\#11411](https://github.com/netdata/netdata/pull/11411) ([underhood](https://github.com/underhood))
- eBPF Hard IRQ latency [\#11410](https://github.com/netdata/netdata/pull/11410) ([UmanShahzad](https://github.com/UmanShahzad))
- minor - Remove autoreconf warning [\#11407](https://github.com/netdata/netdata/pull/11407) ([underhood](https://github.com/underhood))
- Fix bundled protobuf linkage on systems needing -latomic [\#11406](https://github.com/netdata/netdata/pull/11406) ([underhood](https://github.com/underhood))
- replaced reference to raw.githubuser to a relative path [\#11405](https://github.com/netdata/netdata/pull/11405) ([hugovalente-pm](https://github.com/hugovalente-pm))
- remove "vernemq.queue\_messages\_in\_queues" from dashboard\_info.js [\#11403](https://github.com/netdata/netdata/pull/11403) ([ilyam8](https://github.com/ilyam8))
- Split eBPF programs [\#11401](https://github.com/netdata/netdata/pull/11401) ([thiagoftsm](https://github.com/thiagoftsm))
- Use only stock configuration filenames as module names [\#11400](https://github.com/netdata/netdata/pull/11400) ([vlvkobal](https://github.com/vlvkobal))
- Fix freebsd and macos plugin names [\#11398](https://github.com/netdata/netdata/pull/11398) ([vlvkobal](https://github.com/vlvkobal))
- Add ACLK synchronization event loop [\#11396](https://github.com/netdata/netdata/pull/11396) ([stelfrag](https://github.com/stelfrag))
- Add HTTP basic authentication to some exporting connectors [\#11394](https://github.com/netdata/netdata/pull/11394) ([vlvkobal](https://github.com/vlvkobal))
- New Cloud chart related parsers and generators [\#11393](https://github.com/netdata/netdata/pull/11393) ([underhood](https://github.com/underhood))
- Update perf.events and add new charts [\#11392](https://github.com/netdata/netdata/pull/11392) ([thiagoftsm](https://github.com/thiagoftsm))
- charts.d.plugin: set "module" when sending CHART [\#11390](https://github.com/netdata/netdata/pull/11390) ([ilyam8](https://github.com/ilyam8))
- Remove warning when GCC 8.x is used [\#11389](https://github.com/netdata/netdata/pull/11389) ([thiagoftsm](https://github.com/thiagoftsm))
- Specify module for threads [\#11387](https://github.com/netdata/netdata/pull/11387) ([thiagoftsm](https://github.com/thiagoftsm))
- add capsh check before issuing setcap cap\_perfmon [\#11386](https://github.com/netdata/netdata/pull/11386) ([rgiovanardi](https://github.com/rgiovanardi))
- add Travis ctrl file for checking if changes happened [\#11383](https://github.com/netdata/netdata/pull/11383) ([rgiovanardi](https://github.com/rgiovanardi))
- Claiming review to rename claiming action to connect [\#11378](https://github.com/netdata/netdata/pull/11378) ([hugovalente-pm](https://github.com/hugovalente-pm))
- CODEOWNERS: Remove knatsakis [\#11377](https://github.com/netdata/netdata/pull/11377) ([knatsakis](https://github.com/knatsakis))
- Docs: Remove extra 's' [\#11376](https://github.com/netdata/netdata/pull/11376) ([danmichaelo](https://github.com/danmichaelo))
- Update handling of builds of bundled dependencies. [\#11375](https://github.com/netdata/netdata/pull/11375) ([Ferroin](https://github.com/Ferroin))
- Added support for bundling protobuf as part of the install. [\#11374](https://github.com/netdata/netdata/pull/11374) ([Ferroin](https://github.com/Ferroin))
- Ebpf latency description [\#11363](https://github.com/netdata/netdata/pull/11363) ([thiagoftsm](https://github.com/thiagoftsm))
- Properly handle eBPF plugin in RPM packages. [\#11362](https://github.com/netdata/netdata/pull/11362) ([Ferroin](https://github.com/Ferroin))
- fix `gearman\_workers\_queued` alarm [\#11361](https://github.com/netdata/netdata/pull/11361) ([ilyam8](https://github.com/ilyam8))
- update cockroachdb replication alarms [\#11360](https://github.com/netdata/netdata/pull/11360) ([ilyam8](https://github.com/ilyam8))
- disable oom\_kill alarm if the node is k8s node [\#11359](https://github.com/netdata/netdata/pull/11359) ([ilyam8](https://github.com/ilyam8))
- eBPF mount [\#11358](https://github.com/netdata/netdata/pull/11358) ([thiagoftsm](https://github.com/thiagoftsm))
- FIX index reference [\#11356](https://github.com/netdata/netdata/pull/11356) ([thiagoftsm](https://github.com/thiagoftsm))
- fix sending MS Teams notifications to multiple channels [\#11355](https://github.com/netdata/netdata/pull/11355) ([ilyam8](https://github.com/ilyam8))
- Added support for claiming existing installs via kickstarter scripts. [\#11350](https://github.com/netdata/netdata/pull/11350) ([Ferroin](https://github.com/Ferroin))
- packaging: update go.d.plugin version to v0.30.0 [\#11349](https://github.com/netdata/netdata/pull/11349) ([ilyam8](https://github.com/ilyam8))
- eBPF btrfs [\#11348](https://github.com/netdata/netdata/pull/11348) ([thiagoftsm](https://github.com/thiagoftsm))
- Assorted kickstart install fixes. [\#11342](https://github.com/netdata/netdata/pull/11342) ([Ferroin](https://github.com/Ferroin))
- add geth default config [\#11341](https://github.com/netdata/netdata/pull/11341) ([OdysLam](https://github.com/OdysLam))
- Allows ACLK-NG to grow MQTT buffer [\#11340](https://github.com/netdata/netdata/pull/11340) ([underhood](https://github.com/underhood))
- Adds aclk-schemas to dist\_noinst\_DATA [\#11338](https://github.com/netdata/netdata/pull/11338) ([underhood](https://github.com/underhood))
- Allows bundled protobuf [\#11335](https://github.com/netdata/netdata/pull/11335) ([underhood](https://github.com/underhood))
- Add Debian 11 \(Bullseye\) to CI. [\#11334](https://github.com/netdata/netdata/pull/11334) ([Ferroin](https://github.com/Ferroin))
- eBPF ZFS monitoring [\#11330](https://github.com/netdata/netdata/pull/11330) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix typo in analytics.c [\#11329](https://github.com/netdata/netdata/pull/11329) ([eltociear](https://github.com/eltociear))
- add postgresql version to requirements section [\#11328](https://github.com/netdata/netdata/pull/11328) ([charoleizer](https://github.com/charoleizer))
- Add ACLK-NG cloud request type charts [\#11326](https://github.com/netdata/netdata/pull/11326) ([UmanShahzad](https://github.com/UmanShahzad))
- v2.19.1 [\#11323](https://github.com/netdata/netdata/pull/11323) ([allelos](https://github.com/allelos))
- Fixes coverity errors in ACLK [\#11322](https://github.com/netdata/netdata/pull/11322) ([underhood](https://github.com/underhood))
- Fix tiny issues in docs [\#11320](https://github.com/netdata/netdata/pull/11320) ([UmanShahzad](https://github.com/UmanShahzad))
- Add some more entries to gitignore [\#11319](https://github.com/netdata/netdata/pull/11319) ([UmanShahzad](https://github.com/UmanShahzad))
- Add HTTP access log messages for ACLK-NG [\#11318](https://github.com/netdata/netdata/pull/11318) ([UmanShahzad](https://github.com/UmanShahzad))
- Updated dashboard version. [\#11316](https://github.com/netdata/netdata/pull/11316) ([allelos](https://github.com/allelos))
- Log message when the page cache manager sleeps for more than 1 second. [\#11314](https://github.com/netdata/netdata/pull/11314) ([vkalintiris](https://github.com/vkalintiris))
- eBPF NFS monitoring [\#11313](https://github.com/netdata/netdata/pull/11313) ([thiagoftsm](https://github.com/thiagoftsm))
- Add hop count for children [\#11311](https://github.com/netdata/netdata/pull/11311) ([stelfrag](https://github.com/stelfrag))
- fix various python modules charts contexts [\#11310](https://github.com/netdata/netdata/pull/11310) ([ilyam8](https://github.com/ilyam8))
- \[docs\] fix prometheus node cpu alert rule [\#11309](https://github.com/netdata/netdata/pull/11309) ([ilyam8](https://github.com/ilyam8))
- health: remove pythond modules specific last\_collected alarms [\#11307](https://github.com/netdata/netdata/pull/11307) ([ilyam8](https://github.com/ilyam8))
- Update get-started.mdx [\#11303](https://github.com/netdata/netdata/pull/11303) ([jlbriston](https://github.com/jlbriston))
- fix `mdstat` current operation charts context/title [\#11289](https://github.com/netdata/netdata/pull/11289) ([ilyam8](https://github.com/ilyam8))
- Remove access check for install-type file [\#11288](https://github.com/netdata/netdata/pull/11288) ([MrZammler](https://github.com/MrZammler))
- eBPF plugin remove parallel plugins [\#11287](https://github.com/netdata/netdata/pull/11287) ([thiagoftsm](https://github.com/thiagoftsm))
- suport TLS SNI in ACLK-NG [\#11285](https://github.com/netdata/netdata/pull/11285) ([underhood](https://github.com/underhood))
- Check if sendmail supports -F parameter [\#11283](https://github.com/netdata/netdata/pull/11283) ([MrZammler](https://github.com/MrZammler))
- Properly handle the file list for updating the dashboard. [\#11282](https://github.com/netdata/netdata/pull/11282) ([Ferroin](https://github.com/Ferroin))
- fixes confusing error in ACLK Legacy [\#11278](https://github.com/netdata/netdata/pull/11278) ([underhood](https://github.com/underhood))
- Ebpf disk latency [\#11276](https://github.com/netdata/netdata/pull/11276) ([thiagoftsm](https://github.com/thiagoftsm))
- Auto-detect PGID in Dockerfile's ENTRYPOINT script [\#11274](https://github.com/netdata/netdata/pull/11274) ([OdysLam](https://github.com/OdysLam))
- Add code for repository configuration packages. [\#11273](https://github.com/netdata/netdata/pull/11273) ([Ferroin](https://github.com/Ferroin))
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
- ACLK-NG New Cloud NodeInstance related msgs [\#11234](https://github.com/netdata/netdata/pull/11234) ([underhood](https://github.com/underhood))
- Ebpf arrays [\#11230](https://github.com/netdata/netdata/pull/11230) ([thiagoftsm](https://github.com/thiagoftsm))
- Add links to data privacy page [\#11226](https://github.com/netdata/netdata/pull/11226) ([joelhans](https://github.com/joelhans))
- Allows ACLK NG and Legacy to coexist [\#11225](https://github.com/netdata/netdata/pull/11225) ([underhood](https://github.com/underhood))
- eBPF ext4 \(new thread for collector\) [\#11224](https://github.com/netdata/netdata/pull/11224) ([thiagoftsm](https://github.com/thiagoftsm))
- Move cleanup of obsolete charts to a separate thread [\#11222](https://github.com/netdata/netdata/pull/11222) ([vlvkobal](https://github.com/vlvkobal))
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
- Update news in main README for latest release [\#11165](https://github.com/netdata/netdata/pull/11165) ([joelhans](https://github.com/joelhans))
- Move parser from children to main thread [\#11152](https://github.com/netdata/netdata/pull/11152) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove deprecated options. [\#11149](https://github.com/netdata/netdata/pull/11149) ([vkalintiris](https://github.com/vkalintiris))
- Removed Fedora 32 from CI. [\#11143](https://github.com/netdata/netdata/pull/11143) ([Ferroin](https://github.com/Ferroin))
- Compile/Link with absolute paths for bundled/vendored deps. [\#11129](https://github.com/netdata/netdata/pull/11129) ([vkalintiris](https://github.com/vkalintiris))
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

## [v1.30.1](https://github.com/netdata/netdata/tree/v1.30.1) (2021-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.0...v1.30.1)

**Merged pull requests:**

- Don’t use glob expansion in argument to `cd` in updater. [\#10936](https://github.com/netdata/netdata/pull/10936) ([Ferroin](https://github.com/Ferroin))
- Fix memory corruption issue when executing context queries in RAM/SAVE memory mode [\#10933](https://github.com/netdata/netdata/pull/10933) ([stelfrag](https://github.com/stelfrag))
- Update CODEOWNERS [\#10928](https://github.com/netdata/netdata/pull/10928) ([knatsakis](https://github.com/knatsakis))
- Update news and GIF in README, fix typo [\#10900](https://github.com/netdata/netdata/pull/10900) ([joelhans](https://github.com/joelhans))
- Update README.md [\#10898](https://github.com/netdata/netdata/pull/10898) ([slimanio](https://github.com/slimanio))

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
