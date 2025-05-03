# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.4.0...HEAD)

**Merged pull requests:**

- Fix potential null pointer dereference when accessing journalfile [\#20226](https://github.com/netdata/netdata/pull/20226) ([stelfrag](https://github.com/stelfrag))
- fix crashes 8 [\#20225](https://github.com/netdata/netdata/pull/20225) ([ktsaou](https://github.com/ktsaou))
- fix crashes 7 [\#20224](https://github.com/netdata/netdata/pull/20224) ([ktsaou](https://github.com/ktsaou))
- Enable analytics data collection  [\#20221](https://github.com/netdata/netdata/pull/20221) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.7.3 to 9.8.0 in /src/go [\#20220](https://github.com/netdata/netdata/pull/20220) ([dependabot[bot]](https://github.com/apps/dependabot))
- Small fixes2 [\#20219](https://github.com/netdata/netdata/pull/20219) ([ktsaou](https://github.com/ktsaou))
- documentation and helpers for centralizing namespaced logs [\#20217](https://github.com/netdata/netdata/pull/20217) ([ktsaou](https://github.com/ktsaou))
- Improve health log cleanup [\#20213](https://github.com/netdata/netdata/pull/20213) ([stelfrag](https://github.com/stelfrag))
- use nd threads in exporting [\#20212](https://github.com/netdata/netdata/pull/20212) ([ktsaou](https://github.com/ktsaou))
- Clean up prepared statements on thread exit [\#20211](https://github.com/netdata/netdata/pull/20211) ([stelfrag](https://github.com/stelfrag))
- fix hardcoding of eval variables [\#20210](https://github.com/netdata/netdata/pull/20210) ([ktsaou](https://github.com/ktsaou))
- Small fixes [\#20209](https://github.com/netdata/netdata/pull/20209) ([ktsaou](https://github.com/ktsaou))
- Security and Privacy Design [\#20208](https://github.com/netdata/netdata/pull/20208) ([kanelatechnical](https://github.com/kanelatechnical))
- added more annotations in spinlock deadlock detection [\#20207](https://github.com/netdata/netdata/pull/20207) ([ktsaou](https://github.com/ktsaou))
- call spinlock\_init\(\) when initializing rrdlabels spinlock [\#20206](https://github.com/netdata/netdata/pull/20206) ([ktsaou](https://github.com/ktsaou))
- remove the status file spinlock to avoid deadlocks [\#20205](https://github.com/netdata/netdata/pull/20205) ([ktsaou](https://github.com/ktsaou))
- Avoid indexing journal files when db rotation is running [\#20204](https://github.com/netdata/netdata/pull/20204) ([stelfrag](https://github.com/stelfrag))
- rrd metadata search fix [\#20203](https://github.com/netdata/netdata/pull/20203) ([ktsaou](https://github.com/ktsaou))
- Use one spinlock to access v2 and mmap related data [\#20202](https://github.com/netdata/netdata/pull/20202) ([stelfrag](https://github.com/stelfrag))
- Add a default busy timeout [\#20201](https://github.com/netdata/netdata/pull/20201) ([stelfrag](https://github.com/stelfrag))
- rrd metadata needs to be discoverable while replication is running [\#20200](https://github.com/netdata/netdata/pull/20200) ([ktsaou](https://github.com/ktsaou))
- chore\(otel/netdataexporter\): poc version [\#20199](https://github.com/netdata/netdata/pull/20199) ([ilyam8](https://github.com/ilyam8))
- add fast path to waitq [\#20198](https://github.com/netdata/netdata/pull/20198) ([ktsaou](https://github.com/ktsaou))
- spinlocks now timeout at 10 minutes, to reveal deadlocks [\#20197](https://github.com/netdata/netdata/pull/20197) ([ktsaou](https://github.com/ktsaou))
- rrdset/rrddim find function do not return obsolete metadata [\#20196](https://github.com/netdata/netdata/pull/20196) ([ktsaou](https://github.com/ktsaou))
- cleanup ML cached pointers on child disconnection [\#20195](https://github.com/netdata/netdata/pull/20195) ([ktsaou](https://github.com/ktsaou))
- limit the max number of threads based on memory too [\#20192](https://github.com/netdata/netdata/pull/20192) ([ktsaou](https://github.com/ktsaou))
- Exporting exit fix [\#20191](https://github.com/netdata/netdata/pull/20191) ([ktsaou](https://github.com/ktsaou))
- ignore maintenance signals on exit [\#20190](https://github.com/netdata/netdata/pull/20190) ([ktsaou](https://github.com/ktsaou))
- ensure atomicity when logging pending message 3/3 [\#20189](https://github.com/netdata/netdata/pull/20189) ([ktsaou](https://github.com/ktsaou))
- ensure atomicity when logging pending message 2/3 [\#20188](https://github.com/netdata/netdata/pull/20188) ([ktsaou](https://github.com/ktsaou))
- added dyncfg docs [\#20187](https://github.com/netdata/netdata/pull/20187) ([ktsaou](https://github.com/ktsaou))
- Fix repeating alert crash [\#20186](https://github.com/netdata/netdata/pull/20186) ([stelfrag](https://github.com/stelfrag))
- ensure atomicity when logging pending message 1/3 [\#20185](https://github.com/netdata/netdata/pull/20185) ([ktsaou](https://github.com/ktsaou))
- Improve systemd journal logs documentation [\#20184](https://github.com/netdata/netdata/pull/20184) ([kanelatechnical](https://github.com/kanelatechnical))
- Reorganize code \(IIS\) [\#20182](https://github.com/netdata/netdata/pull/20182) ([thiagoftsm](https://github.com/thiagoftsm))
- improve pgc fatal errors [\#20181](https://github.com/netdata/netdata/pull/20181) ([ktsaou](https://github.com/ktsaou))
- Group anomaly rate per chart context instead of type. [\#20180](https://github.com/netdata/netdata/pull/20180) ([vkalintiris](https://github.com/vkalintiris))
- bump go mod 1.24.0 [\#20179](https://github.com/netdata/netdata/pull/20179) ([ilyam8](https://github.com/ilyam8))
- Retry nightly changelog generation. [\#20178](https://github.com/netdata/netdata/pull/20178) ([Ferroin](https://github.com/Ferroin))
- Add Fedora 42 to CI and package builds. [\#20177](https://github.com/netdata/netdata/pull/20177) ([Ferroin](https://github.com/Ferroin))
- build\(deps\): update go toolchain to v1.24.2 [\#20176](https://github.com/netdata/netdata/pull/20176) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump k8s.io/client-go from 0.32.3 to 0.33.0 in /src/go [\#20175](https://github.com/netdata/netdata/pull/20175) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.49.0 to 0.50.0 in /src/go [\#20173](https://github.com/netdata/netdata/pull/20173) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore\(otel/netdataexporter\): add exporter module skeleton [\#20171](https://github.com/netdata/netdata/pull/20171) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20170](https://github.com/netdata/netdata/pull/20170) ([netdatabot](https://github.com/netdatabot))
- fix integrations config file sample [\#20169](https://github.com/netdata/netdata/pull/20169) ([ilyam8](https://github.com/ilyam8))
- improvement\(cgroups\): improve systemd-nspawn filter for default [\#20168](https://github.com/netdata/netdata/pull/20168) ([rhoriguchi](https://github.com/rhoriguchi))
- Update kickstart.md [\#20167](https://github.com/netdata/netdata/pull/20167) ([kanelatechnical](https://github.com/kanelatechnical))
- chore\(go.d\): remove wmi-\>win collector rename handling [\#20166](https://github.com/netdata/netdata/pull/20166) ([ilyam8](https://github.com/ilyam8))
- docs: update macOS/freeBSD versions in  Versions & Platforms [\#20165](https://github.com/netdata/netdata/pull/20165) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): remove unused ddsnmp [\#20164](https://github.com/netdata/netdata/pull/20164) ([ilyam8](https://github.com/ilyam8))
- SNMP profiles units and description generation [\#20163](https://github.com/netdata/netdata/pull/20163) ([Ancairon](https://github.com/Ancairon))
- Dashboards and charts [\#20162](https://github.com/netdata/netdata/pull/20162) ([kanelatechnical](https://github.com/kanelatechnical))
- fix\(dyncfg/health\): correct db lookup absolute option name [\#20161](https://github.com/netdata/netdata/pull/20161) ([ilyam8](https://github.com/ilyam8))
- Fix memory leaks and service thread corruption [\#20159](https://github.com/netdata/netdata/pull/20159) ([ktsaou](https://github.com/ktsaou))
- Fix labels memory accounting [\#20158](https://github.com/netdata/netdata/pull/20158) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/apcupsd\): log UPS response in debug mode [\#20157](https://github.com/netdata/netdata/pull/20157) ([ilyam8](https://github.com/ilyam8))
- improvement\(cgroups\): filter systemd-nspawn payload by default [\#20155](https://github.com/netdata/netdata/pull/20155) ([ilyam8](https://github.com/ilyam8))
- Fix compilation with DBENGINE disabled [\#20154](https://github.com/netdata/netdata/pull/20154) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/docker/docker from 28.0.4+incompatible to 28.1.1+incompatible in /src/go [\#20153](https://github.com/netdata/netdata/pull/20153) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/invopop/jsonschema from 0.12.0 to 0.13.0 in /src/go [\#20152](https://github.com/netdata/netdata/pull/20152) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/go-ldap/ldap/v3 from 3.4.10 to 3.4.11 in /src/go [\#20151](https://github.com/netdata/netdata/pull/20151) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/gosnmp/gosnmp from 1.39.0 to 1.40.0 in /src/go [\#20149](https://github.com/netdata/netdata/pull/20149) ([dependabot[bot]](https://github.com/apps/dependabot))
- Some fixes for macOS \< 11 [\#20145](https://github.com/netdata/netdata/pull/20145) ([barracuda156](https://github.com/barracuda156))
- docs: cleanup language and fix minor grammar issues [\#20144](https://github.com/netdata/netdata/pull/20144) ([luiizaferreirafonseca](https://github.com/luiizaferreirafonseca))
- chore\(otel/journaldexporter\): improve remote tests [\#20143](https://github.com/netdata/netdata/pull/20143) ([ilyam8](https://github.com/ilyam8))
- Improve MSSQL \(Windows.plugin Part II\) [\#20141](https://github.com/netdata/netdata/pull/20141) ([thiagoftsm](https://github.com/thiagoftsm))
- Install fix admonition docs [\#20136](https://github.com/netdata/netdata/pull/20136) ([kanelatechnical](https://github.com/kanelatechnical))
- Update MSI to use a single unified EULA instead of multiple license pages. [\#20134](https://github.com/netdata/netdata/pull/20134) ([Ferroin](https://github.com/Ferroin))
- Update README.md [\#20133](https://github.com/netdata/netdata/pull/20133) ([kanelatechnical](https://github.com/kanelatechnical))
- Improve metadata event loop shutdown [\#20132](https://github.com/netdata/netdata/pull/20132) ([stelfrag](https://github.com/stelfrag))
- Fix Locks \(Windows Locks\) [\#20131](https://github.com/netdata/netdata/pull/20131) ([thiagoftsm](https://github.com/thiagoftsm))
- Make sure pattern array items are added and evaluated in order [\#20130](https://github.com/netdata/netdata/pull/20130) ([stelfrag](https://github.com/stelfrag))
- Fix compilation when using FSANITIZE\_ADDRESS [\#20129](https://github.com/netdata/netdata/pull/20129) ([stelfrag](https://github.com/stelfrag))
- Handle corrupted journal data when populating the MRG during startup. [\#20128](https://github.com/netdata/netdata/pull/20128) ([stelfrag](https://github.com/stelfrag))
- Expression evaluator in re2c/lemon [\#20126](https://github.com/netdata/netdata/pull/20126) ([ktsaou](https://github.com/ktsaou))
- Free ACLK message [\#20125](https://github.com/netdata/netdata/pull/20125) ([stelfrag](https://github.com/stelfrag))
- Create Empty Directories \(Windows installer\) [\#20124](https://github.com/netdata/netdata/pull/20124) ([thiagoftsm](https://github.com/thiagoftsm))
- Installation-Static Build-Windows [\#20122](https://github.com/netdata/netdata/pull/20122) ([kanelatechnical](https://github.com/kanelatechnical))
- fix cleanup and exit and memory leaks [\#20120](https://github.com/netdata/netdata/pull/20120) ([ktsaou](https://github.com/ktsaou))
- Update platforms for CI and package builds. [\#20119](https://github.com/netdata/netdata/pull/20119) ([Ferroin](https://github.com/Ferroin))
- Improve error handling and logging for journal and data files [\#20112](https://github.com/netdata/netdata/pull/20112) ([stelfrag](https://github.com/stelfrag))
- Work to find leaks easily [\#20106](https://github.com/netdata/netdata/pull/20106) ([ktsaou](https://github.com/ktsaou))
- SNMP, Custom descriptions and units [\#20100](https://github.com/netdata/netdata/pull/20100) ([Ancairon](https://github.com/Ancairon))
- Fix MSSQL and improvements [\#20032](https://github.com/netdata/netdata/pull/20032) ([thiagoftsm](https://github.com/thiagoftsm))
- Sqlite upgrade to version 3.49.1 [\#19933](https://github.com/netdata/netdata/pull/19933) ([stelfrag](https://github.com/stelfrag))

## [v2.4.0](https://github.com/netdata/netdata/tree/v2.4.0) (2025-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.2...v2.4.0)

**Merged pull requests:**

- chore\(otel/journaldexporter\): add socket/remote clients [\#20121](https://github.com/netdata/netdata/pull/20121) ([ilyam8](https://github.com/ilyam8))
- feat\(system-info\): improve Windows OS detection and categorization [\#20117](https://github.com/netdata/netdata/pull/20117) ([ktsaou](https://github.com/ktsaou))
- fix memory leaks [\#20116](https://github.com/netdata/netdata/pull/20116) ([ktsaou](https://github.com/ktsaou))
- netdatacli remove/mark stale, swap order in help output [\#20113](https://github.com/netdata/netdata/pull/20113) ([ilyam8](https://github.com/ilyam8))
- Fix completion marking in ACLK cancel node update timer logic [\#20111](https://github.com/netdata/netdata/pull/20111) ([stelfrag](https://github.com/stelfrag))
- docs: clarify static build transition process for EOL platforms [\#20110](https://github.com/netdata/netdata/pull/20110) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/go-sql-driver/mysql from 1.9.1 to 1.9.2 in /src/go [\#20109](https://github.com/netdata/netdata/pull/20109) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/net from 0.38.0 to 0.39.0 in /src/go [\#20108](https://github.com/netdata/netdata/pull/20108) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/prometheus-community/pro-bing from 0.6.1 to 0.7.0 in /src/go [\#20107](https://github.com/netdata/netdata/pull/20107) ([dependabot[bot]](https://github.com/apps/dependabot))
- Further improve ACLK synchronization shutdown [\#20105](https://github.com/netdata/netdata/pull/20105) ([stelfrag](https://github.com/stelfrag))
- daemon status 27f [\#20104](https://github.com/netdata/netdata/pull/20104) ([ktsaou](https://github.com/ktsaou))
- daemon status 27e [\#20101](https://github.com/netdata/netdata/pull/20101) ([ktsaou](https://github.com/ktsaou))
- Improve journal file access error logging protect retention recalculation [\#20098](https://github.com/netdata/netdata/pull/20098) ([stelfrag](https://github.com/stelfrag))
- Fix Windows registry name crashes [\#20097](https://github.com/netdata/netdata/pull/20097) ([ktsaou](https://github.com/ktsaou))
- daemon status 27d [\#20096](https://github.com/netdata/netdata/pull/20096) ([ktsaou](https://github.com/ktsaou))
- Fix ACLK Backoff Timeout Logic [\#20095](https://github.com/netdata/netdata/pull/20095) ([stelfrag](https://github.com/stelfrag))
- Release memory after journalfile creation [\#20094](https://github.com/netdata/netdata/pull/20094) ([stelfrag](https://github.com/stelfrag))
- Protection access improvements 1 [\#20093](https://github.com/netdata/netdata/pull/20093) ([ktsaou](https://github.com/ktsaou))
- protected access against SIGBUS/SIGSEGV for journal v2 files [\#20092](https://github.com/netdata/netdata/pull/20092) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#20091](https://github.com/netdata/netdata/pull/20091) ([netdatabot](https://github.com/netdatabot))
- Fix typo in .github/scripts/gen-docker-tags.py [\#20089](https://github.com/netdata/netdata/pull/20089) ([Ferroin](https://github.com/Ferroin))
- daemon status 27c [\#20088](https://github.com/netdata/netdata/pull/20088) ([ktsaou](https://github.com/ktsaou))
- Fix inverted logic for skipping non-native CI jobs on PRs. [\#20087](https://github.com/netdata/netdata/pull/20087) ([Ferroin](https://github.com/Ferroin))
- Properly integrate dlib into our build system. [\#20086](https://github.com/netdata/netdata/pull/20086) ([Ferroin](https://github.com/Ferroin))
- Alerts and Notifications [\#20085](https://github.com/netdata/netdata/pull/20085) ([kanelatechnical](https://github.com/kanelatechnical))
- Fix memory allocation for timer callback data when cancelling a timer [\#20084](https://github.com/netdata/netdata/pull/20084) ([stelfrag](https://github.com/stelfrag))
- Fix crash during shutdown when there are pending messages to cloud [\#20080](https://github.com/netdata/netdata/pull/20080) ([stelfrag](https://github.com/stelfrag))
- Do not try to index jv2 files during shutdown [\#20079](https://github.com/netdata/netdata/pull/20079) ([stelfrag](https://github.com/stelfrag))
- Cleanup during shutdown [\#20078](https://github.com/netdata/netdata/pull/20078) ([stelfrag](https://github.com/stelfrag))
- ACLK synchronization improvements [\#20077](https://github.com/netdata/netdata/pull/20077) ([stelfrag](https://github.com/stelfrag))
- daemon status 27b [\#20076](https://github.com/netdata/netdata/pull/20076) ([ktsaou](https://github.com/ktsaou))
- Document switching from a native package to a static build [\#20075](https://github.com/netdata/netdata/pull/20075) ([ralphm](https://github.com/ralphm))
- agent events No 7 [\#20074](https://github.com/netdata/netdata/pull/20074) ([ktsaou](https://github.com/ktsaou))
- Update nodes-ephemerality.md [\#20073](https://github.com/netdata/netdata/pull/20073) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/miekg/dns from 1.1.64 to 1.1.65 in /src/go [\#20072](https://github.com/netdata/netdata/pull/20072) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/text from 0.23.0 to 0.24.0 in /src/go [\#20071](https://github.com/netdata/netdata/pull/20071) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/fsnotify/fsnotify from 1.8.0 to 1.9.0 in /src/go [\#20070](https://github.com/netdata/netdata/pull/20070) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix\(go.d/prometheus\): don't use "ratio" as unit [\#20069](https://github.com/netdata/netdata/pull/20069) ([ilyam8](https://github.com/ilyam8))
- agent-events: fix more metrics [\#20068](https://github.com/netdata/netdata/pull/20068) ([ktsaou](https://github.com/ktsaou))
- agent-events: Consolidate metrics into a single labeled counter [\#20067](https://github.com/netdata/netdata/pull/20067) ([ktsaou](https://github.com/ktsaou))
- ci: remove codeql-action build-mode none [\#20066](https://github.com/netdata/netdata/pull/20066) ([ilyam8](https://github.com/ilyam8))
- agent-events: fix metrics [\#20065](https://github.com/netdata/netdata/pull/20065) ([ktsaou](https://github.com/ktsaou))
- agent-events: fix metric names [\#20064](https://github.com/netdata/netdata/pull/20064) ([ktsaou](https://github.com/ktsaou))
- Improve agent-events web server [\#20063](https://github.com/netdata/netdata/pull/20063) ([ktsaou](https://github.com/ktsaou))
- Fix memory leaks [\#20062](https://github.com/netdata/netdata/pull/20062) ([ktsaou](https://github.com/ktsaou))
- Address Chart name \(Windows Hyper V\) [\#20060](https://github.com/netdata/netdata/pull/20060) ([thiagoftsm](https://github.com/thiagoftsm))
- daemon status 27 [\#20058](https://github.com/netdata/netdata/pull/20058) ([ktsaou](https://github.com/ktsaou))
- Improve ephemerality docs, adding `remove-stale-node` [\#20057](https://github.com/netdata/netdata/pull/20057) ([ralphm](https://github.com/ralphm))
- nested code block in doc [\#20056](https://github.com/netdata/netdata/pull/20056) ([Ancairon](https://github.com/Ancairon))
- Skip non-native builds in CI on PRs in most cases. [\#20055](https://github.com/netdata/netdata/pull/20055) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#20054](https://github.com/netdata/netdata/pull/20054) ([netdatabot](https://github.com/netdatabot))
- Remove unnecessary parameters for oidc configuration [\#20053](https://github.com/netdata/netdata/pull/20053) ([juacker](https://github.com/juacker))
- Observability cent points improved [\#20052](https://github.com/netdata/netdata/pull/20052) ([kanelatechnical](https://github.com/kanelatechnical))
- daemon status 26e [\#20051](https://github.com/netdata/netdata/pull/20051) ([ktsaou](https://github.com/ktsaou))
- fix cgroup netdev renames [\#20050](https://github.com/netdata/netdata/pull/20050) ([ktsaou](https://github.com/ktsaou))
- added checksum to detect corruption in netdev rename tasks [\#20048](https://github.com/netdata/netdata/pull/20048) ([ktsaou](https://github.com/ktsaou))
- daemon status 26d [\#20047](https://github.com/netdata/netdata/pull/20047) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/megacli\): handle Adapters with no drives [\#20046](https://github.com/netdata/netdata/pull/20046) ([ilyam8](https://github.com/ilyam8))
- Fix releasing statements after databases are closed [\#20045](https://github.com/netdata/netdata/pull/20045) ([stelfrag](https://github.com/stelfrag))
- daemon status 26c [\#20044](https://github.com/netdata/netdata/pull/20044) ([ktsaou](https://github.com/ktsaou))
- trace crashes No 4 [\#20043](https://github.com/netdata/netdata/pull/20043) ([ktsaou](https://github.com/ktsaou))
- daemon status 26b [\#20041](https://github.com/netdata/netdata/pull/20041) ([ktsaou](https://github.com/ktsaou))
- Update netdata-updater-daily.in [\#20039](https://github.com/netdata/netdata/pull/20039) ([dave818](https://github.com/dave818))
- chore\(otel/journaldexporter\): add trusted journald fields [\#20038](https://github.com/netdata/netdata/pull/20038) ([ilyam8](https://github.com/ilyam8))
- daemon status 26 - dmi strings [\#20037](https://github.com/netdata/netdata/pull/20037) ([ktsaou](https://github.com/ktsaou))
- Fix ACLK synchronization fatal on shutdown [\#20034](https://github.com/netdata/netdata/pull/20034) ([stelfrag](https://github.com/stelfrag))
- chore\(otel/journaldexporter\): convert logs to journald format [\#20033](https://github.com/netdata/netdata/pull/20033) ([ilyam8](https://github.com/ilyam8))
- Check for host timer validity in ACLK synchronization [\#20031](https://github.com/netdata/netdata/pull/20031) ([stelfrag](https://github.com/stelfrag))
- improvement\(go.d\): add `_hostname` label for virtual nodes [\#20030](https://github.com/netdata/netdata/pull/20030) ([ilyam8](https://github.com/ilyam8))
- trim-all [\#20029](https://github.com/netdata/netdata/pull/20029) ([ktsaou](https://github.com/ktsaou))
- fix crash [\#20028](https://github.com/netdata/netdata/pull/20028) ([ktsaou](https://github.com/ktsaou))
- logs enhancements [\#20027](https://github.com/netdata/netdata/pull/20027) ([ktsaou](https://github.com/ktsaou))
- daemon status 25 [\#20026](https://github.com/netdata/netdata/pull/20026) ([ktsaou](https://github.com/ktsaou))
- kickstart.sh: add missing option --offline-install-source to USAGE [\#20025](https://github.com/netdata/netdata/pull/20025) ([ycdtosa](https://github.com/ycdtosa))
- Improve kickstart so it can add the netdata user/group on Synology DSM [\#20024](https://github.com/netdata/netdata/pull/20024) ([ycdtosa](https://github.com/ycdtosa))
- on prem files moved to their own repo [\#20023](https://github.com/netdata/netdata/pull/20023) ([Ancairon](https://github.com/Ancairon))
- Series of NFCs to make the code more maintainable. [\#20022](https://github.com/netdata/netdata/pull/20022) ([vkalintiris](https://github.com/vkalintiris))
- Windows installer + ML \(all\) improved [\#20021](https://github.com/netdata/netdata/pull/20021) ([kanelatechnical](https://github.com/kanelatechnical))
- SNMP Collector, use custom YAML files for auto single metrics [\#20020](https://github.com/netdata/netdata/pull/20020) ([Ancairon](https://github.com/Ancairon))
- Improve estimated disk space usage for data file rotation [\#20019](https://github.com/netdata/netdata/pull/20019) ([stelfrag](https://github.com/stelfrag))
- Additional checks then creating a v2 journal file [\#20018](https://github.com/netdata/netdata/pull/20018) ([stelfrag](https://github.com/stelfrag))
- Properly clean up install paths after runtime checks in static builds. [\#20017](https://github.com/netdata/netdata/pull/20017) ([Ferroin](https://github.com/Ferroin))
- blacklist leaked machine guids [\#20016](https://github.com/netdata/netdata/pull/20016) ([ktsaou](https://github.com/ktsaou))
- agent-events: add deduplicating web server [\#20014](https://github.com/netdata/netdata/pull/20014) ([ktsaou](https://github.com/ktsaou))
- Validate journal file headers to prevent invalid memory access [\#20013](https://github.com/netdata/netdata/pull/20013) ([stelfrag](https://github.com/stelfrag))
- added agent-events backend [\#20012](https://github.com/netdata/netdata/pull/20012) ([ktsaou](https://github.com/ktsaou))
- daemon status 24d [\#20011](https://github.com/netdata/netdata/pull/20011) ([ktsaou](https://github.com/ktsaou))
- Update synology.md [\#20010](https://github.com/netdata/netdata/pull/20010) ([ycdtosa](https://github.com/ycdtosa))
- More completely disable our own telemetry in CI. [\#20009](https://github.com/netdata/netdata/pull/20009) ([Ferroin](https://github.com/Ferroin))
- fix\(go.d/megacli\): handle BBU hardware component is not present [\#20008](https://github.com/netdata/netdata/pull/20008) ([ilyam8](https://github.com/ilyam8))
- Fix crashes No 3 [\#20007](https://github.com/netdata/netdata/pull/20007) ([ktsaou](https://github.com/ktsaou))
- Minor changes when handling systemd integration. [\#20006](https://github.com/netdata/netdata/pull/20006) ([vkalintiris](https://github.com/vkalintiris))
- Deployment Guides Improved [\#20004](https://github.com/netdata/netdata/pull/20004) ([kanelatechnical](https://github.com/kanelatechnical))
- daemon status 24c [\#20003](https://github.com/netdata/netdata/pull/20003) ([ktsaou](https://github.com/ktsaou))
- use v4 UUIDs [\#20002](https://github.com/netdata/netdata/pull/20002) ([ktsaou](https://github.com/ktsaou))
- Update synology.md [\#20001](https://github.com/netdata/netdata/pull/20001) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump golang.org/x/net from 0.37.0 to 0.38.0 in /src/go [\#20000](https://github.com/netdata/netdata/pull/20000) ([dependabot[bot]](https://github.com/apps/dependabot))
- detect more CI [\#19999](https://github.com/netdata/netdata/pull/19999) ([ktsaou](https://github.com/ktsaou))
- status file 24 [\#19996](https://github.com/netdata/netdata/pull/19996) ([ktsaou](https://github.com/ktsaou))
- Improve jv2 load [\#19995](https://github.com/netdata/netdata/pull/19995) ([stelfrag](https://github.com/stelfrag))
- add kanelatechnical to CODEOWNERS [\#19994](https://github.com/netdata/netdata/pull/19994) ([ilyam8](https://github.com/ilyam8))
- docs: improve Synology NAS installation documentation clarity [\#19993](https://github.com/netdata/netdata/pull/19993) ([ilyam8](https://github.com/ilyam8))
- added worker last job id to status file [\#19992](https://github.com/netdata/netdata/pull/19992) ([ktsaou](https://github.com/ktsaou))
- Improve shutdown and datafile rotation [\#19991](https://github.com/netdata/netdata/pull/19991) ([stelfrag](https://github.com/stelfrag))
- Windows Services Monitoring [\#19990](https://github.com/netdata/netdata/pull/19990) ([thiagoftsm](https://github.com/thiagoftsm))
- Update synology.md [\#19989](https://github.com/netdata/netdata/pull/19989) ([ycdtosa](https://github.com/ycdtosa))
- Regenerate integrations docs [\#19988](https://github.com/netdata/netdata/pull/19988) ([netdatabot](https://github.com/netdatabot))
- Installation + docker, improvements [\#19987](https://github.com/netdata/netdata/pull/19987) ([kanelatechnical](https://github.com/kanelatechnical))
- Regenerate integrations docs [\#19986](https://github.com/netdata/netdata/pull/19986) ([netdatabot](https://github.com/netdatabot))
- perflib: do not dereference null pointer [\#19985](https://github.com/netdata/netdata/pull/19985) ([ktsaou](https://github.com/ktsaou))
- keep errno in out of memory situations [\#19984](https://github.com/netdata/netdata/pull/19984) ([ktsaou](https://github.com/ktsaou))
- do not allocate or access zero sized arrays [\#19983](https://github.com/netdata/netdata/pull/19983) ([ktsaou](https://github.com/ktsaou))
- Revert "fix undefined" [\#19982](https://github.com/netdata/netdata/pull/19982) ([stelfrag](https://github.com/stelfrag))
- Installation section Improvements [\#19981](https://github.com/netdata/netdata/pull/19981) ([kanelatechnical](https://github.com/kanelatechnical))
- Improve agent shutdown [\#19980](https://github.com/netdata/netdata/pull/19980) ([stelfrag](https://github.com/stelfrag))
- Release memory when calculating metric correlations [\#19979](https://github.com/netdata/netdata/pull/19979) ([stelfrag](https://github.com/stelfrag))
- Fix random crash during shutdown [\#19978](https://github.com/netdata/netdata/pull/19978) ([stelfrag](https://github.com/stelfrag))
- set max datafile size to 1GiB [\#19977](https://github.com/netdata/netdata/pull/19977) ([ktsaou](https://github.com/ktsaou))
- Doc Linux improved order in kickstart [\#19975](https://github.com/netdata/netdata/pull/19975) ([kanelatechnical](https://github.com/kanelatechnical))
- fix crash in variable\_lookup\_add\_result\_with\_score\(\) [\#19972](https://github.com/netdata/netdata/pull/19972) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19970](https://github.com/netdata/netdata/pull/19970) ([netdatabot](https://github.com/netdatabot))
- Update SCIM docs with Groups support [\#19969](https://github.com/netdata/netdata/pull/19969) ([juacker](https://github.com/juacker))
- build\(deps\): bump github.com/jackc/pgx/v5 from 5.7.3 to 5.7.4 in /src/go [\#19968](https://github.com/netdata/netdata/pull/19968) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.0.2+incompatible to 28.0.4+incompatible in /src/go [\#19967](https://github.com/netdata/netdata/pull/19967) ([dependabot[bot]](https://github.com/apps/dependabot))
- Improve ACLK sync shutdown process [\#19966](https://github.com/netdata/netdata/pull/19966) ([stelfrag](https://github.com/stelfrag))
- Handle journal\_v2 file creation failure due to OOM [\#19965](https://github.com/netdata/netdata/pull/19965) ([stelfrag](https://github.com/stelfrag))
- Fast restart on busy parents [\#19964](https://github.com/netdata/netdata/pull/19964) ([ktsaou](https://github.com/ktsaou))
- Set sqlite max soft and hard heap limit [\#19963](https://github.com/netdata/netdata/pull/19963) ([stelfrag](https://github.com/stelfrag))
- fix MSI installer [\#19962](https://github.com/netdata/netdata/pull/19962) ([ktsaou](https://github.com/ktsaou))
- Don’t skip building Go code on static builds. [\#19961](https://github.com/netdata/netdata/pull/19961) ([Ferroin](https://github.com/Ferroin))
- fix undefined [\#19960](https://github.com/netdata/netdata/pull/19960) ([ktsaou](https://github.com/ktsaou))
- daemon status 22c [\#19959](https://github.com/netdata/netdata/pull/19959) ([ktsaou](https://github.com/ktsaou))
- Use UPDATE\_DISCONNECTED mode for libbacktrace. [\#19958](https://github.com/netdata/netdata/pull/19958) ([Ferroin](https://github.com/Ferroin))
- status file 22b [\#19957](https://github.com/netdata/netdata/pull/19957) ([ktsaou](https://github.com/ktsaou))
- fix rrdcalc\_unlink\_from\_rrdset\(\) [\#19956](https://github.com/netdata/netdata/pull/19956) ([ktsaou](https://github.com/ktsaou))
- Fix claiming on startup [\#19954](https://github.com/netdata/netdata/pull/19954) ([stelfrag](https://github.com/stelfrag))
- daemon status 22 [\#19953](https://github.com/netdata/netdata/pull/19953) ([ktsaou](https://github.com/ktsaou))
- Enable interface to release sqlite memory [\#19952](https://github.com/netdata/netdata/pull/19952) ([stelfrag](https://github.com/stelfrag))
- Improve event loop thread creation [\#19951](https://github.com/netdata/netdata/pull/19951) ([stelfrag](https://github.com/stelfrag))
- IIS Application Pool \(Windows.plugin\) [\#19950](https://github.com/netdata/netdata/pull/19950) ([thiagoftsm](https://github.com/thiagoftsm))
- Disable generation of debuginfo packages for DEB distros. [\#19948](https://github.com/netdata/netdata/pull/19948) ([Ferroin](https://github.com/Ferroin))
- Set default CMake build type to include debug info. [\#19946](https://github.com/netdata/netdata/pull/19946) ([Ferroin](https://github.com/Ferroin))
- build\(deps\): bump github.com/miekg/dns from 1.1.63 to 1.1.64 in /src/go [\#19945](https://github.com/netdata/netdata/pull/19945) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/jackc/pgx/v5 from 5.7.2 to 5.7.3 in /src/go [\#19944](https://github.com/netdata/netdata/pull/19944) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/go-sql-driver/mysql from 1.9.0 to 1.9.1 in /src/go [\#19943](https://github.com/netdata/netdata/pull/19943) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.0.1+incompatible to 28.0.2+incompatible in /src/go [\#19942](https://github.com/netdata/netdata/pull/19942) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/DataDog/datadog-agent/pkg/networkdevice/profile from 0.65.0-devel.0.20250317105920-ce55f088ab29 to 0.66.0-devel in /src/go [\#19941](https://github.com/netdata/netdata/pull/19941) ([dependabot[bot]](https://github.com/apps/dependabot))
- Update src/aclk/aclk-schemas to latest version. [\#19940](https://github.com/netdata/netdata/pull/19940) ([Ferroin](https://github.com/Ferroin))
- Don't build libunwind in static builds when it's not needed. [\#19939](https://github.com/netdata/netdata/pull/19939) ([Ferroin](https://github.com/Ferroin))
- detect low ram conditions more aggresively [\#19938](https://github.com/netdata/netdata/pull/19938) ([ktsaou](https://github.com/ktsaou))
- status file 21b [\#19937](https://github.com/netdata/netdata/pull/19937) ([ktsaou](https://github.com/ktsaou))
- Fix logic for libbacktrace enablement in CMakeLists,txt [\#19936](https://github.com/netdata/netdata/pull/19936) ([Ferroin](https://github.com/Ferroin))
- Fix path to copy drop-in crontab from [\#19935](https://github.com/netdata/netdata/pull/19935) ([ralphm](https://github.com/ralphm))
- Fix max\_page\_length calculation for GORILLA\_32BIT page type [\#19932](https://github.com/netdata/netdata/pull/19932) ([stelfrag](https://github.com/stelfrag))
- Fix compile without dbengine [\#19930](https://github.com/netdata/netdata/pull/19930) ([stelfrag](https://github.com/stelfrag))
- Metadata event loop code cleanup [\#19929](https://github.com/netdata/netdata/pull/19929) ([stelfrag](https://github.com/stelfrag))
- status file v21 [\#19928](https://github.com/netdata/netdata/pull/19928) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.7.1 to 9.7.3 in /src/go [\#19926](https://github.com/netdata/netdata/pull/19926) ([dependabot[bot]](https://github.com/apps/dependabot))
- do not expose web server filenames [\#19925](https://github.com/netdata/netdata/pull/19925) ([ktsaou](https://github.com/ktsaou))
- Fix TOCTOU race in daemon status file handling. [\#19924](https://github.com/netdata/netdata/pull/19924) ([Ferroin](https://github.com/Ferroin))
- Exclude external code from CodeQL scanning. [\#19923](https://github.com/netdata/netdata/pull/19923) ([Ferroin](https://github.com/Ferroin))
- remove ilove endpoint [\#19919](https://github.com/netdata/netdata/pull/19919) ([ilyam8](https://github.com/ilyam8))
- Dump Netdata buildinfo during CI. [\#19918](https://github.com/netdata/netdata/pull/19918) ([Ferroin](https://github.com/Ferroin))
- Align cmsgbuf to size\_t to avoid unaligned memory access. [\#19917](https://github.com/netdata/netdata/pull/19917) ([vkalintiris](https://github.com/vkalintiris))
- Make sure ACLK sync thread completes initialization [\#19916](https://github.com/netdata/netdata/pull/19916) ([stelfrag](https://github.com/stelfrag))
- do not enqueue command if aclk is not initialized [\#19914](https://github.com/netdata/netdata/pull/19914) ([ktsaou](https://github.com/ktsaou))
- detect null datafile while finding datafiles in range [\#19913](https://github.com/netdata/netdata/pull/19913) ([ktsaou](https://github.com/ktsaou))
- post the first status when there is no last status [\#19912](https://github.com/netdata/netdata/pull/19912) ([ktsaou](https://github.com/ktsaou))
- initial implementation of libbacktrace [\#19910](https://github.com/netdata/netdata/pull/19910) ([ktsaou](https://github.com/ktsaou))
- fix reliability calculation [\#19909](https://github.com/netdata/netdata/pull/19909) ([ktsaou](https://github.com/ktsaou))
- improvement\(health/dyncfg\): add widget to load available contexts [\#19904](https://github.com/netdata/netdata/pull/19904) ([ilyam8](https://github.com/ilyam8))
- new exit cause: shutdown timeout [\#19903](https://github.com/netdata/netdata/pull/19903) ([ktsaou](https://github.com/ktsaou))
- Store alert config asynchronously [\#19885](https://github.com/netdata/netdata/pull/19885) ([stelfrag](https://github.com/stelfrag))
- Large-scale cleanup of static build infrastructure. [\#19852](https://github.com/netdata/netdata/pull/19852) ([Ferroin](https://github.com/Ferroin))
- ebpf.plugin: rework memory [\#19844](https://github.com/netdata/netdata/pull/19844) ([thiagoftsm](https://github.com/thiagoftsm))
- Add Docker tags for the last few nightly builds. [\#19734](https://github.com/netdata/netdata/pull/19734) ([Ferroin](https://github.com/Ferroin))

## [v2.3.2](https://github.com/netdata/netdata/tree/v2.3.2) (2025-04-02)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.1...v2.3.2)

## [v2.3.1](https://github.com/netdata/netdata/tree/v2.3.1) (2025-03-24)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.3.0...v2.3.1)

## [v2.3.0](https://github.com/netdata/netdata/tree/v2.3.0) (2025-03-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.2.6...v2.3.0)

**Merged pull requests:**

- Remove auto-retry on changelog generation. [\#19908](https://github.com/netdata/netdata/pull/19908) ([Ferroin](https://github.com/Ferroin))
- Bump repoconfig version used in kickstart script to 5-1. [\#19906](https://github.com/netdata/netdata/pull/19906) ([Ferroin](https://github.com/Ferroin))
- Revert "Fix compile without dbengine" [\#19905](https://github.com/netdata/netdata/pull/19905) ([stelfrag](https://github.com/stelfrag))
- Fix compile without dbengine [\#19902](https://github.com/netdata/netdata/pull/19902) ([stelfrag](https://github.com/stelfrag))
- do not use errno when hashing status events [\#19900](https://github.com/netdata/netdata/pull/19900) ([ktsaou](https://github.com/ktsaou))
- more compilation flags for stack traces [\#19899](https://github.com/netdata/netdata/pull/19899) ([ktsaou](https://github.com/ktsaou))
- more strict checks on log-fw [\#19898](https://github.com/netdata/netdata/pull/19898) ([ktsaou](https://github.com/ktsaou))
- fix for system shutdown [\#19897](https://github.com/netdata/netdata/pull/19897) ([ktsaou](https://github.com/ktsaou))
- build: update otel deps to v0.122.0 [\#19895](https://github.com/netdata/netdata/pull/19895) ([ilyam8](https://github.com/ilyam8))
- do not recurse cleanup on shutdown [\#19894](https://github.com/netdata/netdata/pull/19894) ([ktsaou](https://github.com/ktsaou))
- make sure all rrdcalcs are unlinked the moment they are deleted [\#19893](https://github.com/netdata/netdata/pull/19893) ([ktsaou](https://github.com/ktsaou))
- Fix typo in README title [\#19891](https://github.com/netdata/netdata/pull/19891) ([felipecrs](https://github.com/felipecrs))
- remove deadlock from dyncfg health [\#19890](https://github.com/netdata/netdata/pull/19890) ([ktsaou](https://github.com/ktsaou))
- Update DEB/RPM package signing key info. [\#19888](https://github.com/netdata/netdata/pull/19888) ([Ferroin](https://github.com/Ferroin))
- fix\(go.d/snmp/ddsnmp\): correct profile directory path [\#19887](https://github.com/netdata/netdata/pull/19887) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp/ddsnmp\): use dd profile definition [\#19886](https://github.com/netdata/netdata/pull/19886) ([ilyam8](https://github.com/ilyam8))
- daemon status 18b [\#19884](https://github.com/netdata/netdata/pull/19884) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19883](https://github.com/netdata/netdata/pull/19883) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/snmp\): improve auto-detection section [\#19882](https://github.com/netdata/netdata/pull/19882) ([ilyam8](https://github.com/ilyam8))
- ci: use step-security/changed-files [\#19881](https://github.com/netdata/netdata/pull/19881) ([ilyam8](https://github.com/ilyam8))
- change log priorities on agent-events [\#19880](https://github.com/netdata/netdata/pull/19880) ([ktsaou](https://github.com/ktsaou))
- add stack trace information to the compiler and linker [\#19879](https://github.com/netdata/netdata/pull/19879) ([ktsaou](https://github.com/ktsaou))
- SIGABRT and already running are fatal conditions [\#19878](https://github.com/netdata/netdata/pull/19878) ([ktsaou](https://github.com/ktsaou))
- daemon-status-18 [\#19876](https://github.com/netdata/netdata/pull/19876) ([ktsaou](https://github.com/ktsaou))
- do not lose exit reasons [\#19875](https://github.com/netdata/netdata/pull/19875) ([ktsaou](https://github.com/ktsaou))
- make sure the daemon status hash does not depend on random bytes [\#19874](https://github.com/netdata/netdata/pull/19874) ([ktsaou](https://github.com/ktsaou))
- add the fatal to the exit reasons [\#19873](https://github.com/netdata/netdata/pull/19873) ([ktsaou](https://github.com/ktsaou))
- sentry events annotations [\#19872](https://github.com/netdata/netdata/pull/19872) ([ktsaou](https://github.com/ktsaou))
- Remove tj-actions/changed-files from CI jobs. [\#19870](https://github.com/netdata/netdata/pull/19870) ([Ferroin](https://github.com/Ferroin))
- daemon status file 17 [\#19869](https://github.com/netdata/netdata/pull/19869) ([ktsaou](https://github.com/ktsaou))
- fixed sentry version [\#19868](https://github.com/netdata/netdata/pull/19868) ([ktsaou](https://github.com/ktsaou))
- fixed sentry dedup [\#19867](https://github.com/netdata/netdata/pull/19867) ([ktsaou](https://github.com/ktsaou))
- fix\(freebsd.plugin\): correct disks/network devices charts [\#19866](https://github.com/netdata/netdata/pull/19866) ([ilyam8](https://github.com/ilyam8))
- improvement\(macos.plugin\): add options to filter net ifaces and mountpoints [\#19865](https://github.com/netdata/netdata/pull/19865) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/prometheus/common from 0.62.0 to 0.63.0 in /src/go [\#19864](https://github.com/netdata/netdata/pull/19864) ([dependabot[bot]](https://github.com/apps/dependabot))
- daemon status file 16 [\#19863](https://github.com/netdata/netdata/pull/19863) ([ktsaou](https://github.com/ktsaou))
- Release memory on shutdown - detect invalid extent in journal files [\#19861](https://github.com/netdata/netdata/pull/19861) ([stelfrag](https://github.com/stelfrag))
- restore needed variables for pluginsd [\#19860](https://github.com/netdata/netdata/pull/19860) ([ktsaou](https://github.com/ktsaou))
- fix\(macos.plugin\): correct disks/network devices charts [\#19859](https://github.com/netdata/netdata/pull/19859) ([ilyam8](https://github.com/ilyam8))
- disable UNW\_LOCAL\_ONLY on static builds [\#19858](https://github.com/netdata/netdata/pull/19858) ([ktsaou](https://github.com/ktsaou))
- daemon status 15 [\#19857](https://github.com/netdata/netdata/pull/19857) ([ktsaou](https://github.com/ktsaou))
- fix crashes identified by sentry [\#19856](https://github.com/netdata/netdata/pull/19856) ([ktsaou](https://github.com/ktsaou))
- netdata-uninstaller: improve input prompt with more descriptive guidance [\#19855](https://github.com/netdata/netdata/pull/19855) ([ilyam8](https://github.com/ilyam8))
- make sure alerts are concurrently altered by dyncfg [\#19854](https://github.com/netdata/netdata/pull/19854) ([ktsaou](https://github.com/ktsaou))
- fix contexts labels to avoid clearing the rrdlabels pointer [\#19853](https://github.com/netdata/netdata/pull/19853) ([ktsaou](https://github.com/ktsaou))
- fix updating on RPi2+ [\#19850](https://github.com/netdata/netdata/pull/19850) ([ilyam8](https://github.com/ilyam8))
- minor fixes [\#19849](https://github.com/netdata/netdata/pull/19849) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump k8s.io/client-go from 0.32.2 to 0.32.3 in /src/go [\#19848](https://github.com/netdata/netdata/pull/19848) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.48.1 to 0.49.0 in /src/go [\#19845](https://github.com/netdata/netdata/pull/19845) ([dependabot[bot]](https://github.com/apps/dependabot))
- docs: fix typos in nodes-ephemerality.md [\#19840](https://github.com/netdata/netdata/pull/19840) ([ilyam8](https://github.com/ilyam8))
- Add oci meta info [\#19839](https://github.com/netdata/netdata/pull/19839) ([Passific](https://github.com/Passific))
- fix rrdset name crash on cleanup [\#19838](https://github.com/netdata/netdata/pull/19838) ([ktsaou](https://github.com/ktsaou))
- when destroying pgc, check if the cache is null [\#19837](https://github.com/netdata/netdata/pull/19837) ([ktsaou](https://github.com/ktsaou))
- Fix for building with protobuf 30.0 [\#19835](https://github.com/netdata/netdata/pull/19835) ([vkalintiris](https://github.com/vkalintiris))
- Improve CI reliability by allowing for better retry behavior. [\#19834](https://github.com/netdata/netdata/pull/19834) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19833](https://github.com/netdata/netdata/pull/19833) ([netdatabot](https://github.com/netdatabot))
- Fix typo in otel collector build infra. [\#19832](https://github.com/netdata/netdata/pull/19832) ([Ferroin](https://github.com/Ferroin))
- store status file in /var/lib/netdata, not in /var/cache/netdata [\#19831](https://github.com/netdata/netdata/pull/19831) ([ktsaou](https://github.com/ktsaou))
- Fix RRDDIM\_MEM storage engine index [\#19830](https://github.com/netdata/netdata/pull/19830) ([ktsaou](https://github.com/ktsaou))
- improvement\(go.d/k8state\): add CronJob suspend status [\#19829](https://github.com/netdata/netdata/pull/19829) ([ilyam8](https://github.com/ilyam8))
- Revert "fix rrdset name crash on rrdset obsoletion" [\#19828](https://github.com/netdata/netdata/pull/19828) ([ktsaou](https://github.com/ktsaou))
- free strings judy arrays to show leaked strings [\#19827](https://github.com/netdata/netdata/pull/19827) ([ktsaou](https://github.com/ktsaou))
- rrdhost name fix heap-use-after-free [\#19826](https://github.com/netdata/netdata/pull/19826) ([ktsaou](https://github.com/ktsaou))
- use notice log level for "machine ID found" [\#19825](https://github.com/netdata/netdata/pull/19825) ([ilyam8](https://github.com/ilyam8))
- build\(otel-collector\): update to v0.121.0 [\#19824](https://github.com/netdata/netdata/pull/19824) ([ilyam8](https://github.com/ilyam8))
- Finding leaks No 2 [\#19823](https://github.com/netdata/netdata/pull/19823) ([ktsaou](https://github.com/ktsaou))
- Free all memory on exit [\#19821](https://github.com/netdata/netdata/pull/19821) ([ktsaou](https://github.com/ktsaou))
- Fix LSAN and memory leaks [\#19819](https://github.com/netdata/netdata/pull/19819) ([ktsaou](https://github.com/ktsaou))
- Include libucontext in static builds to vendor libunwind even on POWER. [\#19817](https://github.com/netdata/netdata/pull/19817) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19816](https://github.com/netdata/netdata/pull/19816) ([netdatabot](https://github.com/netdatabot))
- fix\(go.d/filecheck\): remove dyncfg path validation pattern  [\#19815](https://github.com/netdata/netdata/pull/19815) ([ilyam8](https://github.com/ilyam8))
- Initial commit with snmp profile code [\#19813](https://github.com/netdata/netdata/pull/19813) ([Ancairon](https://github.com/Ancairon))
- Acquire datafile for deletion before calculating retention [\#19812](https://github.com/netdata/netdata/pull/19812) ([stelfrag](https://github.com/stelfrag))
- Detect memory leaks [\#19811](https://github.com/netdata/netdata/pull/19811) ([ktsaou](https://github.com/ktsaou))
- Avoid zero timeout in libuv timers [\#19810](https://github.com/netdata/netdata/pull/19810) ([stelfrag](https://github.com/stelfrag))
- fix fsanitize ifdefs [\#19809](https://github.com/netdata/netdata/pull/19809) ([ktsaou](https://github.com/ktsaou))
- do not change the scheduling policy by default [\#19808](https://github.com/netdata/netdata/pull/19808) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/pihole\): switch to pihole6 api [\#19807](https://github.com/netdata/netdata/pull/19807) ([ilyam8](https://github.com/ilyam8))
- Help finding leaks and running valgrind [\#19806](https://github.com/netdata/netdata/pull/19806) ([ktsaou](https://github.com/ktsaou))
- fix memory corruption in streaming [\#19805](https://github.com/netdata/netdata/pull/19805) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19804](https://github.com/netdata/netdata/pull/19804) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations docs [\#19803](https://github.com/netdata/netdata/pull/19803) ([netdatabot](https://github.com/netdatabot))
- async-signal-safe stack traces [\#19802](https://github.com/netdata/netdata/pull/19802) ([ktsaou](https://github.com/ktsaou))
- add k8s\_state\_cronjob\_last\_execution\_failed alert [\#19801](https://github.com/netdata/netdata/pull/19801) ([ilyam8](https://github.com/ilyam8))
- bump dag jinja to 3.1.6 [\#19800](https://github.com/netdata/netdata/pull/19800) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump golang.org/x/net from 0.35.0 to 0.37.0 in /src/go [\#19799](https://github.com/netdata/netdata/pull/19799) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations docs [\#19797](https://github.com/netdata/netdata/pull/19797) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8s\_state\): add more CronJob metrics [\#19796](https://github.com/netdata/netdata/pull/19796) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19794](https://github.com/netdata/netdata/pull/19794) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8s\_state\): collect cronjobs [\#19793](https://github.com/netdata/netdata/pull/19793) ([ilyam8](https://github.com/ilyam8))
- status file improvements 12 [\#19792](https://github.com/netdata/netdata/pull/19792) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19791](https://github.com/netdata/netdata/pull/19791) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/snmp\): add snmp discovery information [\#19790](https://github.com/netdata/netdata/pull/19790) ([ilyam8](https://github.com/ilyam8))
- User configurable crash reporting [\#19789](https://github.com/netdata/netdata/pull/19789) ([ktsaou](https://github.com/ktsaou))
- detect when running in CI and disable posting status [\#19787](https://github.com/netdata/netdata/pull/19787) ([ktsaou](https://github.com/ktsaou))
- chore: rename snmp.profiles.d -\> snmp.profiles [\#19786](https://github.com/netdata/netdata/pull/19786) ([ilyam8](https://github.com/ilyam8))
- add datadog profiles for snmp collector [\#19785](https://github.com/netdata/netdata/pull/19785) ([Ancairon](https://github.com/Ancairon))
- Revert broken DEB priority configuration in repoconfig packages. [\#19783](https://github.com/netdata/netdata/pull/19783) ([Ferroin](https://github.com/Ferroin))
- Restructure shutdown logic used during updates. [\#19781](https://github.com/netdata/netdata/pull/19781) ([Ferroin](https://github.com/Ferroin))
- add unique machine id to status file [\#19778](https://github.com/netdata/netdata/pull/19778) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/sd\): fix logging cfg source when disabled [\#19777](https://github.com/netdata/netdata/pull/19777) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/sd\): add file path to k8s/snmp discovered job source [\#19776](https://github.com/netdata/netdata/pull/19776) ([ilyam8](https://github.com/ilyam8))
- Improve agent shutdown [\#19775](https://github.com/netdata/netdata/pull/19775) ([stelfrag](https://github.com/stelfrag))
- Fix SIGSEGV on static installs due to dengine log [\#19774](https://github.com/netdata/netdata/pull/19774) ([ktsaou](https://github.com/ktsaou))
- kickstart: install native pkg on RPi2+ [\#19773](https://github.com/netdata/netdata/pull/19773) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/sd\): rename discoverers pkgs [\#19772](https://github.com/netdata/netdata/pull/19772) ([ilyam8](https://github.com/ilyam8))
- block signals before curl [\#19771](https://github.com/netdata/netdata/pull/19771) ([ktsaou](https://github.com/ktsaou))
- block all signals before spawning any threads [\#19770](https://github.com/netdata/netdata/pull/19770) ([ktsaou](https://github.com/ktsaou))
- add handling for sigabrt in the status file [\#19769](https://github.com/netdata/netdata/pull/19769) ([ktsaou](https://github.com/ktsaou))
- copy fields only when the source is valid [\#19768](https://github.com/netdata/netdata/pull/19768) ([ktsaou](https://github.com/ktsaou))
- detect crashes during status file processing [\#19767](https://github.com/netdata/netdata/pull/19767) ([ktsaou](https://github.com/ktsaou))
- post status syncrhonously [\#19766](https://github.com/netdata/netdata/pull/19766) ([ktsaou](https://github.com/ktsaou))
- enable libunwind in static builds [\#19764](https://github.com/netdata/netdata/pull/19764) ([ktsaou](https://github.com/ktsaou))
- fix invalid free [\#19763](https://github.com/netdata/netdata/pull/19763) ([ktsaou](https://github.com/ktsaou))
- make status file use fixed size character arrays [\#19761](https://github.com/netdata/netdata/pull/19761) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/sd/snmp\): use rescan and cache ttl only when set [\#19760](https://github.com/netdata/netdata/pull/19760) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/nvidia\_smi\): handle xml gpu\_power\_readings change [\#19759](https://github.com/netdata/netdata/pull/19759) ([ilyam8](https://github.com/ilyam8))
- status file timings per step [\#19758](https://github.com/netdata/netdata/pull/19758) ([ktsaou](https://github.com/ktsaou))
- improvement\(go.d/sd/snmp\): support device cache ttl 0 [\#19756](https://github.com/netdata/netdata/pull/19756) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/sd/snmp\): comment out  defaults in snmp.conf [\#19755](https://github.com/netdata/netdata/pull/19755) ([ilyam8](https://github.com/ilyam8))
- Add documentation outlining how to use custom CA certificates with Netdata. [\#19754](https://github.com/netdata/netdata/pull/19754) ([Ferroin](https://github.com/Ferroin))
- status file version 8 [\#19753](https://github.com/netdata/netdata/pull/19753) ([ktsaou](https://github.com/ktsaou))
- status file improvements \(dedup and signal handler use\) [\#19751](https://github.com/netdata/netdata/pull/19751) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/axiomhq/hyperloglog from 0.2.3 to 0.2.5 in /src/go [\#19750](https://github.com/netdata/netdata/pull/19750) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/likexian/whois from 1.15.5 to 1.15.6 in /src/go [\#19749](https://github.com/netdata/netdata/pull/19749) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump go.mongodb.org/mongo-driver from 1.17.2 to 1.17.3 in /src/go [\#19748](https://github.com/netdata/netdata/pull/19748) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/gosnmp/gosnmp from 1.38.0 to 1.39.0 in /src/go [\#19747](https://github.com/netdata/netdata/pull/19747) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.0.0+incompatible to 28.0.1+incompatible in /src/go [\#19746](https://github.com/netdata/netdata/pull/19746) ([dependabot[bot]](https://github.com/apps/dependabot))
- more strict parsing of the output of system-info.sh [\#19745](https://github.com/netdata/netdata/pull/19745) ([ktsaou](https://github.com/ktsaou))
- pass NULL to sensors\_init\(\) when the standard files exist in /etc/ [\#19744](https://github.com/netdata/netdata/pull/19744) ([ktsaou](https://github.com/ktsaou))
- allow coredumps to be generated [\#19743](https://github.com/netdata/netdata/pull/19743) ([ktsaou](https://github.com/ktsaou))
- work on agent-events crashes [\#19741](https://github.com/netdata/netdata/pull/19741) ([ktsaou](https://github.com/ktsaou))
- zero mtime when a fallback check fails [\#19740](https://github.com/netdata/netdata/pull/19740) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d\): ignore sigpipe to exit gracefully [\#19739](https://github.com/netdata/netdata/pull/19739) ([ilyam8](https://github.com/ilyam8))
- Capture deadly signals [\#19737](https://github.com/netdata/netdata/pull/19737) ([ktsaou](https://github.com/ktsaou))
- allow insecure cloud connections [\#19736](https://github.com/netdata/netdata/pull/19736) ([ktsaou](https://github.com/ktsaou))
- add more information about claiming failures [\#19735](https://github.com/netdata/netdata/pull/19735) ([ktsaou](https://github.com/ktsaou))
- support https\_proxy too [\#19733](https://github.com/netdata/netdata/pull/19733) ([ktsaou](https://github.com/ktsaou))
- fix json generation of apps.plugin processes function info [\#19732](https://github.com/netdata/netdata/pull/19732) ([ktsaou](https://github.com/ktsaou))
- add another step when initializing web [\#19731](https://github.com/netdata/netdata/pull/19731) ([ktsaou](https://github.com/ktsaou))
- improved descriptions of exit reasons [\#19730](https://github.com/netdata/netdata/pull/19730) ([ktsaou](https://github.com/ktsaou))
- do not post empty reports [\#19729](https://github.com/netdata/netdata/pull/19729) ([ktsaou](https://github.com/ktsaou))
- docs: clarify Windows Agent limits on free plans [\#19727](https://github.com/netdata/netdata/pull/19727) ([ilyam8](https://github.com/ilyam8))
- improve status file deduplication [\#19726](https://github.com/netdata/netdata/pull/19726) ([ktsaou](https://github.com/ktsaou))
- handle flushing state during exit [\#19725](https://github.com/netdata/netdata/pull/19725) ([ktsaou](https://github.com/ktsaou))
- allow configuring journal v2 unmount time; turn it off for parents [\#19724](https://github.com/netdata/netdata/pull/19724) ([ktsaou](https://github.com/ktsaou))
- minor status file annotation fixes [\#19723](https://github.com/netdata/netdata/pull/19723) ([ktsaou](https://github.com/ktsaou))
- status has install type [\#19722](https://github.com/netdata/netdata/pull/19722) ([ktsaou](https://github.com/ktsaou))
- more status file annotations [\#19721](https://github.com/netdata/netdata/pull/19721) ([ktsaou](https://github.com/ktsaou))
- feat\(go.d\): add snmp devices discovery [\#19720](https://github.com/netdata/netdata/pull/19720) ([ilyam8](https://github.com/ilyam8))
- save status on out of memory event [\#19719](https://github.com/netdata/netdata/pull/19719) ([ktsaou](https://github.com/ktsaou))
- attempt to save status file from the signal handler [\#19718](https://github.com/netdata/netdata/pull/19718) ([ktsaou](https://github.com/ktsaou))
- unified out of memory handling [\#19717](https://github.com/netdata/netdata/pull/19717) ([ktsaou](https://github.com/ktsaou))
- chore\(go.d\): add file persister [\#19716](https://github.com/netdata/netdata/pull/19716) ([ilyam8](https://github.com/ilyam8))
- do not call cleanup and exit on fatal conditions during startup [\#19715](https://github.com/netdata/netdata/pull/19715) ([ktsaou](https://github.com/ktsaou))
- do not use mmap when the mmap limit is too low [\#19714](https://github.com/netdata/netdata/pull/19714) ([ktsaou](https://github.com/ktsaou))
- systemd-journal: allow almost all fields to be facets [\#19713](https://github.com/netdata/netdata/pull/19713) ([ktsaou](https://github.com/ktsaou))
- deduplicate all crash reports [\#19712](https://github.com/netdata/netdata/pull/19712) ([ktsaou](https://github.com/ktsaou))
- 4 malloc arenas for parents, not IoT [\#19711](https://github.com/netdata/netdata/pull/19711) ([ktsaou](https://github.com/ktsaou))
- Fix Fresh Installation on Microsoft [\#19710](https://github.com/netdata/netdata/pull/19710) ([thiagoftsm](https://github.com/thiagoftsm))
- Avoid post initialization errors repeateadly [\#19709](https://github.com/netdata/netdata/pull/19709) ([ktsaou](https://github.com/ktsaou))
- Check for final step [\#19708](https://github.com/netdata/netdata/pull/19708) ([stelfrag](https://github.com/stelfrag))
- daemon status improvements 3 [\#19707](https://github.com/netdata/netdata/pull/19707) ([ktsaou](https://github.com/ktsaou))
- fix runtime directory; annotate daemon status file [\#19706](https://github.com/netdata/netdata/pull/19706) ([ktsaou](https://github.com/ktsaou))
- Add repository priority configuration for DEB package repositories. [\#19705](https://github.com/netdata/netdata/pull/19705) ([Ferroin](https://github.com/Ferroin))
- add host/os fields to status file [\#19704](https://github.com/netdata/netdata/pull/19704) ([ktsaou](https://github.com/ktsaou))
- under MSYS2 use stat [\#19703](https://github.com/netdata/netdata/pull/19703) ([ktsaou](https://github.com/ktsaou))
- Integrate OpenTelemetry collector build into build system. [\#19702](https://github.com/netdata/netdata/pull/19702) ([Ferroin](https://github.com/Ferroin))
- Document journal v2 index file format. [\#19701](https://github.com/netdata/netdata/pull/19701) ([vkalintiris](https://github.com/vkalintiris))
- build\(deps\): update go.d packages [\#19700](https://github.com/netdata/netdata/pull/19700) ([ilyam8](https://github.com/ilyam8))
- ADFS \(windows.plugin\) [\#19699](https://github.com/netdata/netdata/pull/19699) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/sijms/go-ora/v2 from 2.8.23 to 2.8.24 in /src/go [\#19698](https://github.com/netdata/netdata/pull/19698) ([dependabot[bot]](https://github.com/apps/dependabot))
- change the moto and the description of netdata [\#19696](https://github.com/netdata/netdata/pull/19696) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.7.0 to 9.7.1 in /src/go [\#19693](https://github.com/netdata/netdata/pull/19693) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 27.5.1+incompatible to 28.0.0+incompatible in /src/go [\#19692](https://github.com/netdata/netdata/pull/19692) ([dependabot[bot]](https://github.com/apps/dependabot))
- load health config before creating localhost [\#19689](https://github.com/netdata/netdata/pull/19689) ([ktsaou](https://github.com/ktsaou))
- chore\(go.d/pkg/iprange\): add iterator [\#19688](https://github.com/netdata/netdata/pull/19688) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/mysql\): InnodbOSLogIO in MariaDB \>= 10.8 [\#19687](https://github.com/netdata/netdata/pull/19687) ([arkamar](https://github.com/arkamar))

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

[Full Changelog](https://github.com/netdata/netdata/compare/1.34.0...v1.34.1)

## [1.34.0](https://github.com/netdata/netdata/tree/1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.0...1.34.0)

## [v1.34.0](https://github.com/netdata/netdata/tree/v1.34.0) (2022-04-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.33.1...v1.34.0)

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
