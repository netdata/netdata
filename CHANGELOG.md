# Changelog

## [v1.44.2](https://github.com/netdata/netdata/tree/v1.44.2) (2024-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.1...v1.44.2)

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
