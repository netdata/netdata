# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.1...HEAD)

**Merged pull requests:**

- Fixed a single typo in documentation [\#11082](https://github.com/netdata/netdata/pull/11082) ([yavin87](https://github.com/yavin87))
- Change eBPF chart type [\#11074](https://github.com/netdata/netdata/pull/11074) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix coverity issue \(CID 370510\) [\#11060](https://github.com/netdata/netdata/pull/11060) ([stelfrag](https://github.com/stelfrag))
- Add functionality to store node\_id for a host [\#11059](https://github.com/netdata/netdata/pull/11059) ([stelfrag](https://github.com/stelfrag))
- Add `charts` to templates [\#11054](https://github.com/netdata/netdata/pull/11054) ([thiagoftsm](https://github.com/thiagoftsm))
- Add documentation for claiming during kickstart installation [\#11052](https://github.com/netdata/netdata/pull/11052) ([joelhans](https://github.com/joelhans))
- Remove dots in cgroup ids [\#11050](https://github.com/netdata/netdata/pull/11050) ([vlvkobal](https://github.com/vlvkobal))
- health/vernemq: use `average` instead of  `sum` [\#11037](https://github.com/netdata/netdata/pull/11037) ([ilyam8](https://github.com/ilyam8))
- Fix storing an NULL claim id on a parent node [\#11036](https://github.com/netdata/netdata/pull/11036) ([stelfrag](https://github.com/stelfrag))
- Load names [\#11034](https://github.com/netdata/netdata/pull/11034) ([thiagoftsm](https://github.com/thiagoftsm))
- Add Third-party collector: nextcloud plugin [\#11032](https://github.com/netdata/netdata/pull/11032) ([tknobi](https://github.com/tknobi))
- Create ebpf.d directory in PLUGINDIR for debian and rpm package\(netdata\#11017\) [\#11031](https://github.com/netdata/netdata/pull/11031) ([wangpei-nice](https://github.com/wangpei-nice))
- proc/mdstat: add raid level to the family [\#11024](https://github.com/netdata/netdata/pull/11024) ([ilyam8](https://github.com/ilyam8))
- bump to netdata-pandas==0.0.38 [\#11022](https://github.com/netdata/netdata/pull/11022) ([andrewm4894](https://github.com/andrewm4894))
- Provide more agent analytics to posthog [\#11020](https://github.com/netdata/netdata/pull/11020) ([MrZammler](https://github.com/MrZammler))
- Rename struct fields from class to classification. [\#11019](https://github.com/netdata/netdata/pull/11019) ([vkalintiris](https://github.com/vkalintiris))
- Improve dashboard documentation \(part 1\) [\#11015](https://github.com/netdata/netdata/pull/11015) ([joelhans](https://github.com/joelhans))
- Remove links to old install doc [\#11014](https://github.com/netdata/netdata/pull/11014) ([joelhans](https://github.com/joelhans))
- Revert "Provide more agent analytics to posthog" [\#11011](https://github.com/netdata/netdata/pull/11011) ([MrZammler](https://github.com/MrZammler))
- anonymous-statistics: add a timeout when using `curl` [\#11010](https://github.com/netdata/netdata/pull/11010) ([ilyam8](https://github.com/ilyam8))
- python.d: add plugin and module names to the runtime charts [\#11007](https://github.com/netdata/netdata/pull/11007) ([ilyam8](https://github.com/ilyam8))
- \[area/collectors\] Added support for libvirtd LXC containers to the `cgroup-name.sh` cgroup name normalization script [\#11006](https://github.com/netdata/netdata/pull/11006) ([endreszabo](https://github.com/endreszabo))
- Allow the remote write configuration have multiple destinations [\#11005](https://github.com/netdata/netdata/pull/11005) ([vlvkobal](https://github.com/vlvkobal))
- improvements to anomalies collector following dogfooding [\#11003](https://github.com/netdata/netdata/pull/11003) ([andrewm4894](https://github.com/andrewm4894))
- Backend chart filtering backward compatibility fix [\#11002](https://github.com/netdata/netdata/pull/11002) ([vlvkobal](https://github.com/vlvkobal))
- Add a chart with netdata uptime [\#10997](https://github.com/netdata/netdata/pull/10997) ([vlvkobal](https://github.com/vlvkobal))
- Improve get started/installation docs [\#10995](https://github.com/netdata/netdata/pull/10995) ([joelhans](https://github.com/joelhans))
- Persist claim ids in local database for parent and children [\#10993](https://github.com/netdata/netdata/pull/10993) ([stelfrag](https://github.com/stelfrag))
- ci: fix aws-kinesis builds [\#10992](https://github.com/netdata/netdata/pull/10992) ([ilyam8](https://github.com/ilyam8))
- Move global stats to a separate thread [\#10991](https://github.com/netdata/netdata/pull/10991) ([vlvkobal](https://github.com/vlvkobal))
- adds missing SPDX license info into ACLK-NG [\#10990](https://github.com/netdata/netdata/pull/10990) ([underhood](https://github.com/underhood))
- K6 quality of life updates [\#10985](https://github.com/netdata/netdata/pull/10985) ([OdysLam](https://github.com/OdysLam))
- Update eBPF documentation [\#10982](https://github.com/netdata/netdata/pull/10982) ([thiagoftsm](https://github.com/thiagoftsm))
- remove vneg from ACLK-NG [\#10980](https://github.com/netdata/netdata/pull/10980) ([underhood](https://github.com/underhood))
- Remove outdated privacy policy and terms of use [\#10979](https://github.com/netdata/netdata/pull/10979) ([joelhans](https://github.com/joelhans))
- collectors/charts.d/opensips: fix detection of `opensipsctl` executable  [\#10978](https://github.com/netdata/netdata/pull/10978) ([ilyam8](https://github.com/ilyam8))
- Update fping version [\#10977](https://github.com/netdata/netdata/pull/10977) ([Habetdin](https://github.com/Habetdin))
- fix uil in statsd guide [\#10975](https://github.com/netdata/netdata/pull/10975) ([OdysLam](https://github.com/OdysLam))
- health: fix alarm line options syntax in the docs [\#10974](https://github.com/netdata/netdata/pull/10974) ([ilyam8](https://github.com/ilyam8))
- Upgrade OKay repository RPM for RHEL8 [\#10973](https://github.com/netdata/netdata/pull/10973) ([BastienBalaud](https://github.com/BastienBalaud))
- Remove condition that was creating gaps [\#10972](https://github.com/netdata/netdata/pull/10972) ([thiagoftsm](https://github.com/thiagoftsm))
- Add a metric for percpu memory [\#10964](https://github.com/netdata/netdata/pull/10964) ([vlvkobal](https://github.com/vlvkobal))
- Bring flexible adjust for eBPF hash tables [\#10962](https://github.com/netdata/netdata/pull/10962) ([thiagoftsm](https://github.com/thiagoftsm))
- Provide new attributes in health conf files [\#10961](https://github.com/netdata/netdata/pull/10961) ([MrZammler](https://github.com/MrZammler))
- Fix epbf crash when process exit [\#10957](https://github.com/netdata/netdata/pull/10957) ([thiagoftsm](https://github.com/thiagoftsm))
- Contributing revamp, take 2 [\#10956](https://github.com/netdata/netdata/pull/10956) ([OdysLam](https://github.com/OdysLam))
- health: add Inconsistent state to the mysql\_galera\_cluster\_state alarm [\#10945](https://github.com/netdata/netdata/pull/10945) ([ilyam8](https://github.com/ilyam8))
- Update cloud-providers.md [\#10942](https://github.com/netdata/netdata/pull/10942) ([Avre](https://github.com/Avre))
- ACLK new cloud architecture new TBEB [\#10941](https://github.com/netdata/netdata/pull/10941) ([underhood](https://github.com/underhood))
- Add new charts for extended disk metrics [\#10939](https://github.com/netdata/netdata/pull/10939) ([vlvkobal](https://github.com/vlvkobal))
- Adds --recursive to docu git clones [\#10932](https://github.com/netdata/netdata/pull/10932) ([underhood](https://github.com/underhood))
- Add lists of monitored metrics to the cgroups plugin documentation [\#10924](https://github.com/netdata/netdata/pull/10924) ([vlvkobal](https://github.com/vlvkobal))
- Spelling web gui [\#10922](https://github.com/netdata/netdata/pull/10922) ([jsoref](https://github.com/jsoref))
- Spelling web api server [\#10921](https://github.com/netdata/netdata/pull/10921) ([jsoref](https://github.com/jsoref))
- Spelling tests [\#10920](https://github.com/netdata/netdata/pull/10920) ([jsoref](https://github.com/jsoref))
- Spelling streaming [\#10919](https://github.com/netdata/netdata/pull/10919) ([jsoref](https://github.com/jsoref))
- spelling: bidirectional [\#10918](https://github.com/netdata/netdata/pull/10918) ([jsoref](https://github.com/jsoref))
- Spelling libnetdata [\#10917](https://github.com/netdata/netdata/pull/10917) ([jsoref](https://github.com/jsoref))
- Spelling health [\#10916](https://github.com/netdata/netdata/pull/10916) ([jsoref](https://github.com/jsoref))
- Spelling exporting [\#10915](https://github.com/netdata/netdata/pull/10915) ([jsoref](https://github.com/jsoref))
- Spelling database [\#10914](https://github.com/netdata/netdata/pull/10914) ([jsoref](https://github.com/jsoref))
- Spelling daemon [\#10913](https://github.com/netdata/netdata/pull/10913) ([jsoref](https://github.com/jsoref))
- Spelling collectors [\#10912](https://github.com/netdata/netdata/pull/10912) ([jsoref](https://github.com/jsoref))
- spelling: backend [\#10911](https://github.com/netdata/netdata/pull/10911) ([jsoref](https://github.com/jsoref))
- Spelling aclk [\#10910](https://github.com/netdata/netdata/pull/10910) ([jsoref](https://github.com/jsoref))
- Spelling build [\#10909](https://github.com/netdata/netdata/pull/10909) ([jsoref](https://github.com/jsoref))
- health: add synchronization.conf to the Makefile [\#10907](https://github.com/netdata/netdata/pull/10907) ([ilyam8](https://github.com/ilyam8))
- health: add systemdunits alarms [\#10906](https://github.com/netdata/netdata/pull/10906) ([ilyam8](https://github.com/ilyam8))
- web/gui: add systemdunits info to the dashboard\_info.js [\#10904](https://github.com/netdata/netdata/pull/10904) ([ilyam8](https://github.com/ilyam8))
- Add a plugin for the system clock synchronization state [\#10895](https://github.com/netdata/netdata/pull/10895) ([vlvkobal](https://github.com/vlvkobal))
- Provide more agent analytics to posthog [\#10887](https://github.com/netdata/netdata/pull/10887) ([MrZammler](https://github.com/MrZammler))
- Add a chart for out of memory kills [\#10880](https://github.com/netdata/netdata/pull/10880) ([vlvkobal](https://github.com/vlvkobal))
- Remove RewriteEngine for dedicated vHost [\#10873](https://github.com/netdata/netdata/pull/10873) ([Steve8291](https://github.com/Steve8291))
- python.d\(smartd\_log\): collect attribute 249 -- NAND Writes 1GiB [\#10872](https://github.com/netdata/netdata/pull/10872) ([RaitoBezarius](https://github.com/RaitoBezarius))
- Improvements to dash-example.html [\#10870](https://github.com/netdata/netdata/pull/10870) ([tnyeanderson](https://github.com/tnyeanderson))
- Replace references to Google Analytics with Posthog where relevant [\#10868](https://github.com/netdata/netdata/pull/10868) ([andrewm4894](https://github.com/andrewm4894))
- ACLK Passwd endpoint update [\#10859](https://github.com/netdata/netdata/pull/10859) ([underhood](https://github.com/underhood))
- Dashboard version 2.17.0 [\#10856](https://github.com/netdata/netdata/pull/10856) ([allelos](https://github.com/allelos))
- Ebpf directory cache [\#10855](https://github.com/netdata/netdata/pull/10855) ([thiagoftsm](https://github.com/thiagoftsm))
- prevents mqtt connection attempt on OTP failure [\#10839](https://github.com/netdata/netdata/pull/10839) ([underhood](https://github.com/underhood))
- implements ACLK env endpoint [\#10833](https://github.com/netdata/netdata/pull/10833) ([underhood](https://github.com/underhood))
- implements new https client for ACLK [\#10805](https://github.com/netdata/netdata/pull/10805) ([underhood](https://github.com/underhood))
- Overhaul streaming documentation [\#10709](https://github.com/netdata/netdata/pull/10709) ([joelhans](https://github.com/joelhans))
- Zscores python collector [\#10673](https://github.com/netdata/netdata/pull/10673) ([andrewm4894](https://github.com/andrewm4894))
- add python changefinder collector [\#10672](https://github.com/netdata/netdata/pull/10672) ([andrewm4894](https://github.com/andrewm4894))
- Report porcelain output [\#10494](https://github.com/netdata/netdata/pull/10494) ([jsoref](https://github.com/jsoref))

## [v1.30.1](https://github.com/netdata/netdata/tree/v1.30.1) (2021-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.30.0...v1.30.1)

**Merged pull requests:**

- Don’t use glob expansion in argument to `cd` in updater. [\#10936](https://github.com/netdata/netdata/pull/10936) ([Ferroin](https://github.com/Ferroin))
- Fix memory corruption issue when executing context queries in RAM/SAVE memory mode [\#10933](https://github.com/netdata/netdata/pull/10933) ([stelfrag](https://github.com/stelfrag))
- Update CODEOWNERS [\#10928](https://github.com/netdata/netdata/pull/10928) ([knatsakis](https://github.com/knatsakis))
- Update news and GIF in README, fix typo [\#10900](https://github.com/netdata/netdata/pull/10900) ([joelhans](https://github.com/joelhans))
- Update README.md [\#10898](https://github.com/netdata/netdata/pull/10898) ([slimanio](https://github.com/slimanio))
- Fixed bundling of ACLK-NG components in dist tarballs. [\#10894](https://github.com/netdata/netdata/pull/10894) ([Ferroin](https://github.com/Ferroin))
- Add a CRASH event when the agent fails to properly shutdown [\#10893](https://github.com/netdata/netdata/pull/10893) ([stelfrag](https://github.com/stelfrag))
- Bumped version of OpenSSL bundled in static builds to 1.1.1k. [\#10884](https://github.com/netdata/netdata/pull/10884) ([Ferroin](https://github.com/Ferroin))
- Fix incorrect health log entries [\#10822](https://github.com/netdata/netdata/pull/10822) ([stelfrag](https://github.com/stelfrag))

## [v1.30.0](https://github.com/netdata/netdata/tree/v1.30.0) (2021-03-31)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.3...v1.30.0)

**Merged pull requests:**

- Properly handle different netcat command names in binary package test code. [\#10883](https://github.com/netdata/netdata/pull/10883) ([Ferroin](https://github.com/Ferroin))
- Add carrier and mtu charts for network interfaces [\#10866](https://github.com/netdata/netdata/pull/10866) ([vlvkobal](https://github.com/vlvkobal))
- Fix typo in main.h [\#10858](https://github.com/netdata/netdata/pull/10858) ([eltociear](https://github.com/eltociear))
- health: improve alarms infos [\#10853](https://github.com/netdata/netdata/pull/10853) ([ilyam8](https://github.com/ilyam8))
- minor - add info about --aclk-ng into netdata-installer [\#10852](https://github.com/netdata/netdata/pull/10852) ([underhood](https://github.com/underhood))
- mqtt-c coverity fix [\#10851](https://github.com/netdata/netdata/pull/10851) ([underhood](https://github.com/underhood))
- web/gui: make network state map sytanx consistent in the dashboard info [\#10849](https://github.com/netdata/netdata/pull/10849) ([ilyam8](https://github.com/ilyam8))
- fix\_repeat: Update repeat\_every and avoid unecessary test [\#10846](https://github.com/netdata/netdata/pull/10846) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix agent crash when executing data query with context and non-existing chart\_label\_key [\#10844](https://github.com/netdata/netdata/pull/10844) ([stelfrag](https://github.com/stelfrag))
- Check device names in diskstats plugin [\#10843](https://github.com/netdata/netdata/pull/10843) ([vlvkobal](https://github.com/vlvkobal))
- Fix memory leak when archived data is requested [\#10837](https://github.com/netdata/netdata/pull/10837) ([stelfrag](https://github.com/stelfrag))
- add Installation method to the bug template [\#10836](https://github.com/netdata/netdata/pull/10836) ([ilyam8](https://github.com/ilyam8))
- Add lock check to avoid shutdown when compiled with internal and locking checks [\#10835](https://github.com/netdata/netdata/pull/10835) ([stelfrag](https://github.com/stelfrag))
- health: apply megacli alarms for all adapters/physical disks [\#10834](https://github.com/netdata/netdata/pull/10834) ([ilyam8](https://github.com/ilyam8))
- Fix broken link in StatsD guide [\#10831](https://github.com/netdata/netdata/pull/10831) ([joelhans](https://github.com/joelhans))
- health: add collector prefix to the external collectors alarms/templates [\#10830](https://github.com/netdata/netdata/pull/10830) ([ilyam8](https://github.com/ilyam8))
- health: remove exporting\_metrics\_lost template [\#10829](https://github.com/netdata/netdata/pull/10829) ([ilyam8](https://github.com/ilyam8))
- Fix name of PackageCLoud API token secret in workflows. [\#10828](https://github.com/netdata/netdata/pull/10828) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.28.1 [\#10826](https://github.com/netdata/netdata/pull/10826) ([ilyam8](https://github.com/ilyam8))
- alarm\(irc\): add support to change IRC\_PORT [\#10824](https://github.com/netdata/netdata/pull/10824) ([RaitoBezarius](https://github.com/RaitoBezarius))
- Update syntax for Caddy v2 [\#10823](https://github.com/netdata/netdata/pull/10823) ([salazarp](https://github.com/salazarp))
- health: apply adapter\_raid alarms for every logical/physical device [\#10820](https://github.com/netdata/netdata/pull/10820) ([ilyam8](https://github.com/ilyam8))
- Fix handling of nightly and release packages in GHA workflows. [\#10819](https://github.com/netdata/netdata/pull/10819) ([Ferroin](https://github.com/Ferroin))
- health: log an error if any when send email notification [\#10818](https://github.com/netdata/netdata/pull/10818) ([ilyam8](https://github.com/ilyam8))
- Ebpf extend sync [\#10814](https://github.com/netdata/netdata/pull/10814) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix coverity issue \(CID 367566\) [\#10813](https://github.com/netdata/netdata/pull/10813) ([stelfrag](https://github.com/stelfrag))
- fix claiming via env vars in docker container [\#10811](https://github.com/netdata/netdata/pull/10811) ([ilyam8](https://github.com/ilyam8))
- Fix eBPF compilation [\#10810](https://github.com/netdata/netdata/pull/10810) ([thiagoftsm](https://github.com/thiagoftsm))
- update bug report template [\#10807](https://github.com/netdata/netdata/pull/10807) ([underhood](https://github.com/underhood))
- health: exclude cgroups net ifaces from packets dropped alarms [\#10806](https://github.com/netdata/netdata/pull/10806) ([ilyam8](https://github.com/ilyam8))
- Don't show alarms for charts without data [\#10804](https://github.com/netdata/netdata/pull/10804) ([vlvkobal](https://github.com/vlvkobal))
- claiming: increase curl connect-timeout and decrease number of claim attempts [\#10800](https://github.com/netdata/netdata/pull/10800) ([ilyam8](https://github.com/ilyam8))
- Added Ubuntu 21.04 and Fedora 34 to our CI checks and binary package builds. [\#10791](https://github.com/netdata/netdata/pull/10791) ([Ferroin](https://github.com/Ferroin))
- health: remove ram\_in\_swap alarm [\#10789](https://github.com/netdata/netdata/pull/10789) ([ilyam8](https://github.com/ilyam8))
- Add a new parameter 'chart' to the /api/v1/alarm\_log. [\#10788](https://github.com/netdata/netdata/pull/10788) ([MrZammler](https://github.com/MrZammler))
- Add check for children connecting to a parent agent with unsupported memory mode [\#10787](https://github.com/netdata/netdata/pull/10787) ([stelfrag](https://github.com/stelfrag))
- health: use separate packets\_dropped\_ratio alarms for wifi network interfaces [\#10785](https://github.com/netdata/netdata/pull/10785) ([ilyam8](https://github.com/ilyam8))
- ACLK separate https client [\#10784](https://github.com/netdata/netdata/pull/10784) ([underhood](https://github.com/underhood))
- health: add `wmi\_` prefix to the wmi collector network alarms [\#10782](https://github.com/netdata/netdata/pull/10782) ([ilyam8](https://github.com/ilyam8))
- web/gui: add max value to the nvidia\_smi.fan\_speed gauge [\#10780](https://github.com/netdata/netdata/pull/10780) ([ilyam8](https://github.com/ilyam8))
- health/: fix various alarms critical and warning thresholds hysteresis [\#10779](https://github.com/netdata/netdata/pull/10779) ([ilyam8](https://github.com/ilyam8))
- Adds \_aclk\_impl label [\#10778](https://github.com/netdata/netdata/pull/10778) ([underhood](https://github.com/underhood))
- adding a default job with some params and example of additional job. [\#10777](https://github.com/netdata/netdata/pull/10777) ([andrewm4894](https://github.com/andrewm4894))
- Fix typo in dashboard\_info.js [\#10775](https://github.com/netdata/netdata/pull/10775) ([eltociear](https://github.com/eltociear))
- Fixed Travis config issues related to new packaging workflows. [\#10774](https://github.com/netdata/netdata/pull/10774) ([Ferroin](https://github.com/Ferroin))
- add a dump\_methods parameter to alarm-notify.sh.in [\#10772](https://github.com/netdata/netdata/pull/10772) ([MrZammler](https://github.com/MrZammler))
- Add data query support for archived charts [\#10771](https://github.com/netdata/netdata/pull/10771) ([stelfrag](https://github.com/stelfrag))
- health: make vernemq alarms less sensitive [\#10770](https://github.com/netdata/netdata/pull/10770) ([ilyam8](https://github.com/ilyam8))
- Fixed handling of perf.plugin capabilities. [\#10766](https://github.com/netdata/netdata/pull/10766) ([Ferroin](https://github.com/Ferroin))
- dashboard@v2.13.28 [\#10761](https://github.com/netdata/netdata/pull/10761) ([jacekkolasa](https://github.com/jacekkolasa))
- collectors/cgroups: fix cpuset.cpus count [\#10757](https://github.com/netdata/netdata/pull/10757) ([ilyam8](https://github.com/ilyam8))
- eBPF plugin \(fixes 10727\) [\#10756](https://github.com/netdata/netdata/pull/10756) ([thiagoftsm](https://github.com/thiagoftsm))
- web/gui: add supervisord to the dashboard\_info.js [\#10754](https://github.com/netdata/netdata/pull/10754) ([ilyam8](https://github.com/ilyam8))
- Add state map to duplex and operstate charts [\#10752](https://github.com/netdata/netdata/pull/10752) ([vlvkobal](https://github.com/vlvkobal))
- comment out memory mode mention in example [\#10751](https://github.com/netdata/netdata/pull/10751) ([OdysLam](https://github.com/OdysLam))
- collectors/apps.plugin: Add wireguard to vpn [\#10743](https://github.com/netdata/netdata/pull/10743) ([liepumartins](https://github.com/liepumartins))
- Enable metadata persistence in all memory modes [\#10742](https://github.com/netdata/netdata/pull/10742) ([stelfrag](https://github.com/stelfrag))
- Move network interface speed, duplex, and operstate variables to charts [\#10740](https://github.com/netdata/netdata/pull/10740) ([vlvkobal](https://github.com/vlvkobal))
- Use of out-of-line struct definitions. [\#10739](https://github.com/netdata/netdata/pull/10739) ([vkalintiris](https://github.com/vkalintiris))
- Use a parameter name that is not a reserved keyword in C++ [\#10738](https://github.com/netdata/netdata/pull/10738) ([vkalintiris](https://github.com/vkalintiris))
- Skip C++ incompatible header in main libnetdata header [\#10737](https://github.com/netdata/netdata/pull/10737) ([vkalintiris](https://github.com/vkalintiris))
- Rename struct avl to avl\_element and the typedef to avl\_t [\#10735](https://github.com/netdata/netdata/pull/10735) ([vkalintiris](https://github.com/vkalintiris))
- Fix claim behind squid proxy [\#10734](https://github.com/netdata/netdata/pull/10734) ([underhood](https://github.com/underhood))
- add k6.conf [\#10733](https://github.com/netdata/netdata/pull/10733) ([OdysLam](https://github.com/OdysLam))
- Always configure multihost database context [\#10732](https://github.com/netdata/netdata/pull/10732) ([stelfrag](https://github.com/stelfrag))
- Removes unused fnc warning in ACLK Legacy [\#10731](https://github.com/netdata/netdata/pull/10731) ([underhood](https://github.com/underhood))
- Update chart's metadata in database when it already exists during creation [\#10728](https://github.com/netdata/netdata/pull/10728) ([stelfrag](https://github.com/stelfrag))
- New thread for ebpf.plugin [\#10726](https://github.com/netdata/netdata/pull/10726) ([thiagoftsm](https://github.com/thiagoftsm))
- Support VS Code container devenv [\#10723](https://github.com/netdata/netdata/pull/10723) ([OdysLam](https://github.com/OdysLam))
- Fixed detection of already claimed node in Docker images. [\#10720](https://github.com/netdata/netdata/pull/10720) ([Ferroin](https://github.com/Ferroin))
- Add statsd guide [\#10719](https://github.com/netdata/netdata/pull/10719) ([OdysLam](https://github.com/OdysLam))
- Add the ability to store chart labels in the database [\#10718](https://github.com/netdata/netdata/pull/10718) ([stelfrag](https://github.com/stelfrag))
- Fix a parameter binding issue when storing chart names in the database [\#10717](https://github.com/netdata/netdata/pull/10717) ([stelfrag](https://github.com/stelfrag))
- Fix typo in backend\_prometheus.c [\#10716](https://github.com/netdata/netdata/pull/10716) ([eltociear](https://github.com/eltociear))
- Add guide: Unsupervised anomaly detection for Raspberry Pi monitoring [\#10713](https://github.com/netdata/netdata/pull/10713) ([joelhans](https://github.com/joelhans))
- Add Working Set charts to the cgroups plugin [\#10712](https://github.com/netdata/netdata/pull/10712) ([vlvkobal](https://github.com/vlvkobal))
- python.d/smartd\_log: collect attribute 233 \(Media Wearout Indicator \(SSD\)\). [\#10711](https://github.com/netdata/netdata/pull/10711) ([aazedo](https://github.com/aazedo))
- Add guide: Develop a custom data collector for Netdata in Python [\#10710](https://github.com/netdata/netdata/pull/10710) ([joelhans](https://github.com/joelhans))
- New version eBPF programs. [\#10707](https://github.com/netdata/netdata/pull/10707) ([thiagoftsm](https://github.com/thiagoftsm))
- Add JSON output option for buildinfo. [\#10706](https://github.com/netdata/netdata/pull/10706) ([Ferroin](https://github.com/Ferroin))
- Fix disk utilization and backlog charts [\#10705](https://github.com/netdata/netdata/pull/10705) ([vlvkobal](https://github.com/vlvkobal))
- update\_kernel\_version: Fix overflow on Centos and probably Ubuntu [\#10704](https://github.com/netdata/netdata/pull/10704) ([thiagoftsm](https://github.com/thiagoftsm))
- Docs: Convert references to `service` to `systemctl` [\#10703](https://github.com/netdata/netdata/pull/10703) ([joelhans](https://github.com/joelhans))
- Add noauthcodecheck workaround flag to the freeipmi plugin [\#10701](https://github.com/netdata/netdata/pull/10701) ([vlvkobal](https://github.com/vlvkobal))
- Add guide: LAMP stack monitoring [\#10698](https://github.com/netdata/netdata/pull/10698) ([joelhans](https://github.com/joelhans))
- Log ACLK cloud commands to access.log [\#10697](https://github.com/netdata/netdata/pull/10697) ([stelfrag](https://github.com/stelfrag))
- Add Linux page cache metrics to eBPF [\#10693](https://github.com/netdata/netdata/pull/10693) ([thiagoftsm](https://github.com/thiagoftsm))
- Update guide: Kubernetes monitoring with Netdata: Overview and visualizations [\#10691](https://github.com/netdata/netdata/pull/10691) ([joelhans](https://github.com/joelhans))
- health: make alarms less sensitive [\#10688](https://github.com/netdata/netdata/pull/10688) ([ilyam8](https://github.com/ilyam8))
- Ebpf support new collectors [\#10680](https://github.com/netdata/netdata/pull/10680) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix broken links in active alarms doc [\#10678](https://github.com/netdata/netdata/pull/10678) ([joelhans](https://github.com/joelhans))
- Add new cookie to fix 8094 [\#10676](https://github.com/netdata/netdata/pull/10676) ([thiagoftsm](https://github.com/thiagoftsm))
- Alarms collector add alarm values [\#10675](https://github.com/netdata/netdata/pull/10675) ([andrewm4894](https://github.com/andrewm4894))
- Don't add duplicate \_total suffixes for the prometheus go.d module [\#10674](https://github.com/netdata/netdata/pull/10674) ([vlvkobal](https://github.com/vlvkobal))
- fix a typo in the email notifications readme [\#10668](https://github.com/netdata/netdata/pull/10668) ([ossimantylahti](https://github.com/ossimantylahti))
- Update screenshots and text for new Cloud nav [\#10664](https://github.com/netdata/netdata/pull/10664) ([joelhans](https://github.com/joelhans))
- Improve the Kubernetes deployment documentation [\#10662](https://github.com/netdata/netdata/pull/10662) ([joelhans](https://github.com/joelhans))
- installer: update go.d.plugin version to v0.28.0 [\#10660](https://github.com/netdata/netdata/pull/10660) ([ilyam8](https://github.com/ilyam8))
- Changed Docker image tagging to use semver tags for releases. [\#10648](https://github.com/netdata/netdata/pull/10648) ([Ferroin](https://github.com/Ferroin))
- Revamp statsd docs [\#10637](https://github.com/netdata/netdata/pull/10637) ([OdysLam](https://github.com/OdysLam))
- replace GA with PostHog for backend telemetry events. [\#10636](https://github.com/netdata/netdata/pull/10636) ([andrewm4894](https://github.com/andrewm4894))
- cpu stats per query thread [\#10634](https://github.com/netdata/netdata/pull/10634) ([MrZammler](https://github.com/MrZammler))
- Assorted updater fixes. [\#10613](https://github.com/netdata/netdata/pull/10613) ([Ferroin](https://github.com/Ferroin))
- add stats per cloud query type [\#10602](https://github.com/netdata/netdata/pull/10602) ([underhood](https://github.com/underhood))
- Add a new workflow to test that updater works as expected [\#10599](https://github.com/netdata/netdata/pull/10599) ([kaskavel](https://github.com/kaskavel))
- Add support for changing the number of pages per extent [\#10593](https://github.com/netdata/netdata/pull/10593) ([mfundul](https://github.com/mfundul))
- web/gui: Fix broken external links [\#10586](https://github.com/netdata/netdata/pull/10586) ([Habetdin](https://github.com/Habetdin))
- Fix wrong count for entries [\#10564](https://github.com/netdata/netdata/pull/10564) ([thiagoftsm](https://github.com/thiagoftsm))
- Try to keep all pages from extents read from disk in the cache. [\#10558](https://github.com/netdata/netdata/pull/10558) ([mfundul](https://github.com/mfundul))
- Remove unreachable \#else directives in plugins. [\#10523](https://github.com/netdata/netdata/pull/10523) ([vkalintiris](https://github.com/vkalintiris))
- Fixed handling of permissions for some plugins. [\#10490](https://github.com/netdata/netdata/pull/10490) ([Ferroin](https://github.com/Ferroin))

## [v1.29.3](https://github.com/netdata/netdata/tree/v1.29.3) (2021-02-23)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.2...v1.29.3)

**Merged pull requests:**

- Invalidate RRDSETVAR pointers on obsoletion. [\#10667](https://github.com/netdata/netdata/pull/10667) ([mfundul](https://github.com/mfundul))
- Fixed condition controlling use of static LWS in RPM builds. [\#10661](https://github.com/netdata/netdata/pull/10661) ([Ferroin](https://github.com/Ferroin))
- fix wrong link on docs Netdata Agent Daemon [\#10659](https://github.com/netdata/netdata/pull/10659) ([OdysLam](https://github.com/OdysLam))
- Fix broken links in docs and add collectors to list [\#10651](https://github.com/netdata/netdata/pull/10651) ([joelhans](https://github.com/joelhans))
- Statsd dashboard [\#10640](https://github.com/netdata/netdata/pull/10640) ([OdysLam](https://github.com/OdysLam))

## [v1.29.2](https://github.com/netdata/netdata/tree/v1.29.2) (2021-02-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.1...v1.29.2)

**Merged pull requests:**

- Fix the context filtering on the data query endpoint [\#10652](https://github.com/netdata/netdata/pull/10652) ([stelfrag](https://github.com/stelfrag))
- fix container/host detection in system-info script [\#10647](https://github.com/netdata/netdata/pull/10647) ([ilyam8](https://github.com/ilyam8))
- Enable apps.plugin aggregation debug messages [\#10645](https://github.com/netdata/netdata/pull/10645) ([vlvkobal](https://github.com/vlvkobal))
- add small delay to the ipv4\_tcp\_resets alarms [\#10644](https://github.com/netdata/netdata/pull/10644) ([ilyam8](https://github.com/ilyam8))
- collectors/proc: fix collecting operstate for virtual network interfaces [\#10633](https://github.com/netdata/netdata/pull/10633) ([ilyam8](https://github.com/ilyam8))
- fix sendmail unrecognized option F error [\#10631](https://github.com/netdata/netdata/pull/10631) ([ilyam8](https://github.com/ilyam8))
- Fix typo in web/gui/readme.md [\#10623](https://github.com/netdata/netdata/pull/10623) ([OdysLam](https://github.com/OdysLam))
- add freeswitch to apps\_groups [\#10621](https://github.com/netdata/netdata/pull/10621) ([fayak](https://github.com/fayak))
- Add ACLK proxy setting as host label [\#10619](https://github.com/netdata/netdata/pull/10619) ([underhood](https://github.com/underhood))
- dashboard@v2.13.6 [\#10618](https://github.com/netdata/netdata/pull/10618) ([jacekkolasa](https://github.com/jacekkolasa))
- Disable stock alarms [\#10617](https://github.com/netdata/netdata/pull/10617) ([thiagoftsm](https://github.com/thiagoftsm))
- Fixes \#10597 raw binary data should never be printed [\#10603](https://github.com/netdata/netdata/pull/10603) ([rda0](https://github.com/rda0))
- collectors/proc: change ksm mem chart type to stacked [\#10598](https://github.com/netdata/netdata/pull/10598) ([ilyam8](https://github.com/ilyam8))
- ACLK reduce excessive logging [\#10596](https://github.com/netdata/netdata/pull/10596) ([underhood](https://github.com/underhood))
- add k8s\_cluster\_id host label [\#10588](https://github.com/netdata/netdata/pull/10588) ([ilyam8](https://github.com/ilyam8))
- add resetting CapabilityBoundingSet workaround to the python.d collectors \(that use `sudo`\) readmes [\#10587](https://github.com/netdata/netdata/pull/10587) ([ilyam8](https://github.com/ilyam8))
- collectors/elasticsearch: document `scheme` option [\#10572](https://github.com/netdata/netdata/pull/10572) ([vjt](https://github.com/vjt))
- Update claiming docs for Docker containers. [\#10570](https://github.com/netdata/netdata/pull/10570) ([Ferroin](https://github.com/Ferroin))
- health: make Opsgenie API URL configurable [\#10561](https://github.com/netdata/netdata/pull/10561) ([tinyhammers](https://github.com/tinyhammers))
- Allow the REMOVED alarm status via ACLK if the previous status was WARN/CRIT [\#10533](https://github.com/netdata/netdata/pull/10533) ([stelfrag](https://github.com/stelfrag))

## [v1.29.1](https://github.com/netdata/netdata/tree/v1.29.1) (2021-02-09)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.29.0...v1.29.1)

**Merged pull requests:**

- Fix crash during shutdown of cgroups internal plugin. [\#10614](https://github.com/netdata/netdata/pull/10614) ([mfundul](https://github.com/mfundul))
- Update latest release on main README [\#10590](https://github.com/netdata/netdata/pull/10590) ([joelhans](https://github.com/joelhans))

## [v1.29.0](https://github.com/netdata/netdata/tree/v1.29.0) (2021-02-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.27.0_0104103941...v1.29.0)

**Merged pull requests:**

- Fixed Netdata Cloud support in RPM packages. [\#10578](https://github.com/netdata/netdata/pull/10578) ([Ferroin](https://github.com/Ferroin))
- Fix container detection from systemd-detect-virt [\#10569](https://github.com/netdata/netdata/pull/10569) ([cakrit](https://github.com/cakrit))
- dashboard v2.13.0 [\#10565](https://github.com/netdata/netdata/pull/10565) ([jacekkolasa](https://github.com/jacekkolasa))
- bytes after last '}' trip JSON parser [\#10563](https://github.com/netdata/netdata/pull/10563) ([underhood](https://github.com/underhood))
- Fix prometheus remote write header [\#10560](https://github.com/netdata/netdata/pull/10560) ([vlvkobal](https://github.com/vlvkobal))
- fix minor vulnerability alert, updating socket-io dependency [\#10557](https://github.com/netdata/netdata/pull/10557) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix values in Prometheus export for metrics, collected by the Prometheus collector [\#10551](https://github.com/netdata/netdata/pull/10551) ([vlvkobal](https://github.com/vlvkobal))
- Properly handle saved temporary directory on updates. [\#10550](https://github.com/netdata/netdata/pull/10550) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.27.0 [\#10544](https://github.com/netdata/netdata/pull/10544) ([ilyam8](https://github.com/ilyam8))
- Update README.md on postgres collector [\#10532](https://github.com/netdata/netdata/pull/10532) ([OdysLam](https://github.com/OdysLam))
- fix postgres password bug and change default config [\#10531](https://github.com/netdata/netdata/pull/10531) ([OdysLam](https://github.com/OdysLam))
- Make some tweaks/improvements to conf docs [\#10528](https://github.com/netdata/netdata/pull/10528) ([joelhans](https://github.com/joelhans))
- Spelling python plugin [\#10525](https://github.com/netdata/netdata/pull/10525) ([jsoref](https://github.com/jsoref))
- Reduce the number of alarm updates on ACLK [\#10524](https://github.com/netdata/netdata/pull/10524) ([stelfrag](https://github.com/stelfrag))
- dashboard@v2.12.5 [\#10520](https://github.com/netdata/netdata/pull/10520) ([jacekkolasa](https://github.com/jacekkolasa))
- Remove unused entries from structures [\#10519](https://github.com/netdata/netdata/pull/10519) ([stelfrag](https://github.com/stelfrag))
- Mark internal functions as static in health code. [\#10518](https://github.com/netdata/netdata/pull/10518) ([vkalintiris](https://github.com/vkalintiris))
- Remove unused struct in health code. [\#10517](https://github.com/netdata/netdata/pull/10517) ([vkalintiris](https://github.com/vkalintiris))
- Fix coverity issue CID 365322 [\#10516](https://github.com/netdata/netdata/pull/10516) ([stelfrag](https://github.com/stelfrag))
- health/mysql: fix `mysql.slave\_status` alarm for go mysql collector [\#10513](https://github.com/netdata/netdata/pull/10513) ([ilyam8](https://github.com/ilyam8))
- Spelling md [\#10508](https://github.com/netdata/netdata/pull/10508) ([jsoref](https://github.com/jsoref))
- Switched to using system libwebsockets for RPM builds. [\#10507](https://github.com/netdata/netdata/pull/10507) ([Ferroin](https://github.com/Ferroin))
- Add link to specific feedback megathread for the anomalies collector [\#10506](https://github.com/netdata/netdata/pull/10506) ([andrewm4894](https://github.com/andrewm4894))
- Add link to specific feedback megathread for the anomalies collector [\#10505](https://github.com/netdata/netdata/pull/10505) ([andrewm4894](https://github.com/andrewm4894))
- Fix broken dbengine stress tests. [\#10502](https://github.com/netdata/netdata/pull/10502) ([mfundul](https://github.com/mfundul))
- add `\_is\_k8s\_node` label to the host labels [\#10501](https://github.com/netdata/netdata/pull/10501) ([ilyam8](https://github.com/ilyam8))
- Fix segmentation fault in the agent [\#10498](https://github.com/netdata/netdata/pull/10498) ([mfundul](https://github.com/mfundul))
- Fixed handling of TLS config so that cURL works in all cases. [\#10491](https://github.com/netdata/netdata/pull/10491) ([Ferroin](https://github.com/Ferroin))
- Bump ini from 1.3.5 to 1.3.8 [\#10489](https://github.com/netdata/netdata/pull/10489) ([dependabot[bot]](https://github.com/apps/dependabot))
- health: make mdstat\_mismatch\_cnt alarm less strict [\#10488](https://github.com/netdata/netdata/pull/10488) ([ilyam8](https://github.com/ilyam8))
- Mention PostgreSQL Prometheus Adapter in the documentation [\#10487](https://github.com/netdata/netdata/pull/10487) ([vlvkobal](https://github.com/vlvkobal))

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
