# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.19.0...HEAD)

**Merged pull requests:**

- Missing extern [\#7877](https://github.com/netdata/netdata/pull/7877) ([thiagoftsm](https://github.com/thiagoftsm))
- collectors/python.d/phpfpm: fix readme and per process chart titles [\#7876](https://github.com/netdata/netdata/pull/7876) ([ilyam8](https://github.com/ilyam8))
- .travis.yml: Add -fno-common to CFLAGS [\#7870](https://github.com/netdata/netdata/pull/7870) ([candrews](https://github.com/candrews))
- Add disk size detection to system-info.sh. [\#7866](https://github.com/netdata/netdata/pull/7866) ([Ferroin](https://github.com/Ferroin))
- /collectors/python.d: remove unbound module [\#7853](https://github.com/netdata/netdata/pull/7853) ([ilyam8](https://github.com/ilyam8))
- python.d/retroshare: add readme [\#7849](https://github.com/netdata/netdata/pull/7849) ([ilyam8](https://github.com/ilyam8))
- Fixes and improves the installer/updater shell scripts. [\#7847](https://github.com/netdata/netdata/pull/7847) ([prologic](https://github.com/prologic))
- Adds support for opting out of telemetry via the DO\_NOT\_TRACK envirnment variable [\#7846](https://github.com/netdata/netdata/pull/7846) ([prologic](https://github.com/prologic))
- Fixed typo in README [\#7843](https://github.com/netdata/netdata/pull/7843) ([Jiab77](https://github.com/Jiab77))
- Docs: Overhaul of installation documentation [\#7841](https://github.com/netdata/netdata/pull/7841) ([joelhans](https://github.com/joelhans))
- Update collect-apache-nginx-web-logs.md to deprecated [\#7835](https://github.com/netdata/netdata/pull/7835) ([joelhans](https://github.com/joelhans))
- collectors/python.d: format modules code [\#7832](https://github.com/netdata/netdata/pull/7832) ([ilyam8](https://github.com/ilyam8))
- Remove all refernces to .keep files [\#7829](https://github.com/netdata/netdata/pull/7829) ([prologic](https://github.com/prologic))
- Adds ReviewDog CI checks for JavaScript [\#7828](https://github.com/netdata/netdata/pull/7828) ([prologic](https://github.com/prologic))
- Adds ReviewDog CI checks for Golang [\#7827](https://github.com/netdata/netdata/pull/7827) ([prologic](https://github.com/prologic))
- Don't remove groups/users in Debian postrm [\#7817](https://github.com/netdata/netdata/pull/7817) ([prologic](https://github.com/prologic))
- node.d/snmp.node.js: format code [\#7816](https://github.com/netdata/netdata/pull/7816) ([ilyam8](https://github.com/ilyam8))
- Improve the system-info.sh script to report CPU and RAM meta-data. [\#7815](https://github.com/netdata/netdata/pull/7815) ([Ferroin](https://github.com/Ferroin))
- Attempt to use system service manager to shut down Netdata. [\#7814](https://github.com/netdata/netdata/pull/7814) ([Ferroin](https://github.com/Ferroin))
- bug\_report improvements [\#7805](https://github.com/netdata/netdata/pull/7805) ([ilyam8](https://github.com/ilyam8))
- node.d/snmp: snmpv3 support [\#7802](https://github.com/netdata/netdata/pull/7802) ([ilyam8](https://github.com/ilyam8))
- Fixes install on FreeBSD systems with non GNU sed \(do't use -i\) [\#7796](https://github.com/netdata/netdata/pull/7796) ([prologic](https://github.com/prologic))
- Adds reviewdog/shellcheck to CI via Github Actions on changed shell scripts in PRs [\#7795](https://github.com/netdata/netdata/pull/7795) ([prologic](https://github.com/prologic))
- Fixes Source0 URL in RPM spec [\#7794](https://github.com/netdata/netdata/pull/7794) ([prologic](https://github.com/prologic))
- Better systemd service file [\#7790](https://github.com/netdata/netdata/pull/7790) ([amishmm](https://github.com/amishmm))
- Fix unit tests for the exporting engine [\#7784](https://github.com/netdata/netdata/pull/7784) ([vlvkobal](https://github.com/vlvkobal))
- Remove unnessecary `echo` call in updater. [\#7783](https://github.com/netdata/netdata/pull/7783) ([Ferroin](https://github.com/Ferroin))
- Fix CSV -\> SSV in docs [\#7782](https://github.com/netdata/netdata/pull/7782) ([cosmix](https://github.com/cosmix))
- Fix a Coverity issue [\#7780](https://github.com/netdata/netdata/pull/7780) ([vlvkobal](https://github.com/vlvkobal))
- Fix libuv IPC pipe cleanup problem [\#7778](https://github.com/netdata/netdata/pull/7778) ([mfundul](https://github.com/mfundul))
- Add a missing parameter to the allmetrics endpoint in Swagger Editor [\#7776](https://github.com/netdata/netdata/pull/7776) ([vlvkobal](https://github.com/vlvkobal))
- Issue 7488 docker labels [\#7770](https://github.com/netdata/netdata/pull/7770) ([amoss](https://github.com/amoss))
- Limit PR labeler runs to the main repo. [\#7768](https://github.com/netdata/netdata/pull/7768) ([Ferroin](https://github.com/Ferroin))
- Add an environment variable check to Travis configuration to allow disabling nightlies. [\#7765](https://github.com/netdata/netdata/pull/7765) ([Ferroin](https://github.com/Ferroin))
- add swagger docu for `fixed\_width\_lbl` and `fixed\_width\_val` [\#7764](https://github.com/netdata/netdata/pull/7764) ([underhood](https://github.com/underhood))
- Fix the formatting of the trailer line in the Debian changelog template. [\#7763](https://github.com/netdata/netdata/pull/7763) ([Ferroin](https://github.com/Ferroin))
- Filter out lxc cgroups which are not useful [\#7760](https://github.com/netdata/netdata/pull/7760) ([vlvkobal](https://github.com/vlvkobal))
- Small updates to dash.html [\#7757](https://github.com/netdata/netdata/pull/7757) ([tnyeanderson](https://github.com/tnyeanderson))
- Improve styling of documentation site and use Algolia search [\#7753](https://github.com/netdata/netdata/pull/7753) ([joelhans](https://github.com/joelhans))
- multiple files: fix typos [\#7752](https://github.com/netdata/netdata/pull/7752) ([schneiderl](https://github.com/schneiderl))
- on cloud error, inform user to update their netdata. [\#7750](https://github.com/netdata/netdata/pull/7750) ([jacekkolasa](https://github.com/jacekkolasa))
- Update stop-notifications-alarms.md [\#7737](https://github.com/netdata/netdata/pull/7737) ([yasharne](https://github.com/yasharne))
- Adds Docker based build system for Binary Packages, CI/CD, Smoke Testing and Development. [\#7735](https://github.com/netdata/netdata/pull/7735) ([prologic](https://github.com/prologic))
- Do not alert the \#automation channel on checksum failures that will fail a PR in CI anyway [\#7733](https://github.com/netdata/netdata/pull/7733) ([prologic](https://github.com/prologic))
- installer: include go.d.plugin version v0.14.1 [\#7732](https://github.com/netdata/netdata/pull/7732) ([ilyam8](https://github.com/ilyam8))
- Fix a check for nfnetlink\_conntrack.h [\#7727](https://github.com/netdata/netdata/pull/7727) ([vlvkobal](https://github.com/vlvkobal))
- Fixes support for read-only /lib on SystemD systems like CoreOS in  kickstart static64 [\#7726](https://github.com/netdata/netdata/pull/7726) ([prologic](https://github.com/prologic))
- Cleanup packaging/makeself/build-x86\_64-static.sh to use /bin/sh and remove use of sudo [\#7725](https://github.com/netdata/netdata/pull/7725) ([prologic](https://github.com/prologic))
- Add Korean translation to docs [\#7723](https://github.com/netdata/netdata/pull/7723) ([cakrit](https://github.com/cakrit))
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
- Fix install permissions [\#7632](https://github.com/netdata/netdata/pull/7632) ([Ferroin](https://github.com/Ferroin))
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
- Skip unit testing during CI when it's not needed. [\#7559](https://github.com/netdata/netdata/pull/7559) ([Ferroin](https://github.com/Ferroin))
- Send host labels via exporting connectors  [\#7554](https://github.com/netdata/netdata/pull/7554) ([vlvkobal](https://github.com/vlvkobal))
- \[github/templates\] Add samples cmds to get OS env [\#7550](https://github.com/netdata/netdata/pull/7550) ([Saruspete](https://github.com/Saruspete))
- Stream with labels [\#7549](https://github.com/netdata/netdata/pull/7549) ([thiagoftsm](https://github.com/thiagoftsm))
- Alarm Log labels [\#7548](https://github.com/netdata/netdata/pull/7548) ([thiagoftsm](https://github.com/thiagoftsm))
- Limit 'support activities on main branch' to main repo. [\#7543](https://github.com/netdata/netdata/pull/7543) ([Ferroin](https://github.com/Ferroin))
- Docs: Linter fixes for main README [\#7526](https://github.com/netdata/netdata/pull/7526) ([joelhans](https://github.com/joelhans))
- Agent claiming [\#7525](https://github.com/netdata/netdata/pull/7525) ([mfundul](https://github.com/mfundul))
- The step-by-step Netdata tutorial [\#7489](https://github.com/netdata/netdata/pull/7489) ([joelhans](https://github.com/joelhans))
- silencers\_info: Change error to info [\#7479](https://github.com/netdata/netdata/pull/7479) ([thiagoftsm](https://github.com/thiagoftsm))
- Add anon tracking notice for installers [\#7437](https://github.com/netdata/netdata/pull/7437) ([ncmans](https://github.com/ncmans))
- Docs: Tweaks and linter fixes to contributing guidelines [\#7407](https://github.com/netdata/netdata/pull/7407) ([joelhans](https://github.com/joelhans))
- packaging: Set default release channel to stable for gh releases [\#7399](https://github.com/netdata/netdata/pull/7399) ([ncmans](https://github.com/ncmans))
- network interface speed, duplex, operstate \#5989 [\#7395](https://github.com/netdata/netdata/pull/7395) ([stelfrag](https://github.com/stelfrag))
- Fix typos in documentation [\#7375](https://github.com/netdata/netdata/pull/7375) ([rex4539](https://github.com/rex4539))
- Add release channel customization to docker build [\#7373](https://github.com/netdata/netdata/pull/7373) ([ncmans](https://github.com/ncmans))

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
