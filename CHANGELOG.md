# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.22.1...HEAD)

**Merged pull requests:**

- Remove Gentoo from CI [\#9327](https://github.com/netdata/netdata/pull/9327) ([prologic](https://github.com/prologic))
- Fixed invalid memory access [\#9326](https://github.com/netdata/netdata/pull/9326) ([stelfrag](https://github.com/stelfrag))
- Add support for persistent metadata [\#9324](https://github.com/netdata/netdata/pull/9324) ([stelfrag](https://github.com/stelfrag))
- Change streaming terminology to parent-child in the code [\#9323](https://github.com/netdata/netdata/pull/9323) ([amoss](https://github.com/amoss))
- Fix check for remote write header in unit tests [\#9318](https://github.com/netdata/netdata/pull/9318) ([vlvkobal](https://github.com/vlvkobal))
- Adds missing files for streaming changes into cmake build [\#9316](https://github.com/netdata/netdata/pull/9316) ([underhood](https://github.com/underhood))
- apps\_groups.conf: add agent-service-discovery [\#9315](https://github.com/netdata/netdata/pull/9315) ([ilyam8](https://github.com/ilyam8))
- Change streaming terminology to parent/child in docs [\#9312](https://github.com/netdata/netdata/pull/9312) ([joelhans](https://github.com/joelhans))
- Added dummy `--enable-ebpf` flag to avoid breaking updates. [\#9310](https://github.com/netdata/netdata/pull/9310) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.19.1 [\#9309](https://github.com/netdata/netdata/pull/9309) ([ilyam8](https://github.com/ilyam8))
- Correct the repo in the docs for CentOS 8. [\#9308](https://github.com/netdata/netdata/pull/9308) ([Ferroin](https://github.com/Ferroin))
- Fix consistency of kubernetes cgroup names [\#9303](https://github.com/netdata/netdata/pull/9303) ([cakrit](https://github.com/cakrit))
- Fix remote write HTTP header [\#9302](https://github.com/netdata/netdata/pull/9302) ([vlvkobal](https://github.com/vlvkobal))
- minor copy edits [\#9298](https://github.com/netdata/netdata/pull/9298) ([MeganBishopMoore](https://github.com/MeganBishopMoore))
- Fix crash in \#9291 [\#9297](https://github.com/netdata/netdata/pull/9297) ([amoss](https://github.com/amoss))
- Add frontmatter to Matrix notifications doc [\#9295](https://github.com/netdata/netdata/pull/9295) ([joelhans](https://github.com/joelhans))
- installer: update go.d.plugin version to v0.19.0 [\#9294](https://github.com/netdata/netdata/pull/9294) ([ilyam8](https://github.com/ilyam8))
- dashboard\_info.js: ebpf: fix close code block [\#9293](https://github.com/netdata/netdata/pull/9293) ([ilyam8](https://github.com/ilyam8))
- Add guide to exporting metrics to Graphite [\#9285](https://github.com/netdata/netdata/pull/9285) ([joelhans](https://github.com/joelhans))
- update\_apps\_groups: Bring imunify and lsphp to apps groups [\#9284](https://github.com/netdata/netdata/pull/9284) ([thiagoftsm](https://github.com/thiagoftsm))
- Adds metrics for ACLK performance and status [\#9269](https://github.com/netdata/netdata/pull/9269) ([underhood](https://github.com/underhood))
- Fix Coverity defects 359164, 359165 and 358989. [\#9268](https://github.com/netdata/netdata/pull/9268) ([amoss](https://github.com/amoss))
- Move/refactor docs to accomodate new Guides section on Learn [\#9266](https://github.com/netdata/netdata/pull/9266) ([joelhans](https://github.com/joelhans))
- Cleanup of main README and registry doc [\#9265](https://github.com/netdata/netdata/pull/9265) ([joelhans](https://github.com/joelhans))
- Fixed handling of OpenSSL on CentOS/RHEL by bundling a static copy and selecting a configuration directory at install time. [\#9263](https://github.com/netdata/netdata/pull/9263) ([Ferroin](https://github.com/Ferroin))
- Fix frontmatter in circular\_buffer README [\#9262](https://github.com/netdata/netdata/pull/9262) ([joelhans](https://github.com/joelhans))
- Fixes documentation ambiguity leading into issue \#8239 [\#9255](https://github.com/netdata/netdata/pull/9255) ([underhood](https://github.com/underhood))
- Add new exporting "home base" document [\#9246](https://github.com/netdata/netdata/pull/9246) ([joelhans](https://github.com/joelhans))
- Add a random offset to the update script when running non-interactively. [\#9245](https://github.com/netdata/netdata/pull/9245) ([Ferroin](https://github.com/Ferroin))
- Add CI check for building against LibreSSL [\#9216](https://github.com/netdata/netdata/pull/9216) ([prologic](https://github.com/prologic))
- Gap-detection and slew [\#9214](https://github.com/netdata/netdata/pull/9214) ([amoss](https://github.com/amoss))
- Update eBPF to use kernel-collector version 0.4.0. [\#9212](https://github.com/netdata/netdata/pull/9212) ([Ferroin](https://github.com/Ferroin))
- Add link to kernel docs for ftrace [\#9211](https://github.com/netdata/netdata/pull/9211) ([Steve8291](https://github.com/Steve8291))
- fix small typo [\#9205](https://github.com/netdata/netdata/pull/9205) ([Steve8291](https://github.com/Steve8291))
- Update apps.plugin documentation and dashboard.info [\#9199](https://github.com/netdata/netdata/pull/9199) ([thiagoftsm](https://github.com/thiagoftsm))
- fix compilation for older systems [\#9198](https://github.com/netdata/netdata/pull/9198) ([ktsaou](https://github.com/ktsaou))
- Support for matrix notifications [\#9196](https://github.com/netdata/netdata/pull/9196) ([okias](https://github.com/okias))
- Clean type\_name in exporting connector instance configuration [\#9188](https://github.com/netdata/netdata/pull/9188) ([vlvkobal](https://github.com/vlvkobal))
- Fixed cmake build affected by \#9074 [\#9186](https://github.com/netdata/netdata/pull/9186) ([stelfrag](https://github.com/stelfrag))
- Fix exporting unit tests [\#9183](https://github.com/netdata/netdata/pull/9183) ([vlvkobal](https://github.com/vlvkobal))
- Fix missing ebpf packaging files from dist archive [\#9182](https://github.com/netdata/netdata/pull/9182) ([prologic](https://github.com/prologic))
- cov\_358988: Remove coverity bug [\#9180](https://github.com/netdata/netdata/pull/9180) ([thiagoftsm](https://github.com/thiagoftsm))
- Integration between eBPF and Apps [\#9178](https://github.com/netdata/netdata/pull/9178) ([thiagoftsm](https://github.com/thiagoftsm))
- Improve dbengine docs for streaming setups [\#9177](https://github.com/netdata/netdata/pull/9177) ([joelhans](https://github.com/joelhans))
- Really prevent overwriting netdata.conf on static installs. [\#9174](https://github.com/netdata/netdata/pull/9174) ([Ferroin](https://github.com/Ferroin))
- Remove knatsakis and ncmans from CODEOWNERS for the agent. [\#9173](https://github.com/netdata/netdata/pull/9173) ([Ferroin](https://github.com/Ferroin))
- Added health check functionality to our Docker images. [\#9172](https://github.com/netdata/netdata/pull/9172) ([Ferroin](https://github.com/Ferroin))
- Remove the experimental label from the exporting engine documentation [\#9171](https://github.com/netdata/netdata/pull/9171) ([vlvkobal](https://github.com/vlvkobal))
- Fix reliability of kickstart/kickstart-static64 with checksums sometimes failing [\#9165](https://github.com/netdata/netdata/pull/9165) ([prologic](https://github.com/prologic))
- Revert "Introduce a random sleep in the Netdata updater" [\#9161](https://github.com/netdata/netdata/pull/9161) ([prologic](https://github.com/prologic))
- Fixed bug in accepting empty lines in parser [\#9158](https://github.com/netdata/netdata/pull/9158) ([stelfrag](https://github.com/stelfrag))
- Fixed coverity warning \(CID 358971\) [\#9157](https://github.com/netdata/netdata/pull/9157) ([stelfrag](https://github.com/stelfrag))
- Update README.md [\#9151](https://github.com/netdata/netdata/pull/9151) ([stephenrauch](https://github.com/stephenrauch))
- fix typo in step-03.md [\#9150](https://github.com/netdata/netdata/pull/9150) ([waybeforenow](https://github.com/waybeforenow))
- eBPF modular [\#9148](https://github.com/netdata/netdata/pull/9148) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix error \> emerge openssl-devel [\#9141](https://github.com/netdata/netdata/pull/9141) ([vsc55](https://github.com/vsc55))
- Revert "Fix macOS builds building and linking against openssl" [\#9137](https://github.com/netdata/netdata/pull/9137) ([prologic](https://github.com/prologic))
- Fix docs CI to handle absolute links between docs [\#9132](https://github.com/netdata/netdata/pull/9132) ([joelhans](https://github.com/joelhans))
- Check update interval for exporting connector instance [\#9131](https://github.com/netdata/netdata/pull/9131) ([vlvkobal](https://github.com/vlvkobal))
- Add CI for our Static Netdata builds \(which kickstart-static64 uses\) [\#9130](https://github.com/netdata/netdata/pull/9130) ([prologic](https://github.com/prologic))
- Fix paths to trigger docs CI workflow [\#9128](https://github.com/netdata/netdata/pull/9128) ([joelhans](https://github.com/joelhans))
- Remove the unused Dockerfile.docs and associated Docker Hub image [\#9126](https://github.com/netdata/netdata/pull/9126) ([prologic](https://github.com/prologic))
- Send anonymous statistics from backends and exporting engine [\#9125](https://github.com/netdata/netdata/pull/9125) ([vlvkobal](https://github.com/vlvkobal))
- Fix buffer splitting in the Kinesis exporting connector [\#9122](https://github.com/netdata/netdata/pull/9122) ([vlvkobal](https://github.com/vlvkobal))
- Update the kernel-collector version to v0.2.0 [\#9118](https://github.com/netdata/netdata/pull/9118) ([prologic](https://github.com/prologic))
- Dynamic memory cleanup for Pub/Sub exporting connector [\#9112](https://github.com/netdata/netdata/pull/9112) ([vlvkobal](https://github.com/vlvkobal))
- Package: obsoletes conflicting EPEL packages \(\#6879 \#8784\) [\#9108](https://github.com/netdata/netdata/pull/9108) ([Saruspete](https://github.com/Saruspete))
- Restore SIGCHLD signal handler after being replaced by libuv [\#9107](https://github.com/netdata/netdata/pull/9107) ([mfundul](https://github.com/mfundul))
- Update eBPF documentation to reflect default enabled status [\#9105](https://github.com/netdata/netdata/pull/9105) ([joelhans](https://github.com/joelhans))
- Add support for eBPF for Netdata static64 \(kickstart-static64.sh\) [\#9104](https://github.com/netdata/netdata/pull/9104) ([prologic](https://github.com/prologic))
- Dynamic memory cleanup for MongoDB exporting connector [\#9103](https://github.com/netdata/netdata/pull/9103) ([vlvkobal](https://github.com/vlvkobal))
- Prepare the main cleanup function for the exporting engine [\#9099](https://github.com/netdata/netdata/pull/9099) ([vlvkobal](https://github.com/vlvkobal))
- Exporting cleanup [\#9098](https://github.com/netdata/netdata/pull/9098) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix typo in dashboard description [\#9096](https://github.com/netdata/netdata/pull/9096) ([Neamar](https://github.com/Neamar))
- Reduce minimum size for dbengine disk space [\#9094](https://github.com/netdata/netdata/pull/9094) ([mfundul](https://github.com/mfundul))
- Fix issue \#9085:  Prometheus TYPE lines incorrectly formatted \(when enabled via query parm\) [\#9086](https://github.com/netdata/netdata/pull/9086) ([jeffgdotorg](https://github.com/jeffgdotorg))
- Clean instances [\#9081](https://github.com/netdata/netdata/pull/9081) ([thiagoftsm](https://github.com/thiagoftsm))
- Introduce a random sleep in the Netdata updater [\#9079](https://github.com/netdata/netdata/pull/9079) ([prologic](https://github.com/prologic))
- New alarms \(exporting and Backend\) [\#9075](https://github.com/netdata/netdata/pull/9075) ([thiagoftsm](https://github.com/thiagoftsm))
- Implement new incremental parser [\#9074](https://github.com/netdata/netdata/pull/9074) ([stelfrag](https://github.com/stelfrag))
- python.d/proxysql: add `Requirements` to the readme [\#9071](https://github.com/netdata/netdata/pull/9071) ([ilyam8](https://github.com/ilyam8))
- OpenTSDB and TLS [\#9068](https://github.com/netdata/netdata/pull/9068) ([thiagoftsm](https://github.com/thiagoftsm))
- Add links to promote database engine calculator [\#9067](https://github.com/netdata/netdata/pull/9067) ([joelhans](https://github.com/joelhans))
- Update the exporting documentation [\#9066](https://github.com/netdata/netdata/pull/9066) ([vlvkobal](https://github.com/vlvkobal))
- Add proc.plugin configuration example for high-processor systems [\#9062](https://github.com/netdata/netdata/pull/9062) ([joelhans](https://github.com/joelhans))
- Added required bundle for libuuid on ClearLinux. [\#9060](https://github.com/netdata/netdata/pull/9060) ([Ferroin](https://github.com/Ferroin))
- claim: fix `New/ephemeral Agent containers` instruction [\#9058](https://github.com/netdata/netdata/pull/9058) ([ilyam8](https://github.com/ilyam8))
- Add notes/known issues section to installation page [\#9053](https://github.com/netdata/netdata/pull/9053) ([joelhans](https://github.com/joelhans))
- Add frontmatter to exporting connectors [\#9052](https://github.com/netdata/netdata/pull/9052) ([joelhans](https://github.com/joelhans))
- Add agent restart note for reclaiming [\#9049](https://github.com/netdata/netdata/pull/9049) ([zack-shoylev](https://github.com/zack-shoylev))
- Missing error aws [\#9048](https://github.com/netdata/netdata/pull/9048) ([thiagoftsm](https://github.com/thiagoftsm))
- Add ACLK Connection Details [\#9047](https://github.com/netdata/netdata/pull/9047) ([zack-shoylev](https://github.com/zack-shoylev))
- Don't overwrite netdata.conf on update on static installs. [\#9046](https://github.com/netdata/netdata/pull/9046) ([Ferroin](https://github.com/Ferroin))
- Change backends to exporting engine in general documentation pages [\#9045](https://github.com/netdata/netdata/pull/9045) ([vlvkobal](https://github.com/vlvkobal))
- Regenerate topic base on connect [\#9044](https://github.com/netdata/netdata/pull/9044) ([amoss](https://github.com/amoss))
- \[python.d/samba\] Only use sudo when not running as root user [\#9038](https://github.com/netdata/netdata/pull/9038) ([Duffyx](https://github.com/Duffyx))
- Missing checks exporting [\#9034](https://github.com/netdata/netdata/pull/9034) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix link in web server log guide [\#9033](https://github.com/netdata/netdata/pull/9033) ([joelhans](https://github.com/joelhans))
- Included 'cmake' in the list of pkgs installed [\#9031](https://github.com/netdata/netdata/pull/9031) ([zvarnes](https://github.com/zvarnes))
- Move nc backend [\#9030](https://github.com/netdata/netdata/pull/9030) ([thiagoftsm](https://github.com/thiagoftsm))
- Add text to claiming doc about reclaiming with id= [\#9027](https://github.com/netdata/netdata/pull/9027) ([joelhans](https://github.com/joelhans))
- Fixes enable/start of netdata service in debian package [\#9005](https://github.com/netdata/netdata/pull/9005) ([MrFreezeex](https://github.com/MrFreezeex))
- Add GitHub CI job to check Markdown links during PRs [\#9003](https://github.com/netdata/netdata/pull/9003) ([joelhans](https://github.com/joelhans))
- Update step-10.md [\#9000](https://github.com/netdata/netdata/pull/9000) ([Jelmerrevers](https://github.com/Jelmerrevers))
- Fix suid bits on plugin for debian packaging [\#8996](https://github.com/netdata/netdata/pull/8996) ([MrFreezeex](https://github.com/MrFreezeex))
- Fix incorrect issue link URL in install-required-packages.sh [\#8911](https://github.com/netdata/netdata/pull/8911) ([prologic](https://github.com/prologic))
- Docs: Remove old Cloud/dashboard and replace with new Cloud/dashboard [\#8874](https://github.com/netdata/netdata/pull/8874) ([joelhans](https://github.com/joelhans))
- removed simley face per Chris since it didn't show up [\#8872](https://github.com/netdata/netdata/pull/8872) ([MeganBishopMoore](https://github.com/MeganBishopMoore))
- Fix macOS builds building and linking against openssl [\#8865](https://github.com/netdata/netdata/pull/8865) ([prologic](https://github.com/prologic))
- Removeed Polyverse Polymorphic Linux from Docker builds. [\#8802](https://github.com/netdata/netdata/pull/8802) ([Ferroin](https://github.com/Ferroin))
- Remove old docs generation tooling [\#8783](https://github.com/netdata/netdata/pull/8783) ([prologic](https://github.com/prologic))
- Docs: Update contributing guidelines [\#8781](https://github.com/netdata/netdata/pull/8781) ([joelhans](https://github.com/joelhans))
- Sort alphabetically and automatic scroll [\#8762](https://github.com/netdata/netdata/pull/8762) ([tnyeanderson](https://github.com/tnyeanderson))
- single quote apostrophe [\#8723](https://github.com/netdata/netdata/pull/8723) ([zack-shoylev](https://github.com/zack-shoylev))
- Correctly track last num vcpus in xenstat\_plugin [\#8720](https://github.com/netdata/netdata/pull/8720) ([rushikeshjadhav](https://github.com/rushikeshjadhav))
- Typo. [\#8703](https://github.com/netdata/netdata/pull/8703) ([cherouvim](https://github.com/cherouvim))
- install and enable eBPF Plugin by default [\#8665](https://github.com/netdata/netdata/pull/8665) ([Ferroin](https://github.com/Ferroin))
- Update synology.md [\#8658](https://github.com/netdata/netdata/pull/8658) ([thenktor](https://github.com/thenktor))
- Ceph: Added OSD size collection [\#8649](https://github.com/netdata/netdata/pull/8649) ([elelayan](https://github.com/elelayan))
- Update freebsd.md [\#8643](https://github.com/netdata/netdata/pull/8643) ([thenktor](https://github.com/thenktor))

## [v1.22.1](https://github.com/netdata/netdata/tree/v1.22.1) (2020-05-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.22.0...v1.22.1)

**Merged pull requests:**

- Fix the latency issue on the ACLK and suppress the diagnostics [\#8992](https://github.com/netdata/netdata/pull/8992) ([amoss](https://github.com/amoss))
- Fixed bundling of React dashboard in DEB and RPM packages. [\#8988](https://github.com/netdata/netdata/pull/8988) ([Ferroin](https://github.com/Ferroin))
- Restore old semantics of "netdata -W set" command [\#8987](https://github.com/netdata/netdata/pull/8987) ([mfundul](https://github.com/mfundul))
- Added JSON-C packaging fils to make dist. [\#8986](https://github.com/netdata/netdata/pull/8986) ([Ferroin](https://github.com/Ferroin))
- Remove check for old alarm status \(CID 358436\) [\#8978](https://github.com/netdata/netdata/pull/8978) ([stelfrag](https://github.com/stelfrag))
- Remove UNUSED word from flood protection documentation [\#8964](https://github.com/netdata/netdata/pull/8964) ([mfundul](https://github.com/mfundul))
- Docs: Update with go-live claiming and ACLK information \(\#8859\) [\#8960](https://github.com/netdata/netdata/pull/8960) ([prologic](https://github.com/prologic))
- Docs: Fix internal links and remove obsolete admonitions [\#8946](https://github.com/netdata/netdata/pull/8946) ([joelhans](https://github.com/joelhans))
- Fix shutdown via netdatacli with musl C library [\#8931](https://github.com/netdata/netdata/pull/8931) ([mfundul](https://github.com/mfundul))

## [v1.22.0](https://github.com/netdata/netdata/tree/v1.22.0) (2020-05-11)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.21.1...v1.22.0)

**Merged pull requests:**

- Updates main copyright and links for the year 2020 [\#8937](https://github.com/netdata/netdata/pull/8937) ([zack-shoylev](https://github.com/zack-shoylev))
- Docs: Add custom label to collectors frontmatter to fix sidebar titles [\#8936](https://github.com/netdata/netdata/pull/8936) ([joelhans](https://github.com/joelhans))
- Fix missing NETDATA\_STOP\_CMD in netdata-installer.sh [\#8897](https://github.com/netdata/netdata/pull/8897) ([prologic](https://github.com/prologic))
- Update Running-behind-nginx.md [\#8880](https://github.com/netdata/netdata/pull/8880) ([slavaGanzin](https://github.com/slavaGanzin))
- Added docmentation about workaround for clang build errors. [\#8867](https://github.com/netdata/netdata/pull/8867) ([Ferroin](https://github.com/Ferroin))
- correct typo [\#8861](https://github.com/netdata/netdata/pull/8861) ([carehart](https://github.com/carehart))
- Fix command name for getting postfix queue [\#8857](https://github.com/netdata/netdata/pull/8857) ([ghasrfakhri](https://github.com/ghasrfakhri))
- Fix kickstart error removing old cron symlink [\#8849](https://github.com/netdata/netdata/pull/8849) ([prologic](https://github.com/prologic))
- Fixed bundling of dashboard in binary packages. [\#8844](https://github.com/netdata/netdata/pull/8844) ([Ferroin](https://github.com/Ferroin))
- Add CI check for building against LibreSSL [\#8842](https://github.com/netdata/netdata/pull/8842) ([prologic](https://github.com/prologic))
- Removed old function call in netdata-installer.sh [\#8824](https://github.com/netdata/netdata/pull/8824) ([Ferroin](https://github.com/Ferroin))
- Fix build and add bundle-dashbaord.sh to dist\_noinst\_DATA [\#8823](https://github.com/netdata/netdata/pull/8823) ([prologic](https://github.com/prologic))
- Docs: Add instructions to persist metrics and restart policy [\#8813](https://github.com/netdata/netdata/pull/8813) ([joelhans](https://github.com/joelhans))
- Fix typo in netdata-installer [\#8811](https://github.com/netdata/netdata/pull/8811) ([adamwolf](https://github.com/adamwolf))
- health: fix mdstat `failed devices` alarm [\#8794](https://github.com/netdata/netdata/pull/8794) ([ilyam8](https://github.com/ilyam8))
- dashboard v0.4.18 [\#8786](https://github.com/netdata/netdata/pull/8786) ([jacekkolasa](https://github.com/jacekkolasa))
- fix\_lock: Add the missing lock [\#8780](https://github.com/netdata/netdata/pull/8780) ([thiagoftsm](https://github.com/thiagoftsm))
- Added JSON-C dependency handling to instlal and packaging. [\#8776](https://github.com/netdata/netdata/pull/8776) ([Ferroin](https://github.com/Ferroin))
- TTL headers [\#8760](https://github.com/netdata/netdata/pull/8760) ([amoss](https://github.com/amoss))
- web/gui/demo2.html: Silence Netlify's mixed content warnings [\#8759](https://github.com/netdata/netdata/pull/8759) ([knatsakis](https://github.com/knatsakis))
- dashboard v.0.4.17: [\#8757](https://github.com/netdata/netdata/pull/8757) ([jacekkolasa](https://github.com/jacekkolasa))
- Docs: Add Docker instructions to claiming [\#8755](https://github.com/netdata/netdata/pull/8755) ([joelhans](https://github.com/joelhans))
- Fixed issue in `system-info.sh`regarding the parsing of `lscpu` output. [\#8754](https://github.com/netdata/netdata/pull/8754) ([Ferroin](https://github.com/Ferroin))
- Use a prefix for the old dashboard. [\#8752](https://github.com/netdata/netdata/pull/8752) ([Ferroin](https://github.com/Ferroin))
- Additional cases for the thread exit fix [\#8750](https://github.com/netdata/netdata/pull/8750) ([amoss](https://github.com/amoss))
- health/portcheck: remove no-clear-notification option [\#8748](https://github.com/netdata/netdata/pull/8748) ([ilyam8](https://github.com/ilyam8))
- packaging/docker/{build,publish}.sh: Simplify scripts. Support only single ARCH [\#8747](https://github.com/netdata/netdata/pull/8747) ([knatsakis](https://github.com/knatsakis))
- Ebpf index size [\#8743](https://github.com/netdata/netdata/pull/8743) ([thiagoftsm](https://github.com/thiagoftsm))
- \[docs\]: fix enabling charts.d modules instruction for IOT [\#8740](https://github.com/netdata/netdata/pull/8740) ([Jiab77](https://github.com/Jiab77))
- Improved ACLK reconnection sequence [\#8729](https://github.com/netdata/netdata/pull/8729) ([stelfrag](https://github.com/stelfrag))
- Revert "Improved ACLK reconnection sequence " [\#8728](https://github.com/netdata/netdata/pull/8728) ([cosmix](https://github.com/cosmix))
- Fix crash when shutdown with ACLK disabled [\#8725](https://github.com/netdata/netdata/pull/8725) ([lassebm](https://github.com/lassebm))
- Docs: Combined claiming+ACLK documentation [\#8724](https://github.com/netdata/netdata/pull/8724) ([joelhans](https://github.com/joelhans))
- Fix docs Docker-based builder image [\#8718](https://github.com/netdata/netdata/pull/8718) ([prologic](https://github.com/prologic))
- Fixed the build matrix in the build & install checks. [\#8715](https://github.com/netdata/netdata/pull/8715) ([Ferroin](https://github.com/Ferroin))
- capitalize title [\#8712](https://github.com/netdata/netdata/pull/8712) ([zack-shoylev](https://github.com/zack-shoylev))
- Improved ACLK reconnection sequence  [\#8708](https://github.com/netdata/netdata/pull/8708) ([stelfrag](https://github.com/stelfrag))
- added whoisquery health templates [\#8700](https://github.com/netdata/netdata/pull/8700) ([yasharne](https://github.com/yasharne))
- Fixed Arch Linux Ci checks. [\#8699](https://github.com/netdata/netdata/pull/8699) ([Ferroin](https://github.com/Ferroin))
- yamllint: enable truthy rule [\#8698](https://github.com/netdata/netdata/pull/8698) ([ilyam8](https://github.com/ilyam8))
- Fixes compatibility with RH 7.x family [\#8694](https://github.com/netdata/netdata/pull/8694) ([thiagoftsm](https://github.com/thiagoftsm))
- charts.d/apcupsd: fix ups status check [\#8688](https://github.com/netdata/netdata/pull/8688) ([ilyam8](https://github.com/ilyam8))
- Update pfSense doc and add warning for apcupsd users [\#8686](https://github.com/netdata/netdata/pull/8686) ([cryptoluks](https://github.com/cryptoluks))
- added certificate revocation alert [\#8684](https://github.com/netdata/netdata/pull/8684) ([yasharne](https://github.com/yasharne))
- \[ReOpen \#8626\] Improved offline installation instructions to point to correct installation scripts and clarify process [\#8680](https://github.com/netdata/netdata/pull/8680) ([IceCodeNew](https://github.com/IceCodeNew))
- Docs: Standardize links between documentation [\#8638](https://github.com/netdata/netdata/pull/8638) ([joelhans](https://github.com/joelhans))

## [v1.21.1](https://github.com/netdata/netdata/tree/v1.21.1) (2020-04-13)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.21.0...v1.21.1)

**Merged pull requests:**

- V1.21.0 dashboard performance fix extended [\#8664](https://github.com/netdata/netdata/pull/8664) ([jacekkolasa](https://github.com/jacekkolasa))
- Update apps\_groups.conf [\#8659](https://github.com/netdata/netdata/pull/8659) ([thenktor](https://github.com/thenktor))
- Update apps\_groups.conf [\#8656](https://github.com/netdata/netdata/pull/8656) ([thenktor](https://github.com/thenktor))
- Update apps\_groups.conf [\#8655](https://github.com/netdata/netdata/pull/8655) ([thenktor](https://github.com/thenktor))
- health/alarm\_notify: add dynatrace enabled check [\#8654](https://github.com/netdata/netdata/pull/8654) ([ilyam8](https://github.com/ilyam8))
- Update apps\_groups.conf [\#8646](https://github.com/netdata/netdata/pull/8646) ([thenktor](https://github.com/thenktor))
- Docs: Pin mkdocs-material to older version to re-enable builds [\#8639](https://github.com/netdata/netdata/pull/8639) ([joelhans](https://github.com/joelhans))
- collectors/python.d/mysql: fix `threads\_creation\_rate` chart context [\#8636](https://github.com/netdata/netdata/pull/8636) ([ilyam8](https://github.com/ilyam8))
- Show internal stats for the exporting engine [\#8635](https://github.com/netdata/netdata/pull/8635) ([vlvkobal](https://github.com/vlvkobal))
- Add session-id using connect timestamp [\#8633](https://github.com/netdata/netdata/pull/8633) ([amoss](https://github.com/amoss))
- Update main README with 1.21 release news [\#8619](https://github.com/netdata/netdata/pull/8619) ([joelhans](https://github.com/joelhans))
- Improved ACLK memory management and shutdown sequence [\#8611](https://github.com/netdata/netdata/pull/8611) ([stelfrag](https://github.com/stelfrag))
- packaging: fix errors during install-requred-packages [\#8606](https://github.com/netdata/netdata/pull/8606) ([ilyam8](https://github.com/ilyam8))
- Remove an automatic restart of the apps.plugin [\#8592](https://github.com/netdata/netdata/pull/8592) ([vlvkobal](https://github.com/vlvkobal))

## [v1.21.0](https://github.com/netdata/netdata/tree/v1.21.0) (2020-04-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.20.0...v1.21.0)

**Merged pull requests:**

- dashboard v0.4.12: [\#8599](https://github.com/netdata/netdata/pull/8599) ([jacekkolasa](https://github.com/jacekkolasa))
- Change all https://app.netdata.cloud URLs to https://netdata.cloud [\#8598](https://github.com/netdata/netdata/pull/8598) ([mfundul](https://github.com/mfundul))
- Fixes Ubuntu build with both libcap-dev and libcapng [\#8596](https://github.com/netdata/netdata/pull/8596) ([underhood](https://github.com/underhood))
- Correctly fixed RPM package builds on Fedora. [\#8595](https://github.com/netdata/netdata/pull/8595) ([Ferroin](https://github.com/Ferroin))
- Ensure we only enable jessie-backports for Debian 8 \(jessie\) once [\#8593](https://github.com/netdata/netdata/pull/8593) ([prologic](https://github.com/prologic))

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

[Full Changelog](https://github.com/netdata/netdata/compare/v1.16.1...v1.17.0)

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
