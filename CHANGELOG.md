# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.47.4...HEAD)

**Merged pull requests:**

- Fix crash on agent initialization [\#18746](https://github.com/netdata/netdata/pull/18746) ([stelfrag](https://github.com/stelfrag))
- fix\(apps.plugin\): fix debug msg spam on macOS/freeBSD [\#18743](https://github.com/netdata/netdata/pull/18743) ([ilyam8](https://github.com/ilyam8))
- docs\(apps.plugin\): fix prefix/suffix pattern example [\#18742](https://github.com/netdata/netdata/pull/18742) ([ilyam8](https://github.com/ilyam8))
- feat\(go.d/nvme\): add model\_number label [\#18741](https://github.com/netdata/netdata/pull/18741) ([ilyam8](https://github.com/ilyam8))
- sanitizers should not remove trailing underscores [\#18738](https://github.com/netdata/netdata/pull/18738) ([ktsaou](https://github.com/ktsaou))
- Add `ilert` cloud notification integration [\#18736](https://github.com/netdata/netdata/pull/18736) ([car12o](https://github.com/car12o))
- fix\(go.d/sensors\): fix parsing power accuracy [\#18735](https://github.com/netdata/netdata/pull/18735) ([ilyam8](https://github.com/ilyam8))
- apps.plugin; allow parents to identify the children [\#18734](https://github.com/netdata/netdata/pull/18734) ([ktsaou](https://github.com/ktsaou))
- Windows deploy metadata [\#18733](https://github.com/netdata/netdata/pull/18733) ([Ancairon](https://github.com/Ancairon))
- \[storcli\] Support for controller ROC temperature. [\#18732](https://github.com/netdata/netdata/pull/18732) ([eatnumber1](https://github.com/eatnumber1))
- systemd-cat-native negative timeout [\#18729](https://github.com/netdata/netdata/pull/18729) ([ktsaou](https://github.com/ktsaou))
- fix\(perf.plugin\): disable if all events disabled during init [\#18728](https://github.com/netdata/netdata/pull/18728) ([ilyam8](https://github.com/ilyam8))
- apps.plugin: print also the original comm [\#18727](https://github.com/netdata/netdata/pull/18727) ([ktsaou](https://github.com/ktsaou))
- Fix handling of workflow artifacts. [\#18726](https://github.com/netdata/netdata/pull/18726) ([Ferroin](https://github.com/Ferroin))
- reset the log sources to apply user selection [\#18725](https://github.com/netdata/netdata/pull/18725) ([ktsaou](https://github.com/ktsaou))
- fix logs POST query payload parsing [\#18722](https://github.com/netdata/netdata/pull/18722) ([ktsaou](https://github.com/ktsaou))
- fix\(go.d/portcheck\): stop checking UDP ports on ICMP listen error [\#18721](https://github.com/netdata/netdata/pull/18721) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18720](https://github.com/netdata/netdata/pull/18720) ([netdatabot](https://github.com/netdatabot))
- static install: bump openssl and curl to latest stable versions [\#18719](https://github.com/netdata/netdata/pull/18719) ([ilyam8](https://github.com/ilyam8))
- go.d: use lib function to check if stderr connected to journal [\#18718](https://github.com/netdata/netdata/pull/18718) ([ilyam8](https://github.com/ilyam8))
- Pass correct GOOS and GOARCH on to package builders in CI. [\#18717](https://github.com/netdata/netdata/pull/18717) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18715](https://github.com/netdata/netdata/pull/18715) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#18714](https://github.com/netdata/netdata/pull/18714) ([netdatabot](https://github.com/netdatabot))
- Add link to meta section on integrations template [\#18713](https://github.com/netdata/netdata/pull/18713) ([Ancairon](https://github.com/Ancairon))
- Delay child disconnect update [\#18712](https://github.com/netdata/netdata/pull/18712) ([stelfrag](https://github.com/stelfrag))
- Windows installer \(Change descriptions add helping\) [\#18711](https://github.com/netdata/netdata/pull/18711) ([thiagoftsm](https://github.com/thiagoftsm))
- add instructions to configure SCIM integration in Okta [\#18710](https://github.com/netdata/netdata/pull/18710) ([juacker](https://github.com/juacker))
- fix wrong config file name in go.d/oracledb meta [\#18709](https://github.com/netdata/netdata/pull/18709) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18708](https://github.com/netdata/netdata/pull/18708) ([netdatabot](https://github.com/netdatabot))
- feat\(go.d/sensors\): add a config option to update/add sensor label value [\#18707](https://github.com/netdata/netdata/pull/18707) ([ilyam8](https://github.com/ilyam8))
- improve apps.plugin readme [\#18705](https://github.com/netdata/netdata/pull/18705) ([ilyam8](https://github.com/ilyam8))
- Update windows documentation [\#18703](https://github.com/netdata/netdata/pull/18703) ([Ancairon](https://github.com/Ancairon))
- Detect when swap is disabled when agent is running [\#18702](https://github.com/netdata/netdata/pull/18702) ([stelfrag](https://github.com/stelfrag))
- Bump golang.org/x/net from 0.29.0 to 0.30.0 in /src/go [\#18701](https://github.com/netdata/netdata/pull/18701) ([dependabot[bot]](https://github.com/apps/dependabot))
- fix system-info disk space in LXC [\#18696](https://github.com/netdata/netdata/pull/18696) ([ilyam8](https://github.com/ilyam8))
- fix ram usage calculation in LXC [\#18695](https://github.com/netdata/netdata/pull/18695) ([ilyam8](https://github.com/ilyam8))
- cgroups.plugin: call `setresuid` before spawn server init [\#18694](https://github.com/netdata/netdata/pull/18694) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18693](https://github.com/netdata/netdata/pull/18693) ([netdatabot](https://github.com/netdatabot))
- go.d/nvidia\_smi: use configured "timeout" in loop mode [\#18692](https://github.com/netdata/netdata/pull/18692) ([ilyam8](https://github.com/ilyam8))
- fix\(cgroups.plugin\): handle containers no env vars [\#18691](https://github.com/netdata/netdata/pull/18691) ([daniel-sampliner](https://github.com/daniel-sampliner))
- MSSQL Metrics \(Part II\). [\#18689](https://github.com/netdata/netdata/pull/18689) ([thiagoftsm](https://github.com/thiagoftsm))
- fix sanitization issues [\#18687](https://github.com/netdata/netdata/pull/18687) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18686](https://github.com/netdata/netdata/pull/18686) ([netdatabot](https://github.com/netdatabot))
- go.d/chrony: collect serverstats using chronyc [\#18685](https://github.com/netdata/netdata/pull/18685) ([ilyam8](https://github.com/ilyam8))
- UTF8 support for chart ids, names and other metadata [\#18684](https://github.com/netdata/netdata/pull/18684) ([ktsaou](https://github.com/ktsaou))
- Send node info update after ACLK connection timeout [\#18683](https://github.com/netdata/netdata/pull/18683) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18682](https://github.com/netdata/netdata/pull/18682) ([netdatabot](https://github.com/netdatabot))
- Bump github.com/tidwall/gjson from 1.17.3 to 1.18.0 in /src/go [\#18681](https://github.com/netdata/netdata/pull/18681) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.59.1 to 0.60.0 in /src/go [\#18680](https://github.com/netdata/netdata/pull/18680) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump go.mongodb.org/mongo-driver from 1.17.0 to 1.17.1 in /src/go [\#18679](https://github.com/netdata/netdata/pull/18679) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d downgrade go-ora to v2.8.20 [\#18677](https://github.com/netdata/netdata/pull/18677) ([ilyam8](https://github.com/ilyam8))
- Docs fixes [\#18676](https://github.com/netdata/netdata/pull/18676) ([Ancairon](https://github.com/Ancairon))
- Apps plugin improvements2 [\#18673](https://github.com/netdata/netdata/pull/18673) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18672](https://github.com/netdata/netdata/pull/18672) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#18671](https://github.com/netdata/netdata/pull/18671) ([netdatabot](https://github.com/netdatabot))
- src dir docs pass [\#18670](https://github.com/netdata/netdata/pull/18670) ([Ancairon](https://github.com/Ancairon))
- Remove section in python plugin readme [\#18669](https://github.com/netdata/netdata/pull/18669) ([Ancairon](https://github.com/Ancairon))
- Properly set start/shutdown times to parent/child [\#18668](https://github.com/netdata/netdata/pull/18668) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18667](https://github.com/netdata/netdata/pull/18667) ([netdatabot](https://github.com/netdatabot))
- apps\_groups.conf: add oracledb [\#18666](https://github.com/netdata/netdata/pull/18666) ([ilyam8](https://github.com/ilyam8))
- Docs lint on `packaging/` dir [\#18665](https://github.com/netdata/netdata/pull/18665) ([Ancairon](https://github.com/Ancairon))
- Add FAQ to SCIM integration doc [\#18664](https://github.com/netdata/netdata/pull/18664) ([juacker](https://github.com/juacker))
- Fix win apps uptime [\#18662](https://github.com/netdata/netdata/pull/18662) ([ktsaou](https://github.com/ktsaou))
- Embed CPU architecture info in Windows installer filename. [\#18661](https://github.com/netdata/netdata/pull/18661) ([Ferroin](https://github.com/Ferroin))
- Docs directory lint documentation and fix issues [\#18660](https://github.com/netdata/netdata/pull/18660) ([Ancairon](https://github.com/Ancairon))
- bump go toolchain v1.22.8 [\#18659](https://github.com/netdata/netdata/pull/18659) ([ilyam8](https://github.com/ilyam8))
- go.d sd fix sprig funcmap [\#18658](https://github.com/netdata/netdata/pull/18658) ([ilyam8](https://github.com/ilyam8))
- Adjust content api/v1/info \(Windows\) [\#18656](https://github.com/netdata/netdata/pull/18656) ([thiagoftsm](https://github.com/thiagoftsm))
- add go.d/oracle [\#18654](https://github.com/netdata/netdata/pull/18654) ([ilyam8](https://github.com/ilyam8))
- Handle mqtt ping timeouts [\#18653](https://github.com/netdata/netdata/pull/18653) ([stelfrag](https://github.com/stelfrag))
- apps.plugin improvements [\#18652](https://github.com/netdata/netdata/pull/18652) ([ktsaou](https://github.com/ktsaou))
- remove python implementation of oracledb [\#18651](https://github.com/netdata/netdata/pull/18651) ([Ancairon](https://github.com/Ancairon))
- go.d remove duplicate chart check in tests [\#18650](https://github.com/netdata/netdata/pull/18650) ([ilyam8](https://github.com/ilyam8))
- Improve windows installer [\#18649](https://github.com/netdata/netdata/pull/18649) ([thiagoftsm](https://github.com/thiagoftsm))
- fixed freebsd cpu calculation [\#18648](https://github.com/netdata/netdata/pull/18648) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18647](https://github.com/netdata/netdata/pull/18647) ([netdatabot](https://github.com/netdatabot))
- Use temporary file for commit date check. [\#18646](https://github.com/netdata/netdata/pull/18646) ([Ferroin](https://github.com/Ferroin))
- New wording about edit-config script in docs [\#18639](https://github.com/netdata/netdata/pull/18639) ([Ancairon](https://github.com/Ancairon))
- Update file names. [\#18638](https://github.com/netdata/netdata/pull/18638) ([vkalintiris](https://github.com/vkalintiris))
- Move plugins.d directory outside of collectors [\#18637](https://github.com/netdata/netdata/pull/18637) ([vkalintiris](https://github.com/vkalintiris))
- go.d/smartctl: fix exit status check in scan [\#18635](https://github.com/netdata/netdata/pull/18635) ([ilyam8](https://github.com/ilyam8))
- go.d pkg/socket: keep only one timeout option [\#18633](https://github.com/netdata/netdata/pull/18633) ([ilyam8](https://github.com/ilyam8))
- Log  agent start / stop timing events [\#18632](https://github.com/netdata/netdata/pull/18632) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18630](https://github.com/netdata/netdata/pull/18630) ([netdatabot](https://github.com/netdatabot))
- go.d/postgres: fix checkpoints query for postgres 17 [\#18629](https://github.com/netdata/netdata/pull/18629) ([ilyam8](https://github.com/ilyam8))
- go.d/ceph: fix leftovers after \#18582 [\#18628](https://github.com/netdata/netdata/pull/18628) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18627](https://github.com/netdata/netdata/pull/18627) ([netdatabot](https://github.com/netdatabot))
- Remove Python OpenLDAP implementation [\#18626](https://github.com/netdata/netdata/pull/18626) ([Ancairon](https://github.com/Ancairon))
- Port the OpenLDAP collector from Python to Go [\#18625](https://github.com/netdata/netdata/pull/18625) ([Ancairon](https://github.com/Ancairon))
- Change default pages per extent [\#18623](https://github.com/netdata/netdata/pull/18623) ([stelfrag](https://github.com/stelfrag))
- Misc mqtt related code cleanup [\#18622](https://github.com/netdata/netdata/pull/18622) ([stelfrag](https://github.com/stelfrag))
- Revert "Add ceph commands to ndsudo" [\#18620](https://github.com/netdata/netdata/pull/18620) ([ilyam8](https://github.com/ilyam8))
- go.d/hddtemp: connect and read [\#18619](https://github.com/netdata/netdata/pull/18619) ([ilyam8](https://github.com/ilyam8))
- go.d/uwsgi: don't write just connect and read [\#18618](https://github.com/netdata/netdata/pull/18618) ([ilyam8](https://github.com/ilyam8))
- Windows Installer \(Silent mode\) [\#18613](https://github.com/netdata/netdata/pull/18613) ([thiagoftsm](https://github.com/thiagoftsm))
- POST Functions [\#18611](https://github.com/netdata/netdata/pull/18611) ([ktsaou](https://github.com/ktsaou))
- Correctly include Windows installer in release creation. [\#18609](https://github.com/netdata/netdata/pull/18609) ([Ferroin](https://github.com/Ferroin))
- feat: HW req for onprem installation. [\#18608](https://github.com/netdata/netdata/pull/18608) ([M4itee](https://github.com/M4itee))
- WEB SERVER: retry sending data when errno is EAGAIN [\#18607](https://github.com/netdata/netdata/pull/18607) ([ktsaou](https://github.com/ktsaou))
- Publish Windows installers on nightly builds. [\#18603](https://github.com/netdata/netdata/pull/18603) ([Ferroin](https://github.com/Ferroin))
- Bump github.com/docker/docker from 27.3.0+incompatible to 27.3.1+incompatible in /src/go [\#18600](https://github.com/netdata/netdata/pull/18600) ([dependabot[bot]](https://github.com/apps/dependabot))
- Windows Plugin metadata [\#18599](https://github.com/netdata/netdata/pull/18599) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18598](https://github.com/netdata/netdata/pull/18598) ([netdatabot](https://github.com/netdatabot))
- go.d/sensors fix meta [\#18597](https://github.com/netdata/netdata/pull/18597) ([ilyam8](https://github.com/ilyam8))
- go.d/sensors update meta [\#18595](https://github.com/netdata/netdata/pull/18595) ([ilyam8](https://github.com/ilyam8))
- apps.plugin for windows [\#18594](https://github.com/netdata/netdata/pull/18594) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18592](https://github.com/netdata/netdata/pull/18592) ([netdatabot](https://github.com/netdatabot))
- Add MSSQL metrics \(Part I\). [\#18591](https://github.com/netdata/netdata/pull/18591) ([thiagoftsm](https://github.com/thiagoftsm))
- Add DLLs to CmakeLists.txt [\#18590](https://github.com/netdata/netdata/pull/18590) ([thiagoftsm](https://github.com/thiagoftsm))
- Bump go.mongodb.org/mongo-driver from 1.16.1 to 1.17.0 in /src/go [\#18589](https://github.com/netdata/netdata/pull/18589) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.2.1+incompatible to 27.3.0+incompatible in /src/go [\#18588](https://github.com/netdata/netdata/pull/18588) ([dependabot[bot]](https://github.com/apps/dependabot))
- Update kickstart.sh [\#18587](https://github.com/netdata/netdata/pull/18587) ([eya46](https://github.com/eya46))
- Remove python ceph collector implementation [\#18584](https://github.com/netdata/netdata/pull/18584) ([Ancairon](https://github.com/Ancairon))
- Add ceph commands to ndsudo [\#18583](https://github.com/netdata/netdata/pull/18583) ([Ancairon](https://github.com/Ancairon))
- Port Ceph collector to Go [\#18582](https://github.com/netdata/netdata/pull/18582) ([Ancairon](https://github.com/Ancairon))
- go.d/sensors refactor [\#18581](https://github.com/netdata/netdata/pull/18581) ([ilyam8](https://github.com/ilyam8))
- go.d move packages [\#18580](https://github.com/netdata/netdata/pull/18580) ([ilyam8](https://github.com/ilyam8))
- WEIGHTS: use node\_id when available, otherwise host\_id [\#18579](https://github.com/netdata/netdata/pull/18579) ([ktsaou](https://github.com/ktsaou))
- go.d/portcheck: update status duration calculation [\#18577](https://github.com/netdata/netdata/pull/18577) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18576](https://github.com/netdata/netdata/pull/18576) ([netdatabot](https://github.com/netdatabot))
- go.d/portcheck schema add tabs [\#18575](https://github.com/netdata/netdata/pull/18575) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18574](https://github.com/netdata/netdata/pull/18574) ([netdatabot](https://github.com/netdatabot))
- go.d portcheck update meta [\#18573](https://github.com/netdata/netdata/pull/18573) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18571](https://github.com/netdata/netdata/pull/18571) ([netdatabot](https://github.com/netdatabot))
- go.d sd docker: remove unnecessary info message [\#18570](https://github.com/netdata/netdata/pull/18570) ([ilyam8](https://github.com/ilyam8))
- go.d/portcheck: add UDP support [\#18569](https://github.com/netdata/netdata/pull/18569) ([ilyam8](https://github.com/ilyam8))
- Reduce connection timeout and fallback to IPV4 for ACLK connections [\#18568](https://github.com/netdata/netdata/pull/18568) ([stelfrag](https://github.com/stelfrag))
- Windows Events Log improvements 4 [\#18567](https://github.com/netdata/netdata/pull/18567) ([ktsaou](https://github.com/ktsaou))
- windows.plugin \(IIS\) [\#18566](https://github.com/netdata/netdata/pull/18566) ([thiagoftsm](https://github.com/thiagoftsm))
- Add check for 64bit builtin atomics [\#18565](https://github.com/netdata/netdata/pull/18565) ([kraj](https://github.com/kraj))
- Windows Events Log Explorer improvements 3 [\#18564](https://github.com/netdata/netdata/pull/18564) ([ktsaou](https://github.com/ktsaou))
- Windows Events Improvements 2 [\#18563](https://github.com/netdata/netdata/pull/18563) ([ktsaou](https://github.com/ktsaou))
- add cpu model to host labels [\#18562](https://github.com/netdata/netdata/pull/18562) ([ilyam8](https://github.com/ilyam8))
- go.d rename example =\> testrandom [\#18561](https://github.com/netdata/netdata/pull/18561) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18560](https://github.com/netdata/netdata/pull/18560) ([netdatabot](https://github.com/netdatabot))
- go.d/prometheus: add label\_prefix config option [\#18559](https://github.com/netdata/netdata/pull/18559) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18558](https://github.com/netdata/netdata/pull/18558) ([netdatabot](https://github.com/netdatabot))
- add nginx unit to apps\_groups.conf [\#18557](https://github.com/netdata/netdata/pull/18557) ([ilyam8](https://github.com/ilyam8))
- go.d fix typesense/nginxunit meta [\#18556](https://github.com/netdata/netdata/pull/18556) ([ilyam8](https://github.com/ilyam8))
- add go.d/nginxunit [\#18554](https://github.com/netdata/netdata/pull/18554) ([ilyam8](https://github.com/ilyam8))
- fix some docs issues [\#18553](https://github.com/netdata/netdata/pull/18553) ([ilyam8](https://github.com/ilyam8))
- go.d fix Goland code inspection warnings [\#18552](https://github.com/netdata/netdata/pull/18552) ([ilyam8](https://github.com/ilyam8))
- Bump k8s.io/client-go from 0.31.0 to 0.31.1 in /src/go [\#18549](https://github.com/netdata/netdata/pull/18549) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d move doing http req logic to web [\#18546](https://github.com/netdata/netdata/pull/18546) ([ilyam8](https://github.com/ilyam8))
- go.d pkg web renames [\#18545](https://github.com/netdata/netdata/pull/18545) ([ilyam8](https://github.com/ilyam8))
- go.d fix duplicate closeBody func [\#18544](https://github.com/netdata/netdata/pull/18544) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18543](https://github.com/netdata/netdata/pull/18543) ([netdatabot](https://github.com/netdatabot))
- go.d typesense: fix name in meta [\#18542](https://github.com/netdata/netdata/pull/18542) ([ilyam8](https://github.com/ilyam8))
- Misc code cleanup [\#18540](https://github.com/netdata/netdata/pull/18540) ([stelfrag](https://github.com/stelfrag))
- go.d add typesense collector [\#18538](https://github.com/netdata/netdata/pull/18538) ([ilyam8](https://github.com/ilyam8))
- add typesense to apps\_groups.conf [\#18537](https://github.com/netdata/netdata/pull/18537) ([ilyam8](https://github.com/ilyam8))
- Fetch metadata by hash for DEB repos. [\#18536](https://github.com/netdata/netdata/pull/18536) ([Ferroin](https://github.com/Ferroin))
- go.d snmp change label name organization-\>vendor [\#18535](https://github.com/netdata/netdata/pull/18535) ([ilyam8](https://github.com/ilyam8))
- go.d snmp fix vnode host labels [\#18534](https://github.com/netdata/netdata/pull/18534) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/vmware/govmomi from 0.42.0 to 0.43.0 in /src/go [\#18532](https://github.com/netdata/netdata/pull/18532) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d add vnode guid validation [\#18531](https://github.com/netdata/netdata/pull/18531) ([ilyam8](https://github.com/ilyam8))
- go.d snmp handle multiline sysDescr [\#18530](https://github.com/netdata/netdata/pull/18530) ([ilyam8](https://github.com/ilyam8))
- go.d/snmp: add "organization" label \(vnode\) [\#18529](https://github.com/netdata/netdata/pull/18529) ([ilyam8](https://github.com/ilyam8))
- Windows Events Improvements 1 [\#18528](https://github.com/netdata/netdata/pull/18528) ([ktsaou](https://github.com/ktsaou))
- go.d snmp: add sys descr, contact and loc as host labels for vnode [\#18527](https://github.com/netdata/netdata/pull/18527) ([ilyam8](https://github.com/ilyam8))
- Add charts for TCPv4/TCPV6/ICMP errors in windows [\#18526](https://github.com/netdata/netdata/pull/18526) ([stelfrag](https://github.com/stelfrag))
- Windows Events: recalculate the length of unicode strings returned every time [\#18525](https://github.com/netdata/netdata/pull/18525) ([ktsaou](https://github.com/ktsaou))
- go.d snmp add private enterprise numbers mapping [\#18523](https://github.com/netdata/netdata/pull/18523) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18522](https://github.com/netdata/netdata/pull/18522) ([netdatabot](https://github.com/netdatabot))
- go.d/snmp: add an option to automatically create vnode [\#18520](https://github.com/netdata/netdata/pull/18520) ([ilyam8](https://github.com/ilyam8))
- remove save-database from netdatacli usage [\#18519](https://github.com/netdata/netdata/pull/18519) ([ilyam8](https://github.com/ilyam8))
- improve netdatacli docs [\#18518](https://github.com/netdata/netdata/pull/18518) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18517](https://github.com/netdata/netdata/pull/18517) ([netdatabot](https://github.com/netdatabot))
- go.d/varnish update meta [\#18516](https://github.com/netdata/netdata/pull/18516) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/jackc/pgx/v5 from 5.7.0 to 5.7.1 in /src/go [\#18515](https://github.com/netdata/netdata/pull/18515) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d update redis lib to v9 [\#18513](https://github.com/netdata/netdata/pull/18513) ([ilyam8](https://github.com/ilyam8))
- go.d/varnish: add docker support [\#18512](https://github.com/netdata/netdata/pull/18512) ([ilyam8](https://github.com/ilyam8))
- go.d add function to execute a command inside a Docker container [\#18509](https://github.com/netdata/netdata/pull/18509) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18508](https://github.com/netdata/netdata/pull/18508) ([netdatabot](https://github.com/netdatabot))
- server dashboard v3 static files, when available [\#18507](https://github.com/netdata/netdata/pull/18507) ([ktsaou](https://github.com/ktsaou))
- add varnishstat and varnishadm to ndsudo [\#18503](https://github.com/netdata/netdata/pull/18503) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/docker/docker from 27.2.0+incompatible to 27.2.1+incompatible in /src/go [\#18502](https://github.com/netdata/netdata/pull/18502) ([dependabot[bot]](https://github.com/apps/dependabot))
- Assorted build cleanup for external data collection plugins. [\#18501](https://github.com/netdata/netdata/pull/18501) ([Ferroin](https://github.com/Ferroin))
- remove python.d/varnish [\#18499](https://github.com/netdata/netdata/pull/18499) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/jackc/pgx/v5 from 5.6.0 to 5.7.0 in /src/go [\#18498](https://github.com/netdata/netdata/pull/18498) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.58.0 to 0.59.1 in /src/go [\#18497](https://github.com/netdata/netdata/pull/18497) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/net from 0.28.0 to 0.29.0 in /src/go [\#18496](https://github.com/netdata/netdata/pull/18496) ([dependabot[bot]](https://github.com/apps/dependabot))
- Windows Plugin Metrics \(Thermal and Memory\) [\#18494](https://github.com/netdata/netdata/pull/18494) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18493](https://github.com/netdata/netdata/pull/18493) ([netdatabot](https://github.com/netdatabot))
- varnish collector Go implementation [\#18491](https://github.com/netdata/netdata/pull/18491) ([Ancairon](https://github.com/Ancairon))
- add go.d/apcupsd [\#18489](https://github.com/netdata/netdata/pull/18489) ([ilyam8](https://github.com/ilyam8))
- Improve processing on removed alerts after agent restart [\#18488](https://github.com/netdata/netdata/pull/18488) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/prometheus/common from 0.57.0 to 0.58.0 in /src/go [\#18487](https://github.com/netdata/netdata/pull/18487) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/text from 0.17.0 to 0.18.0 in /src/go [\#18486](https://github.com/netdata/netdata/pull/18486) ([dependabot[bot]](https://github.com/apps/dependabot))
- Remove Warnings \(ebpf\) [\#18484](https://github.com/netdata/netdata/pull/18484) ([thiagoftsm](https://github.com/thiagoftsm))
- \[WIP\] Windows-Events Logs Explorer [\#18483](https://github.com/netdata/netdata/pull/18483) ([ktsaou](https://github.com/ktsaou))
- fix win sysinfo installed ram calculation [\#18482](https://github.com/netdata/netdata/pull/18482) ([ilyam8](https://github.com/ilyam8))
- remove charts.d/apcupsd [\#18481](https://github.com/netdata/netdata/pull/18481) ([ilyam8](https://github.com/ilyam8))
- Update LIbbpf [\#18480](https://github.com/netdata/netdata/pull/18480) ([thiagoftsm](https://github.com/thiagoftsm))
- added missing comma in Access-Control-Allow-Headers [\#18479](https://github.com/netdata/netdata/pull/18479) ([ktsaou](https://github.com/ktsaou))
- add Access-Control-Allow-Headers: x-transaction-id [\#18478](https://github.com/netdata/netdata/pull/18478) ([ktsaou](https://github.com/ktsaou))
- add Access-Control-Allow-Headers: x-netdata-auth [\#18477](https://github.com/netdata/netdata/pull/18477) ([ktsaou](https://github.com/ktsaou))
- prevent sigsegv in config-parsers [\#18476](https://github.com/netdata/netdata/pull/18476) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18475](https://github.com/netdata/netdata/pull/18475) ([netdatabot](https://github.com/netdatabot))
- added version to systemd-journal info response [\#18474](https://github.com/netdata/netdata/pull/18474) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18473](https://github.com/netdata/netdata/pull/18473) ([netdatabot](https://github.com/netdatabot))
- Remove w1sensor in favor of Go implementation [\#18471](https://github.com/netdata/netdata/pull/18471) ([Ancairon](https://github.com/Ancairon))
- Improve processing of pending alerts [\#18470](https://github.com/netdata/netdata/pull/18470) ([stelfrag](https://github.com/stelfrag))
- Fix node index in alerts [\#18469](https://github.com/netdata/netdata/pull/18469) ([stelfrag](https://github.com/stelfrag))
- go.d storcli: fix unmarshal driveInfo [\#18466](https://github.com/netdata/netdata/pull/18466) ([ilyam8](https://github.com/ilyam8))
- w1sensor collector Go implementation [\#18464](https://github.com/netdata/netdata/pull/18464) ([Ancairon](https://github.com/Ancairon))
- Check correct number of bits for LZC of XOR value. [\#18463](https://github.com/netdata/netdata/pull/18463) ([vkalintiris](https://github.com/vkalintiris))
- netdata-claim.sh: fix parsing url arg [\#18460](https://github.com/netdata/netdata/pull/18460) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/likexian/whois from 1.15.4 to 1.15.5 in /src/go [\#18457](https://github.com/netdata/netdata/pull/18457) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/likexian/whois-parser from 1.24.19 to 1.24.20 in /src/go [\#18456](https://github.com/netdata/netdata/pull/18456) ([dependabot[bot]](https://github.com/apps/dependabot))
- Cleanup, rename and packaging fix \(Windows Codes\) [\#18455](https://github.com/netdata/netdata/pull/18455) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18454](https://github.com/netdata/netdata/pull/18454) ([netdatabot](https://github.com/netdatabot))
- Bump github.com/Masterminds/sprig/v3 from 3.2.3 to 3.3.0 in /src/go [\#18453](https://github.com/netdata/netdata/pull/18453) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.55.0 to 0.57.0 in /src/go [\#18452](https://github.com/netdata/netdata/pull/18452) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.1.2+incompatible to 27.2.0+incompatible in /src/go [\#18451](https://github.com/netdata/netdata/pull/18451) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#18450](https://github.com/netdata/netdata/pull/18450) ([netdatabot](https://github.com/netdatabot))
- go.d sensors add parsing intrusion to exec method [\#18449](https://github.com/netdata/netdata/pull/18449) ([ilyam8](https://github.com/ilyam8))
- Exit slabinfo.plugin on EPIPE [\#18448](https://github.com/netdata/netdata/pull/18448) ([teqwve](https://github.com/teqwve))
- ilert Integration [\#18447](https://github.com/netdata/netdata/pull/18447) ([DaTiMy](https://github.com/DaTiMy))
- go.d remove vnode disable [\#18446](https://github.com/netdata/netdata/pull/18446) ([ilyam8](https://github.com/ilyam8))
- go.d add support for symlinked vnode config files [\#18445](https://github.com/netdata/netdata/pull/18445) ([ilyam8](https://github.com/ilyam8))
- Proper precedence when calculating time\_to\_evict [\#18444](https://github.com/netdata/netdata/pull/18444) ([stelfrag](https://github.com/stelfrag))
- Windows Permissions [\#18443](https://github.com/netdata/netdata/pull/18443) ([thiagoftsm](https://github.com/thiagoftsm))
- do not free the sender when the sender thread exits [\#18441](https://github.com/netdata/netdata/pull/18441) ([ktsaou](https://github.com/ktsaou))
- fix receiver deadlock [\#18440](https://github.com/netdata/netdata/pull/18440) ([ktsaou](https://github.com/ktsaou))
- fix charts.d/sensors leftovers [\#18439](https://github.com/netdata/netdata/pull/18439) ([ilyam8](https://github.com/ilyam8))
- remove deadlock from sender [\#18438](https://github.com/netdata/netdata/pull/18438) ([ktsaou](https://github.com/ktsaou))
- go.d remove duplicates in testing [\#18435](https://github.com/netdata/netdata/pull/18435) ([ilyam8](https://github.com/ilyam8))
- Improve agent shutdown time [\#18434](https://github.com/netdata/netdata/pull/18434) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18432](https://github.com/netdata/netdata/pull/18432) ([netdatabot](https://github.com/netdatabot))
- go.d/sensors: add sysfs scan method to collect metrics [\#18431](https://github.com/netdata/netdata/pull/18431) ([ilyam8](https://github.com/ilyam8))
- stream paths propagated to children and parents [\#18430](https://github.com/netdata/netdata/pull/18430) ([ktsaou](https://github.com/ktsaou))
- go.d lmsensors improve performance [\#18429](https://github.com/netdata/netdata/pull/18429) ([ilyam8](https://github.com/ilyam8))
- ci fix InvalidDefaultArgInFrom warn [\#18428](https://github.com/netdata/netdata/pull/18428) ([ilyam8](https://github.com/ilyam8))
- vendor https://github.com/mdlayher/lmsensors [\#18427](https://github.com/netdata/netdata/pull/18427) ([ilyam8](https://github.com/ilyam8))
- remove charts.d/sensors [\#18426](https://github.com/netdata/netdata/pull/18426) ([ilyam8](https://github.com/ilyam8))
- Reset last connected when removing stale nodes with netdatacli [\#18425](https://github.com/netdata/netdata/pull/18425) ([stelfrag](https://github.com/stelfrag))
- remove checks.plugin dir [\#18424](https://github.com/netdata/netdata/pull/18424) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18421](https://github.com/netdata/netdata/pull/18421) ([netdatabot](https://github.com/netdatabot))
- fix hyperlink in go.d samba meta [\#18420](https://github.com/netdata/netdata/pull/18420) ([ilyam8](https://github.com/ilyam8))
- add go.d samba [\#18418](https://github.com/netdata/netdata/pull/18418) ([ilyam8](https://github.com/ilyam8))
- ACLK code cleanup [\#18417](https://github.com/netdata/netdata/pull/18417) ([stelfrag](https://github.com/stelfrag))
- restore /api/v1/badge.svg [\#18416](https://github.com/netdata/netdata/pull/18416) ([ktsaou](https://github.com/ktsaou))
- add "smbstatus -P" to ndsudo [\#18414](https://github.com/netdata/netdata/pull/18414) ([ilyam8](https://github.com/ilyam8))
- remove python.d/sambsa [\#18413](https://github.com/netdata/netdata/pull/18413) ([ilyam8](https://github.com/ilyam8))
- SPAWN-SERVER: re-evaluate signals even 500ms [\#18411](https://github.com/netdata/netdata/pull/18411) ([ktsaou](https://github.com/ktsaou))
- Claim on Windows [\#18410](https://github.com/netdata/netdata/pull/18410) ([thiagoftsm](https://github.com/thiagoftsm))
- kickstart: fix write\_claim\_config when executed as a regular user [\#18406](https://github.com/netdata/netdata/pull/18406) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#18405](https://github.com/netdata/netdata/pull/18405) ([stelfrag](https://github.com/stelfrag))
- remove pyyaml2 [\#18404](https://github.com/netdata/netdata/pull/18404) ([ilyam8](https://github.com/ilyam8))
- imporve netdatacli help usage readability [\#18403](https://github.com/netdata/netdata/pull/18403) ([ilyam8](https://github.com/ilyam8))
- remove python.d/anomalies [\#18402](https://github.com/netdata/netdata/pull/18402) ([ilyam8](https://github.com/ilyam8))
- go.d dnsmasqdhcp: fix potential panic in parseDHCPRangeValue [\#18401](https://github.com/netdata/netdata/pull/18401) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18400](https://github.com/netdata/netdata/pull/18400) ([netdatabot](https://github.com/netdatabot))
- go.d boinc [\#18398](https://github.com/netdata/netdata/pull/18398) ([ilyam8](https://github.com/ilyam8))
- remove python.d/boinc [\#18397](https://github.com/netdata/netdata/pull/18397) ([ilyam8](https://github.com/ilyam8))
- fix warnings in Dockerfile [\#18395](https://github.com/netdata/netdata/pull/18395) ([NicolasCARPi](https://github.com/NicolasCARPi))
- Use existing ACLK event loop for cloud queries [\#18218](https://github.com/netdata/netdata/pull/18218) ([stelfrag](https://github.com/stelfrag))

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

**Merged pull requests:**

- go.d dnsmsasq\_dhcp: improve parsing of dhcp ranges [\#18394](https://github.com/netdata/netdata/pull/18394) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18391](https://github.com/netdata/netdata/pull/18391) ([netdatabot](https://github.com/netdatabot))
- remove proc zfspools [\#18389](https://github.com/netdata/netdata/pull/18389) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18387](https://github.com/netdata/netdata/pull/18387) ([netdatabot](https://github.com/netdatabot))
- Modify CLI command remove-stale-node to accept hostname [\#18386](https://github.com/netdata/netdata/pull/18386) ([stelfrag](https://github.com/stelfrag))
- Update windows installer [\#18385](https://github.com/netdata/netdata/pull/18385) ([thiagoftsm](https://github.com/thiagoftsm))
- go.d zfspool: collect vdev health state [\#18383](https://github.com/netdata/netdata/pull/18383) ([ilyam8](https://github.com/ilyam8))
- Remove debug message [\#18382](https://github.com/netdata/netdata/pull/18382) ([stelfrag](https://github.com/stelfrag))
- Remove host immediately on stale node removal [\#18381](https://github.com/netdata/netdata/pull/18381) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18380](https://github.com/netdata/netdata/pull/18380) ([netdatabot](https://github.com/netdatabot))
- go.d docs: add a note that debug mode not supported for Dyncfg jobs [\#18379](https://github.com/netdata/netdata/pull/18379) ([ilyam8](https://github.com/ilyam8))
- ci gen integrations: add cloud-authentication dir [\#18378](https://github.com/netdata/netdata/pull/18378) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18377](https://github.com/netdata/netdata/pull/18377) ([netdatabot](https://github.com/netdatabot))
- go.d dnsmasq: query metrics individually to handle v2.90+ SERVFAIL [\#18376](https://github.com/netdata/netdata/pull/18376) ([ilyam8](https://github.com/ilyam8))
- Switch to DEB822 format for APT repository configuration. [\#18374](https://github.com/netdata/netdata/pull/18374) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18373](https://github.com/netdata/netdata/pull/18373) ([netdatabot](https://github.com/netdatabot))
- Origin-sign all DEB packages regardless of upload target. [\#18372](https://github.com/netdata/netdata/pull/18372) ([Ferroin](https://github.com/Ferroin))
- remove python.d/changefinder [\#18370](https://github.com/netdata/netdata/pull/18370) ([ilyam8](https://github.com/ilyam8))
- remove python.d/example [\#18369](https://github.com/netdata/netdata/pull/18369) ([ilyam8](https://github.com/ilyam8))
- go.d squidlog: improve parser init and parsing [\#18368](https://github.com/netdata/netdata/pull/18368) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/axiomhq/hyperloglog from 0.0.0-20240507144631-af9851f82b27 to 0.1.0 in /src/go [\#18367](https://github.com/netdata/netdata/pull/18367) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.40.0 to 0.42.0 in /src/go [\#18366](https://github.com/netdata/netdata/pull/18366) ([dependabot[bot]](https://github.com/apps/dependabot))
- eBPF \(reduce CPU and memory usage\) [\#18365](https://github.com/netdata/netdata/pull/18365) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18363](https://github.com/netdata/netdata/pull/18363) ([netdatabot](https://github.com/netdatabot))
- add go.d/tor [\#18361](https://github.com/netdata/netdata/pull/18361) ([ilyam8](https://github.com/ilyam8))
- remove python.d/tor [\#18358](https://github.com/netdata/netdata/pull/18358) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18357](https://github.com/netdata/netdata/pull/18357) ([netdatabot](https://github.com/netdatabot))
- remove python.d lm\_sensors.py [\#18356](https://github.com/netdata/netdata/pull/18356) ([ilyam8](https://github.com/ilyam8))
- remove python.d/retroshare [\#18355](https://github.com/netdata/netdata/pull/18355) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18353](https://github.com/netdata/netdata/pull/18353) ([netdatabot](https://github.com/netdatabot))
- go.d httpcheck: add status description to docs [\#18351](https://github.com/netdata/netdata/pull/18351) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18350](https://github.com/netdata/netdata/pull/18350) ([netdatabot](https://github.com/netdatabot))
- Add missing initial slashes for internal documation links [\#18348](https://github.com/netdata/netdata/pull/18348) ([ralphm](https://github.com/ralphm))
- fix sending CLEAR notifications with critical severity modifier [\#18347](https://github.com/netdata/netdata/pull/18347) ([ilyam8](https://github.com/ilyam8))
- add license to readmes menu [\#18345](https://github.com/netdata/netdata/pull/18345) ([ilyam8](https://github.com/ilyam8))
- add go.d/monit [\#18344](https://github.com/netdata/netdata/pull/18344) ([ilyam8](https://github.com/ilyam8))
- remove python.d/monit [\#18343](https://github.com/netdata/netdata/pull/18343) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/docker/docker from 27.1.1+incompatible to 27.1.2+incompatible in /src/go [\#18340](https://github.com/netdata/netdata/pull/18340) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.39.0 to 0.40.0 in /src/go [\#18338](https://github.com/netdata/netdata/pull/18338) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/miekg/dns from 1.1.61 to 1.1.62 in /src/go [\#18337](https://github.com/netdata/netdata/pull/18337) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump k8s.io/client-go from 0.30.3 to 0.31.0 in /src/go [\#18336](https://github.com/netdata/netdata/pull/18336) ([dependabot[bot]](https://github.com/apps/dependabot))
- add i2pd to apps\_groups.conf [\#18335](https://github.com/netdata/netdata/pull/18335) ([ilyam8](https://github.com/ilyam8))
- add dashboard v2 license to readme [\#18334](https://github.com/netdata/netdata/pull/18334) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18333](https://github.com/netdata/netdata/pull/18333) ([netdatabot](https://github.com/netdatabot))
- go.d riakkv [\#18330](https://github.com/netdata/netdata/pull/18330) ([ilyam8](https://github.com/ilyam8))
- remove python.d/riakkv [\#18329](https://github.com/netdata/netdata/pull/18329) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18328](https://github.com/netdata/netdata/pull/18328) ([netdatabot](https://github.com/netdatabot))
- add go.d/uwsgi [\#18326](https://github.com/netdata/netdata/pull/18326) ([ilyam8](https://github.com/ilyam8))
- remove python.d/uwsgi [\#18325](https://github.com/netdata/netdata/pull/18325) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18324](https://github.com/netdata/netdata/pull/18324) ([netdatabot](https://github.com/netdatabot))
- remove python.d/dovecot [\#18322](https://github.com/netdata/netdata/pull/18322) ([ilyam8](https://github.com/ilyam8))
- add go.d dovecot [\#18321](https://github.com/netdata/netdata/pull/18321) ([ilyam8](https://github.com/ilyam8))
- go.d redis: fix default "address" in config\_schema.json [\#18320](https://github.com/netdata/netdata/pull/18320) ([ilyam8](https://github.com/ilyam8))
- Ensure files in /usr/lib/netdata/system are not executable. [\#18318](https://github.com/netdata/netdata/pull/18318) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18317](https://github.com/netdata/netdata/pull/18317) ([netdatabot](https://github.com/netdatabot))
- remove python.d/nvidia\_smi [\#18316](https://github.com/netdata/netdata/pull/18316) ([ilyam8](https://github.com/ilyam8))
- go.d nvidia\_smi: enable by default [\#18315](https://github.com/netdata/netdata/pull/18315) ([ilyam8](https://github.com/ilyam8))
- go.d nvidia\_smi: add loop mode [\#18313](https://github.com/netdata/netdata/pull/18313) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18312](https://github.com/netdata/netdata/pull/18312) ([netdatabot](https://github.com/netdatabot))
- go.d nvidia\_smi remove "csv" mode [\#18311](https://github.com/netdata/netdata/pull/18311) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18308](https://github.com/netdata/netdata/pull/18308) ([netdatabot](https://github.com/netdatabot))
- add go.d/exim [\#18306](https://github.com/netdata/netdata/pull/18306) ([ilyam8](https://github.com/ilyam8))
- remove python.d/exim [\#18305](https://github.com/netdata/netdata/pull/18305) ([ilyam8](https://github.com/ilyam8))
- add exim to ndsudo [\#18304](https://github.com/netdata/netdata/pull/18304) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18303](https://github.com/netdata/netdata/pull/18303) ([netdatabot](https://github.com/netdatabot))
- add go.d/nsd [\#18302](https://github.com/netdata/netdata/pull/18302) ([ilyam8](https://github.com/ilyam8))
- add nsd-control to ndsudo [\#18301](https://github.com/netdata/netdata/pull/18301) ([ilyam8](https://github.com/ilyam8))
- remove python.d/nsd [\#18300](https://github.com/netdata/netdata/pull/18300) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18299](https://github.com/netdata/netdata/pull/18299) ([netdatabot](https://github.com/netdatabot))
- go.d gearman fix meta [\#18298](https://github.com/netdata/netdata/pull/18298) ([ilyam8](https://github.com/ilyam8))
- Handle GOROOT inside build system instead of outside. [\#18296](https://github.com/netdata/netdata/pull/18296) ([Ferroin](https://github.com/Ferroin))
- add go.d/gearman [\#18294](https://github.com/netdata/netdata/pull/18294) ([ilyam8](https://github.com/ilyam8))
- Use system certificate configuration for Yum/DNF repos. [\#18293](https://github.com/netdata/netdata/pull/18293) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18292](https://github.com/netdata/netdata/pull/18292) ([netdatabot](https://github.com/netdatabot))
- remove python.d/gearman [\#18291](https://github.com/netdata/netdata/pull/18291) ([ilyam8](https://github.com/ilyam8))
- remove python.d/alarms [\#18290](https://github.com/netdata/netdata/pull/18290) ([ilyam8](https://github.com/ilyam8))
- go.d rethinkdb fix cluster\_servers\_stats\_request [\#18289](https://github.com/netdata/netdata/pull/18289) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/gosnmp/gosnmp from 1.37.0 to 1.38.0 in /src/go [\#18287](https://github.com/netdata/netdata/pull/18287) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump go.mongodb.org/mongo-driver from 1.16.0 to 1.16.1 in /src/go [\#18286](https://github.com/netdata/netdata/pull/18286) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/net from 0.27.0 to 0.28.0 in /src/go [\#18284](https://github.com/netdata/netdata/pull/18284) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#18282](https://github.com/netdata/netdata/pull/18282) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#18280](https://github.com/netdata/netdata/pull/18280) ([netdatabot](https://github.com/netdatabot))
- Remove python squid collector implementation  [\#18279](https://github.com/netdata/netdata/pull/18279) ([Ancairon](https://github.com/Ancairon))
- add go.d/rethinkdb [\#18278](https://github.com/netdata/netdata/pull/18278) ([ilyam8](https://github.com/ilyam8))
- remove python.d/rethinkdb [\#18277](https://github.com/netdata/netdata/pull/18277) ([ilyam8](https://github.com/ilyam8))
- Squid collector port to Go [\#18276](https://github.com/netdata/netdata/pull/18276) ([Ancairon](https://github.com/Ancairon))
- set GOPROXY when building go.d.plugin [\#18275](https://github.com/netdata/netdata/pull/18275) ([ilyam8](https://github.com/ilyam8))
- go.d snmp: adjust max repetitions automatically [\#18274](https://github.com/netdata/netdata/pull/18274) ([ilyam8](https://github.com/ilyam8))
- go.d fix dimension id check [\#18272](https://github.com/netdata/netdata/pull/18272) ([ilyam8](https://github.com/ilyam8))
- go.d smartctl: undo extra\_devices skip from \#18269 [\#18270](https://github.com/netdata/netdata/pull/18270) ([ilyam8](https://github.com/ilyam8))
- go.d smartctl: improve checking scsi-sat in scan [\#18269](https://github.com/netdata/netdata/pull/18269) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18268](https://github.com/netdata/netdata/pull/18268) ([netdatabot](https://github.com/netdatabot))
- apps conf add beanstalkd [\#18267](https://github.com/netdata/netdata/pull/18267) ([ilyam8](https://github.com/ilyam8))
- Fix CI issues in build workflow. [\#18266](https://github.com/netdata/netdata/pull/18266) ([Ferroin](https://github.com/Ferroin))
- Add detailed reporting of failed checksums in kickstart script. [\#18265](https://github.com/netdata/netdata/pull/18265) ([Ferroin](https://github.com/Ferroin))
- go.d beanstalk [\#18263](https://github.com/netdata/netdata/pull/18263) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18262](https://github.com/netdata/netdata/pull/18262) ([netdatabot](https://github.com/netdatabot))
- go.d x509check add not\_revoked dimension [\#18261](https://github.com/netdata/netdata/pull/18261) ([ilyam8](https://github.com/ilyam8))
- remove python.d/beanstalk [\#18259](https://github.com/netdata/netdata/pull/18259) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18258](https://github.com/netdata/netdata/pull/18258) ([netdatabot](https://github.com/netdatabot))
- docs: improve "Settings on Microsoft Teams" description [\#18257](https://github.com/netdata/netdata/pull/18257) ([ilyam8](https://github.com/ilyam8))
- docs: add a note that the min dbengine tier size is 256 MB [\#18256](https://github.com/netdata/netdata/pull/18256) ([ilyam8](https://github.com/ilyam8))
- Update Installer Code \(Services\) [\#18253](https://github.com/netdata/netdata/pull/18253) ([thiagoftsm](https://github.com/thiagoftsm))
- Don’t install netdata-updater code on Windows. [\#18251](https://github.com/netdata/netdata/pull/18251) ([Ferroin](https://github.com/Ferroin))
- Add trigger to clean up chart labels and charts [\#18248](https://github.com/netdata/netdata/pull/18248) ([stelfrag](https://github.com/stelfrag))
- Update README.md [\#18246](https://github.com/netdata/netdata/pull/18246) ([Steve8291](https://github.com/Steve8291))
- Bump github.com/tidwall/gjson from 1.17.1 to 1.17.3 in /src/go [\#18244](https://github.com/netdata/netdata/pull/18244) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix OS detection messages in CMake. [\#18243](https://github.com/netdata/netdata/pull/18243) ([Ferroin](https://github.com/Ferroin))
- Clean up unneeded depencdencies. [\#18242](https://github.com/netdata/netdata/pull/18242) ([Ferroin](https://github.com/Ferroin))
- Update node info payload [\#18240](https://github.com/netdata/netdata/pull/18240) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/prometheus-community/pro-bing from 0.4.0 to 0.4.1 in /src/go [\#18237](https://github.com/netdata/netdata/pull/18237) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.38.0 to 0.39.0 in /src/go [\#18236](https://github.com/netdata/netdata/pull/18236) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/gofrs/flock from 0.12.0 to 0.12.1 in /src/go [\#18235](https://github.com/netdata/netdata/pull/18235) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.0.3+incompatible to 27.1.1+incompatible in /src/go [\#18234](https://github.com/netdata/netdata/pull/18234) ([dependabot[bot]](https://github.com/apps/dependabot))
- eBPF memory [\#18232](https://github.com/netdata/netdata/pull/18232) ([thiagoftsm](https://github.com/thiagoftsm))
- Windows build fix [\#18231](https://github.com/netdata/netdata/pull/18231) ([Ferroin](https://github.com/Ferroin))
- Default to release with debug symbols for Windows builds. [\#18230](https://github.com/netdata/netdata/pull/18230) ([Ferroin](https://github.com/Ferroin))
- Fix up CMake feature handling for Windows. [\#18229](https://github.com/netdata/netdata/pull/18229) ([Ferroin](https://github.com/Ferroin))
- Improve windows agent [\#18227](https://github.com/netdata/netdata/pull/18227) ([thiagoftsm](https://github.com/thiagoftsm))
- Update libbpf \(1.45.0\) [\#18226](https://github.com/netdata/netdata/pull/18226) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18225](https://github.com/netdata/netdata/pull/18225) ([netdatabot](https://github.com/netdatabot))
- Update Cloud MSTeam documentation [\#18224](https://github.com/netdata/netdata/pull/18224) ([car12o](https://github.com/car12o))
- Add code signing for Windows executables. [\#18222](https://github.com/netdata/netdata/pull/18222) ([Ferroin](https://github.com/Ferroin))
- go.d fix netdata dir path on win under msys [\#18221](https://github.com/netdata/netdata/pull/18221) ([ilyam8](https://github.com/ilyam8))
- Port Tomcat collector to Go [\#18220](https://github.com/netdata/netdata/pull/18220) ([Ancairon](https://github.com/Ancairon))
- go.d drop using cancelreader [\#18219](https://github.com/netdata/netdata/pull/18219) ([ilyam8](https://github.com/ilyam8))
- Support IPV6 when establishing MQTT connection to the cloud [\#18217](https://github.com/netdata/netdata/pull/18217) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18216](https://github.com/netdata/netdata/pull/18216) ([netdatabot](https://github.com/netdatabot))
- go.d chrony fix client read/write timeout [\#18215](https://github.com/netdata/netdata/pull/18215) ([ilyam8](https://github.com/ilyam8))
- dont install test bash scripts by default [\#18214](https://github.com/netdata/netdata/pull/18214) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18212](https://github.com/netdata/netdata/pull/18212) ([netdatabot](https://github.com/netdatabot))
- go.d megacli: add bbu capacity degradation % [\#18211](https://github.com/netdata/netdata/pull/18211) ([ilyam8](https://github.com/ilyam8))
- Port memcached collector to Go [\#18209](https://github.com/netdata/netdata/pull/18209) ([Ancairon](https://github.com/Ancairon))
- Bump k8s.io/client-go from 0.30.2 to 0.30.3 in /src/go [\#18208](https://github.com/netdata/netdata/pull/18208) ([dependabot[bot]](https://github.com/apps/dependabot))
- docs: simplify "Disk Requirements and Retention" [\#18205](https://github.com/netdata/netdata/pull/18205) ([ilyam8](https://github.com/ilyam8))
- do not rely on the queued flag to queue a context [\#18198](https://github.com/netdata/netdata/pull/18198) ([ktsaou](https://github.com/ktsaou))
- Do not include REMOVED status in the alert snapshot [\#18197](https://github.com/netdata/netdata/pull/18197) ([stelfrag](https://github.com/stelfrag))

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
