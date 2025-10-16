# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.7.2...HEAD)

**Merged pull requests:**

- Customize node name addition [\#21151](https://github.com/netdata/netdata/pull/21151) ([kanelatechnical](https://github.com/kanelatechnical))
- Skip adding Sentry breadcrumb during shutdown timeout [\#21150](https://github.com/netdata/netdata/pull/21150) ([stelfrag](https://github.com/stelfrag))
- Fix NaN check in anomaly score calculation [\#21149](https://github.com/netdata/netdata/pull/21149) ([stelfrag](https://github.com/stelfrag))
- Additional checks during cgroup discovery [\#21148](https://github.com/netdata/netdata/pull/21148) ([stelfrag](https://github.com/stelfrag))
- Fix AS400 metrics [\#21147](https://github.com/netdata/netdata/pull/21147) ([ktsaou](https://github.com/ktsaou))
- Add Fedora 43 to CI and package builds. [\#21142](https://github.com/netdata/netdata/pull/21142) ([Ferroin](https://github.com/Ferroin))
- chore\(go.d\): update dyncfg path [\#21141](https://github.com/netdata/netdata/pull/21141) ([ilyam8](https://github.com/ilyam8))
- Skip status file on windows on crash [\#21140](https://github.com/netdata/netdata/pull/21140) ([stelfrag](https://github.com/stelfrag))
- improve\(go.d/snmp\): automatically disable SNMP bulkwalk when not supported [\#21139](https://github.com/netdata/netdata/pull/21139) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/gohugoio/hashstructure from 0.5.0 to 0.6.0 in /src/go [\#21138](https://github.com/netdata/netdata/pull/21138) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/net from 0.44.0 to 0.46.0 in /src/go [\#21137](https://github.com/netdata/netdata/pull/21137) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.5.0+incompatible to 28.5.1+incompatible in /src/go [\#21135](https://github.com/netdata/netdata/pull/21135) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/gofrs/flock from 0.12.1 to 0.13.0 in /src/go [\#21134](https://github.com/netdata/netdata/pull/21134) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore: move go.d/ibm.d shared pkgs out of go.d [\#21132](https://github.com/netdata/netdata/pull/21132) ([ilyam8](https://github.com/ilyam8))
- Switch to using a relative RUNPATH for IBM plugin library lookup. [\#21131](https://github.com/netdata/netdata/pull/21131) ([Ferroin](https://github.com/Ferroin))
- build: update go otel deps [\#21129](https://github.com/netdata/netdata/pull/21129) ([ilyam8](https://github.com/ilyam8))
- chore: go.d/ibm.d various fixes [\#21128](https://github.com/netdata/netdata/pull/21128) ([ilyam8](https://github.com/ilyam8))
- Improve agent startup on windows [\#21125](https://github.com/netdata/netdata/pull/21125) ([stelfrag](https://github.com/stelfrag))
- fix\(ibm.d/mq\): change default ExponentialBackoff attempts to 2 [\#21124](https://github.com/netdata/netdata/pull/21124) ([ilyam8](https://github.com/ilyam8))
- fix\(ibm.d\): various fixes [\#21123](https://github.com/netdata/netdata/pull/21123) ([ilyam8](https://github.com/ilyam8))
- Update IBM plugin documentation. [\#21122](https://github.com/netdata/netdata/pull/21122) ([Ferroin](https://github.com/Ferroin))
- Improve free disk space calculation for Windows [\#21121](https://github.com/netdata/netdata/pull/21121) ([stelfrag](https://github.com/stelfrag))
- Fix issues with IBM libs plugin. [\#21120](https://github.com/netdata/netdata/pull/21120) ([Ferroin](https://github.com/Ferroin))
- Make native package dependencies consistent between DEB/RPM packages. [\#21118](https://github.com/netdata/netdata/pull/21118) ([Ferroin](https://github.com/Ferroin))
- Properly include client compoents for IBM MQ libraries. [\#21117](https://github.com/netdata/netdata/pull/21117) ([Ferroin](https://github.com/Ferroin))
- Properly check for ODBC for IBM plugin at configuration time. [\#21116](https://github.com/netdata/netdata/pull/21116) ([Ferroin](https://github.com/Ferroin))
- Fix windows build [\#21113](https://github.com/netdata/netdata/pull/21113) ([stelfrag](https://github.com/stelfrag))
- Address NULL access \(windows.plugin\) [\#21112](https://github.com/netdata/netdata/pull/21112) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/prometheus/common from 0.66.1 to 0.67.1 in /src/go [\#21111](https://github.com/netdata/netdata/pull/21111) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/ibmdb/go\_ibm\_db from 0.4.5 to 0.5.3 in /src/go [\#21110](https://github.com/netdata/netdata/pull/21110) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix freeipmi crash [\#21109](https://github.com/netdata/netdata/pull/21109) ([stelfrag](https://github.com/stelfrag))
- Fix invalid map.csv [\#21108](https://github.com/netdata/netdata/pull/21108) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations docs [\#21106](https://github.com/netdata/netdata/pull/21106) ([netdatabot](https://github.com/netdatabot))
- Update `cloud-notifications` documentation [\#21105](https://github.com/netdata/netdata/pull/21105) ([car12o](https://github.com/car12o))
- improve\(go.d/snmp\): Add APC PowerNet-MIB sysObjectID mappings and categories [\#21104](https://github.com/netdata/netdata/pull/21104) ([ilyam8](https://github.com/ilyam8))
- Update building-native-packages-locally.md [\#21101](https://github.com/netdata/netdata/pull/21101) ([Ferroin](https://github.com/Ferroin))
- Add openSUSE Leap 16.0 and Ubuntu 25.10 to CI and package builds. [\#21100](https://github.com/netdata/netdata/pull/21100) ([Ferroin](https://github.com/Ferroin))
- Improve logging and packet handling for unknown packet IDs [\#21099](https://github.com/netdata/netdata/pull/21099) ([stelfrag](https://github.com/stelfrag))
- Use datafile block pos [\#21098](https://github.com/netdata/netdata/pull/21098) ([stelfrag](https://github.com/stelfrag))
- fix\(packaging/docker\): add missing nd-run [\#21097](https://github.com/netdata/netdata/pull/21097) ([ilyam8](https://github.com/ilyam8))
- Update installer documentation [\#21096](https://github.com/netdata/netdata/pull/21096) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/docker/docker from 28.4.0+incompatible to 28.5.0+incompatible in /src/go [\#21095](https://github.com/netdata/netdata/pull/21095) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/go-ldap/ldap/v3 from 3.4.11 to 3.4.12 in /src/go [\#21094](https://github.com/netdata/netdata/pull/21094) ([dependabot[bot]](https://github.com/apps/dependabot))
- Hide  mem\_private\_usage on Windows. [\#21093](https://github.com/netdata/netdata/pull/21093) ([thiagoftsm](https://github.com/thiagoftsm))
- Event loop cleanup [\#21091](https://github.com/netdata/netdata/pull/21091) ([stelfrag](https://github.com/stelfrag))
- Improve installer \(Windows\) [\#21090](https://github.com/netdata/netdata/pull/21090) ([thiagoftsm](https://github.com/thiagoftsm))
- Correctly split MCP registry update to it’s own workflow. [\#21089](https://github.com/netdata/netdata/pull/21089) ([Ferroin](https://github.com/Ferroin))
- Register Netdata to MCP Registry [\#21088](https://github.com/netdata/netdata/pull/21088) ([ktsaou](https://github.com/ktsaou))
- MCP docs and log spamming fix [\#21087](https://github.com/netdata/netdata/pull/21087) ([ktsaou](https://github.com/ktsaou))
- Fix app.mem\_usage  \(Windows\) [\#21085](https://github.com/netdata/netdata/pull/21085) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix duplicate header leak in ACLK HTTPS client [\#21084](https://github.com/netdata/netdata/pull/21084) ([ktsaou](https://github.com/ktsaou))
- Adjust Disk Size \(Windows.plugin\) [\#21081](https://github.com/netdata/netdata/pull/21081) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix cgroup-network spawn server cleanup on fatal exit [\#21080](https://github.com/netdata/netdata/pull/21080) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#21079](https://github.com/netdata/netdata/pull/21079) ([netdatabot](https://github.com/netdatabot))
- docs: update SNMP collector metadata to reflect profile-based collection [\#21078](https://github.com/netdata/netdata/pull/21078) ([ilyam8](https://github.com/ilyam8))
- build\(go\): add config dirs [\#21077](https://github.com/netdata/netdata/pull/21077) ([ilyam8](https://github.com/ilyam8))
- Make `nd-run` silent unless exiting with an error [\#21076](https://github.com/netdata/netdata/pull/21076) ([ilyam8](https://github.com/ilyam8))
- docs: add note about using ``--init` when not running with `pid: host` [\#21075](https://github.com/netdata/netdata/pull/21075) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump openssl version in static build [\#21074](https://github.com/netdata/netdata/pull/21074) ([ilyam8](https://github.com/ilyam8))
- Updated child node behaviour change [\#21073](https://github.com/netdata/netdata/pull/21073) ([kanelatechnical](https://github.com/kanelatechnical))
- Declare flatten-serde-json at the workspace. [\#21072](https://github.com/netdata/netdata/pull/21072) ([vkalintiris](https://github.com/vkalintiris))
- Add missing extension go.d \(Windows\) [\#21070](https://github.com/netdata/netdata/pull/21070) ([thiagoftsm](https://github.com/thiagoftsm))
- Win plugin files with .plugin extension [\#21068](https://github.com/netdata/netdata/pull/21068) ([stelfrag](https://github.com/stelfrag))
- Convert go collectors to use ndexec module for external command invocation [\#21067](https://github.com/netdata/netdata/pull/21067) ([Ferroin](https://github.com/Ferroin))
- ibm.d.plugin: i, db2, mq, websphere [\#21066](https://github.com/netdata/netdata/pull/21066) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#21065](https://github.com/netdata/netdata/pull/21065) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/snmp\): add `ping_only` option [\#21064](https://github.com/netdata/netdata/pull/21064) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsmp\): profile definition cleanup [\#21062](https://github.com/netdata/netdata/pull/21062) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#21058](https://github.com/netdata/netdata/pull/21058) ([netdatabot](https://github.com/netdatabot))
- chore\(go.d/snmp\): remove legacy custom oid collection [\#21056](https://github.com/netdata/netdata/pull/21056) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#21055](https://github.com/netdata/netdata/pull/21055) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/snmp\): enable ping by default [\#21054](https://github.com/netdata/netdata/pull/21054) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#21053](https://github.com/netdata/netdata/pull/21053) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/snmp\): add optional ICMP ping metrics [\#21052](https://github.com/netdata/netdata/pull/21052) ([ilyam8](https://github.com/ilyam8))
- Fix libbpf.a build path [\#21051](https://github.com/netdata/netdata/pull/21051) ([hack3ric](https://github.com/hack3ric))
- improve\(go.d/rabbitmq\): add support for old RabbitMQ whoami tags format [\#21049](https://github.com/netdata/netdata/pull/21049) ([ilyam8](https://github.com/ilyam8))
- ml: implement fixed time-based training windows - corrected [\#21046](https://github.com/netdata/netdata/pull/21046) ([ktsaou](https://github.com/ktsaou))
- Add documentation and fallback to /host/ for getting the machine id [\#21044](https://github.com/netdata/netdata/pull/21044) ([vkalintiris](https://github.com/vkalintiris))
- ai-docs [\#21043](https://github.com/netdata/netdata/pull/21043) ([shyamvalsan](https://github.com/shyamvalsan))
- Remote MCP support \(streamable http and sse\) [\#21036](https://github.com/netdata/netdata/pull/21036) ([ktsaou](https://github.com/ktsaou))
- Add helper to run external commands without additional privileges. [\#20990](https://github.com/netdata/netdata/pull/20990) ([Ferroin](https://github.com/Ferroin))
- Clean up handling of compiler flags in our build code. [\#20821](https://github.com/netdata/netdata/pull/20821) ([Ferroin](https://github.com/Ferroin))

## [v2.7.2](https://github.com/netdata/netdata/tree/v2.7.2) (2025-10-15)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.7.1...v2.7.2)

## [v2.7.1](https://github.com/netdata/netdata/tree/v2.7.1) (2025-09-29)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.7.0...v2.7.1)

## [v2.7.0](https://github.com/netdata/netdata/tree/v2.7.0) (2025-09-25)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.6.3...v2.7.0)

**Merged pull requests:**

- Revert "ml: implement fixed time-based training windows \(\#20638\)" [\#21045](https://github.com/netdata/netdata/pull/21045) ([vkalintiris](https://github.com/vkalintiris))
- chore\(go.d/ddsnmp\): Improve profile sorting by match specificity [\#21042](https://github.com/netdata/netdata/pull/21042) ([ilyam8](https://github.com/ilyam8))
- Add documentation on using custom CA certificates to Learn [\#21041](https://github.com/netdata/netdata/pull/21041) ([ralphm](https://github.com/ralphm))
- Context loading priority to vnodes [\#21040](https://github.com/netdata/netdata/pull/21040) ([stelfrag](https://github.com/stelfrag))
- improve\(go.d/ddsnmp\): switch profile matching to `selector` [\#21039](https://github.com/netdata/netdata/pull/21039) ([ilyam8](https://github.com/ilyam8))
- fix\(docs\): update mermaid diagrams leftovers plus syntax issues [\#21034](https://github.com/netdata/netdata/pull/21034) ([kanelatechnical](https://github.com/kanelatechnical))
- docs: fix mdx parsing scalability.md [\#21032](https://github.com/netdata/netdata/pull/21032) ([ilyam8](https://github.com/ilyam8))
- Revert "chore\(docs\): rename REST API sidebar to Netdata APIs" [\#21031](https://github.com/netdata/netdata/pull/21031) ([ilyam8](https://github.com/ilyam8))
- Update scalability.md [\#21030](https://github.com/netdata/netdata/pull/21030) ([kanelatechnical](https://github.com/kanelatechnical))
- chore\(docs\): rename REST API sidebar to Netdata APIs [\#21029](https://github.com/netdata/netdata/pull/21029) ([kanelatechnical](https://github.com/kanelatechnical))
- Regenerate integrations docs [\#21028](https://github.com/netdata/netdata/pull/21028) ([netdatabot](https://github.com/netdatabot))
- Update README.md [\#21027](https://github.com/netdata/netdata/pull/21027) ([kanelatechnical](https://github.com/kanelatechnical))
- chore\(go.d/snmp\): remove/disable legacy components [\#21026](https://github.com/netdata/netdata/pull/21026) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): update go toolchain to v1.25.1 [\#21025](https://github.com/netdata/netdata/pull/21025) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump openssl and curl version in static build [\#21024](https://github.com/netdata/netdata/pull/21024) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add `manual_profiles` option [\#21023](https://github.com/netdata/netdata/pull/21023) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): update vmBuildGroupKey in per-row mode without group\_by [\#21022](https://github.com/netdata/netdata/pull/21022) ([ilyam8](https://github.com/ilyam8))
- Fix duplicate ToC entries for Top Consumers [\#21021](https://github.com/netdata/netdata/pull/21021) ([ktsaou](https://github.com/ktsaou))
- docs: Remove parentheses from Top Consumers title to fix URL issues [\#21020](https://github.com/netdata/netdata/pull/21020) ([ktsaou](https://github.com/ktsaou))
- docs: Rename 'Top Monitoring' to 'Top Consumers' in Functions documentation [\#21019](https://github.com/netdata/netdata/pull/21019) ([ktsaou](https://github.com/ktsaou))
- docs: Add comprehensive scalability architecture documentation [\#21018](https://github.com/netdata/netdata/pull/21018) ([ktsaou](https://github.com/ktsaou))
- improve\(go.d/ddsnmp\): refactor IF-MIB profile with unified virtual metrics and 64-bit preference [\#21017](https://github.com/netdata/netdata/pull/21017) ([ilyam8](https://github.com/ilyam8))
- Fix MDX parsing errors in realtime-monitoring.md [\#21016](https://github.com/netdata/netdata/pull/21016) ([ktsaou](https://github.com/ktsaou))
- Fix MDX compilation errors in realtime-monitoring.md [\#21015](https://github.com/netdata/netdata/pull/21015) ([ktsaou](https://github.com/ktsaou))
- chore\(go.d\): don’t log "no such file or directory" for user SNMP profiles [\#21014](https://github.com/netdata/netdata/pull/21014) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/ddsnmp\): add alternatives support for virtual metrics [\#21013](https://github.com/netdata/netdata/pull/21013) ([ilyam8](https://github.com/ilyam8))
- docs: Update ToC - add realtime-monitoring and reorganize functions [\#21012](https://github.com/netdata/netdata/pull/21012) ([ktsaou](https://github.com/ktsaou))
- Fix Mermaid diagrams across multiple files [\#21011](https://github.com/netdata/netdata/pull/21011) ([kanelatechnical](https://github.com/kanelatechnical))
- chore\(go.d/snmp\): remove recreating client on "packet is not authentic" [\#21010](https://github.com/netdata/netdata/pull/21010) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): use forked gosnmp to fix SNMPv3 REPORT handling for UniFi APs [\#21009](https://github.com/netdata/netdata/pull/21009) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): correct buf truncate in processMetrics [\#21008](https://github.com/netdata/netdata/pull/21008) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#21007](https://github.com/netdata/netdata/pull/21007) ([netdatabot](https://github.com/netdatabot))
- deps\(mcp/bridge/stdio-golang\): switch to github.com/coder/websocket [\#21006](https://github.com/netdata/netdata/pull/21006) ([ilyam8](https://github.com/ilyam8))
- Rework dbengine async wakeup on windows [\#21003](https://github.com/netdata/netdata/pull/21003) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#21002](https://github.com/netdata/netdata/pull/21002) ([netdatabot](https://github.com/netdatabot))
- docs: add "cpu idle state" procstat config option [\#21001](https://github.com/netdata/netdata/pull/21001) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#21000](https://github.com/netdata/netdata/pull/21000) ([netdatabot](https://github.com/netdatabot))
- docs: Use fallback title "Config options" when folding title is empty [\#20999](https://github.com/netdata/netdata/pull/20999) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20998](https://github.com/netdata/netdata/pull/20998) ([netdatabot](https://github.com/netdatabot))
- docs: add "per cpu core utilization" option to proc/stat meta [\#20997](https://github.com/netdata/netdata/pull/20997) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): skip HOST for vnodes during Cleanup with stale label [\#20996](https://github.com/netdata/netdata/pull/20996) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): skip writing HOST line for vnodes with no collected metrics [\#20995](https://github.com/netdata/netdata/pull/20995) ([ilyam8](https://github.com/ilyam8))
- fix\(macos.plugin\): drop SyncookiesFailed metric on macOS 16+ [\#20994](https://github.com/netdata/netdata/pull/20994) ([ilyam8](https://github.com/ilyam8))
- ci: rm macos 13, add macos 26 [\#20993](https://github.com/netdata/netdata/pull/20993) ([ilyam8](https://github.com/ilyam8))
- Document accuracy implications of sampling algorithm [\#20991](https://github.com/netdata/netdata/pull/20991) ([ktsaou](https://github.com/ktsaou))
- docs: add grouped headers to config options [\#20987](https://github.com/netdata/netdata/pull/20987) ([ilyam8](https://github.com/ilyam8))
- Update welcome-to-netdata.md [\#20986](https://github.com/netdata/netdata/pull/20986) ([kanelatechnical](https://github.com/kanelatechnical))
- Update best-practices.md [\#20984](https://github.com/netdata/netdata/pull/20984) ([kanelatechnical](https://github.com/kanelatechnical))
- chore: refactor get\_doc\_integrations.py to use main\(\) and improve structure [\#20983](https://github.com/netdata/netdata/pull/20983) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20982](https://github.com/netdata/netdata/pull/20982) ([netdatabot](https://github.com/netdatabot))
- chore: add `-c` option to gen docs for a single collector [\#20981](https://github.com/netdata/netdata/pull/20981) ([ilyam8](https://github.com/ilyam8))
- docs: improve config options table with grouped section headers [\#20980](https://github.com/netdata/netdata/pull/20980) ([ilyam8](https://github.com/ilyam8))
- perf\(go.d/ddsnmp\): cache group key per aggregator per metric [\#20979](https://github.com/netdata/netdata/pull/20979) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20978](https://github.com/netdata/netdata/pull/20978) ([netdatabot](https://github.com/netdatabot))
- docs: restructure Setup, update SNMP prerequisites, and fix SNMP options [\#20977](https://github.com/netdata/netdata/pull/20977) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20976](https://github.com/netdata/netdata/pull/20976) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d\): add UI configuration instructions and restructure Configuration section [\#20975](https://github.com/netdata/netdata/pull/20975) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20973](https://github.com/netdata/netdata/pull/20973) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/snmp\): remove legacy "custom oid" cfg examples [\#20972](https://github.com/netdata/netdata/pull/20972) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/ddsnmp\): add per\_row mode for virtual metrics [\#20971](https://github.com/netdata/netdata/pull/20971) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/ddsnmp\): add group\_by for virtual metrics [\#20970](https://github.com/netdata/netdata/pull/20970) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump k8s.io/client-go from 0.34.0 to 0.34.1 in /src/go [\#20968](https://github.com/netdata/netdata/pull/20968) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.13.0 to 9.14.0 in /src/go [\#20966](https://github.com/netdata/netdata/pull/20966) ([dependabot[bot]](https://github.com/apps/dependabot))
- improve\(go.d/ddsnmp\): add composite metrics for CPU & Load Average in std-ucd-mib [\#20964](https://github.com/netdata/netdata/pull/20964) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/ddsnmp\): add composite virtual metrics [\#20962](https://github.com/netdata/netdata/pull/20962) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/sd/snmp\): remove skipping servers in snmp discovery [\#20960](https://github.com/netdata/netdata/pull/20960) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add net-snmp.yaml profile [\#20959](https://github.com/netdata/netdata/pull/20959) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): remove hrStorageTable from mikrotik-router.yaml [\#20958](https://github.com/netdata/netdata/pull/20958) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add transform for Host-Resources-MIB storage metrics [\#20957](https://github.com/netdata/netdata/pull/20957) ([ilyam8](https://github.com/ilyam8))
- Ensure memory ordering when updating partition list and the bitmap [\#20956](https://github.com/netdata/netdata/pull/20956) ([stelfrag](https://github.com/stelfrag))
- Fix FreeBSD \(Part II\) [\#20955](https://github.com/netdata/netdata/pull/20955) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(health/rocketchat\): add missing "Content-Type: application/json" header [\#20954](https://github.com/netdata/netdata/pull/20954) ([ilyam8](https://github.com/ilyam8))
- Enhance README with publishing instructions for docs [\#20952](https://github.com/netdata/netdata/pull/20952) ([Ancairon](https://github.com/Ancairon))
- move whole category and fix leftovers [\#20951](https://github.com/netdata/netdata/pull/20951) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/jackc/pgx/v5 from 5.7.5 to 5.7.6 in /src/go [\#20950](https://github.com/netdata/netdata/pull/20950) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/net from 0.43.0 to 0.44.0 in /src/go [\#20949](https://github.com/netdata/netdata/pull/20949) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix use after free in metric registry [\#20947](https://github.com/netdata/netdata/pull/20947) ([stelfrag](https://github.com/stelfrag))
- Update account.md [\#20946](https://github.com/netdata/netdata/pull/20946) ([kanelatechnical](https://github.com/kanelatechnical))
- Add map.csv to triggers for Docs ingest workflow [\#20945](https://github.com/netdata/netdata/pull/20945) ([Ancairon](https://github.com/Ancairon))
- BSD Compilation [\#20944](https://github.com/netdata/netdata/pull/20944) ([thiagoftsm](https://github.com/thiagoftsm))
- Migrate the map from Learn repo to netdata/netdata [\#20942](https://github.com/netdata/netdata/pull/20942) ([Ancairon](https://github.com/Ancairon))
- Replace uv mutex and condition variables with netdata equivalents [\#20941](https://github.com/netdata/netdata/pull/20941) ([stelfrag](https://github.com/stelfrag))
- Update account.md [\#20940](https://github.com/netdata/netdata/pull/20940) ([Ancairon](https://github.com/Ancairon))
- Add Network Labels to Windows. [\#20938](https://github.com/netdata/netdata/pull/20938) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump golang.org/x/text from 0.28.0 to 0.29.0 in /src/go [\#20937](https://github.com/netdata/netdata/pull/20937) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore\(go.d/pkgs/logs\): validation fixes, resource safety, and cleanup [\#20931](https://github.com/netdata/netdata/pull/20931) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/prometheus/common from 0.65.0 to 0.66.1 in /src/go [\#20930](https://github.com/netdata/netdata/pull/20930) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.3.3+incompatible to 28.4.0+incompatible in /src/go [\#20929](https://github.com/netdata/netdata/pull/20929) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.12.1 to 9.13.0 in /src/go [\#20928](https://github.com/netdata/netdata/pull/20928) ([dependabot[bot]](https://github.com/apps/dependabot))
- Add documentation for account deletion process [\#20927](https://github.com/netdata/netdata/pull/20927) ([kanelatechnical](https://github.com/kanelatechnical))
- Regenerate integrations docs [\#20925](https://github.com/netdata/netdata/pull/20925) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/nginx\): improve prerequisites for NGINX collector [\#20924](https://github.com/netdata/netdata/pull/20924) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/weblog\): remove path pattern validation in dyncfg [\#20923](https://github.com/netdata/netdata/pull/20923) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): remove unused resolve hostname functionality [\#20922](https://github.com/netdata/netdata/pull/20922) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20916](https://github.com/netdata/netdata/pull/20916) ([netdatabot](https://github.com/netdatabot))
- fix\(netdata-updater\): resolve "run: command not found" error in offline install [\#20915](https://github.com/netdata/netdata/pull/20915) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): add build-time configuration directory paths [\#20913](https://github.com/netdata/netdata/pull/20913) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): try rel path before checking well-known path for user and stock dirs [\#20912](https://github.com/netdata/netdata/pull/20912) ([ilyam8](https://github.com/ilyam8))
- SIGNL4 Alert Notification [\#20911](https://github.com/netdata/netdata/pull/20911) ([rons4](https://github.com/rons4))
- Update view-plan-and-billing.md [\#20910](https://github.com/netdata/netdata/pull/20910) ([kanelatechnical](https://github.com/kanelatechnical))
- Update link for opt-out section [\#20909](https://github.com/netdata/netdata/pull/20909) ([gatesry](https://github.com/gatesry))
- build\(deps\): bump k8s.io/client-go from 0.33.4 to 0.34.0 in /src/go [\#20908](https://github.com/netdata/netdata/pull/20908) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/stretchr/testify from 1.11.0 to 1.11.1 in /src/go [\#20906](https://github.com/netdata/netdata/pull/20906) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix processes function: Add PPID grouping and fix WOps typo [\#20902](https://github.com/netdata/netdata/pull/20902) ([ktsaou](https://github.com/ktsaou))
- Update demo link formatting in documentation [\#20899](https://github.com/netdata/netdata/pull/20899) ([andrewm4894](https://github.com/andrewm4894))
- improve\(go.d/snmp\): make snmp v3 auth and priv keys hidden in UI [\#20898](https://github.com/netdata/netdata/pull/20898) ([ilyam8](https://github.com/ilyam8))
- refactor\(go.d/snmp\): recreate client on SNMPv3 "packet is not authentic" errors [\#20897](https://github.com/netdata/netdata/pull/20897) ([ilyam8](https://github.com/ilyam8))
- improve\(cgroups\): skip KubeVirt helper containers in virt-launcher pods [\#20896](https://github.com/netdata/netdata/pull/20896) ([ilyam8](https://github.com/ilyam8))
- improve snmp ubiquiti unifi ap model [\#20895](https://github.com/netdata/netdata/pull/20895) ([ilyam8](https://github.com/ilyam8))
- Move exporting integrations to their own folder [\#20894](https://github.com/netdata/netdata/pull/20894) ([Ancairon](https://github.com/Ancairon))
- Fixing Supported Linux Platforms and Versions [\#20893](https://github.com/netdata/netdata/pull/20893) ([BenjaminFosters](https://github.com/BenjaminFosters))
- Change MSSQL Cleanup and Queries \(windows.plugin\) [\#20892](https://github.com/netdata/netdata/pull/20892) ([thiagoftsm](https://github.com/thiagoftsm))
- improved alerting docs [\#20891](https://github.com/netdata/netdata/pull/20891) ([ktsaou](https://github.com/ktsaou))
- Improve exporting documentation clarity and structure [\#20890](https://github.com/netdata/netdata/pull/20890) ([kanelatechnical](https://github.com/kanelatechnical))
- improve\(go.d/snmp\): update Allied Telesis meta [\#20889](https://github.com/netdata/netdata/pull/20889) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): update Fortinet meta [\#20888](https://github.com/netdata/netdata/pull/20888) ([ilyam8](https://github.com/ilyam8))
- Windows: round sleep to clock resolution to prevent sub-ms early-wake logs [\#20887](https://github.com/netdata/netdata/pull/20887) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/stretchr/testify from 1.10.0 to 1.11.0 in /src/go [\#20886](https://github.com/netdata/netdata/pull/20886) ([dependabot[bot]](https://github.com/apps/dependabot))
- improve\(go.d/snmp\): add mikrotik mtxrHlProcessorTemperature [\#20885](https://github.com/netdata/netdata/pull/20885) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/snmp\): handle invalid SFP temperature readings for empty slots [\#20884](https://github.com/netdata/netdata/pull/20884) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add MikroTik type and model detection [\#20883](https://github.com/netdata/netdata/pull/20883) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): update tplink snmp meta [\#20882](https://github.com/netdata/netdata/pull/20882) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): fix goroutine leak and panic risk in Docker exec [\#20881](https://github.com/netdata/netdata/pull/20881) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add zyxel snmp meta file [\#20879](https://github.com/netdata/netdata/pull/20879) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): use plugincofng for loading profiles [\#20878](https://github.com/netdata/netdata/pull/20878) ([ilyam8](https://github.com/ilyam8))
- Update libbpf to 1.6.2 [\#20875](https://github.com/netdata/netdata/pull/20875) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/coreos/go-systemd/v22 from 22.5.0 to 22.6.0 in /src/go [\#20874](https://github.com/netdata/netdata/pull/20874) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix\(go.d\): create vnode internal data\_collection\_status charts in the main context [\#20872](https://github.com/netdata/netdata/pull/20872) ([ilyam8](https://github.com/ilyam8))
- Fix Table field comparison in SNMP collector table tests [\#20871](https://github.com/netdata/netdata/pull/20871) ([Copilot](https://github.com/apps/copilot-swe-agent))
- Rename default port for the OpenTelemetry Collector [\#20868](https://github.com/netdata/netdata/pull/20868) ([ralphm](https://github.com/ralphm))
- improve\(go.d/snmp\): add more entries in juniper metadata file [\#20867](https://github.com/netdata/netdata/pull/20867) ([ilyam8](https://github.com/ilyam8))
- Update documentation [\#20865](https://github.com/netdata/netdata/pull/20865) ([thiagoftsm](https://github.com/thiagoftsm))
- Update documentation \(Windows.plugin\) [\#20864](https://github.com/netdata/netdata/pull/20864) ([thiagoftsm](https://github.com/thiagoftsm))
- chore\(go.d/snmp\): more vendor-scoped meta yaml files [\#20863](https://github.com/netdata/netdata/pull/20863) ([ilyam8](https://github.com/ilyam8))
- Properly integrate Rust code checks in CI. [\#20862](https://github.com/netdata/netdata/pull/20862) ([Ferroin](https://github.com/Ferroin))
- improve\(go.d/snmp\): update snmp meta copilot [\#20861](https://github.com/netdata/netdata/pull/20861) ([ilyam8](https://github.com/ilyam8))
- Handle virtual host disconnection [\#20860](https://github.com/netdata/netdata/pull/20860) ([stelfrag](https://github.com/stelfrag))
- Add cargo lock file. [\#20855](https://github.com/netdata/netdata/pull/20855) ([vkalintiris](https://github.com/vkalintiris))
- build\(deps\): bump github.com/vmware/govmomi from 0.51.0 to 0.52.0 in /src/go [\#20852](https://github.com/netdata/netdata/pull/20852) ([dependabot[bot]](https://github.com/apps/dependabot))
- Add build-time check to reject known bad compiler flags. [\#20851](https://github.com/netdata/netdata/pull/20851) ([Ferroin](https://github.com/Ferroin))
- ci: bump GoTestTools/gotestfmt-action version [\#20850](https://github.com/netdata/netdata/pull/20850) ([ilyam8](https://github.com/ilyam8))
- Revert "Revert "Switch to a Debian 13 base for our Docker images."" [\#20848](https://github.com/netdata/netdata/pull/20848) ([ilyam8](https://github.com/ilyam8))
- ci: fix docker-test.sh handling to fail on error [\#20847](https://github.com/netdata/netdata/pull/20847) ([ilyam8](https://github.com/ilyam8))
- Do not set virtual host flag on agent restart [\#20845](https://github.com/netdata/netdata/pull/20845) ([stelfrag](https://github.com/stelfrag))
- improve\(go.d/snmp\): update h3c categories in snmp meta [\#20844](https://github.com/netdata/netdata/pull/20844) ([ilyam8](https://github.com/ilyam8))
- Revert "Switch to a Debian 13 base for our Docker images." [\#20842](https://github.com/netdata/netdata/pull/20842) ([stelfrag](https://github.com/stelfrag))
- Store virtual host labels [\#20841](https://github.com/netdata/netdata/pull/20841) ([stelfrag](https://github.com/stelfrag))
- Windows Plugin \(Sensors\) [\#20840](https://github.com/netdata/netdata/pull/20840) ([thiagoftsm](https://github.com/thiagoftsm))
- improve\(go.d/snmp\): update netgear/dlink category in snmp meta [\#20839](https://github.com/netdata/netdata/pull/20839) ([ilyam8](https://github.com/ilyam8))
- Improve ACLK message parsing [\#20838](https://github.com/netdata/netdata/pull/20838) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp\): load & merge per-vendor SNMP metadata overrides [\#20837](https://github.com/netdata/netdata/pull/20837) ([ilyam8](https://github.com/ilyam8))
- Use atomics for mqtt statistics [\#20836](https://github.com/netdata/netdata/pull/20836) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp\): update category of h3c devices in meta\_overrides.yaml [\#20835](https://github.com/netdata/netdata/pull/20835) ([ilyam8](https://github.com/ilyam8))
- Mqtt adjust buffer size [\#20834](https://github.com/netdata/netdata/pull/20834) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp\): merge sysObjectIDs.json into meta\_overrides.yaml [\#20831](https://github.com/netdata/netdata/pull/20831) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add more models to meta\_overrides.yaml [\#20830](https://github.com/netdata/netdata/pull/20830) ([ilyam8](https://github.com/ilyam8))
- updated logging documentation and added natural siem integration [\#20829](https://github.com/netdata/netdata/pull/20829) ([ktsaou](https://github.com/ktsaou))
- feat\(go.d/snmp\): add YAML overrides for sysobjectids mapping [\#20828](https://github.com/netdata/netdata/pull/20828) ([ilyam8](https://github.com/ilyam8))
- refactor\(go.d\): move nd directories to dedicated pluginconfig package [\#20827](https://github.com/netdata/netdata/pull/20827) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump k8s.io/client-go from 0.33.3 to 0.33.4 in /src/go [\#20826](https://github.com/netdata/netdata/pull/20826) ([dependabot[bot]](https://github.com/apps/dependabot))
- Kickstarter Fix for DNF5 System [\#20823](https://github.com/netdata/netdata/pull/20823) ([BenjaminFosters](https://github.com/BenjaminFosters))
- Add ACLK buffer usage metrics [\#20820](https://github.com/netdata/netdata/pull/20820) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d/ddsnmp\): correct profile matching, metadata precedence, and OID handling [\#20819](https://github.com/netdata/netdata/pull/20819) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20818](https://github.com/netdata/netdata/pull/20818) ([netdatabot](https://github.com/netdatabot))
- Switch to a Debian 13 base for our Docker images. [\#20816](https://github.com/netdata/netdata/pull/20816) ([Ferroin](https://github.com/Ferroin))
- Fix Charts \(windows.plugin\) [\#20815](https://github.com/netdata/netdata/pull/20815) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(go.d/ddsnmp\): fix match\_pattern regex behavior in metadata and metric collection [\#20814](https://github.com/netdata/netdata/pull/20814) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): optimize Check\(\) to avoid heavy collection [\#20813](https://github.com/netdata/netdata/pull/20813) ([ilyam8](https://github.com/ilyam8))
- Render latency chart if ACLK online [\#20811](https://github.com/netdata/netdata/pull/20811) ([stelfrag](https://github.com/stelfrag))
- Monitor memory reclamation and buffer compact [\#20810](https://github.com/netdata/netdata/pull/20810) ([stelfrag](https://github.com/stelfrag))
- Clear packet id when processed from PUBACK [\#20808](https://github.com/netdata/netdata/pull/20808) ([stelfrag](https://github.com/stelfrag))
- improve\(go.d/zfspool\): add Ubuntu zpool binary path to config [\#20806](https://github.com/netdata/netdata/pull/20806) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/snmp\): add SNMP sysObjectID mappings for device identification [\#20805](https://github.com/netdata/netdata/pull/20805) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.12.0 to 9.12.1 in /src/go [\#20804](https://github.com/netdata/netdata/pull/20804) ([dependabot[bot]](https://github.com/apps/dependabot))
- feat\(go.d/ddsnmp\): sysobjectid-based metadata override support for SNMP profiles [\#20803](https://github.com/netdata/netdata/pull/20803) ([ilyam8](https://github.com/ilyam8))
- feat\(aclk\): Add detailed pulse metrics for ACLK telemetry [\#20802](https://github.com/netdata/netdata/pull/20802) ([ktsaou](https://github.com/ktsaou))
- fix pathvalidate non unix [\#20801](https://github.com/netdata/netdata/pull/20801) ([ilyam8](https://github.com/ilyam8))
- Fix ping latency calculation [\#20800](https://github.com/netdata/netdata/pull/20800) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d\): resolve potential toctou vulnerability in binary path validation [\#20798](https://github.com/netdata/netdata/pull/20798) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump golang.org/x/net from 0.42.0 to 0.43.0 in /src/go [\#20796](https://github.com/netdata/netdata/pull/20796) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.11.0 to 9.12.0 in /src/go [\#20794](https://github.com/netdata/netdata/pull/20794) ([dependabot[bot]](https://github.com/apps/dependabot))
- ci: handle boolean values in EOL API responses for newly released distros [\#20792](https://github.com/netdata/netdata/pull/20792) ([ilyam8](https://github.com/ilyam8))
- Update sqlite to version 3.50.4 [\#20791](https://github.com/netdata/netdata/pull/20791) ([stelfrag](https://github.com/stelfrag))
- Add more MSSQL metrics \(windows.plugin\) [\#20788](https://github.com/netdata/netdata/pull/20788) ([thiagoftsm](https://github.com/thiagoftsm))
- feat\(go.d/ddsnmp\): add MultiValue for state metrics and aggregation [\#20787](https://github.com/netdata/netdata/pull/20787) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/ddsnmp\): add metric aggregation support for SNMP profiles [\#20786](https://github.com/netdata/netdata/pull/20786) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): respect metric tag order from profile definition [\#20784](https://github.com/netdata/netdata/pull/20784) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20783](https://github.com/netdata/netdata/pull/20783) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/mysql\): add MariaDB 10.5.9+ SLAVE MONITOR privilege [\#20782](https://github.com/netdata/netdata/pull/20782) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20781](https://github.com/netdata/netdata/pull/20781) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/memcached\): add UNIX socket access prerequisite [\#20780](https://github.com/netdata/netdata/pull/20780) ([ilyam8](https://github.com/ilyam8))
- Virtual node version adjustment [\#20777](https://github.com/netdata/netdata/pull/20777) ([stelfrag](https://github.com/stelfrag))
- Add Debian 13 to CI and package builds. [\#20776](https://github.com/netdata/netdata/pull/20776) ([Ferroin](https://github.com/Ferroin))
- Aclk improvements [\#20775](https://github.com/netdata/netdata/pull/20775) ([stelfrag](https://github.com/stelfrag))
- Fix MSSQL Charts [\#20774](https://github.com/netdata/netdata/pull/20774) ([thiagoftsm](https://github.com/thiagoftsm))
- Update machine-learning-and-assisted-troubleshooting.md [\#20772](https://github.com/netdata/netdata/pull/20772) ([kanelatechnical](https://github.com/kanelatechnical))
- Regenerate integrations docs [\#20771](https://github.com/netdata/netdata/pull/20771) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/snmp\): add configurable device down threshold for vnodes [\#20770](https://github.com/netdata/netdata/pull/20770) ([ilyam8](https://github.com/ilyam8))
- Add publish latency to the aclk-state command [\#20769](https://github.com/netdata/netdata/pull/20769) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp\): add \_net\_default\_iface\_ip host label [\#20768](https://github.com/netdata/netdata/pull/20768) ([ilyam8](https://github.com/ilyam8))
- Add default iface info to host labels [\#20767](https://github.com/netdata/netdata/pull/20767) ([stelfrag](https://github.com/stelfrag))
- Fix packet timeout handling [\#20766](https://github.com/netdata/netdata/pull/20766) ([stelfrag](https://github.com/stelfrag))
- Add OpenTelemetry plugin implementation. [\#20765](https://github.com/netdata/netdata/pull/20765) ([vkalintiris](https://github.com/vkalintiris))
- feat\(system-info\): add default network interface IP detection [\#20764](https://github.com/netdata/netdata/pull/20764) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): remove patternProperties from config\_schema.json [\#20763](https://github.com/netdata/netdata/pull/20763) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/snmp-profile\): fix extends in cradlepoint.yaml [\#20762](https://github.com/netdata/netdata/pull/20762) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): validate custom binary path [\#20761](https://github.com/netdata/netdata/pull/20761) ([ilyam8](https://github.com/ilyam8))
- Update welcome-to-netdata.md [\#20760](https://github.com/netdata/netdata/pull/20760) ([kanelatechnical](https://github.com/kanelatechnical))
- Troubleshooting: add troubleshoot and custom investigations docs [\#20759](https://github.com/netdata/netdata/pull/20759) ([kanelatechnical](https://github.com/kanelatechnical))
- chore\(go.d/snmp\): update org to vendor mapping [\#20757](https://github.com/netdata/netdata/pull/20757) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): update hostname to not include IP [\#20756](https://github.com/netdata/netdata/pull/20756) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): add \_clean\_hostname host label [\#20755](https://github.com/netdata/netdata/pull/20755) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): add \_vnode\_type host label and rm duplicates [\#20754](https://github.com/netdata/netdata/pull/20754) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/miekg/dns from 1.1.67 to 1.1.68 in /src/go [\#20753](https://github.com/netdata/netdata/pull/20753) ([dependabot[bot]](https://github.com/apps/dependabot))
- Improve Disk Usage Measure \(Windows.plugin\) [\#20752](https://github.com/netdata/netdata/pull/20752) ([thiagoftsm](https://github.com/thiagoftsm))
- chore\(go.d/snmp\): remove SNMP prefix from hostname [\#20751](https://github.com/netdata/netdata/pull/20751) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): add org to vendor map [\#20750](https://github.com/netdata/netdata/pull/20750) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20749](https://github.com/netdata/netdata/pull/20749) ([netdatabot](https://github.com/netdatabot))
- Update deployment-with-centralization-points.md [\#20748](https://github.com/netdata/netdata/pull/20748) ([kanelatechnical](https://github.com/kanelatechnical))
- Record proxy information when establishing ACLK [\#20747](https://github.com/netdata/netdata/pull/20747) ([stelfrag](https://github.com/stelfrag))
- Change remaining pthread\_ cases [\#20746](https://github.com/netdata/netdata/pull/20746) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp-profiles\): use same metric name for cpu usage  [\#20745](https://github.com/netdata/netdata/pull/20745) ([ilyam8](https://github.com/ilyam8))
- Remove redundant defines [\#20744](https://github.com/netdata/netdata/pull/20744) ([stelfrag](https://github.com/stelfrag))
- streaming routing documentation [\#20743](https://github.com/netdata/netdata/pull/20743) ([ktsaou](https://github.com/ktsaou))
- docs: remove customize.md [\#20742](https://github.com/netdata/netdata/pull/20742) ([ilyam8](https://github.com/ilyam8))
- MCP WEB CHAT: add ollama, deepseek support [\#20741](https://github.com/netdata/netdata/pull/20741) ([ktsaou](https://github.com/ktsaou))
- Fix SNDR thread startup [\#20740](https://github.com/netdata/netdata/pull/20740) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/docker/docker from 28.3.2+incompatible to 28.3.3+incompatible in /src/go [\#20739](https://github.com/netdata/netdata/pull/20739) ([dependabot[bot]](https://github.com/apps/dependabot))
- Use netdata mutex cond and lock [\#20737](https://github.com/netdata/netdata/pull/20737) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump openssl and curl version in static build [\#20734](https://github.com/netdata/netdata/pull/20734) ([ilyam8](https://github.com/ilyam8))
- Thread creation code cleanup [\#20732](https://github.com/netdata/netdata/pull/20732) ([stelfrag](https://github.com/stelfrag))
- Add additional database checks during shutdown  [\#20731](https://github.com/netdata/netdata/pull/20731) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/bmatcuk/doublestar/v4 from 4.9.0 to 4.9.1 in /src/go [\#20730](https://github.com/netdata/netdata/pull/20730) ([dependabot[bot]](https://github.com/apps/dependabot))
- Revert "Revert "Deployment Guides: add and update documentation for deployment strate"" [\#20729](https://github.com/netdata/netdata/pull/20729) ([ilyam8](https://github.com/ilyam8))
- Revert "Deployment Guides: add and update documentation for deployment strate" [\#20728](https://github.com/netdata/netdata/pull/20728) ([ilyam8](https://github.com/ilyam8))
- Reset chart variable after release [\#20727](https://github.com/netdata/netdata/pull/20727) ([stelfrag](https://github.com/stelfrag))
- Improve thread shutdown handling for MSSQL plugin [\#20725](https://github.com/netdata/netdata/pull/20725) ([stelfrag](https://github.com/stelfrag))
- Update README.md [\#20724](https://github.com/netdata/netdata/pull/20724) ([kanelatechnical](https://github.com/kanelatechnical))
- Update README.md [\#20723](https://github.com/netdata/netdata/pull/20723) ([kanelatechnical](https://github.com/kanelatechnical))
- Avoid static initialization [\#20722](https://github.com/netdata/netdata/pull/20722) ([stelfrag](https://github.com/stelfrag))
- docs: add Network-connections to the functions table [\#20721](https://github.com/netdata/netdata/pull/20721) ([ilyam8](https://github.com/ilyam8))
- Enable services status \(windows.plugin\) [\#20720](https://github.com/netdata/netdata/pull/20720) ([thiagoftsm](https://github.com/thiagoftsm))
- Mark the completion from the worker thread [\#20719](https://github.com/netdata/netdata/pull/20719) ([stelfrag](https://github.com/stelfrag))
- improve\(go.d/snmp\): add profile device meta to vnode labels [\#20718](https://github.com/netdata/netdata/pull/20718) ([ilyam8](https://github.com/ilyam8))
- Ignore timestamps recording in gzip metadata \(for reproducible builds\) [\#20714](https://github.com/netdata/netdata/pull/20714) ([Antiz96](https://github.com/Antiz96))
- Remove H2O web server code from Netdata. [\#20713](https://github.com/netdata/netdata/pull/20713) ([Ferroin](https://github.com/Ferroin))
- Deployment Guides: add and update documentation for deployment strate… [\#20712](https://github.com/netdata/netdata/pull/20712) ([kanelatechnical](https://github.com/kanelatechnical))
- Detect missing ACLK MQTT packet acknowledgents [\#20711](https://github.com/netdata/netdata/pull/20711) ([stelfrag](https://github.com/stelfrag))
- SNMP: make units ucum [\#20710](https://github.com/netdata/netdata/pull/20710) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/gosnmp/gosnmp from 1.42.0 to 1.42.1 in /src/go [\#20709](https://github.com/netdata/netdata/pull/20709) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/gosnmp/gosnmp from 1.41.0 to 1.42.0 in /src/go [\#20707](https://github.com/netdata/netdata/pull/20707) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump k8s.io/client-go from 0.33.2 to 0.33.3 in /src/go [\#20705](https://github.com/netdata/netdata/pull/20705) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix\(nvme\)!: query controller instead of namespace for SMART metrics [\#20704](https://github.com/netdata/netdata/pull/20704) ([ilyam8](https://github.com/ilyam8))
- Update UPDATE.md [\#20701](https://github.com/netdata/netdata/pull/20701) ([kanelatechnical](https://github.com/kanelatechnical))
- Ensure chat exists before handling message input [\#20700](https://github.com/netdata/netdata/pull/20700) ([emmanuel-ferdman](https://github.com/emmanuel-ferdman))
- Escape \< character in plaintext [\#20699](https://github.com/netdata/netdata/pull/20699) ([Ancairon](https://github.com/Ancairon))
- Replace legacy functions-table.md with comprehensive UI documentation [\#20697](https://github.com/netdata/netdata/pull/20697) ([ktsaou](https://github.com/ktsaou))
- Update LIBBPF [\#20696](https://github.com/netdata/netdata/pull/20696) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix SSL certificate detection for Rocky Linux and static curl [\#20695](https://github.com/netdata/netdata/pull/20695) ([ktsaou](https://github.com/ktsaou))
- update otel-collector components deps [\#20693](https://github.com/netdata/netdata/pull/20693) ([ilyam8](https://github.com/ilyam8))
- Add Oracle Linux 10 to CI and package builds. [\#20684](https://github.com/netdata/netdata/pull/20684) ([Ferroin](https://github.com/Ferroin))
- Split collection \(Windows.plugin\) [\#20677](https://github.com/netdata/netdata/pull/20677) ([thiagoftsm](https://github.com/thiagoftsm))
- Add Getting Started Netdata guide [\#20642](https://github.com/netdata/netdata/pull/20642) ([kanelatechnical](https://github.com/kanelatechnical))
- ml: implement fixed time-based training windows [\#20638](https://github.com/netdata/netdata/pull/20638) ([ktsaou](https://github.com/ktsaou))

## [v2.6.3](https://github.com/netdata/netdata/tree/v2.6.3) (2025-08-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.6.2...v2.6.3)

## [v2.6.2](https://github.com/netdata/netdata/tree/v2.6.2) (2025-08-13)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.6.1...v2.6.2)

## [v2.6.1](https://github.com/netdata/netdata/tree/v2.6.1) (2025-07-25)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.6.0...v2.6.1)

## [v2.6.0](https://github.com/netdata/netdata/tree/v2.6.0) (2025-07-17)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.5.4...v2.6.0)

**Merged pull requests:**

- docs: remove Profiles heading from collapsible section [\#20691](https://github.com/netdata/netdata/pull/20691) ([ilyam8](https://github.com/ilyam8))
- docs: fix file location in continue setup [\#20690](https://github.com/netdata/netdata/pull/20690) ([ilyam8](https://github.com/ilyam8))
- docs: update continue ext setup [\#20689](https://github.com/netdata/netdata/pull/20689) ([ilyam8](https://github.com/ilyam8))
- Fix log message format for buffered reader error [\#20687](https://github.com/netdata/netdata/pull/20687) ([stelfrag](https://github.com/stelfrag))
- Fix systemd-journal-plugin RPM package. [\#20686](https://github.com/netdata/netdata/pull/20686) ([Ferroin](https://github.com/Ferroin))
- Remove Fedora 40 from CI and package builds. [\#20685](https://github.com/netdata/netdata/pull/20685) ([Ferroin](https://github.com/Ferroin))
- Remove Ubuntu 24.10 from CI and package builds. [\#20681](https://github.com/netdata/netdata/pull/20681) ([Ferroin](https://github.com/Ferroin))
- chore\(charts.d\): suppress broken pipe error from echo during cleanup [\#20680](https://github.com/netdata/netdata/pull/20680) ([ilyam8](https://github.com/ilyam8))
- Fix deadlock in dictionary cleanup [\#20679](https://github.com/netdata/netdata/pull/20679) ([stelfrag](https://github.com/stelfrag))
- Agent docs alignement [\#20676](https://github.com/netdata/netdata/pull/20676) ([kanelatechnical](https://github.com/kanelatechnical))
- Regenerate integrations docs [\#20675](https://github.com/netdata/netdata/pull/20675) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/snmp\): enable table metrics by default [\#20674](https://github.com/netdata/netdata/pull/20674) ([ilyam8](https://github.com/ilyam8))
- Code cleanup [\#20673](https://github.com/netdata/netdata/pull/20673) ([stelfrag](https://github.com/stelfrag))
- Improve agent shutdown on windows [\#20672](https://github.com/netdata/netdata/pull/20672) ([stelfrag](https://github.com/stelfrag))
- Escape chars on documentation [\#20671](https://github.com/netdata/netdata/pull/20671) ([Ancairon](https://github.com/Ancairon))
- Add comprehensive welcome document [\#20669](https://github.com/netdata/netdata/pull/20669) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#20668](https://github.com/netdata/netdata/pull/20668) ([netdatabot](https://github.com/netdatabot))
- build\(deps\): bump golang.org/x/net from 0.41.0 to 0.42.0 in /src/go [\#20667](https://github.com/netdata/netdata/pull/20667) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/bmatcuk/doublestar/v4 from 4.8.1 to 4.9.0 in /src/go [\#20666](https://github.com/netdata/netdata/pull/20666) ([dependabot[bot]](https://github.com/apps/dependabot))
- docs: fix "Unsupported markdown: list" in NC readme diagram [\#20665](https://github.com/netdata/netdata/pull/20665) ([ilyam8](https://github.com/ilyam8))
- Add ML anomaly detection accuracy analysis documentation [\#20663](https://github.com/netdata/netdata/pull/20663) ([ktsaou](https://github.com/ktsaou))
- Fix datafile creation race condition [\#20662](https://github.com/netdata/netdata/pull/20662) ([stelfrag](https://github.com/stelfrag))
- Cloud Docs: updated [\#20661](https://github.com/netdata/netdata/pull/20661) ([kanelatechnical](https://github.com/kanelatechnical))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p8 [\#20660](https://github.com/netdata/netdata/pull/20660) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20659](https://github.com/netdata/netdata/pull/20659) ([netdatabot](https://github.com/netdatabot))
- Improve job completion handling with timeout mechanism [\#20657](https://github.com/netdata/netdata/pull/20657) ([stelfrag](https://github.com/stelfrag))
- Fix coverity issues [\#20656](https://github.com/netdata/netdata/pull/20656) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#20655](https://github.com/netdata/netdata/pull/20655) ([netdatabot](https://github.com/netdatabot))
- Stop submitting analytics [\#20654](https://github.com/netdata/netdata/pull/20654) ([stelfrag](https://github.com/stelfrag))
- Fix documentation regarding header\_match [\#20652](https://github.com/netdata/netdata/pull/20652) ([tobias-richter](https://github.com/tobias-richter))
- build\(deps\): bump golang.org/x/text from 0.26.0 to 0.27.0 in /src/go [\#20651](https://github.com/netdata/netdata/pull/20651) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.3.1+incompatible to 28.3.2+incompatible in /src/go [\#20650](https://github.com/netdata/netdata/pull/20650) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/miekg/dns from 1.1.66 to 1.1.67 in /src/go [\#20649](https://github.com/netdata/netdata/pull/20649) ([dependabot[bot]](https://github.com/apps/dependabot))
- SNMP profile edits ep3 [\#20648](https://github.com/netdata/netdata/pull/20648) ([Ancairon](https://github.com/Ancairon))
- SNMP profiles pass ep2 [\#20647](https://github.com/netdata/netdata/pull/20647) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p7 [\#20646](https://github.com/netdata/netdata/pull/20646) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): fix quotes [\#20645](https://github.com/netdata/netdata/pull/20645) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20644](https://github.com/netdata/netdata/pull/20644) ([netdatabot](https://github.com/netdatabot))
- Update Cloud OIDC Authorization Server setup docs [\#20643](https://github.com/netdata/netdata/pull/20643) ([car12o](https://github.com/car12o))
- SNMP Profiles pass ep1 [\#20641](https://github.com/netdata/netdata/pull/20641) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p6 [\#20640](https://github.com/netdata/netdata/pull/20640) ([ilyam8](https://github.com/ilyam8))
- Additional checks for ACLK proxy setting [\#20639](https://github.com/netdata/netdata/pull/20639) ([stelfrag](https://github.com/stelfrag))
- MCP in Netdata Operations Diagram [\#20637](https://github.com/netdata/netdata/pull/20637) ([ktsaou](https://github.com/ktsaou))
- refactor\(go.d/iprange\): migrate from net to net/netip [\#20636](https://github.com/netdata/netdata/pull/20636) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p5 [\#20635](https://github.com/netdata/netdata/pull/20635) ([ilyam8](https://github.com/ilyam8))
- Update NIDL-Framework.md [\#20634](https://github.com/netdata/netdata/pull/20634) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/docker/docker from 28.3.0+incompatible to 28.3.1+incompatible in /src/go [\#20633](https://github.com/netdata/netdata/pull/20633) ([dependabot[bot]](https://github.com/apps/dependabot))
- move NIDL to docs [\#20632](https://github.com/netdata/netdata/pull/20632) ([ktsaou](https://github.com/ktsaou))
- Move NIDL-Framework.md from repository root to docs/ directory [\#20630](https://github.com/netdata/netdata/pull/20630) ([Copilot](https://github.com/apps/copilot-swe-agent))
- Nidl Framework Documentation [\#20629](https://github.com/netdata/netdata/pull/20629) ([ktsaou](https://github.com/ktsaou))
- Fix syntax error on learn doc [\#20628](https://github.com/netdata/netdata/pull/20628) ([Ancairon](https://github.com/Ancairon))
- At a glance [\#20627](https://github.com/netdata/netdata/pull/20627) ([kanelatechnical](https://github.com/kanelatechnical))
- Improve ACLK connection handling [\#20625](https://github.com/netdata/netdata/pull/20625) ([stelfrag](https://github.com/stelfrag))
- Improve packet ID generation [\#20624](https://github.com/netdata/netdata/pull/20624) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p4 [\#20623](https://github.com/netdata/netdata/pull/20623) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p3 [\#20622](https://github.com/netdata/netdata/pull/20622) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p2 [\#20621](https://github.com/netdata/netdata/pull/20621) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): charts meta fixes and fam updates p1 [\#20620](https://github.com/netdata/netdata/pull/20620) ([ilyam8](https://github.com/ilyam8))
- Improve journal v2 file creation on startup  [\#20619](https://github.com/netdata/netdata/pull/20619) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp-profiles\): small cleanup [\#20618](https://github.com/netdata/netdata/pull/20618) ([ilyam8](https://github.com/ilyam8))
- bump otel-collector components to v0.129.0 [\#20615](https://github.com/netdata/netdata/pull/20615) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): move fam desc and unit under chart\_meta [\#20614](https://github.com/netdata/netdata/pull/20614) ([ilyam8](https://github.com/ilyam8))
- update tripplite snmp profiles [\#20613](https://github.com/netdata/netdata/pull/20613) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#20612](https://github.com/netdata/netdata/pull/20612) ([stelfrag](https://github.com/stelfrag))
- SNMP Mikrotik profile make units in transform ucum [\#20611](https://github.com/netdata/netdata/pull/20611) ([Ancairon](https://github.com/Ancairon))

## [v2.5.4](https://github.com/netdata/netdata/tree/v2.5.4) (2025-06-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.5.3...v2.5.4)

## [v2.5.3](https://github.com/netdata/netdata/tree/v2.5.3) (2025-06-05)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.5.2...v2.5.3)

## [v2.5.2](https://github.com/netdata/netdata/tree/v2.5.2) (2025-05-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.5.1...v2.5.2)

## [v2.5.1](https://github.com/netdata/netdata/tree/v2.5.1) (2025-05-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.5.0...v2.5.1)

## [v2.5.0](https://github.com/netdata/netdata/tree/v2.5.0) (2025-05-05)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.4.0...v2.5.0)

## [v2.4.0](https://github.com/netdata/netdata/tree/v2.4.0) (2025-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.2...v2.4.0)

## [v2.3.2](https://github.com/netdata/netdata/tree/v2.3.2) (2025-04-02)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.1...v2.3.2)

## [v2.3.1](https://github.com/netdata/netdata/tree/v2.3.1) (2025-03-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.0...v2.3.1)

## [v2.3.0](https://github.com/netdata/netdata/tree/v2.3.0) (2025-03-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.6...v2.3.0)

## [v2.2.6](https://github.com/netdata/netdata/tree/v2.2.6) (2025-02-20)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.5...v2.2.6)

## [v2.2.5](https://github.com/netdata/netdata/tree/v2.2.5) (2025-02-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.4...v2.2.5)

## [v2.2.4](https://github.com/netdata/netdata/tree/v2.2.4) (2025-02-04)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.3...v2.2.4)

## [v2.2.3](https://github.com/netdata/netdata/tree/v2.2.3) (2025-01-31)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.2...v2.2.3)

## [v2.2.2](https://github.com/netdata/netdata/tree/v2.2.2) (2025-01-30)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.1...v2.2.2)

## [v2.2.1](https://github.com/netdata/netdata/tree/v2.2.1) (2025-01-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.0...v2.2.1)

## [v2.2.0](https://github.com/netdata/netdata/tree/v2.2.0) (2025-01-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.1.1...v2.2.0)

## [v2.1.1](https://github.com/netdata/netdata/tree/v2.1.1) (2025-01-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.1.0...v2.1.1)

## [v2.1.0](https://github.com/netdata/netdata/tree/v2.1.0) (2024-12-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.3...v2.1.0)

## [v2.0.3](https://github.com/netdata/netdata/tree/v2.0.3) (2024-11-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.2...v2.0.3)

## [v2.0.2](https://github.com/netdata/netdata/tree/v2.0.2) (2024-11-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.1...v2.0.2)

## [v2.0.1](https://github.com/netdata/netdata/tree/v2.0.1) (2024-11-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.0...v2.0.1)

## [v2.0.0](https://github.com/netdata/netdata/tree/v2.0.0) (2024-11-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.5...v2.0.0)

## [v1.47.5](https://github.com/netdata/netdata/tree/v1.47.5) (2024-10-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.4...v1.47.5)

## [v1.47.4](https://github.com/netdata/netdata/tree/v1.47.4) (2024-10-09)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.3...v1.47.4)

## [v1.47.3](https://github.com/netdata/netdata/tree/v1.47.3) (2024-10-02)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.2...v1.47.3)

## [v1.47.2](https://github.com/netdata/netdata/tree/v1.47.2) (2024-09-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.1...v1.47.2)

## [v1.47.1](https://github.com/netdata/netdata/tree/v1.47.1) (2024-09-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.99.0...v1.47.1)

## [v1.99.0](https://github.com/netdata/netdata/tree/v1.99.0) (2024-08-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.0...v1.99.0)

## [v1.47.0](https://github.com/netdata/netdata/tree/v1.47.0) (2024-08-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.3...v1.47.0)

## [v1.46.3](https://github.com/netdata/netdata/tree/v1.46.3) (2024-07-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.2...v1.46.3)

## [v1.46.2](https://github.com/netdata/netdata/tree/v1.46.2) (2024-07-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.1...v1.46.2)

## [v1.46.1](https://github.com/netdata/netdata/tree/v1.46.1) (2024-06-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.0...v1.46.1)

## [v1.46.0](https://github.com/netdata/netdata/tree/v1.46.0) (2024-06-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.6...v1.46.0)

## [v1.45.6](https://github.com/netdata/netdata/tree/v1.45.6) (2024-06-05)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.5...v1.45.6)

## [v1.45.5](https://github.com/netdata/netdata/tree/v1.45.5) (2024-05-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.4...v1.45.5)

## [v1.45.4](https://github.com/netdata/netdata/tree/v1.45.4) (2024-05-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.3...v1.45.4)

## [v1.45.3](https://github.com/netdata/netdata/tree/v1.45.3) (2024-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.2...v1.45.3)

## [v1.45.2](https://github.com/netdata/netdata/tree/v1.45.2) (2024-04-01)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.1...v1.45.2)

## [v1.45.1](https://github.com/netdata/netdata/tree/v1.45.1) (2024-03-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.0...v1.45.1)

## [v1.45.0](https://github.com/netdata/netdata/tree/v1.45.0) (2024-03-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.3...v1.45.0)

## [v1.44.3](https://github.com/netdata/netdata/tree/v1.44.3) (2024-02-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.2...v1.44.3)

## [v1.44.2](https://github.com/netdata/netdata/tree/v1.44.2) (2024-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.1...v1.44.2)

## [v1.44.1](https://github.com/netdata/netdata/tree/v1.44.1) (2023-12-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.0...v1.44.1)

## [v1.44.0](https://github.com/netdata/netdata/tree/v1.44.0) (2023-12-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.2...v1.44.0)

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

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.0...v1.34.1)

## [v1.34.0](https://github.com/netdata/netdata/tree/v1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/1.34.0...v1.34.0)

## [1.34.0](https://github.com/netdata/netdata/tree/1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.1...1.34.0)

## [v1.33.1](https://github.com/netdata/netdata/tree/v1.33.1) (2022-02-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.0...v1.33.1)

## [v1.33.0](https://github.com/netdata/netdata/tree/v1.33.0) (2022-01-26)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.32.1...v1.33.0)

## [v1.32.1](https://github.com/netdata/netdata/tree/v1.32.1) (2021-12-14)

[Full Changelog](https://github.com/netdata/netdata/compare/1.32.1...v1.32.1)

## [1.32.1](https://github.com/netdata/netdata/tree/1.32.1) (2021-12-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.32.0...1.32.1)

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
