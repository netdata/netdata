# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.19.0...HEAD)

**Merged pull requests:**

- Remove unnessecary `echo` call in updater. [\#7783](https://github.com/netdata/netdata/pull/7783) ([Ferroin](https://github.com/Ferroin))
- Fix CSV -\> SSV in docs [\#7782](https://github.com/netdata/netdata/pull/7782) ([cosmix](https://github.com/cosmix))
- Fix a Coverity issue [\#7780](https://github.com/netdata/netdata/pull/7780) ([vlvkobal](https://github.com/vlvkobal))
- Fix libuv IPC pipe cleanup problem [\#7778](https://github.com/netdata/netdata/pull/7778) ([mfundul](https://github.com/mfundul))
- Add a missing parameter to the allmetrics endpoint in Swagger Editor [\#7776](https://github.com/netdata/netdata/pull/7776) ([vlvkobal](https://github.com/vlvkobal))
- Limit PR labeler runs to the main repo. [\#7768](https://github.com/netdata/netdata/pull/7768) ([Ferroin](https://github.com/Ferroin))
- Fix the formatting of the trailer line in the Debian changelog template. [\#7763](https://github.com/netdata/netdata/pull/7763) ([Ferroin](https://github.com/Ferroin))
- Filter out lxc cgroups which are not useful [\#7760](https://github.com/netdata/netdata/pull/7760) ([vlvkobal](https://github.com/vlvkobal))
- Improve styling of documentation site and use Algolia search [\#7753](https://github.com/netdata/netdata/pull/7753) ([joelhans](https://github.com/joelhans))
- multiple files: fix typos [\#7752](https://github.com/netdata/netdata/pull/7752) ([schneiderl](https://github.com/schneiderl))
- on cloud error, inform user to update their netdata. [\#7750](https://github.com/netdata/netdata/pull/7750) ([jacekkolasa](https://github.com/jacekkolasa))
- Update stop-notifications-alarms.md [\#7737](https://github.com/netdata/netdata/pull/7737) ([yasharne](https://github.com/yasharne))
- Do not alert the \#automation channel on checksum failures that will fail a PR in CI anyway [\#7733](https://github.com/netdata/netdata/pull/7733) ([prologic](https://github.com/prologic))
- installer: include go.d.plugin version v0.14.1 [\#7732](https://github.com/netdata/netdata/pull/7732) ([ilyam8](https://github.com/ilyam8))
- Fix a check for nfnetlink\_conntrack.h [\#7727](https://github.com/netdata/netdata/pull/7727) ([vlvkobal](https://github.com/vlvkobal))
- Fixes support for read-only /lib on SystemD systems like CoreOS in  kickstart static64 [\#7726](https://github.com/netdata/netdata/pull/7726) ([prologic](https://github.com/prologic))
- Cleanup packaging/makeself/build-x86\_64-static.sh to use /bin/sh and remove use of sudo [\#7725](https://github.com/netdata/netdata/pull/7725) ([prologic](https://github.com/prologic))
- Control introduction of new languages in docs translation [\#7722](https://github.com/netdata/netdata/pull/7722) ([cakrit](https://github.com/cakrit))
- .travis.yml: Reduce notifications [\#7714](https://github.com/netdata/netdata/pull/7714) ([knatsakis](https://github.com/knatsakis))
- litespeed: add support for different .rtreport format [\#7705](https://github.com/netdata/netdata/pull/7705) ([lucasRolff](https://github.com/lucasRolff))
- Make auto-updates work on kickstart-static64 installs. [\#7704](https://github.com/netdata/netdata/pull/7704) ([Ferroin](https://github.com/Ferroin))
- Fix PR labeling \(again\). [\#7699](https://github.com/netdata/netdata/pull/7699) ([Ferroin](https://github.com/Ferroin))
- General fixes to the installer. [\#7698](https://github.com/netdata/netdata/pull/7698) ([Ferroin](https://github.com/Ferroin))
- Fix PR labeling GitHub Action. [\#7697](https://github.com/netdata/netdata/pull/7697) ([Ferroin](https://github.com/Ferroin))
- Fixes \#7680 Remote write [\#7694](https://github.com/netdata/netdata/pull/7694) ([Ehekatl](https://github.com/Ehekatl))
- Fix unclosed brackets in softnet alarm [\#7693](https://github.com/netdata/netdata/pull/7693) ([Ehekatl](https://github.com/Ehekatl))
- Adds a Dockerfile.docs for more easily and reproducibly building/rebuilding docs [\#7688](https://github.com/netdata/netdata/pull/7688) ([prologic](https://github.com/prologic))
- Fix a syntax error in the packaging functions. [\#7686](https://github.com/netdata/netdata/pull/7686) ([Ferroin](https://github.com/Ferroin))
- Add missing quoting in shell scripts. [\#7685](https://github.com/netdata/netdata/pull/7685) ([Ferroin](https://github.com/Ferroin))
- Restore support for protobuf 3.0 [\#7683](https://github.com/netdata/netdata/pull/7683) ([vlvkobal](https://github.com/vlvkobal))
- Fix spelling of Prometheus \(\#7673\) [\#7674](https://github.com/netdata/netdata/pull/7674) ([candrews](https://github.com/candrews))
- installer: include go.d.plugin version v0.14.0 [\#7666](https://github.com/netdata/netdata/pull/7666) ([ilyam8](https://github.com/ilyam8))
- Fixes for pfSense Installation [\#7665](https://github.com/netdata/netdata/pull/7665) ([prologic](https://github.com/prologic))
- \[Fix\] remove pthread\_setname\_np segfault on musl [\#7664](https://github.com/netdata/netdata/pull/7664) ([Saruspete](https://github.com/Saruspete))
- error exit when rrdhost localhost init fails \#7504 [\#7663](https://github.com/netdata/netdata/pull/7663) ([underhood](https://github.com/underhood))
- Fix buildyaml.sh script so that docs generation works correctly. [\#7662](https://github.com/netdata/netdata/pull/7662) ([Ferroin](https://github.com/Ferroin))
- samba: properly check if it is allowed to run smbstatus with sudo [\#7655](https://github.com/netdata/netdata/pull/7655) ([ilyam8](https://github.com/ilyam8))
- Bump handlebars from 4.2.0 to 4.5.3 [\#7654](https://github.com/netdata/netdata/pull/7654) ([dependabot[bot]](https://github.com/apps/dependabot))
- \[libnetdata/threads\] Change log level on error [\#7653](https://github.com/netdata/netdata/pull/7653) ([Saruspete](https://github.com/Saruspete))
- python.d logger: unicode\_str handle TypeError [\#7645](https://github.com/netdata/netdata/pull/7645) ([ilyam8](https://github.com/ilyam8))
- Redirect when url =~ \/host\/hostname$ \(\#7539\) [\#7643](https://github.com/netdata/netdata/pull/7643) ([underhood](https://github.com/underhood))
- redis: populate `keys\_redis` chart in runtime [\#7639](https://github.com/netdata/netdata/pull/7639) ([ilyam8](https://github.com/ilyam8))
- Minor: Documentation Typo alamrs -\> alarms [\#7637](https://github.com/netdata/netdata/pull/7637) ([underhood](https://github.com/underhood))
- Update the distribution support matrix to represent reality. [\#7636](https://github.com/netdata/netdata/pull/7636) ([Ferroin](https://github.com/Ferroin))
- Switch PR labeling to use GitHub Actions. [\#7630](https://github.com/netdata/netdata/pull/7630) ([Ferroin](https://github.com/Ferroin))
- Add Ubuntu 19.10 to packaging and lifecycle checks. [\#7629](https://github.com/netdata/netdata/pull/7629) ([Ferroin](https://github.com/Ferroin))
- Remove EOL distros from CI jobs. [\#7628](https://github.com/netdata/netdata/pull/7628) ([Ferroin](https://github.com/Ferroin))
- Clean up host labels in API responses [\#7616](https://github.com/netdata/netdata/pull/7616) ([vlvkobal](https://github.com/vlvkobal))
- python.d logger: do not unicode decode if it is already unicode [\#7614](https://github.com/netdata/netdata/pull/7614) ([ilyam8](https://github.com/ilyam8))
- Fix a warning in prometheus remote write backend [\#7609](https://github.com/netdata/netdata/pull/7609) ([vlvkobal](https://github.com/vlvkobal))
- python.d.plugin: UrlService bytes decode, logger unicode encoding fix [\#7601](https://github.com/netdata/netdata/pull/7601) ([ilyam8](https://github.com/ilyam8))
- Adjust alarm labels [\#7600](https://github.com/netdata/netdata/pull/7600) ([thiagoftsm](https://github.com/thiagoftsm))
- Docs: Improve documentation of opting out of anonymous statistics [\#7597](https://github.com/netdata/netdata/pull/7597) ([joelhans](https://github.com/joelhans))
- Update handling of shutdown of the Netdata agent on update and uninstall. [\#7595](https://github.com/netdata/netdata/pull/7595) ([Ferroin](https://github.com/Ferroin))
- Restrict quotes in label values [\#7594](https://github.com/netdata/netdata/pull/7594) ([thiagoftsm](https://github.com/thiagoftsm))
- Add netdata-claim.sh to the RPM spec file. [\#7592](https://github.com/netdata/netdata/pull/7592) ([Ferroin](https://github.com/Ferroin))
- Reduce broken pipe errors [\#7588](https://github.com/netdata/netdata/pull/7588) ([thiagoftsm](https://github.com/thiagoftsm))
- Set standard name to non-libnetdata threads \(libuv, pthread\) [\#7584](https://github.com/netdata/netdata/pull/7584) ([Saruspete](https://github.com/Saruspete))
- Docs: add configuration details for vhost about DOSPageCount to Apache proxy guide [\#7582](https://github.com/netdata/netdata/pull/7582) ([kkoomen](https://github.com/kkoomen))
- CODEOWNERS: Replace @netdata/automation with individual team members [\#7581](https://github.com/netdata/netdata/pull/7581) ([knatsakis](https://github.com/knatsakis))
- Fix not detecting more than one adapter in hpssa collector [\#7580](https://github.com/netdata/netdata/pull/7580) ([gnoddep](https://github.com/gnoddep))
- Docs: Add notice about mod\_evasive to Apache proxy guide [\#7578](https://github.com/netdata/netdata/pull/7578) ([joelhans](https://github.com/joelhans))
- installer: include go.d.plugin version v0.13.0 [\#7574](https://github.com/netdata/netdata/pull/7574) ([ilyam8](https://github.com/ilyam8))
- Fix race condition in dbengine [\#7565](https://github.com/netdata/netdata/pull/7565) ([thiagoftsm](https://github.com/thiagoftsm))
- Move the script for installing required packages into the main repo. [\#7563](https://github.com/netdata/netdata/pull/7563) ([Ferroin](https://github.com/Ferroin))
- Revert "Fix race condition in dbengine \(\#7533\)" [\#7560](https://github.com/netdata/netdata/pull/7560) ([amoss](https://github.com/amoss))
- Skip unit testing during CI when it's not needed. [\#7559](https://github.com/netdata/netdata/pull/7559) ([Ferroin](https://github.com/Ferroin))
- Cleanup the main exporting engine thread on exit [\#7558](https://github.com/netdata/netdata/pull/7558) ([vlvkobal](https://github.com/vlvkobal))
- Send host labels via exporting connectors  [\#7554](https://github.com/netdata/netdata/pull/7554) ([vlvkobal](https://github.com/vlvkobal))
- \[github/templates\] Add samples cmds to get OS env [\#7550](https://github.com/netdata/netdata/pull/7550) ([Saruspete](https://github.com/Saruspete))
- Stream with labels [\#7549](https://github.com/netdata/netdata/pull/7549) ([thiagoftsm](https://github.com/thiagoftsm))
- Alarm Log labels [\#7548](https://github.com/netdata/netdata/pull/7548) ([thiagoftsm](https://github.com/thiagoftsm))
- proc\_pressure: increment fail\_count on read fail [\#7547](https://github.com/netdata/netdata/pull/7547) ([hexchain](https://github.com/hexchain))
- Merge the matrix and jobs keys in Travis config. [\#7544](https://github.com/netdata/netdata/pull/7544) ([Ferroin](https://github.com/Ferroin))
- Limit 'support activities on main branch' to main repo. [\#7543](https://github.com/netdata/netdata/pull/7543) ([Ferroin](https://github.com/Ferroin))
- Fix backend config [\#7538](https://github.com/netdata/netdata/pull/7538) ([vlvkobal](https://github.com/vlvkobal))
- Fix race condition in dbengine [\#7533](https://github.com/netdata/netdata/pull/7533) ([mfundul](https://github.com/mfundul))
- Fix valgrind errors [\#7532](https://github.com/netdata/netdata/pull/7532) ([mfundul](https://github.com/mfundul))
- Update codeowners [\#7530](https://github.com/netdata/netdata/pull/7530) ([knatsakis](https://github.com/knatsakis))
- Docs: Linter fixes for main README [\#7526](https://github.com/netdata/netdata/pull/7526) ([joelhans](https://github.com/joelhans))
- Agent claiming [\#7525](https://github.com/netdata/netdata/pull/7525) ([mfundul](https://github.com/mfundul))
- Add Fedora 31 CI integrations. [\#7524](https://github.com/netdata/netdata/pull/7524) ([Ferroin](https://github.com/Ferroin))
- Labels issues [\#7515](https://github.com/netdata/netdata/pull/7515) ([amoss](https://github.com/amoss))
- Update Netdata RPM spec file to package netdatacli. [\#7513](https://github.com/netdata/netdata/pull/7513) ([Ferroin](https://github.com/Ferroin))
- Remove `-f` option from `groupdel` in uninstaller. [\#7507](https://github.com/netdata/netdata/pull/7507) ([Ferroin](https://github.com/Ferroin))
- Force the repo name to be lowercase in the tag for docker builds. [\#7506](https://github.com/netdata/netdata/pull/7506) ([Ferroin](https://github.com/Ferroin))
- Inject archived backports repository on Debian Jessie for CI package builds. [\#7495](https://github.com/netdata/netdata/pull/7495) ([Ferroin](https://github.com/Ferroin))
- The step-by-step Netdata tutorial [\#7489](https://github.com/netdata/netdata/pull/7489) ([joelhans](https://github.com/joelhans))
- silencers\_info: Change error to info [\#7479](https://github.com/netdata/netdata/pull/7479) ([thiagoftsm](https://github.com/thiagoftsm))
- Add anon tracking notice for installers [\#7437](https://github.com/netdata/netdata/pull/7437) ([ncmans](https://github.com/ncmans))
- Docs: Tweaks and linter fixes to contributing guidelines [\#7407](https://github.com/netdata/netdata/pull/7407) ([joelhans](https://github.com/joelhans))
- packaging: Set default release channel to stable for gh releases [\#7399](https://github.com/netdata/netdata/pull/7399) ([ncmans](https://github.com/ncmans))
- network interface speed, duplex, operstate \#5989 [\#7395](https://github.com/netdata/netdata/pull/7395) ([stelfrag](https://github.com/stelfrag))
- Fix typos in documentation [\#7375](https://github.com/netdata/netdata/pull/7375) ([rex4539](https://github.com/rex4539))
- Minor docker related cleanups [\#7240](https://github.com/netdata/netdata/pull/7240) ([knatsakis](https://github.com/knatsakis))
- Updates and grammar fixes to README.md [\#7193](https://github.com/netdata/netdata/pull/7193) ([joelhans](https://github.com/joelhans))
- Implement the main flow for the Exporting Engine [\#7149](https://github.com/netdata/netdata/pull/7149) ([vlvkobal](https://github.com/vlvkobal))
- feat\(docker\): Add freeipmi support [\#7081](https://github.com/netdata/netdata/pull/7081) ([stevenh](https://github.com/stevenh))

## [v1.19.0](https://github.com/netdata/netdata/tree/v1.19.0) (2019-11-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.18.1...v1.19.0)

**Merged pull requests:**

- installer: include go.d.plugin version v0.11.0 [\#7365](https://github.com/netdata/netdata/pull/7365) ([ilyam8](https://github.com/ilyam8))
- Minor grammar change in /web/gui documentation [\#7363](https://github.com/netdata/netdata/pull/7363) ([eviemsrs](https://github.com/eviemsrs))
- Correct versions of FreeNAS that Netdata is available on [\#7355](https://github.com/netdata/netdata/pull/7355) ([knatsakis](https://github.com/knatsakis))
- Update netdata-security.md [\#7343](https://github.com/netdata/netdata/pull/7343) ([cakrit](https://github.com/cakrit))
- 7232: Fix Debian dependencies [\#7342](https://github.com/netdata/netdata/pull/7342) ([andyundso](https://github.com/andyundso))
- Update plugins.d/README.md [\#7335](https://github.com/netdata/netdata/pull/7335) ([OdysLam](https://github.com/OdysLam))
- collectors: apps.plugin: apps\_groups: added new frr daemons. [\#7333](https://github.com/netdata/netdata/pull/7333) ([k0ste](https://github.com/k0ste))
- Update README.md [\#7330](https://github.com/netdata/netdata/pull/7330) ([cakrit](https://github.com/cakrit))
- installer: add missing trailing backslash [\#7326](https://github.com/netdata/netdata/pull/7326) ([oxplot](https://github.com/oxplot))
- Fine tune various alarm values. [\#7322](https://github.com/netdata/netdata/pull/7322) ([Ferroin](https://github.com/Ferroin))
- - Retrieve current affinity of the process and make sure not to [\#7318](https://github.com/netdata/netdata/pull/7318) ([stelfrag](https://github.com/stelfrag))
- Updating the Travis pipeline \(issue 7189\) [\#7312](https://github.com/netdata/netdata/pull/7312) ([amoss](https://github.com/amoss))
- CMocka tests for Issue 7274 [\#7308](https://github.com/netdata/netdata/pull/7308) ([amoss](https://github.com/amoss))
- Fix missing streaming when slave has SSL activated. [\#7306](https://github.com/netdata/netdata/pull/7306) ([thiagoftsm](https://github.com/thiagoftsm))
- apps.plugin: add process group for git-related processes [\#7289](https://github.com/netdata/netdata/pull/7289) ([nodiscc](https://github.com/nodiscc))
- container-engines: add balena\* to apps\_group.conf [\#7287](https://github.com/netdata/netdata/pull/7287) ([xginn8](https://github.com/xginn8))
- Initial CMocka testing against web\_client.c \(issue \#7229\). [\#7264](https://github.com/netdata/netdata/pull/7264) ([amoss](https://github.com/amoss))
- Remove documentation about kickstart-static64.sh and netdata updater [\#7262](https://github.com/netdata/netdata/pull/7262) ([knatsakis](https://github.com/knatsakis))
- Upgraded swagger docs from Dolphin tool. [\#7257](https://github.com/netdata/netdata/pull/7257) ([amoss](https://github.com/amoss))
- web\_log: treat 401 Unauthorized requests as successful [\#7256](https://github.com/netdata/netdata/pull/7256) ([amichelic](https://github.com/amichelic))
- Makefile.am files indentation [\#7252](https://github.com/netdata/netdata/pull/7252) ([knatsakis](https://github.com/knatsakis))
- Update SYN cookie alarm to be less aggressive. [\#7250](https://github.com/netdata/netdata/pull/7250) ([Ferroin](https://github.com/Ferroin))
- netdata/docs: netdata installer documentation minor nit [\#7246](https://github.com/netdata/netdata/pull/7246) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Ownership and permissions of /etc/netdata [\#7244](https://github.com/netdata/netdata/pull/7244) ([knatsakis](https://github.com/knatsakis))
- fix\_irc\_notification: Remove line break from message [\#7243](https://github.com/netdata/netdata/pull/7243) ([thiagoftsm](https://github.com/thiagoftsm))
- Typo [\#7242](https://github.com/netdata/netdata/pull/7242) ([cherouvim](https://github.com/cherouvim))
- .travis.yml: Prevent nightly jobs from timing out \(again\) [\#7238](https://github.com/netdata/netdata/pull/7238) ([knatsakis](https://github.com/knatsakis))
- Disable pagetypeinfo by default [\#7230](https://github.com/netdata/netdata/pull/7230) ([vlvkobal](https://github.com/vlvkobal))
- postgres: do not return cached data [\#7228](https://github.com/netdata/netdata/pull/7228) ([ilyam8](https://github.com/ilyam8))
- rabbitmq: handle "disk\_free": "disk\_free\_monitoring\_disabled" [\#7226](https://github.com/netdata/netdata/pull/7226) ([ilyam8](https://github.com/ilyam8))
- Clarify database engine/RAM in getting started guide [\#7225](https://github.com/netdata/netdata/pull/7225) ([joelhans](https://github.com/joelhans))
- database: include limits.h before using LONG\_MAX [\#7224](https://github.com/netdata/netdata/pull/7224) ([mniestroj](https://github.com/mniestroj))
- UrlService: allow to skip tls\_verify for http scheme [\#7223](https://github.com/netdata/netdata/pull/7223) ([ilyam8](https://github.com/ilyam8))
- Fix counter reset detection [\#7220](https://github.com/netdata/netdata/pull/7220) ([mfundul](https://github.com/mfundul))
- .travis.yml: Increase timeout for docker image builds to 20 minutes [\#7214](https://github.com/netdata/netdata/pull/7214) ([knatsakis](https://github.com/knatsakis))
- Building a fuzzer against the API \(issue \#7163\) [\#7210](https://github.com/netdata/netdata/pull/7210) ([amoss](https://github.com/amoss))
- Suggest using /var/run/netdata for the unix socket [\#7206](https://github.com/netdata/netdata/pull/7206) ([CtrlAltDel64](https://github.com/CtrlAltDel64))
- netdata-installer.sh follow-up based on \#7060 review [\#7200](https://github.com/netdata/netdata/pull/7200) ([knatsakis](https://github.com/knatsakis))
- Added GA links to new documents [\#7194](https://github.com/netdata/netdata/pull/7194) ([joelhans](https://github.com/joelhans))
- Fix sizeof inside callocz [\#7187](https://github.com/netdata/netdata/pull/7187) ([thiagoftsm](https://github.com/thiagoftsm))
- TimescaleDB connection page [\#7180](https://github.com/netdata/netdata/pull/7180) ([joelhans](https://github.com/joelhans))
- contrib/debian: Fix typo in Description [\#7154](https://github.com/netdata/netdata/pull/7154) ([arkamar](https://github.com/arkamar))
- Update alarm-notify.sh to enable IRC notifications [\#7148](https://github.com/netdata/netdata/pull/7148) ([Strykar](https://github.com/Strykar))
- detect if the disk cannot keep up with data collection [\#7139](https://github.com/netdata/netdata/pull/7139) ([mfundul](https://github.com/mfundul))
- Fixing DNS-lookup performance issue on FreeBSD. [\#7132](https://github.com/netdata/netdata/pull/7132) ([amoss](https://github.com/amoss))
- Add user information to MySQL Python module documentation [\#7128](https://github.com/netdata/netdata/pull/7128) ([prhomhyse](https://github.com/prhomhyse))
- Results of the spike investigation into CMake. [\#7114](https://github.com/netdata/netdata/pull/7114) ([amoss](https://github.com/amoss))
- xenstat.plugin: check xenstat\_vbd\_error presence [\#7103](https://github.com/netdata/netdata/pull/7103) ([arkamar](https://github.com/arkamar))
- Fix to docker-compose+Caddy installation [\#7088](https://github.com/netdata/netdata/pull/7088) ([joelhans](https://github.com/joelhans))
- Second part of fix for \#7040 [\#7083](https://github.com/netdata/netdata/pull/7083) ([knatsakis](https://github.com/knatsakis))
- kickstart-static64.sh passes --auto-update to netdata-latest.gz.run [\#7076](https://github.com/netdata/netdata/pull/7076) ([knatsakis](https://github.com/knatsakis))

## [v1.18.1](https://github.com/netdata/netdata/tree/v1.18.1) (2019-10-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.18.0...v1.18.1)

**Merged pull requests:**

- Fix build when CMocka isn't installed [\#7129](https://github.com/netdata/netdata/pull/7129) ([vlvkobal](https://github.com/vlvkobal))
- Fixing broken links in docs [\#7123](https://github.com/netdata/netdata/pull/7123) ([joelhans](https://github.com/joelhans))
- Convert recursion timings to miliseconds. [\#7121](https://github.com/netdata/netdata/pull/7121) ([Ferroin](https://github.com/Ferroin))
- Fix upgrade path from v1.17.1 to v1.18.x for deb packages [\#7118](https://github.com/netdata/netdata/pull/7118) ([knatsakis](https://github.com/knatsakis))
- Fix CPU charts in apps plugin on FreeBSD [\#7115](https://github.com/netdata/netdata/pull/7115) ([vlvkobal](https://github.com/vlvkobal))
- unbound: fix init [\#7112](https://github.com/netdata/netdata/pull/7112) ([ilyam8](https://github.com/ilyam8))
- Add VMware VMXNET3 driver to the default interafaces list [\#7109](https://github.com/netdata/netdata/pull/7109) ([samm-git](https://github.com/samm-git))
- megacli: search binary and sudo check fix [\#7108](https://github.com/netdata/netdata/pull/7108) ([ilyam8](https://github.com/ilyam8))
- Run the triggers for deb and rpm package build in separate stages [\#7105](https://github.com/netdata/netdata/pull/7105) ([knatsakis](https://github.com/knatsakis))
- Fix segmentation fault in FreeBSD when statsd is disabled [\#7102](https://github.com/netdata/netdata/pull/7102) ([vlvkobal](https://github.com/vlvkobal))
- Clang warnings [\#7090](https://github.com/netdata/netdata/pull/7090) ([thiagoftsm](https://github.com/thiagoftsm))
- SimpleService: change chart suppress msg level to info [\#7085](https://github.com/netdata/netdata/pull/7085) ([ilyam8](https://github.com/ilyam8))
- 7040 enable stable channel option [\#7082](https://github.com/netdata/netdata/pull/7082) ([knatsakis](https://github.com/knatsakis))
- fix\(freeipmi\): Update frequency config check [\#7078](https://github.com/netdata/netdata/pull/7078) ([stevenh](https://github.com/stevenh))

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
