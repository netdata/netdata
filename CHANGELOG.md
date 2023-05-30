# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.39.1...HEAD)

**Merged pull requests:**

- Python Dependency Migration - OracleDB Python Module [\#15074](https://github.com/netdata/netdata/pull/15074) ([EricAndrechek](https://github.com/EricAndrechek))
- Free context when establishing ACLK connection [\#15073](https://github.com/netdata/netdata/pull/15073) ([stelfrag](https://github.com/stelfrag))
- Update Security doc [\#15072](https://github.com/netdata/netdata/pull/15072) ([tkatsoulas](https://github.com/tkatsoulas))
- Update netdata-security.md [\#15068](https://github.com/netdata/netdata/pull/15068) ([cakrit](https://github.com/cakrit))
- Update netdata-security.md [\#15067](https://github.com/netdata/netdata/pull/15067) ([cakrit](https://github.com/cakrit))
- Simplify loop in alert checkpoint [\#15065](https://github.com/netdata/netdata/pull/15065) ([MrZammler](https://github.com/MrZammler))
- Update CODEOWNERS [\#15064](https://github.com/netdata/netdata/pull/15064) ([cakrit](https://github.com/cakrit))
- Update netdata-security.md [\#15063](https://github.com/netdata/netdata/pull/15063) ([sashwathn](https://github.com/sashwathn))
- Fix CodeQL warning  [\#15062](https://github.com/netdata/netdata/pull/15062) ([stelfrag](https://github.com/stelfrag))
- Fix memory leak when sending alerts checkoint [\#15060](https://github.com/netdata/netdata/pull/15060) ([stelfrag](https://github.com/stelfrag))
- bump go.d.plugin to v0.53.0 [\#15059](https://github.com/netdata/netdata/pull/15059) ([ilyam8](https://github.com/ilyam8))
- Fix ACLK memleak [\#15055](https://github.com/netdata/netdata/pull/15055) ([underhood](https://github.com/underhood))
- fix\(debugfs/zswap\): don't collect metrics if Zswap is disabled [\#15054](https://github.com/netdata/netdata/pull/15054) ([ilyam8](https://github.com/ilyam8))
- Comment out default `role_recipients_*` values [\#15047](https://github.com/netdata/netdata/pull/15047) ([jamgregory](https://github.com/jamgregory))
- Small update ml defaults [\#15046](https://github.com/netdata/netdata/pull/15046) ([andrewm4894](https://github.com/andrewm4894))
- Better cleanup of health log table [\#15045](https://github.com/netdata/netdata/pull/15045) ([MrZammler](https://github.com/MrZammler))
- Fix handling of permissions in static installs. [\#15042](https://github.com/netdata/netdata/pull/15042) ([Ferroin](https://github.com/Ferroin))
- Update tor.chart.py [\#15041](https://github.com/netdata/netdata/pull/15041) ([jmphilippe](https://github.com/jmphilippe))
- Wording fix in interact with charts doc [\#15040](https://github.com/netdata/netdata/pull/15040) ([Ancairon](https://github.com/Ancairon))
- fatal in claim\(\) only if --claim-only is used [\#15039](https://github.com/netdata/netdata/pull/15039) ([ilyam8](https://github.com/ilyam8))
- Slight wording fix on the database readme [\#15034](https://github.com/netdata/netdata/pull/15034) ([Ancairon](https://github.com/Ancairon))
- Update SQLITE to version 3.41.2 [\#15031](https://github.com/netdata/netdata/pull/15031) ([stelfrag](https://github.com/stelfrag))
- Update troubleshooting-agent-with-cloud-connection.md [\#15029](https://github.com/netdata/netdata/pull/15029) ([cakrit](https://github.com/cakrit))
- Adjust buffers to prevent overflow [\#15025](https://github.com/netdata/netdata/pull/15025) ([stelfrag](https://github.com/stelfrag))
- Reduce netdatacli size [\#15024](https://github.com/netdata/netdata/pull/15024) ([stelfrag](https://github.com/stelfrag))
- Debugfs collector [\#15017](https://github.com/netdata/netdata/pull/15017) ([thiagoftsm](https://github.com/thiagoftsm))
- review the billing docs for the flow [\#15014](https://github.com/netdata/netdata/pull/15014) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Rollback ML transaction on failure. [\#15013](https://github.com/netdata/netdata/pull/15013) ([vkalintiris](https://github.com/vkalintiris))
- Silence dimensions with noisy ML models [\#15011](https://github.com/netdata/netdata/pull/15011) ([vkalintiris](https://github.com/vkalintiris))
- Update chart documentation [\#15010](https://github.com/netdata/netdata/pull/15010) ([Ancairon](https://github.com/Ancairon))
- Honor maximum message size limit of MQTT server [\#15009](https://github.com/netdata/netdata/pull/15009) ([underhood](https://github.com/underhood))
- libjudy: remove JudyLTablesGen [\#14984](https://github.com/netdata/netdata/pull/14984) ([mochaaP](https://github.com/mochaaP))
- Use chart labels to filter alerts [\#14982](https://github.com/netdata/netdata/pull/14982) ([MrZammler](https://github.com/MrZammler))
- Remove Fedora 36 from CI and platform support. [\#14938](https://github.com/netdata/netdata/pull/14938) ([Ferroin](https://github.com/Ferroin))
- make zlib compulsory dep [\#14928](https://github.com/netdata/netdata/pull/14928) ([underhood](https://github.com/underhood))
- Try to detect bind mounts [\#14831](https://github.com/netdata/netdata/pull/14831) ([MrZammler](https://github.com/MrZammler))
- Remove old logic for handling of legacy stock config files. [\#14829](https://github.com/netdata/netdata/pull/14829) ([Ferroin](https://github.com/Ferroin))
- fix infiniband bytes counters multiplier and divisor [\#14748](https://github.com/netdata/netdata/pull/14748) ([ilyam8](https://github.com/ilyam8))
- initial minimal h2o webserver integration [\#14585](https://github.com/netdata/netdata/pull/14585) ([underhood](https://github.com/underhood))

## [v1.39.1](https://github.com/netdata/netdata/tree/v1.39.1) (2023-05-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.39.0...v1.39.1)

## [v1.39.0](https://github.com/netdata/netdata/tree/v1.39.0) (2023-05-08)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.38.1...v1.39.0)

**Merged pull requests:**

- Fix typo in file capabilities settings in static installer. [\#15023](https://github.com/netdata/netdata/pull/15023) ([Ferroin](https://github.com/Ferroin))
- On data and weight queries now instances filter matches also instance\_id@node\_id [\#15021](https://github.com/netdata/netdata/pull/15021) ([ktsaou](https://github.com/ktsaou))
- add `grafana` to `apps_groups.conf` [\#15020](https://github.com/netdata/netdata/pull/15020) ([andrewm4894](https://github.com/andrewm4894))
- Set file capabilities correctly on static installs. [\#15018](https://github.com/netdata/netdata/pull/15018) ([Ferroin](https://github.com/Ferroin))
- Differentiate error codes better when claiming from kickstart script. [\#15015](https://github.com/netdata/netdata/pull/15015) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues  [\#15005](https://github.com/netdata/netdata/pull/15005) ([stelfrag](https://github.com/stelfrag))
- weights endpoint: volume diff of anomaly rates [\#15004](https://github.com/netdata/netdata/pull/15004) ([ktsaou](https://github.com/ktsaou))
- Bump Coverity scan tool version. [\#15003](https://github.com/netdata/netdata/pull/15003) ([Ferroin](https://github.com/Ferroin))
- feat\(apps.plugin\): collect context switches  [\#15002](https://github.com/netdata/netdata/pull/15002) ([ilyam8](https://github.com/ilyam8))
- add support for monitoring thp, ballooning, zswap, ksm cow [\#15000](https://github.com/netdata/netdata/pull/15000) ([ktsaou](https://github.com/ktsaou))
- Fix cmake errors [\#14998](https://github.com/netdata/netdata/pull/14998) ([stelfrag](https://github.com/stelfrag))
- Remove lighttpd2 from docs [\#14997](https://github.com/netdata/netdata/pull/14997) ([Ancairon](https://github.com/Ancairon))
- Update netdata-security.md [\#14995](https://github.com/netdata/netdata/pull/14995) ([cakrit](https://github.com/cakrit))
- feat: add OpsGenie alert levels to payload [\#14992](https://github.com/netdata/netdata/pull/14992) ([OliverNChalk](https://github.com/OliverNChalk))
- disable CPU full pressure at the system level [\#14991](https://github.com/netdata/netdata/pull/14991) ([ilyam8](https://github.com/ilyam8))
- fix config generation for plugins [\#14990](https://github.com/netdata/netdata/pull/14990) ([ilyam8](https://github.com/ilyam8))
- Load/Store ML models [\#14981](https://github.com/netdata/netdata/pull/14981) ([vkalintiris](https://github.com/vkalintiris))
- Fix TYPO in README.md [\#14980](https://github.com/netdata/netdata/pull/14980) ([DevinNorgarb](https://github.com/DevinNorgarb))
- add metrics.csv to proc.plugin [\#14979](https://github.com/netdata/netdata/pull/14979) ([thiagoftsm](https://github.com/thiagoftsm))
- interrupt callback on api/v1/data [\#14978](https://github.com/netdata/netdata/pull/14978) ([ktsaou](https://github.com/ktsaou))
- add metrics.csv to macos, freebsd and cgroups plugins [\#14977](https://github.com/netdata/netdata/pull/14977) ([ilyam8](https://github.com/ilyam8))
- fix adding chart labels in tc.plugin [\#14976](https://github.com/netdata/netdata/pull/14976) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to some c collectors [\#14974](https://github.com/netdata/netdata/pull/14974) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to perf.plugin [\#14973](https://github.com/netdata/netdata/pull/14973) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to xenstat.plugin [\#14972](https://github.com/netdata/netdata/pull/14972) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to slabinfo.plugin [\#14971](https://github.com/netdata/netdata/pull/14971) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to timex.plugin [\#14970](https://github.com/netdata/netdata/pull/14970) ([ilyam8](https://github.com/ilyam8))
- do not convert to percentage, when the raw option is given [\#14969](https://github.com/netdata/netdata/pull/14969) ([ktsaou](https://github.com/ktsaou))
- add metrics.csv to apps.plugin [\#14968](https://github.com/netdata/netdata/pull/14968) ([ilyam8](https://github.com/ilyam8))
- add metrics.csv to charts.d [\#14966](https://github.com/netdata/netdata/pull/14966) ([ilyam8](https://github.com/ilyam8))
- Add metrics.csv for ebpf [\#14965](https://github.com/netdata/netdata/pull/14965) ([thiagoftsm](https://github.com/thiagoftsm))
- Update ML README.md [\#14964](https://github.com/netdata/netdata/pull/14964) ([ktsaou](https://github.com/ktsaou))
- Document netdatacli dumpconfig option [\#14963](https://github.com/netdata/netdata/pull/14963) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14962](https://github.com/netdata/netdata/pull/14962) ([cakrit](https://github.com/cakrit))
- Fix handling of users and groups on install. [\#14961](https://github.com/netdata/netdata/pull/14961) ([Ferroin](https://github.com/Ferroin))
- Reject child when context is loading [\#14960](https://github.com/netdata/netdata/pull/14960) ([stelfrag](https://github.com/stelfrag))
- Add metadata.csv to python.d.plugin [\#14959](https://github.com/netdata/netdata/pull/14959) ([Ancairon](https://github.com/Ancairon))
- Address log issue [\#14958](https://github.com/netdata/netdata/pull/14958) ([thiagoftsm](https://github.com/thiagoftsm))
- Add adaptec\_raid metrics.csv [\#14955](https://github.com/netdata/netdata/pull/14955) ([Ancairon](https://github.com/Ancairon))
- Set api v2 version 2 [\#14954](https://github.com/netdata/netdata/pull/14954) ([stelfrag](https://github.com/stelfrag))
- Cancel Pending Request [\#14953](https://github.com/netdata/netdata/pull/14953) ([underhood](https://github.com/underhood))
- Terminate JSX element in doc file [\#14952](https://github.com/netdata/netdata/pull/14952) ([Ancairon](https://github.com/Ancairon))
- Update dbengine README.md [\#14951](https://github.com/netdata/netdata/pull/14951) ([ktsaou](https://github.com/ktsaou))
- Prevent pager from preventing non-interactive install [\#14950](https://github.com/netdata/netdata/pull/14950) ([bompus](https://github.com/bompus))
- Update README.md [\#14948](https://github.com/netdata/netdata/pull/14948) ([cakrit](https://github.com/cakrit))
- Disable SQL operations in training thread [\#14947](https://github.com/netdata/netdata/pull/14947) ([vkalintiris](https://github.com/vkalintiris))
- Add support for acquire/release operations on RRDSETs [\#14945](https://github.com/netdata/netdata/pull/14945) ([vkalintiris](https://github.com/vkalintiris))
- fix 32bit segv [\#14940](https://github.com/netdata/netdata/pull/14940) ([ktsaou](https://github.com/ktsaou))
- Update using-host-labels.md [\#14939](https://github.com/netdata/netdata/pull/14939) ([cakrit](https://github.com/cakrit))
- Fix broken image, in database/README.md [\#14936](https://github.com/netdata/netdata/pull/14936) ([Ancairon](https://github.com/Ancairon))
- Add a description to proc.plugin/README.md [\#14935](https://github.com/netdata/netdata/pull/14935) ([Ancairon](https://github.com/Ancairon))
- zfspool: add suspended state [\#14934](https://github.com/netdata/netdata/pull/14934) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin v0.52.2 [\#14933](https://github.com/netdata/netdata/pull/14933) ([ilyam8](https://github.com/ilyam8))
- Make the document more generic [\#14932](https://github.com/netdata/netdata/pull/14932) ([cakrit](https://github.com/cakrit))
- Replace "XYZ view" with "XYZ tab" in documentation files [\#14930](https://github.com/netdata/netdata/pull/14930) ([Ancairon](https://github.com/Ancairon))
- Add windows MSI installer start stop restart instructions to docs [\#14929](https://github.com/netdata/netdata/pull/14929) ([Ancairon](https://github.com/Ancairon))
- Add Docker instructions to enable Nvidia GPUs [\#14924](https://github.com/netdata/netdata/pull/14924) ([D34DC3N73R](https://github.com/D34DC3N73R))
- Initialize machine GUID earlier in the agent startup sequence [\#14922](https://github.com/netdata/netdata/pull/14922) ([stelfrag](https://github.com/stelfrag))
- bump go.d.plugin to v0.52.1 [\#14921](https://github.com/netdata/netdata/pull/14921) ([ilyam8](https://github.com/ilyam8))
- Skip ML initialization when it's been disabled in netdata.conf [\#14920](https://github.com/netdata/netdata/pull/14920) ([vkalintiris](https://github.com/vkalintiris))
- Fix warnings and error when compiling with --disable-dbengine [\#14919](https://github.com/netdata/netdata/pull/14919) ([stelfrag](https://github.com/stelfrag))
- minor - remove RX\_MSGLEN\_MAX [\#14918](https://github.com/netdata/netdata/pull/14918) ([underhood](https://github.com/underhood))
- change docusaurus admonitions to our style of admonitions [\#14917](https://github.com/netdata/netdata/pull/14917) ([Ancairon](https://github.com/Ancairon))
- Add windows diagram [\#14916](https://github.com/netdata/netdata/pull/14916) ([cakrit](https://github.com/cakrit))
- Add section for scaling parent nodes [\#14915](https://github.com/netdata/netdata/pull/14915) ([cakrit](https://github.com/cakrit))
- Update suggested replication setups [\#14914](https://github.com/netdata/netdata/pull/14914) ([cakrit](https://github.com/cakrit))
- Revert ML changes. [\#14908](https://github.com/netdata/netdata/pull/14908) ([vkalintiris](https://github.com/vkalintiris))
- Remove netdatacli response size limitation [\#14906](https://github.com/netdata/netdata/pull/14906) ([stelfrag](https://github.com/stelfrag))
- Update change-metrics-storage.md [\#14905](https://github.com/netdata/netdata/pull/14905) ([cakrit](https://github.com/cakrit))
- /api/v2 part 10 [\#14904](https://github.com/netdata/netdata/pull/14904) ([ktsaou](https://github.com/ktsaou))
- Optimize the cheat sheet to be in a printable form factor  [\#14903](https://github.com/netdata/netdata/pull/14903) ([Ancairon](https://github.com/Ancairon))
- Address issues on `EC2` \(eBPF\). [\#14902](https://github.com/netdata/netdata/pull/14902) ([thiagoftsm](https://github.com/thiagoftsm))
- Update REFERENCE.md [\#14900](https://github.com/netdata/netdata/pull/14900) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14899](https://github.com/netdata/netdata/pull/14899) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14898](https://github.com/netdata/netdata/pull/14898) ([cakrit](https://github.com/cakrit))
- Disable threads while we are investigating [\#14897](https://github.com/netdata/netdata/pull/14897) ([thiagoftsm](https://github.com/thiagoftsm))
- add opsgenie as a business level notificaiton method [\#14895](https://github.com/netdata/netdata/pull/14895) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Remove dry run option from uninstall documentation [\#14894](https://github.com/netdata/netdata/pull/14894) ([Ancairon](https://github.com/Ancairon))
- cgroups: add option to use Kubelet for pods metadata [\#14891](https://github.com/netdata/netdata/pull/14891) ([ilyam8](https://github.com/ilyam8))
- /api/v2 part 9 [\#14888](https://github.com/netdata/netdata/pull/14888) ([ktsaou](https://github.com/ktsaou))
- Add example configuration to w1sensor collector [\#14886](https://github.com/netdata/netdata/pull/14886) ([Ancairon](https://github.com/Ancairon))
- /api/v2 part 8 [\#14885](https://github.com/netdata/netdata/pull/14885) ([ktsaou](https://github.com/ktsaou))
- Fix/introduce links inside charts.d.plugin documentation  [\#14884](https://github.com/netdata/netdata/pull/14884) ([Ancairon](https://github.com/Ancairon))
- Add support for alert notifications to ntfy.sh [\#14875](https://github.com/netdata/netdata/pull/14875) ([Dim-P](https://github.com/Dim-P))
- WEBRTC for communication between agents and browsers [\#14874](https://github.com/netdata/netdata/pull/14874) ([ktsaou](https://github.com/ktsaou))
- Remove alpine 3.14 from the ci [\#14873](https://github.com/netdata/netdata/pull/14873) ([tkatsoulas](https://github.com/tkatsoulas))
- cgroups.plugin: add image label [\#14872](https://github.com/netdata/netdata/pull/14872) ([ilyam8](https://github.com/ilyam8))
- Fix regex syntax for clang-format checks. [\#14871](https://github.com/netdata/netdata/pull/14871) ([Ferroin](https://github.com/Ferroin))
- bump go.d.plugin v0.52.0 [\#14870](https://github.com/netdata/netdata/pull/14870) ([ilyam8](https://github.com/ilyam8))
- eBPF bug fixes [\#14869](https://github.com/netdata/netdata/pull/14869) ([thiagoftsm](https://github.com/thiagoftsm))
- Update link from http to https [\#14864](https://github.com/netdata/netdata/pull/14864) ([Ancairon](https://github.com/Ancairon))
- Fix js tag in documentation [\#14862](https://github.com/netdata/netdata/pull/14862) ([Ancairon](https://github.com/Ancairon))
- Set a default registry unique id when there is none for statistics script [\#14861](https://github.com/netdata/netdata/pull/14861) ([MrZammler](https://github.com/MrZammler))
- review usage of you to say user instead [\#14858](https://github.com/netdata/netdata/pull/14858) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Add labels for cgroup name [\#14856](https://github.com/netdata/netdata/pull/14856) ([thiagoftsm](https://github.com/thiagoftsm))
- fix typo alerms -\> alarms [\#14854](https://github.com/netdata/netdata/pull/14854) ([slavox](https://github.com/slavox))
- Add a checkpoint message to alerts stream [\#14847](https://github.com/netdata/netdata/pull/14847) ([MrZammler](https://github.com/MrZammler))
- fix  \#14841 Exception funktion call Rados.mon\_command\(\) [\#14844](https://github.com/netdata/netdata/pull/14844) ([farax4de](https://github.com/farax4de))
- Update parent child examples [\#14842](https://github.com/netdata/netdata/pull/14842) ([cakrit](https://github.com/cakrit))
- fix double host prefix when reading ZFS pools state [\#14840](https://github.com/netdata/netdata/pull/14840) ([ilyam8](https://github.com/ilyam8))
- Update enable-notifications.md [\#14838](https://github.com/netdata/netdata/pull/14838) ([cakrit](https://github.com/cakrit))
- Update deployment-strategies.md [\#14837](https://github.com/netdata/netdata/pull/14837) ([cakrit](https://github.com/cakrit))
- Update database engine readme [\#14836](https://github.com/netdata/netdata/pull/14836) ([cakrit](https://github.com/cakrit))
- Update change-metrics-storage.md [\#14835](https://github.com/netdata/netdata/pull/14835) ([cakrit](https://github.com/cakrit))
- Update change-metrics-storage.md [\#14834](https://github.com/netdata/netdata/pull/14834) ([cakrit](https://github.com/cakrit))
- Update netdata-security.md [\#14833](https://github.com/netdata/netdata/pull/14833) ([cakrit](https://github.com/cakrit))
- Boost dbengine [\#14832](https://github.com/netdata/netdata/pull/14832) ([ktsaou](https://github.com/ktsaou))
- add some third party collectors [\#14830](https://github.com/netdata/netdata/pull/14830) ([andrewm4894](https://github.com/andrewm4894))
- Add opsgenie integration docs [\#14828](https://github.com/netdata/netdata/pull/14828) ([iorvd](https://github.com/iorvd))
- Update Agent notification methods documentation [\#14827](https://github.com/netdata/netdata/pull/14827) ([Ancairon](https://github.com/Ancairon))
- Delete installation instructions specific to FreeNAS [\#14826](https://github.com/netdata/netdata/pull/14826) ([Ancairon](https://github.com/Ancairon))
- First batch of adding descriptions to the documentation [\#14825](https://github.com/netdata/netdata/pull/14825) ([Ancairon](https://github.com/Ancairon))
- Fix Btrfs unallocated space accounting [\#14824](https://github.com/netdata/netdata/pull/14824) ([intelfx](https://github.com/intelfx))
- Update the bundled version of makeself used to create static builds. [\#14822](https://github.com/netdata/netdata/pull/14822) ([Ferroin](https://github.com/Ferroin))
- configure extent cache size [\#14821](https://github.com/netdata/netdata/pull/14821) ([ktsaou](https://github.com/ktsaou))
- Docs, shorten too long titles, and add a description below [\#14820](https://github.com/netdata/netdata/pull/14820) ([Ancairon](https://github.com/Ancairon))
- update posthog domain [\#14818](https://github.com/netdata/netdata/pull/14818) ([andrewm4894](https://github.com/andrewm4894))
- minor - add capability signifying this agent can speak apiv2 [\#14817](https://github.com/netdata/netdata/pull/14817) ([underhood](https://github.com/underhood))
- Minor improvements to netdata-security.md [\#14815](https://github.com/netdata/netdata/pull/14815) ([cakrit](https://github.com/cakrit))
- Update privacy link in aclk doc [\#14813](https://github.com/netdata/netdata/pull/14813) ([cakrit](https://github.com/cakrit))
- Consolidate security and privacy documents [\#14812](https://github.com/netdata/netdata/pull/14812) ([cakrit](https://github.com/cakrit))
- Update role-based-access.md [\#14811](https://github.com/netdata/netdata/pull/14811) ([cakrit](https://github.com/cakrit))
- Save and load ML models [\#14810](https://github.com/netdata/netdata/pull/14810) ([vkalintiris](https://github.com/vkalintiris))
- diskspace: don't collect inodes on msdosfs [\#14809](https://github.com/netdata/netdata/pull/14809) ([ilyam8](https://github.com/ilyam8))
- Document CetusGuard as a Docker socket proxy solution [\#14806](https://github.com/netdata/netdata/pull/14806) ([hectorm](https://github.com/hectorm))
- Address Learn feedback from users [\#14802](https://github.com/netdata/netdata/pull/14802) ([Ancairon](https://github.com/Ancairon))
- add validation step before using GCP metadata [\#14801](https://github.com/netdata/netdata/pull/14801) ([ilyam8](https://github.com/ilyam8))
- /api/v2/X part 7 [\#14797](https://github.com/netdata/netdata/pull/14797) ([ktsaou](https://github.com/ktsaou))
- Replace `/docs` links with GitHub links [\#14796](https://github.com/netdata/netdata/pull/14796) ([Ancairon](https://github.com/Ancairon))
- Fix links in README.md [\#14794](https://github.com/netdata/netdata/pull/14794) ([cakrit](https://github.com/cakrit))
- Fix capitalization on readme [\#14793](https://github.com/netdata/netdata/pull/14793) ([cakrit](https://github.com/cakrit))
- Fix handling of logrotate on static installs. [\#14792](https://github.com/netdata/netdata/pull/14792) ([Ferroin](https://github.com/Ferroin))
- use mCPU in k8s cgroup cpu charts title [\#14791](https://github.com/netdata/netdata/pull/14791) ([ilyam8](https://github.com/ilyam8))
- Schedule node info to the cloud after child connection [\#14790](https://github.com/netdata/netdata/pull/14790) ([stelfrag](https://github.com/stelfrag))
- uuid\_compare\(\) replaced with uuid\_memcmp\(\) [\#14787](https://github.com/netdata/netdata/pull/14787) ([ktsaou](https://github.com/ktsaou))
- /api/v2/X part 6 [\#14785](https://github.com/netdata/netdata/pull/14785) ([ktsaou](https://github.com/ktsaou))
- Update dashboard to version v2.30.1. [\#14784](https://github.com/netdata/netdata/pull/14784) ([netdatabot](https://github.com/netdatabot))
- Revert "Use static thread-pool for training. \(\#14702\)" [\#14782](https://github.com/netdata/netdata/pull/14782) ([vkalintiris](https://github.com/vkalintiris))
- Fix how we are handling system services in RPM packages. [\#14781](https://github.com/netdata/netdata/pull/14781) ([Ferroin](https://github.com/Ferroin))
- Replace hardcoded links pointing to "learn.netdata.cloud" with github absolute links [\#14779](https://github.com/netdata/netdata/pull/14779) ([Ancairon](https://github.com/Ancairon))
- Improve performance.md [\#14778](https://github.com/netdata/netdata/pull/14778) ([cakrit](https://github.com/cakrit))
- Update reverse-proxies.md [\#14777](https://github.com/netdata/netdata/pull/14777) ([cakrit](https://github.com/cakrit))
- Update performance.md [\#14776](https://github.com/netdata/netdata/pull/14776) ([cakrit](https://github.com/cakrit))
- add validation step before using Azure metadata \(AZURE\_IMDS\_DATA\)  [\#14775](https://github.com/netdata/netdata/pull/14775) ([ilyam8](https://github.com/ilyam8))
- Create reverse-proxies.md [\#14774](https://github.com/netdata/netdata/pull/14774) ([cakrit](https://github.com/cakrit))
- Add gzip compression info to nginx proxy readme [\#14773](https://github.com/netdata/netdata/pull/14773) ([cakrit](https://github.com/cakrit))
- Update API [\#14772](https://github.com/netdata/netdata/pull/14772) ([cakrit](https://github.com/cakrit))
- Add Amazon Linux 2023 to CI, packaging, and platform support. [\#14771](https://github.com/netdata/netdata/pull/14771) ([Ferroin](https://github.com/Ferroin))
- Pass node\_id and config\_hash vars when queueing alert configurations [\#14769](https://github.com/netdata/netdata/pull/14769) ([MrZammler](https://github.com/MrZammler))
- Assorted improvements for our platform EOL check code. [\#14768](https://github.com/netdata/netdata/pull/14768) ([Ferroin](https://github.com/Ferroin))
- minor addition to distros matrix [\#14767](https://github.com/netdata/netdata/pull/14767) ([tkatsoulas](https://github.com/tkatsoulas))
- Update "View active alerts" documentation [\#14766](https://github.com/netdata/netdata/pull/14766) ([Ancairon](https://github.com/Ancairon))
- Skip alert template variables from alert snapshots [\#14763](https://github.com/netdata/netdata/pull/14763) ([MrZammler](https://github.com/MrZammler))
- Accept all=true for alarms api v1 call [\#14762](https://github.com/netdata/netdata/pull/14762) ([MrZammler](https://github.com/MrZammler))
- fix /sys/block/zram in docker [\#14759](https://github.com/netdata/netdata/pull/14759) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.51.4 [\#14756](https://github.com/netdata/netdata/pull/14756) ([ilyam8](https://github.com/ilyam8))
- Add ethtool in third party collectors [\#14753](https://github.com/netdata/netdata/pull/14753) ([ghanapunq](https://github.com/ghanapunq))
- Update sign-in.md [\#14751](https://github.com/netdata/netdata/pull/14751) ([cakrit](https://github.com/cakrit))
- Update journal v2 [\#14750](https://github.com/netdata/netdata/pull/14750) ([stelfrag](https://github.com/stelfrag))
- Update edit-config documentation [\#14749](https://github.com/netdata/netdata/pull/14749) ([cakrit](https://github.com/cakrit))
- bump go.d.plugin version to v0.51.3 [\#14745](https://github.com/netdata/netdata/pull/14745) ([ilyam8](https://github.com/ilyam8))
- increase RRD\_ID\_LENGTH\_MAX to 1000 [\#14744](https://github.com/netdata/netdata/pull/14744) ([ilyam8](https://github.com/ilyam8))
- Update Performance Optimization Options [\#14743](https://github.com/netdata/netdata/pull/14743) ([cakrit](https://github.com/cakrit))
- Update change-metrics-storage.md [\#14742](https://github.com/netdata/netdata/pull/14742) ([cakrit](https://github.com/cakrit))
- Add contexts to privacy doc. [\#14741](https://github.com/netdata/netdata/pull/14741) ([cakrit](https://github.com/cakrit))
- Create pci-soc-hipaa.md [\#14740](https://github.com/netdata/netdata/pull/14740) ([cakrit](https://github.com/cakrit))
- Update data-privacy.md [\#14739](https://github.com/netdata/netdata/pull/14739) ([cakrit](https://github.com/cakrit))
- Update data-privacy.md [\#14738](https://github.com/netdata/netdata/pull/14738) ([cakrit](https://github.com/cakrit))
- pandas collector replace `self.warn()` with `self.warning()` [\#14736](https://github.com/netdata/netdata/pull/14736) ([andrewm4894](https://github.com/andrewm4894))
- Add CI support for Fedora 38 & Ubuntu 23.04 native packages [\#14735](https://github.com/netdata/netdata/pull/14735) ([tkatsoulas](https://github.com/tkatsoulas))
- Update change-metrics-storage.md [\#14734](https://github.com/netdata/netdata/pull/14734) ([cakrit](https://github.com/cakrit))
- Correct calc and explain how to get METRICS in RAM usage [\#14733](https://github.com/netdata/netdata/pull/14733) ([cakrit](https://github.com/cakrit))
- Update dashboard to version v2.30.0. [\#14732](https://github.com/netdata/netdata/pull/14732) ([netdatabot](https://github.com/netdatabot))
- remove ubuntu 18.04 from our CI [\#14731](https://github.com/netdata/netdata/pull/14731) ([tkatsoulas](https://github.com/tkatsoulas))
- Organize information from war-rooms.md to its correct location [\#14729](https://github.com/netdata/netdata/pull/14729) ([Ancairon](https://github.com/Ancairon))
- Update change-metrics-storage.md [\#14726](https://github.com/netdata/netdata/pull/14726) ([cakrit](https://github.com/cakrit))
- New build\_external scenario. [\#14725](https://github.com/netdata/netdata/pull/14725) ([vkalintiris](https://github.com/vkalintiris))
- Suggest PRs to go to the community project [\#14724](https://github.com/netdata/netdata/pull/14724) ([cakrit](https://github.com/cakrit))
- add go.d example collector to \#etc section [\#14722](https://github.com/netdata/netdata/pull/14722) ([andrewm4894](https://github.com/andrewm4894))
- /api/v2/X part 5 [\#14718](https://github.com/netdata/netdata/pull/14718) ([ktsaou](https://github.com/ktsaou))
- Update deployment-strategies.md [\#14716](https://github.com/netdata/netdata/pull/14716) ([cakrit](https://github.com/cakrit))
- Change H1 of collector docs to separate from the website [\#14715](https://github.com/netdata/netdata/pull/14715) ([cakrit](https://github.com/cakrit))
- Add instructions for reconnecting a Docker node to another Space [\#14714](https://github.com/netdata/netdata/pull/14714) ([Ancairon](https://github.com/Ancairon))
- fix system info disk size detection on raspberry pi  [\#14711](https://github.com/netdata/netdata/pull/14711) ([ilyam8](https://github.com/ilyam8))
- /api/v2 part 4 [\#14706](https://github.com/netdata/netdata/pull/14706) ([ktsaou](https://github.com/ktsaou))
- Don’t try to use tput in edit-config unless it’s installed. [\#14705](https://github.com/netdata/netdata/pull/14705) ([Ferroin](https://github.com/Ferroin))
- Bundle libyaml [\#14704](https://github.com/netdata/netdata/pull/14704) ([MrZammler](https://github.com/MrZammler))
- Handle conffiles for DEB packages explicitly instead of automatically. [\#14703](https://github.com/netdata/netdata/pull/14703) ([Ferroin](https://github.com/Ferroin))
- Use static thread-pool for training. [\#14702](https://github.com/netdata/netdata/pull/14702) ([vkalintiris](https://github.com/vkalintiris))
- Revert "Handle conffiles for DEB packages explicitly instead of automatically." [\#14700](https://github.com/netdata/netdata/pull/14700) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix compilation error when --disable-cloud is specified [\#14695](https://github.com/netdata/netdata/pull/14695) ([stelfrag](https://github.com/stelfrag))
- fix: detect the host os in k8s on non-docker cri [\#14694](https://github.com/netdata/netdata/pull/14694) ([witalisoft](https://github.com/witalisoft))
- Remove google hangouts from list of integrations [\#14689](https://github.com/netdata/netdata/pull/14689) ([cakrit](https://github.com/cakrit))
- Fix Azure IMDS [\#14686](https://github.com/netdata/netdata/pull/14686) ([shyamvalsan](https://github.com/shyamvalsan))
- Send an EOF from charts.d.plugin before exit [\#14680](https://github.com/netdata/netdata/pull/14680) ([MrZammler](https://github.com/MrZammler))
- Fix conditionals for claim-only case in kickstart.sh. [\#14679](https://github.com/netdata/netdata/pull/14679) ([Ferroin](https://github.com/Ferroin))
- Improve guideline docs [\#14678](https://github.com/netdata/netdata/pull/14678) ([Ancairon](https://github.com/Ancairon))
- Fix kernel test script [\#14676](https://github.com/netdata/netdata/pull/14676) ([thiagoftsm](https://github.com/thiagoftsm))
- add note on readme on how to easily see all ml related blog posts [\#14675](https://github.com/netdata/netdata/pull/14675) ([andrewm4894](https://github.com/andrewm4894))
- Guard for null host when sending node instances [\#14673](https://github.com/netdata/netdata/pull/14673) ([MrZammler](https://github.com/MrZammler))
- reviewed role description to be according to app [\#14672](https://github.com/netdata/netdata/pull/14672) ([hugovalente-pm](https://github.com/hugovalente-pm))
- If a child is not streaming, send to the cloud last known version instead of unknown [\#14671](https://github.com/netdata/netdata/pull/14671) ([MrZammler](https://github.com/MrZammler))
- /api/v2/X improvements part 3 [\#14665](https://github.com/netdata/netdata/pull/14665) ([ktsaou](https://github.com/ktsaou))
- add FAQ information provided by Finance [\#14664](https://github.com/netdata/netdata/pull/14664) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Handle conffiles for DEB packages explicitly instead of automatically. [\#14662](https://github.com/netdata/netdata/pull/14662) ([Ferroin](https://github.com/Ferroin))
- Fix cloud node stale status when a virtual host is created [\#14660](https://github.com/netdata/netdata/pull/14660) ([stelfrag](https://github.com/stelfrag))
- Refactor ML code. [\#14659](https://github.com/netdata/netdata/pull/14659) ([vkalintiris](https://github.com/vkalintiris))
- Properly handle service type detection failures when installing as a system service. [\#14658](https://github.com/netdata/netdata/pull/14658) ([Ferroin](https://github.com/Ferroin))
- fix simple\_pattern\_create on freebsd [\#14656](https://github.com/netdata/netdata/pull/14656) ([ilyam8](https://github.com/ilyam8))
- Move images in "interact-new-charts" from zenhub to github [\#14654](https://github.com/netdata/netdata/pull/14654) ([Ancairon](https://github.com/Ancairon))
- Fix broken links in glossary.md [\#14653](https://github.com/netdata/netdata/pull/14653) ([Ancairon](https://github.com/Ancairon))
- Fix links [\#14651](https://github.com/netdata/netdata/pull/14651) ([cakrit](https://github.com/cakrit))
- Fix doc links [\#14650](https://github.com/netdata/netdata/pull/14650) ([cakrit](https://github.com/cakrit))
- Update guidelines.md [\#14649](https://github.com/netdata/netdata/pull/14649) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14647](https://github.com/netdata/netdata/pull/14647) ([cakrit](https://github.com/cakrit))
- Update pi-hole-raspberry-pi.md [\#14644](https://github.com/netdata/netdata/pull/14644) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix handling of missing release codename on DEB systems. [\#14642](https://github.com/netdata/netdata/pull/14642) ([Ferroin](https://github.com/Ferroin))
- Update change-metrics-storage.md [\#14641](https://github.com/netdata/netdata/pull/14641) ([cakrit](https://github.com/cakrit))
- Update change-metrics-storage.md [\#14640](https://github.com/netdata/netdata/pull/14640) ([cakrit](https://github.com/cakrit))
- Collect additional BTRFS metrics [\#14636](https://github.com/netdata/netdata/pull/14636) ([Dim-P](https://github.com/Dim-P))
- Fix broken links [\#14634](https://github.com/netdata/netdata/pull/14634) ([Ancairon](https://github.com/Ancairon))
- Add link to native packages also on the list [\#14633](https://github.com/netdata/netdata/pull/14633) ([cakrit](https://github.com/cakrit))
- Assorted installer code cleanup. [\#14632](https://github.com/netdata/netdata/pull/14632) ([Ferroin](https://github.com/Ferroin))
- Re-add link from install page to DEB/RPM package documentation. [\#14631](https://github.com/netdata/netdata/pull/14631) ([Ferroin](https://github.com/Ferroin))
- Fix broken link [\#14630](https://github.com/netdata/netdata/pull/14630) ([cakrit](https://github.com/cakrit))
- Fix intermittent permissions issues in some Docker builds. [\#14629](https://github.com/netdata/netdata/pull/14629) ([Ferroin](https://github.com/Ferroin))
- Update REFERENCE.md [\#14627](https://github.com/netdata/netdata/pull/14627) ([cakrit](https://github.com/cakrit))
- Make the title metadata H1 in all markdown files [\#14625](https://github.com/netdata/netdata/pull/14625) ([Ancairon](https://github.com/Ancairon))
- eBPF new charts \(user ring\) [\#14623](https://github.com/netdata/netdata/pull/14623) ([thiagoftsm](https://github.com/thiagoftsm))
- rename glossary [\#14622](https://github.com/netdata/netdata/pull/14622) ([cakrit](https://github.com/cakrit))
- Reorg learn 0227 [\#14621](https://github.com/netdata/netdata/pull/14621) ([cakrit](https://github.com/cakrit))
- Assorted improvements to OpenRC support. [\#14620](https://github.com/netdata/netdata/pull/14620) ([Ferroin](https://github.com/Ferroin))
- bump go.d.plugin v0.51.2 [\#14618](https://github.com/netdata/netdata/pull/14618) ([ilyam8](https://github.com/ilyam8))
- fix python version check to work for 3.10 and above [\#14616](https://github.com/netdata/netdata/pull/14616) ([andrewm4894](https://github.com/andrewm4894))
- fix relative link to anonymous statistics [\#14614](https://github.com/netdata/netdata/pull/14614) ([cakrit](https://github.com/cakrit))
- fix proxy links in netdata security [\#14613](https://github.com/netdata/netdata/pull/14613) ([cakrit](https://github.com/cakrit))
- fix links from removed docs [\#14612](https://github.com/netdata/netdata/pull/14612) ([cakrit](https://github.com/cakrit))
- update go.d.plugin v0.51.1 [\#14611](https://github.com/netdata/netdata/pull/14611) ([ilyam8](https://github.com/ilyam8))
- Reorg learn 0226 [\#14610](https://github.com/netdata/netdata/pull/14610) ([cakrit](https://github.com/cakrit))
- Fix links to chart interactions [\#14609](https://github.com/netdata/netdata/pull/14609) ([cakrit](https://github.com/cakrit))
- Reorg information and add titles [\#14608](https://github.com/netdata/netdata/pull/14608) ([cakrit](https://github.com/cakrit))
- Update overview.md [\#14607](https://github.com/netdata/netdata/pull/14607) ([cakrit](https://github.com/cakrit))
- Fix broken links [\#14605](https://github.com/netdata/netdata/pull/14605) ([Ancairon](https://github.com/Ancairon))
- Misc SSL improvements 3 [\#14602](https://github.com/netdata/netdata/pull/14602) ([MrZammler](https://github.com/MrZammler))
- Update deployment-strategies.md [\#14601](https://github.com/netdata/netdata/pull/14601) ([cakrit](https://github.com/cakrit))
- Add deployment strategies [\#14600](https://github.com/netdata/netdata/pull/14600) ([cakrit](https://github.com/cakrit))
- Add Amazon Linux 2 to CI and platform support. [\#14599](https://github.com/netdata/netdata/pull/14599) ([Ferroin](https://github.com/Ferroin))
- Replace web server readme with its improved replica [\#14598](https://github.com/netdata/netdata/pull/14598) ([cakrit](https://github.com/cakrit))
- Update interact-new-charts.md [\#14596](https://github.com/netdata/netdata/pull/14596) ([cakrit](https://github.com/cakrit))
- Fix context unittest coredump [\#14595](https://github.com/netdata/netdata/pull/14595) ([stelfrag](https://github.com/stelfrag))
- Delete interact-dashboard-charts [\#14594](https://github.com/netdata/netdata/pull/14594) ([cakrit](https://github.com/cakrit))
- /api/v2/contexts [\#14592](https://github.com/netdata/netdata/pull/14592) ([ktsaou](https://github.com/ktsaou))
- Use vector allocation whenever is possible \(eBPF\) [\#14591](https://github.com/netdata/netdata/pull/14591) ([thiagoftsm](https://github.com/thiagoftsm))
- Change link text to collectors.md [\#14590](https://github.com/netdata/netdata/pull/14590) ([cakrit](https://github.com/cakrit))
- Add an option to the kickstart script to override distro detection. [\#14589](https://github.com/netdata/netdata/pull/14589) ([Ferroin](https://github.com/Ferroin))
- Merge security documents [\#14588](https://github.com/netdata/netdata/pull/14588) ([cakrit](https://github.com/cakrit))
- Prevent core dump when the agent is performing a quick shutdown [\#14587](https://github.com/netdata/netdata/pull/14587) ([stelfrag](https://github.com/stelfrag))
- Clean host structure [\#14584](https://github.com/netdata/netdata/pull/14584) ([stelfrag](https://github.com/stelfrag))
- Correct the sidebar position label metdata for learn [\#14583](https://github.com/netdata/netdata/pull/14583) ([cakrit](https://github.com/cakrit))
- final install reorg for learn [\#14580](https://github.com/netdata/netdata/pull/14580) ([cakrit](https://github.com/cakrit))
- Learn installation reorg part 2 [\#14579](https://github.com/netdata/netdata/pull/14579) ([cakrit](https://github.com/cakrit))
- Add link to all installation options [\#14578](https://github.com/netdata/netdata/pull/14578) ([cakrit](https://github.com/cakrit))
- Reorg learn 2102 1 [\#14577](https://github.com/netdata/netdata/pull/14577) ([cakrit](https://github.com/cakrit))
- Update README.md [\#14576](https://github.com/netdata/netdata/pull/14576) ([cakrit](https://github.com/cakrit))
- Reorg learn [\#14575](https://github.com/netdata/netdata/pull/14575) ([cakrit](https://github.com/cakrit))
- Update static binary readme [\#14574](https://github.com/netdata/netdata/pull/14574) ([cakrit](https://github.com/cakrit))
- Get update every from page [\#14573](https://github.com/netdata/netdata/pull/14573) ([stelfrag](https://github.com/stelfrag))
- bump go.d to v0.51.0 [\#14572](https://github.com/netdata/netdata/pull/14572) ([ilyam8](https://github.com/ilyam8))
- Remove References category from learn [\#14571](https://github.com/netdata/netdata/pull/14571) ([cakrit](https://github.com/cakrit))
- Fix doc capitalization and remove obsolete section [\#14569](https://github.com/netdata/netdata/pull/14569) ([cakrit](https://github.com/cakrit))
- Remove obsolete instruction to lower memory usage [\#14568](https://github.com/netdata/netdata/pull/14568) ([cakrit](https://github.com/cakrit))
- Port ML from C++ to C. [\#14567](https://github.com/netdata/netdata/pull/14567) ([vkalintiris](https://github.com/vkalintiris))
- Fix broken links in our documentation [\#14565](https://github.com/netdata/netdata/pull/14565) ([Ancairon](https://github.com/Ancairon))
- /api/v2/data - multi-host/context/instance/dimension/label queries [\#14564](https://github.com/netdata/netdata/pull/14564) ([ktsaou](https://github.com/ktsaou))
- pandas collector add `read_sql()` support [\#14563](https://github.com/netdata/netdata/pull/14563) ([andrewm4894](https://github.com/andrewm4894))
- reviewed plans page to be according to latest updates [\#14560](https://github.com/netdata/netdata/pull/14560) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Fix kickstart link [\#14559](https://github.com/netdata/netdata/pull/14559) ([cakrit](https://github.com/cakrit))
- Make secure nodes a category landing page [\#14558](https://github.com/netdata/netdata/pull/14558) ([cakrit](https://github.com/cakrit))
- Add misc landing page and move proxy guides [\#14557](https://github.com/netdata/netdata/pull/14557) ([cakrit](https://github.com/cakrit))
- Reorg learn 021723 [\#14556](https://github.com/netdata/netdata/pull/14556) ([cakrit](https://github.com/cakrit))
- Update email notification docs with info about setup in Docker. [\#14555](https://github.com/netdata/netdata/pull/14555) ([Ferroin](https://github.com/Ferroin))
- Fix broken links in integrations-overview [\#14554](https://github.com/netdata/netdata/pull/14554) ([Ancairon](https://github.com/Ancairon))
- Add vnodes default configuration file. [\#14553](https://github.com/netdata/netdata/pull/14553) ([Ferroin](https://github.com/Ferroin))
- RPM: Added elfutils-libelf-devel for build with eBPF [\#14552](https://github.com/netdata/netdata/pull/14552) ([k0ste](https://github.com/k0ste))
- More reorg learn 021623 [\#14550](https://github.com/netdata/netdata/pull/14550) ([cakrit](https://github.com/cakrit))
- Update learn path of python plugin readme [\#14549](https://github.com/netdata/netdata/pull/14549) ([cakrit](https://github.com/cakrit))
- Hide netdata for IoT from learn. [\#14548](https://github.com/netdata/netdata/pull/14548) ([cakrit](https://github.com/cakrit))
- Reorg markdown files for learn [\#14547](https://github.com/netdata/netdata/pull/14547) ([cakrit](https://github.com/cakrit))
- Fix two issues with the edit-config script. [\#14545](https://github.com/netdata/netdata/pull/14545) ([Ferroin](https://github.com/Ferroin))
- Reorganize system directory to better reflect what files are actually used for. [\#14544](https://github.com/netdata/netdata/pull/14544) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues [\#14543](https://github.com/netdata/netdata/pull/14543) ([stelfrag](https://github.com/stelfrag))
- Remove unused config options and functions [\#14542](https://github.com/netdata/netdata/pull/14542) ([stelfrag](https://github.com/stelfrag))
- Add renamed markdown files [\#14540](https://github.com/netdata/netdata/pull/14540) ([cakrit](https://github.com/cakrit))
- Fix broken svgs and improve database queries API doc [\#14539](https://github.com/netdata/netdata/pull/14539) ([cakrit](https://github.com/cakrit))
- Reorganize learn documents under Integrations part 2 [\#14538](https://github.com/netdata/netdata/pull/14538) ([cakrit](https://github.com/cakrit))
- Roles docs: Add Early Bird and Member role [\#14537](https://github.com/netdata/netdata/pull/14537) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Fix broken Alma Linux entries in build matrix generation. [\#14536](https://github.com/netdata/netdata/pull/14536) ([Ferroin](https://github.com/Ferroin))
- Re-index when machine guid changes [\#14535](https://github.com/netdata/netdata/pull/14535) ([MrZammler](https://github.com/MrZammler))
- Use BoxListItemRegexLink component in docs/quickstart/insfrastructure.md [\#14533](https://github.com/netdata/netdata/pull/14533) ([Ancairon](https://github.com/Ancairon))
- Update main metric retention docs [\#14530](https://github.com/netdata/netdata/pull/14530) ([cakrit](https://github.com/cakrit))
- Add Debian 12 to our CI and platform support document. [\#14529](https://github.com/netdata/netdata/pull/14529) ([Ferroin](https://github.com/Ferroin))
- Update role-based-access.md [\#14528](https://github.com/netdata/netdata/pull/14528) ([cakrit](https://github.com/cakrit))
- added section to explain impacts on member role [\#14527](https://github.com/netdata/netdata/pull/14527) ([hugovalente-pm](https://github.com/hugovalente-pm))
- fix setting go.d.plugin capabilities [\#14525](https://github.com/netdata/netdata/pull/14525) ([ilyam8](https://github.com/ilyam8))
- Simplify parser README.md and add parser files to CMakeLists.txt [\#14523](https://github.com/netdata/netdata/pull/14523) ([stelfrag](https://github.com/stelfrag))
- Link statically libnetfilter\_acct into our static builds [\#14516](https://github.com/netdata/netdata/pull/14516) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix broken links in markdown files [\#14513](https://github.com/netdata/netdata/pull/14513) ([Ancairon](https://github.com/Ancairon))
- Make external plugins a category page in learn [\#14511](https://github.com/netdata/netdata/pull/14511) ([cakrit](https://github.com/cakrit))
- Learn integrations category changes [\#14510](https://github.com/netdata/netdata/pull/14510) ([cakrit](https://github.com/cakrit))
- Move collectors under Integrations/Monitoring [\#14509](https://github.com/netdata/netdata/pull/14509) ([cakrit](https://github.com/cakrit))
- Guides and collectors reorg and cleanup part 1 [\#14507](https://github.com/netdata/netdata/pull/14507) ([cakrit](https://github.com/cakrit))
- replicating gaps [\#14506](https://github.com/netdata/netdata/pull/14506) ([ktsaou](https://github.com/ktsaou))
- More learn reorg/reordering [\#14505](https://github.com/netdata/netdata/pull/14505) ([cakrit](https://github.com/cakrit))
- Revert changes to platform support policy [\#14504](https://github.com/netdata/netdata/pull/14504) ([cakrit](https://github.com/cakrit))
- Top level learn changes [\#14503](https://github.com/netdata/netdata/pull/14503) ([cakrit](https://github.com/cakrit))
- Fix broken links in collectors/COLLECTORS.md [\#14502](https://github.com/netdata/netdata/pull/14502) ([Ancairon](https://github.com/Ancairon))
- Update Demo-Sites.md [\#14501](https://github.com/netdata/netdata/pull/14501) ([cakrit](https://github.com/cakrit))
- Member role on roles permissions docs [\#14500](https://github.com/netdata/netdata/pull/14500) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Reorganize contents of Getting Started [\#14499](https://github.com/netdata/netdata/pull/14499) ([cakrit](https://github.com/cakrit))
- Correct title of contribute to doccumentation [\#14498](https://github.com/netdata/netdata/pull/14498) ([cakrit](https://github.com/cakrit))
- Delete getting-started-overview.md [\#14497](https://github.com/netdata/netdata/pull/14497) ([Ancairon](https://github.com/Ancairon))
- added Challenge secret and rooms object on the payload [\#14496](https://github.com/netdata/netdata/pull/14496) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Category overview pages [\#14495](https://github.com/netdata/netdata/pull/14495) ([Ancairon](https://github.com/Ancairon))
- JSON internal API, IEEE754 base64/hex streaming, weights endpoint optimization [\#14493](https://github.com/netdata/netdata/pull/14493) ([ktsaou](https://github.com/ktsaou))
- Fix crash when child connects [\#14492](https://github.com/netdata/netdata/pull/14492) ([stelfrag](https://github.com/stelfrag))
- Plans docs [\#14491](https://github.com/netdata/netdata/pull/14491) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Try making it landing page of getting started directly [\#14489](https://github.com/netdata/netdata/pull/14489) ([cakrit](https://github.com/cakrit))
- Update Demo-Sites.md [\#14488](https://github.com/netdata/netdata/pull/14488) ([Ancairon](https://github.com/Ancairon))
- Make the introduction a category link [\#14485](https://github.com/netdata/netdata/pull/14485) ([Ancairon](https://github.com/Ancairon))
- Update AD title [\#14484](https://github.com/netdata/netdata/pull/14484) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix coverity issues [\#14480](https://github.com/netdata/netdata/pull/14480) ([stelfrag](https://github.com/stelfrag))
- Remove obsolete or redundant docs [\#14476](https://github.com/netdata/netdata/pull/14476) ([cakrit](https://github.com/cakrit))
- Incorporate interoperability and fix edit link [\#14475](https://github.com/netdata/netdata/pull/14475) ([cakrit](https://github.com/cakrit))
- Upgrade demo sites to the getting started section [\#14474](https://github.com/netdata/netdata/pull/14474) ([cakrit](https://github.com/cakrit))
- Add a file to Learn [\#14473](https://github.com/netdata/netdata/pull/14473) ([Ancairon](https://github.com/Ancairon))
- fix a possible bug with an image in the md file [\#14472](https://github.com/netdata/netdata/pull/14472) ([Ancairon](https://github.com/Ancairon))
- Add sbindir\_POST template for v235 service file [\#14471](https://github.com/netdata/netdata/pull/14471) ([MrZammler](https://github.com/MrZammler))
- Fix random crash on agent shutdown [\#14470](https://github.com/netdata/netdata/pull/14470) ([stelfrag](https://github.com/stelfrag))
- Move ansible md [\#14469](https://github.com/netdata/netdata/pull/14469) ([cakrit](https://github.com/cakrit))
- Correct link to ansible playbook [\#14468](https://github.com/netdata/netdata/pull/14468) ([cakrit](https://github.com/cakrit))
- Moved contents of get started to installer readme [\#14467](https://github.com/netdata/netdata/pull/14467) ([cakrit](https://github.com/cakrit))
- Add markdown files in Learn [\#14466](https://github.com/netdata/netdata/pull/14466) ([Ancairon](https://github.com/Ancairon))
- Virtual hosts for data collection [\#14464](https://github.com/netdata/netdata/pull/14464) ([ktsaou](https://github.com/ktsaou))
- Memory management eBPF [\#14462](https://github.com/netdata/netdata/pull/14462) ([thiagoftsm](https://github.com/thiagoftsm))
- Add contents of packaging/installer/readme.md [\#14461](https://github.com/netdata/netdata/pull/14461) ([cakrit](https://github.com/cakrit))
- Add mention of cloud in next steps UI etc [\#14459](https://github.com/netdata/netdata/pull/14459) ([cakrit](https://github.com/cakrit))
- Fix links and add to learn [\#14458](https://github.com/netdata/netdata/pull/14458) ([cakrit](https://github.com/cakrit))
- Add export for people running their own registry [\#14457](https://github.com/netdata/netdata/pull/14457) ([cakrit](https://github.com/cakrit))
- Support installing extra packages in Docker images at runtime. [\#14456](https://github.com/netdata/netdata/pull/14456) ([Ferroin](https://github.com/Ferroin))
- Prevent crash when running '-W createdataset' [\#14455](https://github.com/netdata/netdata/pull/14455) ([MrZammler](https://github.com/MrZammler))
- remove deprecated python.d collectors announced in v1.38.0 [\#14454](https://github.com/netdata/netdata/pull/14454) ([ilyam8](https://github.com/ilyam8))
- Update static build dependencies [\#14450](https://github.com/netdata/netdata/pull/14450) ([tkatsoulas](https://github.com/tkatsoulas))

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
