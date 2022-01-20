# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/1.32.1...HEAD)

**Merged pull requests:**

- Remove internal dbengine header from spawn/spawn\_client.c [\#12009](https://github.com/netdata/netdata/pull/12009) ([vkalintiris](https://github.com/vkalintiris))
- Fix typo in the dashboard\_info.js spigot part [\#12008](https://github.com/netdata/netdata/pull/12008) ([lokerhp](https://github.com/lokerhp))
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
- add note with link to guide for using on Pi [\#11605](https://github.com/netdata/netdata/pull/11605) ([andrewm4894](https://github.com/andrewm4894))

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
- Remove Fedora 33 from CI. [\#11640](https://github.com/netdata/netdata/pull/11640) ([Ferroin](https://github.com/Ferroin))
- Remove OpenSUSE Leap 15.2 from CI. [\#11600](https://github.com/netdata/netdata/pull/11600) ([Ferroin](https://github.com/Ferroin))

## [v1.32.0](https://github.com/netdata/netdata/tree/v1.32.0) (2021-11-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.31.0...v1.32.0)

**Merged pull requests:**

- fix\(health\): `pihole\_blocklist\_gravity\_file` and `pihole\_status` info lines [\#11844](https://github.com/netdata/netdata/pull/11844) ([ilyam8](https://github.com/ilyam8))
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
- Disable C++ warnings from dlib library. [\#11738](https://github.com/netdata/netdata/pull/11738) ([vkalintiris](https://github.com/vkalintiris))
- Fix typo in aclk\_query.c [\#11737](https://github.com/netdata/netdata/pull/11737) ([eltociear](https://github.com/eltociear))
- Fix online chart in NG not updated properly [\#11734](https://github.com/netdata/netdata/pull/11734) ([underhood](https://github.com/underhood))
- Add command for new health entity file. [\#11733](https://github.com/netdata/netdata/pull/11733) ([DShreve2](https://github.com/DShreve2))
- Removing dated contact suggestion. [\#11732](https://github.com/netdata/netdata/pull/11732) ([DShreve2](https://github.com/DShreve2))
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
- eBPF Shared Memory system call tracking [\#11560](https://github.com/netdata/netdata/pull/11560) ([UmanShahzad](https://github.com/UmanShahzad))
- Add shared memory to cgroup [\#11559](https://github.com/netdata/netdata/pull/11559) ([thiagoftsm](https://github.com/thiagoftsm))
- End of support for Ubuntu 16.04 [\#11556](https://github.com/netdata/netdata/pull/11556) ([Ferroin](https://github.com/Ferroin))
- Add alert message support for ACLK new architecture [\#11552](https://github.com/netdata/netdata/pull/11552) ([MrZammler](https://github.com/MrZammler))
- Anomaly Detection MVP [\#11548](https://github.com/netdata/netdata/pull/11548) ([vkalintiris](https://github.com/vkalintiris))
- Fix handling of claiming in kickstart script when running as non-root. [\#11507](https://github.com/netdata/netdata/pull/11507) ([Ferroin](https://github.com/Ferroin))
- Added static builds for ARMv7l and ARMv8a [\#11490](https://github.com/netdata/netdata/pull/11490) ([Ferroin](https://github.com/Ferroin))
- Update alarms info [\#11481](https://github.com/netdata/netdata/pull/11481) ([ilyam8](https://github.com/ilyam8))
- Announce proto capability and enable if cloud supports [\#11476](https://github.com/netdata/netdata/pull/11476) ([underhood](https://github.com/underhood))

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
