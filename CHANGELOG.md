# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.1...HEAD)

**Merged pull requests:**

- Use dagger to build and test the agent. [\#16868](https://github.com/netdata/netdata/pull/16868) ([vkalintiris](https://github.com/vkalintiris))
- Local sockets for network namespaces [\#16867](https://github.com/netdata/netdata/pull/16867) ([ktsaou](https://github.com/ktsaou))
- Fix coverity issue [\#16866](https://github.com/netdata/netdata/pull/16866) ([stelfrag](https://github.com/stelfrag))
- update alpine 3.16 fts-dev [\#16865](https://github.com/netdata/netdata/pull/16865) ([ilyam8](https://github.com/ilyam8))
- Remove old mention of save db mode [\#16864](https://github.com/netdata/netdata/pull/16864) ([Ancairon](https://github.com/Ancairon))
- port useful code from incomplete PRs [\#16863](https://github.com/netdata/netdata/pull/16863) ([ktsaou](https://github.com/ktsaou))
- apply the right prototype to instances [\#16862](https://github.com/netdata/netdata/pull/16862) ([ktsaou](https://github.com/ktsaou))
- detect sockets direction [\#16861](https://github.com/netdata/netdata/pull/16861) ([ktsaou](https://github.com/ktsaou))
- add freebsd jail detection to system-info.sh [\#16858](https://github.com/netdata/netdata/pull/16858) ([ilyam8](https://github.com/ilyam8))
- Add ARMv6 static builds. [\#16853](https://github.com/netdata/netdata/pull/16853) ([Ferroin](https://github.com/Ferroin))
- CNCF link fix [\#16851](https://github.com/netdata/netdata/pull/16851) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Disable local Go builds in our RPM packages. [\#16848](https://github.com/netdata/netdata/pull/16848) ([Ferroin](https://github.com/Ferroin))
- Include timer units failed state alert as default [\#16845](https://github.com/netdata/netdata/pull/16845) ([tkatsoulas](https://github.com/tkatsoulas))
- kickstart: use extended-regexp in `get_redirect()` [\#16844](https://github.com/netdata/netdata/pull/16844) ([ilyam8](https://github.com/ilyam8))
- Make the kickstart checksum's placeholder value more concrete [\#16843](https://github.com/netdata/netdata/pull/16843) ([tkatsoulas](https://github.com/tkatsoulas))
- Improve service thread shutdown [\#16841](https://github.com/netdata/netdata/pull/16841) ([stelfrag](https://github.com/stelfrag))
- Update statistics to address slow queries [\#16838](https://github.com/netdata/netdata/pull/16838) ([stelfrag](https://github.com/stelfrag))
- New Permissions System [\#16837](https://github.com/netdata/netdata/pull/16837) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16835](https://github.com/netdata/netdata/pull/16835) ([netdatabot](https://github.com/netdatabot))
- adds docs for cloud MS Teams integration [\#16834](https://github.com/netdata/netdata/pull/16834) ([papazach](https://github.com/papazach))
- Fix coverity issue [\#16831](https://github.com/netdata/netdata/pull/16831) ([stelfrag](https://github.com/stelfrag))
- add brotli and libyaml to buildinfo [\#16830](https://github.com/netdata/netdata/pull/16830) ([ktsaou](https://github.com/ktsaou))
- Fix directory handling in Go toolchain handling script. [\#16828](https://github.com/netdata/netdata/pull/16828) ([Ferroin](https://github.com/Ferroin))
- Change query label matching logic [\#16827](https://github.com/netdata/netdata/pull/16827) ([stelfrag](https://github.com/stelfrag))
- Improve container detection logic for edit-config. [\#16825](https://github.com/netdata/netdata/pull/16825) ([Ferroin](https://github.com/Ferroin))
- Preserve label source during migration [\#16821](https://github.com/netdata/netdata/pull/16821) ([stelfrag](https://github.com/stelfrag))
- Add explicit callback types for readability. [\#16820](https://github.com/netdata/netdata/pull/16820) ([vkalintiris](https://github.com/vkalintiris))
- Add script to ensure a usable Go toolchain is installed. [\#16815](https://github.com/netdata/netdata/pull/16815) ([Ferroin](https://github.com/Ferroin))
- fix verify\_netdata\_host\_prefix log spam [\#16814](https://github.com/netdata/netdata/pull/16814) ([ilyam8](https://github.com/ilyam8))
- use fs type to veryfiy procfs/sysfs [\#16813](https://github.com/netdata/netdata/pull/16813) ([ilyam8](https://github.com/ilyam8))
- updates to light onprem docs [\#16811](https://github.com/netdata/netdata/pull/16811) ([M4itee](https://github.com/M4itee))
- set app\_group label automatically [\#16810](https://github.com/netdata/netdata/pull/16810) ([boxjan](https://github.com/boxjan))
- Apply ASCII-based comparisons to commands in kickstart script that rely on a particular language setting [\#16806](https://github.com/netdata/netdata/pull/16806) ([tkatsoulas](https://github.com/tkatsoulas))
- Remove help text that no longer applies. [\#16805](https://github.com/netdata/netdata/pull/16805) ([vkalintiris](https://github.com/vkalintiris))
- Move mqtt\_websockets under aclk/ [\#16804](https://github.com/netdata/netdata/pull/16804) ([vkalintiris](https://github.com/vkalintiris))
- Fix incorrect major version check in updater. [\#16803](https://github.com/netdata/netdata/pull/16803) ([Ferroin](https://github.com/Ferroin))
- cgroups: containers-vms add CPU throttling % [\#16800](https://github.com/netdata/netdata/pull/16800) ([ilyam8](https://github.com/ilyam8))
- Add additional fail reason and source during database initialization [\#16794](https://github.com/netdata/netdata/pull/16794) ([stelfrag](https://github.com/stelfrag))
- Use original summary for alert transition [\#16793](https://github.com/netdata/netdata/pull/16793) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16792](https://github.com/netdata/netdata/pull/16792) ([netdatabot](https://github.com/netdatabot))
- Update role-based-access.md [\#16791](https://github.com/netdata/netdata/pull/16791) ([vkuznecovas](https://github.com/vkuznecovas))
- Free key and search, replace patterns [\#16789](https://github.com/netdata/netdata/pull/16789) ([stelfrag](https://github.com/stelfrag))
- Use named constants for keyword tokens. [\#16787](https://github.com/netdata/netdata/pull/16787) ([vkalintiris](https://github.com/vkalintiris))
- diskspace: reworked the cleanup to fix race conditions [\#16786](https://github.com/netdata/netdata/pull/16786) ([ktsaou](https://github.com/ktsaou))
- diskspace missing mutex use [\#16784](https://github.com/netdata/netdata/pull/16784) ([ktsaou](https://github.com/ktsaou))
- Remove h2o header from libnetdata [\#16780](https://github.com/netdata/netdata/pull/16780) ([vkalintiris](https://github.com/vkalintiris))
- DYNCFG: dynamically configured alerts [\#16779](https://github.com/netdata/netdata/pull/16779) ([ktsaou](https://github.com/ktsaou))
- Update replication documentation [\#16778](https://github.com/netdata/netdata/pull/16778) ([thiagoftsm](https://github.com/thiagoftsm))
- Update telegram documentation [\#16777](https://github.com/netdata/netdata/pull/16777) ([thiagoftsm](https://github.com/thiagoftsm))
- Delete unused variable. [\#16776](https://github.com/netdata/netdata/pull/16776) ([vkalintiris](https://github.com/vkalintiris))
- Use unsigned char for binary data in mqtt. [\#16775](https://github.com/netdata/netdata/pull/16775) ([vkalintiris](https://github.com/vkalintiris))
- Fix warning. [\#16774](https://github.com/netdata/netdata/pull/16774) ([vkalintiris](https://github.com/vkalintiris))
- fix thread name on fatal and cgroup netdev rename crash [\#16771](https://github.com/netdata/netdata/pull/16771) ([ktsaou](https://github.com/ktsaou))
- allow POST requests to be received from ACLK [\#16770](https://github.com/netdata/netdata/pull/16770) ([ktsaou](https://github.com/ktsaou))
- Keep transaction id of request headers [\#16769](https://github.com/netdata/netdata/pull/16769) ([ktsaou](https://github.com/ktsaou))
- Change default build directory in installer to `build`. [\#16768](https://github.com/netdata/netdata/pull/16768) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues [\#16766](https://github.com/netdata/netdata/pull/16766) ([stelfrag](https://github.com/stelfrag))
- Add missing call for aral\_freez \(eBPF\) [\#16765](https://github.com/netdata/netdata/pull/16765) ([thiagoftsm](https://github.com/thiagoftsm))
- /api/v1/config tree improvements and swagger documentation [\#16764](https://github.com/netdata/netdata/pull/16764) ([ktsaou](https://github.com/ktsaou))
- fix compiler warnings [\#16763](https://github.com/netdata/netdata/pull/16763) ([ktsaou](https://github.com/ktsaou))
- fix cmake \_GNU\_SOURCE warnings [\#16761](https://github.com/netdata/netdata/pull/16761) ([ktsaou](https://github.com/ktsaou))
- fix phtread-detatch\(\) call [\#16760](https://github.com/netdata/netdata/pull/16760) ([ktsaou](https://github.com/ktsaou))
- Fix sanitizer errors [\#16759](https://github.com/netdata/netdata/pull/16759) ([ktsaou](https://github.com/ktsaou))
- report timestamps with progress [\#16758](https://github.com/netdata/netdata/pull/16758) ([ktsaou](https://github.com/ktsaou))
- add schemas to /usr/lib/netdata/conf.d/schema.d [\#16757](https://github.com/netdata/netdata/pull/16757) ([ktsaou](https://github.com/ktsaou))
- Add netdata\_os\_info metric [\#16756](https://github.com/netdata/netdata/pull/16756) ([colinleroy](https://github.com/colinleroy))
- Recursively merge mqtt\_websockets [\#16755](https://github.com/netdata/netdata/pull/16755) ([vkalintiris](https://github.com/vkalintiris))
- packaging: add cap\_dac\_read\_search to go.d.plugin [\#16754](https://github.com/netdata/netdata/pull/16754) ([ilyam8](https://github.com/ilyam8))
- Name storage engine variables consistently. [\#16753](https://github.com/netdata/netdata/pull/16753) ([vkalintiris](https://github.com/vkalintiris))
- minor - fix/update codeowners [\#16750](https://github.com/netdata/netdata/pull/16750) ([underhood](https://github.com/underhood))
- fix cpu per core charts priority [\#16749](https://github.com/netdata/netdata/pull/16749) ([ilyam8](https://github.com/ilyam8))
- Address sanitizer through CMake and use it for unit tests. [\#16748](https://github.com/netdata/netdata/pull/16748) ([vkalintiris](https://github.com/vkalintiris))
- Remove unused file [\#16747](https://github.com/netdata/netdata/pull/16747) ([stelfrag](https://github.com/stelfrag))
- cleanup proc net-dev renames [\#16745](https://github.com/netdata/netdata/pull/16745) ([ktsaou](https://github.com/ktsaou))
- uninstaller: improve removing `netdata` from groups [\#16742](https://github.com/netdata/netdata/pull/16742) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16739](https://github.com/netdata/netdata/pull/16739) ([netdatabot](https://github.com/netdatabot))
- change get kickstart url to https://get.netdata.cloud/kickstart.sh [\#16738](https://github.com/netdata/netdata/pull/16738) ([ilyam8](https://github.com/ilyam8))
- health: add httpcheck bad header alert [\#16736](https://github.com/netdata/netdata/pull/16736) ([ilyam8](https://github.com/ilyam8))
- update default netdata.conf used for native packages [\#16734](https://github.com/netdata/netdata/pull/16734) ([ilyam8](https://github.com/ilyam8))
- fix missing CPU frequency [\#16732](https://github.com/netdata/netdata/pull/16732) ([ilyam8](https://github.com/ilyam8))
- Fix handling of hardening flags with Clang [\#16731](https://github.com/netdata/netdata/pull/16731) ([Ferroin](https://github.com/Ferroin))
- fix excessive "maximum number of cgroups reached" log messages [\#16730](https://github.com/netdata/netdata/pull/16730) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16728](https://github.com/netdata/netdata/pull/16728) ([netdatabot](https://github.com/netdatabot))
- update ebpf-socket function name and columns [\#16727](https://github.com/netdata/netdata/pull/16727) ([ilyam8](https://github.com/ilyam8))
- Fix --distro-override parameter name in docs [\#16726](https://github.com/netdata/netdata/pull/16726) ([moschlar](https://github.com/moschlar))
- update go.d.plugin to v0.58.0 [\#16725](https://github.com/netdata/netdata/pull/16725) ([ilyam8](https://github.com/ilyam8))
- Add GHA workflow to upload kickstart script to our repo server. [\#16724](https://github.com/netdata/netdata/pull/16724) ([Ferroin](https://github.com/Ferroin))
- Add Netdata Mobile App to issue template config [\#16723](https://github.com/netdata/netdata/pull/16723) ([ilyam8](https://github.com/ilyam8))
- fix clock resolution detection [\#16720](https://github.com/netdata/netdata/pull/16720) ([ktsaou](https://github.com/ktsaou))
- cgroups: don't multiply cgroup\_check\_for\_new\_every by update\_every [\#16719](https://github.com/netdata/netdata/pull/16719) ([ilyam8](https://github.com/ilyam8))
- Add info to distros.yml for handling of legacy platforms. [\#16718](https://github.com/netdata/netdata/pull/16718) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#16716](https://github.com/netdata/netdata/pull/16716) ([netdatabot](https://github.com/netdatabot))
- Add the Mobile App notification Integration [\#16715](https://github.com/netdata/netdata/pull/16715) ([sashwathn](https://github.com/sashwathn))
- Update GHA steps that handle artifacts to use latest versions of upload/download actions. [\#16714](https://github.com/netdata/netdata/pull/16714) ([Ferroin](https://github.com/Ferroin))
- CI runtime check cleanup [\#16713](https://github.com/netdata/netdata/pull/16713) ([Ferroin](https://github.com/Ferroin))
- disable logsmanagement when installing on macOS [\#16708](https://github.com/netdata/netdata/pull/16708) ([ilyam8](https://github.com/ilyam8))
- dyncfg v2 [\#16702](https://github.com/netdata/netdata/pull/16702) ([ktsaou](https://github.com/ktsaou))
- delay collecting double linked network interfaces [\#16701](https://github.com/netdata/netdata/pull/16701) ([ilyam8](https://github.com/ilyam8))
- fix quota calculation when the the db is empty [\#16699](https://github.com/netdata/netdata/pull/16699) ([ktsaou](https://github.com/ktsaou))
- disable logsmanagement when installing on macOS [\#16697](https://github.com/netdata/netdata/pull/16697) ([ilyam8](https://github.com/ilyam8))
- Remove Ubuntu 23.04 from the CI [\#16694](https://github.com/netdata/netdata/pull/16694) ([tkatsoulas](https://github.com/tkatsoulas))
- fix installing service file and start/stop ND using `launchctl` on macOS [\#16693](https://github.com/netdata/netdata/pull/16693) ([ilyam8](https://github.com/ilyam8))
- improve the error message when accessing functions [\#16692](https://github.com/netdata/netdata/pull/16692) ([ktsaou](https://github.com/ktsaou))
- cups exit on sigpipe [\#16691](https://github.com/netdata/netdata/pull/16691) ([ilyam8](https://github.com/ilyam8))
- fix minor omission on netdata-installers arguments [\#16690](https://github.com/netdata/netdata/pull/16690) ([tkatsoulas](https://github.com/tkatsoulas))
- Revert "Update artifact-handling actions to latest version." [\#16689](https://github.com/netdata/netdata/pull/16689) ([tkatsoulas](https://github.com/tkatsoulas))
- cmake log2journal netdatacli [\#16688](https://github.com/netdata/netdata/pull/16688) ([ktsaou](https://github.com/ktsaou))
- atomically load the metric reference count [\#16687](https://github.com/netdata/netdata/pull/16687) ([ktsaou](https://github.com/ktsaou))
- fix claiming on macOS [\#16686](https://github.com/netdata/netdata/pull/16686) ([ilyam8](https://github.com/ilyam8))
- add --disable-logsmanagement when building static [\#16684](https://github.com/netdata/netdata/pull/16684) ([ilyam8](https://github.com/ilyam8))
- fix exporting internal charts context and family [\#16683](https://github.com/netdata/netdata/pull/16683) ([ilyam8](https://github.com/ilyam8))
- Fatal relaxation of unknown page types. [\#16682](https://github.com/netdata/netdata/pull/16682) ([vkalintiris](https://github.com/vkalintiris))
- docs: add "Require Cloud" column to functions table [\#16681](https://github.com/netdata/netdata/pull/16681) ([ilyam8](https://github.com/ilyam8))
- cmake missing defines [\#16680](https://github.com/netdata/netdata/pull/16680) ([ktsaou](https://github.com/ktsaou))
- Fix alerts-configuration-manager.md [\#16679](https://github.com/netdata/netdata/pull/16679) ([Ancairon](https://github.com/Ancairon))
- kickstart: dont run install-required-packages.sh as root on macOS [\#16675](https://github.com/netdata/netdata/pull/16675) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.75.2 [\#16674](https://github.com/netdata/netdata/pull/16674) ([ilyam8](https://github.com/ilyam8))
- kickstart: add a note on how to access the UI to the success banner [\#16673](https://github.com/netdata/netdata/pull/16673) ([ilyam8](https://github.com/ilyam8))
- remove contrib/rhel [\#16672](https://github.com/netdata/netdata/pull/16672) ([ilyam8](https://github.com/ilyam8))
- Update binaries \(eBPF\) [\#16671](https://github.com/netdata/netdata/pull/16671) ([thiagoftsm](https://github.com/thiagoftsm))
- eBPF socket \(eBPF\) [\#16669](https://github.com/netdata/netdata/pull/16669) ([thiagoftsm](https://github.com/thiagoftsm))
- fix compiler warnings [\#16665](https://github.com/netdata/netdata/pull/16665) ([ktsaou](https://github.com/ktsaou))
- dont exceed buffer boundaries, when the buffer is empty [\#16664](https://github.com/netdata/netdata/pull/16664) ([ktsaou](https://github.com/ktsaou))
- set log level of too-old-data message to debug  [\#16663](https://github.com/netdata/netdata/pull/16663) ([ilyam8](https://github.com/ilyam8))
- Improve context load  [\#16659](https://github.com/netdata/netdata/pull/16659) ([stelfrag](https://github.com/stelfrag))
- Shutdown dbengine event loop properly [\#16658](https://github.com/netdata/netdata/pull/16658) ([stelfrag](https://github.com/stelfrag))
- docs: Correct chart\_labels summary [\#16656](https://github.com/netdata/netdata/pull/16656) ([sepek](https://github.com/sepek))
- Fix coverity issues [\#16655](https://github.com/netdata/netdata/pull/16655) ([stelfrag](https://github.com/stelfrag))
- Fix overrun in crc32set [\#16654](https://github.com/netdata/netdata/pull/16654) ([stelfrag](https://github.com/stelfrag))
- Necessary changes for Learn [\#16651](https://github.com/netdata/netdata/pull/16651) ([Ancairon](https://github.com/Ancairon))
- docs: add a few examples how to query Netdata logs using journalctl [\#16650](https://github.com/netdata/netdata/pull/16650) ([ilyam8](https://github.com/ilyam8))
- increase max response size to 100MiB [\#16649](https://github.com/netdata/netdata/pull/16649) ([ktsaou](https://github.com/ktsaou))
- rename bundle dashboard scripts [\#16648](https://github.com/netdata/netdata/pull/16648) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.72.0 [\#16647](https://github.com/netdata/netdata/pull/16647) ([ilyam8](https://github.com/ilyam8))
- Fix compilation error when using --disable-dbengine [\#16645](https://github.com/netdata/netdata/pull/16645) ([stelfrag](https://github.com/stelfrag))
- Create alerts-configuration-manager.md [\#16642](https://github.com/netdata/netdata/pull/16642) ([sashwathn](https://github.com/sashwathn))
- Add extra build flags to CMakeLists.txt. [\#16641](https://github.com/netdata/netdata/pull/16641) ([Ferroin](https://github.com/Ferroin))
- Update artifact-handling actions to latest version. [\#16639](https://github.com/netdata/netdata/pull/16639) ([Ferroin](https://github.com/Ferroin))
- cmake: make WEB\_DIR configurable [\#16638](https://github.com/netdata/netdata/pull/16638) ([ilyam8](https://github.com/ilyam8))
- Remove code relying on autotools. [\#16634](https://github.com/netdata/netdata/pull/16634) ([vkalintiris](https://github.com/vkalintiris))
- docs: add "Rootless mode" to Docker install guide [\#16632](https://github.com/netdata/netdata/pull/16632) ([ilyam8](https://github.com/ilyam8))
- Correctly handle basic permissions for most scripts on install. [\#16629](https://github.com/netdata/netdata/pull/16629) ([Ferroin](https://github.com/Ferroin))
- Fix UB of unaligned loads/stores and signed shifts. [\#16628](https://github.com/netdata/netdata/pull/16628) ([vkalintiris](https://github.com/vkalintiris))
- cgroups: filter lxcfs.service/.control [\#16620](https://github.com/netdata/netdata/pull/16620) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues, logically dead code and error checking [\#16618](https://github.com/netdata/netdata/pull/16618) ([stelfrag](https://github.com/stelfrag))
- Added energy efficiency img README.md [\#16617](https://github.com/netdata/netdata/pull/16617) ([Aliki92](https://github.com/Aliki92))
- Fix small coverity issue [\#16616](https://github.com/netdata/netdata/pull/16616) ([stelfrag](https://github.com/stelfrag))
- ndsudo - a helper to run privileged commands [\#16614](https://github.com/netdata/netdata/pull/16614) ([ktsaou](https://github.com/ktsaou))
- Robustness improvements to netdata-updater.sh [\#16613](https://github.com/netdata/netdata/pull/16613) ([candlerb](https://github.com/candlerb))
- Remove assert [\#16611](https://github.com/netdata/netdata/pull/16611) ([stelfrag](https://github.com/stelfrag))
- Remove CPack stuff from CMake [\#16608](https://github.com/netdata/netdata/pull/16608) ([vkalintiris](https://github.com/vkalintiris))
- Remove includes outside of libnetdata. [\#16607](https://github.com/netdata/netdata/pull/16607) ([vkalintiris](https://github.com/vkalintiris))
- Fix \(and improve\) Coverity scanning. [\#16605](https://github.com/netdata/netdata/pull/16605) ([Ferroin](https://github.com/Ferroin))
- Delete memory mode "map" and "save". [\#16604](https://github.com/netdata/netdata/pull/16604) ([vkalintiris](https://github.com/vkalintiris))
- remove v1 dashboard version check from installer [\#16603](https://github.com/netdata/netdata/pull/16603) ([ilyam8](https://github.com/ilyam8))
- fix not assigned proc\_count in installer [\#16602](https://github.com/netdata/netdata/pull/16602) ([ilyam8](https://github.com/ilyam8))
- improve enable\_feature function in the installer [\#16601](https://github.com/netdata/netdata/pull/16601) ([ilyam8](https://github.com/ilyam8))
- Remove build/ [\#16600](https://github.com/netdata/netdata/pull/16600) ([vkalintiris](https://github.com/vkalintiris))
- Allow passing cmake options with NETDATA\_CMAKE\_OPTIONS. [\#16598](https://github.com/netdata/netdata/pull/16598) ([vkalintiris](https://github.com/vkalintiris))
- Cleanup am files [\#16597](https://github.com/netdata/netdata/pull/16597) ([vkalintiris](https://github.com/vkalintiris))
- Fix coverity issues [\#16596](https://github.com/netdata/netdata/pull/16596) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16595](https://github.com/netdata/netdata/pull/16595) ([netdatabot](https://github.com/netdatabot))
- fix: use black version icon for Splunk in order to make it visible [\#16593](https://github.com/netdata/netdata/pull/16593) ([juacker](https://github.com/juacker))
- systemd-journal: exit if unable to locate journal data directories [\#16592](https://github.com/netdata/netdata/pull/16592) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#16589](https://github.com/netdata/netdata/pull/16589) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16587](https://github.com/netdata/netdata/pull/16587) ([netdatabot](https://github.com/netdatabot))
- Adds docs for Splunk cloud notifications [\#16586](https://github.com/netdata/netdata/pull/16586) ([juacker](https://github.com/juacker))
- uninstaller remove log2journal and systemd-cat-native [\#16585](https://github.com/netdata/netdata/pull/16585) ([ilyam8](https://github.com/ilyam8))
- Handle coverity issues related to Y2K38\_SAFETY [\#16583](https://github.com/netdata/netdata/pull/16583) ([stelfrag](https://github.com/stelfrag))
- Update categories.yaml to add Logs [\#16582](https://github.com/netdata/netdata/pull/16582) ([sashwathn](https://github.com/sashwathn))
- Add Alpine Linux 3.19 to CI. [\#16579](https://github.com/netdata/netdata/pull/16579) ([Ferroin](https://github.com/Ferroin))
- Queries Progress [\#16574](https://github.com/netdata/netdata/pull/16574) ([ktsaou](https://github.com/ktsaou))
- disable cpu per core metrics by default [\#16572](https://github.com/netdata/netdata/pull/16572) ([ilyam8](https://github.com/ilyam8))
- make debugfs exit on sigpipe [\#16569](https://github.com/netdata/netdata/pull/16569) ([ilyam8](https://github.com/ilyam8))
- Fix memory leak during host chart label cleanup [\#16568](https://github.com/netdata/netdata/pull/16568) ([stelfrag](https://github.com/stelfrag))
- fix cpu arch/ram/disk values in buildinfo [\#16567](https://github.com/netdata/netdata/pull/16567) ([ilyam8](https://github.com/ilyam8))
- Remove Netdata packages from APT cache when attempting to install. [\#16566](https://github.com/netdata/netdata/pull/16566) ([Ferroin](https://github.com/Ferroin))
- Resolve issue on startup in servers with 1 core [\#16565](https://github.com/netdata/netdata/pull/16565) ([stelfrag](https://github.com/stelfrag))
- Update naming for swagger api [\#16564](https://github.com/netdata/netdata/pull/16564) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix release metadata workflow [\#16563](https://github.com/netdata/netdata/pull/16563) ([tkatsoulas](https://github.com/tkatsoulas))
- Make  the systemd-journal mandatory package on Centos 7  and Amazon linux 2 [\#16562](https://github.com/netdata/netdata/pull/16562) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix for AMD GPU drm different format proc file [\#16561](https://github.com/netdata/netdata/pull/16561) ([MrZammler](https://github.com/MrZammler))
- Revert "remove discourse badge from readme" [\#16560](https://github.com/netdata/netdata/pull/16560) ([ilyam8](https://github.com/ilyam8))
- Change the workflow on how we set the right permissions for perf-plugin [\#16558](https://github.com/netdata/netdata/pull/16558) ([tkatsoulas](https://github.com/tkatsoulas))
- Add README for gorilla [\#16553](https://github.com/netdata/netdata/pull/16553) ([vkalintiris](https://github.com/vkalintiris))
- Bump new version on the release changelog GHA [\#16551](https://github.com/netdata/netdata/pull/16551) ([tkatsoulas](https://github.com/tkatsoulas))
- set "HOME" after switching to netdata user [\#16548](https://github.com/netdata/netdata/pull/16548) ([ilyam8](https://github.com/ilyam8))
- code cleanup [\#16542](https://github.com/netdata/netdata/pull/16542) ([ktsaou](https://github.com/ktsaou))
- Assorted kickstart script fixes. [\#16537](https://github.com/netdata/netdata/pull/16537) ([Ferroin](https://github.com/Ferroin))
- wip documentation about functions table [\#16535](https://github.com/netdata/netdata/pull/16535) ([ktsaou](https://github.com/ktsaou))
- Remove openSUSE 15.4 from CI [\#16449](https://github.com/netdata/netdata/pull/16449) ([tkatsoulas](https://github.com/tkatsoulas))
- Remove fedora 37 from CI [\#16422](https://github.com/netdata/netdata/pull/16422) ([tkatsoulas](https://github.com/tkatsoulas))

## [v1.44.1](https://github.com/netdata/netdata/tree/v1.44.1) (2023-12-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.0...v1.44.1)

## [v1.44.0](https://github.com/netdata/netdata/tree/v1.44.0) (2023-12-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.2...v1.44.0)

**Merged pull requests:**

- update bundled UI to v6.66.1 [\#16554](https://github.com/netdata/netdata/pull/16554) ([ilyam8](https://github.com/ilyam8))
- Improve page validity check during database extent load [\#16552](https://github.com/netdata/netdata/pull/16552) ([stelfrag](https://github.com/stelfrag))
- Proper Learn-friendly links [\#16547](https://github.com/netdata/netdata/pull/16547) ([Ancairon](https://github.com/Ancairon))
- docs required for release [\#16546](https://github.com/netdata/netdata/pull/16546) ([ktsaou](https://github.com/ktsaou))
- Add option to change page type for tier 0 to gorilla [\#16545](https://github.com/netdata/netdata/pull/16545) ([vkalintiris](https://github.com/vkalintiris))
- fix alpine deps [\#16543](https://github.com/netdata/netdata/pull/16543) ([tkatsoulas](https://github.com/tkatsoulas))
- change level to debug "took too long to be updated" [\#16540](https://github.com/netdata/netdata/pull/16540) ([ilyam8](https://github.com/ilyam8))
- apps: fix uptime for groups with 0 processes [\#16538](https://github.com/netdata/netdata/pull/16538) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16536](https://github.com/netdata/netdata/pull/16536) ([netdatabot](https://github.com/netdatabot))
- Reorg kickstart guide's steps [\#16534](https://github.com/netdata/netdata/pull/16534) ([tkatsoulas](https://github.com/tkatsoulas))
- update go.d plugin to v0.57.2 [\#16533](https://github.com/netdata/netdata/pull/16533) ([ilyam8](https://github.com/ilyam8))
- Update getting-started-light-poc.md [\#16532](https://github.com/netdata/netdata/pull/16532) ([M4itee](https://github.com/M4itee))
- Acquire receiver\_lock to to avoid race condition [\#16531](https://github.com/netdata/netdata/pull/16531) ([stelfrag](https://github.com/stelfrag))
- link aclk.log to stdout in docker [\#16529](https://github.com/netdata/netdata/pull/16529) ([ilyam8](https://github.com/ilyam8))
- Update getting-started.md [\#16528](https://github.com/netdata/netdata/pull/16528) ([Ancairon](https://github.com/Ancairon))
- Make image available to Learn + add a category overview page for new … [\#16527](https://github.com/netdata/netdata/pull/16527) ([Ancairon](https://github.com/Ancairon))
- logs-management: Disable logs management monitoring section [\#16525](https://github.com/netdata/netdata/pull/16525) ([Dim-P](https://github.com/Dim-P))
- log method = none is not respected [\#16523](https://github.com/netdata/netdata/pull/16523) ([ktsaou](https://github.com/ktsaou))
- include more cases for megacli degraded state [\#16522](https://github.com/netdata/netdata/pull/16522) ([ClaraCrazy](https://github.com/ClaraCrazy))
- update bundled UI to v6.65.0 [\#16520](https://github.com/netdata/netdata/pull/16520) ([ilyam8](https://github.com/ilyam8))
- log2journal improvements 5 [\#16519](https://github.com/netdata/netdata/pull/16519) ([ktsaou](https://github.com/ktsaou))
- change log level to debug for dbengine routine operations on start [\#16518](https://github.com/netdata/netdata/pull/16518) ([ilyam8](https://github.com/ilyam8))
- remove system info logging [\#16517](https://github.com/netdata/netdata/pull/16517) ([ilyam8](https://github.com/ilyam8))
- python.d: logger: remove timestamp when logging to journald. [\#16516](https://github.com/netdata/netdata/pull/16516) ([ilyam8](https://github.com/ilyam8))
- python.d: mute stock jobs logging during check\(\) [\#16515](https://github.com/netdata/netdata/pull/16515) ([ilyam8](https://github.com/ilyam8))
- logs-management: Add prefix to chart names [\#16514](https://github.com/netdata/netdata/pull/16514) ([Dim-P](https://github.com/Dim-P))
- docs: add with-systemd-units-monitoring example to docker [\#16513](https://github.com/netdata/netdata/pull/16513) ([ilyam8](https://github.com/ilyam8))
- apps: fix "has aggregated" debug output [\#16512](https://github.com/netdata/netdata/pull/16512) ([ilyam8](https://github.com/ilyam8))
- log2journal improvements 4 [\#16510](https://github.com/netdata/netdata/pull/16510) ([ktsaou](https://github.com/ktsaou))
- journal improvements part 3 [\#16509](https://github.com/netdata/netdata/pull/16509) ([ktsaou](https://github.com/ktsaou))
- convert some error messages to info [\#16508](https://github.com/netdata/netdata/pull/16508) ([ilyam8](https://github.com/ilyam8))
- Resolve coverity issue 410232 [\#16507](https://github.com/netdata/netdata/pull/16507) ([stelfrag](https://github.com/stelfrag))
- convert some error messages to info [\#16505](https://github.com/netdata/netdata/pull/16505) ([ilyam8](https://github.com/ilyam8))
- diskspace/diskstats: don't create runtime disk config by default [\#16503](https://github.com/netdata/netdata/pull/16503) ([ilyam8](https://github.com/ilyam8))
- Fix CID 410152 Dereference after null check [\#16502](https://github.com/netdata/netdata/pull/16502) ([stelfrag](https://github.com/stelfrag))
- proc\_net\_dev: don't create runtime device config by default [\#16501](https://github.com/netdata/netdata/pull/16501) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16500](https://github.com/netdata/netdata/pull/16500) ([netdatabot](https://github.com/netdatabot))
- remove discourse badge from readme [\#16499](https://github.com/netdata/netdata/pull/16499) ([ilyam8](https://github.com/ilyam8))
- add curl example to create\_netdata\_conf\(\) [\#16498](https://github.com/netdata/netdata/pull/16498) ([ilyam8](https://github.com/ilyam8))
- add /var/log mount to docker [\#16496](https://github.com/netdata/netdata/pull/16496) ([ilyam8](https://github.com/ilyam8))
- Fix occasional shutdown deadlock [\#16495](https://github.com/netdata/netdata/pull/16495) ([stelfrag](https://github.com/stelfrag))
- Log2journal improvements part2 [\#16494](https://github.com/netdata/netdata/pull/16494) ([ktsaou](https://github.com/ktsaou))
- proc\_net\_dev: remove device config section [\#16492](https://github.com/netdata/netdata/pull/16492) ([ilyam8](https://github.com/ilyam8))
- Spelling fixes to documentation [\#16490](https://github.com/netdata/netdata/pull/16490) ([M4itee](https://github.com/M4itee))
- Fix builds on macOS due to missing endianness functions [\#16489](https://github.com/netdata/netdata/pull/16489) ([vkalintiris](https://github.com/vkalintiris))
- log2journal: added missing yaml elements [\#16488](https://github.com/netdata/netdata/pull/16488) ([ktsaou](https://github.com/ktsaou))
- When unregistering an ephemeral host, delete its chart labels [\#16486](https://github.com/netdata/netdata/pull/16486) ([stelfrag](https://github.com/stelfrag))
- logs-management: Add option to submit logs to system journal [\#16485](https://github.com/netdata/netdata/pull/16485) ([Dim-P](https://github.com/Dim-P))
- logs-management: Add function cancellability [\#16484](https://github.com/netdata/netdata/pull/16484) ([Dim-P](https://github.com/Dim-P))
- Fix incorrect DEB package build dep. [\#16483](https://github.com/netdata/netdata/pull/16483) ([Ferroin](https://github.com/Ferroin))
- Bump new version to cov-analysis tool [\#16482](https://github.com/netdata/netdata/pull/16482) ([tkatsoulas](https://github.com/tkatsoulas))
- log2journal moved to collectors [\#16481](https://github.com/netdata/netdata/pull/16481) ([ktsaou](https://github.com/ktsaou))
- Disable netdata monitoring section by default [\#16480](https://github.com/netdata/netdata/pull/16480) ([MrZammler](https://github.com/MrZammler))
- Log2journal yaml configuration support [\#16479](https://github.com/netdata/netdata/pull/16479) ([ktsaou](https://github.com/ktsaou))
- log alarm notifications to health.log [\#16476](https://github.com/netdata/netdata/pull/16476) ([ktsaou](https://github.com/ktsaou))
- journals management improvements [\#16475](https://github.com/netdata/netdata/pull/16475) ([ktsaou](https://github.com/ktsaou))
- SEO changes for Collector names [\#16473](https://github.com/netdata/netdata/pull/16473) ([sashwathn](https://github.com/sashwathn))
- Check context post processing queue before sending status to cloud [\#16472](https://github.com/netdata/netdata/pull/16472) ([stelfrag](https://github.com/stelfrag))
- fix charts.d plugin loading configuration [\#16471](https://github.com/netdata/netdata/pull/16471) ([ilyam8](https://github.com/ilyam8))
- Fix error limit to respect the log every [\#16469](https://github.com/netdata/netdata/pull/16469) ([stelfrag](https://github.com/stelfrag))
- Journal better estimations and watcher [\#16467](https://github.com/netdata/netdata/pull/16467) ([ktsaou](https://github.com/ktsaou))
- update go.d plugin version to v0.57.1 [\#16465](https://github.com/netdata/netdata/pull/16465) ([ilyam8](https://github.com/ilyam8))
- Add option to disable ML. [\#16463](https://github.com/netdata/netdata/pull/16463) ([vkalintiris](https://github.com/vkalintiris))
- fix analytics logs [\#16462](https://github.com/netdata/netdata/pull/16462) ([ktsaou](https://github.com/ktsaou))
- fix logs bashism [\#16461](https://github.com/netdata/netdata/pull/16461) ([ktsaou](https://github.com/ktsaou))
- fix log2journal incorrect log [\#16460](https://github.com/netdata/netdata/pull/16460) ([ktsaou](https://github.com/ktsaou))
- fixes for logging [\#16459](https://github.com/netdata/netdata/pull/16459) ([ktsaou](https://github.com/ktsaou))
- when the namespace socket does not work, continue trying [\#16458](https://github.com/netdata/netdata/pull/16458) ([ktsaou](https://github.com/ktsaou))
- set journal path for logging [\#16457](https://github.com/netdata/netdata/pull/16457) ([ktsaou](https://github.com/ktsaou))
- add sbindir\_POST to PATH of bash scripts that use `systemd-cat-native` [\#16456](https://github.com/netdata/netdata/pull/16456) ([ilyam8](https://github.com/ilyam8))
- add LogNamespace to systemd units [\#16454](https://github.com/netdata/netdata/pull/16454) ([ilyam8](https://github.com/ilyam8))
- Update non-zero uuid key + child conf. [\#16452](https://github.com/netdata/netdata/pull/16452) ([vkalintiris](https://github.com/vkalintiris))
- Add missing argument. [\#16451](https://github.com/netdata/netdata/pull/16451) ([vkalintiris](https://github.com/vkalintiris))
- log flood protection to 1000 log lines / 1 minute [\#16450](https://github.com/netdata/netdata/pull/16450) ([ilyam8](https://github.com/ilyam8))
- Code cleanup [\#16448](https://github.com/netdata/netdata/pull/16448) ([stelfrag](https://github.com/stelfrag))
- fix: link daemon.log to stderr in docker [\#16447](https://github.com/netdata/netdata/pull/16447) ([ilyam8](https://github.com/ilyam8))
- Doc change: Curl no longer supports spaces in the URL. [\#16446](https://github.com/netdata/netdata/pull/16446) ([luisj1983](https://github.com/luisj1983))
- journal estimations [\#16445](https://github.com/netdata/netdata/pull/16445) ([ktsaou](https://github.com/ktsaou))
- journal startup [\#16443](https://github.com/netdata/netdata/pull/16443) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16442](https://github.com/netdata/netdata/pull/16442) ([netdatabot](https://github.com/netdatabot))
- Fix icon filename  [\#16441](https://github.com/netdata/netdata/pull/16441) ([shyamvalsan](https://github.com/shyamvalsan))
- On-Prem documentation full and light [\#16440](https://github.com/netdata/netdata/pull/16440) ([M4itee](https://github.com/M4itee))
- Minor: Small health docs typo fix [\#16439](https://github.com/netdata/netdata/pull/16439) ([MrZammler](https://github.com/MrZammler))
- Removes Observabilitycon banner README.md [\#16434](https://github.com/netdata/netdata/pull/16434) ([Aliki92](https://github.com/Aliki92))
- Journal sampling [\#16433](https://github.com/netdata/netdata/pull/16433) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16431](https://github.com/netdata/netdata/pull/16431) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#16430](https://github.com/netdata/netdata/pull/16430) ([netdatabot](https://github.com/netdatabot))
- proc\_net\_dev: keep nic\_speed\_max in kilobits [\#16429](https://github.com/netdata/netdata/pull/16429) ([ilyam8](https://github.com/ilyam8))
- update go.d plugin to v0.57.0 [\#16427](https://github.com/netdata/netdata/pull/16427) ([ilyam8](https://github.com/ilyam8))
- Adds config info for Telegram cloud notification [\#16424](https://github.com/netdata/netdata/pull/16424) ([juacker](https://github.com/juacker))
- Minor: Remove backtick from doc [\#16423](https://github.com/netdata/netdata/pull/16423) ([MrZammler](https://github.com/MrZammler))
- Update netdata-functions.md [\#16421](https://github.com/netdata/netdata/pull/16421) ([shyamvalsan](https://github.com/shyamvalsan))
- disable socket port reuse [\#16420](https://github.com/netdata/netdata/pull/16420) ([ilyam8](https://github.com/ilyam8))
- fix proc net dev: keep iface speed chart var in Mbits [\#16418](https://github.com/netdata/netdata/pull/16418) ([ilyam8](https://github.com/ilyam8))
- Don't print errors from reading filtered alerts [\#16417](https://github.com/netdata/netdata/pull/16417) ([MrZammler](https://github.com/MrZammler))
- /api/v1/charts: bring back chart id to `title` [\#16416](https://github.com/netdata/netdata/pull/16416) ([ilyam8](https://github.com/ilyam8))
- fix: don't count reused connections as new [\#16414](https://github.com/netdata/netdata/pull/16414) ([ilyam8](https://github.com/ilyam8))
- Add support for installing a specific major version of the agent on install. [\#16413](https://github.com/netdata/netdata/pull/16413) ([Ferroin](https://github.com/Ferroin))
- Remove queue limit from ACLK sync event loop [\#16411](https://github.com/netdata/netdata/pull/16411) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16409](https://github.com/netdata/netdata/pull/16409) ([netdatabot](https://github.com/netdatabot))
- Improve handling around EPEL requirement for RPM packages. [\#16406](https://github.com/netdata/netdata/pull/16406) ([Ferroin](https://github.com/Ferroin))
- Fix typo in metadata \(eBPF\) [\#16405](https://github.com/netdata/netdata/pull/16405) ([thiagoftsm](https://github.com/thiagoftsm))
- docker: use /host/etc/hostname if mounted [\#16401](https://github.com/netdata/netdata/pull/16401) ([ilyam8](https://github.com/ilyam8))
- adaptec\_raid: fix parsing PD without NCQ status [\#16400](https://github.com/netdata/netdata/pull/16400) ([ilyam8](https://github.com/ilyam8))
- eBPF apps order [\#16395](https://github.com/netdata/netdata/pull/16395) ([thiagoftsm](https://github.com/thiagoftsm))
- fix systemd-units func expiration time [\#16393](https://github.com/netdata/netdata/pull/16393) ([ilyam8](https://github.com/ilyam8))
- docker: mount /etc/localtime [\#16392](https://github.com/netdata/netdata/pull/16392) ([ilyam8](https://github.com/ilyam8))
- fix "differ in signedness" warn in cgroup [\#16391](https://github.com/netdata/netdata/pull/16391) ([ilyam8](https://github.com/ilyam8))
- fix v0 dashboard [\#16389](https://github.com/netdata/netdata/pull/16389) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16386](https://github.com/netdata/netdata/pull/16386) ([netdatabot](https://github.com/netdatabot))
- skip spaces when reading cpuset [\#16385](https://github.com/netdata/netdata/pull/16385) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16384](https://github.com/netdata/netdata/pull/16384) ([netdatabot](https://github.com/netdatabot))
- use pre-configured message\_ids to identify common logs [\#16383](https://github.com/netdata/netdata/pull/16383) ([ktsaou](https://github.com/ktsaou))
- Handle ephemeral hosts [\#16381](https://github.com/netdata/netdata/pull/16381) ([stelfrag](https://github.com/stelfrag))
- docs: remove 'families' from health reference [\#16380](https://github.com/netdata/netdata/pull/16380) ([ilyam8](https://github.com/ilyam8))
- fix cloud aws sns notification meta [\#16379](https://github.com/netdata/netdata/pull/16379) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16378](https://github.com/netdata/netdata/pull/16378) ([netdatabot](https://github.com/netdatabot))
- update bundled UI to v6.59.0 [\#16377](https://github.com/netdata/netdata/pull/16377) ([ilyam8](https://github.com/ilyam8))
- health guides: remove guides for alerts that don't exist in the repo [\#16375](https://github.com/netdata/netdata/pull/16375) ([ilyam8](https://github.com/ilyam8))
- add pids current to cgroups meta [\#16374](https://github.com/netdata/netdata/pull/16374) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16373](https://github.com/netdata/netdata/pull/16373) ([netdatabot](https://github.com/netdatabot))
- docs: remove unused cloud notification methods mds [\#16372](https://github.com/netdata/netdata/pull/16372) ([ilyam8](https://github.com/ilyam8))
- Add configuration documentation for Cloud AWS SNS [\#16371](https://github.com/netdata/netdata/pull/16371) ([car12o](https://github.com/car12o))
- pacakging: add zstd dev to install-required-packages [\#16370](https://github.com/netdata/netdata/pull/16370) ([ilyam8](https://github.com/ilyam8))
- cgroups: collect pids/pids.current [\#16369](https://github.com/netdata/netdata/pull/16369) ([ilyam8](https://github.com/ilyam8))
- docs: Correct time unit for tier 2 explanation [\#16368](https://github.com/netdata/netdata/pull/16368) ([sepek](https://github.com/sepek))
- cgroups: fix throttle\_duration chart context [\#16367](https://github.com/netdata/netdata/pull/16367) ([ilyam8](https://github.com/ilyam8))
- Introduce agent release metadata pipelines [\#16366](https://github.com/netdata/netdata/pull/16366) ([tkatsoulas](https://github.com/tkatsoulas))
- fix system.net when inside lxc  [\#16364](https://github.com/netdata/netdata/pull/16364) ([ilyam8](https://github.com/ilyam8))
- collectors/freeipmi: add ipmi-sensors function [\#16363](https://github.com/netdata/netdata/pull/16363) ([ilyam8](https://github.com/ilyam8))
- Add assorted improvements to the version policy draft. [\#16362](https://github.com/netdata/netdata/pull/16362) ([Ferroin](https://github.com/Ferroin))
- Add a apcupsd status code metric [\#16361](https://github.com/netdata/netdata/pull/16361) ([thomasbeaudry](https://github.com/thomasbeaudry))
- Switch alarm\_log to use the buffer json functions [\#16360](https://github.com/netdata/netdata/pull/16360) ([stelfrag](https://github.com/stelfrag))
- Switch charts / chart to use buffer json functions [\#16359](https://github.com/netdata/netdata/pull/16359) ([stelfrag](https://github.com/stelfrag))
- health: put guides into subdirs [\#16358](https://github.com/netdata/netdata/pull/16358) ([ilyam8](https://github.com/ilyam8))
- New logging layer [\#16357](https://github.com/netdata/netdata/pull/16357) ([ktsaou](https://github.com/ktsaou))
- Import alert guides from Netdata Assistant [\#16355](https://github.com/netdata/netdata/pull/16355) ([ralphm](https://github.com/ralphm))
- update bundle UI to v6.58.5 [\#16354](https://github.com/netdata/netdata/pull/16354) ([ilyam8](https://github.com/ilyam8))
- Update CODEOWNERS [\#16353](https://github.com/netdata/netdata/pull/16353) ([Ancairon](https://github.com/Ancairon))
- Copy outdated alert guides to health/guides [\#16352](https://github.com/netdata/netdata/pull/16352) ([Ancairon](https://github.com/Ancairon))
- Replace rrdset\_is\_obsolete & rrdset\_isnot\_obsolete [\#16351](https://github.com/netdata/netdata/pull/16351) ([MrZammler](https://github.com/MrZammler))
- fix zstd in static build [\#16349](https://github.com/netdata/netdata/pull/16349) ([ilyam8](https://github.com/ilyam8))
- add rrddim\_get\_last\_stored\_value to simplify function code in internal collectors [\#16348](https://github.com/netdata/netdata/pull/16348) ([ilyam8](https://github.com/ilyam8))
- change defaults for functions [\#16347](https://github.com/netdata/netdata/pull/16347) ([ktsaou](https://github.com/ktsaou))
- give the streaming function to nightly users [\#16346](https://github.com/netdata/netdata/pull/16346) ([ktsaou](https://github.com/ktsaou))
- diskspace: add mount-points function [\#16345](https://github.com/netdata/netdata/pull/16345) ([ilyam8](https://github.com/ilyam8))
- Update packaging instructions [\#16344](https://github.com/netdata/netdata/pull/16344) ([tkatsoulas](https://github.com/tkatsoulas))
- Better database corruption detention during runtime [\#16343](https://github.com/netdata/netdata/pull/16343) ([stelfrag](https://github.com/stelfrag))
- Improve agent to cloud status update process [\#16342](https://github.com/netdata/netdata/pull/16342) ([stelfrag](https://github.com/stelfrag))
- h2o add api/v2 support [\#16340](https://github.com/netdata/netdata/pull/16340) ([underhood](https://github.com/underhood))
- proc/diskstats: add block-devices function [\#16338](https://github.com/netdata/netdata/pull/16338) ([ilyam8](https://github.com/ilyam8))
- network-interfaces function: add UsedBy field to  [\#16337](https://github.com/netdata/netdata/pull/16337) ([ilyam8](https://github.com/ilyam8))
- Network-interfaces function small improvements [\#16336](https://github.com/netdata/netdata/pull/16336) ([ilyam8](https://github.com/ilyam8))
- proc netstat: add network interface statistics function [\#16334](https://github.com/netdata/netdata/pull/16334) ([ilyam8](https://github.com/ilyam8))
- systemd-units improvements [\#16333](https://github.com/netdata/netdata/pull/16333) ([ktsaou](https://github.com/ktsaou))
- cleanup systemd unit files After [\#16332](https://github.com/netdata/netdata/pull/16332) ([ilyam8](https://github.com/ilyam8))
- fix: check for null rrdim in cgroup functions [\#16331](https://github.com/netdata/netdata/pull/16331) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16330](https://github.com/netdata/netdata/pull/16330) ([netdatabot](https://github.com/netdatabot))
- Improve unittests [\#16329](https://github.com/netdata/netdata/pull/16329) ([stelfrag](https://github.com/stelfrag))
- fix coverity warnings in cgroups [\#16328](https://github.com/netdata/netdata/pull/16328) ([ilyam8](https://github.com/ilyam8))
- Fix readme images [\#16327](https://github.com/netdata/netdata/pull/16327) ([Ancairon](https://github.com/Ancairon))
- integrations: fix nightly tag in helm deploy [\#16326](https://github.com/netdata/netdata/pull/16326) ([ilyam8](https://github.com/ilyam8))
- rename newly added functions [\#16325](https://github.com/netdata/netdata/pull/16325) ([ktsaou](https://github.com/ktsaou))
- Added section Blog posts README.md [\#16323](https://github.com/netdata/netdata/pull/16323) ([Aliki92](https://github.com/Aliki92))
- Keep precompiled statements for alarm log queries to improve performance [\#16321](https://github.com/netdata/netdata/pull/16321) ([stelfrag](https://github.com/stelfrag))
- Fix README images [\#16320](https://github.com/netdata/netdata/pull/16320) ([Ancairon](https://github.com/Ancairon))
- Fix journal file index when collision is detected [\#16319](https://github.com/netdata/netdata/pull/16319) ([stelfrag](https://github.com/stelfrag))
- Systemd units function [\#16318](https://github.com/netdata/netdata/pull/16318) ([ktsaou](https://github.com/ktsaou))
- Optimize database before agent shutdown [\#16317](https://github.com/netdata/netdata/pull/16317) ([stelfrag](https://github.com/stelfrag))
- `tcp_v6_connect` monitoring [\#16316](https://github.com/netdata/netdata/pull/16316) ([thiagoftsm](https://github.com/thiagoftsm))
- Improve shutdown when collectors are active [\#16315](https://github.com/netdata/netdata/pull/16315) ([stelfrag](https://github.com/stelfrag))
- cgroup-top function [\#16314](https://github.com/netdata/netdata/pull/16314) ([ktsaou](https://github.com/ktsaou))
- Add a note for the docker deployment alongside with cetus [\#16312](https://github.com/netdata/netdata/pull/16312) ([tkatsoulas](https://github.com/tkatsoulas))
- Update ObservabilityCon README.md [\#16311](https://github.com/netdata/netdata/pull/16311) ([Aliki92](https://github.com/Aliki92))
- update docker swarm deploy info [\#16308](https://github.com/netdata/netdata/pull/16308) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16306](https://github.com/netdata/netdata/pull/16306) ([netdatabot](https://github.com/netdatabot))
- Use proper icons for deploy integrations [\#16305](https://github.com/netdata/netdata/pull/16305) ([Ancairon](https://github.com/Ancairon))
- bump openssl for static in 3.1.4 [\#16303](https://github.com/netdata/netdata/pull/16303) ([tkatsoulas](https://github.com/tkatsoulas))
- claim.sh: use echo instead of /bin/echo [\#16300](https://github.com/netdata/netdata/pull/16300) ([ilyam8](https://github.com/ilyam8))
- update journal sources once per minute [\#16298](https://github.com/netdata/netdata/pull/16298) ([ktsaou](https://github.com/ktsaou))
- Fix label copy [\#16297](https://github.com/netdata/netdata/pull/16297) ([stelfrag](https://github.com/stelfrag))
- fix missing labels from parents [\#16296](https://github.com/netdata/netdata/pull/16296) ([ktsaou](https://github.com/ktsaou))
- do not propagate upstream internal label sources [\#16295](https://github.com/netdata/netdata/pull/16295) ([ktsaou](https://github.com/ktsaou))
- fix various issues identified by coverity [\#16294](https://github.com/netdata/netdata/pull/16294) ([ktsaou](https://github.com/ktsaou))
- fix missing labels from parents [\#16293](https://github.com/netdata/netdata/pull/16293) ([ktsaou](https://github.com/ktsaou))
- fix renames in freebsd [\#16292](https://github.com/netdata/netdata/pull/16292) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16291](https://github.com/netdata/netdata/pull/16291) ([netdatabot](https://github.com/netdatabot))
- fix retention loading [\#16290](https://github.com/netdata/netdata/pull/16290) ([ktsaou](https://github.com/ktsaou))
- integrations: yes/no instead of True/False in tables [\#16289](https://github.com/netdata/netdata/pull/16289) ([ilyam8](https://github.com/ilyam8))
- typo fixed in gen\_docs\_integrations.py [\#16288](https://github.com/netdata/netdata/pull/16288) ([khalid586](https://github.com/khalid586))
- Brotli streaming compression [\#16287](https://github.com/netdata/netdata/pull/16287) ([ktsaou](https://github.com/ktsaou))
- Apcupsd selftest metric [\#16286](https://github.com/netdata/netdata/pull/16286) ([thomasbeaudry](https://github.com/thomasbeaudry))

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
