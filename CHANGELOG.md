# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.2...HEAD)

**Merged pull requests:**

- Regenerate integrations.js [\#16373](https://github.com/netdata/netdata/pull/16373) ([netdatabot](https://github.com/netdatabot))
- docs: remove unused cloud notification methods mds [\#16372](https://github.com/netdata/netdata/pull/16372) ([ilyam8](https://github.com/ilyam8))
- Add configuration documentation for Cloud AWS SNS [\#16371](https://github.com/netdata/netdata/pull/16371) ([car12o](https://github.com/car12o))
- pacakging: add zstd dev to install-required-packages [\#16370](https://github.com/netdata/netdata/pull/16370) ([ilyam8](https://github.com/ilyam8))
- cgroups: collect pids/pids.current [\#16369](https://github.com/netdata/netdata/pull/16369) ([ilyam8](https://github.com/ilyam8))
- docs: Correct time unit for tier 2 explanation [\#16368](https://github.com/netdata/netdata/pull/16368) ([sepek](https://github.com/sepek))
- cgroups: fix throttle\_duration chart context [\#16367](https://github.com/netdata/netdata/pull/16367) ([ilyam8](https://github.com/ilyam8))
- fix system.net when inside lxc  [\#16364](https://github.com/netdata/netdata/pull/16364) ([ilyam8](https://github.com/ilyam8))
- collectors/freeipmi: add ipmi-sensors function [\#16363](https://github.com/netdata/netdata/pull/16363) ([ilyam8](https://github.com/ilyam8))
- Switch charts / chart to use buffer json functions [\#16359](https://github.com/netdata/netdata/pull/16359) ([stelfrag](https://github.com/stelfrag))
- health: put guides into subdirs [\#16358](https://github.com/netdata/netdata/pull/16358) ([ilyam8](https://github.com/ilyam8))
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
- Fix 404s in markdown files [\#16285](https://github.com/netdata/netdata/pull/16285) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#16284](https://github.com/netdata/netdata/pull/16284) ([netdatabot](https://github.com/netdatabot))
- Small optimization of alert queries [\#16282](https://github.com/netdata/netdata/pull/16282) ([MrZammler](https://github.com/MrZammler))
- update go.d version to 0.56.4 [\#16281](https://github.com/netdata/netdata/pull/16281) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.57.0 [\#16277](https://github.com/netdata/netdata/pull/16277) ([ilyam8](https://github.com/ilyam8))
- Remove semicolons from strings [\#16276](https://github.com/netdata/netdata/pull/16276) ([Ancairon](https://github.com/Ancairon))
- Prevent wrong optimization armv7l static build [\#16274](https://github.com/netdata/netdata/pull/16274) ([stelfrag](https://github.com/stelfrag))
- local\_listeners: add cmd args for reading specific files [\#16273](https://github.com/netdata/netdata/pull/16273) ([ilyam8](https://github.com/ilyam8))
- DYNCFG fix REPORT\_JOB\_STATUS streaming [\#16272](https://github.com/netdata/netdata/pull/16272) ([underhood](https://github.com/underhood))
- fix sources match [\#16271](https://github.com/netdata/netdata/pull/16271) ([ktsaou](https://github.com/ktsaou))
- Add an obsoletion time for statsd private charts [\#16269](https://github.com/netdata/netdata/pull/16269) ([MrZammler](https://github.com/MrZammler))
- ZSTD and GZIP/DEFLATE streaming support [\#16268](https://github.com/netdata/netdata/pull/16268) ([ktsaou](https://github.com/ktsaou))
- journal minor updates [\#16267](https://github.com/netdata/netdata/pull/16267) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16266](https://github.com/netdata/netdata/pull/16266) ([netdatabot](https://github.com/netdatabot))
- Fix coverity issue 403725 [\#16265](https://github.com/netdata/netdata/pull/16265) ([stelfrag](https://github.com/stelfrag))
- SUBSTRING simple patterns fix [\#16264](https://github.com/netdata/netdata/pull/16264) ([ktsaou](https://github.com/ktsaou))
- QUERIES: use tiers only when they have useful data [\#16263](https://github.com/netdata/netdata/pull/16263) ([ktsaou](https://github.com/ktsaou))
- Improve dimension ML model load [\#16262](https://github.com/netdata/netdata/pull/16262) ([stelfrag](https://github.com/stelfrag))
- cgroup: add net container\_device label [\#16261](https://github.com/netdata/netdata/pull/16261) ([ilyam8](https://github.com/ilyam8))
- Replace distutils with packaging for version [\#16259](https://github.com/netdata/netdata/pull/16259) ([MrZammler](https://github.com/MrZammler))
- Regenerate integrations.js [\#16258](https://github.com/netdata/netdata/pull/16258) ([netdatabot](https://github.com/netdatabot))
- Fix Discord webhook payload [\#16257](https://github.com/netdata/netdata/pull/16257) ([luchaos](https://github.com/luchaos))
- Fix HAProxy server status parsing and add MAINT status chart [\#16253](https://github.com/netdata/netdata/pull/16253) ([seniorquico](https://github.com/seniorquico))
- Journal multiple sources [\#16252](https://github.com/netdata/netdata/pull/16252) ([ktsaou](https://github.com/ktsaou))
- `most_popular` on markdown metadata for integrations [\#16251](https://github.com/netdata/netdata/pull/16251) ([Ancairon](https://github.com/Ancairon))
- Dyncfg improvements [\#16250](https://github.com/netdata/netdata/pull/16250) ([ktsaou](https://github.com/ktsaou))
- Fix label copy to correctly handle duplicate keys [\#16249](https://github.com/netdata/netdata/pull/16249) ([stelfrag](https://github.com/stelfrag))
- added systemd-journal forward\_secure\_sealing [\#16247](https://github.com/netdata/netdata/pull/16247) ([ktsaou](https://github.com/ktsaou))
- Terminate cgroups discovery thread faster during shutdown [\#16246](https://github.com/netdata/netdata/pull/16246) ([stelfrag](https://github.com/stelfrag))
- python.d\(smartd\_log\): collect Total LBAs written/read [\#16245](https://github.com/netdata/netdata/pull/16245) ([watsonbox](https://github.com/watsonbox))
- fix apps plugin metric names in meta [\#16243](https://github.com/netdata/netdata/pull/16243) ([ilyam8](https://github.com/ilyam8))
- Drop an unused index from aclk\_alert table [\#16242](https://github.com/netdata/netdata/pull/16242) ([stelfrag](https://github.com/stelfrag))
- add DYNCFG\_RESET  [\#16241](https://github.com/netdata/netdata/pull/16241) ([underhood](https://github.com/underhood))
- Reuse ML load prepared statement [\#16240](https://github.com/netdata/netdata/pull/16240) ([stelfrag](https://github.com/stelfrag))
- update bundled UI to v6.53.0 [\#16239](https://github.com/netdata/netdata/pull/16239) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16237](https://github.com/netdata/netdata/pull/16237) ([netdatabot](https://github.com/netdatabot))
- Active journal centralization guide no encryption [\#16236](https://github.com/netdata/netdata/pull/16236) ([tkatsoulas](https://github.com/tkatsoulas))
- journal: script to generate self-signed-certificates [\#16235](https://github.com/netdata/netdata/pull/16235) ([ktsaou](https://github.com/ktsaou))
- Fix dimension HETEROGENEOUS check [\#16234](https://github.com/netdata/netdata/pull/16234) ([stelfrag](https://github.com/stelfrag))
- uninstaller: remove /etc/cron.d/netdata-updater-daily [\#16233](https://github.com/netdata/netdata/pull/16233) ([ilyam8](https://github.com/ilyam8))
- Add Erlang to Apps configuration [\#16231](https://github.com/netdata/netdata/pull/16231) ([andyundso](https://github.com/andyundso))
- remove charts.d/nut [\#16230](https://github.com/netdata/netdata/pull/16230) ([ilyam8](https://github.com/ilyam8))
- kickstart: rename auto-update-method to auto-update-type [\#16229](https://github.com/netdata/netdata/pull/16229) ([ilyam8](https://github.com/ilyam8))
- update go.d plugin version to v0.56.3 [\#16228](https://github.com/netdata/netdata/pull/16228) ([ilyam8](https://github.com/ilyam8))
- Add document outlining our versioning policy and public API. [\#16227](https://github.com/netdata/netdata/pull/16227) ([Ferroin](https://github.com/Ferroin))
- Changes to `systemd-journal` docs [\#16225](https://github.com/netdata/netdata/pull/16225) ([Ancairon](https://github.com/Ancairon))
- Fix statistics calculation in 32bit systems [\#16222](https://github.com/netdata/netdata/pull/16222) ([stelfrag](https://github.com/stelfrag))
- Fix meta unittest [\#16221](https://github.com/netdata/netdata/pull/16221) ([stelfrag](https://github.com/stelfrag))
- facets: minimize hashtable collisions [\#16215](https://github.com/netdata/netdata/pull/16215) ([ktsaou](https://github.com/ktsaou))
- Removing support for Alpine 3.15 [\#16205](https://github.com/netdata/netdata/pull/16205) ([tkatsoulas](https://github.com/tkatsoulas))
- Improve context load on startup [\#16203](https://github.com/netdata/netdata/pull/16203) ([stelfrag](https://github.com/stelfrag))
- cgroup-network: don't log an error opening pid file if doesn't exist [\#16196](https://github.com/netdata/netdata/pull/16196) ([ilyam8](https://github.com/ilyam8))
- docker install: support for Proxmox vms/containers name resolution [\#16193](https://github.com/netdata/netdata/pull/16193) ([ilyam8](https://github.com/ilyam8))
- Introduce workflow to always update bundled packages \(static builds\) into their latest release \(part1\) [\#16191](https://github.com/netdata/netdata/pull/16191) ([tkatsoulas](https://github.com/tkatsoulas))
- Improvements for labels handling [\#16172](https://github.com/netdata/netdata/pull/16172) ([stelfrag](https://github.com/stelfrag))
- Faster parents [\#16127](https://github.com/netdata/netdata/pull/16127) ([ktsaou](https://github.com/ktsaou))
- Update info about custom dashboards [\#16121](https://github.com/netdata/netdata/pull/16121) ([elizabyte8](https://github.com/elizabyte8))
- Add info to native packages docs about mirroring our repos. [\#16069](https://github.com/netdata/netdata/pull/16069) ([Ferroin](https://github.com/Ferroin))
- shutdown while waiting for collectors to finish [\#16023](https://github.com/netdata/netdata/pull/16023) ([ktsaou](https://github.com/ktsaou))
- Add integrations JSON file for website usage. [\#15959](https://github.com/netdata/netdata/pull/15959) ([Ferroin](https://github.com/Ferroin))
- Docs: getting started with netdata cloud onprem [\#15954](https://github.com/netdata/netdata/pull/15954) ([M4itee](https://github.com/M4itee))

## [v1.43.2](https://github.com/netdata/netdata/tree/v1.43.2) (2023-10-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.1...v1.43.2)

## [v1.43.1](https://github.com/netdata/netdata/tree/v1.43.1) (2023-10-26)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.0...v1.43.1)

## [v1.43.0](https://github.com/netdata/netdata/tree/v1.43.0) (2023-10-16)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.4...v1.43.0)

**Merged pull requests:**

- update bundled UI to v6.52.2 [\#16219](https://github.com/netdata/netdata/pull/16219) ([ilyam8](https://github.com/ilyam8))
- dynamic meta queue size [\#16218](https://github.com/netdata/netdata/pull/16218) ([ktsaou](https://github.com/ktsaou))
- update bundled UI to v6.52.1 [\#16217](https://github.com/netdata/netdata/pull/16217) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.52.0 [\#16216](https://github.com/netdata/netdata/pull/16216) ([ilyam8](https://github.com/ilyam8))
- disable logging to syslog by default [\#16214](https://github.com/netdata/netdata/pull/16214) ([ilyam8](https://github.com/ilyam8))
- add summary to /alerts [\#16213](https://github.com/netdata/netdata/pull/16213) ([MrZammler](https://github.com/MrZammler))
- registry action hello should always work [\#16212](https://github.com/netdata/netdata/pull/16212) ([ktsaou](https://github.com/ktsaou))
- apps: fix divide by zero when calc avg uptime [\#16211](https://github.com/netdata/netdata/pull/16211) ([ilyam8](https://github.com/ilyam8))
- allow patterns in journal queries [\#16210](https://github.com/netdata/netdata/pull/16210) ([ktsaou](https://github.com/ktsaou))
- ui-6.51.0 [\#16208](https://github.com/netdata/netdata/pull/16208) ([ktsaou](https://github.com/ktsaou))
- add order in available histograms [\#16204](https://github.com/netdata/netdata/pull/16204) ([ktsaou](https://github.com/ktsaou))
- update ui to 6.50.2 again [\#16202](https://github.com/netdata/netdata/pull/16202) ([ktsaou](https://github.com/ktsaou))
- update ui to 6.50.2 [\#16201](https://github.com/netdata/netdata/pull/16201) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16200](https://github.com/netdata/netdata/pull/16200) ([netdatabot](https://github.com/netdatabot))
- health: attach drops ratio alarms to net.drops [\#16199](https://github.com/netdata/netdata/pull/16199) ([ilyam8](https://github.com/ilyam8))
- apps: always expose "other" group [\#16198](https://github.com/netdata/netdata/pull/16198) ([ilyam8](https://github.com/ilyam8))
- journal timeout [\#16195](https://github.com/netdata/netdata/pull/16195) ([ktsaou](https://github.com/ktsaou))
- systemd-journal timeout to 55 secs [\#16194](https://github.com/netdata/netdata/pull/16194) ([ktsaou](https://github.com/ktsaou))
- update bundled UI to v6.49.0 [\#16192](https://github.com/netdata/netdata/pull/16192) ([ilyam8](https://github.com/ilyam8))
- Faster facets [\#16190](https://github.com/netdata/netdata/pull/16190) ([ktsaou](https://github.com/ktsaou))
- Journal updates [\#16189](https://github.com/netdata/netdata/pull/16189) ([ktsaou](https://github.com/ktsaou))
- Add agent version on startup [\#16188](https://github.com/netdata/netdata/pull/16188) ([stelfrag](https://github.com/stelfrag))
- Suppress "families" log [\#16186](https://github.com/netdata/netdata/pull/16186) ([stelfrag](https://github.com/stelfrag))
- Fix access of memory after free [\#16185](https://github.com/netdata/netdata/pull/16185) ([stelfrag](https://github.com/stelfrag))
- functions columns [\#16184](https://github.com/netdata/netdata/pull/16184) ([ktsaou](https://github.com/ktsaou))
- disable \_go\_build in centos 8 & 9  [\#16183](https://github.com/netdata/netdata/pull/16183) ([tkatsoulas](https://github.com/tkatsoulas))
- Regenerate integrations.js [\#16182](https://github.com/netdata/netdata/pull/16182) ([netdatabot](https://github.com/netdatabot))
- update go.d to v0.56.2 [\#16181](https://github.com/netdata/netdata/pull/16181) ([ilyam8](https://github.com/ilyam8))
- Add support for Fedora 39 native packages into our CI [\#16180](https://github.com/netdata/netdata/pull/16180) ([tkatsoulas](https://github.com/tkatsoulas))
- Add support for Ubuntu 23.10 native packages into our CI [\#16179](https://github.com/netdata/netdata/pull/16179) ([tkatsoulas](https://github.com/tkatsoulas))
- Update bundled static packages  [\#16177](https://github.com/netdata/netdata/pull/16177) ([tkatsoulas](https://github.com/tkatsoulas))
- Regenerate integrations.js [\#16176](https://github.com/netdata/netdata/pull/16176) ([netdatabot](https://github.com/netdatabot))
- facets: do not corrupt the index when doubling the hashtable [\#16171](https://github.com/netdata/netdata/pull/16171) ([ktsaou](https://github.com/ktsaou))
- Add icons to integrations markdown files [\#16169](https://github.com/netdata/netdata/pull/16169) ([Ancairon](https://github.com/Ancairon))
- Fix netdata-uninstaller; blindly deletes NETDATA\_PREFIX env var [\#16167](https://github.com/netdata/netdata/pull/16167) ([tkatsoulas](https://github.com/tkatsoulas))
- apps: remove mem\_private on FreeBSD [\#16166](https://github.com/netdata/netdata/pull/16166) ([ilyam8](https://github.com/ilyam8))
- fix repo path for openSUSE 15.5 packages [\#16161](https://github.com/netdata/netdata/pull/16161) ([tkatsoulas](https://github.com/tkatsoulas))
- Modify eBPF exit [\#16159](https://github.com/netdata/netdata/pull/16159) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix compilation warnings [\#16158](https://github.com/netdata/netdata/pull/16158) ([stelfrag](https://github.com/stelfrag))
- Don't queue removed when there is a newer alert [\#16157](https://github.com/netdata/netdata/pull/16157) ([MrZammler](https://github.com/MrZammler))
- docker: make chmod o+rX /  non fatal [\#16156](https://github.com/netdata/netdata/pull/16156) ([ilyam8](https://github.com/ilyam8))
- Batch ML model load commands [\#16155](https://github.com/netdata/netdata/pull/16155) ([stelfrag](https://github.com/stelfrag))
- \[BUGFIX\] MQTT ARM fix [\#16154](https://github.com/netdata/netdata/pull/16154) ([underhood](https://github.com/underhood))
- Rework guide, add SSL with self-signed certs [\#16153](https://github.com/netdata/netdata/pull/16153) ([tkatsoulas](https://github.com/tkatsoulas))
- make io charts "write" negative in apps and cgroups \(systemd\) [\#16152](https://github.com/netdata/netdata/pull/16152) ([ilyam8](https://github.com/ilyam8))
- journal: updates [\#16150](https://github.com/netdata/netdata/pull/16150) ([ktsaou](https://github.com/ktsaou))
- uninstaller: remove ND systemd preset and tmp dir [\#16148](https://github.com/netdata/netdata/pull/16148) ([ilyam8](https://github.com/ilyam8))
- fix `test -x` check for uninstaller script [\#16146](https://github.com/netdata/netdata/pull/16146) ([ilyam8](https://github.com/ilyam8))
- health: don't log an unknown key error for "families" [\#16145](https://github.com/netdata/netdata/pull/16145) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16144](https://github.com/netdata/netdata/pull/16144) ([netdatabot](https://github.com/netdatabot))
- Update python.d./varnish/metadata.yaml [\#16143](https://github.com/netdata/netdata/pull/16143) ([Ancairon](https://github.com/Ancairon))
- Bugfix in integrations/setup/template [\#16142](https://github.com/netdata/netdata/pull/16142) ([Ancairon](https://github.com/Ancairon))
- Fixes in integration generation script [\#16141](https://github.com/netdata/netdata/pull/16141) ([Ancairon](https://github.com/Ancairon))
- Introduce stringify function for integrations [\#16140](https://github.com/netdata/netdata/pull/16140) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#16138](https://github.com/netdata/netdata/pull/16138) ([netdatabot](https://github.com/netdatabot))
- fix random crashes on pthread\_detach\(\) [\#16137](https://github.com/netdata/netdata/pull/16137) ([ktsaou](https://github.com/ktsaou))
- fix journal help and mark debug keys in the output [\#16133](https://github.com/netdata/netdata/pull/16133) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16132](https://github.com/netdata/netdata/pull/16132) ([netdatabot](https://github.com/netdatabot))
- apps: change user\_group to usergroup [\#16131](https://github.com/netdata/netdata/pull/16131) ([ilyam8](https://github.com/ilyam8))
- Retain a list structure instead of a set for data collection integrations categories [\#16130](https://github.com/netdata/netdata/pull/16130) ([Ancairon](https://github.com/Ancairon))
- Add summary to alerts configurations [\#16129](https://github.com/netdata/netdata/pull/16129) ([MrZammler](https://github.com/MrZammler))
- Remove multiple categories due to bug [\#16126](https://github.com/netdata/netdata/pull/16126) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#16125](https://github.com/netdata/netdata/pull/16125) ([netdatabot](https://github.com/netdatabot))
- update UI to v6.45.0 [\#16124](https://github.com/netdata/netdata/pull/16124) ([ilyam8](https://github.com/ilyam8))
- journal: fix the 1 second latency in play mode [\#16123](https://github.com/netdata/netdata/pull/16123) ([ktsaou](https://github.com/ktsaou))
- fix proc netstat metrics [\#16122](https://github.com/netdata/netdata/pull/16122) ([ilyam8](https://github.com/ilyam8))
- dont strip newlines when forwarding FUNCTION\_PAYLOAD [\#16120](https://github.com/netdata/netdata/pull/16120) ([underhood](https://github.com/underhood))
- Do not force OOMKill [\#16115](https://github.com/netdata/netdata/pull/16115) ([thiagoftsm](https://github.com/thiagoftsm))
- fix crash on parsing clabel command with no source [\#16114](https://github.com/netdata/netdata/pull/16114) ([ilyam8](https://github.com/ilyam8))
- update UI to v6.43.0 [\#16112](https://github.com/netdata/netdata/pull/16112) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16111](https://github.com/netdata/netdata/pull/16111) ([netdatabot](https://github.com/netdatabot))
- journal: respect anchor on non-data-only queries [\#16109](https://github.com/netdata/netdata/pull/16109) ([ktsaou](https://github.com/ktsaou))
- Fix in generate integrations docs script [\#16108](https://github.com/netdata/netdata/pull/16108) ([Ancairon](https://github.com/Ancairon))
- journal: go up to stop anchor on data only queries [\#16107](https://github.com/netdata/netdata/pull/16107) ([ktsaou](https://github.com/ktsaou))
- Update collectors/python.d.plugin/pandas/metadata.yaml [\#16106](https://github.com/netdata/netdata/pull/16106) ([Ancairon](https://github.com/Ancairon))
- Code improvements [\#16104](https://github.com/netdata/netdata/pull/16104) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16103](https://github.com/netdata/netdata/pull/16103) ([netdatabot](https://github.com/netdatabot))
- Add integrations/cloud-notifications to cleanup [\#16102](https://github.com/netdata/netdata/pull/16102) ([Ancairon](https://github.com/Ancairon))
- better journal logging [\#16101](https://github.com/netdata/netdata/pull/16101) ([ktsaou](https://github.com/ktsaou))
- update UI to v6.42.2 [\#16100](https://github.com/netdata/netdata/pull/16100) ([ilyam8](https://github.com/ilyam8))
- a simple journal optimization [\#16099](https://github.com/netdata/netdata/pull/16099) ([ktsaou](https://github.com/ktsaou))
- journal: fix incremental queries [\#16098](https://github.com/netdata/netdata/pull/16098) ([ktsaou](https://github.com/ktsaou))
- Update categories.yaml [\#16097](https://github.com/netdata/netdata/pull/16097) ([Ancairon](https://github.com/Ancairon))
- Fix systemd-journal.plugin README and prepare it for Learn [\#16096](https://github.com/netdata/netdata/pull/16096) ([Ancairon](https://github.com/Ancairon))
- Split apps charts [\#16095](https://github.com/netdata/netdata/pull/16095) ([thiagoftsm](https://github.com/thiagoftsm))
- fix querying out of retention [\#16094](https://github.com/netdata/netdata/pull/16094) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16093](https://github.com/netdata/netdata/pull/16093) ([netdatabot](https://github.com/netdatabot))
- update go.d.plugin to v0.56.1 [\#16092](https://github.com/netdata/netdata/pull/16092) ([ilyam8](https://github.com/ilyam8))
- update UI to v6.42.1 [\#16091](https://github.com/netdata/netdata/pull/16091) ([ilyam8](https://github.com/ilyam8))
- dont use sd\_journal\_open\_files\_fd\(\) that is buggy on older libsystemd [\#16090](https://github.com/netdata/netdata/pull/16090) ([ktsaou](https://github.com/ktsaou))
- external plugins: respect env NETDATA\_LOG\_SEVERITY\_LEVEL [\#16089](https://github.com/netdata/netdata/pull/16089) ([ilyam8](https://github.com/ilyam8))
- update UI to v6.42.0 [\#16088](https://github.com/netdata/netdata/pull/16088) ([ilyam8](https://github.com/ilyam8))
- functions: prevent a busy wait loop [\#16086](https://github.com/netdata/netdata/pull/16086) ([ktsaou](https://github.com/ktsaou))
- charts.d: respect env NETDATA\_LOG\_SEVERITY\_LEVEL [\#16085](https://github.com/netdata/netdata/pull/16085) ([ilyam8](https://github.com/ilyam8))
- python.d: respect env NETDATA\_LOG\_SEVERITY\_LEVEL [\#16084](https://github.com/netdata/netdata/pull/16084) ([ilyam8](https://github.com/ilyam8))
- Address reported socket issue [\#16083](https://github.com/netdata/netdata/pull/16083) ([thiagoftsm](https://github.com/thiagoftsm))
- Change @linuxnetdata to @netdatahq [\#16082](https://github.com/netdata/netdata/pull/16082) ([ralphm](https://github.com/ralphm))
- \[Integrations Docs\] Add a badge for either netdata or community maintained [\#16073](https://github.com/netdata/netdata/pull/16073) ([Ancairon](https://github.com/Ancairon))
- Skip database migration steps in new installation [\#16071](https://github.com/netdata/netdata/pull/16071) ([stelfrag](https://github.com/stelfrag))
- Improve description about tc.plugin [\#16068](https://github.com/netdata/netdata/pull/16068) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#16062](https://github.com/netdata/netdata/pull/16062) ([netdatabot](https://github.com/netdatabot))
- update go.d version to v0.56.0 [\#16061](https://github.com/netdata/netdata/pull/16061) ([ilyam8](https://github.com/ilyam8))
- Bugfix on integrations/gen\_docs\_integrations.py [\#16059](https://github.com/netdata/netdata/pull/16059) ([Ancairon](https://github.com/Ancairon))
- Fix coverity 402975 [\#16058](https://github.com/netdata/netdata/pull/16058) ([stelfrag](https://github.com/stelfrag))
- Send alerts summary field to cloud [\#16056](https://github.com/netdata/netdata/pull/16056) ([MrZammler](https://github.com/MrZammler))
- update bundled ui version to v6.41.1 [\#16054](https://github.com/netdata/netdata/pull/16054) ([ilyam8](https://github.com/ilyam8))
- Update to use versioned base images for CI. [\#16053](https://github.com/netdata/netdata/pull/16053) ([Ferroin](https://github.com/Ferroin))
- Fix missing find command when installing/updating on Rocky Linux systems. [\#16052](https://github.com/netdata/netdata/pull/16052) ([Ferroin](https://github.com/Ferroin))
- Fix summary field in table [\#16050](https://github.com/netdata/netdata/pull/16050) ([MrZammler](https://github.com/MrZammler))
- Switch to uint64\_t to avoid overflow in 32bit systems [\#16048](https://github.com/netdata/netdata/pull/16048) ([stelfrag](https://github.com/stelfrag))
- Convert the ML database  [\#16046](https://github.com/netdata/netdata/pull/16046) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16044](https://github.com/netdata/netdata/pull/16044) ([netdatabot](https://github.com/netdatabot))
- Doc about running a local dashboard through Cloudflare \(community\) [\#16043](https://github.com/netdata/netdata/pull/16043) ([Ancairon](https://github.com/Ancairon))
- Have one documentation page about Netdata Charts [\#16042](https://github.com/netdata/netdata/pull/16042) ([Ancairon](https://github.com/Ancairon))
- Remove discontinued Hangouts and StackPulse notification methods [\#16041](https://github.com/netdata/netdata/pull/16041) ([Ancairon](https://github.com/Ancairon))
- systemd-Journal by file [\#16038](https://github.com/netdata/netdata/pull/16038) ([ktsaou](https://github.com/ktsaou))
- health: add upsd alerts [\#16036](https://github.com/netdata/netdata/pull/16036) ([ilyam8](https://github.com/ilyam8))
- Disable mongodb exporter builds where broken. [\#16033](https://github.com/netdata/netdata/pull/16033) ([Ferroin](https://github.com/Ferroin))
- Run health queries from tier 0 [\#16032](https://github.com/netdata/netdata/pull/16032) ([MrZammler](https://github.com/MrZammler))
- use `status` as units for `anomaly_detection.detector_events` [\#16028](https://github.com/netdata/netdata/pull/16028) ([andrewm4894](https://github.com/andrewm4894))
- add description for Homebrew on Apple Silicon Mac\(netdata/learn/\#1789\) [\#16027](https://github.com/netdata/netdata/pull/16027) ([theggs](https://github.com/theggs))
- Fix package builds on Rocky Linux. [\#16026](https://github.com/netdata/netdata/pull/16026) ([Ferroin](https://github.com/Ferroin))
- Remove family from alerts [\#16025](https://github.com/netdata/netdata/pull/16025) ([MrZammler](https://github.com/MrZammler))
- add systemd-journal.plugin to apps\_groups.conf [\#16024](https://github.com/netdata/netdata/pull/16024) ([ilyam8](https://github.com/ilyam8))
- Fix handling of CI skipping. [\#16022](https://github.com/netdata/netdata/pull/16022) ([Ferroin](https://github.com/Ferroin))
- update bundled UI to v6.39.0 [\#16020](https://github.com/netdata/netdata/pull/16020) ([ilyam8](https://github.com/ilyam8))
- Update collector metadata for python collectors [\#16019](https://github.com/netdata/netdata/pull/16019) ([tkatsoulas](https://github.com/tkatsoulas))
- fix crash on setting thread name [\#16016](https://github.com/netdata/netdata/pull/16016) ([ilyam8](https://github.com/ilyam8))
- Systemd-Journal: fix crash when the uid or gid do not have names [\#16015](https://github.com/netdata/netdata/pull/16015) ([ktsaou](https://github.com/ktsaou))
- Avoid duplicate keys in labels [\#16014](https://github.com/netdata/netdata/pull/16014) ([stelfrag](https://github.com/stelfrag))
- remove the line length limit from pluginsd [\#16013](https://github.com/netdata/netdata/pull/16013) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16011](https://github.com/netdata/netdata/pull/16011) ([netdatabot](https://github.com/netdatabot))
- Simplify the script for generating documentation from integrations [\#16009](https://github.com/netdata/netdata/pull/16009) ([Ancairon](https://github.com/Ancairon))
- some collector metadata improvements [\#16008](https://github.com/netdata/netdata/pull/16008) ([andrewm4894](https://github.com/andrewm4894))
- Fix compilation warnings [\#16006](https://github.com/netdata/netdata/pull/16006) ([stelfrag](https://github.com/stelfrag))
- Update CMakeLists.txt [\#16005](https://github.com/netdata/netdata/pull/16005) ([stelfrag](https://github.com/stelfrag))
- eBPF socket: function with event loop [\#16004](https://github.com/netdata/netdata/pull/16004) ([thiagoftsm](https://github.com/thiagoftsm))
- fix compilation warnings [\#16001](https://github.com/netdata/netdata/pull/16001) ([ktsaou](https://github.com/ktsaou))
- Update integrations/gen\_docs\_integrations.py [\#15997](https://github.com/netdata/netdata/pull/15997) ([Ancairon](https://github.com/Ancairon))
- Make collectors/COLLECTORS.md have its list autogenerated from integrations.js [\#15995](https://github.com/netdata/netdata/pull/15995) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#15985](https://github.com/netdata/netdata/pull/15985) ([netdatabot](https://github.com/netdatabot))
- Re-store rrdvars on late dimensions [\#15984](https://github.com/netdata/netdata/pull/15984) ([MrZammler](https://github.com/MrZammler))
- Functions: allow collectors to be restarted [\#15983](https://github.com/netdata/netdata/pull/15983) ([ktsaou](https://github.com/ktsaou))
- Metadata fixes for some collectors [\#15982](https://github.com/netdata/netdata/pull/15982) ([Ancairon](https://github.com/Ancairon))
- update go.d.plugin to v0.55.0 [\#15981](https://github.com/netdata/netdata/pull/15981) ([ilyam8](https://github.com/ilyam8))
- bump UI to v6.37.1 [\#15980](https://github.com/netdata/netdata/pull/15980) ([ilyam8](https://github.com/ilyam8))
- Maintain node's last connected timestamp in the db [\#15979](https://github.com/netdata/netdata/pull/15979) ([stelfrag](https://github.com/stelfrag))
- apps.plugin function is not thread safe [\#15978](https://github.com/netdata/netdata/pull/15978) ([ktsaou](https://github.com/ktsaou))
- functions cancelling [\#15977](https://github.com/netdata/netdata/pull/15977) ([ktsaou](https://github.com/ktsaou))
- Facets: fixes 5 [\#15976](https://github.com/netdata/netdata/pull/15976) ([ktsaou](https://github.com/ktsaou))
- Split cgroup charts [\#15975](https://github.com/netdata/netdata/pull/15975) ([thiagoftsm](https://github.com/thiagoftsm))
- facets histogram: do not send db retention for facets [\#15974](https://github.com/netdata/netdata/pull/15974) ([ktsaou](https://github.com/ktsaou))
- extend ml default training from ~24 to ~48 hours [\#15971](https://github.com/netdata/netdata/pull/15971) ([andrewm4894](https://github.com/andrewm4894))
- facets histogram when empty [\#15970](https://github.com/netdata/netdata/pull/15970) ([ktsaou](https://github.com/ktsaou))
- facets: do not shadow local variable [\#15968](https://github.com/netdata/netdata/pull/15968) ([ktsaou](https://github.com/ktsaou))
- Skip trying to preserve file owners when bundling external code. [\#15966](https://github.com/netdata/netdata/pull/15966) ([Ferroin](https://github.com/Ferroin))
- fix using undefined var when loading job statuses in python.d [\#15965](https://github.com/netdata/netdata/pull/15965) ([ilyam8](https://github.com/ilyam8))
- facets: data-only queries [\#15961](https://github.com/netdata/netdata/pull/15961) ([ktsaou](https://github.com/ktsaou))
- Clarify shipping repos cases [\#15960](https://github.com/netdata/netdata/pull/15960) ([tkatsoulas](https://github.com/tkatsoulas))
- Clarifying the possible installation types [\#15958](https://github.com/netdata/netdata/pull/15958) ([tkatsoulas](https://github.com/tkatsoulas))
- fix journal direction parsing [\#15957](https://github.com/netdata/netdata/pull/15957) ([ktsaou](https://github.com/ktsaou))
- facets and journal improvements [\#15956](https://github.com/netdata/netdata/pull/15956) ([ktsaou](https://github.com/ktsaou))
- Preserve README info inside metadata.yaml for freeipmi.plugin [\#15955](https://github.com/netdata/netdata/pull/15955) ([Ancairon](https://github.com/Ancairon))
- Fix CID 400366 [\#15953](https://github.com/netdata/netdata/pull/15953) ([stelfrag](https://github.com/stelfrag))
- Update descriptions. [\#15952](https://github.com/netdata/netdata/pull/15952) ([thiagoftsm](https://github.com/thiagoftsm))
- Update slabinfo metadata [\#15951](https://github.com/netdata/netdata/pull/15951) ([thiagoftsm](https://github.com/thiagoftsm))
- Disk Labels [\#15949](https://github.com/netdata/netdata/pull/15949) ([ktsaou](https://github.com/ktsaou))
- streaming logs [\#15948](https://github.com/netdata/netdata/pull/15948) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15946](https://github.com/netdata/netdata/pull/15946) ([netdatabot](https://github.com/netdatabot))
- Integrations: Add a note to enable the collectors [\#15945](https://github.com/netdata/netdata/pull/15945) ([MrZammler](https://github.com/MrZammler))
- Change the build image of EL packages from alma to rocky [\#15944](https://github.com/netdata/netdata/pull/15944) ([tkatsoulas](https://github.com/tkatsoulas))
- Integrations: Add a note to install charts.d plugin [\#15943](https://github.com/netdata/netdata/pull/15943) ([MrZammler](https://github.com/MrZammler))
- Add description about packages [\#15941](https://github.com/netdata/netdata/pull/15941) ([thiagoftsm](https://github.com/thiagoftsm))
- facets optimizations [\#15940](https://github.com/netdata/netdata/pull/15940) ([ktsaou](https://github.com/ktsaou))
- improved facets info [\#15936](https://github.com/netdata/netdata/pull/15936) ([ktsaou](https://github.com/ktsaou))
- feat: Adds access control configuration for ntfy [\#15932](https://github.com/netdata/netdata/pull/15932) ([miversen33](https://github.com/miversen33))
- fix memory leak on prometheus exporter and code cleanup [\#15929](https://github.com/netdata/netdata/pull/15929) ([ktsaou](https://github.com/ktsaou))
- systemd-journal and facets: info and sources [\#15928](https://github.com/netdata/netdata/pull/15928) ([ktsaou](https://github.com/ktsaou))
- systemd-journal and facets improvements [\#15926](https://github.com/netdata/netdata/pull/15926) ([ktsaou](https://github.com/ktsaou))
- add specific info on how to access the dashboards [\#15925](https://github.com/netdata/netdata/pull/15925) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Reduce workload during cleanup [\#15919](https://github.com/netdata/netdata/pull/15919) ([stelfrag](https://github.com/stelfrag))
- Replace \_ with spaces for name variable for ntfy [\#15909](https://github.com/netdata/netdata/pull/15909) ([MAH69IK](https://github.com/MAH69IK))
- python.d/sensors: Increase voltage limits 127 -\> 400 [\#15905](https://github.com/netdata/netdata/pull/15905) ([kylemanna](https://github.com/kylemanna))
- Assorted Dockerfile cleanup. [\#15902](https://github.com/netdata/netdata/pull/15902) ([Ferroin](https://github.com/Ferroin))
- Improve shutdown of the metadata thread [\#15901](https://github.com/netdata/netdata/pull/15901) ([stelfrag](https://github.com/stelfrag))
- bump ui to v6.32.0 [\#15897](https://github.com/netdata/netdata/pull/15897) ([andrewm4894](https://github.com/andrewm4894))
- Update change-metrics-storage.md [\#15896](https://github.com/netdata/netdata/pull/15896) ([Ancairon](https://github.com/Ancairon))
- make `anomaly_detection.type_anomaly_rate` stacked [\#15895](https://github.com/netdata/netdata/pull/15895) ([andrewm4894](https://github.com/andrewm4894))
- Update pfsense.md [\#15894](https://github.com/netdata/netdata/pull/15894) ([Ancairon](https://github.com/Ancairon))
- Initial tooling for Integrations Documentation [\#15893](https://github.com/netdata/netdata/pull/15893) ([Ancairon](https://github.com/Ancairon))
- Reset the obsolete flag on service thread [\#15892](https://github.com/netdata/netdata/pull/15892) ([MrZammler](https://github.com/MrZammler))
- Add better recovery for corrupted metadata [\#15891](https://github.com/netdata/netdata/pull/15891) ([stelfrag](https://github.com/stelfrag))
- Add index to ACLK table to improve update statements [\#15890](https://github.com/netdata/netdata/pull/15890) ([stelfrag](https://github.com/stelfrag))
- Limit atomic operations for statistics [\#15887](https://github.com/netdata/netdata/pull/15887) ([ktsaou](https://github.com/ktsaou))
- Add a summary field to alerts [\#15886](https://github.com/netdata/netdata/pull/15886) ([MrZammler](https://github.com/MrZammler))
- Properly document issues with installing on hosts without IPv4. [\#15882](https://github.com/netdata/netdata/pull/15882) ([Ferroin](https://github.com/Ferroin))
- allow any field to be a facet [\#15880](https://github.com/netdata/netdata/pull/15880) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15879](https://github.com/netdata/netdata/pull/15879) ([netdatabot](https://github.com/netdatabot))
- use the newer XXH3 128bits algorithm, instead of the classic XXH128 [\#15878](https://github.com/netdata/netdata/pull/15878) ([ktsaou](https://github.com/ktsaou))
- Skip copying environment/install-type files when checking existing installs. [\#15876](https://github.com/netdata/netdata/pull/15876) ([Ferroin](https://github.com/Ferroin))
- ML add new `delete old models param` to readme [\#15873](https://github.com/netdata/netdata/pull/15873) ([andrewm4894](https://github.com/andrewm4894))
- Update SQLITE version to 3.42.0 [\#15870](https://github.com/netdata/netdata/pull/15870) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#15867](https://github.com/netdata/netdata/pull/15867) ([netdatabot](https://github.com/netdatabot))
- Add a fail reason to pinpoint exactly what went wrong [\#15866](https://github.com/netdata/netdata/pull/15866) ([stelfrag](https://github.com/stelfrag))
- Add plugin and module information to collector integrations. [\#15864](https://github.com/netdata/netdata/pull/15864) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#15862](https://github.com/netdata/netdata/pull/15862) ([netdatabot](https://github.com/netdatabot))
- Explicitly depend on version-matched plugins in native packages. [\#15861](https://github.com/netdata/netdata/pull/15861) ([Ferroin](https://github.com/Ferroin))
- Apply a label prefix for netdata labels [\#15860](https://github.com/netdata/netdata/pull/15860) ([kevin-fwu](https://github.com/kevin-fwu))
- fix proc meminfo cached calculation [\#15859](https://github.com/netdata/netdata/pull/15859) ([ilyam8](https://github.com/ilyam8))
- Fix compilation warnings [\#15858](https://github.com/netdata/netdata/pull/15858) ([stelfrag](https://github.com/stelfrag))
- packaging cleanup after \#15842 [\#15857](https://github.com/netdata/netdata/pull/15857) ([ilyam8](https://github.com/ilyam8))
- Add a chart that groups anomaly rate by chart type. [\#15856](https://github.com/netdata/netdata/pull/15856) ([vkalintiris](https://github.com/vkalintiris))
- fix packaging static build openssl 32bit [\#15855](https://github.com/netdata/netdata/pull/15855) ([ilyam8](https://github.com/ilyam8))
- fix packaging mark stable static build [\#15854](https://github.com/netdata/netdata/pull/15854) ([ilyam8](https://github.com/ilyam8))
- eBPF socket function [\#15850](https://github.com/netdata/netdata/pull/15850) ([thiagoftsm](https://github.com/thiagoftsm))
- Facets histograms [\#15846](https://github.com/netdata/netdata/pull/15846) ([ktsaou](https://github.com/ktsaou))
- reworked pluginsd caching of RDAs to avoid crashes [\#15845](https://github.com/netdata/netdata/pull/15845) ([ktsaou](https://github.com/ktsaou))
- Fix static build SSL [\#15842](https://github.com/netdata/netdata/pull/15842) ([ktsaou](https://github.com/ktsaou))
- bump bundled ui to v6.29.0 [\#15841](https://github.com/netdata/netdata/pull/15841) ([ilyam8](https://github.com/ilyam8))
- Fix configure: WARNING: unrecognized options: --with-zlib [\#15840](https://github.com/netdata/netdata/pull/15840) ([stelfrag](https://github.com/stelfrag))
- Fix compilation warning [\#15839](https://github.com/netdata/netdata/pull/15839) ([stelfrag](https://github.com/stelfrag))
- Fix warning when compiling with -flto [\#15838](https://github.com/netdata/netdata/pull/15838) ([stelfrag](https://github.com/stelfrag))
- workaround for systems that do not have SD\_JOURNAL\_OS\_ROOT [\#15837](https://github.com/netdata/netdata/pull/15837) ([ktsaou](https://github.com/ktsaou))
- added ilove.html [\#15836](https://github.com/netdata/netdata/pull/15836) ([ktsaou](https://github.com/ktsaou))
- Fix CID 382964:  Code maintainability issues  \(SIZEOF\_MISMATCH\) [\#15833](https://github.com/netdata/netdata/pull/15833) ([stelfrag](https://github.com/stelfrag))
- Fix coverity 393052:  API usage errors  \(LOCK\) [\#15832](https://github.com/netdata/netdata/pull/15832) ([stelfrag](https://github.com/stelfrag))
- systemd-journal in containers [\#15830](https://github.com/netdata/netdata/pull/15830) ([ktsaou](https://github.com/ktsaou))
- RPM: fixed attrs for conf.d dirs [\#15828](https://github.com/netdata/netdata/pull/15828) ([k0ste](https://github.com/k0ste))
- Avoid resource leak  [\#15827](https://github.com/netdata/netdata/pull/15827) ([stelfrag](https://github.com/stelfrag))
- Release fd if setsockopt or bind fails [\#15826](https://github.com/netdata/netdata/pull/15826) ([stelfrag](https://github.com/stelfrag))
- Fix use after free [\#15825](https://github.com/netdata/netdata/pull/15825) ([stelfrag](https://github.com/stelfrag))
- Improve dyncfg exit [\#15824](https://github.com/netdata/netdata/pull/15824) ([underhood](https://github.com/underhood))
- Release job message status to avoid memory leak [\#15822](https://github.com/netdata/netdata/pull/15822) ([stelfrag](https://github.com/stelfrag))
- ML improve init [\#15819](https://github.com/netdata/netdata/pull/15819) ([stelfrag](https://github.com/stelfrag))
- Update cmakelist [\#15817](https://github.com/netdata/netdata/pull/15817) ([stelfrag](https://github.com/stelfrag))
- added /api/v2/ilove.svg endpoint [\#15815](https://github.com/netdata/netdata/pull/15815) ([ktsaou](https://github.com/ktsaou))
- systemd-journal fixes [\#15814](https://github.com/netdata/netdata/pull/15814) ([ktsaou](https://github.com/ktsaou))
- fix packaging: link health.log to stdout [\#15813](https://github.com/netdata/netdata/pull/15813) ([ilyam8](https://github.com/ilyam8))
- docs rename alarm to alert [\#15812](https://github.com/netdata/netdata/pull/15812) ([ilyam8](https://github.com/ilyam8))
- bump ui to v6.28.0 [\#15810](https://github.com/netdata/netdata/pull/15810) ([ilyam8](https://github.com/ilyam8))
- return 412 instead of 403 when a bearer token is required [\#15808](https://github.com/netdata/netdata/pull/15808) ([ktsaou](https://github.com/ktsaou))
- installer setuid fallback for perf and slabinfo plugins [\#15807](https://github.com/netdata/netdata/pull/15807) ([ilyam8](https://github.com/ilyam8))
- fix api v1 mgmt/health [\#15806](https://github.com/netdata/netdata/pull/15806) ([underhood](https://github.com/underhood))
- Fix systemd journal build deps in DEB packages. [\#15805](https://github.com/netdata/netdata/pull/15805) ([Ferroin](https://github.com/Ferroin))
- Clean up python deps for RPM packages. [\#15804](https://github.com/netdata/netdata/pull/15804) ([Ferroin](https://github.com/Ferroin))
- Add proper SUID fallback for DEB plugin packages. [\#15803](https://github.com/netdata/netdata/pull/15803) ([Ferroin](https://github.com/Ferroin))
- nfacct.plugin increase restart time from 4 hours to 1 day [\#15801](https://github.com/netdata/netdata/pull/15801) ([ilyam8](https://github.com/ilyam8))
- Function systemd-journal: always have a nd\_journal\_process [\#15798](https://github.com/netdata/netdata/pull/15798) ([ktsaou](https://github.com/ktsaou))
- prevent reporting negative retention when the db is empty [\#15796](https://github.com/netdata/netdata/pull/15796) ([ktsaou](https://github.com/ktsaou))
- Fix typo in Readme [\#15794](https://github.com/netdata/netdata/pull/15794) ([shyamvalsan](https://github.com/shyamvalsan))
- fix hpssa handle unassigned drives [\#15793](https://github.com/netdata/netdata/pull/15793) ([ilyam8](https://github.com/ilyam8))
- Dyncfg add streaming support [\#15791](https://github.com/netdata/netdata/pull/15791) ([underhood](https://github.com/underhood))
- count functions as collections, to restart plugins [\#15787](https://github.com/netdata/netdata/pull/15787) ([ktsaou](https://github.com/ktsaou))
- Set correct path for ansible-playbook in deployment tutorial [\#15786](https://github.com/netdata/netdata/pull/15786) ([novotnyJiri](https://github.com/novotnyJiri))
- minor Dyncfg mvp0 fixes [\#15785](https://github.com/netdata/netdata/pull/15785) ([underhood](https://github.com/underhood))
- fix docker-compose example [\#15784](https://github.com/netdata/netdata/pull/15784) ([zhqu1148980644](https://github.com/zhqu1148980644))
- mark integrations milestones as completed in README.md [\#15783](https://github.com/netdata/netdata/pull/15783) ([tkatsoulas](https://github.com/tkatsoulas))
- Update an oversight on the openSUSE 15.5 packages [\#15781](https://github.com/netdata/netdata/pull/15781) ([tkatsoulas](https://github.com/tkatsoulas))
- Bump openssl version of static builds to 1.1.1v [\#15779](https://github.com/netdata/netdata/pull/15779) ([tkatsoulas](https://github.com/tkatsoulas))
- fix: the cleanup was not performed during the kickstart.sh dry run [\#15775](https://github.com/netdata/netdata/pull/15775) ([ilyam8](https://github.com/ilyam8))
- don't return `-1` if the socket was closed [\#15771](https://github.com/netdata/netdata/pull/15771) ([moonbreon](https://github.com/moonbreon))

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

**Merged pull requests:**

- ci: codacy exclude web/gui/v2/ [\#15780](https://github.com/netdata/netdata/pull/15780) ([ilyam8](https://github.com/ilyam8))
- update UI to v6.27.0 [\#15778](https://github.com/netdata/netdata/pull/15778) ([ilyam8](https://github.com/ilyam8))
- ci: fix labeler area/docs [\#15776](https://github.com/netdata/netdata/pull/15776) ([ilyam8](https://github.com/ilyam8))
- fix claiming via UI for static build [\#15774](https://github.com/netdata/netdata/pull/15774) ([ilyam8](https://github.com/ilyam8))
- extend the trimming window to avoid empty points at the end of queries [\#15773](https://github.com/netdata/netdata/pull/15773) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15772](https://github.com/netdata/netdata/pull/15772) ([netdatabot](https://github.com/netdatabot))
- Change FreeBSD / macOS system.swap\(io\) to mem.swap\(io\) [\#15769](https://github.com/netdata/netdata/pull/15769) ([Dim-P](https://github.com/Dim-P))

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
