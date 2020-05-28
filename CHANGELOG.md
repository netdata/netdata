# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.22.1...HEAD)

**Merged pull requests:**

- Fix missing ebpf packaging files from dist archive [\#9182](https://github.com/netdata/netdata/pull/9182) ([prologic](https://github.com/prologic))
- cov\_358988: Remove coverity bug [\#9180](https://github.com/netdata/netdata/pull/9180) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove knatsakis and ncmans from CODEOWNERS for the agent. [\#9173](https://github.com/netdata/netdata/pull/9173) ([Ferroin](https://github.com/Ferroin))
- Remove the experimental label from the exporting engine documentation [\#9171](https://github.com/netdata/netdata/pull/9171) ([vlvkobal](https://github.com/vlvkobal))
- Revert "Introduce a random sleep in the Netdata updater" [\#9161](https://github.com/netdata/netdata/pull/9161) ([prologic](https://github.com/prologic))
- Fixed bug in accepting empty lines in parser [\#9158](https://github.com/netdata/netdata/pull/9158) ([stelfrag](https://github.com/stelfrag))
- Fixed coverity warning \(CID 358971\) [\#9157](https://github.com/netdata/netdata/pull/9157) ([stelfrag](https://github.com/stelfrag))
- Update README.md [\#9151](https://github.com/netdata/netdata/pull/9151) ([stephenrauch](https://github.com/stephenrauch))
- fix typo in step-03.md [\#9150](https://github.com/netdata/netdata/pull/9150) ([waybeforenow](https://github.com/waybeforenow))
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
- Caddy section lacked data persist volumes [\#8999](https://github.com/netdata/netdata/pull/8999) ([webash](https://github.com/webash))
- Fix suid bits on plugin for debian packaging [\#8996](https://github.com/netdata/netdata/pull/8996) ([MrFreezeex](https://github.com/MrFreezeex))
- Add text to ACLK doc mentioning WebSockets and port [\#8968](https://github.com/netdata/netdata/pull/8968) ([joelhans](https://github.com/joelhans))
- Update daemon output with new URLs and dates [\#8965](https://github.com/netdata/netdata/pull/8965) ([joelhans](https://github.com/joelhans))
- Update news section of main README with 1.22 news [\#8963](https://github.com/netdata/netdata/pull/8963) ([joelhans](https://github.com/joelhans))
- Fix file name weblog.conf -\> web\_log.conf [\#8959](https://github.com/netdata/netdata/pull/8959) ([gruentee](https://github.com/gruentee))
- \[varnish\] : added compatibility for varnish-plus [\#8940](https://github.com/netdata/netdata/pull/8940) ([pgjavier](https://github.com/pgjavier))
- postgres.chart.py: fix template databases ignore [\#8929](https://github.com/netdata/netdata/pull/8929) ([slavaGanzin](https://github.com/slavaGanzin))
- Account for zfs.arc\_size.min, and correct calc [\#8913](https://github.com/netdata/netdata/pull/8913) ([araemo](https://github.com/araemo))
- Fix incorrect issue link URL in install-required-packages.sh [\#8911](https://github.com/netdata/netdata/pull/8911) ([prologic](https://github.com/prologic))
- Fix error handling in exporting connector [\#8910](https://github.com/netdata/netdata/pull/8910) ([vlvkobal](https://github.com/vlvkobal))
- Fix wrong hostnames in the exporting engine [\#8892](https://github.com/netdata/netdata/pull/8892) ([vlvkobal](https://github.com/vlvkobal))
- Ebpf options [\#8879](https://github.com/netdata/netdata/pull/8879) ([thiagoftsm](https://github.com/thiagoftsm))
- Docs: Remove old Cloud/dashboard and replace with new Cloud/dashboard [\#8874](https://github.com/netdata/netdata/pull/8874) ([joelhans](https://github.com/joelhans))
- removed simley face per Chris since it didn't show up [\#8872](https://github.com/netdata/netdata/pull/8872) ([MeganBishopMoore](https://github.com/MeganBishopMoore))
- Fix macOS builds building and linking against openssl [\#8865](https://github.com/netdata/netdata/pull/8865) ([prologic](https://github.com/prologic))
- Add a Google Cloud Pub/Sub connector to the exporting engine [\#8855](https://github.com/netdata/netdata/pull/8855) ([vlvkobal](https://github.com/vlvkobal))
- Rename eBPF collector [\#8822](https://github.com/netdata/netdata/pull/8822) ([thiagoftsm](https://github.com/thiagoftsm))
- Fixed formatting in API swagger json file [\#8814](https://github.com/netdata/netdata/pull/8814) ([dpsy4](https://github.com/dpsy4))
- Removeed Polyverse Polymorphic Linux from Docker builds. [\#8802](https://github.com/netdata/netdata/pull/8802) ([Ferroin](https://github.com/Ferroin))
- Remove old docs generation tooling [\#8783](https://github.com/netdata/netdata/pull/8783) ([prologic](https://github.com/prologic))
- Docs: Update contributing guidelines [\#8781](https://github.com/netdata/netdata/pull/8781) ([joelhans](https://github.com/joelhans))
- Sort alphabetically and automatic scroll [\#8762](https://github.com/netdata/netdata/pull/8762) ([tnyeanderson](https://github.com/tnyeanderson))
- Correctly track last num vcpus in xenstat\_plugin [\#8720](https://github.com/netdata/netdata/pull/8720) ([rushikeshjadhav](https://github.com/rushikeshjadhav))
- Typo. [\#8703](https://github.com/netdata/netdata/pull/8703) ([cherouvim](https://github.com/cherouvim))
- install and enable eBPF Plugin by default [\#8665](https://github.com/netdata/netdata/pull/8665) ([Ferroin](https://github.com/Ferroin))
- Update synology.md [\#8658](https://github.com/netdata/netdata/pull/8658) ([thenktor](https://github.com/thenktor))
- Update freebsd.md [\#8643](https://github.com/netdata/netdata/pull/8643) ([thenktor](https://github.com/thenktor))
- Update pfsense.md [\#8544](https://github.com/netdata/netdata/pull/8544) ([electropup42](https://github.com/electropup42))

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
- github/workflow: disable `document-start` yamllint check [\#8522](https://github.com/netdata/netdata/pull/8522) ([ilyam8](https://github.com/ilyam8))
- bind to should be in \[web\] section and update netdata.service.v235.in too [\#8454](https://github.com/netdata/netdata/pull/8454) ([amishmm](https://github.com/amishmm))

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
- charts.d/libreswan: fix sudo check [\#8569](https://github.com/netdata/netdata/pull/8569) ([ilyam8](https://github.com/ilyam8))
- Docs: Change MacOS to macOS [\#8562](https://github.com/netdata/netdata/pull/8562) ([joelhans](https://github.com/joelhans))
- Prometheus web api connector [\#8540](https://github.com/netdata/netdata/pull/8540) ([vlvkobal](https://github.com/vlvkobal))
- Health Alarm to Dynatrace Event implementation [\#8476](https://github.com/netdata/netdata/pull/8476) ([illumine](https://github.com/illumine))

## [v1.21.0](https://github.com/netdata/netdata/tree/v1.21.0) (2020-04-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.20.0...v1.21.0)

**Merged pull requests:**

- dashboard v0.4.12: [\#8599](https://github.com/netdata/netdata/pull/8599) ([jacekkolasa](https://github.com/jacekkolasa))
- Change all https://app.netdata.cloud URLs to https://netdata.cloud [\#8598](https://github.com/netdata/netdata/pull/8598) ([mfundul](https://github.com/mfundul))
- Fixes Ubuntu build with both libcap-dev and libcapng [\#8596](https://github.com/netdata/netdata/pull/8596) ([underhood](https://github.com/underhood))
- Correctly fixed RPM package builds on Fedora. [\#8595](https://github.com/netdata/netdata/pull/8595) ([Ferroin](https://github.com/Ferroin))
- Ensure we only enable jessie-backports for Debian 8 \(jessie\) once [\#8593](https://github.com/netdata/netdata/pull/8593) ([prologic](https://github.com/prologic))
- Fix Debian 8 \(jessie\) support [\#8590](https://github.com/netdata/netdata/pull/8590) ([prologic](https://github.com/prologic))
- Read configuration when section is open [\#8588](https://github.com/netdata/netdata/pull/8588) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix Coverity Defect CID-349684 [\#8586](https://github.com/netdata/netdata/pull/8586) ([thiagoftsm](https://github.com/thiagoftsm))
- Coverity scan [\#8579](https://github.com/netdata/netdata/pull/8579) ([amoss](https://github.com/amoss))
- Fix broken Fedora 30/31 RPM builds [\#8572](https://github.com/netdata/netdata/pull/8572) ([prologic](https://github.com/prologic))
- Fix regressions in cloud functionality \(build, CI, claiming\) [\#8568](https://github.com/netdata/netdata/pull/8568) ([underhood](https://github.com/underhood))
- Fix compiler warnings in the claiming code [\#8567](https://github.com/netdata/netdata/pull/8567) ([vlvkobal](https://github.com/vlvkobal))
- Added logic to bail early on LWS build if cmake is not present. [\#8559](https://github.com/netdata/netdata/pull/8559) ([Ferroin](https://github.com/Ferroin))
- Add netdata.service.\* to .gitignore [\#8556](https://github.com/netdata/netdata/pull/8556) ([vlvkobal](https://github.com/vlvkobal))
- Fix broken pipe ignoring in apps plugin [\#8554](https://github.com/netdata/netdata/pull/8554) ([vlvkobal](https://github.com/vlvkobal))
- dashboard v0.4.10 [\#8553](https://github.com/netdata/netdata/pull/8553) ([jacekkolasa](https://github.com/jacekkolasa))
- Update README.md [\#8552](https://github.com/netdata/netdata/pull/8552) ([bceylan](https://github.com/bceylan))
- github/workflow: remove duplicate key \(`line-length`\) from the yamlli… [\#8551](https://github.com/netdata/netdata/pull/8551) ([ilyam8](https://github.com/ilyam8))
- apache: fix `bytespersec` chart context [\#8550](https://github.com/netdata/netdata/pull/8550) ([ilyam8](https://github.com/ilyam8))
- Add missing override for Ubuntu eoan [\#8547](https://github.com/netdata/netdata/pull/8547) ([prologic](https://github.com/prologic))
- Switching over to soft feature flag [\#8545](https://github.com/netdata/netdata/pull/8545) ([amoss](https://github.com/amoss))
- github/workflow:  increase yamllint line length 80=\>120 [\#8542](https://github.com/netdata/netdata/pull/8542) ([ilyam8](https://github.com/ilyam8))
- github/workflow: add python.d configuration files to the yaml-files [\#8541](https://github.com/netdata/netdata/pull/8541) ([ilyam8](https://github.com/ilyam8))
- Write the failure reason during ACLK challenge / response [\#8538](https://github.com/netdata/netdata/pull/8538) ([stelfrag](https://github.com/stelfrag))
- fix minimist vulnerability [\#8537](https://github.com/netdata/netdata/pull/8537) ([jacekkolasa](https://github.com/jacekkolasa))
- charts.d.plugin: add keepalive to global\_update [\#8529](https://github.com/netdata/netdata/pull/8529) ([ilyam8](https://github.com/ilyam8))
- github/workflow: add ACLK to the labeler config [\#8521](https://github.com/netdata/netdata/pull/8521) ([ilyam8](https://github.com/ilyam8))
- The 4th flag [\#8519](https://github.com/netdata/netdata/pull/8519) ([amoss](https://github.com/amoss))
- Claiming issues [\#8516](https://github.com/netdata/netdata/pull/8516) ([amoss](https://github.com/amoss))
- Remove stackscale demo link and clean up page [\#8509](https://github.com/netdata/netdata/pull/8509) ([joelhans](https://github.com/joelhans))
- Fix auto updates for static installs \(kickstart\_static64.sh\) [\#8507](https://github.com/netdata/netdata/pull/8507) ([prologic](https://github.com/prologic))
- Extend TLS Support [\#8505](https://github.com/netdata/netdata/pull/8505) ([thiagoftsm](https://github.com/thiagoftsm))
- Adds install-fake-charts.d.sh to gitignore [\#8502](https://github.com/netdata/netdata/pull/8502) ([underhood](https://github.com/underhood))
- Cleans up cloud config files \[agent\_cloud\_link\] -\> \[cloud\] [\#8501](https://github.com/netdata/netdata/pull/8501) ([underhood](https://github.com/underhood))
- Enhanced ACLK header payload to include timestamp-offset-usec [\#8499](https://github.com/netdata/netdata/pull/8499) ([stelfrag](https://github.com/stelfrag))
- Improved ACLK  [\#8498](https://github.com/netdata/netdata/pull/8498) ([stelfrag](https://github.com/stelfrag))
- Fix openSUSE 15.1 RPM Package Buidls [\#8494](https://github.com/netdata/netdata/pull/8494) ([prologic](https://github.com/prologic))
- python.d/SimpleService: fix module name [\#8492](https://github.com/netdata/netdata/pull/8492) ([ilyam8](https://github.com/ilyam8))
- Fix install-required-packages script to self-update apt [\#8491](https://github.com/netdata/netdata/pull/8491) ([prologic](https://github.com/prologic))
- Relaxes SSL checks for testing [\#8489](https://github.com/netdata/netdata/pull/8489) ([underhood](https://github.com/underhood))
- packaging/docker: add --build-arg CFLAGS support [\#8485](https://github.com/netdata/netdata/pull/8485) ([nicolasparada](https://github.com/nicolasparada))
- installer: update go.d.plugin version to v0.18.0 [\#8477](https://github.com/netdata/netdata/pull/8477) ([ilyam8](https://github.com/ilyam8))
- Installer creates claim.d but is run as root, patch to correct ownership [\#8475](https://github.com/netdata/netdata/pull/8475) ([amoss](https://github.com/amoss))
- python.d.plugin: add prefix to the module name during loading source file [\#8474](https://github.com/netdata/netdata/pull/8474) ([ilyam8](https://github.com/ilyam8))
- Added Docker build arg to pass extra options to installer. [\#8472](https://github.com/netdata/netdata/pull/8472) ([Ferroin](https://github.com/Ferroin))
- Make mosq and LWS lib build fails more prominent [\#8470](https://github.com/netdata/netdata/pull/8470) ([underhood](https://github.com/underhood))
- Fix our Debian/Ubuntu packages to actually package the SystemD Unit files we expect. [\#8468](https://github.com/netdata/netdata/pull/8468) ([prologic](https://github.com/prologic))
- Adding the claiming script to the multi-stage whitelist [\#8465](https://github.com/netdata/netdata/pull/8465) ([amoss](https://github.com/amoss))
- Memory leak with labels on stream [\#8460](https://github.com/netdata/netdata/pull/8460) ([thiagoftsm](https://github.com/thiagoftsm))
- Report ACLK Connection Failure [\#8456](https://github.com/netdata/netdata/pull/8456) ([underhood](https://github.com/underhood))
- Fix syntax error in claiming script. [\#8452](https://github.com/netdata/netdata/pull/8452) ([mfundul](https://github.com/mfundul))

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
