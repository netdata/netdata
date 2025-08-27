# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v2.6.3...HEAD)

**Merged pull requests:**

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
- build\(deps\): bump github.com/coreos/go-systemd/v22 from 22.5.0 to 22.6.0 in /src/go [\#20874](https://github.com/netdata/netdata/pull/20874) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix\(go.d\): create vnode internal data\_collection\_status charts in the main context [\#20872](https://github.com/netdata/netdata/pull/20872) ([ilyam8](https://github.com/ilyam8))
- Fix Table field comparison in SNMP collector table tests [\#20871](https://github.com/netdata/netdata/pull/20871) ([Copilot](https://github.com/apps/copilot-swe-agent))
- Rename default port for the OpenTelemetry Collector [\#20868](https://github.com/netdata/netdata/pull/20868) ([ralphm](https://github.com/ralphm))
- improve\(go.d/snmp\): add more entries in juniper metadata file [\#20867](https://github.com/netdata/netdata/pull/20867) ([ilyam8](https://github.com/ilyam8))
- Update documentation [\#20865](https://github.com/netdata/netdata/pull/20865) ([thiagoftsm](https://github.com/thiagoftsm))
- Update documentation \(Windows.plugin\) [\#20864](https://github.com/netdata/netdata/pull/20864) ([thiagoftsm](https://github.com/thiagoftsm))
- chore\(go.d/snmp\): more vendor-scoped meta yaml files [\#20863](https://github.com/netdata/netdata/pull/20863) ([ilyam8](https://github.com/ilyam8))
- improve\(go.d/snmp\): update snmp meta copilot [\#20861](https://github.com/netdata/netdata/pull/20861) ([ilyam8](https://github.com/ilyam8))
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
- update fortinet snmp profiles [\#20609](https://github.com/netdata/netdata/pull/20609) ([ilyam8](https://github.com/ilyam8))
- improve netapp snmp profile [\#20608](https://github.com/netdata/netdata/pull/20608) ([ilyam8](https://github.com/ilyam8))
- Improve datafile indexing [\#20607](https://github.com/netdata/netdata/pull/20607) ([stelfrag](https://github.com/stelfrag))
- chore\(go.d/snmp\): add disable\_legacy\_collection option [\#20606](https://github.com/netdata/netdata/pull/20606) ([ilyam8](https://github.com/ilyam8))
- improve mikrotik-router snmp profile [\#20605](https://github.com/netdata/netdata/pull/20605) ([ilyam8](https://github.com/ilyam8))
- small snmp-related changes [\#20603](https://github.com/netdata/netdata/pull/20603) ([ilyam8](https://github.com/ilyam8))
- Fix compilation on windows [\#20602](https://github.com/netdata/netdata/pull/20602) ([stelfrag](https://github.com/stelfrag))
- Update sqlite version to 3.50.2 [\#20601](https://github.com/netdata/netdata/pull/20601) ([stelfrag](https://github.com/stelfrag))
- transfer Learn PR 2473 [\#20600](https://github.com/netdata/netdata/pull/20600) ([Ancairon](https://github.com/Ancairon))
- update generic snmp profiles [\#20599](https://github.com/netdata/netdata/pull/20599) ([ilyam8](https://github.com/ilyam8))
- Metadata worker should respect shutdown request [\#20598](https://github.com/netdata/netdata/pull/20598) ([stelfrag](https://github.com/stelfrag))
- docs: fix 404 link in README.md [\#20597](https://github.com/netdata/netdata/pull/20597) ([ilyam8](https://github.com/ilyam8))
- build\(deps\): bump github.com/docker/docker from 28.2.2+incompatible to 28.3.0+incompatible in /src/go [\#20595](https://github.com/netdata/netdata/pull/20595) ([dependabot[bot]](https://github.com/apps/dependabot))
- improve\(go.d/snmp-profiles\): extend transformEntitySensorValue [\#20594](https://github.com/netdata/netdata/pull/20594) ([ilyam8](https://github.com/ilyam8))
- Add Screen to Windows installer [\#20593](https://github.com/netdata/netdata/pull/20593) ([thiagoftsm](https://github.com/thiagoftsm))
- build\(deps\): bump github.com/go-viper/mapstructure/v2 from 2.2.1 to 2.3.0 in /src/go/otel-collector/exporter/journaldexporter [\#20592](https://github.com/netdata/netdata/pull/20592) ([dependabot[bot]](https://github.com/apps/dependabot))
- build\(deps\): bump github.com/go-viper/mapstructure/v2 from 2.2.1 to 2.3.0 in /src/go/otel-collector/exporter/netdataexporter [\#20591](https://github.com/netdata/netdata/pull/20591) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations docs [\#20589](https://github.com/netdata/netdata/pull/20589) ([netdatabot](https://github.com/netdatabot))
- doc: update SCIM doc [\#20588](https://github.com/netdata/netdata/pull/20588) ([juacker](https://github.com/juacker))
- ddsnmp add pow transform func and allow mapping duplicate values [\#20587](https://github.com/netdata/netdata/pull/20587) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): correct matching same profile multiple times [\#20586](https://github.com/netdata/netdata/pull/20586) ([ilyam8](https://github.com/ilyam8))
- remove devType/Vendor/ from ddsnmp metric families [\#20585](https://github.com/netdata/netdata/pull/20585) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): include table name in config id [\#20584](https://github.com/netdata/netdata/pull/20584) ([ilyam8](https://github.com/ilyam8))
- fix\(go.d/ddsnmp\): walk cross-table columns when referenced table has no metrics [\#20583](https://github.com/netdata/netdata/pull/20583) ([ilyam8](https://github.com/ilyam8))
- Rework datafiles [\#20581](https://github.com/netdata/netdata/pull/20581) ([stelfrag](https://github.com/stelfrag))
- Windows Pluging \(Freedom to update every\) [\#20580](https://github.com/netdata/netdata/pull/20580) ([thiagoftsm](https://github.com/thiagoftsm))
- Ignore duplicate entries when rebuilding the alert version table [\#20579](https://github.com/netdata/netdata/pull/20579) ([stelfrag](https://github.com/stelfrag))
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
- Migrate from stable to nightly and vice versa [\#20551](https://github.com/netdata/netdata/pull/20551) ([kanelatechnical](https://github.com/kanelatechnical))
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
- ASP.NET \(windows.plugin\) [\#20485](https://github.com/netdata/netdata/pull/20485) ([thiagoftsm](https://github.com/thiagoftsm))
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
- nd-mcp add claude cli cmd for adding netdata mcp [\#20440](https://github.com/netdata/netdata/pull/20440) ([andrewm4894](https://github.com/andrewm4894))
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
