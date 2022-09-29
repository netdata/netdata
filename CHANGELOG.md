# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.36.1...HEAD)

**Merged pull requests:**

- Dont send NodeInfo during first database cleanup [\#13740](https://github.com/netdata/netdata/pull/13740) ([MrZammler](https://github.com/MrZammler))
- CMake - add possibility to build without ACLK [\#13736](https://github.com/netdata/netdata/pull/13736) ([underhood](https://github.com/underhood))
- Change cast to remove coverity warnings [\#13735](https://github.com/netdata/netdata/pull/13735) ([thiagoftsm](https://github.com/thiagoftsm))
- Do not try to start an archived host in dbengine if dbengine is not compiled [\#13724](https://github.com/netdata/netdata/pull/13724) ([stelfrag](https://github.com/stelfrag))
- feat\(health\): add new Redis alarms [\#13715](https://github.com/netdata/netdata/pull/13715) ([ilyam8](https://github.com/ilyam8))
- Faster streaming by 25% on the child [\#13708](https://github.com/netdata/netdata/pull/13708) ([ktsaou](https://github.com/ktsaou))
- Do not create train/predict dimensions meant for tracking anomaly rates. [\#13707](https://github.com/netdata/netdata/pull/13707) ([vkalintiris](https://github.com/vkalintiris))
- Update exporting unit tests [\#13706](https://github.com/netdata/netdata/pull/13706) ([vlvkobal](https://github.com/vlvkobal))
- bump go.d.plugin to v0.40.1 [\#13704](https://github.com/netdata/netdata/pull/13704) ([ilyam8](https://github.com/ilyam8))
- Build judy even without dbengine [\#13703](https://github.com/netdata/netdata/pull/13703) ([underhood](https://github.com/underhood))
- alarms collector: ability to exclude certain alarms via config [\#13701](https://github.com/netdata/netdata/pull/13701) ([andrewm4894](https://github.com/andrewm4894))
- Fix inconsistent alert class names [\#13699](https://github.com/netdata/netdata/pull/13699) ([ralphm](https://github.com/ralphm))
- disable Postgres last vacuum/analyze alarms [\#13698](https://github.com/netdata/netdata/pull/13698) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.29.1. [\#13696](https://github.com/netdata/netdata/pull/13696) ([netdatabot](https://github.com/netdatabot))
- docs: nvidia-smi in a container limitation note [\#13695](https://github.com/netdata/netdata/pull/13695) ([ilyam8](https://github.com/ilyam8))
- Use CMake generated config.h also in out of tree CMake build [\#13692](https://github.com/netdata/netdata/pull/13692) ([underhood](https://github.com/underhood))
- Store nulls instead of empty strings in health tables [\#13683](https://github.com/netdata/netdata/pull/13683) ([MrZammler](https://github.com/MrZammler))
- Fix warnings during compilation time on ARM \(32 bits\) [\#13681](https://github.com/netdata/netdata/pull/13681) ([thiagoftsm](https://github.com/thiagoftsm))
- dictionary updated documentation and cosmetics [\#13679](https://github.com/netdata/netdata/pull/13679) ([ktsaou](https://github.com/ktsaou))
- disable internal log [\#13678](https://github.com/netdata/netdata/pull/13678) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin v0.40.0 [\#13675](https://github.com/netdata/netdata/pull/13675) ([ilyam8](https://github.com/ilyam8))
- remove \_instance\_family label [\#13674](https://github.com/netdata/netdata/pull/13674) ([ilyam8](https://github.com/ilyam8))
- fix typo not deleting collected flag; force removing collected flag on child disconnect [\#13672](https://github.com/netdata/netdata/pull/13672) ([ktsaou](https://github.com/ktsaou))
- feat\(health\): add Postgres alarms [\#13671](https://github.com/netdata/netdata/pull/13671) ([ilyam8](https://github.com/ilyam8))
- add proxysql dashboard info [\#13669](https://github.com/netdata/netdata/pull/13669) ([ilyam8](https://github.com/ilyam8))
- Additional sqlite statistics [\#13668](https://github.com/netdata/netdata/pull/13668) ([stelfrag](https://github.com/stelfrag))
- Advance the buffer properly to scan the journal file [\#13666](https://github.com/netdata/netdata/pull/13666) ([stelfrag](https://github.com/stelfrag))
- Add sqlite page cache hit and miss statistics [\#13665](https://github.com/netdata/netdata/pull/13665) ([stelfrag](https://github.com/stelfrag))
- bump go.d.plugin to v0.39.0 [\#13662](https://github.com/netdata/netdata/pull/13662) ([ilyam8](https://github.com/ilyam8))
- update Postgres dashboard info [\#13661](https://github.com/netdata/netdata/pull/13661) ([ilyam8](https://github.com/ilyam8))
- Use mmap if possible during startup for journal replay [\#13660](https://github.com/netdata/netdata/pull/13660) ([stelfrag](https://github.com/stelfrag))
- Update dashboard to version v2.29.0. [\#13654](https://github.com/netdata/netdata/pull/13654) ([netdatabot](https://github.com/netdatabot))
- Fix container virtualization info [\#13653](https://github.com/netdata/netdata/pull/13653) ([vlvkobal](https://github.com/vlvkobal))
- Do not free AR dimensions from within ML. [\#13651](https://github.com/netdata/netdata/pull/13651) ([vkalintiris](https://github.com/vkalintiris))
- Remove Chart/Dim based communication [\#13650](https://github.com/netdata/netdata/pull/13650) ([underhood](https://github.com/underhood))
- Improve agent shutdown time [\#13649](https://github.com/netdata/netdata/pull/13649) ([stelfrag](https://github.com/stelfrag))
- add \_collect\_job label to python.d/\* charts [\#13648](https://github.com/netdata/netdata/pull/13648) ([ilyam8](https://github.com/ilyam8))
- RRD structures managed by dictionaries [\#13646](https://github.com/netdata/netdata/pull/13646) ([ktsaou](https://github.com/ktsaou))
- fix rrdcontexts left in the post-processing queue from the garbage collector [\#13645](https://github.com/netdata/netdata/pull/13645) ([ktsaou](https://github.com/ktsaou))
- apps.plugin: Re-add `chrome` to the `webbrowser` group. [\#13642](https://github.com/netdata/netdata/pull/13642) ([Ferroin](https://github.com/Ferroin))
- Fix a memory leak on archived host creation [\#13641](https://github.com/netdata/netdata/pull/13641) ([stelfrag](https://github.com/stelfrag))
- fix compile issues [\#13640](https://github.com/netdata/netdata/pull/13640) ([ktsaou](https://github.com/ktsaou))
- Obsolete RRDSET state [\#13635](https://github.com/netdata/netdata/pull/13635) ([ktsaou](https://github.com/ktsaou))
- Updated tc.plugin \(linux bandwidth QoS\) [\#13634](https://github.com/netdata/netdata/pull/13634) ([ktsaou](https://github.com/ktsaou))
- Fix worker utilization cleanup [\#13633](https://github.com/netdata/netdata/pull/13633) ([stelfrag](https://github.com/stelfrag))
- remove forgotten avl structure from rrdcalc [\#13632](https://github.com/netdata/netdata/pull/13632) ([ktsaou](https://github.com/ktsaou))
- Deaggregate the `gui` and `email` app groupx and improve GUI coverage. [\#13631](https://github.com/netdata/netdata/pull/13631) ([Ferroin](https://github.com/Ferroin))
- Faster rrdcontext [\#13629](https://github.com/netdata/netdata/pull/13629) ([ktsaou](https://github.com/ktsaou))
- Update uninstaller documentation. [\#13627](https://github.com/netdata/netdata/pull/13627) ([Ferroin](https://github.com/Ferroin))
- eBPF different improvements [\#13624](https://github.com/netdata/netdata/pull/13624) ([thiagoftsm](https://github.com/thiagoftsm))
- adjust systemdunits alarms [\#13623](https://github.com/netdata/netdata/pull/13623) ([ilyam8](https://github.com/ilyam8))
- fix apps plugin users charts descriptipon [\#13621](https://github.com/netdata/netdata/pull/13621) ([ilyam8](https://github.com/ilyam8))
- add Postgres total connection utilization alarm [\#13620](https://github.com/netdata/netdata/pull/13620) ([ilyam8](https://github.com/ilyam8))
- update Postgres "connections" dashboard info [\#13619](https://github.com/netdata/netdata/pull/13619) ([ilyam8](https://github.com/ilyam8))
- Assorted updates for apps\_groups.conf. [\#13618](https://github.com/netdata/netdata/pull/13618) ([Ferroin](https://github.com/Ferroin))
- add spiceproxy to proxmox group [\#13615](https://github.com/netdata/netdata/pull/13615) ([ilyam8](https://github.com/ilyam8))
- Improve coverage of Linux kernel threads in apps\_groups.conf [\#13612](https://github.com/netdata/netdata/pull/13612) ([Ferroin](https://github.com/Ferroin))
- Clean chart hash map  [\#13611](https://github.com/netdata/netdata/pull/13611) ([stelfrag](https://github.com/stelfrag))
- Update dashboard to version v2.28.8. [\#13609](https://github.com/netdata/netdata/pull/13609) ([netdatabot](https://github.com/netdatabot))
- Don't try to load db rows when chart\_id or dim\_id is null [\#13608](https://github.com/netdata/netdata/pull/13608) ([MrZammler](https://github.com/MrZammler))
- Add info text for wal and update for checkpoints [\#13607](https://github.com/netdata/netdata/pull/13607) ([shyamvalsan](https://github.com/shyamvalsan))
- bump go.d.plugin to v0.38.0 [\#13603](https://github.com/netdata/netdata/pull/13603) ([ilyam8](https://github.com/ilyam8))
- Use prepared statements for context related queries [\#13602](https://github.com/netdata/netdata/pull/13602) ([stelfrag](https://github.com/stelfrag))
- fix\(cgroups.plugin\): fix chart id length check [\#13601](https://github.com/netdata/netdata/pull/13601) ([ilyam8](https://github.com/ilyam8))
- Temporary fix for command injection vulnerability in GHA workflow. [\#13600](https://github.com/netdata/netdata/pull/13600) ([Ferroin](https://github.com/Ferroin))
- update logind dashboard info [\#13597](https://github.com/netdata/netdata/pull/13597) ([ilyam8](https://github.com/ilyam8))
- Add link to the performance optimization guide [\#13595](https://github.com/netdata/netdata/pull/13595) ([cakrit](https://github.com/cakrit))
- sqlite3 global statistics [\#13594](https://github.com/netdata/netdata/pull/13594) ([ktsaou](https://github.com/ktsaou))
- feat\(python.d/nvidia\_smi\): collect power state [\#13580](https://github.com/netdata/netdata/pull/13580) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d/nvidia\_smi\): repsect update\_every for polling [\#13579](https://github.com/netdata/netdata/pull/13579) ([ilyam8](https://github.com/ilyam8))
- prevent crash on rrdcontext apis when rrdcontexts is not initialized [\#13578](https://github.com/netdata/netdata/pull/13578) ([ktsaou](https://github.com/ktsaou))
- CMake improvements part 1 [\#13575](https://github.com/netdata/netdata/pull/13575) ([underhood](https://github.com/underhood))
- bump go.d.plugin to v0.37.2 [\#13574](https://github.com/netdata/netdata/pull/13574) ([ilyam8](https://github.com/ilyam8))
- Updating info for postgreqsql metrics [\#13573](https://github.com/netdata/netdata/pull/13573) ([shyamvalsan](https://github.com/shyamvalsan))
- add `apt` to `apps_groups.conf` [\#13571](https://github.com/netdata/netdata/pull/13571) ([andrewm4894](https://github.com/andrewm4894))
- Deduplicate all netdata strings [\#13570](https://github.com/netdata/netdata/pull/13570) ([ktsaou](https://github.com/ktsaou))
- eBPF cmake missing include dir [\#13568](https://github.com/netdata/netdata/pull/13568) ([underhood](https://github.com/underhood))
- chore: removing logging that a chart collection in the same interpolation point [\#13567](https://github.com/netdata/netdata/pull/13567) ([ilyam8](https://github.com/ilyam8))
- add more monitoring tools to `apps_groups.conf` [\#13566](https://github.com/netdata/netdata/pull/13566) ([andrewm4894](https://github.com/andrewm4894))
- fix\(health.d/mysql\): adjust `mysql_galera_cluster_size_max_2m` lookup to make time in warn/crit predictable [\#13563](https://github.com/netdata/netdata/pull/13563) ([ilyam8](https://github.com/ilyam8))
- Update dashboard to version v2.28.6. [\#13562](https://github.com/netdata/netdata/pull/13562) ([netdatabot](https://github.com/netdatabot))
- Prefer context attributes from non archived charts [\#13559](https://github.com/netdata/netdata/pull/13559) ([MrZammler](https://github.com/MrZammler))
- bump go.d.plugin to v0.37.1 [\#13555](https://github.com/netdata/netdata/pull/13555) ([ilyam8](https://github.com/ilyam8))
- Fix coverity 380387 [\#13551](https://github.com/netdata/netdata/pull/13551) ([MrZammler](https://github.com/MrZammler))
- add docker dashboard info [\#13547](https://github.com/netdata/netdata/pull/13547) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.37.0 [\#13546](https://github.com/netdata/netdata/pull/13546) ([ilyam8](https://github.com/ilyam8))
- feat\(python.d/sensors\): discover chips, features at runtime [\#13545](https://github.com/netdata/netdata/pull/13545) ([ilyam8](https://github.com/ilyam8))
- Remove aclk\_api.\[ch\] [\#13540](https://github.com/netdata/netdata/pull/13540) ([underhood](https://github.com/underhood))
- Cleanup of APIs [\#13539](https://github.com/netdata/netdata/pull/13539) ([underhood](https://github.com/underhood))
- bump go.d version to v0.36.0 [\#13538](https://github.com/netdata/netdata/pull/13538) ([ilyam8](https://github.com/ilyam8))
- chore\(python.d\): rename dockerd job on lock registration [\#13537](https://github.com/netdata/netdata/pull/13537) ([ilyam8](https://github.com/ilyam8))
- Update MacOS community support details [\#13536](https://github.com/netdata/netdata/pull/13536) ([DShreve2](https://github.com/DShreve2))
- Fix a crash when xen libraries are misconfigured [\#13535](https://github.com/netdata/netdata/pull/13535) ([vlvkobal](https://github.com/vlvkobal))
- Add summary dashboard for PostgreSQL [\#13534](https://github.com/netdata/netdata/pull/13534) ([shyamvalsan](https://github.com/shyamvalsan))
- add `jupyter` to `apps_groups.conf` [\#13533](https://github.com/netdata/netdata/pull/13533) ([andrewm4894](https://github.com/andrewm4894))
- Schedule next rotation based on absolute time [\#13531](https://github.com/netdata/netdata/pull/13531) ([MrZammler](https://github.com/MrZammler))
- Improve PID monitoring \(step 2\) [\#13530](https://github.com/netdata/netdata/pull/13530) ([thiagoftsm](https://github.com/thiagoftsm))
- fix\(health\): set default curl connection timeout if not set [\#13529](https://github.com/netdata/netdata/pull/13529) ([ilyam8](https://github.com/ilyam8))
- Update FreeIPMI and CUPS plugin documentation. [\#13526](https://github.com/netdata/netdata/pull/13526) ([Ferroin](https://github.com/Ferroin))
- Use LVM UUIDs in chart ids for logical volumes [\#13525](https://github.com/netdata/netdata/pull/13525) ([vlvkobal](https://github.com/vlvkobal))
- fix\(cgroups.plugin\): use Docker API for name resolution when Docker is a snap package [\#13523](https://github.com/netdata/netdata/pull/13523) ([ilyam8](https://github.com/ilyam8))
- remove reference to charts now in netdata monitoring [\#13521](https://github.com/netdata/netdata/pull/13521) ([andrewm4894](https://github.com/andrewm4894))
- fix\(ci\): fix fetching tags in Build workflow [\#13517](https://github.com/netdata/netdata/pull/13517) ([ilyam8](https://github.com/ilyam8))
- docs\(postfix\): add a note about `authorized_mailq_users` [\#13515](https://github.com/netdata/netdata/pull/13515) ([ilyam8](https://github.com/ilyam8))
- Remove extra U from log message [\#13514](https://github.com/netdata/netdata/pull/13514) ([uplime](https://github.com/uplime))
- Print rrdcontexts versions with PRIu64 [\#13511](https://github.com/netdata/netdata/pull/13511) ([MrZammler](https://github.com/MrZammler))
- Calculate name hash after rrdvar\_fix\_name [\#13509](https://github.com/netdata/netdata/pull/13509) ([MrZammler](https://github.com/MrZammler))
- fix\(packaging\): add CAP\_NET\_ADMIN for go.d.plugin [\#13507](https://github.com/netdata/netdata/pull/13507) ([ilyam8](https://github.com/ilyam8))
- netdata.service: Update PIDFile to avoid systemd legacy path warning [\#13504](https://github.com/netdata/netdata/pull/13504) ([candrews](https://github.com/candrews))
- chore\(python.d\): remove python.d/\* announced in v1.36.0 deprecation notice [\#13503](https://github.com/netdata/netdata/pull/13503) ([ilyam8](https://github.com/ilyam8))
- reduce memcpy and memory usage on mqtt5 [\#13450](https://github.com/netdata/netdata/pull/13450) ([underhood](https://github.com/underhood))
- Modify PID monitoring \(ebpf.plugin\) [\#13397](https://github.com/netdata/netdata/pull/13397) ([thiagoftsm](https://github.com/thiagoftsm))
- Support chart labels in alerts [\#13290](https://github.com/netdata/netdata/pull/13290) ([MrZammler](https://github.com/MrZammler))
- Fix telegram-bot rate limit [\#13119](https://github.com/netdata/netdata/pull/13119) ([MAH69IK](https://github.com/MAH69IK))

## [v1.36.1](https://github.com/netdata/netdata/tree/v1.36.1) (2022-08-15)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.36.0...v1.36.1)

## [v1.36.0](https://github.com/netdata/netdata/tree/v1.36.0) (2022-08-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.1...v1.36.0)

**Merged pull requests:**

- rrdcontexts allow not linked dimensions and charts [\#13501](https://github.com/netdata/netdata/pull/13501) ([ktsaou](https://github.com/ktsaou))
- docs: add deprecation notice to python.d/postgres readme [\#13497](https://github.com/netdata/netdata/pull/13497) ([ilyam8](https://github.com/ilyam8))
- docs: change postgres links to go version [\#13496](https://github.com/netdata/netdata/pull/13496) ([ilyam8](https://github.com/ilyam8))
- bump go.d version to v0.35.0 [\#13494](https://github.com/netdata/netdata/pull/13494) ([ilyam8](https://github.com/ilyam8))
- add PgBouncer charts description and icon to dashboard info [\#13493](https://github.com/netdata/netdata/pull/13493) ([ilyam8](https://github.com/ilyam8))
- Add chart\_context to alert snapshots [\#13492](https://github.com/netdata/netdata/pull/13492) ([MrZammler](https://github.com/MrZammler))
- Remove prompt to add dashboard issues [\#13490](https://github.com/netdata/netdata/pull/13490) ([cakrit](https://github.com/cakrit))
- docs: fix unresolved file references [\#13488](https://github.com/netdata/netdata/pull/13488) ([ilyam8](https://github.com/ilyam8))
- docs: add a note about edit-config for docker installs [\#13487](https://github.com/netdata/netdata/pull/13487) ([ilyam8](https://github.com/ilyam8))
- health: disable go python last collected alarms [\#13485](https://github.com/netdata/netdata/pull/13485) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin version to v0.34.0 [\#13484](https://github.com/netdata/netdata/pull/13484) ([ilyam8](https://github.com/ilyam8))
- chore: add WireGuard description and icon to dashboard info [\#13483](https://github.com/netdata/netdata/pull/13483) ([ilyam8](https://github.com/ilyam8))
- feat\(cgroups.plugin\): resolve nomad containers name \(docker driver only\) [\#13481](https://github.com/netdata/netdata/pull/13481) ([ilyam8](https://github.com/ilyam8))
- Check for protected when excluding mounts [\#13479](https://github.com/netdata/netdata/pull/13479) ([MrZammler](https://github.com/MrZammler))
- update postgres dashboard info [\#13474](https://github.com/netdata/netdata/pull/13474) ([ilyam8](https://github.com/ilyam8))
- Remove the single threaded arrayallocator optiomization during agent startup [\#13473](https://github.com/netdata/netdata/pull/13473) ([stelfrag](https://github.com/stelfrag))
- Handle cases where entries where stored as text \(with strftime\("%s"\)\) [\#13472](https://github.com/netdata/netdata/pull/13472) ([stelfrag](https://github.com/stelfrag))
- Enable rrdcontexts by default [\#13471](https://github.com/netdata/netdata/pull/13471) ([stelfrag](https://github.com/stelfrag))
- Fix cgroup name detection for docker containers in containerd cgroup [\#13470](https://github.com/netdata/netdata/pull/13470) ([xkisu](https://github.com/xkisu))
- Trimmed-median, trimmed-mean and percentile [\#13469](https://github.com/netdata/netdata/pull/13469) ([ktsaou](https://github.com/ktsaou))
- rrdcontext support for hidden charts [\#13466](https://github.com/netdata/netdata/pull/13466) ([ktsaou](https://github.com/ktsaou))
- Load host labels for archived hosts [\#13464](https://github.com/netdata/netdata/pull/13464) ([stelfrag](https://github.com/stelfrag))
- fix\(python.d/smartd\_log\): handle log rotation [\#13460](https://github.com/netdata/netdata/pull/13460) ([ilyam8](https://github.com/ilyam8))
- docs: add a note about network interface monitoring when running in a Docker container [\#13458](https://github.com/netdata/netdata/pull/13458) ([ilyam8](https://github.com/ilyam8))
- fix a guide so we can reference it's subsections [\#13455](https://github.com/netdata/netdata/pull/13455) ([tkatsoulas](https://github.com/tkatsoulas))
- Revert "Query queue only for queries" [\#13452](https://github.com/netdata/netdata/pull/13452) ([stelfrag](https://github.com/stelfrag))
- /api/v1/weights endpoint [\#13449](https://github.com/netdata/netdata/pull/13449) ([ktsaou](https://github.com/ktsaou))
- Get last\_entry\_t only when st changes [\#13448](https://github.com/netdata/netdata/pull/13448) ([MrZammler](https://github.com/MrZammler))
- additional stats [\#13445](https://github.com/netdata/netdata/pull/13445) ([ktsaou](https://github.com/ktsaou))
- Store host label information in the metadata database [\#13441](https://github.com/netdata/netdata/pull/13441) ([stelfrag](https://github.com/stelfrag))
- Fix typo in PostgreSQL section header [\#13440](https://github.com/netdata/netdata/pull/13440) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix tests so that the actual metadata database is not accessed [\#13439](https://github.com/netdata/netdata/pull/13439) ([stelfrag](https://github.com/stelfrag))
- Delete aclk\_alert table on start streaming from seq 1 batch 1 [\#13438](https://github.com/netdata/netdata/pull/13438) ([MrZammler](https://github.com/MrZammler))
- Fix agent crash when archived host has not been registered to the cloud [\#13437](https://github.com/netdata/netdata/pull/13437) ([stelfrag](https://github.com/stelfrag))
- Dont duplicate buffered bytes [\#13435](https://github.com/netdata/netdata/pull/13435) ([vlvkobal](https://github.com/vlvkobal))
- Show last 15 alerts in notification [\#13434](https://github.com/netdata/netdata/pull/13434) ([MrZammler](https://github.com/MrZammler))
- Query queue only for queries [\#13431](https://github.com/netdata/netdata/pull/13431) ([underhood](https://github.com/underhood))
- Remove octopus from demo-sites [\#13423](https://github.com/netdata/netdata/pull/13423) ([cakrit](https://github.com/cakrit))
- Tiering statistics API endpoint [\#13420](https://github.com/netdata/netdata/pull/13420) ([ktsaou](https://github.com/ktsaou))
- add discord, youtube, linkedin links to README [\#13419](https://github.com/netdata/netdata/pull/13419) ([andrewm4894](https://github.com/andrewm4894))
- add ML bullet point to features section on README [\#13418](https://github.com/netdata/netdata/pull/13418) ([andrewm4894](https://github.com/andrewm4894))
- Set value to SN\_EMPTY\_SLOT if flags is SN\_EMPTY\_SLOT [\#13417](https://github.com/netdata/netdata/pull/13417) ([MrZammler](https://github.com/MrZammler))
- Add missing comma \(handle coverity warning CID 379360\) [\#13413](https://github.com/netdata/netdata/pull/13413) ([stelfrag](https://github.com/stelfrag))
- codacy/lgtm ignore judy sources [\#13411](https://github.com/netdata/netdata/pull/13411) ([underhood](https://github.com/underhood))
- Send chart context with alert events to the cloud [\#13409](https://github.com/netdata/netdata/pull/13409) ([MrZammler](https://github.com/MrZammler))
- Remove SIGSEGV and SIGABRT \(ebpf.plugin\) [\#13407](https://github.com/netdata/netdata/pull/13407) ([thiagoftsm](https://github.com/thiagoftsm))
- minor fixes on metadata fields  [\#13406](https://github.com/netdata/netdata/pull/13406) ([tkatsoulas](https://github.com/tkatsoulas))
- chore\(health\): remove py web\_log alarms [\#13404](https://github.com/netdata/netdata/pull/13404) ([ilyam8](https://github.com/ilyam8))
- Store host system information in the database [\#13402](https://github.com/netdata/netdata/pull/13402) ([stelfrag](https://github.com/stelfrag))
- Fix coverity issue 379240 \(Unchecked return value\) [\#13401](https://github.com/netdata/netdata/pull/13401) ([stelfrag](https://github.com/stelfrag))
- Fix netdata-updater.sh sha256sum on BSDs [\#13391](https://github.com/netdata/netdata/pull/13391) ([tnyeanderson](https://github.com/tnyeanderson))
- chore\(python.d\): clarify haproxy module readme  [\#13388](https://github.com/netdata/netdata/pull/13388) ([ilyam8](https://github.com/ilyam8))
- Fix bitmap unit tests [\#13374](https://github.com/netdata/netdata/pull/13374) ([stelfrag](https://github.com/stelfrag))
- chore\(dashboard\): update chrony dashboard info [\#13371](https://github.com/netdata/netdata/pull/13371) ([ilyam8](https://github.com/ilyam8))
- chore\(python.d\): remove python.d/\* announced in v1.35.0 deprecation notice [\#13370](https://github.com/netdata/netdata/pull/13370) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin version to v0.33.1 [\#13369](https://github.com/netdata/netdata/pull/13369) ([ilyam8](https://github.com/ilyam8))
- Add Oracle Linux 9 to officially supported platforms. [\#13367](https://github.com/netdata/netdata/pull/13367) ([Ferroin](https://github.com/Ferroin))
- Address Coverity issues [\#13364](https://github.com/netdata/netdata/pull/13364) ([stelfrag](https://github.com/stelfrag))
- chore\(python.d\): improve config file parsing error message [\#13363](https://github.com/netdata/netdata/pull/13363) ([ilyam8](https://github.com/ilyam8))
- in source Judy [\#13362](https://github.com/netdata/netdata/pull/13362) ([underhood](https://github.com/underhood))
- Add additional Docker image build with debug info included. [\#13359](https://github.com/netdata/netdata/pull/13359) ([Ferroin](https://github.com/Ferroin))
- Move host tags to netdata\_info [\#13358](https://github.com/netdata/netdata/pull/13358) ([vlvkobal](https://github.com/vlvkobal))
- Get rid of extra comma in OpenTSDB exporting [\#13355](https://github.com/netdata/netdata/pull/13355) ([vlvkobal](https://github.com/vlvkobal))
- Fix chart update ebpf.plugin [\#13351](https://github.com/netdata/netdata/pull/13351) ([thiagoftsm](https://github.com/thiagoftsm))
- add job to move bugs to project board [\#13350](https://github.com/netdata/netdata/pull/13350) ([hugovalente-pm](https://github.com/hugovalente-pm))
- added another way to get ansible plays [\#13349](https://github.com/netdata/netdata/pull/13349) ([mhkarimi1383](https://github.com/mhkarimi1383))
- Send node info message sooner [\#13348](https://github.com/netdata/netdata/pull/13348) ([MrZammler](https://github.com/MrZammler))
- query engine: omit first point if not needed [\#13345](https://github.com/netdata/netdata/pull/13345) ([ktsaou](https://github.com/ktsaou))
- fix 32bit calculation on array allocator [\#13343](https://github.com/netdata/netdata/pull/13343) ([ktsaou](https://github.com/ktsaou))
- fix crash on start on slow disks because ml is initialized before dbengine starts [\#13342](https://github.com/netdata/netdata/pull/13342) ([ktsaou](https://github.com/ktsaou))
- fix\(packaging\): respect CFLAGS arg when building Docker image [\#13340](https://github.com/netdata/netdata/pull/13340) ([ilyam8](https://github.com/ilyam8))
- add github stars badge to readme [\#13338](https://github.com/netdata/netdata/pull/13338) ([andrewm4894](https://github.com/andrewm4894))
- Fix coverity 379241 [\#13336](https://github.com/netdata/netdata/pull/13336) ([MrZammler](https://github.com/MrZammler))
- Rrdcontext [\#13335](https://github.com/netdata/netdata/pull/13335) ([ktsaou](https://github.com/ktsaou))
- Detect stored metric size by page type  [\#13334](https://github.com/netdata/netdata/pull/13334) ([stelfrag](https://github.com/stelfrag))
- Silence compile warnings on external source [\#13332](https://github.com/netdata/netdata/pull/13332) ([MrZammler](https://github.com/MrZammler))
- UpdateNodeCollectors message [\#13330](https://github.com/netdata/netdata/pull/13330) ([MrZammler](https://github.com/MrZammler))
- Cid 379238 379238 [\#13328](https://github.com/netdata/netdata/pull/13328) ([stelfrag](https://github.com/stelfrag))
- Docs fix metric storage [\#13327](https://github.com/netdata/netdata/pull/13327) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix two helgrind reports [\#13325](https://github.com/netdata/netdata/pull/13325) ([vkalintiris](https://github.com/vkalintiris))
- fix\(cgroups.plugin\): adjust kubepods patterns to filter pods when using Kind cluster [\#13324](https://github.com/netdata/netdata/pull/13324) ([ilyam8](https://github.com/ilyam8))
- Add link to docker config section [\#13323](https://github.com/netdata/netdata/pull/13323) ([cakrit](https://github.com/cakrit))
- Guide for troubleshooting Agent with Cloud connection for new nodes [\#13322](https://github.com/netdata/netdata/pull/13322) ([Ancairon](https://github.com/Ancairon))
- fix\(apps.plugin\): adjust `zmstat*` pattern to exclude zoneminder scripts [\#13314](https://github.com/netdata/netdata/pull/13314) ([ilyam8](https://github.com/ilyam8))
- array allocator for dbengine page descriptors [\#13312](https://github.com/netdata/netdata/pull/13312) ([ktsaou](https://github.com/ktsaou))
- fix\(health\): disable go.d last collected alarm for prometheus module [\#13309](https://github.com/netdata/netdata/pull/13309) ([ilyam8](https://github.com/ilyam8))
- Explicitly skip uploads and notifications in third-party repositories. [\#13308](https://github.com/netdata/netdata/pull/13308) ([Ferroin](https://github.com/Ferroin))
- Protect shared variables with log lock. [\#13306](https://github.com/netdata/netdata/pull/13306) ([vkalintiris](https://github.com/vkalintiris))
- Keep rc before freeing it during labels unlink alarms [\#13305](https://github.com/netdata/netdata/pull/13305) ([MrZammler](https://github.com/MrZammler))
- fix\(cgroups.plugin\): adjust kubepods regex to fix name resolution in a kind cluster [\#13302](https://github.com/netdata/netdata/pull/13302) ([ilyam8](https://github.com/ilyam8))
- Null terminate string if file read was not successful [\#13299](https://github.com/netdata/netdata/pull/13299) ([stelfrag](https://github.com/stelfrag))
- fix\(health\): fix incorrect Redis dimension names [\#13296](https://github.com/netdata/netdata/pull/13296) ([ilyam8](https://github.com/ilyam8))
- chore: remove python-mysql from install-required-packages.sh [\#13288](https://github.com/netdata/netdata/pull/13288) ([ilyam8](https://github.com/ilyam8))
- Use new MQTT as default \(revert \#13258\)" [\#13287](https://github.com/netdata/netdata/pull/13287) ([underhood](https://github.com/underhood))
- query engine fixes for alarms and dashboards [\#13282](https://github.com/netdata/netdata/pull/13282) ([ktsaou](https://github.com/ktsaou))
- Better ACLK debug communication log [\#13281](https://github.com/netdata/netdata/pull/13281) ([underhood](https://github.com/underhood))
- bump go.d.plugin version to v0.33.0 [\#13280](https://github.com/netdata/netdata/pull/13280) ([ilyam8](https://github.com/ilyam8))
- Fixes vbi parser in mqtt5 implementation [\#13277](https://github.com/netdata/netdata/pull/13277) ([underhood](https://github.com/underhood))
- Fix alignment in charts endpoint [\#13275](https://github.com/netdata/netdata/pull/13275) ([thiagoftsm](https://github.com/thiagoftsm))
- Dont print io errors for cgroups [\#13274](https://github.com/netdata/netdata/pull/13274) ([vlvkobal](https://github.com/vlvkobal))
- Pluginsd doc [\#13273](https://github.com/netdata/netdata/pull/13273) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove obsolete --use-system-lws option from netdata-installer.sh help [\#13272](https://github.com/netdata/netdata/pull/13272) ([Dim-P](https://github.com/Dim-P))
- Rename the chart of real memory usage in FreeBSD [\#13271](https://github.com/netdata/netdata/pull/13271) ([vlvkobal](https://github.com/vlvkobal))
- Update documentation about our REST API documentation. [\#13269](https://github.com/netdata/netdata/pull/13269) ([Ferroin](https://github.com/Ferroin))
- Fix package build filtering on PRs. [\#13267](https://github.com/netdata/netdata/pull/13267) ([Ferroin](https://github.com/Ferroin))
- Add document explaining how to proxy Netdata via H2O [\#13266](https://github.com/netdata/netdata/pull/13266) ([Ferroin](https://github.com/Ferroin))
- chore\(python.d\): remove deprecated modules from python.d.conf [\#13264](https://github.com/netdata/netdata/pull/13264) ([ilyam8](https://github.com/ilyam8))
- Multi-Tier database backend for long term metrics storage [\#13263](https://github.com/netdata/netdata/pull/13263) ([stelfrag](https://github.com/stelfrag))
- Get rid of extra semicolon in Graphite exporting [\#13261](https://github.com/netdata/netdata/pull/13261) ([vlvkobal](https://github.com/vlvkobal))
- fix RAM calculation on macOS in system-info [\#13260](https://github.com/netdata/netdata/pull/13260) ([ilyam8](https://github.com/ilyam8))
- Ebpf issues [\#13259](https://github.com/netdata/netdata/pull/13259) ([thiagoftsm](https://github.com/thiagoftsm))
- Use old mqtt implementation as default [\#13258](https://github.com/netdata/netdata/pull/13258) ([MrZammler](https://github.com/MrZammler))
- Remove warnings while compiling ML on FreeBSD [\#13255](https://github.com/netdata/netdata/pull/13255) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix issues with DEB postinstall script. [\#13252](https://github.com/netdata/netdata/pull/13252) ([Ferroin](https://github.com/Ferroin))
- Remove strftime from statements and use unixepoch instead [\#13250](https://github.com/netdata/netdata/pull/13250) ([stelfrag](https://github.com/stelfrag))
- Query engine with natural and virtual points [\#13248](https://github.com/netdata/netdata/pull/13248) ([ktsaou](https://github.com/ktsaou))
- Add fstype labels to disk charts [\#13245](https://github.com/netdata/netdata/pull/13245) ([vlvkobal](https://github.com/vlvkobal))
- Don’t pull in GCC for build if Clang is already present. [\#13244](https://github.com/netdata/netdata/pull/13244) ([Ferroin](https://github.com/Ferroin))
- Delay health until obsoletions check is complete [\#13239](https://github.com/netdata/netdata/pull/13239) ([MrZammler](https://github.com/MrZammler))
- Improve anomaly detection guide [\#13238](https://github.com/netdata/netdata/pull/13238) ([andrewm4894](https://github.com/andrewm4894))
- Implement PackageCloud cleanup [\#13236](https://github.com/netdata/netdata/pull/13236) ([maneamarius](https://github.com/maneamarius))
- Bump repoconfig package version used in kickstart.sh [\#13235](https://github.com/netdata/netdata/pull/13235) ([Ferroin](https://github.com/Ferroin))
- Updates the sqlite version in the agent [\#13233](https://github.com/netdata/netdata/pull/13233) ([stelfrag](https://github.com/stelfrag))
- Migrate data when machine GUID changes [\#13232](https://github.com/netdata/netdata/pull/13232) ([stelfrag](https://github.com/stelfrag))
- Add more sqlite unittests [\#13227](https://github.com/netdata/netdata/pull/13227) ([stelfrag](https://github.com/stelfrag))
- ci: add issues to the Agent Board project workflow [\#13225](https://github.com/netdata/netdata/pull/13225) ([ilyam8](https://github.com/ilyam8))
- Exporting/send variables [\#13221](https://github.com/netdata/netdata/pull/13221) ([boxjan](https://github.com/boxjan))
- fix\(cgroups.plugin\): fix qemu VMs and LXC containers name resolution [\#13220](https://github.com/netdata/netdata/pull/13220) ([ilyam8](https://github.com/ilyam8))
- netdata doubles [\#13217](https://github.com/netdata/netdata/pull/13217) ([ktsaou](https://github.com/ktsaou))
- deduplicate mountinfo based on mount point [\#13215](https://github.com/netdata/netdata/pull/13215) ([ktsaou](https://github.com/ktsaou))
- feat\(python.d\): load modules from user plugin directories \(NETDATA\_USER\_PLUGINS\_DIRS\) [\#13214](https://github.com/netdata/netdata/pull/13214) ([ilyam8](https://github.com/ilyam8))
- Properly handle interactivity in the updater code. [\#13209](https://github.com/netdata/netdata/pull/13209) ([Ferroin](https://github.com/Ferroin))
- Don’t use realpath to find kickstart source path. [\#13208](https://github.com/netdata/netdata/pull/13208) ([Ferroin](https://github.com/Ferroin))
- Print INTERNAL BUG messages only when NETDATA\_INTERNAL\_CHECKS is enabled [\#13207](https://github.com/netdata/netdata/pull/13207) ([MrZammler](https://github.com/MrZammler))
- Ensure tmpdir is set for every function that uses it. [\#13206](https://github.com/netdata/netdata/pull/13206) ([Ferroin](https://github.com/Ferroin))
- Add user plugin dirs to environment [\#13203](https://github.com/netdata/netdata/pull/13203) ([vlvkobal](https://github.com/vlvkobal))
- Fix cgroups netdev chart labels [\#13200](https://github.com/netdata/netdata/pull/13200) ([vlvkobal](https://github.com/vlvkobal))
- Add hostname in the worker structure to avoid constant lookups [\#13199](https://github.com/netdata/netdata/pull/13199) ([stelfrag](https://github.com/stelfrag))
- Rpm group creation [\#13197](https://github.com/netdata/netdata/pull/13197) ([iigorkarpov](https://github.com/iigorkarpov))
- Allow for an easy way to do metadata migrations [\#13196](https://github.com/netdata/netdata/pull/13196) ([stelfrag](https://github.com/stelfrag))
- Dictionaries with reference counters and full deletion support during traversal [\#13195](https://github.com/netdata/netdata/pull/13195) ([ktsaou](https://github.com/ktsaou))
- Add configuration for dbengine page fetch timeout and retry count [\#13194](https://github.com/netdata/netdata/pull/13194) ([stelfrag](https://github.com/stelfrag))
- Clean sqlite prepared statements on thread shutdown [\#13193](https://github.com/netdata/netdata/pull/13193) ([stelfrag](https://github.com/stelfrag))
- Update dashboard to version v2.26.5. [\#13192](https://github.com/netdata/netdata/pull/13192) ([netdatabot](https://github.com/netdatabot))
- chore\(netdata-installer\): remove a call to 'cleanup\_old\_netdata\_updater\(\)' because it is no longer exists [\#13189](https://github.com/netdata/netdata/pull/13189) ([ilyam8](https://github.com/ilyam8))
- feat\(python.d/smartd\_log\): add 2nd job that tries to read from '/var/lib/smartmontools/' [\#13188](https://github.com/netdata/netdata/pull/13188) ([ilyam8](https://github.com/ilyam8))
- Add type label for network interfaces [\#13187](https://github.com/netdata/netdata/pull/13187) ([vlvkobal](https://github.com/vlvkobal))
- fix\(freebsd.plugin\): fix wired/cached/avail memory calculation on FreeBSD with ZFS [\#13183](https://github.com/netdata/netdata/pull/13183) ([ilyam8](https://github.com/ilyam8))
- make configuration example clearer [\#13182](https://github.com/netdata/netdata/pull/13182) ([andrewm4894](https://github.com/andrewm4894))
- add k8s\_state dashboard\_info [\#13181](https://github.com/netdata/netdata/pull/13181) ([ilyam8](https://github.com/ilyam8))
- Docs housekeeping [\#13179](https://github.com/netdata/netdata/pull/13179) ([tkatsoulas](https://github.com/tkatsoulas))
- Update dashboard to version v2.26.2. [\#13177](https://github.com/netdata/netdata/pull/13177) ([netdatabot](https://github.com/netdatabot))
- feat\(proc/proc\_net\_dev\): add dim per phys link state to the "Interface Physical Link State" chart [\#13176](https://github.com/netdata/netdata/pull/13176) ([ilyam8](https://github.com/ilyam8))
- set default for `minimum num samples to train` to `900` [\#13174](https://github.com/netdata/netdata/pull/13174) ([andrewm4894](https://github.com/andrewm4894))
- Add ml alerts examples [\#13173](https://github.com/netdata/netdata/pull/13173) ([andrewm4894](https://github.com/andrewm4894))
- Revert "Configurable storage engine for Netdata agents: step 3 \(\#12892\)" [\#13171](https://github.com/netdata/netdata/pull/13171) ([vkalintiris](https://github.com/vkalintiris))
- Remove warnings when openssl 3 is used. [\#13170](https://github.com/netdata/netdata/pull/13170) ([thiagoftsm](https://github.com/thiagoftsm))
- Don’t manipulate positional parameters in DEB postinst script. [\#13169](https://github.com/netdata/netdata/pull/13169) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues [\#13168](https://github.com/netdata/netdata/pull/13168) ([stelfrag](https://github.com/stelfrag))
- feat\(proc/proc\_net\_dev\): add dim per operstate to the "Interface Operational State" chart [\#13167](https://github.com/netdata/netdata/pull/13167) ([ilyam8](https://github.com/ilyam8))
- feat\(proc/proc\_net\_dev\): add dim per duplex state to the "Interface Duplex State" chart [\#13165](https://github.com/netdata/netdata/pull/13165) ([ilyam8](https://github.com/ilyam8))
- allow traversing null-value dictionaries [\#13162](https://github.com/netdata/netdata/pull/13162) ([ktsaou](https://github.com/ktsaou))
- Use memset to mark the empty words in the quoted\_strings\_splitter function [\#13161](https://github.com/netdata/netdata/pull/13161) ([stelfrag](https://github.com/stelfrag))
- Fix data query on stale chart [\#13159](https://github.com/netdata/netdata/pull/13159) ([stelfrag](https://github.com/stelfrag))
- enable ml by default [\#13158](https://github.com/netdata/netdata/pull/13158) ([andrewm4894](https://github.com/andrewm4894))
- Fix labels unit test [\#13156](https://github.com/netdata/netdata/pull/13156) ([stelfrag](https://github.com/stelfrag))
- Query Engine multi-granularity support \(and MC improvements\) [\#13155](https://github.com/netdata/netdata/pull/13155) ([ktsaou](https://github.com/ktsaou))
- add CAP\_SYS\_RAWIO to Netdata's systemd unit CapabilityBoundingSet [\#13154](https://github.com/netdata/netdata/pull/13154) ([ilyam8](https://github.com/ilyam8))
- Update docs on what to do if collector not there [\#13152](https://github.com/netdata/netdata/pull/13152) ([cakrit](https://github.com/cakrit))
- revert to default of `host anomaly rate threshold=0.01` [\#13150](https://github.com/netdata/netdata/pull/13150) ([andrewm4894](https://github.com/andrewm4894))
- fix conditions for nightly build triggers [\#13145](https://github.com/netdata/netdata/pull/13145) ([maneamarius](https://github.com/maneamarius))
- Add cargo/rustc/bazel/buck to apps\_groups.conf [\#13143](https://github.com/netdata/netdata/pull/13143) ([vkalintiris](https://github.com/vkalintiris))
- Add an option to use malloc for page cache instead of mmap [\#13142](https://github.com/netdata/netdata/pull/13142) ([stelfrag](https://github.com/stelfrag))
- Add mem.available chart to FreeBSD [\#13140](https://github.com/netdata/netdata/pull/13140) ([MrZammler](https://github.com/MrZammler))
- fix crashes due to misaligned allocations [\#13137](https://github.com/netdata/netdata/pull/13137) ([ktsaou](https://github.com/ktsaou))
- fix\(python.d\): urllib3 import collection for py3.10+ [\#13136](https://github.com/netdata/netdata/pull/13136) ([ilyam8](https://github.com/ilyam8))
- fix\(python.d/mongodb\): set `serverSelectionTimeoutMS` for pymongo4+ [\#13135](https://github.com/netdata/netdata/pull/13135) ([ilyam8](https://github.com/ilyam8))
- Use new mqtt implementation as default [\#13132](https://github.com/netdata/netdata/pull/13132) ([underhood](https://github.com/underhood))
- use ks2 as MC default [\#13131](https://github.com/netdata/netdata/pull/13131) ([andrewm4894](https://github.com/andrewm4894))
- allow label names to have slashes [\#13125](https://github.com/netdata/netdata/pull/13125) ([ktsaou](https://github.com/ktsaou))
- fixed coveriry 379136 379135 379134 379133 [\#13123](https://github.com/netdata/netdata/pull/13123) ([ktsaou](https://github.com/ktsaou))
- buffer overflow detected by the compiler [\#13120](https://github.com/netdata/netdata/pull/13120) ([ktsaou](https://github.com/ktsaou))
- Ci coverage [\#13118](https://github.com/netdata/netdata/pull/13118) ([maneamarius](https://github.com/maneamarius))
- Add missing control to streaming [\#13112](https://github.com/netdata/netdata/pull/13112) ([thiagoftsm](https://github.com/thiagoftsm))
- Removes Legacy JSON Cloud Protocol Support In Agent [\#13111](https://github.com/netdata/netdata/pull/13111) ([underhood](https://github.com/underhood))
- Re-enable updates for systems using static builds. [\#13110](https://github.com/netdata/netdata/pull/13110) ([Ferroin](https://github.com/Ferroin))
- Add user netdata to secondary group in DEB package [\#13109](https://github.com/netdata/netdata/pull/13109) ([iigorkarpov](https://github.com/iigorkarpov))
- Remove pinned page reference [\#13108](https://github.com/netdata/netdata/pull/13108) ([stelfrag](https://github.com/stelfrag))
- 73x times faster metrics correlations at the agent [\#13107](https://github.com/netdata/netdata/pull/13107) ([ktsaou](https://github.com/ktsaou))
- fix\(updater\): fix updating when using `--force-update` and new version of the updater script is available [\#13104](https://github.com/netdata/netdata/pull/13104) ([ilyam8](https://github.com/ilyam8))
- Remove unnescesary ‘cleanup’ code. [\#13103](https://github.com/netdata/netdata/pull/13103) ([Ferroin](https://github.com/Ferroin))
- Temporarily disable updates for static builds. [\#13100](https://github.com/netdata/netdata/pull/13100) ([Ferroin](https://github.com/Ferroin))
- docs\(statsd.plugin\): fix indentation [\#13096](https://github.com/netdata/netdata/pull/13096) ([ilyam8](https://github.com/ilyam8))
- Statistics on bytes recvd and sent [\#13091](https://github.com/netdata/netdata/pull/13091) ([underhood](https://github.com/underhood))
- fix virtualization detection on FreeBSD [\#13087](https://github.com/netdata/netdata/pull/13087) ([ilyam8](https://github.com/ilyam8))
- Update netdata commands [\#13080](https://github.com/netdata/netdata/pull/13080) ([tkatsoulas](https://github.com/tkatsoulas))
- fix: fix a base64\_encode bug [\#13074](https://github.com/netdata/netdata/pull/13074) ([kklionz](https://github.com/kklionz))
- Labels with dictionary [\#13070](https://github.com/netdata/netdata/pull/13070) ([ktsaou](https://github.com/ktsaou))
- Use a separate thread for slow mountpoints in the diskspace plugin [\#13067](https://github.com/netdata/netdata/pull/13067) ([vlvkobal](https://github.com/vlvkobal))
- Remove official support for Debian 9. [\#13065](https://github.com/netdata/netdata/pull/13065) ([Ferroin](https://github.com/Ferroin))

## [v1.35.1](https://github.com/netdata/netdata/tree/v1.35.1) (2022-06-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.35.0...v1.35.1)

## [v1.35.0](https://github.com/netdata/netdata/tree/v1.35.0) (2022-06-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.34.1...v1.35.0)

**Merged pull requests:**

- Update README.md [\#13089](https://github.com/netdata/netdata/pull/13089) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#13088](https://github.com/netdata/netdata/pull/13088) ([ktsaou](https://github.com/ktsaou))
- fix\(updater\): return 0 on successful update for native packages when running interactively [\#13083](https://github.com/netdata/netdata/pull/13083) ([ilyam8](https://github.com/ilyam8))
- Fix Coverity errors in mqtt\_websockets submodule [\#13082](https://github.com/netdata/netdata/pull/13082) ([underhood](https://github.com/underhood))
- fix\(updater\): don't produce any output if binpkg update completed successfully [\#13081](https://github.com/netdata/netdata/pull/13081) ([ilyam8](https://github.com/ilyam8))
- Fix handling of DEB package naming in CI. [\#13076](https://github.com/netdata/netdata/pull/13076) ([Ferroin](https://github.com/Ferroin))
- Update default value for "host anomaly rate threshold" [\#13075](https://github.com/netdata/netdata/pull/13075) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix locking access to chart labels [\#13064](https://github.com/netdata/netdata/pull/13064) ([stelfrag](https://github.com/stelfrag))

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
