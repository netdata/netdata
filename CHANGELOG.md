# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.31.0...HEAD)

**Merged pull requests:**

- Show stats for protected mount points in diskspace plugin [\#11767](https://github.com/netdata/netdata/pull/11767) ([vlvkobal](https://github.com/vlvkobal))
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
- Disable C++ warnings from dlib library. [\#11738](https://github.com/netdata/netdata/pull/11738) ([vkalintiris](https://github.com/vkalintiris))
- Fix typo in aclk\_query.c [\#11737](https://github.com/netdata/netdata/pull/11737) ([eltociear](https://github.com/eltociear))
- Fix online chart in NG not updated properly [\#11734](https://github.com/netdata/netdata/pull/11734) ([underhood](https://github.com/underhood))
- Fix Link to New Charts [\#11729](https://github.com/netdata/netdata/pull/11729) ([DShreve2](https://github.com/DShreve2))
- Fix Header Link.md [\#11728](https://github.com/netdata/netdata/pull/11728) ([DShreve2](https://github.com/DShreve2))
- Implements cloud initiated disconnect command [\#11723](https://github.com/netdata/netdata/pull/11723) ([underhood](https://github.com/underhood))
- Adding \(eBPF\) to submenu [\#11721](https://github.com/netdata/netdata/pull/11721) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix coverity CID \#373610 [\#11719](https://github.com/netdata/netdata/pull/11719) ([MrZammler](https://github.com/MrZammler))
- add sensors to charts.d.conf and add a note how to enable it [\#11715](https://github.com/netdata/netdata/pull/11715) ([ilyam8](https://github.com/ilyam8))
- Add Cloud sign-up link to README.md [\#11714](https://github.com/netdata/netdata/pull/11714) ([DShreve2](https://github.com/DShreve2))
- Updating Docker Node Instructions for Clarity [\#11713](https://github.com/netdata/netdata/pull/11713) ([DShreve2](https://github.com/DShreve2))
- Update jQuery Dependency [\#11710](https://github.com/netdata/netdata/pull/11710) ([rupokify](https://github.com/rupokify))
- Bring eBPF to static binaries [\#11709](https://github.com/netdata/netdata/pull/11709) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix kickstart.md Installation Guide Links [\#11708](https://github.com/netdata/netdata/pull/11708) ([DShreve2](https://github.com/DShreve2))
- Queue removed alerts to cloud for new architecture [\#11704](https://github.com/netdata/netdata/pull/11704) ([MrZammler](https://github.com/MrZammler))
- Ebpf doc [\#11703](https://github.com/netdata/netdata/pull/11703) ([thiagoftsm](https://github.com/thiagoftsm))
- Charts 2.0 - fix broken link [\#11701](https://github.com/netdata/netdata/pull/11701) ([hugovalente-pm](https://github.com/hugovalente-pm))
- postgres collector: Fix crash the wal query if wal-file was removed concurrently [\#11697](https://github.com/netdata/netdata/pull/11697) ([unhandled-exception](https://github.com/unhandled-exception))
- Fix handling of disabling telemetry in static installs. [\#11689](https://github.com/netdata/netdata/pull/11689) ([Ferroin](https://github.com/Ferroin))
- fix "lsns: unknown column" logging in cgroup-network-helper script [\#11687](https://github.com/netdata/netdata/pull/11687) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues 373612 & 373611 [\#11684](https://github.com/netdata/netdata/pull/11684) ([MrZammler](https://github.com/MrZammler))
- eBPF mdflush [\#11681](https://github.com/netdata/netdata/pull/11681) ([UmanShahzad](https://github.com/UmanShahzad))
- New eBPF and libbpf releases [\#11680](https://github.com/netdata/netdata/pull/11680) ([thiagoftsm](https://github.com/thiagoftsm))
- Mark g++ for freebsd as NOTREQUIRED [\#11678](https://github.com/netdata/netdata/pull/11678) ([MrZammler](https://github.com/MrZammler))
- Fix warnings from -Wformat-truncation=2 [\#11676](https://github.com/netdata/netdata/pull/11676) ([MrZammler](https://github.com/MrZammler))
- Stream chart labels [\#11675](https://github.com/netdata/netdata/pull/11675) ([MrZammler](https://github.com/MrZammler))
- Update pfsense.md [\#11674](https://github.com/netdata/netdata/pull/11674) ([78Star](https://github.com/78Star))
- fix swap\_used alarm calc [\#11672](https://github.com/netdata/netdata/pull/11672) ([ilyam8](https://github.com/ilyam8))
- Fix line arguments \(eBPF\) [\#11670](https://github.com/netdata/netdata/pull/11670) ([thiagoftsm](https://github.com/thiagoftsm))
- Add snapshot message for cloud new architecture [\#11664](https://github.com/netdata/netdata/pull/11664) ([MrZammler](https://github.com/MrZammler))
- Fix interval usage and reduce I/O [\#11662](https://github.com/netdata/netdata/pull/11662) ([thiagoftsm](https://github.com/thiagoftsm))
- Update dashboard to version v2.20.9. [\#11661](https://github.com/netdata/netdata/pull/11661) ([netdatabot](https://github.com/netdatabot))
- Optimize static build and update various dependencies. [\#11660](https://github.com/netdata/netdata/pull/11660) ([Ferroin](https://github.com/Ferroin))
- Sanely handle installing on systems with limited RAM. [\#11658](https://github.com/netdata/netdata/pull/11658) ([Ferroin](https://github.com/Ferroin))
- Mark unmaintained tests as expected failures. [\#11657](https://github.com/netdata/netdata/pull/11657) ([vkalintiris](https://github.com/vkalintiris))
- Fix build issue related to legacy aclk and new arch code [\#11655](https://github.com/netdata/netdata/pull/11655) ([MrZammler](https://github.com/MrZammler))
- minor - fixes typo in URL when calling env [\#11651](https://github.com/netdata/netdata/pull/11651) ([underhood](https://github.com/underhood))
- Fix false poll timeout [\#11650](https://github.com/netdata/netdata/pull/11650) ([underhood](https://github.com/underhood))
- Use submodules in Clang build checks. [\#11649](https://github.com/netdata/netdata/pull/11649) ([Ferroin](https://github.com/Ferroin))
- Fix chart config overflow [\#11645](https://github.com/netdata/netdata/pull/11645) ([stelfrag](https://github.com/stelfrag))
- Explicitly opt out of LTO in RPM builds. [\#11644](https://github.com/netdata/netdata/pull/11644) ([Ferroin](https://github.com/Ferroin))
- eBPF process \(collector improvements\) [\#11643](https://github.com/netdata/netdata/pull/11643) ([thiagoftsm](https://github.com/thiagoftsm))
- eBPF cgroup integration [\#11642](https://github.com/netdata/netdata/pull/11642) ([thiagoftsm](https://github.com/thiagoftsm))
- Add Fedora 35 to CI. [\#11641](https://github.com/netdata/netdata/pull/11641) ([Ferroin](https://github.com/Ferroin))
- various fixes and updates for dashboard info [\#11639](https://github.com/netdata/netdata/pull/11639) ([ilyam8](https://github.com/ilyam8))
- Fix an overflow when unsigned integer subtracted [\#11638](https://github.com/netdata/netdata/pull/11638) ([vlvkobal](https://github.com/vlvkobal))
- add note for the new release of charts on the cloud [\#11637](https://github.com/netdata/netdata/pull/11637) ([hugovalente-pm](https://github.com/hugovalente-pm))
- add timex.plugin charts info [\#11635](https://github.com/netdata/netdata/pull/11635) ([ilyam8](https://github.com/ilyam8))
- Add protobuf to `-W buildinfo` output. [\#11634](https://github.com/netdata/netdata/pull/11634) ([Ferroin](https://github.com/Ferroin))
- Revert "Update alarms info" [\#11633](https://github.com/netdata/netdata/pull/11633) ([ilyam8](https://github.com/ilyam8))
- Fix nfsd RPC metrics and remove unused nfsd charts and metrics [\#11632](https://github.com/netdata/netdata/pull/11632) ([vlvkobal](https://github.com/vlvkobal))
- add proc zfs charts info [\#11630](https://github.com/netdata/netdata/pull/11630) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.20.7. [\#11629](https://github.com/netdata/netdata/pull/11629) ([netdatabot](https://github.com/netdatabot))
- add sys\_class\_infiniband charts info [\#11628](https://github.com/netdata/netdata/pull/11628) ([ilyam8](https://github.com/ilyam8))
- add proc\_pagetypeinfo charts info [\#11627](https://github.com/netdata/netdata/pull/11627) ([ilyam8](https://github.com/ilyam8))
- add proc\_net\_wireless charts info [\#11626](https://github.com/netdata/netdata/pull/11626) ([ilyam8](https://github.com/ilyam8))
- add proc\_net\_rpc\_nfs and nfsd charts info [\#11625](https://github.com/netdata/netdata/pull/11625) ([ilyam8](https://github.com/ilyam8))
- fix proc nfsd "proc4ops" chart family [\#11623](https://github.com/netdata/netdata/pull/11623) ([ilyam8](https://github.com/ilyam8))
- Initialize struct with zeroes [\#11621](https://github.com/netdata/netdata/pull/11621) ([MrZammler](https://github.com/MrZammler))
- add sys\_class\_power\_supply charts info [\#11619](https://github.com/netdata/netdata/pull/11619) ([ilyam8](https://github.com/ilyam8))
- add cgroups.plugin systemd units charts info [\#11618](https://github.com/netdata/netdata/pull/11618) ([ilyam8](https://github.com/ilyam8))
- Fix swap size calculation for cgroups [\#11617](https://github.com/netdata/netdata/pull/11617) ([vlvkobal](https://github.com/vlvkobal))
- Fix RSS memory counter for systemd services [\#11616](https://github.com/netdata/netdata/pull/11616) ([vlvkobal](https://github.com/vlvkobal))
- Add @iigorkarpov to CODEOWNERS. [\#11614](https://github.com/netdata/netdata/pull/11614) ([Ferroin](https://github.com/Ferroin))
- Adds new alarm status protocol messages [\#11612](https://github.com/netdata/netdata/pull/11612) ([underhood](https://github.com/underhood))
- eBPF and cgroup \(process, file descriptor, VFS, directory cache and OOMkill\) [\#11611](https://github.com/netdata/netdata/pull/11611) ([thiagoftsm](https://github.com/thiagoftsm))
- apps: disable reporting min/avg/max group uptime by default [\#11609](https://github.com/netdata/netdata/pull/11609) ([ilyam8](https://github.com/ilyam8))
- fix https client  [\#11608](https://github.com/netdata/netdata/pull/11608) ([underhood](https://github.com/underhood))
- add cgroups.plugin charts descriptions [\#11607](https://github.com/netdata/netdata/pull/11607) ([ilyam8](https://github.com/ilyam8))
- Add flag to mark containers as created from official images in analytics. [\#11606](https://github.com/netdata/netdata/pull/11606) ([Ferroin](https://github.com/Ferroin))
- Update optional parameters for upcoming installer. [\#11604](https://github.com/netdata/netdata/pull/11604) ([DShreve2](https://github.com/DShreve2))
- add apps.plugin charts descriptions [\#11601](https://github.com/netdata/netdata/pull/11601) ([ilyam8](https://github.com/ilyam8))
- add proc\_vmstat charts info [\#11597](https://github.com/netdata/netdata/pull/11597) ([ilyam8](https://github.com/ilyam8))
- fix varnish VBE parsing [\#11596](https://github.com/netdata/netdata/pull/11596) ([ilyam8](https://github.com/ilyam8))
- add sys\_kernel\_mm\_ksm charts info [\#11595](https://github.com/netdata/netdata/pull/11595) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.20.2. [\#11593](https://github.com/netdata/netdata/pull/11593) ([netdatabot](https://github.com/netdatabot))
- Add POWER8+ support to our official Docker images. [\#11592](https://github.com/netdata/netdata/pull/11592) ([Ferroin](https://github.com/Ferroin))
- add sys\_devices\_system\_edac\_mc charts info [\#11589](https://github.com/netdata/netdata/pull/11589) ([ilyam8](https://github.com/ilyam8))
- Adds local webserver API/v1 call "aclk" [\#11588](https://github.com/netdata/netdata/pull/11588) ([underhood](https://github.com/underhood))
- Makes New Cloud architecture optional for ACLK-NG [\#11587](https://github.com/netdata/netdata/pull/11587) ([underhood](https://github.com/underhood))
- add proc\_stat charts info [\#11586](https://github.com/netdata/netdata/pull/11586) ([ilyam8](https://github.com/ilyam8))
- Add Ubuntu 21.10 to CI. [\#11585](https://github.com/netdata/netdata/pull/11585) ([Ferroin](https://github.com/Ferroin))
- Remove unused synproxy chart [\#11582](https://github.com/netdata/netdata/pull/11582) ([vlvkobal](https://github.com/vlvkobal))
- add proc\_net\_stat\_synproxy charts info [\#11581](https://github.com/netdata/netdata/pull/11581) ([ilyam8](https://github.com/ilyam8))
- Sorting the Postgres cluster databases in the postgres collector [\#11580](https://github.com/netdata/netdata/pull/11580) ([unhandled-exception](https://github.com/unhandled-exception))
- Enable additional functionality for the new cloud architecture [\#11579](https://github.com/netdata/netdata/pull/11579) ([stelfrag](https://github.com/stelfrag))
- Fix CID 339027 and reverse arguments [\#11578](https://github.com/netdata/netdata/pull/11578) ([thiagoftsm](https://github.com/thiagoftsm))
- add proc\_softirqs charts info [\#11577](https://github.com/netdata/netdata/pull/11577) ([ilyam8](https://github.com/ilyam8))
- add proc\_net\_stat\_conntrack charts info [\#11576](https://github.com/netdata/netdata/pull/11576) ([ilyam8](https://github.com/ilyam8))
- Free analytics data when analytics thread stops [\#11575](https://github.com/netdata/netdata/pull/11575) ([MrZammler](https://github.com/MrZammler))
- add missing privilege to fix MySQL slave reporting [\#11574](https://github.com/netdata/netdata/pull/11574) ([steffenweber](https://github.com/steffenweber))
- Integrate eBPF and cgroup \(consumer side\) [\#11573](https://github.com/netdata/netdata/pull/11573) ([thiagoftsm](https://github.com/thiagoftsm))
- add proc\_uptime charts info [\#11569](https://github.com/netdata/netdata/pull/11569) ([ilyam8](https://github.com/ilyam8))
- add proc\_net\_sockstat and sockstat6 charts info [\#11567](https://github.com/netdata/netdata/pull/11567) ([ilyam8](https://github.com/ilyam8))
- Disable eBPF compilation in different platforms [\#11566](https://github.com/netdata/netdata/pull/11566) ([thiagoftsm](https://github.com/thiagoftsm))
- add proc\_net\_snmp6 charts info [\#11565](https://github.com/netdata/netdata/pull/11565) ([ilyam8](https://github.com/ilyam8))
- add proc\_net\_sctp\_snmp charts info [\#11564](https://github.com/netdata/netdata/pull/11564) ([ilyam8](https://github.com/ilyam8))
- eBPF Shared Memory system call tracking [\#11560](https://github.com/netdata/netdata/pull/11560) ([UmanShahzad](https://github.com/UmanShahzad))
- Add shared memory to cgroup [\#11559](https://github.com/netdata/netdata/pull/11559) ([thiagoftsm](https://github.com/thiagoftsm))
- add proc\_net\_snmp charts info [\#11557](https://github.com/netdata/netdata/pull/11557) ([ilyam8](https://github.com/ilyam8))
- End of support for Ubuntu 16.04 [\#11556](https://github.com/netdata/netdata/pull/11556) ([Ferroin](https://github.com/Ferroin))
- add proc\_net\_netstat charts info [\#11554](https://github.com/netdata/netdata/pull/11554) ([ilyam8](https://github.com/ilyam8))
- Add alert message support for ACLK new architecture [\#11552](https://github.com/netdata/netdata/pull/11552) ([MrZammler](https://github.com/MrZammler))
- Anomaly Detection MVP [\#11548](https://github.com/netdata/netdata/pull/11548) ([vkalintiris](https://github.com/vkalintiris))
- Update ebpf dashboard [\#11547](https://github.com/netdata/netdata/pull/11547) ([thiagoftsm](https://github.com/thiagoftsm))
- add proc\_net\_ip\_vs\_stats charts info [\#11546](https://github.com/netdata/netdata/pull/11546) ([ilyam8](https://github.com/ilyam8))
- fix proc collector: Undefined state DEGRADED for zpool [\#11545](https://github.com/netdata/netdata/pull/11545) ([elelayan](https://github.com/elelayan))
- add proc\_net\_dev charts info [\#11543](https://github.com/netdata/netdata/pull/11543) ([ilyam8](https://github.com/ilyam8))
- add proc\_meminfo charts info [\#11541](https://github.com/netdata/netdata/pull/11541) ([ilyam8](https://github.com/ilyam8))
- fix\(docs\): broken links [\#11540](https://github.com/netdata/netdata/pull/11540) ([ilyam8](https://github.com/ilyam8))
- Fix installer flag --use-system-protobuf [\#11539](https://github.com/netdata/netdata/pull/11539) ([underhood](https://github.com/underhood))
- add proc\_mdstat charts info [\#11537](https://github.com/netdata/netdata/pull/11537) ([ilyam8](https://github.com/ilyam8))
- Add New Cloud Protocol files to CMake [\#11536](https://github.com/netdata/netdata/pull/11536) ([underhood](https://github.com/underhood))
- Fix coverity issues for health config [\#11535](https://github.com/netdata/netdata/pull/11535) ([MrZammler](https://github.com/MrZammler))
- update london demo to point at london3 [\#11533](https://github.com/netdata/netdata/pull/11533) ([andrewm4894](https://github.com/andrewm4894))
- add/update proc\_interrupts charts info [\#11532](https://github.com/netdata/netdata/pull/11532) ([ilyam8](https://github.com/ilyam8))
- add diskstats charts info  [\#11528](https://github.com/netdata/netdata/pull/11528) ([ilyam8](https://github.com/ilyam8))
- docs\(health\): add a note about handling backslashes in configuration files [\#11527](https://github.com/netdata/netdata/pull/11527) ([ilyam8](https://github.com/ilyam8))
- Re-added EPEL on CentOS 7. [\#11525](https://github.com/netdata/netdata/pull/11525) ([Ferroin](https://github.com/Ferroin))
- Fix issue with log messages appearing in the terminal instead of the error.log on startup [\#11524](https://github.com/netdata/netdata/pull/11524) ([stelfrag](https://github.com/stelfrag))
- add ipc semaphores charts info [\#11523](https://github.com/netdata/netdata/pull/11523) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.20.0. [\#11521](https://github.com/netdata/netdata/pull/11521) ([netdatabot](https://github.com/netdatabot))
- Use the correct exit status for the updater with static updates. [\#11520](https://github.com/netdata/netdata/pull/11520) ([Ferroin](https://github.com/Ferroin))
- remove `reset\_netdata\_trace.sh` from netdata.service [\#11517](https://github.com/netdata/netdata/pull/11517) ([ilyam8](https://github.com/ilyam8))
- Remove unused script [\#11516](https://github.com/netdata/netdata/pull/11516) ([thiagoftsm](https://github.com/thiagoftsm))
- Clean up dependency handling for CentOS/RHEL [\#11515](https://github.com/netdata/netdata/pull/11515) ([Ferroin](https://github.com/Ferroin))
- Add node message support for ACLK new architecture [\#11514](https://github.com/netdata/netdata/pull/11514) ([stelfrag](https://github.com/stelfrag))
- Convert uppercase to lowercase for eBPF submenus [\#11511](https://github.com/netdata/netdata/pull/11511) ([thiagoftsm](https://github.com/thiagoftsm))
- Improved some wordings of the file README.md, only a lifecycle test! [\#11510](https://github.com/netdata/netdata/pull/11510) ([siamaktavakoli](https://github.com/siamaktavakoli))
- Install basic netdata deps by default. [\#11508](https://github.com/netdata/netdata/pull/11508) ([Ferroin](https://github.com/Ferroin))
- Fix handling of claiming in kickstart script when running as non-root. [\#11507](https://github.com/netdata/netdata/pull/11507) ([Ferroin](https://github.com/Ferroin))
- Better check for supported -F parameter in sendmail [\#11506](https://github.com/netdata/netdata/pull/11506) ([MrZammler](https://github.com/MrZammler))
- v2.19.9 [\#11505](https://github.com/netdata/netdata/pull/11505) ([allelos](https://github.com/allelos))
- Fix elasticsearch null values returned by \_cat/indices API [\#11501](https://github.com/netdata/netdata/pull/11501) ([vpiserchia](https://github.com/vpiserchia))
- Update claim README.md [\#11492](https://github.com/netdata/netdata/pull/11492) ([car12o](https://github.com/car12o))
- Fix issues in Alarm API [\#11491](https://github.com/netdata/netdata/pull/11491) ([underhood](https://github.com/underhood))
- Added static builds for ARMv7l and ARMv8a [\#11490](https://github.com/netdata/netdata/pull/11490) ([Ferroin](https://github.com/Ferroin))
- Update alarms info [\#11481](https://github.com/netdata/netdata/pull/11481) ([ilyam8](https://github.com/ilyam8))
- Update libbpf [\#11480](https://github.com/netdata/netdata/pull/11480) ([thiagoftsm](https://github.com/thiagoftsm))
- Allow arbitrary options to be passed to make from netdata-installer.sh. [\#11479](https://github.com/netdata/netdata/pull/11479) ([Ferroin](https://github.com/Ferroin))
- eBPF OOM kill tracking [\#11470](https://github.com/netdata/netdata/pull/11470) ([UmanShahzad](https://github.com/UmanShahzad))
- Adds aclk/cloud state command to netdatacli [\#11462](https://github.com/netdata/netdata/pull/11462) ([underhood](https://github.com/underhood))
- Add custom e-mail headers [\#11454](https://github.com/netdata/netdata/pull/11454) ([MrZammler](https://github.com/MrZammler))
- Add chart message support for ACLK new architecture [\#11447](https://github.com/netdata/netdata/pull/11447) ([stelfrag](https://github.com/stelfrag))
- Use sqlite to store the health log and alert configurations. [\#11399](https://github.com/netdata/netdata/pull/11399) ([MrZammler](https://github.com/MrZammler))

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
