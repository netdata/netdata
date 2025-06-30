# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.5.4...HEAD)

**Merged pull requests:**

- build\(deps\): bump github.com/docker/docker from 28.2.2+incompatible to 28.3.0+incompatible in /src/go [\#20595](https://github.com/netdata/netdata/pull/20595) ([dependabot[bot]](https://github.com/apps/dependabot))
- improve\(go.d/snmp-profiles\): extend transformEntitySensorValue [\#20594](https://github.com/netdata/netdata/pull/20594) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/go-viper/mapstructure/v2 from 2.2.1 to 2.3.0 in /src/go/otel-collector/exporter/journaldexporter [\#20592](https://github.com/netdata/netdata/pull/20592) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/go-viper/mapstructure/v2 from 2.2.1 to 2.3.0 in /src/go/otel-collector/exporter/netdataexporter [\#20591](https://github.com/netdata/netdata/pull/20591) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations docs [\#20589](https://github.com/netdata/netdata/pull/20589) ([netdatabot](https://github.com/netdatabot))
- doc: update SCIM doc [\#20588](https://github.com/netdata/netdata/pull/20588) ([juacker](https://github.com/juacker))
- ddsnmp add pow transform func and allow mapping duplicate values [\#20587](https://github.com/netdata/netdata/pull/20587) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): correct matching same profile multiple times [\#20586](https://github.com/netdata/netdata/pull/20586) ([ilyam8](https://github.com/ilyam8))
- remove devType/Vendor/ from ddsnmp metric families [\#20585](https://github.com/netdata/netdata/pull/20585) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): include table name in config id [\#20584](https://github.com/netdata/netdata/pull/20584) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): walk cross-table columns when referenced table has no metrics [\#20583](https://github.com/netdata/netdata/pull/20583) ([ilyam8](https://github.com/ilyam8))
- Add Rocky Linux 10 to CI and package builds. [\#20578](https://github.com/netdata/netdata/pull/20578) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#20577](https://github.com/netdata/netdata/pull/20577) ([netdatabot](https://github.com/netdatabot))
- chore\(go.d/snmp-profiles\): skip abstract when loading [\#20576](https://github.com/netdata/netdata/pull/20576) ([ilyam8](https://github.com/ilyam8))
- SNMP: cyberpower-pdu profile [\#20575](https://github.com/netdata/netdata/pull/20575) ([Ancairon](https://github.com/Ancairon))
- improve\(go.d/smartctl\): add Win default path for smartctl executable [\#20574](https://github.com/netdata/netdata/pull/20574) ([ilyam8](https://github.com/ilyam8))
- NUMA Windows  [\#20573](https://github.com/netdata/netdata/pull/20573) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations docs [\#20571](https://github.com/netdata/netdata/pull/20571) ([netdatabot](https://github.com/netdatabot))
- Add defines for cleanup statements [\#20570](https://github.com/netdata/netdata/pull/20570) ([stelfrag](https://github.com/stelfrag))
- improve\(go.d/smartctl\): add configurable concurrent device scanning [\#20569](https://github.com/netdata/netdata/pull/20569) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.10.0 to 9.11.0 in /src/go [\#20568](https://github.com/netdata/netdata/pull/20568) ([dependabot[bot]](https://github.com/apps/dependabot))
- improve\(go.d/smartctl\): enable direct smartctl execution on non-Linux [\#20567](https://github.com/netdata/netdata/pull/20567) ([ilyam8](https://github.com/ilyam8))
- Switch install types [\#20564](https://github.com/netdata/netdata/pull/20564) ([kanelatechnical](https://github.com/kanelatechnical))
- Mcp disclaimer update [\#20563](https://github.com/netdata/netdata/pull/20563) ([kanelatechnical](https://github.com/kanelatechnical))
- Simplify MRG loading mechanism logic [\#20562](https://github.com/netdata/netdata/pull/20562) ([stelfrag](https://github.com/stelfrag))
- SNMP: cradlepoint profile [\#20561](https://github.com/netdata/netdata/pull/20561) ([Ancairon](https://github.com/Ancairon))
- Additional checks for valid db during db\_execute [\#20560](https://github.com/netdata/netdata/pull/20560) ([stelfrag](https://github.com/stelfrag))
- Improve SQLite library shutdown handling and initialization state [\#20559](https://github.com/netdata/netdata/pull/20559) ([stelfrag](https://github.com/stelfrag))
- Add CLI command to schedule update information [\#20558](https://github.com/netdata/netdata/pull/20558) ([stelfrag](https://github.com/stelfrag))
- SNMP: chrysalis profiles [\#20557](https://github.com/netdata/netdata/pull/20557) ([Ancairon](https://github.com/Ancairon))
- SNMP: checkpoint profiles [\#20556](https://github.com/netdata/netdata/pull/20556) ([Ancairon](https://github.com/Ancairon))
- Check that there is a valid thread when performing ACLK sync shutdown [\#20555](https://github.com/netdata/netdata/pull/20555) ([stelfrag](https://github.com/stelfrag))
- SNMP: Chatsworth profile [\#20554](https://github.com/netdata/netdata/pull/20554) ([Ancairon](https://github.com/Ancairon))
- Fix save alert config transition on shutdown [\#20553](https://github.com/netdata/netdata/pull/20553) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#20552](https://github.com/netdata/netdata/pull/20552) ([netdatabot](https://github.com/netdatabot))
- MSI parameter [\#20550](https://github.com/netdata/netdata/pull/20550) ([thiagoftsm](https://github.com/thiagoftsm))
- Add Remove Node guide [\#20549](https://github.com/netdata/netdata/pull/20549) ([kanelatechnical](https://github.com/kanelatechnical))
- SNMP: brother profile [\#20548](https://github.com/netdata/netdata/pull/20548) ([Ancairon](https://github.com/Ancairon))
- improve\(go.d/snmp-profiles\): add DHCP tags transform to bluecat profile [\#20547](https://github.com/netdata/netdata/pull/20547) ([ilyam8](https://github.com/ilyam8))
- SNMP: brocade profiles [\#20546](https://github.com/netdata/netdata/pull/20546) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/prometheus/common from 0.64.0 to 0.65.0 in /src/go [\#20545](https://github.com/netdata/netdata/pull/20545) ([dependabot[bot]](https://github.com/apps/dependabot))
- refactor\(go.d/ddsnmpcollector\): restructure into components [\#20543](https://github.com/netdata/netdata/pull/20543) ([ilyam8](https://github.com/ilyam8))
- Properly parse disconnect reason [\#20540](https://github.com/netdata/netdata/pull/20540) ([stelfrag](https://github.com/stelfrag))
- Update SQLITE to version 3.50.1 [\#20539](https://github.com/netdata/netdata/pull/20539) ([stelfrag](https://github.com/stelfrag))
- SNMP: bluecat profile [\#20538](https://github.com/netdata/netdata/pull/20538) ([Ancairon](https://github.com/Ancairon))
- SNMP: barracuda Profiles [\#20537](https://github.com/netdata/netdata/pull/20537) ([Ancairon](https://github.com/Ancairon))
- Lock before checking the statement pool [\#20536](https://github.com/netdata/netdata/pull/20536) ([stelfrag](https://github.com/stelfrag))
- SNMP: avtech Profiles [\#20535](https://github.com/netdata/netdata/pull/20535) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump k8s.io/client-go from 0.33.1 to 0.33.2 in /src/go [\#20532](https://github.com/netdata/netdata/pull/20532) ([dependabot[bot]](https://github.com/apps/dependabot))
- improve\(go.d/snmp\): dd support for non-identifying tags in table metrics [\#20530](https://github.com/netdata/netdata/pull/20530) ([ilyam8](https://github.com/ilyam8))
- Mcp5 [\#20529](https://github.com/netdata/netdata/pull/20529) ([ktsaou](https://github.com/ktsaou))
- improve\(go.d/snmp\): add Go template-based metric transformations for SNMP profiles [\#20528](https://github.com/netdata/netdata/pull/20528) ([ilyam8](https://github.com/ilyam8))
- SNMP: avocent profile [\#20527](https://github.com/netdata/netdata/pull/20527) ([Ancairon](https://github.com/Ancairon))
- improve\(go.d/snmp-profiles\): allow users to add custom SNMP profiles [\#20526](https://github.com/netdata/netdata/pull/20526) ([ilyam8](https://github.com/ilyam8))
- SNMP: avaya profiles [\#20525](https://github.com/netdata/netdata/pull/20525) ([Ancairon](https://github.com/Ancairon))
- improve\(go.d/snmp\): log device profiles matched by sysObjectID [\#20524](https://github.com/netdata/netdata/pull/20524) ([ilyam8](https://github.com/ilyam8))
- update units in \_generic-if.yaml [\#20523](https://github.com/netdata/netdata/pull/20523) ([ilyam8](https://github.com/ilyam8))
- Hardware \(Windows.plugin\) [\#20522](https://github.com/netdata/netdata/pull/20522) ([thiagoftsm](https://github.com/thiagoftsm))
- upd generic check in snmp prof metrics deduplication [\#20521](https://github.com/netdata/netdata/pull/20521) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp-profiles\): metrics deduplication [\#20520](https://github.com/netdata/netdata/pull/20520) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp-profiles\): remove unsupported constant\_value\_one metrics [\#20519](https://github.com/netdata/netdata/pull/20519) ([ilyam8](https://github.com/ilyam8))
- Drop POWER8+ builds. [\#20518](https://github.com/netdata/netdata/pull/20518) ([Ferroin](https://github.com/Ferroin))
- fix\(go.d/ddsnmp\): remove singular-to-plural conversion in metric family [\#20517](https://github.com/netdata/netdata/pull/20517) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp-profiles\): Add hrSystemUptime metric [\#20516](https://github.com/netdata/netdata/pull/20516) ([ilyam8](https://github.com/ilyam8))
- Update mcp.md [\#20515](https://github.com/netdata/netdata/pull/20515) ([Ancairon](https://github.com/Ancairon))
- Update machine-learning-and-assisted-troubleshooting.md [\#20514](https://github.com/netdata/netdata/pull/20514) ([kanelatechnical](https://github.com/kanelatechnical))
- docs: add Netdata MCP Server preview announcement [\#20513](https://github.com/netdata/netdata/pull/20513) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): add SNMP- prefix for vnode hostname [\#20512](https://github.com/netdata/netdata/pull/20512) ([ilyam8](https://github.com/ilyam8))
- Cleanup pending statements during shutdown [\#20511](https://github.com/netdata/netdata/pull/20511) ([stelfrag](https://github.com/stelfrag))
- test\(go.d/ddsnmp\): add more tests for table metrics [\#20510](https://github.com/netdata/netdata/pull/20510) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): fix table collection with caching [\#20509](https://github.com/netdata/netdata/pull/20509) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/snmp profile\): add fallback support for duplicate metric tags [\#20508](https://github.com/netdata/netdata/pull/20508) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/snmp profile\): add sensors to mikrotik-router.yaml [\#20507](https://github.com/netdata/netdata/pull/20507) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20506](https://github.com/netdata/netdata/pull/20506) ([netdatabot](https://github.com/netdatabot))
- improve\(go.d/snmp profiles\): simplify \_generic-if.yaml and add interface type tags [\#20505](https://github.com/netdata/netdata/pull/20505) ([ilyam8](https://github.com/ilyam8))
- fix snmp prof mikrotik mem tagging [\#20504](https://github.com/netdata/netdata/pull/20504) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/ddsnmp\): make SNMP profile collection configurable [\#20503](https://github.com/netdata/netdata/pull/20503) ([ilyam8](https://github.com/ilyam8))
- Use ARAL for labels [\#20502](https://github.com/netdata/netdata/pull/20502) ([stelfrag](https://github.com/stelfrag))
- SNMP: audiocodes profile [\#20501](https://github.com/netdata/netdata/pull/20501) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/ddsnmp\): better label values sanitization [\#20500](https://github.com/netdata/netdata/pull/20500) ([ilyam8](https://github.com/ilyam8))
- SNMP: second pass of aruba profiles [\#20499](https://github.com/netdata/netdata/pull/20499) ([Ancairon](https://github.com/Ancairon))
- SNMP: Arista profiles [\#20498](https://github.com/netdata/netdata/pull/20498) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/ddsnmp\): fix table metrics again [\#20497](https://github.com/netdata/netdata/pull/20497) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20496](https://github.com/netdata/netdata/pull/20496) ([netdatabot](https://github.com/netdatabot))
- fix: mark import groups as not supported SCIM feature [\#20495](https://github.com/netdata/netdata/pull/20495) ([juacker](https://github.com/juacker))
- chore\(go.d/ddsnmp\): fix table metrics collection [\#20492](https://github.com/netdata/netdata/pull/20492) ([ilyam8](https://github.com/ilyam8))
- SNMP: APC profiles [\#20491](https://github.com/netdata/netdata/pull/20491) ([Ancairon](https://github.com/Ancairon))
- fix fluentd schema permit\_plugin [\#20490](https://github.com/netdata/netdata/pull/20490) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): add missing props to config schemas [\#20489](https://github.com/netdata/netdata/pull/20489) ([ilyam8](https://github.com/ilyam8))
- anue [\#20488](https://github.com/netdata/netdata/pull/20488) ([Ancairon](https://github.com/Ancairon))
- SNMP: Alcatel profiles [\#20487](https://github.com/netdata/netdata/pull/20487) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/go-sql-driver/mysql from 1.9.2 to 1.9.3 in /src/go [\#20483](https://github.com/netdata/netdata/pull/20483) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore\(go.d/ddsnmp\):  add index-based tags and cross-table index transformation support [\#20482](https://github.com/netdata/netdata/pull/20482) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): collect cross-table metrics and tags [\#20481](https://github.com/netdata/netdata/pull/20481) ([ilyam8](https://github.com/ilyam8))
- Correctly ignore patches that are already applied. [\#20480](https://github.com/netdata/netdata/pull/20480) ([Ferroin](https://github.com/Ferroin))
- chore\(go.d/ddsnmp\): split table collection into walk and process phases [\#20479](https://github.com/netdata/netdata/pull/20479) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/redis\): don't clear tls for `rediss` [\#20478](https://github.com/netdata/netdata/pull/20478) ([ilyam8](https://github.com/ilyam8))
- Enable Rust-based journal file reader in static builds. [\#20477](https://github.com/netdata/netdata/pull/20477) ([Ferroin](https://github.com/Ferroin))
- improvement\(go.d\): add bearer\_token\_file to request cfg [\#20476](https://github.com/netdata/netdata/pull/20476) ([ilyam8](https://github.com/ilyam8))
- Update mcp.md [\#20475](https://github.com/netdata/netdata/pull/20475) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): add dependency-based expiration to table cache [\#20474](https://github.com/netdata/netdata/pull/20474) ([ilyam8](https://github.com/ilyam8))
- SNMP: a10 yamls [\#20472](https://github.com/netdata/netdata/pull/20472) ([Ancairon](https://github.com/Ancairon))
- improvement\(go.d/snmp\): create table charts [\#20471](https://github.com/netdata/netdata/pull/20471) ([ilyam8](https://github.com/ilyam8))
- Remove static build timeouts from regular builds. [\#20470](https://github.com/netdata/netdata/pull/20470) ([Ferroin](https://github.com/Ferroin))
- Add MCP documentation [\#20469](https://github.com/netdata/netdata/pull/20469) ([kanelatechnical](https://github.com/kanelatechnical))
- SNMP: 3com profiles [\#20468](https://github.com/netdata/netdata/pull/20468) ([Ancairon](https://github.com/Ancairon))
- Modify Uninstall Action \(windows.installer\) [\#20467](https://github.com/netdata/netdata/pull/20467) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations docs [\#20466](https://github.com/netdata/netdata/pull/20466) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/ddsnmp\): add table metrics and tags caching optimization [\#20465](https://github.com/netdata/netdata/pull/20465) ([ilyam8](https://github.com/ilyam8))
- Improve datafile rotation and indexing during shutdown [\#20464](https://github.com/netdata/netdata/pull/20464) ([stelfrag](https://github.com/stelfrag))
- improvement\(go.d/ddsnmp\): add table metrics, tags from the same table [\#20463](https://github.com/netdata/netdata/pull/20463) ([ilyam8](https://github.com/ilyam8))
- Handle orphan journal files by deleting unmatched entries [\#20462](https://github.com/netdata/netdata/pull/20462) ([stelfrag](https://github.com/stelfrag))
- build: update otel-collector deps [\#20461](https://github.com/netdata/netdata/pull/20461) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/smartctl\): debug log exec output [\#20460](https://github.com/netdata/netdata/pull/20460) ([ilyam8](https://github.com/ilyam8))
- improve database indexing and rotation handling in event loop [\#20459](https://github.com/netdata/netdata/pull/20459) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/sijms/go-ora/v2 from 2.8.24 to 2.9.0 in /src/go [\#20457](https://github.com/netdata/netdata/pull/20457) ([dependabot[bot]](https://github.com/apps/dependabot))
- improvement\(go.d/ddsnmp\): dedup metrics when merging profiles [\#20456](https://github.com/netdata/netdata/pull/20456) ([ilyam8](https://github.com/ilyam8))
- Additional checks on metasync thread shutdown [\#20455](https://github.com/netdata/netdata/pull/20455) ([stelfrag](https://github.com/stelfrag))
- Monitor Exchange Server \(Window.plugin\) [\#20454](https://github.com/netdata/netdata/pull/20454) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations docs [\#20453](https://github.com/netdata/netdata/pull/20453) ([netdatabot](https://github.com/netdatabot))
- MCP Part 4 [\#20452](https://github.com/netdata/netdata/pull/20452) ([ktsaou](https://github.com/ktsaou))
- docs: improve SCIM documentation [\#20451](https://github.com/netdata/netdata/pull/20451) ([juacker](https://github.com/juacker))
- build\(deps\): bump github.com/gosnmp/gosnmp from 1.40.0 to 1.41.0 in /src/go [\#20449](https://github.com/netdata/netdata/pull/20449) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump go.mongodb.org/mongo-driver from 1.17.3 to 1.17.4 in /src/go [\#20447](https://github.com/netdata/netdata/pull/20447) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/lmittmann/tint from 1.1.1 to 1.1.2 in /src/go [\#20446](https://github.com/netdata/netdata/pull/20446) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.9.0 to 9.10.0 in /src/go [\#20445](https://github.com/netdata/netdata/pull/20445) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/net from 0.40.0 to 0.41.0 in /src/go [\#20444](https://github.com/netdata/netdata/pull/20444) ([dependabot[bot]](https://github.com/apps/dependabot))
- Weblog collector: Exclude 429 from 4xx [\#20443](https://github.com/netdata/netdata/pull/20443) ([Slind14](https://github.com/Slind14))
- chore\(go.d/ddsnmp\): add basic SNMP table walking functionality [\#20441](https://github.com/netdata/netdata/pull/20441) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/ddsnmp\): use dev type and vendor from meta for family [\#20439](https://github.com/netdata/netdata/pull/20439) ([ilyam8](https://github.com/ilyam8))
- Fix registry save integer overflow and add failure backoff [\#20437](https://github.com/netdata/netdata/pull/20437) ([ktsaou](https://github.com/ktsaou))
- Mcp3 [\#20435](https://github.com/netdata/netdata/pull/20435) ([ktsaou](https://github.com/ktsaou))
- Adjust stream connector timeout during agent shutdown [\#20434](https://github.com/netdata/netdata/pull/20434) ([stelfrag](https://github.com/stelfrag))
- Improve statement finalization and cleanup [\#20433](https://github.com/netdata/netdata/pull/20433) ([stelfrag](https://github.com/stelfrag))
- SNMP: new version of families Cisco pass [\#20432](https://github.com/netdata/netdata/pull/20432) ([Ancairon](https://github.com/Ancairon))
- Fix heap-use-after-free in query progress updates [\#20431](https://github.com/netdata/netdata/pull/20431) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#20430](https://github.com/netdata/netdata/pull/20430) ([netdatabot](https://github.com/netdatabot))
- Update MSSQL Metadata [\#20429](https://github.com/netdata/netdata/pull/20429) ([thiagoftsm](https://github.com/thiagoftsm))
- update ddsnmp mikrotik-router.yaml [\#20428](https://github.com/netdata/netdata/pull/20428) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): lazy ddsnmp profile loading [\#20427](https://github.com/netdata/netdata/pull/20427) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/snmp\): enable profile scalar metrics collection [\#20426](https://github.com/netdata/netdata/pull/20426) ([ilyam8](https://github.com/ilyam8))
- ML: Add documentation for Netdata Insights [\#20425](https://github.com/netdata/netdata/pull/20425) ([kanelatechnical](https://github.com/kanelatechnical))
- docs: remove sizing-netdata-parents.md [\#20421](https://github.com/netdata/netdata/pull/20421) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/ddsnmp\): correctly handle all mapping types [\#20420](https://github.com/netdata/netdata/pull/20420) ([ilyam8](https://github.com/ilyam8))
- SNMP: apc\_ups.yaml [\#20419](https://github.com/netdata/netdata/pull/20419) ([Ancairon](https://github.com/Ancairon))
- update\_installer: Update remove instruction [\#20418](https://github.com/netdata/netdata/pull/20418) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix typo. [\#20417](https://github.com/netdata/netdata/pull/20417) ([de-authority](https://github.com/de-authority))
- Fix context updates [\#20416](https://github.com/netdata/netdata/pull/20416) ([stelfrag](https://github.com/stelfrag))
- improvement\(go.d\): add ddsnmp profile collector \(scalar only\) [\#20415](https://github.com/netdata/netdata/pull/20415) ([ilyam8](https://github.com/ilyam8))
- SNMP: juniper-pulse-secure.yaml [\#20413](https://github.com/netdata/netdata/pull/20413) ([Ancairon](https://github.com/Ancairon))
- Improve metrics centralization points documentation [\#20412](https://github.com/netdata/netdata/pull/20412) ([kanelatechnical](https://github.com/kanelatechnical))
- SNMP: \_juniper-virtualchassis.yaml [\#20410](https://github.com/netdata/netdata/pull/20410) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_juniper-userfirewall.yaml [\#20409](https://github.com/netdata/netdata/pull/20409) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_juniper-scu.yaml [\#20408](https://github.com/netdata/netdata/pull/20408) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_juniper-firewall.yaml [\#20407](https://github.com/netdata/netdata/pull/20407) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_juniper-dcu.yaml [\#20406](https://github.com/netdata/netdata/pull/20406) ([Ancairon](https://github.com/Ancairon))
- Enforce correct CPU architecture for Go plugin builds. [\#20405](https://github.com/netdata/netdata/pull/20405) ([Ferroin](https://github.com/Ferroin))
- Rename nd-mcp on windows [\#20404](https://github.com/netdata/netdata/pull/20404) ([stelfrag](https://github.com/stelfrag))
- SNMP: \_juniper-cos.yaml [\#20402](https://github.com/netdata/netdata/pull/20402) ([Ancairon](https://github.com/Ancairon))
- docs\(go.d\): add example how to debug a specific job [\#20399](https://github.com/netdata/netdata/pull/20399) ([ilyam8](https://github.com/ilyam8))
- Maintenance: update restart, backup, uninstall, and restore docs [\#20398](https://github.com/netdata/netdata/pull/20398) ([kanelatechnical](https://github.com/kanelatechnical))
- feat\(go.d\): allow to debug a specific job [\#20394](https://github.com/netdata/netdata/pull/20394) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/httpcheck\): add resp validation debug logging [\#20392](https://github.com/netdata/netdata/pull/20392) ([ilyam8](https://github.com/ilyam8))
- SNMP: palo-alto.yaml [\#20391](https://github.com/netdata/netdata/pull/20391) ([Ancairon](https://github.com/Ancairon))
- SNMP: aruba-wireless-controller.yaml [\#20389](https://github.com/netdata/netdata/pull/20389) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/docker/docker from 28.2.1+incompatible to 28.2.2+incompatible in /src/go [\#20387](https://github.com/netdata/netdata/pull/20387) ([dependabot[bot]](https://github.com/apps/dependabot))
- apps.plugin documentation and grouping matches improvements [\#20386](https://github.com/netdata/netdata/pull/20386) ([ktsaou](https://github.com/ktsaou))
- SNMP: aruba-switch.yaml [\#20385](https://github.com/netdata/netdata/pull/20385) ([Ancairon](https://github.com/Ancairon))
- Improve DynCfg documentation [\#20384](https://github.com/netdata/netdata/pull/20384) ([kanelatechnical](https://github.com/kanelatechnical))
- SNMP: aruba-cx-switch.yaml [\#20383](https://github.com/netdata/netdata/pull/20383) ([Ancairon](https://github.com/Ancairon))
- SNMP: aruba-clearpass.yaml [\#20382](https://github.com/netdata/netdata/pull/20382) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_aruba-switch-cpu-memory.yaml [\#20381](https://github.com/netdata/netdata/pull/20381) ([Ancairon](https://github.com/Ancairon))
- Update documentation [\#20380](https://github.com/netdata/netdata/pull/20380) ([thiagoftsm](https://github.com/thiagoftsm))
- test\(go.d/oracledb\): fix test [\#20378](https://github.com/netdata/netdata/pull/20378) ([ilyam8](https://github.com/ilyam8))
- SNMP: fortinet-fortiswitch.yaml [\#20377](https://github.com/netdata/netdata/pull/20377) ([Ancairon](https://github.com/Ancairon))
- chore\(otel.plugin\): add more receivers/exporter [\#20376](https://github.com/netdata/netdata/pull/20376) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20375](https://github.com/netdata/netdata/pull/20375) ([netdatabot](https://github.com/netdatabot))
- SNMP: fortinet-fortigate.yaml and remove un-needed profile [\#20374](https://github.com/netdata/netdata/pull/20374) ([Ancairon](https://github.com/Ancairon))
- fix\(go.d/oracledb\): correct tablespace usage calculation for all types [\#20373](https://github.com/netdata/netdata/pull/20373) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/redis/go-redis/v9 from 9.8.0 to 9.9.0 in /src/go [\#20372](https://github.com/netdata/netdata/pull/20372) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 28.1.1+incompatible to 28.2.1+incompatible in /src/go [\#20371](https://github.com/netdata/netdata/pull/20371) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/lmittmann/tint from 1.1.0 to 1.1.1 in /src/go [\#20370](https://github.com/netdata/netdata/pull/20370) ([dependabot[bot]](https://github.com/apps/dependabot))
- SNMP: fortinet-appliance.yaml [\#20369](https://github.com/netdata/netdata/pull/20369) ([Ancairon](https://github.com/Ancairon))
- chore\(otel.plugin\): fix building [\#20368](https://github.com/netdata/netdata/pull/20368) ([ilyam8](https://github.com/ilyam8))
- SNMP: \_fortinet-fortigate-vpn-tunnel.yaml [\#20367](https://github.com/netdata/netdata/pull/20367) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_fortinet-fortigate-cpu-memory.yaml [\#20366](https://github.com/netdata/netdata/pull/20366) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_cisco-wlc.yaml [\#20364](https://github.com/netdata/netdata/pull/20364) ([Ancairon](https://github.com/Ancairon))
- \_cisco-voice.yaml [\#20361](https://github.com/netdata/netdata/pull/20361) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d\): fix some golangcilint warning [\#20360](https://github.com/netdata/netdata/pull/20360) ([ilyam8](https://github.com/ilyam8))
- Windows updated [\#20358](https://github.com/netdata/netdata/pull/20358) ([kanelatechnical](https://github.com/kanelatechnical))
- feat\(go.d/dyncfg\): add autodetect\_retry to dyncfg jobs [\#20357](https://github.com/netdata/netdata/pull/20357) ([ilyam8](https://github.com/ilyam8))
- Improve datafile rotation and indexing [\#20354](https://github.com/netdata/netdata/pull/20354) ([stelfrag](https://github.com/stelfrag))
- SNMP: \_cisco-ipsec-flow-monitor.yaml [\#20353](https://github.com/netdata/netdata/pull/20353) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): update otel dependencies version [\#20352](https://github.com/netdata/netdata/pull/20352) ([ilyam8](https://github.com/ilyam8))
- SNMP: \_generic-ups.yaml [\#20351](https://github.com/netdata/netdata/pull/20351) ([Ancairon](https://github.com/Ancairon))
- Improve retention calculation after datafile deletion [\#20350](https://github.com/netdata/netdata/pull/20350) ([stelfrag](https://github.com/stelfrag))
- SNMP: \_generic-ucd.yaml [\#20349](https://github.com/netdata/netdata/pull/20349) ([Ancairon](https://github.com/Ancairon))
- improvement\(go.d/sd\): better prometheus exporters detection [\#20348](https://github.com/netdata/netdata/pull/20348) ([ilyam8](https://github.com/ilyam8))
- Updated configuration reference [\#20347](https://github.com/netdata/netdata/pull/20347) ([kanelatechnical](https://github.com/kanelatechnical))
- fix\(go.d/dyncfg\): fix duplicate potential "name" in userconfig action [\#20346](https://github.com/netdata/netdata/pull/20346) ([ilyam8](https://github.com/ilyam8))
- Split systemd-journal plugin and add Rust-based journal file reader [\#20345](https://github.com/netdata/netdata/pull/20345) ([vkalintiris](https://github.com/vkalintiris))
- SNMP: \_generic-sip.yaml [\#20344](https://github.com/netdata/netdata/pull/20344) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_generic-rtp.yaml [\#20343](https://github.com/netdata/netdata/pull/20343) ([Ancairon](https://github.com/Ancairon))
- SNMP: \_generic-lldp.yaml [\#20342](https://github.com/netdata/netdata/pull/20342) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/vmware/govmomi from 0.50.0 to 0.51.0 in /src/go [\#20341](https://github.com/netdata/netdata/pull/20341) ([dependabot[bot]](https://github.com/apps/dependabot))
- Switch back to epoll from poll [\#20337](https://github.com/netdata/netdata/pull/20337) ([ilyam8](https://github.com/ilyam8))
- Alerts cloud [\#20334](https://github.com/netdata/netdata/pull/20334) ([kanelatechnical](https://github.com/kanelatechnical))
- Regenerate integrations docs [\#20332](https://github.com/netdata/netdata/pull/20332) ([netdatabot](https://github.com/netdatabot))
- \_generic-ip.yaml [\#20331](https://github.com/netdata/netdata/pull/20331) ([Ancairon](https://github.com/Ancairon))
- Update SCIM documentation [\#20330](https://github.com/netdata/netdata/pull/20330) ([juacker](https://github.com/juacker))
- Update alerting and notification documentation Agent [\#20329](https://github.com/netdata/netdata/pull/20329) ([kanelatechnical](https://github.com/kanelatechnical))
- generic-bgp4.yaml [\#20328](https://github.com/netdata/netdata/pull/20328) ([Ancairon](https://github.com/Ancairon))
- generic-ospf.yaml pass [\#20327](https://github.com/netdata/netdata/pull/20327) ([Ancairon](https://github.com/Ancairon))
- generic-udp.yaml pass [\#20326](https://github.com/netdata/netdata/pull/20326) ([Ancairon](https://github.com/Ancairon))
- SOC 2 cloud doc update [\#20325](https://github.com/netdata/netdata/pull/20325) ([kanelatechnical](https://github.com/kanelatechnical))
- dont init dyncfg for vnode [\#20324](https://github.com/netdata/netdata/pull/20324) ([ilyam8](https://github.com/ilyam8))
- Code cleanup and improvements [\#20323](https://github.com/netdata/netdata/pull/20323) ([stelfrag](https://github.com/stelfrag))
- add installing flex to install-required-packages.sh [\#20322](https://github.com/netdata/netdata/pull/20322) ([ilyam8](https://github.com/ilyam8))
- \_generic-tcp.yaml pass [\#20321](https://github.com/netdata/netdata/pull/20321) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/lmittmann/tint from 1.0.7 to 1.1.0 in /src/go [\#20320](https://github.com/netdata/netdata/pull/20320) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): update otel dependencies version [\#20319](https://github.com/netdata/netdata/pull/20319) ([ilyam8](https://github.com/ilyam8))
- Cancel health initialization if shutdown has been requested [\#20318](https://github.com/netdata/netdata/pull/20318) ([stelfrag](https://github.com/stelfrag))
- SNMP: \_generic-if.yaml pass [\#20317](https://github.com/netdata/netdata/pull/20317) ([Ancairon](https://github.com/Ancairon))
- Update libbpf [\#20316](https://github.com/netdata/netdata/pull/20316) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations docs [\#20315](https://github.com/netdata/netdata/pull/20315) ([netdatabot](https://github.com/netdatabot))
- docs: fix netdata-assistant.md [\#20314](https://github.com/netdata/netdata/pull/20314) ([ilyam8](https://github.com/ilyam8))
- plugins dyncfg is always on localhost [\#20312](https://github.com/netdata/netdata/pull/20312) ([ktsaou](https://github.com/ktsaou))
- docs: fix tip in streaming readme [\#20310](https://github.com/netdata/netdata/pull/20310) ([ilyam8](https://github.com/ilyam8))
- Netdata ai [\#20309](https://github.com/netdata/netdata/pull/20309) ([kanelatechnical](https://github.com/kanelatechnical))
- Improve user transition log messages [\#20308](https://github.com/netdata/netdata/pull/20308) ([ilyam8](https://github.com/ilyam8))
- Add MSSQL Wait statistics \(windows.plugin\) [\#20307](https://github.com/netdata/netdata/pull/20307) ([thiagoftsm](https://github.com/thiagoftsm))
- Reduce memory allocations in event loops [\#20306](https://github.com/netdata/netdata/pull/20306) ([stelfrag](https://github.com/stelfrag))
- fix use after free of streaming current parent [\#20305](https://github.com/netdata/netdata/pull/20305) ([ktsaou](https://github.com/ktsaou))
- fix heap-use-after-free in plugins.d inflight functions [\#20304](https://github.com/netdata/netdata/pull/20304) ([ktsaou](https://github.com/ktsaou))
- Improve metasync shutdown [\#20303](https://github.com/netdata/netdata/pull/20303) ([stelfrag](https://github.com/stelfrag))
- docs: fix `<br>` in streaming [\#20302](https://github.com/netdata/netdata/pull/20302) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/snmp\): replace newline control chars with spaces in system info [\#20301](https://github.com/netdata/netdata/pull/20301) ([ilyam8](https://github.com/ilyam8))
- Updating SOC2 compliance status [\#20300](https://github.com/netdata/netdata/pull/20300) ([shyamvalsan](https://github.com/shyamvalsan))
- build\(deps\): bump github.com/jackc/pgx/v5 from 5.7.4 to 5.7.5 in /src/go [\#20299](https://github.com/netdata/netdata/pull/20299) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/prometheus/common from 0.63.0 to 0.64.0 in /src/go [\#20296](https://github.com/netdata/netdata/pull/20296) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump k8s.io/client-go from 0.33.0 to 0.33.1 in /src/go [\#20295](https://github.com/netdata/netdata/pull/20295) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix\(go.d\): sanitize vnode labels before creating vnode [\#20293](https://github.com/netdata/netdata/pull/20293) ([ilyam8](https://github.com/ilyam8))
- docs: Observability centralization points [\#20292](https://github.com/netdata/netdata/pull/20292) ([kanelatechnical](https://github.com/kanelatechnical))
- Cisco yaml pass [\#20291](https://github.com/netdata/netdata/pull/20291) ([Ancairon](https://github.com/Ancairon))
- Minor code adjustments [\#20290](https://github.com/netdata/netdata/pull/20290) ([stelfrag](https://github.com/stelfrag))
- Fix when docker socket group id points to an existing group in container [\#20288](https://github.com/netdata/netdata/pull/20288) ([felipecrs](https://github.com/felipecrs))
- Model Context Protocol \(MCP\) Part 2 [\#20287](https://github.com/netdata/netdata/pull/20287) ([ktsaou](https://github.com/ktsaou))
- add "unix://" scheme to DOCKER\_HOST in run.sh [\#20286](https://github.com/netdata/netdata/pull/20286) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#20284](https://github.com/netdata/netdata/pull/20284) ([netdatabot](https://github.com/netdatabot))
- Improved StatsD documentation [\#20282](https://github.com/netdata/netdata/pull/20282) ([kanelatechnical](https://github.com/kanelatechnical))
- Improve agent shutdown [\#20280](https://github.com/netdata/netdata/pull/20280) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#20279](https://github.com/netdata/netdata/pull/20279) ([netdatabot](https://github.com/netdatabot))
- docs: update mssql meta [\#20278](https://github.com/netdata/netdata/pull/20278) ([ilyam8](https://github.com/ilyam8))
- New Windows Metrics \(CPU and Memory\) [\#20277](https://github.com/netdata/netdata/pull/20277) ([thiagoftsm](https://github.com/thiagoftsm))
- chore\(go.d/snmp\): small cleanup snmp profiles code [\#20274](https://github.com/netdata/netdata/pull/20274) ([ilyam8](https://github.com/ilyam8))
- Switch to poll from epoll [\#20273](https://github.com/netdata/netdata/pull/20273) ([stelfrag](https://github.com/stelfrag))
- comment metric tags that could be metrics [\#20272](https://github.com/netdata/netdata/pull/20272) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump golang.org/x/net from 0.39.0 to 0.40.0 in /src/go [\#20270](https://github.com/netdata/netdata/pull/20270) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/miekg/dns from 1.1.65 to 1.1.66 in /src/go [\#20268](https://github.com/netdata/netdata/pull/20268) ([dependabot[bot]](https://github.com/apps/dependabot))
- Update Netdata README with improved structure [\#20265](https://github.com/netdata/netdata/pull/20265) ([kanelatechnical](https://github.com/kanelatechnical))
- Schedule journal file indexing after database file rotation [\#20264](https://github.com/netdata/netdata/pull/20264) ([stelfrag](https://github.com/stelfrag))
- Minor fixes [\#20263](https://github.com/netdata/netdata/pull/20263) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d/mysql\): fix MariaDB User CPU Time [\#20262](https://github.com/netdata/netdata/pull/20262) ([ilyam8](https://github.com/ilyam8))
- docs: reword go.d Troubleshooting section for clarity [\#20259](https://github.com/netdata/netdata/pull/20259) ([ilyam8](https://github.com/ilyam8))
- Clearify the path of `plugins.d/go.d.plugin` in docs [\#20258](https://github.com/netdata/netdata/pull/20258) ([n0099](https://github.com/n0099))
- Update documentation for native DEB/RPM packages [\#20257](https://github.com/netdata/netdata/pull/20257) ([kanelatechnical](https://github.com/kanelatechnical))
- fix\(go.d/sd/snmp\): fix snmnpv3 again [\#20256](https://github.com/netdata/netdata/pull/20256) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d/snmp\): make enable\_profiles configurable \(needed for dev\) [\#20255](https://github.com/netdata/netdata/pull/20255) ([ilyam8](https://github.com/ilyam8))
- fix obsolete chart cleanup to properly handle vnodes [\#20254](https://github.com/netdata/netdata/pull/20254) ([ilyam8](https://github.com/ilyam8))
- docs: fix license link and remove GH alerts syntax from FAQ [\#20252](https://github.com/netdata/netdata/pull/20252) ([ilyam8](https://github.com/ilyam8))
- Update Netdata README [\#20251](https://github.com/netdata/netdata/pull/20251) ([kanelatechnical](https://github.com/kanelatechnical))
- Switch to uv threads [\#20250](https://github.com/netdata/netdata/pull/20250) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d/snmp\): use 32bit counters if 64 aren't available [\#20249](https://github.com/netdata/netdata/pull/20249) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/snmp\): use ifDescr for interface name if ifName is empty [\#20248](https://github.com/netdata/netdata/pull/20248) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/sd/snmp\): fix snmpv3 credentials [\#20247](https://github.com/netdata/netdata/pull/20247) ([ilyam8](https://github.com/ilyam8))
- SNMP first cisco yaml file pass [\#20246](https://github.com/netdata/netdata/pull/20246) ([Ancairon](https://github.com/Ancairon))
- IIS W3SCV W3MP Metrics \(windows.plugin\) [\#20245](https://github.com/netdata/netdata/pull/20245) ([thiagoftsm](https://github.com/thiagoftsm))
- Model Context Protocol Server \(MCP\) for Netdata Part 1 [\#20244](https://github.com/netdata/netdata/pull/20244) ([ktsaou](https://github.com/ktsaou))
- Fix build issue on old distros [\#20243](https://github.com/netdata/netdata/pull/20243) ([stelfrag](https://github.com/stelfrag))
- Session claim id in docker [\#20240](https://github.com/netdata/netdata/pull/20240) ([stelfrag](https://github.com/stelfrag))
- Let the user override the default stack size [\#20236](https://github.com/netdata/netdata/pull/20236) ([stelfrag](https://github.com/stelfrag))
- Revert "Revert "fix\(go.d/couchdb\): correct db size charts unit"" [\#20235](https://github.com/netdata/netdata/pull/20235) ([ilyam8](https://github.com/ilyam8))
- Improve MSSQL \(Part III\) [\#20230](https://github.com/netdata/netdata/pull/20230) ([thiagoftsm](https://github.com/thiagoftsm))
- Make all threads joinable and join on agent shutdown [\#20228](https://github.com/netdata/netdata/pull/20228) ([stelfrag](https://github.com/stelfrag))
- ci: ignore changes in src/go/otel-collector/release-config.yaml.in [\#20222](https://github.com/netdata/netdata/pull/20222) ([ilyam8](https://github.com/ilyam8))

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

**Merged pull requests:**

- Revert "fix\(go.d/couchdb\): correct db size charts unit" [\#20234](https://github.com/netdata/netdata/pull/20234) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/couchdb\): correct db size charts unit [\#20233](https://github.com/netdata/netdata/pull/20233) ([ilyam8](https://github.com/ilyam8))
- docs: rename DynCfg developer doc to avoid title conflict [\#20232](https://github.com/netdata/netdata/pull/20232) ([ilyam8](https://github.com/ilyam8))
- status file 28 [\#20229](https://github.com/netdata/netdata/pull/20229) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#20227](https://github.com/netdata/netdata/pull/20227) ([netdatabot](https://github.com/netdatabot))
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
