# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.19.0...HEAD)

**Merged pull requests:**

- .travis.yml: Remove CentOS 6 package building and lifecycle tests [\#7425](https://github.com/netdata/netdata/pull/7425) ([knatsakis](https://github.com/knatsakis))
- Docs: Fixes to new health documentation structure [\#7419](https://github.com/netdata/netdata/pull/7419) ([joelhans](https://github.com/joelhans))
- installer: include go.d.plugin version v0.12.0 [\#7418](https://github.com/netdata/netdata/pull/7418) ([ilyam8](https://github.com/ilyam8))
- Attempt to fix broken docs builds [\#7409](https://github.com/netdata/netdata/pull/7409) ([joelhans](https://github.com/joelhans))
- monit: overwrite \_\_eq\_\_, \_\_ne\_\_ in child classes \(lgtm warnings\) [\#7387](https://github.com/netdata/netdata/pull/7387) ([ilyam8](https://github.com/ilyam8))
- smartd\_log: change ATTR5 chart algorithm to absolute [\#7384](https://github.com/netdata/netdata/pull/7384) ([ilyam8](https://github.com/ilyam8))
- Do not crash when logging UTF-8 data in Python 2 [\#7376](https://github.com/netdata/netdata/pull/7376) ([vzDevelopment](https://github.com/vzDevelopment))
- nvidia-smi: not loop mode [\#7372](https://github.com/netdata/netdata/pull/7372) ([ilyam8](https://github.com/ilyam8))
- Fix typo and markup in packaging/installer README [\#7368](https://github.com/netdata/netdata/pull/7368) ([nabijaczleweli](https://github.com/nabijaczleweli))
- Tutorials to support v1.19 release [\#7359](https://github.com/netdata/netdata/pull/7359) ([joelhans](https://github.com/joelhans))
- Update python.d README  [\#7357](https://github.com/netdata/netdata/pull/7357) ([OdysLam](https://github.com/OdysLam))
- Documentation on per-chart configuration options [\#7345](https://github.com/netdata/netdata/pull/7345) ([joelhans](https://github.com/joelhans))
- Fixing errors in plugins.d/README.md [\#7340](https://github.com/netdata/netdata/pull/7340) ([joelhans](https://github.com/joelhans))
- Health: Proposed restructuring of health documentation [\#7329](https://github.com/netdata/netdata/pull/7329) ([joelhans](https://github.com/joelhans))
- Implement netdata command server and cli tool [\#7325](https://github.com/netdata/netdata/pull/7325) ([mfundul](https://github.com/mfundul))
- proc.plugin: add pressure stall information [\#7209](https://github.com/netdata/netdata/pull/7209) ([hexchain](https://github.com/hexchain))
- Fixing linter errors in packaging/docker/README [\#7199](https://github.com/netdata/netdata/pull/7199) ([joelhans](https://github.com/joelhans))

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
- \[collector/proc.plugin\] Add /proc/pagetypeinfo parser [\#6843](https://github.com/netdata/netdata/pull/6843) ([Saruspete](https://github.com/Saruspete))

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
- zookeeper and hdfs: alarms and dashboard\_info [\#6927](https://github.com/netdata/netdata/pull/6927) ([ilyam8](https://github.com/ilyam8))
- netdata.spec.in: Do not build CUPS plugin subpackage on CentOS 6 and CentOS 7 [\#6926](https://github.com/netdata/netdata/pull/6926) ([knatsakis](https://github.com/knatsakis))
- Fix remark lint for Contrib  [\#6921](https://github.com/netdata/netdata/pull/6921) ([prhomhyse](https://github.com/prhomhyse))
- Fix remark warnings for Daemon README [\#6920](https://github.com/netdata/netdata/pull/6920) ([prhomhyse](https://github.com/prhomhyse))
- Fix Remark Lint Warnings for Backends [\#6917](https://github.com/netdata/netdata/pull/6917) ([prhomhyse](https://github.com/prhomhyse))
- Suggest using /run or /var/run for the unix socket [\#6916](https://github.com/netdata/netdata/pull/6916) ([cakrit](https://github.com/cakrit))
- Improve documentation for the SNMP collector [\#6915](https://github.com/netdata/netdata/pull/6915) ([cakrit](https://github.com/cakrit))
- netdata/ci: nits and fixes around package release workflow [\#6914](https://github.com/netdata/netdata/pull/6914) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Detect deadlock in dbengine page cache [\#6911](https://github.com/netdata/netdata/pull/6911) ([mfundul](https://github.com/mfundul))
- Correct read length of silencers file [\#6909](https://github.com/netdata/netdata/pull/6909) ([cakrit](https://github.com/cakrit))
- netdata/ci: fix branch check [\#6905](https://github.com/netdata/netdata/pull/6905) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- \#3925 implementation [\#6903](https://github.com/netdata/netdata/pull/6903) ([underhood](https://github.com/underhood))
- netdata/packaging: remove rhel7 - i386, until its settled from bug \#6849 [\#6902](https://github.com/netdata/netdata/pull/6902) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Improve changelog generation and add it back to the pipeline [\#6900](https://github.com/netdata/netdata/pull/6900) ([cakrit](https://github.com/cakrit))
- \[collector/slabinfo\] Fix pagesize not defined in non-x86 arches [\#6897](https://github.com/netdata/netdata/pull/6897) ([Saruspete](https://github.com/Saruspete))
- Permit x-auth-token in Access-Control-Allow-Headers [\#6894](https://github.com/netdata/netdata/pull/6894) ([cakrit](https://github.com/cakrit))
- netdata/packaging: fix kickstart-static64 argument parsing [\#6892](https://github.com/netdata/netdata/pull/6892) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Change the log level for chart updates [\#6887](https://github.com/netdata/netdata/pull/6887) ([vlvkobal](https://github.com/vlvkobal))
- Resolve all Kubernetes container names [\#6885](https://github.com/netdata/netdata/pull/6885) ([cakrit](https://github.com/cakrit))
- Update docs for offline install [\#6884](https://github.com/netdata/netdata/pull/6884) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Remove Dollar sign from Bash code in documentation and fix remark-lint warnings [\#6880](https://github.com/netdata/netdata/pull/6880) ([prhomhyse](https://github.com/prhomhyse))
- Markdown syntax fixes for MDX parser [\#6877](https://github.com/netdata/netdata/pull/6877) ([joelhans](https://github.com/joelhans))
- fix LGTM warnings [\#6875](https://github.com/netdata/netdata/pull/6875) ([jacekkolasa](https://github.com/jacekkolasa))
- Update python.d module checklist to match the current paths and build system. [\#6874](https://github.com/netdata/netdata/pull/6874) ([Ferroin](https://github.com/Ferroin))
- installer: include go.d.plugin version v0.9.0 [\#6872](https://github.com/netdata/netdata/pull/6872) ([ilyam8](https://github.com/ilyam8))
- Instructions for simple SMTP transport [\#6870](https://github.com/netdata/netdata/pull/6870) ([cakrit](https://github.com/cakrit))
- Add example for prometheus archiving source parameter [\#6869](https://github.com/netdata/netdata/pull/6869) ([cakrit](https://github.com/cakrit))
- dont redirect when redirectURI is the same [\#6868](https://github.com/netdata/netdata/pull/6868) ([jacekkolasa](https://github.com/jacekkolasa))
- /var/lib/netdata/registry was being left behind after purge [\#6867](https://github.com/netdata/netdata/pull/6867) ([davent](https://github.com/davent))
- python.d.plugin: no job config build fix [\#6856](https://github.com/netdata/netdata/pull/6856) ([ilyam8](https://github.com/ilyam8))
- Resolve broken links in The standard web dashboard doc [\#6854](https://github.com/netdata/netdata/pull/6854) ([prhomhyse](https://github.com/prhomhyse))
- netdata/packaging: Bring on board two scripts that build libuv and judy from source [\#6850](https://github.com/netdata/netdata/pull/6850) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: nits and fixes for packaging [\#6842](https://github.com/netdata/netdata/pull/6842) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: nit - missed trailing slash [\#6840](https://github.com/netdata/netdata/pull/6840) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: Ensure that we do not mess with CI tooling, when building stable [\#6838](https://github.com/netdata/netdata/pull/6838) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: we didnt fix changelog handling, fixes and nits now [\#6837](https://github.com/netdata/netdata/pull/6837) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Add news of 1.17.1 to README [\#6836](https://github.com/netdata/netdata/pull/6836) ([cakrit](https://github.com/cakrit))
- netdata/packaging: no need to overengineer with these checks [\#6834](https://github.com/netdata/netdata/pull/6834) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Buffer overflow [\#6817](https://github.com/netdata/netdata/pull/6817) ([amoss](https://github.com/amoss))
- Docs: Overhaul of Getting started guide [\#6811](https://github.com/netdata/netdata/pull/6811) ([joelhans](https://github.com/joelhans))
- NPM Packages version update [\#6801](https://github.com/netdata/netdata/pull/6801) ([prhomhyse](https://github.com/prhomhyse))
- Collector slabinfo [\#6800](https://github.com/netdata/netdata/pull/6800) ([Saruspete](https://github.com/Saruspete))
- Fix some errors reported by Coverity [\#6797](https://github.com/netdata/netdata/pull/6797) ([thiagoftsm](https://github.com/thiagoftsm))
- Allow hostnames in Access Control Lists [\#6796](https://github.com/netdata/netdata/pull/6796) ([amoss](https://github.com/amoss))
- update grep to be more specific [\#6794](https://github.com/netdata/netdata/pull/6794) ([n0coast](https://github.com/n0coast))
- Common pattern for web and alarms together with two bug fixes [\#6783](https://github.com/netdata/netdata/pull/6783) ([thiagoftsm](https://github.com/thiagoftsm))
- Changes to launching the python.d plugin aggregator. [\#6781](https://github.com/netdata/netdata/pull/6781) ([amoss](https://github.com/amoss))
- vcsa collector: charts descritpion and alarms [\#6772](https://github.com/netdata/netdata/pull/6772) ([ilyam8](https://github.com/ilyam8))
- netdata/packaging: Introduce separate CUPS package for debian distributions [\#6724](https://github.com/netdata/netdata/pull/6724) ([paulkatsoulakis](https://github.com/paulkatsoulakis))

## [v1.17.1](https://github.com/netdata/netdata/tree/v1.17.1) (2019-09-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.17.0...v1.17.1)

**Merged pull requests:**

- netdata/packaging: fix ubuntu/xenial runtime dependencies [\#6825](https://github.com/netdata/netdata/pull/6825) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/ci: Force last good version of git-semver, they broke it [\#6820](https://github.com/netdata/netdata/pull/6820) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Stress test insertions into dbengine and bugfixes [\#6814](https://github.com/netdata/netdata/pull/6814) ([mfundul](https://github.com/mfundul))
- netdata/ci: Fix author on triggering commits for packaging [\#6813](https://github.com/netdata/netdata/pull/6813) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: remove fedora/28, is no longer available [\#6808](https://github.com/netdata/netdata/pull/6808) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/ci: second batch of fixes for coverity scan script and others [\#6804](https://github.com/netdata/netdata/pull/6804) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: work around redhat complaining on build-id binary [\#6792](https://github.com/netdata/netdata/pull/6792) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: fix changelog generation failing the build [\#6778](https://github.com/netdata/netdata/pull/6778) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- netdata/packaging: override control file for debian/buster [\#6777](https://github.com/netdata/netdata/pull/6777) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- \(Documentation\) fix pfsense instructions and links [\#6768](https://github.com/netdata/netdata/pull/6768) ([Fohdeesha](https://github.com/Fohdeesha))
- netdata/packaging: Trigger stable package generation upon release process [\#6766](https://github.com/netdata/netdata/pull/6766) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- update cache hashes for js and css [\#6756](https://github.com/netdata/netdata/pull/6756) ([jacekkolasa](https://github.com/jacekkolasa))
- \[libnetdata/thread\] Set thread name from tag [\#6745](https://github.com/netdata/netdata/pull/6745) ([Saruspete](https://github.com/Saruspete))
- sidebar-info update - DB engine [\#6744](https://github.com/netdata/netdata/pull/6744) ([jacekkolasa](https://github.com/jacekkolasa))

## [v1.17.0](https://github.com/netdata/netdata/tree/v1.17.0) (2019-09-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.16.1...v1.17.0)

**Merged pull requests:**

- Remove changelog generation from release, as it keeps breaking [\#6761](https://github.com/netdata/netdata/pull/6761) ([cakrit](https://github.com/cakrit))
- Skip issues from release changelog [\#6759](https://github.com/netdata/netdata/pull/6759) ([cakrit](https://github.com/cakrit))
- Increase minimum release for changelog [\#6758](https://github.com/netdata/netdata/pull/6758) ([cakrit](https://github.com/cakrit))
- netdata/packaging: Add python3-lxc dependency [\#6753](https://github.com/netdata/netdata/pull/6753) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- make coverity-scan.sh usable by hand [\#6747](https://github.com/netdata/netdata/pull/6747) ([ktsaou](https://github.com/ktsaou))
- netdata/packaging: fix coverity problem in travis \(2 issues\) [\#6743](https://github.com/netdata/netdata/pull/6743) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Add command-line option descriptions for apps.plugin [\#6738](https://github.com/netdata/netdata/pull/6738) ([vlvkobal](https://github.com/vlvkobal))
- netdata/packaging: Add purging logic for package cloud repositories [\#6732](https://github.com/netdata/netdata/pull/6732) ([paulkatsoulakis](https://github.com/paulkatsoulakis))
- Fix corrupted transaction payload handling [\#6731](https://github.com/netdata/netdata/pull/6731) ([mfundul](https://github.com/mfundul))
- Netdata-installer warning removed [\#6715](https://github.com/netdata/netdata/pull/6715) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove of unecessary NULL web server [\#6714](https://github.com/netdata/netdata/pull/6714) ([thiagoftsm](https://github.com/thiagoftsm))

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
