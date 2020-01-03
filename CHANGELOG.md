# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.19.0...HEAD)

**Merged pull requests:**

- \[Fix\] remove pthread\_setname\_np segfault on musl [\#7664](https://github.com/netdata/netdata/pull/7664) ([Saruspete](https://github.com/Saruspete))
- Bump handlebars from 4.2.0 to 4.5.3 [\#7654](https://github.com/netdata/netdata/pull/7654) ([dependabot[bot]](https://github.com/apps/dependabot))
- \[libnetdata/threads\] Change log level on error [\#7653](https://github.com/netdata/netdata/pull/7653) ([Saruspete](https://github.com/Saruspete))
- python.d logger: unicode\_str handle TypeError [\#7645](https://github.com/netdata/netdata/pull/7645) ([ilyam8](https://github.com/ilyam8))
- redis: populate `keys\_redis` chart in runtime [\#7639](https://github.com/netdata/netdata/pull/7639) ([ilyam8](https://github.com/ilyam8))
- Minor: Documentation Typo alamrs -\> alarms [\#7637](https://github.com/netdata/netdata/pull/7637) ([underhood](https://github.com/underhood))
- Add Ubuntu 19.10 to packaging and lifecycle checks. [\#7629](https://github.com/netdata/netdata/pull/7629) ([Ferroin](https://github.com/Ferroin))
- python.d logger: do not unicode decode if it is already unicode [\#7614](https://github.com/netdata/netdata/pull/7614) ([ilyam8](https://github.com/ilyam8))
- Fix a warning in prometheus remote write backend [\#7609](https://github.com/netdata/netdata/pull/7609) ([vlvkobal](https://github.com/vlvkobal))
- python.d.plugin: UrlService bytes decode, logger unicode encoding fix [\#7601](https://github.com/netdata/netdata/pull/7601) ([ilyam8](https://github.com/ilyam8))
- Adjust alarm labels [\#7600](https://github.com/netdata/netdata/pull/7600) ([thiagoftsm](https://github.com/thiagoftsm))
- Add netdata-claim.sh to the RPM spec file. [\#7592](https://github.com/netdata/netdata/pull/7592) ([Ferroin](https://github.com/Ferroin))
- Set standard name to non-libnetdata threads \(libuv, pthread\) [\#7584](https://github.com/netdata/netdata/pull/7584) ([Saruspete](https://github.com/Saruspete))
- Docs: add configuration details for vhost about DOSPageCount to Apache proxy guide [\#7582](https://github.com/netdata/netdata/pull/7582) ([kkoomen](https://github.com/kkoomen))
- CODEOWNERS: Replace @netdata/automation with individual team members [\#7581](https://github.com/netdata/netdata/pull/7581) ([knatsakis](https://github.com/knatsakis))
- Fix not detecting more than one adapter in hpssa collector [\#7580](https://github.com/netdata/netdata/pull/7580) ([gnoddep](https://github.com/gnoddep))
- Docs: Add notice about mod\_evasive to Apache proxy guide [\#7578](https://github.com/netdata/netdata/pull/7578) ([joelhans](https://github.com/joelhans))
- installer: include go.d.plugin version v0.13.0 [\#7574](https://github.com/netdata/netdata/pull/7574) ([ilyam8](https://github.com/ilyam8))
- Fix race condition in dbengine [\#7565](https://github.com/netdata/netdata/pull/7565) ([thiagoftsm](https://github.com/thiagoftsm))
- Revert "Fix race condition in dbengine \(\#7533\)" [\#7560](https://github.com/netdata/netdata/pull/7560) ([amoss](https://github.com/amoss))
- Skip unit testing during CI when it's not needed. [\#7559](https://github.com/netdata/netdata/pull/7559) ([Ferroin](https://github.com/Ferroin))
- Cleanup the main exporting engine thread on exit [\#7558](https://github.com/netdata/netdata/pull/7558) ([vlvkobal](https://github.com/vlvkobal))
- \[github/templates\] Add samples cmds to get OS env [\#7550](https://github.com/netdata/netdata/pull/7550) ([Saruspete](https://github.com/Saruspete))
- proc\_pressure: increment fail\_count on read fail [\#7547](https://github.com/netdata/netdata/pull/7547) ([hexchain](https://github.com/hexchain))
- Merge the matrix and jobs keys in Travis config. [\#7544](https://github.com/netdata/netdata/pull/7544) ([Ferroin](https://github.com/Ferroin))
- Limit 'support activities on main branch' to main repo. [\#7543](https://github.com/netdata/netdata/pull/7543) ([Ferroin](https://github.com/Ferroin))
- Fix backend config [\#7538](https://github.com/netdata/netdata/pull/7538) ([vlvkobal](https://github.com/vlvkobal))
- Fix race condition in dbengine [\#7533](https://github.com/netdata/netdata/pull/7533) ([mfundul](https://github.com/mfundul))
- Fix valgrind errors [\#7532](https://github.com/netdata/netdata/pull/7532) ([mfundul](https://github.com/mfundul))
- Update codeowners [\#7530](https://github.com/netdata/netdata/pull/7530) ([knatsakis](https://github.com/knatsakis))
- Agent claiming [\#7525](https://github.com/netdata/netdata/pull/7525) ([mfundul](https://github.com/mfundul))
- Add Fedora 31 CI integrations. [\#7524](https://github.com/netdata/netdata/pull/7524) ([Ferroin](https://github.com/Ferroin))
- Labels issues [\#7515](https://github.com/netdata/netdata/pull/7515) ([amoss](https://github.com/amoss))
- Update Netdata RPM spec file to package netdatacli. [\#7513](https://github.com/netdata/netdata/pull/7513) ([Ferroin](https://github.com/Ferroin))
- Remove `-f` option from `groupdel` in uninstaller. [\#7507](https://github.com/netdata/netdata/pull/7507) ([Ferroin](https://github.com/Ferroin))
- Force the repo name to be lowercase in the tag for docker builds. [\#7506](https://github.com/netdata/netdata/pull/7506) ([Ferroin](https://github.com/Ferroin))
- Inject archived backports repository on Debian Jessie for CI package builds. [\#7495](https://github.com/netdata/netdata/pull/7495) ([Ferroin](https://github.com/Ferroin))
- The step-by-step Netdata tutorial [\#7489](https://github.com/netdata/netdata/pull/7489) ([joelhans](https://github.com/joelhans))
- ci: remove ubuntu trusty 14.04 from build [\#7481](https://github.com/netdata/netdata/pull/7481) ([ncmans](https://github.com/ncmans))
- silencers\_info: Change error to info [\#7479](https://github.com/netdata/netdata/pull/7479) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix race condition with page cache descriptors [\#7478](https://github.com/netdata/netdata/pull/7478) ([mfundul](https://github.com/mfundul))
- Fix missing parenthesis on softnet.conf [\#7476](https://github.com/netdata/netdata/pull/7476) ([Steve8291](https://github.com/Steve8291))
- Fix dbengine dirty page flushing warning [\#7469](https://github.com/netdata/netdata/pull/7469) ([mfundul](https://github.com/mfundul))
- rabbitmq: fix handle\_disabled\_disk\_monitoring [\#7464](https://github.com/netdata/netdata/pull/7464) ([ilyam8](https://github.com/ilyam8))
- sensors: check fix [\#7447](https://github.com/netdata/netdata/pull/7447) ([ilyam8](https://github.com/ilyam8))
- address lgtm alerts [\#7441](https://github.com/netdata/netdata/pull/7441) ([jacekkolasa](https://github.com/jacekkolasa))
- Add anon tracking notice for installers [\#7437](https://github.com/netdata/netdata/pull/7437) ([ncmans](https://github.com/ncmans))
- Docs: Change build process to allow apostrophes in headers [\#7431](https://github.com/netdata/netdata/pull/7431) ([joelhans](https://github.com/joelhans))
- Indicate we no longer build packages for CentOS 6 [\#7430](https://github.com/netdata/netdata/pull/7430) ([ncmans](https://github.com/ncmans))
- .travis.yml: Remove CentOS 6 package building and lifecycle tests [\#7425](https://github.com/netdata/netdata/pull/7425) ([knatsakis](https://github.com/knatsakis))
- Add docs generator directories to .gitignore [\#7421](https://github.com/netdata/netdata/pull/7421) ([joelhans](https://github.com/joelhans))
- Docs: Fixes to new health documentation structure [\#7419](https://github.com/netdata/netdata/pull/7419) ([joelhans](https://github.com/joelhans))
- installer: include go.d.plugin version v0.12.0 [\#7418](https://github.com/netdata/netdata/pull/7418) ([ilyam8](https://github.com/ilyam8))
- Attempt to fix broken docs builds [\#7409](https://github.com/netdata/netdata/pull/7409) ([joelhans](https://github.com/joelhans))
- packaging: Set default release channel to stable for gh releases [\#7399](https://github.com/netdata/netdata/pull/7399) ([ncmans](https://github.com/ncmans))
- fix to wrong instructions during a non-privileged install from installer [\#7393](https://github.com/netdata/netdata/pull/7393) ([julidegulen](https://github.com/julidegulen))
- monit: overwrite \_\_eq\_\_, \_\_ne\_\_ in child classes \(lgtm warnings\) [\#7387](https://github.com/netdata/netdata/pull/7387) ([ilyam8](https://github.com/ilyam8))
- smartd\_log: change ATTR5 chart algorithm to absolute [\#7384](https://github.com/netdata/netdata/pull/7384) ([ilyam8](https://github.com/ilyam8))
- Do not crash when logging UTF-8 data in Python 2 [\#7376](https://github.com/netdata/netdata/pull/7376) ([vzDevelopment](https://github.com/vzDevelopment))
- Fix typos in documentation [\#7375](https://github.com/netdata/netdata/pull/7375) ([rex4539](https://github.com/rex4539))
- nvidia-smi: not loop mode [\#7372](https://github.com/netdata/netdata/pull/7372) ([ilyam8](https://github.com/ilyam8))
- Fix typo and markup in packaging/installer README [\#7368](https://github.com/netdata/netdata/pull/7368) ([nabijaczleweli](https://github.com/nabijaczleweli))
- Tutorials to support v1.19 release [\#7359](https://github.com/netdata/netdata/pull/7359) ([joelhans](https://github.com/joelhans))
- Update python.d README  [\#7357](https://github.com/netdata/netdata/pull/7357) ([OdysLam](https://github.com/OdysLam))
- Documentation on per-chart configuration options [\#7345](https://github.com/netdata/netdata/pull/7345) ([joelhans](https://github.com/joelhans))
- Fixing errors in plugins.d/README.md [\#7340](https://github.com/netdata/netdata/pull/7340) ([joelhans](https://github.com/joelhans))
- Health: Proposed restructuring of health documentation [\#7329](https://github.com/netdata/netdata/pull/7329) ([joelhans](https://github.com/joelhans))
- Implement netdata command server and cli tool [\#7325](https://github.com/netdata/netdata/pull/7325) ([mfundul](https://github.com/mfundul))
- Minor docker related cleanups [\#7240](https://github.com/netdata/netdata/pull/7240) ([knatsakis](https://github.com/knatsakis))
- .travis.yml: Add timestamps to output [\#7239](https://github.com/netdata/netdata/pull/7239) ([knatsakis](https://github.com/knatsakis))
- proc.plugin: add pressure stall information [\#7209](https://github.com/netdata/netdata/pull/7209) ([hexchain](https://github.com/hexchain))
- Fixing linter errors in packaging/docker/README [\#7199](https://github.com/netdata/netdata/pull/7199) ([joelhans](https://github.com/joelhans))
- Updates and grammar fixes to README.md [\#7193](https://github.com/netdata/netdata/pull/7193) ([joelhans](https://github.com/joelhans))
- Add HP Smart Storage Array python plugin [\#7181](https://github.com/netdata/netdata/pull/7181) ([gnoddep](https://github.com/gnoddep))
- Implement the main flow for the Exporting Engine [\#7149](https://github.com/netdata/netdata/pull/7149) ([vlvkobal](https://github.com/vlvkobal))

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
- kickstart: pass options to installer [\#7051](https://github.com/netdata/netdata/pull/7051) ([oxplot](https://github.com/oxplot))
- fixed - cgroup-network-helper doesn't work on Proxmox 6 [\#7037](https://github.com/netdata/netdata/pull/7037) ([vakartel](https://github.com/vakartel))
- telegram: fix broken links, add setup instructions [\#7033](https://github.com/netdata/netdata/pull/7033) ([half-duplex](https://github.com/half-duplex))
- netdata/installer: add missing flags on installer [\#7027](https://github.com/netdata/netdata/pull/7027) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- add support for am2320 sensor [\#7024](https://github.com/netdata/netdata/pull/7024) ([tommybuck](https://github.com/tommybuck))
- mysql: add cluster\_status alarm [\#6989](https://github.com/netdata/netdata/pull/6989) ([ilyam8](https://github.com/ilyam8))
- Netdata not returning correct value for unknow variables [\#6984](https://github.com/netdata/netdata/pull/6984) ([thiagoftsm](https://github.com/thiagoftsm))

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
- Fix problems with names when alarm is created [\#7069](https://github.com/netdata/netdata/pull/7069) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix dbengine not working when mmap fails [\#7065](https://github.com/netdata/netdata/pull/7065) ([mfundul](https://github.com/mfundul))
- Fix typo in health\_alarm\_notify.conf [\#7062](https://github.com/netdata/netdata/pull/7062) ([sz4bi](https://github.com/sz4bi))
- Fix size of a zeroed block [\#7061](https://github.com/netdata/netdata/pull/7061) ([vlvkobal](https://github.com/vlvkobal))
- Partial fix for \#7039 [\#7060](https://github.com/netdata/netdata/pull/7060) ([knatsakis](https://github.com/knatsakis))
- feat\(reaper\): Add process reaper support [\#7059](https://github.com/netdata/netdata/pull/7059) ([stevenh](https://github.com/stevenh))
- Disable slabinfo plugin by default [\#7056](https://github.com/netdata/netdata/pull/7056) ([vlvkobal](https://github.com/vlvkobal))
- Add release 1.18.0 to news [\#7054](https://github.com/netdata/netdata/pull/7054) ([cakrit](https://github.com/cakrit))
- Fix BSD/pfSense documentation [\#7041](https://github.com/netdata/netdata/pull/7041) ([thiagoftsm](https://github.com/thiagoftsm))
- Add dbengine RAM usage statistics [\#7038](https://github.com/netdata/netdata/pull/7038) ([mfundul](https://github.com/mfundul))
- Don't write an HTTP response 204 to logs [\#7035](https://github.com/netdata/netdata/pull/7035) ([vlvkobal](https://github.com/vlvkobal))
- Implement hangouts chat notifications [\#7013](https://github.com/netdata/netdata/pull/7013) ([hendrikhofstadt](https://github.com/hendrikhofstadt))
- Documenting the structure of the data responses. [\#7012](https://github.com/netdata/netdata/pull/7012) ([amoss](https://github.com/amoss))
- Tutorials to support v1.18 features [\#6993](https://github.com/netdata/netdata/pull/6993) ([joelhans](https://github.com/joelhans))
- Add CMocka unit tests [\#6985](https://github.com/netdata/netdata/pull/6985) ([vlvkobal](https://github.com/vlvkobal))

## [v1.18.0](https://github.com/netdata/netdata/tree/v1.18.0) (2019-10-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.17.1...v1.18.0)

**Merged pull requests:**

- netdata: Add knatsakis as codeowner, wherever paulkatsoulakis was [\#7036](https://github.com/netdata/netdata/pull/7036) ([knatsakis](https://github.com/knatsakis))
- rabbitmq: survive lack of vhosts [\#7019](https://github.com/netdata/netdata/pull/7019) ([ilyam8](https://github.com/ilyam8))
- Fix crash on FreeBSD due to do\_dev\_cpu\_temperature stack corruption [\#7014](https://github.com/netdata/netdata/pull/7014) ([samm-git](https://github.com/samm-git))
- Fix handling of illegal metric timestamps in database engine [\#7008](https://github.com/netdata/netdata/pull/7008) ([mfundul](https://github.com/mfundul))
- Fix a resource leak [\#7007](https://github.com/netdata/netdata/pull/7007) ([vlvkobal](https://github.com/vlvkobal))
- Remove hard cap from page cache size to eliminate deadlocks. [\#7006](https://github.com/netdata/netdata/pull/7006) ([mfundul](https://github.com/mfundul))
- Add Portuguese \(Brazil\) as a language option [\#7004](https://github.com/netdata/netdata/pull/7004) ([cakrit](https://github.com/cakrit))
- fix issue \#7002 [\#7003](https://github.com/netdata/netdata/pull/7003) ([OneCodeMonkey](https://github.com/OneCodeMonkey))
- Increase dbengine default cache size [\#6997](https://github.com/netdata/netdata/pull/6997) ([mfundul](https://github.com/mfundul))
- Checklinks fix [\#6994](https://github.com/netdata/netdata/pull/6994) ([cakrit](https://github.com/cakrit))
- Remove warning from Coverity [\#6992](https://github.com/netdata/netdata/pull/6992) ([thiagoftsm](https://github.com/thiagoftsm))
- netdata/installer: allow netdata service install, when docker runs systemd [\#6987](https://github.com/netdata/netdata/pull/6987) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Fixing broken links found via linkchecker [\#6983](https://github.com/netdata/netdata/pull/6983) ([joelhans](https://github.com/joelhans))
- Fix dbengine consistency [\#6979](https://github.com/netdata/netdata/pull/6979) ([mfundul](https://github.com/mfundul))
- Make dbengine the default memory mode [\#6977](https://github.com/netdata/netdata/pull/6977) ([mfundul](https://github.com/mfundul))
- rabbitmq: collect vhosts msg metrics from `/api/vhosts` [\#6976](https://github.com/netdata/netdata/pull/6976) ([ilyam8](https://github.com/ilyam8))
- Fix coverity erro \(CID 349552\) double lock [\#6970](https://github.com/netdata/netdata/pull/6970) ([thiagoftsm](https://github.com/thiagoftsm))
- web api: include family into allmetrics json response [\#6966](https://github.com/netdata/netdata/pull/6966) ([ilyam8](https://github.com/ilyam8))
- elasticsearch: collect metrics from \_cat/indices [\#6965](https://github.com/netdata/netdata/pull/6965) ([ilyam8](https://github.com/ilyam8))
- Reduce overhead during write io [\#6964](https://github.com/netdata/netdata/pull/6964) ([mfundul](https://github.com/mfundul))
- mysql: collect galera cluster metrics [\#6962](https://github.com/netdata/netdata/pull/6962) ([ilyam8](https://github.com/ilyam8))
- Clarification on configuring notification recipients [\#6961](https://github.com/netdata/netdata/pull/6961) ([cakrit](https://github.com/cakrit))
- netdata/packaging: Make spec file more consistent with version dependencies, plus some documentation nits [\#6948](https://github.com/netdata/netdata/pull/6948) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Fix a memory leak [\#6945](https://github.com/netdata/netdata/pull/6945) ([vlvkobal](https://github.com/vlvkobal))
- Fix Remark Lint for READMEs in Database [\#6942](https://github.com/netdata/netdata/pull/6942) ([prhomhyse](https://github.com/prhomhyse))
- Coverity 20190924 [\#6941](https://github.com/netdata/netdata/pull/6941) ([thiagoftsm](https://github.com/thiagoftsm))
- Restore original alignment behaviour of RRDR [\#6938](https://github.com/netdata/netdata/pull/6938) ([mfundul](https://github.com/mfundul))
- minor - check for curl to not get wrong error message [\#6931](https://github.com/netdata/netdata/pull/6931) ([underhood](https://github.com/underhood))
- netdata/packaging: fix broken links on web files, for deb [\#6930](https://github.com/netdata/netdata/pull/6930) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- installer: include go.d.plugin version v0.10.0 [\#6929](https://github.com/netdata/netdata/pull/6929) ([ilyam8](https://github.com/ilyam8))
- python.d: fix log warnings in start\_job [\#6928](https://github.com/netdata/netdata/pull/6928) ([ilyam8](https://github.com/ilyam8))

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
