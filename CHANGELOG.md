# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.3...HEAD)

**Merged pull requests:**

- Bump github.com/tidwall/gjson from 1.17.1 to 1.17.3 in /src/go [\#18244](https://github.com/netdata/netdata/pull/18244) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix OS detection messages in CMake. [\#18243](https://github.com/netdata/netdata/pull/18243) ([Ferroin](https://github.com/Ferroin))
- Clean up unneeded depencdencies. [\#18242](https://github.com/netdata/netdata/pull/18242) ([Ferroin](https://github.com/Ferroin))
- Update node info payload [\#18240](https://github.com/netdata/netdata/pull/18240) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/prometheus-community/pro-bing from 0.4.0 to 0.4.1 in /src/go [\#18237](https://github.com/netdata/netdata/pull/18237) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.38.0 to 0.39.0 in /src/go [\#18236](https://github.com/netdata/netdata/pull/18236) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/gofrs/flock from 0.12.0 to 0.12.1 in /src/go [\#18235](https://github.com/netdata/netdata/pull/18235) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.0.3+incompatible to 27.1.1+incompatible in /src/go [\#18234](https://github.com/netdata/netdata/pull/18234) ([dependabot[bot]](https://github.com/apps/dependabot))
- Windows build fix [\#18231](https://github.com/netdata/netdata/pull/18231) ([Ferroin](https://github.com/Ferroin))
- Default to release with debug symbols for Windows builds. [\#18230](https://github.com/netdata/netdata/pull/18230) ([Ferroin](https://github.com/Ferroin))
- Fix up CMake feature handling for Windows. [\#18229](https://github.com/netdata/netdata/pull/18229) ([Ferroin](https://github.com/Ferroin))
- Improve windows agent [\#18227](https://github.com/netdata/netdata/pull/18227) ([thiagoftsm](https://github.com/thiagoftsm))
- Update libbpf \(1.45.0\) [\#18226](https://github.com/netdata/netdata/pull/18226) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#18225](https://github.com/netdata/netdata/pull/18225) ([netdatabot](https://github.com/netdatabot))
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
- remove fluent-bit submodule [\#18196](https://github.com/netdata/netdata/pull/18196) ([ilyam8](https://github.com/ilyam8))
- go.d icecast single source response [\#18195](https://github.com/netdata/netdata/pull/18195) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18193](https://github.com/netdata/netdata/pull/18193) ([netdatabot](https://github.com/netdatabot))
- Skip building ndsudo when it’s not actually needed. [\#18191](https://github.com/netdata/netdata/pull/18191) ([Ferroin](https://github.com/Ferroin))
- Port Icecast collector to Go [\#18190](https://github.com/netdata/netdata/pull/18190) ([Ancairon](https://github.com/Ancairon))
- ndsudo setuid before looking for command [\#18189](https://github.com/netdata/netdata/pull/18189) ([ilyam8](https://github.com/ilyam8))
- Add Widnows CI jobs. [\#18187](https://github.com/netdata/netdata/pull/18187) ([Ferroin](https://github.com/Ferroin))
- Remove logs-management plugin. [\#18186](https://github.com/netdata/netdata/pull/18186) ([Ferroin](https://github.com/Ferroin))
- Create release branches for major releases as well. [\#18184](https://github.com/netdata/netdata/pull/18184) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18183](https://github.com/netdata/netdata/pull/18183) ([netdatabot](https://github.com/netdatabot))
- docs: go.d/ap update data\_collection description [\#18182](https://github.com/netdata/netdata/pull/18182) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18181](https://github.com/netdata/netdata/pull/18181) ([netdatabot](https://github.com/netdatabot))
- go.d smarctl simplify scan open [\#18180](https://github.com/netdata/netdata/pull/18180) ([ilyam8](https://github.com/ilyam8))
- Addition to postfix meta [\#18177](https://github.com/netdata/netdata/pull/18177) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#18176](https://github.com/netdata/netdata/pull/18176) ([netdatabot](https://github.com/netdatabot))
- docs: add "install smartmontools" for Docker netdata [\#18175](https://github.com/netdata/netdata/pull/18175) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18174](https://github.com/netdata/netdata/pull/18174) ([netdatabot](https://github.com/netdatabot))
- add support for sending Telegram notifications to topics [\#18173](https://github.com/netdata/netdata/pull/18173) ([ilyam8](https://github.com/ilyam8))
- Fix logic bug in platform EOL check code. [\#18172](https://github.com/netdata/netdata/pull/18172) ([Ferroin](https://github.com/Ferroin))
- Fix issue in platform EOL check workflow. [\#18171](https://github.com/netdata/netdata/pull/18171) ([Ferroin](https://github.com/Ferroin))
- Port AP collector to Go [\#18170](https://github.com/netdata/netdata/pull/18170) ([Ancairon](https://github.com/Ancairon))
- Bump github.com/likexian/whois from 1.15.3 to 1.15.4 in /src/go [\#18169](https://github.com/netdata/netdata/pull/18169) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/lmittmann/tint from 1.0.4 to 1.0.5 in /src/go [\#18167](https://github.com/netdata/netdata/pull/18167) ([dependabot[bot]](https://github.com/apps/dependabot))
- Personalize installer and uninstaller Windows \(Control Panel\) [\#18147](https://github.com/netdata/netdata/pull/18147) ([thiagoftsm](https://github.com/thiagoftsm))
- go.d smartctl: use scan-open when "no\_check\_power\_mode" is "never" [\#18146](https://github.com/netdata/netdata/pull/18146) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18145](https://github.com/netdata/netdata/pull/18145) ([netdatabot](https://github.com/netdatabot))
- go.d smartctl: do scan only once on startup if interval is 0 [\#18144](https://github.com/netdata/netdata/pull/18144) ([ilyam8](https://github.com/ilyam8))
- ndsudo add smartctl scan-open [\#18143](https://github.com/netdata/netdata/pull/18143) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18141](https://github.com/netdata/netdata/pull/18141) ([netdatabot](https://github.com/netdatabot))
- go.d smartctl add "extra\_devices" option [\#18140](https://github.com/netdata/netdata/pull/18140) ([ilyam8](https://github.com/ilyam8))
- Spawn server fixes 6 [\#18136](https://github.com/netdata/netdata/pull/18136) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18135](https://github.com/netdata/netdata/pull/18135) ([netdatabot](https://github.com/netdatabot))
- docs: go.d mysql: remove unix sockets from auto\_detection [\#18134](https://github.com/netdata/netdata/pull/18134) ([ilyam8](https://github.com/ilyam8))
- go.d fix url path overwrite [\#18132](https://github.com/netdata/netdata/pull/18132) ([ilyam8](https://github.com/ilyam8))
- Spawn server improvements 5 [\#18131](https://github.com/netdata/netdata/pull/18131) ([ktsaou](https://github.com/ktsaou))
- Spawn server fixes No 4 [\#18127](https://github.com/netdata/netdata/pull/18127) ([ktsaou](https://github.com/ktsaou))
- go.d filecheck fix dir existence chart label [\#18126](https://github.com/netdata/netdata/pull/18126) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18124](https://github.com/netdata/netdata/pull/18124) ([netdatabot](https://github.com/netdatabot))
- go.d whoisquery fix "days until" in config\_schema.json [\#18121](https://github.com/netdata/netdata/pull/18121) ([ilyam8](https://github.com/ilyam8))
- go.d smartctl: add scsi read/write/verify error rate [\#18119](https://github.com/netdata/netdata/pull/18119) ([ilyam8](https://github.com/ilyam8))
- log in the same line [\#18118](https://github.com/netdata/netdata/pull/18118) ([ktsaou](https://github.com/ktsaou))
- spawn server fixes 3 [\#18117](https://github.com/netdata/netdata/pull/18117) ([ktsaou](https://github.com/ktsaou))
- make claiming work again [\#18116](https://github.com/netdata/netdata/pull/18116) ([ktsaou](https://github.com/ktsaou))
- spawn server improvements [\#18115](https://github.com/netdata/netdata/pull/18115) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18114](https://github.com/netdata/netdata/pull/18114) ([netdatabot](https://github.com/netdatabot))
- Fix detection of Coverity archive path in scan script. [\#18112](https://github.com/netdata/netdata/pull/18112) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18110](https://github.com/netdata/netdata/pull/18110) ([netdatabot](https://github.com/netdatabot))
- move "api key" option in stream.conf [\#18108](https://github.com/netdata/netdata/pull/18108) ([ilyam8](https://github.com/ilyam8))
- add port service names to network viewer [\#18107](https://github.com/netdata/netdata/pull/18107) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#18106](https://github.com/netdata/netdata/pull/18106) ([netdatabot](https://github.com/netdatabot))
- docs: deploy docker add host root mount \(stable\) [\#18105](https://github.com/netdata/netdata/pull/18105) ([ilyam8](https://github.com/ilyam8))
- Fix detection of Coverity archive in scan script. [\#18104](https://github.com/netdata/netdata/pull/18104) ([Ferroin](https://github.com/Ferroin))
- add authorization to spawn requests [\#18103](https://github.com/netdata/netdata/pull/18103) ([ktsaou](https://github.com/ktsaou))
- Bump CMake supported versions. [\#18102](https://github.com/netdata/netdata/pull/18102) ([Ferroin](https://github.com/Ferroin))
- Stop a bit more quickly on a failure when using Ninja. [\#18101](https://github.com/netdata/netdata/pull/18101) ([Ferroin](https://github.com/Ferroin))
- Fix network sent dimensions [\#18099](https://github.com/netdata/netdata/pull/18099) ([stelfrag](https://github.com/stelfrag))
- go.d use pgx v4 for pgbouncer [\#18097](https://github.com/netdata/netdata/pull/18097) ([ilyam8](https://github.com/ilyam8))
- Update securing-netdata-agents.md [\#18096](https://github.com/netdata/netdata/pull/18096) ([yoursweetginger](https://github.com/yoursweetginger))
- ndsudo set uid/gid/egid to 0 before executing command [\#18093](https://github.com/netdata/netdata/pull/18093) ([ilyam8](https://github.com/ilyam8))
- go.d fix compiling for windows [\#18091](https://github.com/netdata/netdata/pull/18091) ([ilyam8](https://github.com/ilyam8))
- go.d megacli: return error if no adapters found \(parsing failed\) [\#18090](https://github.com/netdata/netdata/pull/18090) ([ilyam8](https://github.com/ilyam8))
- Port puppet collector from Python to Go [\#18088](https://github.com/netdata/netdata/pull/18088) ([Ancairon](https://github.com/Ancairon))
- update go.d path in docs and ci [\#18087](https://github.com/netdata/netdata/pull/18087) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18086](https://github.com/netdata/netdata/pull/18086) ([netdatabot](https://github.com/netdatabot))
- Switch to legacy images for CentOS 7 CI. [\#18085](https://github.com/netdata/netdata/pull/18085) ([Ferroin](https://github.com/Ferroin))
- Track LTS for Debian EOL status. [\#18084](https://github.com/netdata/netdata/pull/18084) ([Ferroin](https://github.com/Ferroin))
- Remove Debian 10 from supported platforms. [\#18083](https://github.com/netdata/netdata/pull/18083) ([Ferroin](https://github.com/Ferroin))
- Remove Ubuntu 23.10 from supported platforms. [\#18082](https://github.com/netdata/netdata/pull/18082) ([Ferroin](https://github.com/Ferroin))
- go.d fail2ban: add docker support [\#18081](https://github.com/netdata/netdata/pull/18081) ([ilyam8](https://github.com/ilyam8))
- Improve alerts [\#18080](https://github.com/netdata/netdata/pull/18080) ([stelfrag](https://github.com/stelfrag))
- Bump golang.org/x/net from 0.26.0 to 0.27.0 in /src/go [\#18078](https://github.com/netdata/netdata/pull/18078) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/gofrs/flock from 0.11.0 to 0.12.0 in /src/go [\#18077](https://github.com/netdata/netdata/pull/18077) ([dependabot[bot]](https://github.com/apps/dependabot))
- proc: collect ksm/swap/cma/zswap only when feature enabled [\#18076](https://github.com/netdata/netdata/pull/18076) ([ilyam8](https://github.com/ilyam8))
- health add alarm docker container down [\#18075](https://github.com/netdata/netdata/pull/18075) ([ilyam8](https://github.com/ilyam8))
- go.d ipfs fix tests [\#18074](https://github.com/netdata/netdata/pull/18074) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18073](https://github.com/netdata/netdata/pull/18073) ([netdatabot](https://github.com/netdatabot))
- Port ipfs from python to Go [\#18070](https://github.com/netdata/netdata/pull/18070) ([Ancairon](https://github.com/Ancairon))
- update golang version in netdata.spec [\#18069](https://github.com/netdata/netdata/pull/18069) ([ilyam8](https://github.com/ilyam8))
- go.d set sensitive props to "password" widget [\#18068](https://github.com/netdata/netdata/pull/18068) ([ilyam8](https://github.com/ilyam8))
- netdata.spec/plugin-go: added weak dependency for lm\_sensors [\#18067](https://github.com/netdata/netdata/pull/18067) ([k0ste](https://github.com/k0ste))
- Disable health thread on windows [\#18066](https://github.com/netdata/netdata/pull/18066) ([stelfrag](https://github.com/stelfrag))
- Remove hard-coded url from python.d puppet chart plugin [\#18064](https://github.com/netdata/netdata/pull/18064) ([Hufschmidt](https://github.com/Hufschmidt))
- go.d postgres github.com/jackc/pgx/v5 [\#18062](https://github.com/netdata/netdata/pull/18062) ([ilyam8](https://github.com/ilyam8))
- fix prometeus export: missing comma before "instance" label [\#18061](https://github.com/netdata/netdata/pull/18061) ([ilyam8](https://github.com/ilyam8))
- go.d vsphere add update\_every ui:help [\#18060](https://github.com/netdata/netdata/pull/18060) ([ilyam8](https://github.com/ilyam8))
- restructure go.d [\#18058](https://github.com/netdata/netdata/pull/18058) ([ilyam8](https://github.com/ilyam8))
- freeipmi: add "no-restart" \(workaround \#17931\) [\#18057](https://github.com/netdata/netdata/pull/18057) ([ilyam8](https://github.com/ilyam8))
- ndsudo add 'chronyc serverstats' [\#18056](https://github.com/netdata/netdata/pull/18056) ([ilyam8](https://github.com/ilyam8))
- go.d chrony add serverstats query \(disabled for now\) [\#18055](https://github.com/netdata/netdata/pull/18055) ([ilyam8](https://github.com/ilyam8))
- go.d update packages [\#18054](https://github.com/netdata/netdata/pull/18054) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18052](https://github.com/netdata/netdata/pull/18052) ([netdatabot](https://github.com/netdatabot))
- docs: deploy docker add host root mount [\#18051](https://github.com/netdata/netdata/pull/18051) ([ilyam8](https://github.com/ilyam8))
- Update role-based-access-model.md [\#18050](https://github.com/netdata/netdata/pull/18050) ([Ancairon](https://github.com/Ancairon))
- Bump github.com/prometheus/common from 0.54.0 to 0.55.0 in /src/go/collectors/go.d.plugin [\#18049](https://github.com/netdata/netdata/pull/18049) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump go.mongodb.org/mongo-driver from 1.15.1 to 1.16.0 in /src/go/collectors/go.d.plugin [\#18048](https://github.com/netdata/netdata/pull/18048) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 27.0.0+incompatible to 27.0.2+incompatible in /src/go/collectors/go.d.plugin [\#18047](https://github.com/netdata/netdata/pull/18047) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/likexian/whois-parser from 1.24.16 to 1.24.18 in /src/go/collectors/go.d.plugin [\#18046](https://github.com/netdata/netdata/pull/18046) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/gofrs/flock from 0.8.1 to 0.11.0 in /src/go/collectors/go.d.plugin [\#18045](https://github.com/netdata/netdata/pull/18045) ([dependabot[bot]](https://github.com/apps/dependabot))
- Semaphore \(common context\) [\#18041](https://github.com/netdata/netdata/pull/18041) ([thiagoftsm](https://github.com/thiagoftsm))
- proc/diskstats: Increase accuracy of average IO operation time [\#18040](https://github.com/netdata/netdata/pull/18040) ([ilyam8](https://github.com/ilyam8))
- diskspace: update exclude paths/filesystems [\#18039](https://github.com/netdata/netdata/pull/18039) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18038](https://github.com/netdata/netdata/pull/18038) ([netdatabot](https://github.com/netdatabot))
- docs: fix go.d/weblog parser config [\#18037](https://github.com/netdata/netdata/pull/18037) ([ilyam8](https://github.com/ilyam8))
- fix diskspace plugin in Docker [\#18035](https://github.com/netdata/netdata/pull/18035) ([ilyam8](https://github.com/ilyam8))
- Bump repository config fetched by kickstart to latest version. [\#18034](https://github.com/netdata/netdata/pull/18034) ([Ferroin](https://github.com/Ferroin))
- fix installing netdata-updater svc/timer for native packages [\#18032](https://github.com/netdata/netdata/pull/18032) ([ilyam8](https://github.com/ilyam8))
- Troubleshooter must be assigned to rooms docs [\#18031](https://github.com/netdata/netdata/pull/18031) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#18030](https://github.com/netdata/netdata/pull/18030) ([netdatabot](https://github.com/netdatabot))
- go.d/postfix: simplify and fix tests [\#18029](https://github.com/netdata/netdata/pull/18029) ([ilyam8](https://github.com/ilyam8))
- go.d k8state: skip jobs/cronjobs Pods [\#18028](https://github.com/netdata/netdata/pull/18028) ([ilyam8](https://github.com/ilyam8))
- Port postfix collector from python to go [\#18026](https://github.com/netdata/netdata/pull/18026) ([Ancairon](https://github.com/Ancairon))
- alert prototype: set default "after" to -600 [\#18025](https://github.com/netdata/netdata/pull/18025) ([ilyam8](https://github.com/ilyam8))
- Fix Coverity scan CI. [\#18024](https://github.com/netdata/netdata/pull/18024) ([Ferroin](https://github.com/Ferroin))
- go.d snmp: add config options to filter interfaces by name and type [\#18023](https://github.com/netdata/netdata/pull/18023) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#18022](https://github.com/netdata/netdata/pull/18022) ([netdatabot](https://github.com/netdatabot))
- fix grep warning in kickstart [\#18021](https://github.com/netdata/netdata/pull/18021) ([ilyam8](https://github.com/ilyam8))
- ping meta fix configuring ping\_group\_range [\#18020](https://github.com/netdata/netdata/pull/18020) ([ilyam8](https://github.com/ilyam8))
- Improve global statistics thread shutdown [\#18018](https://github.com/netdata/netdata/pull/18018) ([stelfrag](https://github.com/stelfrag))
- Fix proxy connect response [\#18017](https://github.com/netdata/netdata/pull/18017) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#18016](https://github.com/netdata/netdata/pull/18016) ([netdatabot](https://github.com/netdatabot))
- go.d snmp: add collecting network interface stats [\#18014](https://github.com/netdata/netdata/pull/18014) ([ilyam8](https://github.com/ilyam8))
- rrdlabels: allow uppercase A-Z in label name [\#18013](https://github.com/netdata/netdata/pull/18013) ([ilyam8](https://github.com/ilyam8))
- Fix Slack error reporting for packaging workflows. [\#18011](https://github.com/netdata/netdata/pull/18011) ([Ferroin](https://github.com/Ferroin))
- Enforce proper include ordering for vendored libraries. [\#18008](https://github.com/netdata/netdata/pull/18008) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#18006](https://github.com/netdata/netdata/pull/18006) ([netdatabot](https://github.com/netdatabot))
- docs: add Troubleshoot-\>Getting Logs section to collectors [\#18005](https://github.com/netdata/netdata/pull/18005) ([ilyam8](https://github.com/ilyam8))
- apps.plugin: remove "Normalization Ratio" internal charts [\#18004](https://github.com/netdata/netdata/pull/18004) ([ilyam8](https://github.com/ilyam8))
- Fix RPM repoconfig naming [\#18003](https://github.com/netdata/netdata/pull/18003) ([Ferroin](https://github.com/Ferroin))
- Explicitly disable logsmanagement plugin on known-broken environments. [\#18002](https://github.com/netdata/netdata/pull/18002) ([Ferroin](https://github.com/Ferroin))
- update netdata global stats and enable them by default [\#18001](https://github.com/netdata/netdata/pull/18001) ([ilyam8](https://github.com/ilyam8))
- go.d whoisquery change  default days until expiration 90/30 =\> 30/15 [\#18000](https://github.com/netdata/netdata/pull/18000) ([ilyam8](https://github.com/ilyam8))
- health convert value to days in calc in whoisquery/x509check alarms [\#17999](https://github.com/netdata/netdata/pull/17999) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17986](https://github.com/netdata/netdata/pull/17986) ([netdatabot](https://github.com/netdatabot))
- docs: smartctl: add the "no\_check\_power\_mode" option [\#17985](https://github.com/netdata/netdata/pull/17985) ([ilyam8](https://github.com/ilyam8))
- docs: update "What's New and Coming?" [\#17984](https://github.com/netdata/netdata/pull/17984) ([ilyam8](https://github.com/ilyam8))
- Change logging to debug  [\#17983](https://github.com/netdata/netdata/pull/17983) ([stelfrag](https://github.com/stelfrag))
- improve ping\_host\_reachable alert calc [\#17982](https://github.com/netdata/netdata/pull/17982) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17981](https://github.com/netdata/netdata/pull/17981) ([netdatabot](https://github.com/netdatabot))
- docs: add "For Netdata running in a Docker container" to go.d/smartcl [\#17980](https://github.com/netdata/netdata/pull/17980) ([ilyam8](https://github.com/ilyam8))
- go.d docker respect DOCKER\_HOST env var [\#17979](https://github.com/netdata/netdata/pull/17979) ([ilyam8](https://github.com/ilyam8))
- fix go.d rspamd unexpected response check [\#17974](https://github.com/netdata/netdata/pull/17974) ([ilyam8](https://github.com/ilyam8))
- make cgroups version detection more reliable [\#17973](https://github.com/netdata/netdata/pull/17973) ([ilyam8](https://github.com/ilyam8))
- go.d dyncfg add job name validation [\#17971](https://github.com/netdata/netdata/pull/17971) ([ilyam8](https://github.com/ilyam8))
- cgroups: fix cgroups version detection on non-systemd nodes with cgroupv1 [\#17969](https://github.com/netdata/netdata/pull/17969) ([ilyam8](https://github.com/ilyam8))
- go.d postgres index name replace space [\#17968](https://github.com/netdata/netdata/pull/17968) ([ilyam8](https://github.com/ilyam8))
- go.d replace colon in job name [\#17967](https://github.com/netdata/netdata/pull/17967) ([ilyam8](https://github.com/ilyam8))
- Fix space percentage calculation in dbengine retention chart [\#17963](https://github.com/netdata/netdata/pull/17963) ([stelfrag](https://github.com/stelfrag))
- Tidy-up build related CI jobs. [\#17962](https://github.com/netdata/netdata/pull/17962) ([Ferroin](https://github.com/Ferroin))
- Sign DEB packages in the GHA runners that build them. [\#17949](https://github.com/netdata/netdata/pull/17949) ([Ferroin](https://github.com/Ferroin))
- Detect on startup if the netdata-meta.db file is not a valid database file [\#17924](https://github.com/netdata/netdata/pull/17924) ([stelfrag](https://github.com/stelfrag))
- eBPF cgroup and mutex [\#17915](https://github.com/netdata/netdata/pull/17915) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix small typo [\#17875](https://github.com/netdata/netdata/pull/17875) ([stelfrag](https://github.com/stelfrag))
- spawn server \(Windows support for external plugins\) [\#17866](https://github.com/netdata/netdata/pull/17866) ([ktsaou](https://github.com/ktsaou))
- sysinfo \(WinAPI\) [\#17857](https://github.com/netdata/netdata/pull/17857) ([thiagoftsm](https://github.com/thiagoftsm))
- Run the agent as a Windows service. [\#17766](https://github.com/netdata/netdata/pull/17766) ([vkalintiris](https://github.com/vkalintiris))
- add Win CPU interrupts [\#17753](https://github.com/netdata/netdata/pull/17753) ([thiagoftsm](https://github.com/thiagoftsm))
- Relax strict version constraints for DEB package dependencies. [\#17681](https://github.com/netdata/netdata/pull/17681) ([Ferroin](https://github.com/Ferroin))

## [v1.46.3](https://github.com/netdata/netdata/tree/v1.46.3) (2024-07-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.2...v1.46.3)

## [v1.46.2](https://github.com/netdata/netdata/tree/v1.46.2) (2024-07-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.1...v1.46.2)

## [v1.46.1](https://github.com/netdata/netdata/tree/v1.46.1) (2024-06-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.46.0...v1.46.1)

## [v1.46.0](https://github.com/netdata/netdata/tree/v1.46.0) (2024-06-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.6...v1.46.0)

**Merged pull requests:**

- fix apcupsd status "slave" [\#17961](https://github.com/netdata/netdata/pull/17961) ([ilyam8](https://github.com/ilyam8))
- fix apcupsd status [\#17960](https://github.com/netdata/netdata/pull/17960) ([ilyam8](https://github.com/ilyam8))
- docs: clarify setting time/disk limits to 0 [\#17958](https://github.com/netdata/netdata/pull/17958) ([ilyam8](https://github.com/ilyam8))
- docs: add time-based retention to "Change how long Netdata stores metrics" [\#17957](https://github.com/netdata/netdata/pull/17957) ([ilyam8](https://github.com/ilyam8))
- go.d systemdunits: remove "omitempty" tag from collect\_unit\_files [\#17956](https://github.com/netdata/netdata/pull/17956) ([ilyam8](https://github.com/ilyam8))
- fix installing netdata journald conf for native packages [\#17954](https://github.com/netdata/netdata/pull/17954) ([ilyam8](https://github.com/ilyam8))
- remove Discord badge \(rate limited by upstream service\) [\#17953](https://github.com/netdata/netdata/pull/17953) ([ilyam8](https://github.com/ilyam8))
- rename env var for extended internal monitoring [\#17951](https://github.com/netdata/netdata/pull/17951) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17950](https://github.com/netdata/netdata/pull/17950) ([netdatabot](https://github.com/netdatabot))
- fix indentation in go.d/dnsquery conf [\#17948](https://github.com/netdata/netdata/pull/17948) ([ilyam8](https://github.com/ilyam8))
- go.d add dmcache collector [\#17947](https://github.com/netdata/netdata/pull/17947) ([ilyam8](https://github.com/ilyam8))
- add "dmsetup status --target cache --noflush" to ndsudo [\#17946](https://github.com/netdata/netdata/pull/17946) ([ilyam8](https://github.com/ilyam8))
- Fix disk max calculation [\#17945](https://github.com/netdata/netdata/pull/17945) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#17944](https://github.com/netdata/netdata/pull/17944) ([netdatabot](https://github.com/netdatabot))
- Update enable-an-exporting-connector.md [\#17943](https://github.com/netdata/netdata/pull/17943) ([Ancairon](https://github.com/Ancairon))
- add OpenSearch to exporting prom meta [\#17942](https://github.com/netdata/netdata/pull/17942) ([ilyam8](https://github.com/ilyam8))
- Fix warnings [\#17940](https://github.com/netdata/netdata/pull/17940) ([thiagoftsm](https://github.com/thiagoftsm))
- update bundled UI to v6.138.3 [\#17939](https://github.com/netdata/netdata/pull/17939) ([ilyam8](https://github.com/ilyam8))
- go.d storcli add initial support for mpt3sas controllers [\#17938](https://github.com/netdata/netdata/pull/17938) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17937](https://github.com/netdata/netdata/pull/17937) ([netdatabot](https://github.com/netdatabot))
- Adds GreptimeDB to prometheus metadata [\#17936](https://github.com/netdata/netdata/pull/17936) ([killme2008](https://github.com/killme2008))
- go.d smartctl: don't log found devices on every scan [\#17934](https://github.com/netdata/netdata/pull/17934) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17933](https://github.com/netdata/netdata/pull/17933) ([netdatabot](https://github.com/netdatabot))
- go.d whoisquery fix defaults in config\_schema [\#17932](https://github.com/netdata/netdata/pull/17932) ([ilyam8](https://github.com/ilyam8))
- Move to using CPack for repository configuration packages. [\#17930](https://github.com/netdata/netdata/pull/17930) ([Ferroin](https://github.com/Ferroin))
- go.d whoisquery: use Domain.ExpirationDateInTime if provided [\#17926](https://github.com/netdata/netdata/pull/17926) ([ilyam8](https://github.com/ilyam8))
- updater: handle json decode error in newer\_commit\_date\(\) [\#17925](https://github.com/netdata/netdata/pull/17925) ([ilyam8](https://github.com/ilyam8))
- Bump k8s.io/client-go from 0.30.1 to 0.30.2 in /src/go/collectors/go.d.plugin [\#17923](https://github.com/netdata/netdata/pull/17923) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d bump github.com/docker/docker v27.0.0+incompatible [\#17921](https://github.com/netdata/netdata/pull/17921) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/jessevdk/go-flags from 1.5.0 to 1.6.1 in /src/go/collectors/go.d.plugin [\#17919](https://github.com/netdata/netdata/pull/17919) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump go.mongodb.org/mongo-driver from 1.15.0 to 1.15.1 in /src/go/collectors/go.d.plugin [\#17917](https://github.com/netdata/netdata/pull/17917) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/miekg/dns from 1.1.59 to 1.1.61 in /src/go/collectors/go.d.plugin [\#17916](https://github.com/netdata/netdata/pull/17916) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d whoisquery: try requesting extended data if no expiration date [\#17913](https://github.com/netdata/netdata/pull/17913) ([ilyam8](https://github.com/ilyam8))
- go.d whoisquery: check if exp date is empty [\#17911](https://github.com/netdata/netdata/pull/17911) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17910](https://github.com/netdata/netdata/pull/17910) ([netdatabot](https://github.com/netdatabot))
- Update nvme/metadata: add how to use in a docker [\#17909](https://github.com/netdata/netdata/pull/17909) ([powerman](https://github.com/powerman))
- Update x509check/metadata: add missing smtp schema [\#17908](https://github.com/netdata/netdata/pull/17908) ([powerman](https://github.com/powerman))
- systemd: start `netdata` after network is online [\#17906](https://github.com/netdata/netdata/pull/17906) ([k0ste](https://github.com/k0ste))
- Fix Caddy setup in Install Netdata with Docker [\#17901](https://github.com/netdata/netdata/pull/17901) ([powerman](https://github.com/powerman))
- sys\_block\_zram: don't use "/dev" [\#17900](https://github.com/netdata/netdata/pull/17900) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17897](https://github.com/netdata/netdata/pull/17897) ([netdatabot](https://github.com/netdatabot))
- go.d ll netlisteners add support for wildcard address [\#17896](https://github.com/netdata/netdata/pull/17896) ([ilyam8](https://github.com/ilyam8))
- integrations make `<details>` open [\#17895](https://github.com/netdata/netdata/pull/17895) ([ilyam8](https://github.com/ilyam8))
- allow alerts to be created without too many requirements [\#17894](https://github.com/netdata/netdata/pull/17894) ([ktsaou](https://github.com/ktsaou))
- Improve ml thread termination during agent shutdown [\#17889](https://github.com/netdata/netdata/pull/17889) ([stelfrag](https://github.com/stelfrag))
- Update netdata-charts.md [\#17888](https://github.com/netdata/netdata/pull/17888) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17886](https://github.com/netdata/netdata/pull/17886) ([netdatabot](https://github.com/netdatabot))
- Restore ML thread termination to original order [\#17885](https://github.com/netdata/netdata/pull/17885) ([stelfrag](https://github.com/stelfrag))
- go.d intelgpu add an option to select specific GPU [\#17884](https://github.com/netdata/netdata/pull/17884) ([ilyam8](https://github.com/ilyam8))
- ndsudo update intel\_gpu\_top [\#17883](https://github.com/netdata/netdata/pull/17883) ([ilyam8](https://github.com/ilyam8))
- add netdata journald configuration [\#17882](https://github.com/netdata/netdata/pull/17882) ([ilyam8](https://github.com/ilyam8))
- fix detect\_libc in installer [\#17880](https://github.com/netdata/netdata/pull/17880) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.138.0 [\#17879](https://github.com/netdata/netdata/pull/17879) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17878](https://github.com/netdata/netdata/pull/17878) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#17877](https://github.com/netdata/netdata/pull/17877) ([netdatabot](https://github.com/netdatabot))
- Improve filecheck module metadata. [\#17874](https://github.com/netdata/netdata/pull/17874) ([Ferroin](https://github.com/Ferroin))
- update Telegram Cloud notification docs to include new topic ID field [\#17873](https://github.com/netdata/netdata/pull/17873) ([papazach](https://github.com/papazach))
- go.d phpfpm add config schema [\#17872](https://github.com/netdata/netdata/pull/17872) ([ilyam8](https://github.com/ilyam8))
- Fix updating release info when publishing nightly releases. [\#17871](https://github.com/netdata/netdata/pull/17871) ([Ferroin](https://github.com/Ferroin))
- go.d phpfpm: debug log the response on decoding error [\#17870](https://github.com/netdata/netdata/pull/17870) ([ilyam8](https://github.com/ilyam8))
- Improve agent shutdown [\#17868](https://github.com/netdata/netdata/pull/17868) ([stelfrag](https://github.com/stelfrag))
- Add openSUSE 15.6 to CI. [\#17865](https://github.com/netdata/netdata/pull/17865) ([Ferroin](https://github.com/Ferroin))
- Update CI infrastructure to publish to secondary packaging host. [\#17863](https://github.com/netdata/netdata/pull/17863) ([Ferroin](https://github.com/Ferroin))
- Improve anacron detection in updater. [\#17862](https://github.com/netdata/netdata/pull/17862) ([Ferroin](https://github.com/Ferroin))
- RBAC for dynamic configuration documentation [\#17861](https://github.com/netdata/netdata/pull/17861) ([Ancairon](https://github.com/Ancairon))
- DYNCFG: health, generate userconfig for incomplete alerts [\#17859](https://github.com/netdata/netdata/pull/17859) ([ktsaou](https://github.com/ktsaou))
- Create retention charts for higher tiers [\#17855](https://github.com/netdata/netdata/pull/17855) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/vmware/govmomi from 0.37.2 to 0.37.3 in /src/go/collectors/go.d.plugin [\#17854](https://github.com/netdata/netdata/pull/17854) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/net from 0.25.0 to 0.26.0 in /src/go/collectors/go.d.plugin [\#17852](https://github.com/netdata/netdata/pull/17852) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 26.1.3+incompatible to 26.1.4+incompatible in /src/go/collectors/go.d.plugin [\#17851](https://github.com/netdata/netdata/pull/17851) ([dependabot[bot]](https://github.com/apps/dependabot))
- Delay retention check until agent has initialized [\#17850](https://github.com/netdata/netdata/pull/17850) ([stelfrag](https://github.com/stelfrag))
- Fix tier statistics  [\#17849](https://github.com/netdata/netdata/pull/17849) ([stelfrag](https://github.com/stelfrag))
- fix: check memory mode before creating dbengine retention chart [\#17848](https://github.com/netdata/netdata/pull/17848) ([ilyam8](https://github.com/ilyam8))
- update dbengine retention chart family and priority [\#17847](https://github.com/netdata/netdata/pull/17847) ([ilyam8](https://github.com/ilyam8))
- Remove unused variable [\#17846](https://github.com/netdata/netdata/pull/17846) ([stelfrag](https://github.com/stelfrag))
- Properly initialize spinlock in ARAL. [\#17844](https://github.com/netdata/netdata/pull/17844) ([vkalintiris](https://github.com/vkalintiris))
- Fix compilation without dbengine [\#17843](https://github.com/netdata/netdata/pull/17843) ([stelfrag](https://github.com/stelfrag))
- explicitly disable removed collectors in python.d.conf [\#17840](https://github.com/netdata/netdata/pull/17840) ([ilyam8](https://github.com/ilyam8))
- fix tc plugin undeclared vars [\#17839](https://github.com/netdata/netdata/pull/17839) ([ilyam8](https://github.com/ilyam8))
- hide sqlite config \(netdata.conf\) [\#17838](https://github.com/netdata/netdata/pull/17838) ([ilyam8](https://github.com/ilyam8))
- proc net dev: simplify config [\#17837](https://github.com/netdata/netdata/pull/17837) ([ilyam8](https://github.com/ilyam8))
- aclk: move "proxy" from "netdata.conf" to "cloud.conf" [\#17836](https://github.com/netdata/netdata/pull/17836) ([ilyam8](https://github.com/ilyam8))
- tc plugin simplify config [\#17835](https://github.com/netdata/netdata/pull/17835) ([ilyam8](https://github.com/ilyam8))
- health dyncfg userconfig: remove first newline [\#17834](https://github.com/netdata/netdata/pull/17834) ([ilyam8](https://github.com/ilyam8))
- Dyncfg doc [\#17832](https://github.com/netdata/netdata/pull/17832) ([Ancairon](https://github.com/Ancairon))
- docs: claiming: rename connect button [\#17831](https://github.com/netdata/netdata/pull/17831) ([ilyam8](https://github.com/ilyam8))
- Sockets VFS \(context update\) [\#17830](https://github.com/netdata/netdata/pull/17830) ([thiagoftsm](https://github.com/thiagoftsm))
- fix order of loading schema files in dyncfg\_get\_schema\_from [\#17829](https://github.com/netdata/netdata/pull/17829) ([ilyam8](https://github.com/ilyam8))
- claiming: add proxy to cloud.conf if set [\#17828](https://github.com/netdata/netdata/pull/17828) ([ilyam8](https://github.com/ilyam8))
- Use bundled protobuf for openSUSE packages. [\#17827](https://github.com/netdata/netdata/pull/17827) ([Ferroin](https://github.com/Ferroin))
- Disable updater jitter when run from anacron. [\#17826](https://github.com/netdata/netdata/pull/17826) ([Ferroin](https://github.com/Ferroin))
- Make our LSB init script \_actually\_ LSB compliant. [\#17824](https://github.com/netdata/netdata/pull/17824) ([Ferroin](https://github.com/Ferroin))
- fix health alert load15 info [\#17823](https://github.com/netdata/netdata/pull/17823) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17822](https://github.com/netdata/netdata/pull/17822) ([netdatabot](https://github.com/netdatabot))
- Proper check for static\_thread being NULL [\#17821](https://github.com/netdata/netdata/pull/17821) ([stelfrag](https://github.com/stelfrag))
- Fix coverity report [\#17820](https://github.com/netdata/netdata/pull/17820) ([thiagoftsm](https://github.com/thiagoftsm))
- Update contexts - eBPF.plugin \(part II\) [\#17819](https://github.com/netdata/netdata/pull/17819) ([thiagoftsm](https://github.com/thiagoftsm))
- Add alert meta info \(node index\) [\#17818](https://github.com/netdata/netdata/pull/17818) ([stelfrag](https://github.com/stelfrag))
- remove "ignore 0 metrics" leftovers [\#17817](https://github.com/netdata/netdata/pull/17817) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17815](https://github.com/netdata/netdata/pull/17815) ([netdatabot](https://github.com/netdatabot))
- fix typo: `round tripe` → `round trip` [\#17814](https://github.com/netdata/netdata/pull/17814) ([luckman212](https://github.com/luckman212))
- Change classification to "cls" since "cl" is clear count [\#17811](https://github.com/netdata/netdata/pull/17811) ([stelfrag](https://github.com/stelfrag))
- remove "ignore 0 metrics" from tc/btrfs/ksm [\#17810](https://github.com/netdata/netdata/pull/17810) ([ilyam8](https://github.com/ilyam8))
- fix ebpf units [\#17809](https://github.com/netdata/netdata/pull/17809) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17808](https://github.com/netdata/netdata/pull/17808) ([netdatabot](https://github.com/netdatabot))
- health: add go.d/lvm alerts [\#17807](https://github.com/netdata/netdata/pull/17807) ([ilyam8](https://github.com/ilyam8))
- Update libbpf [\#17806](https://github.com/netdata/netdata/pull/17806) ([thiagoftsm](https://github.com/thiagoftsm))
- remove "ingore zero metrics" from freebsd plugin [\#17805](https://github.com/netdata/netdata/pull/17805) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/prometheus/common from 0.53.0 to 0.54.0 in /src/go/collectors/go.d.plugin [\#17804](https://github.com/netdata/netdata/pull/17804) ([dependabot[bot]](https://github.com/apps/dependabot))
- remove "ingore 0 metrics" from macos plugin [\#17803](https://github.com/netdata/netdata/pull/17803) ([ilyam8](https://github.com/ilyam8))
- fix cgroups pressure [\#17800](https://github.com/netdata/netdata/pull/17800) ([ilyam8](https://github.com/ilyam8))
- fix buffer overflow incgroups\_detect\_systemd\(\) [\#17799](https://github.com/netdata/netdata/pull/17799) ([ilyam8](https://github.com/ilyam8))
- eBPF contexts \(part I\) [\#17797](https://github.com/netdata/netdata/pull/17797) ([thiagoftsm](https://github.com/thiagoftsm))
- cgroup plugin: simplify and remove "ignore zero metrics" [\#17795](https://github.com/netdata/netdata/pull/17795) ([ilyam8](https://github.com/ilyam8))
- Correctly handle eBPF check in package test script. [\#17794](https://github.com/netdata/netdata/pull/17794) ([Ferroin](https://github.com/Ferroin))
- Use correct path for package files. [\#17793](https://github.com/netdata/netdata/pull/17793) ([vkalintiris](https://github.com/vkalintiris))
- proc/net\_dev: remove "ignore zero metrics" [\#17789](https://github.com/netdata/netdata/pull/17789) ([ilyam8](https://github.com/ilyam8))
- Add Alpine 3.20 to CI checks. [\#17788](https://github.com/netdata/netdata/pull/17788) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17786](https://github.com/netdata/netdata/pull/17786) ([netdatabot](https://github.com/netdatabot))
- docs fix statsd conf dir [\#17785](https://github.com/netdata/netdata/pull/17785) ([ilyam8](https://github.com/ilyam8))
- go.d phpfpm switch to github.com/kanocz/fcgi\_client [\#17784](https://github.com/netdata/netdata/pull/17784) ([ilyam8](https://github.com/ilyam8))
- Change "War Room" to "Room" and other docs changes [\#17783](https://github.com/netdata/netdata/pull/17783) ([Ancairon](https://github.com/Ancairon))
- rm "ignore zero metrics" proc meminfo [\#17781](https://github.com/netdata/netdata/pull/17781) ([ilyam8](https://github.com/ilyam8))
- fix links [\#17779](https://github.com/netdata/netdata/pull/17779) ([Ancairon](https://github.com/Ancairon))
- remove "ignore zero metrics" from proc network modules [\#17776](https://github.com/netdata/netdata/pull/17776) ([ilyam8](https://github.com/ilyam8))
- proc/diskstats and diskspace: remove "ignore zero metrics" [\#17775](https://github.com/netdata/netdata/pull/17775) ([ilyam8](https://github.com/ilyam8))
- docs fix "Prevent the double access.log" [\#17773](https://github.com/netdata/netdata/pull/17773) ([ilyam8](https://github.com/ilyam8))
- docs: simplify claiming readme part1 [\#17771](https://github.com/netdata/netdata/pull/17771) ([ilyam8](https://github.com/ilyam8))
- Upgrade sqlite version to 3.45.3 [\#17769](https://github.com/netdata/netdata/pull/17769) ([stelfrag](https://github.com/stelfrag))
- Netdata Cloud docs section edits [\#17768](https://github.com/netdata/netdata/pull/17768) ([Ancairon](https://github.com/Ancairon))
- Fix DEB package builds. [\#17765](https://github.com/netdata/netdata/pull/17765) ([Ferroin](https://github.com/Ferroin))
- Fix version for go.d plugin [\#17764](https://github.com/netdata/netdata/pull/17764) ([vkalintiris](https://github.com/vkalintiris))
- go.d sd local-listeners fix extractComm [\#17763](https://github.com/netdata/netdata/pull/17763) ([ilyam8](https://github.com/ilyam8))
- Schedule a node info on label reload [\#17762](https://github.com/netdata/netdata/pull/17762) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#17761](https://github.com/netdata/netdata/pull/17761) ([netdatabot](https://github.com/netdatabot))
- add clickhouse alerts [\#17760](https://github.com/netdata/netdata/pull/17760) ([ilyam8](https://github.com/ilyam8))
- simplify installation page [\#17759](https://github.com/netdata/netdata/pull/17759) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17758](https://github.com/netdata/netdata/pull/17758) ([netdatabot](https://github.com/netdatabot))
- Collecting metrics docs section simplification [\#17757](https://github.com/netdata/netdata/pull/17757) ([Ancairon](https://github.com/Ancairon))
- go.d clickhouse add more metrics [\#17756](https://github.com/netdata/netdata/pull/17756) ([ilyam8](https://github.com/ilyam8))
- mention how to remove highlight in documentation [\#17755](https://github.com/netdata/netdata/pull/17755) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17752](https://github.com/netdata/netdata/pull/17752) ([netdatabot](https://github.com/netdatabot))
- go.d clickhouse add running queries [\#17751](https://github.com/netdata/netdata/pull/17751) ([ilyam8](https://github.com/ilyam8))
- remove unused go.d/prometheus meta file [\#17749](https://github.com/netdata/netdata/pull/17749) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17748](https://github.com/netdata/netdata/pull/17748) ([netdatabot](https://github.com/netdatabot))
- Use semver releases with sentry. [\#17746](https://github.com/netdata/netdata/pull/17746) ([vkalintiris](https://github.com/vkalintiris))
- add go.d clickhouse [\#17743](https://github.com/netdata/netdata/pull/17743) ([ilyam8](https://github.com/ilyam8))
- fix clickhouse in apps groups [\#17742](https://github.com/netdata/netdata/pull/17742) ([ilyam8](https://github.com/ilyam8))
- fix ebpf cgroup swap context [\#17740](https://github.com/netdata/netdata/pull/17740) ([ilyam8](https://github.com/ilyam8))
- Update netdata-agent-security.md [\#17738](https://github.com/netdata/netdata/pull/17738) ([Ancairon](https://github.com/Ancairon))
- Collecting metrics docs grammar pass [\#17736](https://github.com/netdata/netdata/pull/17736) ([Ancairon](https://github.com/Ancairon))
- Grammar pass on docs [\#17735](https://github.com/netdata/netdata/pull/17735) ([Ancairon](https://github.com/Ancairon))
- eBPF OOMKills adjust and fixes. [\#17734](https://github.com/netdata/netdata/pull/17734) ([thiagoftsm](https://github.com/thiagoftsm))
- Ensure that the choice of compiler and target is passed to sub-projects. [\#17732](https://github.com/netdata/netdata/pull/17732) ([Ferroin](https://github.com/Ferroin))
- Include the Host in the HTTP header \(mqtt\) [\#17731](https://github.com/netdata/netdata/pull/17731) ([stelfrag](https://github.com/stelfrag))
- Add alert meta info [\#17730](https://github.com/netdata/netdata/pull/17730) ([stelfrag](https://github.com/stelfrag))
- grammar pass on alerts and notifications dir [\#17729](https://github.com/netdata/netdata/pull/17729) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17726](https://github.com/netdata/netdata/pull/17726) ([netdatabot](https://github.com/netdatabot))
- go.d systemdunits add "skip\_transient" [\#17725](https://github.com/netdata/netdata/pull/17725) ([ilyam8](https://github.com/ilyam8))
- minor fix on link [\#17722](https://github.com/netdata/netdata/pull/17722) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17721](https://github.com/netdata/netdata/pull/17721) ([netdatabot](https://github.com/netdatabot))
- PR to change absolute links to relative [\#17720](https://github.com/netdata/netdata/pull/17720) ([Ancairon](https://github.com/Ancairon))
- Change links to relative links in one doc [\#17719](https://github.com/netdata/netdata/pull/17719) ([Ancairon](https://github.com/Ancairon))
- fix proc plugin disk\_avgsz [\#17718](https://github.com/netdata/netdata/pull/17718) ([ilyam8](https://github.com/ilyam8))
- go.d weblog ignore reqProcTime on HTTP 101 [\#17717](https://github.com/netdata/netdata/pull/17717) ([ilyam8](https://github.com/ilyam8))
- Fix mongodb default config indentation [\#17715](https://github.com/netdata/netdata/pull/17715) ([louis-lau](https://github.com/louis-lau))
- Fix compilation with disable-cloud [\#17714](https://github.com/netdata/netdata/pull/17714) ([stelfrag](https://github.com/stelfrag))
- fix on link [\#17712](https://github.com/netdata/netdata/pull/17712) ([Ancairon](https://github.com/Ancairon))
- gha labeler add collectors/windows [\#17711](https://github.com/netdata/netdata/pull/17711) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/likexian/whois-parser from 1.24.15 to 1.24.16 in /src/go/collectors/go.d.plugin [\#17710](https://github.com/netdata/netdata/pull/17710) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump k8s.io/client-go from 0.30.0 to 0.30.1 in /src/go/collectors/go.d.plugin [\#17708](https://github.com/netdata/netdata/pull/17708) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 26.1.2+incompatible to 26.1.3+incompatible in /src/go/collectors/go.d.plugin [\#17706](https://github.com/netdata/netdata/pull/17706) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix multipler on Windows \("Memory"\) [\#17705](https://github.com/netdata/netdata/pull/17705) ([thiagoftsm](https://github.com/thiagoftsm))
- Win processes \("System" name\) [\#17704](https://github.com/netdata/netdata/pull/17704) ([thiagoftsm](https://github.com/thiagoftsm))
- some markdown fixes [\#17703](https://github.com/netdata/netdata/pull/17703) ([ilyam8](https://github.com/ilyam8))
- go.d fix some JB code inspection issues [\#17702](https://github.com/netdata/netdata/pull/17702) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17701](https://github.com/netdata/netdata/pull/17701) ([netdatabot](https://github.com/netdatabot))
- Corrected grammar and mispelling [\#17699](https://github.com/netdata/netdata/pull/17699) ([zallaevan](https://github.com/zallaevan))
- go.d dyncfg rm space yaml contentType [\#17698](https://github.com/netdata/netdata/pull/17698) ([ilyam8](https://github.com/ilyam8))
- Revert "Support to WolfSSL \(Step 1\)" [\#17697](https://github.com/netdata/netdata/pull/17697) ([stelfrag](https://github.com/stelfrag))
- fix sender parsing when receiving remote input [\#17696](https://github.com/netdata/netdata/pull/17696) ([ktsaou](https://github.com/ktsaou))
- dyncfg files on disk do not contain colons [\#17694](https://github.com/netdata/netdata/pull/17694) ([ktsaou](https://github.com/ktsaou))
- Simplify and unify the way we are handling versions. [\#17693](https://github.com/netdata/netdata/pull/17693) ([vkalintiris](https://github.com/vkalintiris))
- DYNCFG: add userconfig action [\#17692](https://github.com/netdata/netdata/pull/17692) ([ktsaou](https://github.com/ktsaou))
- Add agent CLI command to remove a stale node [\#17691](https://github.com/netdata/netdata/pull/17691) ([stelfrag](https://github.com/stelfrag))
- Check for empty dimension id from a plugin [\#17690](https://github.com/netdata/netdata/pull/17690) ([stelfrag](https://github.com/stelfrag))
- Fix timex slow shutdown [\#17688](https://github.com/netdata/netdata/pull/17688) ([stelfrag](https://github.com/stelfrag))
- Rename a variable in FreeIMPI \(WolfSSL support\) [\#17687](https://github.com/netdata/netdata/pull/17687) ([thiagoftsm](https://github.com/thiagoftsm))
- go.d sd fix rspamd classify match [\#17686](https://github.com/netdata/netdata/pull/17686) ([ilyam8](https://github.com/ilyam8))
- Properly detect attribute format types [\#17685](https://github.com/netdata/netdata/pull/17685) ([vkalintiris](https://github.com/vkalintiris))
- go.d dyncfg add userconfig action [\#17684](https://github.com/netdata/netdata/pull/17684) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17683](https://github.com/netdata/netdata/pull/17683) ([netdatabot](https://github.com/netdatabot))
- Re-enable ML for RHEL 7 and AL 2 RPM packages. [\#17682](https://github.com/netdata/netdata/pull/17682) ([Ferroin](https://github.com/Ferroin))
- Clean up DEB package dependencies. [\#17680](https://github.com/netdata/netdata/pull/17680) ([Ferroin](https://github.com/Ferroin))
- add go.d/rspamd [\#17679](https://github.com/netdata/netdata/pull/17679) ([ilyam8](https://github.com/ilyam8))
- Restructure packaging related files to better reflect usage. [\#17678](https://github.com/netdata/netdata/pull/17678) ([Ferroin](https://github.com/Ferroin))
- Do not specify linker in compilation flags. [\#17677](https://github.com/netdata/netdata/pull/17677) ([vkalintiris](https://github.com/vkalintiris))
- Regenerate integrations.js [\#17676](https://github.com/netdata/netdata/pull/17676) ([netdatabot](https://github.com/netdatabot))
- fix broken links and links pointing to Learn [\#17675](https://github.com/netdata/netdata/pull/17675) ([Ancairon](https://github.com/Ancairon))
- add rspamd to apps\_groups.conf [\#17674](https://github.com/netdata/netdata/pull/17674) ([ilyam8](https://github.com/ilyam8))
- Fix compilation without h2o, cloud enabled [\#17673](https://github.com/netdata/netdata/pull/17673) ([stelfrag](https://github.com/stelfrag))
- Include the Host in the HTTP header [\#17670](https://github.com/netdata/netdata/pull/17670) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#17668](https://github.com/netdata/netdata/pull/17668) ([netdatabot](https://github.com/netdatabot))
- Fix CentOS 7 builds for ML. [\#17667](https://github.com/netdata/netdata/pull/17667) ([vkalintiris](https://github.com/vkalintiris))
- go.d litespeed [\#17665](https://github.com/netdata/netdata/pull/17665) ([ilyam8](https://github.com/ilyam8))
- Correctly handle required compilation flags for dependencies. [\#17664](https://github.com/netdata/netdata/pull/17664) ([Ferroin](https://github.com/Ferroin))
- python.d remove litespeed [\#17663](https://github.com/netdata/netdata/pull/17663) ([ilyam8](https://github.com/ilyam8))
- files movearound [\#17662](https://github.com/netdata/netdata/pull/17662) ([Ancairon](https://github.com/Ancairon))
- go.d update config dirs [\#17661](https://github.com/netdata/netdata/pull/17661) ([ilyam8](https://github.com/ilyam8))

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
