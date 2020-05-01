# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.21.1...HEAD)

**Merged pull requests:**

- correct typo [\#8861](https://github.com/netdata/netdata/pull/8861) ([carehart](https://github.com/carehart))
- Fix kickstart error removing old cron symlink [\#8849](https://github.com/netdata/netdata/pull/8849) ([prologic](https://github.com/prologic))
- Fixed bundling of dashboard in binary packages. [\#8844](https://github.com/netdata/netdata/pull/8844) ([Ferroin](https://github.com/Ferroin))
- Add CI check for building against LibreSSL [\#8842](https://github.com/netdata/netdata/pull/8842) ([prologic](https://github.com/prologic))
- Removed old function call in netdata-installer.sh [\#8824](https://github.com/netdata/netdata/pull/8824) ([Ferroin](https://github.com/Ferroin))
- Fix build and add bundle-dashbaord.sh to dist\_noinst\_DATA [\#8823](https://github.com/netdata/netdata/pull/8823) ([prologic](https://github.com/prologic))
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
- bind to should be in \[web\] section and update netdata.service.v235.in too [\#8454](https://github.com/netdata/netdata/pull/8454) ([amishmm](https://github.com/amishmm))
- Added support for building libmosquitto on FreeBSD/macOS. [\#8254](https://github.com/netdata/netdata/pull/8254) ([Ferroin](https://github.com/Ferroin))

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
- Improve installer output re newlines [\#8447](https://github.com/netdata/netdata/pull/8447) ([prologic](https://github.com/prologic))
- Fix erroneous \n printed in uninstaller [\#8446](https://github.com/netdata/netdata/pull/8446) ([prologic](https://github.com/prologic))
- Fixes support for uninstalling the eBPF collector in the uninstaller and fixes a minor bug [\#8444](https://github.com/netdata/netdata/pull/8444) ([prologic](https://github.com/prologic))
- Fix the ACLK installation with the installer switch [\#8443](https://github.com/netdata/netdata/pull/8443) ([amoss](https://github.com/amoss))
- Add high precision timer support for plugins such as idlejitter. [\#8441](https://github.com/netdata/netdata/pull/8441) ([mfundul](https://github.com/mfundul))
- health: add dns\_query module alarm [\#8434](https://github.com/netdata/netdata/pull/8434) ([ilyam8](https://github.com/ilyam8))
- Add the new cloud info in the info endpoint [\#8430](https://github.com/netdata/netdata/pull/8430) ([amoss](https://github.com/amoss))
- Report Why ACLK build failed [\#8429](https://github.com/netdata/netdata/pull/8429) ([underhood](https://github.com/underhood))
- Fake collector to provoke ACLK messages [\#8427](https://github.com/netdata/netdata/pull/8427) ([amoss](https://github.com/amoss))
- Fixed JSON parsing [\#8426](https://github.com/netdata/netdata/pull/8426) ([stelfrag](https://github.com/stelfrag))
- Fix flushing error threshold [\#8425](https://github.com/netdata/netdata/pull/8425) ([mfundul](https://github.com/mfundul))
- Fixed response payload to match the new specification [\#8420](https://github.com/netdata/netdata/pull/8420) ([stelfrag](https://github.com/stelfrag))
- HTTP proxy support + some cleanup [\#8418](https://github.com/netdata/netdata/pull/8418) ([underhood](https://github.com/underhood))
- Add a MongoDB connector to the exporting engine [\#8416](https://github.com/netdata/netdata/pull/8416) ([vlvkobal](https://github.com/vlvkobal))
- Fix the lack of cleanup in the netdata updater [\#8414](https://github.com/netdata/netdata/pull/8414) ([prologic](https://github.com/prologic))
- Adds missing files to ACLK CMake build [\#8412](https://github.com/netdata/netdata/pull/8412) ([underhood](https://github.com/underhood))
- Fix Prometheus Remote Write build [\#8411](https://github.com/netdata/netdata/pull/8411) ([vlvkobal](https://github.com/vlvkobal))
- ACLK: Implemented Last Will and Testament [\#8410](https://github.com/netdata/netdata/pull/8410) ([stelfrag](https://github.com/stelfrag))
- Fix outstanding problems in claiming and add SOCKS5 support. [\#8406](https://github.com/netdata/netdata/pull/8406) ([amoss](https://github.com/amoss))
- Support SOCKS5 in ACLK Challenge/Response and rewrite with LWS [\#8404](https://github.com/netdata/netdata/pull/8404) ([underhood](https://github.com/underhood))
- ACLK: Fixes the type value for alarm updates [\#8403](https://github.com/netdata/netdata/pull/8403) ([stelfrag](https://github.com/stelfrag))
- Improved the performance of the ACLK. \(\#8391\) [\#8401](https://github.com/netdata/netdata/pull/8401) ([amoss](https://github.com/amoss))
- Perf fixes [\#8399](https://github.com/netdata/netdata/pull/8399) ([amoss](https://github.com/amoss))
- ACLK: Improved the agent "pop-corning" phase [\#8398](https://github.com/netdata/netdata/pull/8398) ([stelfrag](https://github.com/stelfrag))
- Fix broken dependencies for Ubuntu 19.10 \(eoan\) [\#8397](https://github.com/netdata/netdata/pull/8397) ([prologic](https://github.com/prologic))
- Update the update instructions with per-method details [\#8394](https://github.com/netdata/netdata/pull/8394) ([joelhans](https://github.com/joelhans))
- Fix coverity scan [\#8388](https://github.com/netdata/netdata/pull/8388) ([prologic](https://github.com/prologic))
- Add Patti's dashboard video to docs [\#8385](https://github.com/netdata/netdata/pull/8385) ([joelhans](https://github.com/joelhans))
- Added deferred error message handling to the installer. [\#8381](https://github.com/netdata/netdata/pull/8381) ([Ferroin](https://github.com/Ferroin))
- docs: fix go.d modules in the COLLECTORS.md [\#8380](https://github.com/netdata/netdata/pull/8380) ([ilyam8](https://github.com/ilyam8))
- Fix streaming scaling [\#8375](https://github.com/netdata/netdata/pull/8375) ([mfundul](https://github.com/mfundul))
- Change topics for ACLK [\#8374](https://github.com/netdata/netdata/pull/8374) ([amoss](https://github.com/amoss))
- Migrate make dist validation to GHA Workflows [\#8373](https://github.com/netdata/netdata/pull/8373) ([prologic](https://github.com/prologic))
- Add proper parsing/stripping of comments around docs frontmatter [\#8372](https://github.com/netdata/netdata/pull/8372) ([joelhans](https://github.com/joelhans))
- Format of commit messages [\#8365](https://github.com/netdata/netdata/pull/8365) ([amoss](https://github.com/amoss))
- new version of godplugin and pulsar alarms, dashboard info [\#8364](https://github.com/netdata/netdata/pull/8364) ([ilyam8](https://github.com/ilyam8))
- Switched to the new React dashboard code as the default dashboard. [\#8363](https://github.com/netdata/netdata/pull/8363) ([Ferroin](https://github.com/Ferroin))
- Fix MDX parsing in installation guide [\#8362](https://github.com/netdata/netdata/pull/8362) ([joelhans](https://github.com/joelhans))
- ebpf plugin info typo fix  [\#8360](https://github.com/netdata/netdata/pull/8360) ([ilyam8](https://github.com/ilyam8))
- Improve ACLK according to results of the smoke-test. [\#8358](https://github.com/netdata/netdata/pull/8358) ([amoss](https://github.com/amoss))
- Improve Pull Request template to have a shorter testing section with enhanced instructions [\#8357](https://github.com/netdata/netdata/pull/8357) ([prologic](https://github.com/prologic))
- Add packaging/bundle-lws.sh to dist\_noinst\_DATA \(was missed\) [\#8356](https://github.com/netdata/netdata/pull/8356) ([prologic](https://github.com/prologic))
- Remove possible erroneous blank line causing Travis to fail possibly and default to magical behaviour [\#8355](https://github.com/netdata/netdata/pull/8355) ([prologic](https://github.com/prologic))
- Bulk add frontmatter to all documentation [\#8354](https://github.com/netdata/netdata/pull/8354) ([joelhans](https://github.com/joelhans))
- Update paragraph on install-required-packages [\#8347](https://github.com/netdata/netdata/pull/8347) ([prologic](https://github.com/prologic))
- Fix cosmetic error checking for CentOS 8 version in install-required-packages [\#8339](https://github.com/netdata/netdata/pull/8339) ([prologic](https://github.com/prologic))
- Fixed typo [\#8335](https://github.com/netdata/netdata/pull/8335) ([peroxy](https://github.com/peroxy))
- Fixed dependency names for Arch Linux. [\#8334](https://github.com/netdata/netdata/pull/8334) ([Ferroin](https://github.com/Ferroin))
- Correct a typo in .travis/README.md [\#8333](https://github.com/netdata/netdata/pull/8333) ([felixonmars](https://github.com/felixonmars))
- Rename the review workflow and be consistent about workflow and job names. [\#8332](https://github.com/netdata/netdata/pull/8332) ([prologic](https://github.com/prologic))
- Migrate Tests from Travis CI to Github Workflows [\#8331](https://github.com/netdata/netdata/pull/8331) ([prologic](https://github.com/prologic))
- Migrate Travis based checks to Github Actions [\#8329](https://github.com/netdata/netdata/pull/8329) ([prologic](https://github.com/prologic))
- Add retries for more Travis transient failures [\#8327](https://github.com/netdata/netdata/pull/8327) ([prologic](https://github.com/prologic))
- Removed extra printed `\n` [\#8326](https://github.com/netdata/netdata/pull/8326) ([Jiab77](https://github.com/Jiab77))
- Removed extra printed `\n` [\#8325](https://github.com/netdata/netdata/pull/8325) ([Jiab77](https://github.com/Jiab77))
- Removed extra printed `\n` [\#8324](https://github.com/netdata/netdata/pull/8324) ([Jiab77](https://github.com/Jiab77))
- Migrate coverity-scan to Github Actions [\#8321](https://github.com/netdata/netdata/pull/8321) ([prologic](https://github.com/prologic))
- Fix links in packaging/installer to work on GitHub and docs [\#8319](https://github.com/netdata/netdata/pull/8319) ([joelhans](https://github.com/joelhans))
- Switched to using Python 3 by default instead of Python 2. [\#8318](https://github.com/netdata/netdata/pull/8318) ([Ferroin](https://github.com/Ferroin))
- Added various fixes and improvements to the installers. [\#8315](https://github.com/netdata/netdata/pull/8315) ([Ferroin](https://github.com/Ferroin))
- Fixed missing folders in `/var/` by creating them during postinst [\#8314](https://github.com/netdata/netdata/pull/8314) ([SamK](https://github.com/SamK))
- Add standards for abbreviations/acronyms to docs style guide [\#8313](https://github.com/netdata/netdata/pull/8313) ([joelhans](https://github.com/joelhans))
- Revert "Fixed Source0 URL in RPM spec \(\#7794\)" [\#8305](https://github.com/netdata/netdata/pull/8305) ([prologic](https://github.com/prologic))
- Add a Prometheus Remote Write connector to the exporting engine [\#8292](https://github.com/netdata/netdata/pull/8292) ([vlvkobal](https://github.com/vlvkobal))
- Fix dependencies for Debian Jessie. [\#8290](https://github.com/netdata/netdata/pull/8290) ([Ferroin](https://github.com/Ferroin))
- Added code to bundle LWS in binary packages. [\#8255](https://github.com/netdata/netdata/pull/8255) ([Ferroin](https://github.com/Ferroin))
- Remove mention saying that .deb packages are experimental [\#8250](https://github.com/netdata/netdata/pull/8250) ([toadjaune](https://github.com/toadjaune))
- Enconde slave fields [\#8216](https://github.com/netdata/netdata/pull/8216) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove the confusion around the multiple Dockerfile\(s\) we have [\#8214](https://github.com/netdata/netdata/pull/8214) ([prologic](https://github.com/prologic))
- Remvoed the use of clang-format that does not actually block PRs or surface anything to developers \#8188 [\#8196](https://github.com/netdata/netdata/pull/8196) ([prologic](https://github.com/prologic))

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
