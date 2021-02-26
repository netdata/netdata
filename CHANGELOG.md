# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.3...HEAD)

**Merged pull requests:**

- Ebpf support new collectors [\#10680](https://github.com/netdata/netdata/pull/10680) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix broken links in active alarms doc [\#10678](https://github.com/netdata/netdata/pull/10678) ([joelhans](https://github.com/joelhans))
- Don't add duplicate \_total suffixes for the prometheus go.d module [\#10674](https://github.com/netdata/netdata/pull/10674) ([vlvkobal](https://github.com/vlvkobal))
- fix a typo in the email notifications readme [\#10668](https://github.com/netdata/netdata/pull/10668) ([ossimantylahti](https://github.com/ossimantylahti))
- Update screenshots and text for new Cloud nav [\#10664](https://github.com/netdata/netdata/pull/10664) ([joelhans](https://github.com/joelhans))
- Improve the Kubernetes deployment documentation [\#10662](https://github.com/netdata/netdata/pull/10662) ([joelhans](https://github.com/joelhans))
- installer: update go.d.plugin version to v0.28.0 [\#10660](https://github.com/netdata/netdata/pull/10660) ([ilyam8](https://github.com/ilyam8))
- Revamp statsd docs [\#10637](https://github.com/netdata/netdata/pull/10637) ([OdysLam](https://github.com/OdysLam))
- cpu stats per query thread [\#10634](https://github.com/netdata/netdata/pull/10634) ([MrZammler](https://github.com/MrZammler))
- web/gui: Fix broken external links [\#10586](https://github.com/netdata/netdata/pull/10586) ([Habetdin](https://github.com/Habetdin))
- Try to keep all pages from extents read from disk in the cache. [\#10558](https://github.com/netdata/netdata/pull/10558) ([mfundul](https://github.com/mfundul))
- Remove unreachable \#else directives in plugins. [\#10523](https://github.com/netdata/netdata/pull/10523) ([vkalintiris](https://github.com/vkalintiris))

## [v1.29.3](https://github.com/netdata/netdata/tree/v1.29.3) (2021-02-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.2...v1.29.3)

**Merged pull requests:**

- Invalidate RRDSETVAR pointers on obsoletion. [\#10667](https://github.com/netdata/netdata/pull/10667) ([mfundul](https://github.com/mfundul))
- Fixed condition controlling use of static LWS in RPM builds. [\#10661](https://github.com/netdata/netdata/pull/10661) ([Ferroin](https://github.com/Ferroin))
- fix wrong link on docs Netdata Agent Daemon [\#10659](https://github.com/netdata/netdata/pull/10659) ([OdysLam](https://github.com/OdysLam))
- Fix broken links in docs and add collectors to list [\#10651](https://github.com/netdata/netdata/pull/10651) ([joelhans](https://github.com/joelhans))
- Statsd dashboard [\#10640](https://github.com/netdata/netdata/pull/10640) ([OdysLam](https://github.com/OdysLam))

## [v1.29.2](https://github.com/netdata/netdata/tree/v1.29.2) (2021-02-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.1...v1.29.2)

**Merged pull requests:**

- Fix the context filtering on the data query endpoint [\#10652](https://github.com/netdata/netdata/pull/10652) ([stelfrag](https://github.com/stelfrag))
- fix container/host detection in system-info script [\#10647](https://github.com/netdata/netdata/pull/10647) ([ilyam8](https://github.com/ilyam8))
- Enable apps.plugin aggregation debug messages [\#10645](https://github.com/netdata/netdata/pull/10645) ([vlvkobal](https://github.com/vlvkobal))
- add small delay to the ipv4\_tcp\_resets alarms [\#10644](https://github.com/netdata/netdata/pull/10644) ([ilyam8](https://github.com/ilyam8))
- collectors/proc: fix collecting operstate for virtual network interfaces [\#10633](https://github.com/netdata/netdata/pull/10633) ([ilyam8](https://github.com/ilyam8))
- fix sendmail unrecognized option F error [\#10631](https://github.com/netdata/netdata/pull/10631) ([ilyam8](https://github.com/ilyam8))
- Fix typo in web/gui/readme.md [\#10623](https://github.com/netdata/netdata/pull/10623) ([OdysLam](https://github.com/OdysLam))
- add freeswitch to apps\_groups [\#10621](https://github.com/netdata/netdata/pull/10621) ([fayak](https://github.com/fayak))
- Add ACLK proxy setting as host label [\#10619](https://github.com/netdata/netdata/pull/10619) ([underhood](https://github.com/underhood))
- dashboard@v2.13.6 [\#10618](https://github.com/netdata/netdata/pull/10618) ([jacekkolasa](https://github.com/jacekkolasa))
- Disable stock alarms [\#10617](https://github.com/netdata/netdata/pull/10617) ([thiagoftsm](https://github.com/thiagoftsm))
- Fixes \#10597 raw binary data should never be printed [\#10603](https://github.com/netdata/netdata/pull/10603) ([rda0](https://github.com/rda0))
- collectors/proc: change ksm mem chart type to stacked [\#10598](https://github.com/netdata/netdata/pull/10598) ([ilyam8](https://github.com/ilyam8))
- ACLK reduce excessive logging [\#10596](https://github.com/netdata/netdata/pull/10596) ([underhood](https://github.com/underhood))
- add k8s\_cluster\_id host label [\#10588](https://github.com/netdata/netdata/pull/10588) ([ilyam8](https://github.com/ilyam8))
- add resetting CapabilityBoundingSet workaround to the python.d collectors \(that use `sudo`\) readmes [\#10587](https://github.com/netdata/netdata/pull/10587) ([ilyam8](https://github.com/ilyam8))
- collectors/elasticsearch: document `scheme` option [\#10572](https://github.com/netdata/netdata/pull/10572) ([vjt](https://github.com/vjt))
- Update claiming docs for Docker containers. [\#10570](https://github.com/netdata/netdata/pull/10570) ([Ferroin](https://github.com/Ferroin))
- health: make Opsgenie API URL configurable [\#10561](https://github.com/netdata/netdata/pull/10561) ([tinyhammers](https://github.com/tinyhammers))
- Allow the REMOVED alarm status via ACLK if the previous status was WARN/CRIT [\#10533](https://github.com/netdata/netdata/pull/10533) ([stelfrag](https://github.com/stelfrag))
- Change eBPF plugin internal [\#10442](https://github.com/netdata/netdata/pull/10442) ([thiagoftsm](https://github.com/thiagoftsm))

## [v1.29.1](https://github.com/netdata/netdata/tree/v1.29.1) (2021-02-09)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.0...v1.29.1)

**Merged pull requests:**

- Fix crash during shutdown of cgroups internal plugin. [\#10614](https://github.com/netdata/netdata/pull/10614) ([mfundul](https://github.com/mfundul))
- Update latest release on main README [\#10590](https://github.com/netdata/netdata/pull/10590) ([joelhans](https://github.com/joelhans))

## [v1.29.0](https://github.com/netdata/netdata/tree/v1.29.0) (2021-02-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.28.0...v1.29.0)

**Merged pull requests:**

- Fixed Netdata Cloud support in RPM packages. [\#10578](https://github.com/netdata/netdata/pull/10578) ([Ferroin](https://github.com/Ferroin))
- Fix container detection from systemd-detect-virt [\#10569](https://github.com/netdata/netdata/pull/10569) ([cakrit](https://github.com/cakrit))
- dashboard v2.13.0 [\#10565](https://github.com/netdata/netdata/pull/10565) ([jacekkolasa](https://github.com/jacekkolasa))
- bytes after last '}' trip JSON parser [\#10563](https://github.com/netdata/netdata/pull/10563) ([underhood](https://github.com/underhood))
- Fix prometheus remote write header [\#10560](https://github.com/netdata/netdata/pull/10560) ([vlvkobal](https://github.com/vlvkobal))
- fix minor vulnerability alert, updating socket-io dependency [\#10557](https://github.com/netdata/netdata/pull/10557) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix values in Prometheus export for metrics, collected by the Prometheus collector [\#10551](https://github.com/netdata/netdata/pull/10551) ([vlvkobal](https://github.com/vlvkobal))
- Properly handle saved temporary directory on updates. [\#10550](https://github.com/netdata/netdata/pull/10550) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.27.0 [\#10544](https://github.com/netdata/netdata/pull/10544) ([ilyam8](https://github.com/ilyam8))
- Update README.md on postgres collector [\#10532](https://github.com/netdata/netdata/pull/10532) ([OdysLam](https://github.com/OdysLam))
- fix postgres password bug and change default config [\#10531](https://github.com/netdata/netdata/pull/10531) ([OdysLam](https://github.com/OdysLam))
- Make some tweaks/improvements to conf docs [\#10528](https://github.com/netdata/netdata/pull/10528) ([joelhans](https://github.com/joelhans))
- Spelling python plugin [\#10525](https://github.com/netdata/netdata/pull/10525) ([jsoref](https://github.com/jsoref))
- Reduce the number of alarm updates on ACLK [\#10524](https://github.com/netdata/netdata/pull/10524) ([stelfrag](https://github.com/stelfrag))
- dashboard@v2.12.5 [\#10520](https://github.com/netdata/netdata/pull/10520) ([jacekkolasa](https://github.com/jacekkolasa))
- Remove unused entries from structures [\#10519](https://github.com/netdata/netdata/pull/10519) ([stelfrag](https://github.com/stelfrag))
- Mark internal functions as static in health code. [\#10518](https://github.com/netdata/netdata/pull/10518) ([vkalintiris](https://github.com/vkalintiris))
- Remove unused struct in health code. [\#10517](https://github.com/netdata/netdata/pull/10517) ([vkalintiris](https://github.com/vkalintiris))
- Fix coverity issue CID 365322 [\#10516](https://github.com/netdata/netdata/pull/10516) ([stelfrag](https://github.com/stelfrag))
- health/mysql: fix `mysql.slave\_status` alarm for go mysql collector [\#10513](https://github.com/netdata/netdata/pull/10513) ([ilyam8](https://github.com/ilyam8))
- Spelling md [\#10508](https://github.com/netdata/netdata/pull/10508) ([jsoref](https://github.com/jsoref))
- Switched to using system libwebsockets for RPM builds. [\#10507](https://github.com/netdata/netdata/pull/10507) ([Ferroin](https://github.com/Ferroin))
- Add link to specific feedback megathread for the anomalies collector [\#10506](https://github.com/netdata/netdata/pull/10506) ([andrewm4894](https://github.com/andrewm4894))
- Add link to specific feedback megathread for the anomalies collector [\#10505](https://github.com/netdata/netdata/pull/10505) ([andrewm4894](https://github.com/andrewm4894))
- Fix broken dbengine stress tests. [\#10502](https://github.com/netdata/netdata/pull/10502) ([mfundul](https://github.com/mfundul))
- add `\_is\_k8s\_node` label to the host labels [\#10501](https://github.com/netdata/netdata/pull/10501) ([ilyam8](https://github.com/ilyam8))
- Fix segmentation fault in the agent [\#10498](https://github.com/netdata/netdata/pull/10498) ([mfundul](https://github.com/mfundul))
- Fixed handling of TLS config so that cURL works in all cases. [\#10491](https://github.com/netdata/netdata/pull/10491) ([Ferroin](https://github.com/Ferroin))
- Bump ini from 1.3.5 to 1.3.8 [\#10489](https://github.com/netdata/netdata/pull/10489) ([dependabot[bot]](https://github.com/apps/dependabot))
- health: make mdstat\_mismatch\_cnt alarm less strict [\#10488](https://github.com/netdata/netdata/pull/10488) ([ilyam8](https://github.com/ilyam8))
- Mention PostgreSQL Prometheus Adapter in the documentation [\#10487](https://github.com/netdata/netdata/pull/10487) ([vlvkobal](https://github.com/vlvkobal))
- Fix memory allocation when computing standard deviation [\#10484](https://github.com/netdata/netdata/pull/10484) ([stelfrag](https://github.com/stelfrag))
- Support multiple chart label keys in data queries [\#10483](https://github.com/netdata/netdata/pull/10483) ([stelfrag](https://github.com/stelfrag))
- Claiming retry/backoff [\#10482](https://github.com/netdata/netdata/pull/10482) ([underhood](https://github.com/underhood))
- Add guide: Monitor and visualize anomalies with Netdata [\#10480](https://github.com/netdata/netdata/pull/10480) ([joelhans](https://github.com/joelhans))
- Truncate excessive information from titles for apps and cgroups [\#10479](https://github.com/netdata/netdata/pull/10479) ([vlvkobal](https://github.com/vlvkobal))
- Add vkalintiris to CODEOWNERS for CI, packaging, and installer code. [\#10478](https://github.com/netdata/netdata/pull/10478) ([Ferroin](https://github.com/Ferroin))
- GitHub action markdown link check update [\#10474](https://github.com/netdata/netdata/pull/10474) ([jsoref](https://github.com/jsoref))
- Fix for older compilers [\#10470](https://github.com/netdata/netdata/pull/10470) ([underhood](https://github.com/underhood))
- Fixes for SEO housekeeping/improvements [\#10468](https://github.com/netdata/netdata/pull/10468) ([joelhans](https://github.com/joelhans))
- Update README.md [\#10467](https://github.com/netdata/netdata/pull/10467) ([OdysLam](https://github.com/OdysLam))
- Update pfsense.md [\#10466](https://github.com/netdata/netdata/pull/10466) ([OdysLam](https://github.com/OdysLam))
- Fixed function name in updater script. [\#10462](https://github.com/netdata/netdata/pull/10462) ([Ferroin](https://github.com/Ferroin))
- Fixed bundling of libwebsockets in binary packages. [\#10460](https://github.com/netdata/netdata/pull/10460) ([Ferroin](https://github.com/Ferroin))
- Anomalies collector custom model bugfix for issue \#10456 [\#10459](https://github.com/netdata/netdata/pull/10459) ([andrewm4894](https://github.com/andrewm4894))
- Add missing section to Netdata style guide [\#10453](https://github.com/netdata/netdata/pull/10453) ([joelhans](https://github.com/joelhans))
- Add guide: Detect anomalies in nodes and applications with Netdata [\#10451](https://github.com/netdata/netdata/pull/10451) ([joelhans](https://github.com/joelhans))
- Updated messages about checksum validation failures on install. [\#10448](https://github.com/netdata/netdata/pull/10448) ([Ferroin](https://github.com/Ferroin))
- Fixed handling of environment file in updater script. [\#10447](https://github.com/netdata/netdata/pull/10447) ([Ferroin](https://github.com/Ferroin))
- Exclude autofs by default in diskspace plugin [\#10441](https://github.com/netdata/netdata/pull/10441) ([nabijaczleweli](https://github.com/nabijaczleweli))
- New eBPF kernel [\#10434](https://github.com/netdata/netdata/pull/10434) ([thiagoftsm](https://github.com/thiagoftsm))
- Update and improve the Netdata style guide [\#10433](https://github.com/netdata/netdata/pull/10433) ([joelhans](https://github.com/joelhans))
- Change HDDtemp to report None instead of 0 [\#10429](https://github.com/netdata/netdata/pull/10429) ([slavox](https://github.com/slavox))
- Use bash shell as user netdata for debug [\#10425](https://github.com/netdata/netdata/pull/10425) ([Steve8291](https://github.com/Steve8291))
- Qick and dirty fix for \#10420 [\#10424](https://github.com/netdata/netdata/pull/10424) ([skibbipl](https://github.com/skibbipl))
- Add instructions on enabling explicitly disabled collectors [\#10418](https://github.com/netdata/netdata/pull/10418) ([joelhans](https://github.com/joelhans))
- Change links at bottom of all install docs [\#10416](https://github.com/netdata/netdata/pull/10416) ([joelhans](https://github.com/joelhans))
- Improve configuration docs with common changes and start/stop/restart directions [\#10415](https://github.com/netdata/netdata/pull/10415) ([joelhans](https://github.com/joelhans))
- Add Realtek network cards to the list of physical interfaces on FreeBSD [\#10414](https://github.com/netdata/netdata/pull/10414) ([vlvkobal](https://github.com/vlvkobal))
- Update main README with release news [\#10412](https://github.com/netdata/netdata/pull/10412) ([joelhans](https://github.com/joelhans))
- Small updates, improvements, and housekeeping to docs [\#10405](https://github.com/netdata/netdata/pull/10405) ([joelhans](https://github.com/joelhans))
- python.d/fail2ban: Add handling "yes" and "no" as bool, match flexible spaces [\#10400](https://github.com/netdata/netdata/pull/10400) ([grinapo](https://github.com/grinapo))
- Dispatch cgroup discovery into another thread [\#10399](https://github.com/netdata/netdata/pull/10399) ([vlvkobal](https://github.com/vlvkobal))
- Added instructions on which file to edit. [\#10398](https://github.com/netdata/netdata/pull/10398) ([kdvlr](https://github.com/kdvlr))
- Fix data source option for Prometheus web API in exporting configuration [\#10397](https://github.com/netdata/netdata/pull/10397) ([vlvkobal](https://github.com/vlvkobal))
- ACLK collector list use mguid instead of hostname [\#10394](https://github.com/netdata/netdata/pull/10394) ([underhood](https://github.com/underhood))
- Docs housekeeping for SEO and syntax, part 1 [\#10388](https://github.com/netdata/netdata/pull/10388) ([joelhans](https://github.com/joelhans))
- Persist `$TMPDIR` from installer to updater. [\#10384](https://github.com/netdata/netdata/pull/10384) ([Ferroin](https://github.com/Ferroin))
- Add centralized Cloud notifications to core docs [\#10374](https://github.com/netdata/netdata/pull/10374) ([joelhans](https://github.com/joelhans))
- Change linting standard for Markdown lists [\#10371](https://github.com/netdata/netdata/pull/10371) ([joelhans](https://github.com/joelhans))
- Move ACLK Legacy into subfolder [\#10265](https://github.com/netdata/netdata/pull/10265) ([underhood](https://github.com/underhood))

## [v1.28.0](https://github.com/netdata/netdata/tree/v1.28.0) (2020-12-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.27.0...v1.28.0)

**Merged pull requests:**

- Fix locking after on\_connect failure [\#10401](https://github.com/netdata/netdata/pull/10401) ([stelfrag](https://github.com/stelfrag))

## [v1.27.0](https://github.com/netdata/netdata/tree/v1.27.0) (2020-12-17)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.26.0...v1.27.0)

**Merged pull requests:**

- Fixed option parsing in kickstart.sh. [\#10396](https://github.com/netdata/netdata/pull/10396) ([Ferroin](https://github.com/Ferroin))
- invalid\_addr\_cleanup: Initialize variables [\#10395](https://github.com/netdata/netdata/pull/10395) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix a buffer overflow when extracting information from a STREAM connection [\#10391](https://github.com/netdata/netdata/pull/10391) ([stelfrag](https://github.com/stelfrag))
- fix typo in performance.md [\#10386](https://github.com/netdata/netdata/pull/10386) ([OdysLam](https://github.com/OdysLam))
- Fix a lock check [\#10385](https://github.com/netdata/netdata/pull/10385) ([vlvkobal](https://github.com/vlvkobal))
- dashboard v2.11 [\#10383](https://github.com/netdata/netdata/pull/10383) ([jacekkolasa](https://github.com/jacekkolasa))
- Fixed handling of dependencies on Gentoo. [\#10382](https://github.com/netdata/netdata/pull/10382) ([Ferroin](https://github.com/Ferroin))
- Fix issue with chart metadata sent multiple times over ACLK [\#10381](https://github.com/netdata/netdata/pull/10381) ([stelfrag](https://github.com/stelfrag))
- Update macos.md [\#10379](https://github.com/netdata/netdata/pull/10379) ([ktsaou](https://github.com/ktsaou))
- python.d/alarms: fix sending chart definition on every data collection [\#10378](https://github.com/netdata/netdata/pull/10378) ([ilyam8](https://github.com/ilyam8))
- python.d/alarms: add alarms obsoletion and disable by default [\#10375](https://github.com/netdata/netdata/pull/10375) ([ilyam8](https://github.com/ilyam8))
- add two more insignificant warnings to suppress in anomalies collector [\#10369](https://github.com/netdata/netdata/pull/10369) ([andrewm4894](https://github.com/andrewm4894))
- add paragraph in anomalies collector README to ask for feedback [\#10363](https://github.com/netdata/netdata/pull/10363) ([andrewm4894](https://github.com/andrewm4894))
- Fix hostname configuration in the exporting engine [\#10361](https://github.com/netdata/netdata/pull/10361) ([vlvkobal](https://github.com/vlvkobal))
- New ebpf charts [\#10360](https://github.com/netdata/netdata/pull/10360) ([thiagoftsm](https://github.com/thiagoftsm))
- installer: update go.d.plugin version to v0.26.2 [\#10355](https://github.com/netdata/netdata/pull/10355) ([ilyam8](https://github.com/ilyam8))
- Fixed handling of self-updating in updater script. [\#10352](https://github.com/netdata/netdata/pull/10352) ([Ferroin](https://github.com/Ferroin))
- update alarms collector readme image to use one from the related netdata PR [\#10348](https://github.com/netdata/netdata/pull/10348) ([andrewm4894](https://github.com/andrewm4894))
- Add documentation for time & date picker in Agent and Cloud [\#10347](https://github.com/netdata/netdata/pull/10347) ([joelhans](https://github.com/joelhans))
- Use `glibtoolize` on macOS instead of regular `libtoolize`. [\#10346](https://github.com/netdata/netdata/pull/10346) ([Ferroin](https://github.com/Ferroin))
- Fix handling of Python dependency for RPM package. [\#10345](https://github.com/netdata/netdata/pull/10345) ([Ferroin](https://github.com/Ferroin))
- Fixed handling of PowerTools repo on CentOS 8. [\#10344](https://github.com/netdata/netdata/pull/10344) ([Ferroin](https://github.com/Ferroin))
- Fix backend options [\#10343](https://github.com/netdata/netdata/pull/10343) ([vlvkobal](https://github.com/vlvkobal))
- Add guide: Monitor any process in real-time with Netdata [\#10338](https://github.com/netdata/netdata/pull/10338) ([joelhans](https://github.com/joelhans))
- Added patch to build LWS properly on macOS. [\#10333](https://github.com/netdata/netdata/pull/10333) ([Ferroin](https://github.com/Ferroin))
- Added number of allocated/stored objects within each Varnish storage [\#10329](https://github.com/netdata/netdata/pull/10329) ([ernestojpg](https://github.com/ernestojpg))
- health: disable 'used\_file\_descriptors' alarm [\#10328](https://github.com/netdata/netdata/pull/10328) ([ilyam8](https://github.com/ilyam8))
- Fix exporting config [\#10323](https://github.com/netdata/netdata/pull/10323) ([vlvkobal](https://github.com/vlvkobal))
- Fix a compilation warning [\#10320](https://github.com/netdata/netdata/pull/10320) ([vlvkobal](https://github.com/vlvkobal))
- installer: update go.d.plugin version to v0.26.1 [\#10319](https://github.com/netdata/netdata/pull/10319) ([ilyam8](https://github.com/ilyam8))
- Improve core documentation to align with recent Netdata Cloud releases [\#10318](https://github.com/netdata/netdata/pull/10318) ([joelhans](https://github.com/joelhans))
- Added support for MSE \(Massive Storage Engine\) in Varnish-Plus [\#10317](https://github.com/netdata/netdata/pull/10317) ([ernestojpg](https://github.com/ernestojpg))
- dashboard v2.10.1 [\#10314](https://github.com/netdata/netdata/pull/10314) ([jacekkolasa](https://github.com/jacekkolasa))
- fix UUID\_STR\_LEN undefined on MacOS [\#10313](https://github.com/netdata/netdata/pull/10313) ([underhood](https://github.com/underhood))
- python.d/nvidia\_smi: fix gpu data filtering [\#10312](https://github.com/netdata/netdata/pull/10312) ([ilyam8](https://github.com/ilyam8))
- Add new collectors to supported collectors list [\#10310](https://github.com/netdata/netdata/pull/10310) ([joelhans](https://github.com/joelhans))
- Added numerous improvements to our Docker image. [\#10308](https://github.com/netdata/netdata/pull/10308) ([Ferroin](https://github.com/Ferroin))
- Update README.md [\#10303](https://github.com/netdata/netdata/pull/10303) ([ktsaou](https://github.com/ktsaou))
- Add optional info on how to also add some default alarms as part of s… [\#10302](https://github.com/netdata/netdata/pull/10302) ([andrewm4894](https://github.com/andrewm4894))
- Update UPDATE.md [\#10301](https://github.com/netdata/netdata/pull/10301) ([ysamouhos](https://github.com/ysamouhos))
- HAProxy spelling correction [\#10300](https://github.com/netdata/netdata/pull/10300) ([autoalan](https://github.com/autoalan))
- eBPF synchronization [\#10299](https://github.com/netdata/netdata/pull/10299) ([thiagoftsm](https://github.com/thiagoftsm))
- python.d: always create a runtime chart on `create` call [\#10296](https://github.com/netdata/netdata/pull/10296) ([ilyam8](https://github.com/ilyam8))
- Update macOS instructions with cmake [\#10295](https://github.com/netdata/netdata/pull/10295) ([joelhans](https://github.com/joelhans))
- dbengine extent cache [\#10293](https://github.com/netdata/netdata/pull/10293) ([mfundul](https://github.com/mfundul))
- add privacy information about aclk connection [\#10292](https://github.com/netdata/netdata/pull/10292) ([OdysLam](https://github.com/OdysLam))
- Fixed the data endpoint so that the context param is correctly applied to children [\#10290](https://github.com/netdata/netdata/pull/10290) ([stelfrag](https://github.com/stelfrag))
- installer: update go.d.plugin version to v0.26.0 [\#10284](https://github.com/netdata/netdata/pull/10284) ([ilyam8](https://github.com/ilyam8))
- use new libmosquitto release \(with MacOS libMosq fix\) [\#10283](https://github.com/netdata/netdata/pull/10283) ([underhood](https://github.com/underhood))
- Address coverity errors \(CID 364045,364046\) [\#10282](https://github.com/netdata/netdata/pull/10282) ([stelfrag](https://github.com/stelfrag))
- health/web\_log: remove `crit` from unmatched alarms [\#10280](https://github.com/netdata/netdata/pull/10280) ([ilyam8](https://github.com/ilyam8))
- Fix compilation with https disabled. Fixes \#10278 [\#10279](https://github.com/netdata/netdata/pull/10279) ([KickerTom](https://github.com/KickerTom))
- Fix race condition in rrdset\_first\_entry\_t\(\) and rrdset\_last\_entry\_t\(\) [\#10276](https://github.com/netdata/netdata/pull/10276) ([mfundul](https://github.com/mfundul))
- Fix host name when syslog is used [\#10275](https://github.com/netdata/netdata/pull/10275) ([thiagoftsm](https://github.com/thiagoftsm))
- Add guide: How to optimize Netdata's performance [\#10271](https://github.com/netdata/netdata/pull/10271) ([joelhans](https://github.com/joelhans))
- Document the Agent reinstallation process [\#10270](https://github.com/netdata/netdata/pull/10270) ([joelhans](https://github.com/joelhans))
- fix bug\_report.md syntax error [\#10269](https://github.com/netdata/netdata/pull/10269) ([OdysLam](https://github.com/OdysLam))
- python.d/nvidia\_smi: use `pwd` lib to get username if not inside a container [\#10268](https://github.com/netdata/netdata/pull/10268) ([ilyam8](https://github.com/ilyam8))
- Add kernel to blacklist [\#10262](https://github.com/netdata/netdata/pull/10262) ([thiagoftsm](https://github.com/thiagoftsm))
- Made the update script significantly more robust and user friendly. [\#10261](https://github.com/netdata/netdata/pull/10261) ([Ferroin](https://github.com/Ferroin))
- new issue templates [\#10259](https://github.com/netdata/netdata/pull/10259) ([OdysLam](https://github.com/OdysLam))
- Docs: Point users to proper configure doc [\#10254](https://github.com/netdata/netdata/pull/10254) ([joelhans](https://github.com/joelhans))
- Docs: Cleanup and fix broken links [\#10253](https://github.com/netdata/netdata/pull/10253) ([joelhans](https://github.com/joelhans))
- Update CONTRIBUTING.md [\#10252](https://github.com/netdata/netdata/pull/10252) ([joelhans](https://github.com/joelhans))
- updated 3rd party static dependencies and use alpine 3.12 [\#10241](https://github.com/netdata/netdata/pull/10241) ([ktsaou](https://github.com/ktsaou))
- Fix streaming buffer size [\#10240](https://github.com/netdata/netdata/pull/10240) ([vlvkobal](https://github.com/vlvkobal))
- dashboard v2.9.2 [\#10239](https://github.com/netdata/netdata/pull/10239) ([jacekkolasa](https://github.com/jacekkolasa))
- database: avoid endless loop when cleaning obsolete charts [\#10236](https://github.com/netdata/netdata/pull/10236) ([hexchain](https://github.com/hexchain))
- Update ansible.md [\#10232](https://github.com/netdata/netdata/pull/10232) ([voriol](https://github.com/voriol))
- Disable chart obsoletion code for archived chart creation. [\#10231](https://github.com/netdata/netdata/pull/10231) ([mfundul](https://github.com/mfundul))
- add `nvidia\_smi` collector data to the dashboard\_info.js [\#10230](https://github.com/netdata/netdata/pull/10230) ([ilyam8](https://github.com/ilyam8))
- health: convert `elasticsearch\_last\_collected` alarm to template [\#10226](https://github.com/netdata/netdata/pull/10226) ([ilyam8](https://github.com/ilyam8))
- streaming: fix a typo in the README.md [\#10225](https://github.com/netdata/netdata/pull/10225) ([ilyam8](https://github.com/ilyam8))
- collectors/xenstat.plugin: recieved =\> received [\#10224](https://github.com/netdata/netdata/pull/10224) ([ilyam8](https://github.com/ilyam8))
- dashboard\_info.js: fix a typo \(vernemq\) [\#10223](https://github.com/netdata/netdata/pull/10223) ([ilyam8](https://github.com/ilyam8))
- Fix chart filtering [\#10218](https://github.com/netdata/netdata/pull/10218) ([vlvkobal](https://github.com/vlvkobal))
- Don't stop Prometheus remote write collector when data is not available for dimension formatting [\#10217](https://github.com/netdata/netdata/pull/10217) ([vlvkobal](https://github.com/vlvkobal))
- Fix coverity issues [\#10216](https://github.com/netdata/netdata/pull/10216) ([vlvkobal](https://github.com/vlvkobal))
- Fixed bug in auto-updater for FreeBSD \(\#10198\) [\#10204](https://github.com/netdata/netdata/pull/10204) ([abrbon](https://github.com/abrbon))
- New ebpf release [\#10202](https://github.com/netdata/netdata/pull/10202) ([thiagoftsm](https://github.com/thiagoftsm))
- Add guide: Deploy Netdata with Ansible [\#10199](https://github.com/netdata/netdata/pull/10199) ([joelhans](https://github.com/joelhans))
- Add allocated space metrics to oracledb charts [\#10197](https://github.com/netdata/netdata/pull/10197) ([jurgenhaas](https://github.com/jurgenhaas))
- File descr alarm v01 [\#10192](https://github.com/netdata/netdata/pull/10192) ([Ancairon](https://github.com/Ancairon))
- Update CoC and widen scope to community [\#10186](https://github.com/netdata/netdata/pull/10186) ([OdysLam](https://github.com/OdysLam))
- Migrate metadata log to SQLite [\#10139](https://github.com/netdata/netdata/pull/10139) ([stelfrag](https://github.com/stelfrag))
- Kubernetes labels [\#10107](https://github.com/netdata/netdata/pull/10107) ([ilyam8](https://github.com/ilyam8))
- Remove Docker example from update docs and add section to claim troubleshooting [\#10103](https://github.com/netdata/netdata/pull/10103) ([joelhans](https://github.com/joelhans))
- Anomalies collector [\#10060](https://github.com/netdata/netdata/pull/10060) ([andrewm4894](https://github.com/andrewm4894))
- Alarms collector [\#10042](https://github.com/netdata/netdata/pull/10042) ([andrewm4894](https://github.com/andrewm4894))
- ACLK allow child query [\#10030](https://github.com/netdata/netdata/pull/10030) ([underhood](https://github.com/underhood))

## [v1.26.0](https://github.com/netdata/netdata/tree/v1.26.0) (2020-10-14)

[Full Changelog](https://github.com/netdata/netdata/compare/before_rebase...v1.26.0)

**Merged pull requests:**

- Fix systemd comment syntax [\#10066](https://github.com/netdata/netdata/pull/10066) ([HolgerHees](https://github.com/HolgerHees))
- health/portcheck: add `failed` dim to the `connection\_fails` alarm [\#10048](https://github.com/netdata/netdata/pull/10048) ([ilyam8](https://github.com/ilyam8))
- installer: update go.d.plugin version to v0.23.0 [\#10046](https://github.com/netdata/netdata/pull/10046) ([ilyam8](https://github.com/ilyam8))
- Rename NETDATA\_PORT to NETDATA\_LISTENER\_PORT [\#10045](https://github.com/netdata/netdata/pull/10045) ([knatsakis](https://github.com/knatsakis))
- small docs update - adding note about using `nolock` when debugging [\#10036](https://github.com/netdata/netdata/pull/10036) ([andrewm4894](https://github.com/andrewm4894))
- Fixed the data endpoint to prioritize chart over context if both are present [\#10032](https://github.com/netdata/netdata/pull/10032) ([stelfrag](https://github.com/stelfrag))
- python.d/rabbitmq: Add chart for churn rates [\#10031](https://github.com/netdata/netdata/pull/10031) ([chadknutson](https://github.com/chadknutson))

## [before_rebase](https://github.com/netdata/netdata/tree/before_rebase) (2020-09-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.25.0...before_rebase)

## [v1.25.0](https://github.com/netdata/netdata/tree/v1.25.0) (2020-09-15)

[Full Changelog](https://github.com/netdata/netdata/compare/poc2...v1.25.0)

## [poc2](https://github.com/netdata/netdata/tree/poc2) (2020-08-25)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.24.0...poc2)

## [v1.24.0](https://github.com/netdata/netdata/tree/v1.24.0) (2020-08-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.2...v1.24.0)

## [v1.23.2](https://github.com/netdata/netdata/tree/v1.23.2) (2020-07-16)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.1...v1.23.2)

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
