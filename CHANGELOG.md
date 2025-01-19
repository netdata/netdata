# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.1.1...HEAD)

**Merged pull requests:**

- add more aclk worker jobs [\#19435](https://github.com/netdata/netdata/pull/19435) ([ktsaou](https://github.com/ktsaou))
- fix go.d/ethtool config schema [\#19434](https://github.com/netdata/netdata/pull/19434) ([ilyam8](https://github.com/ilyam8))
- Cleanup context check list on startup [\#19433](https://github.com/netdata/netdata/pull/19433) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations docs [\#19432](https://github.com/netdata/netdata/pull/19432) ([netdatabot](https://github.com/netdatabot))
- Drop Fedora 39 from CI and package builds. [\#19431](https://github.com/netdata/netdata/pull/19431) ([Ferroin](https://github.com/Ferroin))
- docs: fix go.d/ethtool meta [\#19430](https://github.com/netdata/netdata/pull/19430) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ethtool\): use ndsudo for module info [\#19429](https://github.com/netdata/netdata/pull/19429) ([ilyam8](https://github.com/ilyam8))
- add 'ethtool -m' to ndsudo [\#19428](https://github.com/netdata/netdata/pull/19428) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/ethtool\): collect module ddm info using ethtool [\#19426](https://github.com/netdata/netdata/pull/19426) ([ilyam8](https://github.com/ilyam8))
- ACLK timeout [\#19425](https://github.com/netdata/netdata/pull/19425) ([ktsaou](https://github.com/ktsaou))
- log stream\_info payload when it cannot be parsed [\#19424](https://github.com/netdata/netdata/pull/19424) ([ktsaou](https://github.com/ktsaou))
- Fix coverity issues [\#19422](https://github.com/netdata/netdata/pull/19422) ([stelfrag](https://github.com/stelfrag))
- add 'type' to GH report forms [\#19421](https://github.com/netdata/netdata/pull/19421) ([ilyam8](https://github.com/ilyam8))
- fix mmaps accounting [\#19420](https://github.com/netdata/netdata/pull/19420) ([ktsaou](https://github.com/ktsaou))
- PULSE: network traffic [\#19419](https://github.com/netdata/netdata/pull/19419) ([ktsaou](https://github.com/ktsaou))
- hostnames: convert to utf8 and santitize [\#19418](https://github.com/netdata/netdata/pull/19418) ([ktsaou](https://github.com/ktsaou))
- cleanup contexts during loading [\#19416](https://github.com/netdata/netdata/pull/19416) ([ktsaou](https://github.com/ktsaou))
- packaging\(windows\): use local copy of GPL-3 [\#19414](https://github.com/netdata/netdata/pull/19414) ([ilyam8](https://github.com/ilyam8))
- add "netdata-" prefix to streaming and metrics-cardinality functions [\#19413](https://github.com/netdata/netdata/pull/19413) ([ilyam8](https://github.com/ilyam8))
- REFCOUNT: use only compare-and-exchange [\#19411](https://github.com/netdata/netdata/pull/19411) ([ktsaou](https://github.com/ktsaou))
- Alert prototypes: use r/w spinlock instead of spinlock [\#19410](https://github.com/netdata/netdata/pull/19410) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/docker/docker from 27.4.1+incompatible to 27.5.0+incompatible in /src/go [\#19408](https://github.com/netdata/netdata/pull/19408) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/bmatcuk/doublestar/v4 from 4.7.1 to 4.8.0 in /src/go [\#19407](https://github.com/netdata/netdata/pull/19407) ([dependabot[bot]](https://github.com/apps/dependabot))
- fixed http clients accounting [\#19406](https://github.com/netdata/netdata/pull/19406) ([ktsaou](https://github.com/ktsaou))
- RRD files split, renames, cleanup Part 2 [\#19405](https://github.com/netdata/netdata/pull/19405) ([ktsaou](https://github.com/ktsaou))
- fix loading contexts [\#19404](https://github.com/netdata/netdata/pull/19404) ([ktsaou](https://github.com/ktsaou))
- Delay context cleanup checks after startup [\#19403](https://github.com/netdata/netdata/pull/19403) ([stelfrag](https://github.com/stelfrag))
- system memory calculation for cgroups v1 fix [\#19402](https://github.com/netdata/netdata/pull/19402) ([ktsaou](https://github.com/ktsaou))
- do not process contexts before they are loaded [\#19401](https://github.com/netdata/netdata/pull/19401) ([ktsaou](https://github.com/ktsaou))
- Revert "Update kickstart script to use new repository host." [\#19400](https://github.com/netdata/netdata/pull/19400) ([Ferroin](https://github.com/Ferroin))
- split rrdhost/rrdset/rrddim and rrd.h [\#19399](https://github.com/netdata/netdata/pull/19399) ([ktsaou](https://github.com/ktsaou))
- fix nodes staying in initializing status [\#19398](https://github.com/netdata/netdata/pull/19398) ([ktsaou](https://github.com/ktsaou))
- Use worker when dispatching alert transitions to the cloud [\#19397](https://github.com/netdata/netdata/pull/19397) ([stelfrag](https://github.com/stelfrag))
- Unified memory API [\#19396](https://github.com/netdata/netdata/pull/19396) ([ktsaou](https://github.com/ktsaou))
- build\(go.d\): switch to gohugoio/hashstructure [\#19395](https://github.com/netdata/netdata/pull/19395) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19394](https://github.com/netdata/netdata/pull/19394) ([netdatabot](https://github.com/netdatabot))
- Make libunwind opt-in at build time instead of auto-enabled. [\#19393](https://github.com/netdata/netdata/pull/19393) ([Ferroin](https://github.com/Ferroin))
- Remove openSUSE 15.5 from CI and package builds. [\#19392](https://github.com/netdata/netdata/pull/19392) ([Ferroin](https://github.com/Ferroin))
- Reduce glibc fragmentation Part 2 [\#19390](https://github.com/netdata/netdata/pull/19390) ([ktsaou](https://github.com/ktsaou))
- Verify and cleanup deleted contexts [\#19389](https://github.com/netdata/netdata/pull/19389) ([stelfrag](https://github.com/stelfrag))
- Reduce glibc memory fragmentation [\#19385](https://github.com/netdata/netdata/pull/19385) ([ktsaou](https://github.com/ktsaou))
- added mmap count charts [\#19384](https://github.com/netdata/netdata/pull/19384) ([ktsaou](https://github.com/ktsaou))
- used\_arena should exclude unused memory [\#19382](https://github.com/netdata/netdata/pull/19382) ([ktsaou](https://github.com/ktsaou))
- fix mallinfo2 [\#19381](https://github.com/netdata/netdata/pull/19381) ([ktsaou](https://github.com/ktsaou))
- limit the glibc unused memory [\#19380](https://github.com/netdata/netdata/pull/19380) ([ktsaou](https://github.com/ktsaou))
- Pulse extended memory statistics, now report glibc allocations [\#19379](https://github.com/netdata/netdata/pull/19379) ([ktsaou](https://github.com/ktsaou))
- build\(deps\): bump github.com/axiomhq/hyperloglog from 0.2.2 to 0.2.3 in /src/go [\#19378](https://github.com/netdata/netdata/pull/19378) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump go.mongodb.org/mongo-driver from 1.17.1 to 1.17.2 in /src/go [\#19377](https://github.com/netdata/netdata/pull/19377) ([dependabot[bot]](https://github.com/apps/dependabot))
- ARAL: fast path to quickly allocate elements on a new page [\#19376](https://github.com/netdata/netdata/pull/19376) ([ktsaou](https://github.com/ktsaou))
- disable libunwind on forked children [\#19374](https://github.com/netdata/netdata/pull/19374) ([ktsaou](https://github.com/ktsaou))
- Fix alert entry traversal when doing cleanup [\#19373](https://github.com/netdata/netdata/pull/19373) ([stelfrag](https://github.com/stelfrag))
- Fix issues with $PATH and netdatacli detection. [\#19371](https://github.com/netdata/netdata/pull/19371) ([Ferroin](https://github.com/Ferroin))
- fix for PGC wanted\_cache\_size getting to zero [\#19370](https://github.com/netdata/netdata/pull/19370) ([ktsaou](https://github.com/ktsaou))
- metrics cardinality - more statistics and groupings [\#19368](https://github.com/netdata/netdata/pull/19368) ([ktsaou](https://github.com/ktsaou))
- stream-thread fix memory corruption [\#19367](https://github.com/netdata/netdata/pull/19367) ([ktsaou](https://github.com/ktsaou))
- metrics cardinality improvements [\#19366](https://github.com/netdata/netdata/pull/19366) ([ktsaou](https://github.com/ktsaou))
- prevent memory corruption in dbengine [\#19365](https://github.com/netdata/netdata/pull/19365) ([ktsaou](https://github.com/ktsaou))
- Revert "prevent memory corruption in dbengine" [\#19364](https://github.com/netdata/netdata/pull/19364) ([ktsaou](https://github.com/ktsaou))
- prevent memory corruption in dbengine [\#19363](https://github.com/netdata/netdata/pull/19363) ([ktsaou](https://github.com/ktsaou))
- metrics-cardinality function [\#19362](https://github.com/netdata/netdata/pull/19362) ([ktsaou](https://github.com/ktsaou))
- avoid checking replication status all the time [\#19361](https://github.com/netdata/netdata/pull/19361) ([ktsaou](https://github.com/ktsaou))
- respect flood protection configuration for daemon [\#19360](https://github.com/netdata/netdata/pull/19360) ([ktsaou](https://github.com/ktsaou))
- fix os\_system\_memory\(\) for concurrent use and call it from pulse [\#19359](https://github.com/netdata/netdata/pull/19359) ([ktsaou](https://github.com/ktsaou))
- fix flood protection [\#19358](https://github.com/netdata/netdata/pull/19358) ([ktsaou](https://github.com/ktsaou))
- allow compiling with FSANITIZE\_ADDRESS [\#19357](https://github.com/netdata/netdata/pull/19357) ([ktsaou](https://github.com/ktsaou))
- Check cluster centers size in copy constructor of inlined kmeans [\#19356](https://github.com/netdata/netdata/pull/19356) ([vkalintiris](https://github.com/vkalintiris))
- Stream Compression Fix [\#19355](https://github.com/netdata/netdata/pull/19355) ([ktsaou](https://github.com/ktsaou))
- fix compilation on windows [\#19354](https://github.com/netdata/netdata/pull/19354) ([ktsaou](https://github.com/ktsaou))
- Minor fixes [\#19353](https://github.com/netdata/netdata/pull/19353) ([ktsaou](https://github.com/ktsaou))
- Stream receiver/sender compress BEGIN-SET-END performance [\#19352](https://github.com/netdata/netdata/pull/19352) ([ktsaou](https://github.com/ktsaou))
- RRDCONTEXTS: loading report [\#19351](https://github.com/netdata/netdata/pull/19351) ([ktsaou](https://github.com/ktsaou))
- lower compression level to lower cpu resources on parents [\#19350](https://github.com/netdata/netdata/pull/19350) ([ktsaou](https://github.com/ktsaou))
- PGC wanted size [\#19349](https://github.com/netdata/netdata/pull/19349) ([ktsaou](https://github.com/ktsaou))
- log a summary of metadata ignored contexts [\#19348](https://github.com/netdata/netdata/pull/19348) ([ktsaou](https://github.com/ktsaou))
- use sqlite3\_status64\(\) [\#19347](https://github.com/netdata/netdata/pull/19347) ([ktsaou](https://github.com/ktsaou))
- Query systemd for unit file paths on install/uninstall. [\#19346](https://github.com/netdata/netdata/pull/19346) ([Ferroin](https://github.com/Ferroin))
- Assorted systemd detection fixes [\#19345](https://github.com/netdata/netdata/pull/19345) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19344](https://github.com/netdata/netdata/pull/19344) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/k8sstate\): respect ignore annotation [\#19342](https://github.com/netdata/netdata/pull/19342) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/docker\): respect ignore label [\#19341](https://github.com/netdata/netdata/pull/19341) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19340](https://github.com/netdata/netdata/pull/19340) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/docker\): fix syntax error in meta [\#19339](https://github.com/netdata/netdata/pull/19339) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d/docker\): add option to filter containers [\#19337](https://github.com/netdata/netdata/pull/19337) ([ilyam8](https://github.com/ilyam8))
- Contexts Loading [\#19336](https://github.com/netdata/netdata/pull/19336) ([ktsaou](https://github.com/ktsaou))
- Add alert version to aclk-state [\#19335](https://github.com/netdata/netdata/pull/19335) ([stelfrag](https://github.com/stelfrag))
- annotate logs with stack trace when libunwind is available [\#19334](https://github.com/netdata/netdata/pull/19334) ([ktsaou](https://github.com/ktsaou))
- convert invalid utf8 sequences to hex characters [\#19333](https://github.com/netdata/netdata/pull/19333) ([ktsaou](https://github.com/ktsaou))
- Abort on fatal and report system available bytes on allocation failures. [\#19332](https://github.com/netdata/netdata/pull/19332) ([vkalintiris](https://github.com/vkalintiris))
- Add instructions for Docker Compose [\#19331](https://github.com/netdata/netdata/pull/19331) ([enoch85](https://github.com/enoch85))
- build\(deps\): bump golang.org/x/net from 0.33.0 to 0.34.0 in /src/go [\#19330](https://github.com/netdata/netdata/pull/19330) ([dependabot[bot]](https://github.com/apps/dependabot))
- FD Leaks Fix [\#19327](https://github.com/netdata/netdata/pull/19327) ([ktsaou](https://github.com/ktsaou))
- feat\(go.d.plugin\): add YugabyteDB collector [\#19325](https://github.com/netdata/netdata/pull/19325) ([ilyam8](https://github.com/ilyam8))
- fix\(kickstart.sh\): correct wrong function name in perpare\_offline\_install [\#19323](https://github.com/netdata/netdata/pull/19323) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/vmware/govmomi from 0.46.3 to 0.47.0 in /src/go [\#19322](https://github.com/netdata/netdata/pull/19322) ([dependabot[bot]](https://github.com/apps/dependabot))
- Improve context load time during startup [\#19321](https://github.com/netdata/netdata/pull/19321) ([stelfrag](https://github.com/stelfrag))
- fix\(cgroup-rename\): prevent leading comma in Docker LABELS when IMAGE empty [\#19318](https://github.com/netdata/netdata/pull/19318) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#19317](https://github.com/netdata/netdata/pull/19317) ([stelfrag](https://github.com/stelfrag))
- CGROUP labels [\#19316](https://github.com/netdata/netdata/pull/19316) ([ktsaou](https://github.com/ktsaou))
- feat\(cgroup-name.sh\): Add support for `netdata.cloud/*` container labels [\#19315](https://github.com/netdata/netdata/pull/19315) ([ilyam8](https://github.com/ilyam8))
- Locks Improvements [\#19314](https://github.com/netdata/netdata/pull/19314) ([ktsaou](https://github.com/ktsaou))
- add yugabytedb docker manager [\#19313](https://github.com/netdata/netdata/pull/19313) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/sd\): correctly adding tags in classify [\#19312](https://github.com/netdata/netdata/pull/19312) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/nats\): add missing cid label to gw charts [\#19311](https://github.com/netdata/netdata/pull/19311) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19310](https://github.com/netdata/netdata/pull/19310) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/nats\): add missing labels to meta [\#19309](https://github.com/netdata/netdata/pull/19309) ([ilyam8](https://github.com/ilyam8))
- fix aral memory accounting [\#19308](https://github.com/netdata/netdata/pull/19308) ([ktsaou](https://github.com/ktsaou))
- UUIDMap [\#19307](https://github.com/netdata/netdata/pull/19307) ([ktsaou](https://github.com/ktsaou))
- Fix shutdown [\#19306](https://github.com/netdata/netdata/pull/19306) ([ktsaou](https://github.com/ktsaou))
- WAITQ: fixed mixed up ordering [\#19305](https://github.com/netdata/netdata/pull/19305) ([ktsaou](https://github.com/ktsaou))
- load rrdcontext dimensions in batches [\#19304](https://github.com/netdata/netdata/pull/19304) ([ktsaou](https://github.com/ktsaou))
- improvement\(go.d/nats\): add cluster\_name label and jetstream status chart [\#19303](https://github.com/netdata/netdata/pull/19303) ([ilyam8](https://github.com/ilyam8))
- Waiting Queue [\#19302](https://github.com/netdata/netdata/pull/19302) ([ktsaou](https://github.com/ktsaou))
- revert waiting-queue optimization [\#19301](https://github.com/netdata/netdata/pull/19301) ([ktsaou](https://github.com/ktsaou))
- Improve stream sending thread error message [\#19300](https://github.com/netdata/netdata/pull/19300) ([ilyam8](https://github.com/ilyam8))
- Streaming improvements No 12 [\#19299](https://github.com/netdata/netdata/pull/19299) ([ktsaou](https://github.com/ktsaou))
- nd\_poll\(\) fairness [\#19298](https://github.com/netdata/netdata/pull/19298) ([ktsaou](https://github.com/ktsaou))
- more descriptive alert transition logs [\#19297](https://github.com/netdata/netdata/pull/19297) ([ktsaou](https://github.com/ktsaou))
- fix\(debugfs/sensors\): correct driver label value [\#19294](https://github.com/netdata/netdata/pull/19294) ([ilyam8](https://github.com/ilyam8))
- fix\(netdata-updater.sh\): use explicit paths for temp dir creation [\#19293](https://github.com/netdata/netdata/pull/19293) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): add bison and flex [\#19292](https://github.com/netdata/netdata/pull/19292) ([ilyam8](https://github.com/ilyam8))
- remove go.d/windows [\#19290](https://github.com/netdata/netdata/pull/19290) ([ilyam8](https://github.com/ilyam8))
- fix\(netdata-updater.sh\): ensure tmpdir-path argument is always passed [\#19289](https://github.com/netdata/netdata/pull/19289) ([ilyam8](https://github.com/ilyam8))
- fix\(netdata-updater.sh\): remove commit\_check\_file directory [\#19288](https://github.com/netdata/netdata/pull/19288) ([ilyam8](https://github.com/ilyam8))
- bump dag req jinja version [\#19287](https://github.com/netdata/netdata/pull/19287) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19286](https://github.com/netdata/netdata/pull/19286) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/nats\): add basic jetstream metrics [\#19285](https://github.com/netdata/netdata/pull/19285) ([ilyam8](https://github.com/ilyam8))
- fix go.d/nats tests [\#19284](https://github.com/netdata/netdata/pull/19284) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19283](https://github.com/netdata/netdata/pull/19283) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/nats\): add leafz metrics [\#19282](https://github.com/netdata/netdata/pull/19282) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19281](https://github.com/netdata/netdata/pull/19281) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/nats\): add server\_id label [\#19280](https://github.com/netdata/netdata/pull/19280) ([ilyam8](https://github.com/ilyam8))
- docs: improve on-prem troubleshooting readability [\#19279](https://github.com/netdata/netdata/pull/19279) ([ilyam8](https://github.com/ilyam8))
- Fix metric retention check and cleanup [\#19278](https://github.com/netdata/netdata/pull/19278) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d/rabbitmq\): handle insufficient perms when querying definitions [\#19277](https://github.com/netdata/netdata/pull/19277) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19276](https://github.com/netdata/netdata/pull/19276) ([netdatabot](https://github.com/netdatabot))
- Updates to onprem docs [\#19275](https://github.com/netdata/netdata/pull/19275) ([M4itee](https://github.com/M4itee))
- Skip label cleanup during metadata processing [\#19274](https://github.com/netdata/netdata/pull/19274) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): update go toolchain to v1.23.4 [\#19273](https://github.com/netdata/netdata/pull/19273) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/jackc/pgx/v5 from 5.7.1 to 5.7.2 in /src/go [\#19271](https://github.com/netdata/netdata/pull/19271) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/axiomhq/hyperloglog from 0.2.0 to 0.2.2 in /src/go [\#19270](https://github.com/netdata/netdata/pull/19270) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/net from 0.32.0 to 0.33.0 in /src/go [\#19269](https://github.com/netdata/netdata/pull/19269) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/docker/docker from 27.4.0+incompatible to 27.4.1+incompatible in /src/go [\#19268](https://github.com/netdata/netdata/pull/19268) ([dependabot[bot]](https://github.com/apps/dependabot))
- improvement\(go.d/nats\): add gatewayz metrics [\#19266](https://github.com/netdata/netdata/pull/19266) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19265](https://github.com/netdata/netdata/pull/19265) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/nats\): add routez metrics [\#19264](https://github.com/netdata/netdata/pull/19264) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19263](https://github.com/netdata/netdata/pull/19263) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/nats\): add accstatz metrics [\#19262](https://github.com/netdata/netdata/pull/19262) ([ilyam8](https://github.com/ilyam8))
- HELP and TYPE in prometheus fix [\#19261](https://github.com/netdata/netdata/pull/19261) ([ktsaou](https://github.com/ktsaou))
- Add an alert guide for reboot required [\#19260](https://github.com/netdata/netdata/pull/19260) ([ralphm](https://github.com/ralphm))
- fix crash when the DRM file does not contain the right information [\#19258](https://github.com/netdata/netdata/pull/19258) ([ktsaou](https://github.com/ktsaou))
- docs: change "node-membership-rules" filename/title [\#19257](https://github.com/netdata/netdata/pull/19257) ([ilyam8](https://github.com/ilyam8))
- Updated copyright notices [\#19256](https://github.com/netdata/netdata/pull/19256) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19254](https://github.com/netdata/netdata/pull/19254) ([netdatabot](https://github.com/netdatabot))
- docs: fix nats metadata suffix [\#19253](https://github.com/netdata/netdata/pull/19253) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d\): add  NATS collector [\#19252](https://github.com/netdata/netdata/pull/19252) ([ilyam8](https://github.com/ilyam8))
- Monitor sensors using libsensors via debugfs.plugin [\#19251](https://github.com/netdata/netdata/pull/19251) ([ktsaou](https://github.com/ktsaou))
- Add option to updater to report status of auto-updates on the system. [\#19248](https://github.com/netdata/netdata/pull/19248) ([Ferroin](https://github.com/Ferroin))
- DBENGINE: pgc tuning, replication tuning [\#19237](https://github.com/netdata/netdata/pull/19237) ([ktsaou](https://github.com/ktsaou))
- Update kickstart script to use new repository host. [\#18962](https://github.com/netdata/netdata/pull/18962) ([Ferroin](https://github.com/Ferroin))

## [v2.1.1](https://github.com/netdata/netdata/tree/v2.1.1) (2025-01-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.1.0...v2.1.1)

## [v2.1.0](https://github.com/netdata/netdata/tree/v2.1.0) (2024-12-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.3...v2.1.0)

**Merged pull requests:**

- use inactive memory when calculating cgroups total memory [\#19249](https://github.com/netdata/netdata/pull/19249) ([ktsaou](https://github.com/ktsaou))
- chore\(aclk/mqtt\): remove client\_id len check [\#19247](https://github.com/netdata/netdata/pull/19247) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): simplify cli is help [\#19246](https://github.com/netdata/netdata/pull/19246) ([ilyam8](https://github.com/ilyam8))
- Health transition saving optimization [\#19245](https://github.com/netdata/netdata/pull/19245) ([stelfrag](https://github.com/stelfrag))
- Avoid blocking waiting for an event during shutdown [\#19244](https://github.com/netdata/netdata/pull/19244) ([stelfrag](https://github.com/stelfrag))
- Do not call finalize on shutdown [\#19241](https://github.com/netdata/netdata/pull/19241) ([stelfrag](https://github.com/stelfrag))
- fix the renamed function under windows [\#19240](https://github.com/netdata/netdata/pull/19240) ([ktsaou](https://github.com/ktsaou))
- update netdata internal metrics ctx [\#19239](https://github.com/netdata/netdata/pull/19239) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d.plugin\): enable dyncfg vnodes [\#19238](https://github.com/netdata/netdata/pull/19238) ([ilyam8](https://github.com/ilyam8))
- docs: fix win deploy command for nightly [\#19236](https://github.com/netdata/netdata/pull/19236) ([ilyam8](https://github.com/ilyam8))
- RRDHOST system-info isolation [\#19235](https://github.com/netdata/netdata/pull/19235) ([ktsaou](https://github.com/ktsaou))
- Allow more threads to load contexts during startup [\#19234](https://github.com/netdata/netdata/pull/19234) ([stelfrag](https://github.com/stelfrag))
- Fix memory leak  [\#19233](https://github.com/netdata/netdata/pull/19233) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d/mongodb\): add missing disconnect in initClient [\#19232](https://github.com/netdata/netdata/pull/19232) ([ilyam8](https://github.com/ilyam8))
- docs: update ui 3rd party link [\#19231](https://github.com/netdata/netdata/pull/19231) ([ilyam8](https://github.com/ilyam8))
- docs: split redistributed and add judy and dlib [\#19230](https://github.com/netdata/netdata/pull/19230) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/lmittmann/tint from 1.0.5 to 1.0.6 in /src/go [\#19229](https://github.com/netdata/netdata/pull/19229) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix: fix heap use after free in health [\#19228](https://github.com/netdata/netdata/pull/19228) ([ktsaou](https://github.com/ktsaou))
- ci: replace exit 1 with conditional skip in website update workflow [\#19227](https://github.com/netdata/netdata/pull/19227) ([ilyam8](https://github.com/ilyam8))
- fix\(ml\): remove logging for earch not acquired dimension [\#19226](https://github.com/netdata/netdata/pull/19226) ([ilyam8](https://github.com/ilyam8))
- Fix static builds to ensure usability on intended baseline hardware. [\#19224](https://github.com/netdata/netdata/pull/19224) ([Ferroin](https://github.com/Ferroin))
- add MegaCli64 to ndsudo [\#19223](https://github.com/netdata/netdata/pull/19223) ([ilyam8](https://github.com/ilyam8))
- removing IP address information. Bumping traefik version [\#19222](https://github.com/netdata/netdata/pull/19222) ([M4itee](https://github.com/M4itee))
- fix compiler warnings [\#19221](https://github.com/netdata/netdata/pull/19221) ([ktsaou](https://github.com/ktsaou))
- disable h20 [\#19218](https://github.com/netdata/netdata/pull/19218) ([ilyam8](https://github.com/ilyam8))
- add pcre2 dev to install-requires-packages.sh [\#19217](https://github.com/netdata/netdata/pull/19217) ([ilyam8](https://github.com/ilyam8))
- remove ENABLE\_H2O=1 from installer [\#19216](https://github.com/netdata/netdata/pull/19216) ([ilyam8](https://github.com/ilyam8))
- fix: use setuid as a fallback for static builds when setcap fails for plugins [\#19215](https://github.com/netdata/netdata/pull/19215) ([ilyam8](https://github.com/ilyam8))
- add dyncfg vnode option to collectors [\#19214](https://github.com/netdata/netdata/pull/19214) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/vmware/govmomi from 0.46.2 to 0.46.3 [\#19213](https://github.com/netdata/netdata/pull/19213) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump k8s.io/client-go from 0.31.3 to 0.32.0 in /src/go [\#19210](https://github.com/netdata/netdata/pull/19210) ([dependabot[bot]](https://github.com/apps/dependabot))
- dyncfg vnodes improvements [\#19207](https://github.com/netdata/netdata/pull/19207) ([ilyam8](https://github.com/ilyam8))
- Streaming improvements No 8 [\#19206](https://github.com/netdata/netdata/pull/19206) ([ktsaou](https://github.com/ktsaou))
- feat\(go.d.plugin\): add dyncfg vnodes [\#19205](https://github.com/netdata/netdata/pull/19205) ([ilyam8](https://github.com/ilyam8))
- Streaming improvements No 7 [\#19204](https://github.com/netdata/netdata/pull/19204) ([ktsaou](https://github.com/ktsaou))
- Add dynamic rooms docs [\#19199](https://github.com/netdata/netdata/pull/19199) ([kapantzak](https://github.com/kapantzak))
- Streaming improvements No 6 [\#19196](https://github.com/netdata/netdata/pull/19196) ([ktsaou](https://github.com/ktsaou))
- Add cross-architecture build tests for Go code. [\#19195](https://github.com/netdata/netdata/pull/19195) ([Ferroin](https://github.com/Ferroin))
- Remove July arrays [\#19194](https://github.com/netdata/netdata/pull/19194) ([stelfrag](https://github.com/stelfrag))
- Streaming Improvements No 5 [\#19193](https://github.com/netdata/netdata/pull/19193) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19192](https://github.com/netdata/netdata/pull/19192) ([netdatabot](https://github.com/netdatabot))
- rw\_spinlocks: allow recursive readers, even when writers are waiting [\#19191](https://github.com/netdata/netdata/pull/19191) ([ktsaou](https://github.com/ktsaou))
- docs: remove a duplicated row [\#19190](https://github.com/netdata/netdata/pull/19190) ([orisano](https://github.com/orisano))
- build\(deps\): bump golang.org/x/crypto from 0.30.0 to 0.31.0 in /src/go [\#19189](https://github.com/netdata/netdata/pull/19189) ([dependabot[bot]](https://github.com/apps/dependabot))
- Network Metadata \(Windows plugin\) [\#19188](https://github.com/netdata/netdata/pull/19188) ([thiagoftsm](https://github.com/thiagoftsm))
- ci: fix update-website workflow [\#19187](https://github.com/netdata/netdata/pull/19187) ([ilyam8](https://github.com/ilyam8))
- Streaming improvements No 4 [\#19186](https://github.com/netdata/netdata/pull/19186) ([ktsaou](https://github.com/ktsaou))
- Move dependency handling for integrations to script. [\#19185](https://github.com/netdata/netdata/pull/19185) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19184](https://github.com/netdata/netdata/pull/19184) ([netdatabot](https://github.com/netdatabot))
- fix\(kickstart\): netdata\_avail\_check on Ubuntu [\#19183](https://github.com/netdata/netdata/pull/19183) ([ilyam8](https://github.com/ilyam8))
- Disks Metadata \(Windows plugin\) [\#19182](https://github.com/netdata/netdata/pull/19182) ([thiagoftsm](https://github.com/thiagoftsm))
- Bump repository config fetched by kickstart to latest version [\#19181](https://github.com/netdata/netdata/pull/19181) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d\): pass context to init/check/collect/cleanup [\#19180](https://github.com/netdata/netdata/pull/19180) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19177](https://github.com/netdata/netdata/pull/19177) ([netdatabot](https://github.com/netdatabot))
- docs: reorder silent mode and add full pipeline command example [\#19176](https://github.com/netdata/netdata/pull/19176) ([Ancairon](https://github.com/Ancairon))
- Add Objects metadata \(Windows Plugin\) [\#19175](https://github.com/netdata/netdata/pull/19175) ([thiagoftsm](https://github.com/thiagoftsm))
- Fixup URLs in package repo documentation to use index files. [\#19174](https://github.com/netdata/netdata/pull/19174) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations docs [\#19173](https://github.com/netdata/netdata/pull/19173) ([netdatabot](https://github.com/netdatabot))
- build\(deps\): bump github.com/docker/docker from 27.3.1+incompatible to 27.4.0+incompatible in /src/go [\#19172](https://github.com/netdata/netdata/pull/19172) ([dependabot[bot]](https://github.com/apps/dependabot))
- Processor Metadata \(Windows Plugin\) [\#19171](https://github.com/netdata/netdata/pull/19171) ([thiagoftsm](https://github.com/thiagoftsm))
- Streaming improvements No 3 [\#19168](https://github.com/netdata/netdata/pull/19168) ([ktsaou](https://github.com/ktsaou))
- Streaming improvements No 2 [\#19167](https://github.com/netdata/netdata/pull/19167) ([ktsaou](https://github.com/ktsaou))
- send quit to plugins [\#19166](https://github.com/netdata/netdata/pull/19166) ([ktsaou](https://github.com/ktsaou))
- add units per context to /api/v3/contexts [\#19165](https://github.com/netdata/netdata/pull/19165) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations docs [\#19164](https://github.com/netdata/netdata/pull/19164) ([netdatabot](https://github.com/netdatabot))
- Update cloud virtual host name [\#19163](https://github.com/netdata/netdata/pull/19163) ([stelfrag](https://github.com/stelfrag))
- docs: leftover links + changes on api-tokens.md [\#19162](https://github.com/netdata/netdata/pull/19162) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations docs [\#19161](https://github.com/netdata/netdata/pull/19161) ([netdatabot](https://github.com/netdatabot))
- docs: edit Authentication and Authorization section [\#19160](https://github.com/netdata/netdata/pull/19160) ([Ancairon](https://github.com/Ancairon))
- Remove Option from Installer \(Windows\) [\#19159](https://github.com/netdata/netdata/pull/19159) ([thiagoftsm](https://github.com/thiagoftsm))
- NET Framework metadata \(Windows.plugin Part 1\) [\#19158](https://github.com/netdata/netdata/pull/19158) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(build\): fix building go.d on 32bit [\#19156](https://github.com/netdata/netdata/pull/19156) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): correct sd dir [\#19155](https://github.com/netdata/netdata/pull/19155) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): correct unlockall impl [\#19154](https://github.com/netdata/netdata/pull/19154) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d\): unlock job files on quit/restart [\#19153](https://github.com/netdata/netdata/pull/19153) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19152](https://github.com/netdata/netdata/pull/19152) ([netdatabot](https://github.com/netdatabot))
- build\(deps\): bump github.com/axiomhq/hyperloglog from 0.2.0 to 0.2.1 in /src/go [\#19151](https://github.com/netdata/netdata/pull/19151) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump golang.org/x/net from 0.31.0 to 0.32.0 in /src/go [\#19149](https://github.com/netdata/netdata/pull/19149) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/prometheus/common from 0.60.1 to 0.61.0 in /src/go [\#19148](https://github.com/netdata/netdata/pull/19148) ([dependabot[bot]](https://github.com/apps/dependabot))
- MSSQL Metadatas \(windows.plugin\) [\#19147](https://github.com/netdata/netdata/pull/19147) ([thiagoftsm](https://github.com/thiagoftsm))
- chore\(go.d.plugin\): simplify main [\#19146](https://github.com/netdata/netdata/pull/19146) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d.plugin\): simplify netdataapi pkg [\#19145](https://github.com/netdata/netdata/pull/19145) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d.plugin\): improve function parser [\#19143](https://github.com/netdata/netdata/pull/19143) ([ilyam8](https://github.com/ilyam8))
- docs: fix a typo in aclk readme [\#19141](https://github.com/netdata/netdata/pull/19141) ([ilyam8](https://github.com/ilyam8))
- docs: Plans and ACLK docs edits [\#19140](https://github.com/netdata/netdata/pull/19140) ([Ancairon](https://github.com/Ancairon))
- docs: Edits in the main Netdata Cloud readme [\#19139](https://github.com/netdata/netdata/pull/19139) ([Ancairon](https://github.com/Ancairon))
- ci: fix build/create release [\#19138](https://github.com/netdata/netdata/pull/19138) ([ilyam8](https://github.com/ilyam8))
- Streaming improvements No 1 [\#19137](https://github.com/netdata/netdata/pull/19137) ([ktsaou](https://github.com/ktsaou))
- fixed bug in streaming sender read [\#19136](https://github.com/netdata/netdata/pull/19136) ([ktsaou](https://github.com/ktsaou))
- minor beatification of log messages [\#19135](https://github.com/netdata/netdata/pull/19135) ([ktsaou](https://github.com/ktsaou))
- docs: restructure readme intro for better readability [\#19134](https://github.com/netdata/netdata/pull/19134) ([ilyam8](https://github.com/ilyam8))
- ci: fix build/Prepare Artifacts [\#19133](https://github.com/netdata/netdata/pull/19133) ([ilyam8](https://github.com/ilyam8))
- Modify Claim Screen \(Windows Installer\) [\#19132](https://github.com/netdata/netdata/pull/19132) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations docs [\#19131](https://github.com/netdata/netdata/pull/19131) ([netdatabot](https://github.com/netdatabot))
- chore\(windows/hyperv\): small Hyper-V fixes [\#19130](https://github.com/netdata/netdata/pull/19130) ([ilyam8](https://github.com/ilyam8))
- docs\(windows/hyperv\): add Hyper-V metadata [\#19129](https://github.com/netdata/netdata/pull/19129) ([ilyam8](https://github.com/ilyam8))
- fix\(system-info\): change id\_like and name mac -\> macOS [\#19128](https://github.com/netdata/netdata/pull/19128) ([ilyam8](https://github.com/ilyam8))
- fix\(packaging\): correct go linux 386 checksum [\#19127](https://github.com/netdata/netdata/pull/19127) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations docs [\#19126](https://github.com/netdata/netdata/pull/19126) ([netdatabot](https://github.com/netdatabot))
- Integrations gha, remove .js and .json files after the process [\#19125](https://github.com/netdata/netdata/pull/19125) ([Ancairon](https://github.com/Ancairon))
- Avoid scanning charts for replication status [\#19124](https://github.com/netdata/netdata/pull/19124) ([stelfrag](https://github.com/stelfrag))
- Address installer minor issues \(Windows\) [\#19122](https://github.com/netdata/netdata/pull/19122) ([thiagoftsm](https://github.com/thiagoftsm))
- Move eBPF code from linetdata to src/collector [\#19121](https://github.com/netdata/netdata/pull/19121) ([thiagoftsm](https://github.com/thiagoftsm))
- change default nice level to 0 [\#19120](https://github.com/netdata/netdata/pull/19120) ([ilyam8](https://github.com/ilyam8))
- Disable mimalloc by default / enable explicitly if needed [\#19118](https://github.com/netdata/netdata/pull/19118) ([stelfrag](https://github.com/stelfrag))
- Reduce EBPF memory usage [\#19117](https://github.com/netdata/netdata/pull/19117) ([stelfrag](https://github.com/stelfrag))
- Fix undefined behaviour. [\#19116](https://github.com/netdata/netdata/pull/19116) ([vkalintiris](https://github.com/vkalintiris))
- disable python.d/example [\#19114](https://github.com/netdata/netdata/pull/19114) ([ilyam8](https://github.com/ilyam8))
- docs: format, typos, and some simplifications in `docs/` [\#19112](https://github.com/netdata/netdata/pull/19112) ([ilyam8](https://github.com/ilyam8))
- change dim order because of colours in reboot\_required [\#19111](https://github.com/netdata/netdata/pull/19111) ([ilyam8](https://github.com/ilyam8))
- fix\(proc/reboot\_required\): disable on non Debian-based systems [\#19110](https://github.com/netdata/netdata/pull/19110) ([ilyam8](https://github.com/ilyam8))
- feat\(proc.plugin\): add Reboot Required collector [\#19109](https://github.com/netdata/netdata/pull/19109) ([ilyam8](https://github.com/ilyam8))
- docs: update On-Prem System Requirements [\#19107](https://github.com/netdata/netdata/pull/19107) ([ilyam8](https://github.com/ilyam8))
- On-prem docs edits 2 [\#19105](https://github.com/netdata/netdata/pull/19105) ([Ancairon](https://github.com/Ancairon))
- Docs edits on Cloud versions and On Prem [\#19104](https://github.com/netdata/netdata/pull/19104) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/pkg/socket\): add err to callback return values [\#19103](https://github.com/netdata/netdata/pull/19103) ([ilyam8](https://github.com/ilyam8))
- docs: fix img tag [\#19102](https://github.com/netdata/netdata/pull/19102) ([ilyam8](https://github.com/ilyam8))
- Edit the organize doc [\#19101](https://github.com/netdata/netdata/pull/19101) ([Ancairon](https://github.com/Ancairon))
- Update connecting documentation [\#19100](https://github.com/netdata/netdata/pull/19100) ([Ancairon](https://github.com/Ancairon))
- Claiming proxy defaults and additonal log info [\#19098](https://github.com/netdata/netdata/pull/19098) ([ktsaou](https://github.com/ktsaou))
- Reset parameter when generating an alert snapshot [\#19097](https://github.com/netdata/netdata/pull/19097) ([stelfrag](https://github.com/stelfrag))
- Update Registry docs [\#19095](https://github.com/netdata/netdata/pull/19095) ([Ancairon](https://github.com/Ancairon))
- Collected and available metrics, instances and contexts [\#19094](https://github.com/netdata/netdata/pull/19094) ([ktsaou](https://github.com/ktsaou))
- docs\(systemd-journal.plugin\): correct full-text search [\#19093](https://github.com/netdata/netdata/pull/19093) ([ilyam8](https://github.com/ilyam8))
- Daemon docs edits [\#19091](https://github.com/netdata/netdata/pull/19091) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d.plugin\): renames part 2  [\#19090](https://github.com/netdata/netdata/pull/19090) ([ilyam8](https://github.com/ilyam8))
- remove stale docs, and update links and optimization documentation [\#19089](https://github.com/netdata/netdata/pull/19089) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#19088](https://github.com/netdata/netdata/pull/19088) ([netdatabot](https://github.com/netdatabot))
- docs: fix go.d modules rename leftovers [\#19087](https://github.com/netdata/netdata/pull/19087) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19086](https://github.com/netdata/netdata/pull/19086) ([netdatabot](https://github.com/netdatabot))
- update integrations gen script [\#19085](https://github.com/netdata/netdata/pull/19085) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/hpssa\): handle HPE Smart Array line [\#19084](https://github.com/netdata/netdata/pull/19084) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d.plugin\): renames [\#19081](https://github.com/netdata/netdata/pull/19081) ([ilyam8](https://github.com/ilyam8))
- Use mimalloc [\#19080](https://github.com/netdata/netdata/pull/19080) ([vkalintiris](https://github.com/vkalintiris))
- Regenerate integrations.js [\#19079](https://github.com/netdata/netdata/pull/19079) ([netdatabot](https://github.com/netdatabot))
- Remove Go windows integration [\#19078](https://github.com/netdata/netdata/pull/19078) ([Ancairon](https://github.com/Ancairon))
- Split database overview and configuration reference [\#19077](https://github.com/netdata/netdata/pull/19077) ([Ancairon](https://github.com/Ancairon))
- Database docs edits [\#19075](https://github.com/netdata/netdata/pull/19075) ([Ancairon](https://github.com/Ancairon))
- RAM and CPU resource util pages [\#19074](https://github.com/netdata/netdata/pull/19074) ([Ancairon](https://github.com/Ancairon))
- Collector configuration page edits [\#19072](https://github.com/netdata/netdata/pull/19072) ([Ancairon](https://github.com/Ancairon))
- Create a terminology dictionary for Netdata [\#19071](https://github.com/netdata/netdata/pull/19071) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump github.com/prometheus-community/pro-bing from 0.4.2-0.20241106090159-5a5f1d731cf5 to 0.5.0 in /src/go [\#19070](https://github.com/netdata/netdata/pull/19070) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/stretchr/testify from 1.9.0 to 1.10.0 in /src/go [\#19069](https://github.com/netdata/netdata/pull/19069) ([dependabot[bot]](https://github.com/apps/dependabot))
- update gorilla comp internal charts family [\#19068](https://github.com/netdata/netdata/pull/19068) ([ilyam8](https://github.com/ilyam8))
- docs\(systemd-journal.plugin\): correct "Full-text search" [\#19066](https://github.com/netdata/netdata/pull/19066) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19065](https://github.com/netdata/netdata/pull/19065) ([netdatabot](https://github.com/netdatabot))
- Register service to delay start [\#19063](https://github.com/netdata/netdata/pull/19063) ([stelfrag](https://github.com/stelfrag))
- add links to mssql perflib object docs [\#19062](https://github.com/netdata/netdata/pull/19062) ([ilyam8](https://github.com/ilyam8))
- claim -\> connect in docs [\#19060](https://github.com/netdata/netdata/pull/19060) ([Ancairon](https://github.com/Ancairon))
- build\(deps\): bump k8s.io/client-go from 0.31.2 to 0.31.3 in /src/go [\#19059](https://github.com/netdata/netdata/pull/19059) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.46.1 to 0.46.2 in /src/go [\#19058](https://github.com/netdata/netdata/pull/19058) ([dependabot[bot]](https://github.com/apps/dependabot))
- Windows doc updates [\#19054](https://github.com/netdata/netdata/pull/19054) ([Ancairon](https://github.com/Ancairon))
- Securing Agents section docs cleanup [\#19053](https://github.com/netdata/netdata/pull/19053) ([Ancairon](https://github.com/Ancairon))
- fix\(go.d/pkg/web\): correct close idle connections [\#19052](https://github.com/netdata/netdata/pull/19052) ([ilyam8](https://github.com/ilyam8))
- Update documentation about our native package repos. [\#19049](https://github.com/netdata/netdata/pull/19049) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#19048](https://github.com/netdata/netdata/pull/19048) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/pkg/web\): add "force\_http2" option [\#19047](https://github.com/netdata/netdata/pull/19047) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19045](https://github.com/netdata/netdata/pull/19045) ([netdatabot](https://github.com/netdatabot))
- Capitalize the word "Agent" [\#19044](https://github.com/netdata/netdata/pull/19044) ([Ancairon](https://github.com/Ancairon))
- Capitalize the word "cloud" [\#19043](https://github.com/netdata/netdata/pull/19043) ([Ancairon](https://github.com/Ancairon))
- Add a special version number to bypass alert snapshots [\#19042](https://github.com/netdata/netdata/pull/19042) ([stelfrag](https://github.com/stelfrag))
- Add Custom Actions \(Installer\) [\#19041](https://github.com/netdata/netdata/pull/19041) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(go.d/nvidia\_smi\): disable loop mode on Win [\#19040](https://github.com/netdata/netdata/pull/19040) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/nvidia\_smi\): disable loop mode by default on Win [\#19039](https://github.com/netdata/netdata/pull/19039) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d.plugin\): terminate on QUIT command [\#19038](https://github.com/netdata/netdata/pull/19038) ([ilyam8](https://github.com/ilyam8))
- fix\(windows/netframework\): dont sanitize proc name for labels [\#19036](https://github.com/netdata/netdata/pull/19036) ([ilyam8](https://github.com/ilyam8))
- Fix MSSQL algorithm \(Windows.plugin\) [\#19035](https://github.com/netdata/netdata/pull/19035) ([thiagoftsm](https://github.com/thiagoftsm))
- --dev option to installer [\#19034](https://github.com/netdata/netdata/pull/19034) ([ktsaou](https://github.com/ktsaou))
- add `shutdown` keyword to ensure graceful service termination on FreeBSD [\#19033](https://github.com/netdata/netdata/pull/19033) ([ilyam8](https://github.com/ilyam8))
- fix: ensure correct startup order for Netdata service on FreeBSD [\#19032](https://github.com/netdata/netdata/pull/19032) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/gorcon/rcon from 1.3.5 to 1.4.0 in /src/go [\#19031](https://github.com/netdata/netdata/pull/19031) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/vmware/govmomi from 0.46.0 to 0.46.1 in /src/go [\#19030](https://github.com/netdata/netdata/pull/19030) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#19029](https://github.com/netdata/netdata/pull/19029) ([netdatabot](https://github.com/netdatabot))
- improvement\(windows/iis\): add requests by type chart [\#19028](https://github.com/netdata/netdata/pull/19028) ([ilyam8](https://github.com/ilyam8))
- fix\(windows/iis\): dont sanitize site name for labels [\#19027](https://github.com/netdata/netdata/pull/19027) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d.plugin\): set nooplogger for automaxprocs [\#19026](https://github.com/netdata/netdata/pull/19026) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19025](https://github.com/netdata/netdata/pull/19025) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/windows\): remove references to old MSI [\#19024](https://github.com/netdata/netdata/pull/19024) ([ilyam8](https://github.com/ilyam8))
- improvement\(go.d.plugin\): automatically set GOMAXPROCS [\#19023](https://github.com/netdata/netdata/pull/19023) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19022](https://github.com/netdata/netdata/pull/19022) ([netdatabot](https://github.com/netdatabot))
- docs: just iis [\#19021](https://github.com/netdata/netdata/pull/19021) ([ilyam8](https://github.com/ilyam8))
- chore\(windows.plugin\): format win collectors code [\#19019](https://github.com/netdata/netdata/pull/19019) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19018](https://github.com/netdata/netdata/pull/19018) ([netdatabot](https://github.com/netdatabot))
- fix\(go.d/ping\): fix "interface" option [\#19016](https://github.com/netdata/netdata/pull/19016) ([ilyam8](https://github.com/ilyam8))
- Remove MSI test [\#19015](https://github.com/netdata/netdata/pull/19015) ([thiagoftsm](https://github.com/thiagoftsm))
- fix has\_receiver condition in rrdhost\_status\(\) [\#19014](https://github.com/netdata/netdata/pull/19014) ([ktsaou](https://github.com/ktsaou))
- backport of fixes from balance-parents [\#19012](https://github.com/netdata/netdata/pull/19012) ([ktsaou](https://github.com/ktsaou))
- add missing spinlock unlocks on containers [\#19011](https://github.com/netdata/netdata/pull/19011) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#19010](https://github.com/netdata/netdata/pull/19010) ([netdatabot](https://github.com/netdatabot))
- docs\(go.d/windows\): add deprecation notice [\#19009](https://github.com/netdata/netdata/pull/19009) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/dyncfg\): remove additionalProperties [\#19006](https://github.com/netdata/netdata/pull/19006) ([ilyam8](https://github.com/ilyam8))
- Set expires header when serving files [\#19005](https://github.com/netdata/netdata/pull/19005) ([stelfrag](https://github.com/stelfrag))
- fix\(go.d/x509check\): correct check revocation code [\#19004](https://github.com/netdata/netdata/pull/19004) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/dyncfg\): remove additionalProperties check [\#19003](https://github.com/netdata/netdata/pull/19003) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#19002](https://github.com/netdata/netdata/pull/19002) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/x509check\): support checking full chain expiry time [\#19001](https://github.com/netdata/netdata/pull/19001) ([ilyam8](https://github.com/ilyam8))
- fix: exclude volumes w/o drive letter from disk\_space\_usage\_alert [\#19000](https://github.com/netdata/netdata/pull/19000) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18997](https://github.com/netdata/netdata/pull/18997) ([netdatabot](https://github.com/netdatabot))
- docs: win deploy remove `./` [\#18996](https://github.com/netdata/netdata/pull/18996) ([ilyam8](https://github.com/ilyam8))
- docs: single line win deploy [\#18994](https://github.com/netdata/netdata/pull/18994) ([ilyam8](https://github.com/ilyam8))
- Add SQL Express Metrics [\#18992](https://github.com/netdata/netdata/pull/18992) ([thiagoftsm](https://github.com/thiagoftsm))
- Do not intentionally abort on non-0 exit code. [\#18991](https://github.com/netdata/netdata/pull/18991) ([vkalintiris](https://github.com/vkalintiris))
- update plugin\_data\_collection\_status alert summary/info [\#18990](https://github.com/netdata/netdata/pull/18990) ([ilyam8](https://github.com/ilyam8))
- health: enable go.d data collection job status alert [\#18989](https://github.com/netdata/netdata/pull/18989) ([ilyam8](https://github.com/ilyam8))
- update GH bug report [\#18988](https://github.com/netdata/netdata/pull/18988) ([ilyam8](https://github.com/ilyam8))
- chore\(go.d.plugin\): fix duplicate boolToInt [\#18987](https://github.com/netdata/netdata/pull/18987) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump golang.org/x/net from 0.30.0 to 0.31.0 in /src/go [\#18986](https://github.com/netdata/netdata/pull/18986) ([dependabot[bot]](https://github.com/apps/dependabot))
- Improve Installer \(Part II\) [\#18983](https://github.com/netdata/netdata/pull/18983) ([thiagoftsm](https://github.com/thiagoftsm))
- improvement\(go.d.plugin\): add data collection status chart [\#18981](https://github.com/netdata/netdata/pull/18981) ([ilyam8](https://github.com/ilyam8))
- ci: fix win jobs [\#18979](https://github.com/netdata/netdata/pull/18979) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18977](https://github.com/netdata/netdata/pull/18977) ([netdatabot](https://github.com/netdatabot))
- improvement\(go.d/rabbitmq\): add queue status and net partitions [\#18976](https://github.com/netdata/netdata/pull/18976) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18973](https://github.com/netdata/netdata/pull/18973) ([netdatabot](https://github.com/netdatabot))
- add rabbitmq alerts [\#18972](https://github.com/netdata/netdata/pull/18972) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18971](https://github.com/netdata/netdata/pull/18971) ([netdatabot](https://github.com/netdatabot))
- fix\(go.d/snmp\): don't return error if no sysName [\#18970](https://github.com/netdata/netdata/pull/18970) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump golang.org/x/text from 0.19.0 to 0.20.0 in /src/go [\#18968](https://github.com/netdata/netdata/pull/18968) ([dependabot[bot]](https://github.com/apps/dependabot))
- go mod tidy [\#18967](https://github.com/netdata/netdata/pull/18967) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18966](https://github.com/netdata/netdata/pull/18966) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/rabbitmq\): add cluster support [\#18965](https://github.com/netdata/netdata/pull/18965) ([ilyam8](https://github.com/ilyam8))
- Tidy up CI to improve overall run times. [\#18957](https://github.com/netdata/netdata/pull/18957) ([Ferroin](https://github.com/Ferroin))
- Balance streaming parents [\#18945](https://github.com/netdata/netdata/pull/18945) ([ktsaou](https://github.com/ktsaou))
- added /api/v3/stream\_path [\#18943](https://github.com/netdata/netdata/pull/18943) ([ktsaou](https://github.com/ktsaou))
- Update Windows Documentation [\#18928](https://github.com/netdata/netdata/pull/18928) ([thiagoftsm](https://github.com/thiagoftsm))

## [v2.0.3](https://github.com/netdata/netdata/tree/v2.0.3) (2024-11-22)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.2...v2.0.3)

## [v2.0.2](https://github.com/netdata/netdata/tree/v2.0.2) (2024-11-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.1...v2.0.2)

## [v2.0.1](https://github.com/netdata/netdata/tree/v2.0.1) (2024-11-14)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.0.0...v2.0.1)

## [v2.0.0](https://github.com/netdata/netdata/tree/v2.0.0) (2024-11-07)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.5...v2.0.0)

**Merged pull requests:**

- build\(deps\): update go toolchain to v1.23.3 [\#18961](https://github.com/netdata/netdata/pull/18961) ([ilyam8](https://github.com/ilyam8))
- Adjust max possible extent size [\#18960](https://github.com/netdata/netdata/pull/18960) ([stelfrag](https://github.com/stelfrag))
- build\(deps\): bump github.com/vmware/govmomi from 0.45.1 to 0.46.0 in /src/go [\#18959](https://github.com/netdata/netdata/pull/18959) ([dependabot[bot]](https://github.com/apps/dependabot))
- chore\(go.d.plugin\): remove duplicate logging in init/check [\#18955](https://github.com/netdata/netdata/pull/18955) ([ilyam8](https://github.com/ilyam8))
- Update README.md [\#18954](https://github.com/netdata/netdata/pull/18954) ([Ancairon](https://github.com/Ancairon))
- Fix br elements [\#18952](https://github.com/netdata/netdata/pull/18952) ([Ancairon](https://github.com/Ancairon))
- Precompile Python code on Windows. [\#18951](https://github.com/netdata/netdata/pull/18951) ([Ferroin](https://github.com/Ferroin))
- docs: simplify go.d.plugin readme [\#18949](https://github.com/netdata/netdata/pull/18949) ([ilyam8](https://github.com/ilyam8))
- fix memory leak when using libcurl [\#18947](https://github.com/netdata/netdata/pull/18947) ([ktsaou](https://github.com/ktsaou))
- docs: add "Plugin Privileges" section [\#18946](https://github.com/netdata/netdata/pull/18946) ([ilyam8](https://github.com/ilyam8))
- docs: fix Caddy docker compose example [\#18944](https://github.com/netdata/netdata/pull/18944) ([ilyam8](https://github.com/ilyam8))
- docs: grammar/format fixes to `docs/netdata-agent/` [\#18942](https://github.com/netdata/netdata/pull/18942) ([ilyam8](https://github.com/ilyam8))
- Streaming re-organization [\#18941](https://github.com/netdata/netdata/pull/18941) ([ktsaou](https://github.com/ktsaou))
- random numbers No 3 [\#18940](https://github.com/netdata/netdata/pull/18940) ([ktsaou](https://github.com/ktsaou))
- Random numbers improvements [\#18939](https://github.com/netdata/netdata/pull/18939) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/prometheus\): correct unsupported protocol scheme "file" error [\#18938](https://github.com/netdata/netdata/pull/18938) ([ilyam8](https://github.com/ilyam8))
- Improve ACLK sync CPU usage [\#18935](https://github.com/netdata/netdata/pull/18935) ([stelfrag](https://github.com/stelfrag))
- Hyper collector fixes [\#18934](https://github.com/netdata/netdata/pull/18934) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18932](https://github.com/netdata/netdata/pull/18932) ([netdatabot](https://github.com/netdatabot))
- better randomness for heartbeat [\#18930](https://github.com/netdata/netdata/pull/18930) ([ktsaou](https://github.com/ktsaou))
- add randomness per thread to heartbeat [\#18929](https://github.com/netdata/netdata/pull/18929) ([ktsaou](https://github.com/ktsaou))
- Improve the documentation on removing stale nodes [\#18927](https://github.com/netdata/netdata/pull/18927) ([ralphm](https://github.com/ralphm))
- Docs: Changes to title and CPU requirements [\#18925](https://github.com/netdata/netdata/pull/18925) ([Ancairon](https://github.com/Ancairon))
- chore\(go.d/nvidia\_smi\): remove use\_csv\_format \(deprecated\) from config [\#18924](https://github.com/netdata/netdata/pull/18924) ([ilyam8](https://github.com/ilyam8))
- Docs: small fixes and pass on sizing Agents [\#18923](https://github.com/netdata/netdata/pull/18923) ([Ancairon](https://github.com/Ancairon))
- go.d/portcheck: separate tabs for tcp/upd ports [\#18922](https://github.com/netdata/netdata/pull/18922) ([ilyam8](https://github.com/ilyam8))
- Update Libbpf [\#18921](https://github.com/netdata/netdata/pull/18921) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/fsnotify/fsnotify from 1.7.0 to 1.8.0 in /src/go [\#18920](https://github.com/netdata/netdata/pull/18920) ([dependabot[bot]](https://github.com/apps/dependabot))
- log2journal now uses libnetdata [\#18919](https://github.com/netdata/netdata/pull/18919) ([ktsaou](https://github.com/ktsaou))
- docs: fix ui license link [\#18918](https://github.com/netdata/netdata/pull/18918) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18917](https://github.com/netdata/netdata/pull/18917) ([netdatabot](https://github.com/netdatabot))
- Switch DEB/RPM repositories to new subdomain. [\#18916](https://github.com/netdata/netdata/pull/18916) ([Ferroin](https://github.com/Ferroin))
- docs: fix broken links in metadata [\#18915](https://github.com/netdata/netdata/pull/18915) ([ilyam8](https://github.com/ilyam8))
- Update CI to generate MSI installer for Windows using WiX. [\#18914](https://github.com/netdata/netdata/pull/18914) ([Ferroin](https://github.com/Ferroin))
- Fix potential wait forever in mqtt loop [\#18913](https://github.com/netdata/netdata/pull/18913) ([stelfrag](https://github.com/stelfrag))
- add `dagster` to apps\_groups.conf [\#18912](https://github.com/netdata/netdata/pull/18912) ([andrewm4894](https://github.com/andrewm4894))
- Installation section simplification [\#18911](https://github.com/netdata/netdata/pull/18911) ([Ancairon](https://github.com/Ancairon))
- fix\(debugfs/extfrag\): add zone label [\#18910](https://github.com/netdata/netdata/pull/18910) ([ilyam8](https://github.com/ilyam8))
- proc.plugin: log as info if a dir not exists [\#18909](https://github.com/netdata/netdata/pull/18909) ([ilyam8](https://github.com/ilyam8))
- uninstall docs edits [\#18908](https://github.com/netdata/netdata/pull/18908) ([Ancairon](https://github.com/Ancairon))
- Update uninstallation docs and remove reinstallation page [\#18907](https://github.com/netdata/netdata/pull/18907) ([Ancairon](https://github.com/Ancairon))
- Adjust API version [\#18906](https://github.com/netdata/netdata/pull/18906) ([stelfrag](https://github.com/stelfrag))
- Fix a potential invalid double free memory [\#18905](https://github.com/netdata/netdata/pull/18905) ([stelfrag](https://github.com/stelfrag))
- MSI Improvements [\#18903](https://github.com/netdata/netdata/pull/18903) ([thiagoftsm](https://github.com/thiagoftsm))
- versioning for functions [\#18902](https://github.com/netdata/netdata/pull/18902) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18901](https://github.com/netdata/netdata/pull/18901) ([netdatabot](https://github.com/netdatabot))
- chore\(go.d.plugin\): add build tags to modules [\#18900](https://github.com/netdata/netdata/pull/18900) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18899](https://github.com/netdata/netdata/pull/18899) ([netdatabot](https://github.com/netdatabot))
- Updating Netdata docs [\#18898](https://github.com/netdata/netdata/pull/18898) ([Ancairon](https://github.com/Ancairon))
- remove python.d/zscores [\#18897](https://github.com/netdata/netdata/pull/18897) ([ilyam8](https://github.com/ilyam8))
- Coverity fixes [\#18896](https://github.com/netdata/netdata/pull/18896) ([stelfrag](https://github.com/stelfrag))
- docs edit [\#18895](https://github.com/netdata/netdata/pull/18895) ([Ancairon](https://github.com/Ancairon))
- Start-stop-restart for windows, plus move info to its own file [\#18894](https://github.com/netdata/netdata/pull/18894) ([Ancairon](https://github.com/Ancairon))
- log2journal: fix config parsing memory leaks [\#18893](https://github.com/netdata/netdata/pull/18893) ([ktsaou](https://github.com/ktsaou))
- Fix coverity issues [\#18892](https://github.com/netdata/netdata/pull/18892) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18891](https://github.com/netdata/netdata/pull/18891) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d.plugin\): add spigotmc collector [\#18890](https://github.com/netdata/netdata/pull/18890) ([ilyam8](https://github.com/ilyam8))

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
