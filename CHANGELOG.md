# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.28.0...HEAD)

**Merged pull requests:**

- Add Realtek network cards to the list of physical interfaces on FreeBSD [\#10414](https://github.com/netdata/netdata/pull/10414) ([vlvkobal](https://github.com/vlvkobal))
- Update main README with release news [\#10412](https://github.com/netdata/netdata/pull/10412) ([joelhans](https://github.com/joelhans))
- Added instructions on which file to edit. [\#10398](https://github.com/netdata/netdata/pull/10398) ([kdvlr](https://github.com/kdvlr))
- Add centralized Cloud notifications to core docs [\#10374](https://github.com/netdata/netdata/pull/10374) ([joelhans](https://github.com/joelhans))

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
- ACLK Child Availability Messages [\#9918](https://github.com/netdata/netdata/pull/9918) ([underhood](https://github.com/underhood))

## [v1.26.0](https://github.com/netdata/netdata/tree/v1.26.0) (2020-10-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.25.0...v1.26.0)

**Merged pull requests:**

- Fix systemd comment syntax [\#10066](https://github.com/netdata/netdata/pull/10066) ([HolgerHees](https://github.com/HolgerHees))
- health/portcheck: add `failed` dim to the `connection\_fails` alarm [\#10048](https://github.com/netdata/netdata/pull/10048) ([ilyam8](https://github.com/ilyam8))
- installer: update go.d.plugin version to v0.23.0 [\#10046](https://github.com/netdata/netdata/pull/10046) ([ilyam8](https://github.com/ilyam8))
- Rename NETDATA\_PORT to NETDATA\_LISTENER\_PORT [\#10045](https://github.com/netdata/netdata/pull/10045) ([knatsakis](https://github.com/knatsakis))
- small docs update - adding note about using `nolock` when debugging [\#10036](https://github.com/netdata/netdata/pull/10036) ([andrewm4894](https://github.com/andrewm4894))
- Fixed the data endpoint to prioritize chart over context if both are present [\#10032](https://github.com/netdata/netdata/pull/10032) ([stelfrag](https://github.com/stelfrag))
- python.d/rabbitmq: Add chart for churn rates [\#10031](https://github.com/netdata/netdata/pull/10031) ([chadknutson](https://github.com/chadknutson))
- Fixed gauges for go web\_log module [\#10029](https://github.com/netdata/netdata/pull/10029) ([hamedbrd](https://github.com/hamedbrd))
- Fixed incorrect condition in updater type detection. [\#10028](https://github.com/netdata/netdata/pull/10028) ([Ferroin](https://github.com/Ferroin))
- Fix README exporting link [\#10020](https://github.com/netdata/netdata/pull/10020) ([Dim-P](https://github.com/Dim-P))
- Clean up and better cross-link new docsv2 documents [\#10015](https://github.com/netdata/netdata/pull/10015) ([joelhans](https://github.com/joelhans))
- collector infiniband: fix file descriptor leak [\#10013](https://github.com/netdata/netdata/pull/10013) ([Saruspete](https://github.com/Saruspete))
- changes default query thread count [\#10009](https://github.com/netdata/netdata/pull/10009) ([underhood](https://github.com/underhood))
- Add missing tests to the web server [\#10008](https://github.com/netdata/netdata/pull/10008) ([thiagoftsm](https://github.com/thiagoftsm))
- Update freebsd.md [\#10005](https://github.com/netdata/netdata/pull/10005) ([disko](https://github.com/disko))
- Add documentation for claiming k8s parent pods and Prometheus service discovery [\#10001](https://github.com/netdata/netdata/pull/10001) ([joelhans](https://github.com/joelhans))
- Add docsv2 project to master branch [\#10000](https://github.com/netdata/netdata/pull/10000) ([joelhans](https://github.com/joelhans))
- Allow connecting to arbitrary MQTT WSS broker for devs [\#9999](https://github.com/netdata/netdata/pull/9999) ([underhood](https://github.com/underhood))
- minor - removes leading whitespace before JSON in ACLK [\#9998](https://github.com/netdata/netdata/pull/9998) ([underhood](https://github.com/underhood))
- Fixed typos in installer functions. [\#9992](https://github.com/netdata/netdata/pull/9992) ([Ferroin](https://github.com/Ferroin))
- Fixed locking order to address CID\_362348 [\#9991](https://github.com/netdata/netdata/pull/9991) ([stelfrag](https://github.com/stelfrag))
- Switched to our installer's bundling code for libJudy in static installs. [\#9988](https://github.com/netdata/netdata/pull/9988) ([Ferroin](https://github.com/Ferroin))
- Fix cleanup of obsolete charts [\#9985](https://github.com/netdata/netdata/pull/9985) ([mfundul](https://github.com/mfundul))
- Added more stringent check for C99 support in configure script. [\#9982](https://github.com/netdata/netdata/pull/9982) ([Ferroin](https://github.com/Ferroin))
- Improved the data query when using the context parameter [\#9978](https://github.com/netdata/netdata/pull/9978) ([stelfrag](https://github.com/stelfrag))
- Fix missing libelf-dev dependency. [\#9974](https://github.com/netdata/netdata/pull/9974) ([roedie](https://github.com/roedie))
- allow using LWS without SOCKS5 [\#9973](https://github.com/netdata/netdata/pull/9973) ([underhood](https://github.com/underhood))
- Cleanup CODEOWNERS [\#9971](https://github.com/netdata/netdata/pull/9971) ([prologic](https://github.com/prologic))
- Fix Stackpulse doc [\#9968](https://github.com/netdata/netdata/pull/9968) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix setting for disabling eBPF-apps.plugin integration [\#9967](https://github.com/netdata/netdata/pull/9967) ([joelhans](https://github.com/joelhans))
- Added improved auto-update support. [\#9966](https://github.com/netdata/netdata/pull/9966) ([Ferroin](https://github.com/Ferroin))
- Stackpulse integration [\#9965](https://github.com/netdata/netdata/pull/9965) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix typo inside netdata-installer.sh [\#9962](https://github.com/netdata/netdata/pull/9962) ([thiagoftsm](https://github.com/thiagoftsm))
- Add missing period in netdata dashboard [\#9960](https://github.com/netdata/netdata/pull/9960) ([hydrogen-mvm](https://github.com/hydrogen-mvm))
- Fixed chart's last accessed time during context queries [\#9952](https://github.com/netdata/netdata/pull/9952) ([stelfrag](https://github.com/stelfrag))
- adds missing file netdata.crontab to gitignore [\#9946](https://github.com/netdata/netdata/pull/9946) ([underhood](https://github.com/underhood))
- Updated RPM spec file to use automatic dependency list generation. [\#9937](https://github.com/netdata/netdata/pull/9937) ([Ferroin](https://github.com/Ferroin))
- Add information about Cloud disabled status to `-W buildinfo`. [\#9936](https://github.com/netdata/netdata/pull/9936) ([underhood](https://github.com/underhood))
- Fix resource leak in case of malformed cloud request [\#9934](https://github.com/netdata/netdata/pull/9934) ([underhood](https://github.com/underhood))
- Added context parameter to the data endpoint [\#9931](https://github.com/netdata/netdata/pull/9931) ([stelfrag](https://github.com/stelfrag))
- Add notice to Docker docs about systemd volumes [\#9927](https://github.com/netdata/netdata/pull/9927) ([thiagoftsm](https://github.com/thiagoftsm))
- Add persistent configuration details to Docker docs [\#9926](https://github.com/netdata/netdata/pull/9926) ([joelhans](https://github.com/joelhans))
- Update claiming document to instruct users to install `uuidgen`. [\#9925](https://github.com/netdata/netdata/pull/9925) ([OdysLam](https://github.com/OdysLam))
- Added a way to get build configuration info from the agent. [\#9913](https://github.com/netdata/netdata/pull/9913) ([Ferroin](https://github.com/Ferroin))
- add mirrored\_hosts\_status into OpenAPI of api/info [\#9867](https://github.com/netdata/netdata/pull/9867) ([underhood](https://github.com/underhood))
- Fix build for the AWS Kinesis exporting connector [\#9823](https://github.com/netdata/netdata/pull/9823) ([vlvkobal](https://github.com/vlvkobal))

## [v1.25.0](https://github.com/netdata/netdata/tree/v1.25.0) (2020-09-15)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.24.0...v1.25.0)

**Merged pull requests:**

- Fix memory mode none not dropping stale dimension data [\#9917](https://github.com/netdata/netdata/pull/9917) ([mfundul](https://github.com/mfundul))
- Fix memory mode none not marking dimensions as obsolete. [\#9912](https://github.com/netdata/netdata/pull/9912) ([mfundul](https://github.com/mfundul))
- Fix buffer overflow in rrdr structure [\#9903](https://github.com/netdata/netdata/pull/9903) ([mfundul](https://github.com/mfundul))
- Fix missing newline concatentation slash causing rpm build to fail [\#9900](https://github.com/netdata/netdata/pull/9900) ([prologic](https://github.com/prologic))
- installer: update go.d.plugin version to v0.22.0 [\#9898](https://github.com/netdata/netdata/pull/9898) ([ilyam8](https://github.com/ilyam8))
- Add v2 HTTP message with compression to ACLK [\#9895](https://github.com/netdata/netdata/pull/9895) ([underhood](https://github.com/underhood))
- Fix lock order reversal \(Coverity defect CID 361629\) [\#9888](https://github.com/netdata/netdata/pull/9888) ([mfundul](https://github.com/mfundul))
- Fix HTTP error messages in alarm notifications [\#9887](https://github.com/netdata/netdata/pull/9887) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix missing macOS RAM info in `system-info.sh` [\#9882](https://github.com/netdata/netdata/pull/9882) ([weijing24](https://github.com/weijing24))
- Update go.d.plugin version to v0.21.0 [\#9881](https://github.com/netdata/netdata/pull/9881) ([ilyam8](https://github.com/ilyam8))
- Fix handling of libJudy bundling for RPM packages. [\#9875](https://github.com/netdata/netdata/pull/9875) ([Ferroin](https://github.com/Ferroin))
- Fix latency-avg chart units in `python.d/dnsdist` [\#9871](https://github.com/netdata/netdata/pull/9871) ([scottymuse](https://github.com/scottymuse))
- Change instruction to reload HEALTH [\#9869](https://github.com/netdata/netdata/pull/9869) ([thiagoftsm](https://github.com/thiagoftsm))
- labeler: add `area/exporting` [\#9866](https://github.com/netdata/netdata/pull/9866) ([ilyam8](https://github.com/ilyam8))
- Fix race condition with orphan hosts [\#9862](https://github.com/netdata/netdata/pull/9862) ([mfundul](https://github.com/mfundul))
- Fix typo in health documentation [\#9860](https://github.com/netdata/netdata/pull/9860) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove dependency on libJudy for systems which don't have it [\#9859](https://github.com/netdata/netdata/pull/9859) ([Ferroin](https://github.com/Ferroin))
- Fix multi-host DB corruption when legacy metrics reside in localhost. [\#9855](https://github.com/netdata/netdata/pull/9855) ([mfundul](https://github.com/mfundul))
- Fix TLS over LDAP in the `python.d/openldap` collector [\#9853](https://github.com/netdata/netdata/pull/9853) ([scatenag](https://github.com/scatenag))
- Fix broken `Edit this page` link in simple patterns doc [\#9847](https://github.com/netdata/netdata/pull/9847) ([joelhans](https://github.com/joelhans))
- Fixes compilation warnings on FreeBSD [\#9845](https://github.com/netdata/netdata/pull/9845) ([underhood](https://github.com/underhood))
- Fix installation to not install eBPF plugin components when they shouldn't be installed [\#9844](https://github.com/netdata/netdata/pull/9844) ([vlvkobal](https://github.com/vlvkobal))
- Add collecting active processes limit on Linux systems [\#9843](https://github.com/netdata/netdata/pull/9843) ([Ancairon](https://github.com/Ancairon))
- Fixed tmpdir handling failure on macOS/FreeBSD. [\#9842](https://github.com/netdata/netdata/pull/9842) ([Ferroin](https://github.com/Ferroin))
- Fix bugs in handling of Python 3 dependencies on install [\#9839](https://github.com/netdata/netdata/pull/9839) ([Ferroin](https://github.com/Ferroin))
- dashboard v1.4.2 [\#9837](https://github.com/netdata/netdata/pull/9837) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix the log level in cgroup-network helper [\#9836](https://github.com/netdata/netdata/pull/9836) ([vlvkobal](https://github.com/vlvkobal))
- Fix `netdata-uninstaller.sh` to correctly state whether the group was deleted [\#9835](https://github.com/netdata/netdata/pull/9835) ([michmach](https://github.com/michmach))
- Fix updater bug introduced by incomplete variable rename in \#8808 [\#9834](https://github.com/netdata/netdata/pull/9834) ([Ferroin](https://github.com/Ferroin))
- Fixes proxy forwarding claim\_id to old parent [\#9828](https://github.com/netdata/netdata/pull/9828) ([underhood](https://github.com/underhood))
- Remove Google Charts info from API doc [\#9826](https://github.com/netdata/netdata/pull/9826) ([joelhans](https://github.com/joelhans))
- Fix empty dbengine files [\#9820](https://github.com/netdata/netdata/pull/9820) ([mfundul](https://github.com/mfundul))
- Add version negotiation to ACLK [\#9819](https://github.com/netdata/netdata/pull/9819) ([underhood](https://github.com/underhood))
- Improve dbengine docs and add new multihost setting [\#9817](https://github.com/netdata/netdata/pull/9817) ([joelhans](https://github.com/joelhans))
- Fix old dashboard third-party packaging [\#9814](https://github.com/netdata/netdata/pull/9814) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix broken link and clean up frontmatter in health docs [\#9813](https://github.com/netdata/netdata/pull/9813) ([joelhans](https://github.com/joelhans))
- Fix docker packaging caddyserver basicauth link [\#9812](https://github.com/netdata/netdata/pull/9812) ([pando85](https://github.com/pando85))
- Fix install if system does not have ebpf.plugin [\#9809](https://github.com/netdata/netdata/pull/9809) ([roedie](https://github.com/roedie))
- Fix RPM build script version issues [\#9808](https://github.com/netdata/netdata/pull/9808) ([Saruspete](https://github.com/Saruspete))
- Fix handling of offline installs [\#9805](https://github.com/netdata/netdata/pull/9805) ([Ferroin](https://github.com/Ferroin))
- Add `claimed\_id` for child nodes streamed to their parents [\#9804](https://github.com/netdata/netdata/pull/9804) ([underhood](https://github.com/underhood))
- Improve temporary directory checking in installer and updater [\#9797](https://github.com/netdata/netdata/pull/9797) ([Ferroin](https://github.com/Ferroin))
- Fix loading custom dashboard\_info in /old dashboard [\#9792](https://github.com/netdata/netdata/pull/9792) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix redirect with parameters [\#9790](https://github.com/netdata/netdata/pull/9790) ([thiagoftsm](https://github.com/thiagoftsm))
- Update dashboard to v1.3.1 [\#9786](https://github.com/netdata/netdata/pull/9786) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix long stats.d chart names \(suggested by @vince-lessbits\) [\#9783](https://github.com/netdata/netdata/pull/9783) ([amoss](https://github.com/amoss))
- Fix Travis CI builds and skip Fedora 31 i386 build/test cycles [\#9781](https://github.com/netdata/netdata/pull/9781) ([prologic](https://github.com/prologic))
- Fix UNIX socket access with kickstart-static64 [\#9780](https://github.com/netdata/netdata/pull/9780) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix timestamps for global variables in Prometheus output [\#9779](https://github.com/netdata/netdata/pull/9779) ([vlvkobal](https://github.com/vlvkobal))
- Add code to bundle libJudy on systems which do not provide a usable copy of it [\#9776](https://github.com/netdata/netdata/pull/9776) ([Ferroin](https://github.com/Ferroin))
- Fix HTTP header for the remote write exporting connector [\#9775](https://github.com/netdata/netdata/pull/9775) ([vlvkobal](https://github.com/vlvkobal))

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
