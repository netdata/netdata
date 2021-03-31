# Changelog

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
- increases ACLK TBEB randomness [\#10373](https://github.com/netdata/netdata/pull/10373) ([underhood](https://github.com/underhood))
- Rename abs to ABS to avoid clash with standard definitions. Fixes \#10353. [\#10354](https://github.com/netdata/netdata/pull/10354) ([KickerTom](https://github.com/KickerTom))
- ACLK-NG [\#10315](https://github.com/netdata/netdata/pull/10315) ([underhood](https://github.com/underhood))

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
- Change eBPF plugin internal [\#10442](https://github.com/netdata/netdata/pull/10442) ([thiagoftsm](https://github.com/thiagoftsm))

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
- Fix memory allocation when computing standard deviation [\#10484](https://github.com/netdata/netdata/pull/10484) ([stelfrag](https://github.com/stelfrag))
- Support multiple chart label keys in data queries [\#10483](https://github.com/netdata/netdata/pull/10483) ([stelfrag](https://github.com/stelfrag))
- Claiming retry/backoff [\#10482](https://github.com/netdata/netdata/pull/10482) ([underhood](https://github.com/underhood))
- Add guide: Monitor and visualize anomalies with Netdata [\#10480](https://github.com/netdata/netdata/pull/10480) ([joelhans](https://github.com/joelhans))
- Truncate excessive information from titles for apps and cgroups [\#10479](https://github.com/netdata/netdata/pull/10479) ([vlvkobal](https://github.com/vlvkobal))
- Add vkalintiris to CODEOWNERS for CI, packaging, and installer code. [\#10478](https://github.com/netdata/netdata/pull/10478) ([Ferroin](https://github.com/Ferroin))
- GitHub action markdown link check update [\#10474](https://github.com/netdata/netdata/pull/10474) ([jsoref](https://github.com/jsoref))
- Fix for older compilers [\#10470](https://github.com/netdata/netdata/pull/10470) ([underhood](https://github.com/underhood))
- Fixes for SEO housekeeping/improvements [\#10468](https://github.com/netdata/netdata/pull/10468) ([joelhans](https://github.com/joelhans))
- Update README.md [\#10467](https://github.com/netdata/netdata/pull/10467) ([OdysLam](https://github.com/OdysLam))
- Update pfsense.md [\#10466](https://github.com/netdata/netdata/pull/10466) ([OdysLam](https://github.com/OdysLam))
- Fixed function name in updater script. [\#10462](https://github.com/netdata/netdata/pull/10462) ([Ferroin](https://github.com/Ferroin))
- Fixed bundling of libwebsockets in binary packages. [\#10460](https://github.com/netdata/netdata/pull/10460) ([Ferroin](https://github.com/Ferroin))
- Anomalies collector custom model bugfix for issue \#10456 [\#10459](https://github.com/netdata/netdata/pull/10459) ([andrewm4894](https://github.com/andrewm4894))
- Add missing section to Netdata style guide [\#10453](https://github.com/netdata/netdata/pull/10453) ([joelhans](https://github.com/joelhans))
- Add guide: Detect anomalies in nodes and applications with Netdata [\#10451](https://github.com/netdata/netdata/pull/10451) ([joelhans](https://github.com/joelhans))
- Updated messages about checksum validation failures on install. [\#10448](https://github.com/netdata/netdata/pull/10448) ([Ferroin](https://github.com/Ferroin))
- Fixed handling of environment file in updater script. [\#10447](https://github.com/netdata/netdata/pull/10447) ([Ferroin](https://github.com/Ferroin))
- Exclude autofs by default in diskspace plugin [\#10441](https://github.com/netdata/netdata/pull/10441) ([nabijaczleweli](https://github.com/nabijaczleweli))
- New eBPF kernel [\#10434](https://github.com/netdata/netdata/pull/10434) ([thiagoftsm](https://github.com/thiagoftsm))
- Update and improve the Netdata style guide [\#10433](https://github.com/netdata/netdata/pull/10433) ([joelhans](https://github.com/joelhans))
- Change HDDtemp to report None instead of 0 [\#10429](https://github.com/netdata/netdata/pull/10429) ([slavox](https://github.com/slavox))
- Qick and dirty fix for \#10420 [\#10424](https://github.com/netdata/netdata/pull/10424) ([skibbipl](https://github.com/skibbipl))
- Add instructions on enabling explicitly disabled collectors [\#10418](https://github.com/netdata/netdata/pull/10418) ([joelhans](https://github.com/joelhans))
- Change links at bottom of all install docs [\#10416](https://github.com/netdata/netdata/pull/10416) ([joelhans](https://github.com/joelhans))
- Improve configuration docs with common changes and start/stop/restart directions [\#10415](https://github.com/netdata/netdata/pull/10415) ([joelhans](https://github.com/joelhans))
- Small updates, improvements, and housekeeping to docs [\#10405](https://github.com/netdata/netdata/pull/10405) ([joelhans](https://github.com/joelhans))
- python.d/fail2ban: Add handling "yes" and "no" as bool, match flexible spaces [\#10400](https://github.com/netdata/netdata/pull/10400) ([grinapo](https://github.com/grinapo))
- Dispatch cgroup discovery into another thread [\#10399](https://github.com/netdata/netdata/pull/10399) ([vlvkobal](https://github.com/vlvkobal))
- Fix data source option for Prometheus web API in exporting configuration [\#10397](https://github.com/netdata/netdata/pull/10397) ([vlvkobal](https://github.com/vlvkobal))
- Docs housekeeping for SEO and syntax, part 1 [\#10388](https://github.com/netdata/netdata/pull/10388) ([joelhans](https://github.com/joelhans))
- Persist `$TMPDIR` from installer to updater. [\#10384](https://github.com/netdata/netdata/pull/10384) ([Ferroin](https://github.com/Ferroin))
- Change linting standard for Markdown lists [\#10371](https://github.com/netdata/netdata/pull/10371) ([joelhans](https://github.com/joelhans))
- Move ACLK Legacy into subfolder [\#10265](https://github.com/netdata/netdata/pull/10265) ([underhood](https://github.com/underhood))

## [v1.27.0_0104103941](https://github.com/netdata/netdata/tree/v1.27.0_0104103941) (2021-01-04)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.28.0...v1.27.0_0104103941)

**Merged pull requests:**

- Use bash shell as user netdata for debug [\#10425](https://github.com/netdata/netdata/pull/10425) ([Steve8291](https://github.com/Steve8291))
- Add Realtek network cards to the list of physical interfaces on FreeBSD [\#10414](https://github.com/netdata/netdata/pull/10414) ([vlvkobal](https://github.com/vlvkobal))
- Update main README with release news [\#10412](https://github.com/netdata/netdata/pull/10412) ([joelhans](https://github.com/joelhans))
- Added instructions on which file to edit. [\#10398](https://github.com/netdata/netdata/pull/10398) ([kdvlr](https://github.com/kdvlr))
- ACLK collector list use mguid instead of hostname [\#10394](https://github.com/netdata/netdata/pull/10394) ([underhood](https://github.com/underhood))
- Add centralized Cloud notifications to core docs [\#10374](https://github.com/netdata/netdata/pull/10374) ([joelhans](https://github.com/joelhans))

## [v1.28.0](https://github.com/netdata/netdata/tree/v1.28.0) (2020-12-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.27.0...v1.28.0)

**Merged pull requests:**

- Fix locking after on\_connect failure [\#10401](https://github.com/netdata/netdata/pull/10401) ([stelfrag](https://github.com/stelfrag))

## [v1.27.0](https://github.com/netdata/netdata/tree/v1.27.0) (2020-12-17)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.26.0...v1.27.0)

**Merged pull requests:**

- Fixed option parsing in kickstart.sh. [\#10396](https://github.com/netdata/netdata/pull/10396) ([Ferroin](https://github.com/Ferroin))
- invalid\_addr\_cleanup: Initialize variables [\#10395](https://github.com/netdata/netdata/pull/10395) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix a buffer overflow when extracting information from a STREAM connection [\#10391](https://github.com/netdata/netdata/pull/10391) ([stelfrag](https://github.com/stelfrag))
- fix typo in performance.md [\#10386](https://github.com/netdata/netdata/pull/10386) ([OdysLam](https://github.com/OdysLam))
- Fix a lock check [\#10385](https://github.com/netdata/netdata/pull/10385) ([vlvkobal](https://github.com/vlvkobal))
- dashboard v2.11 [\#10383](https://github.com/netdata/netdata/pull/10383) ([jacekkolasa](https://github.com/jacekkolasa))
- Fixed handling of dependencies on Gentoo. [\#10382](https://github.com/netdata/netdata/pull/10382) ([Ferroin](https://github.com/Ferroin))
- Fix issue with chart metadata sent multiple times over ACLK [\#10381](https://github.com/netdata/netdata/pull/10381) ([stelfrag](https://github.com/stelfrag))
- Update macos.md [\#10379](https://github.com/netdata/netdata/pull/10379) ([ktsaou](https://github.com/ktsaou))
- python.d/alarms: fix sending chart definition on every data collection [\#10378](https://github.com/netdata/netdata/pull/10378) ([ilyam8](https://github.com/ilyam8))
- python.d/alarms: add alarms obsoletion and disable by default [\#10375](https://github.com/netdata/netdata/pull/10375) ([ilyam8](https://github.com/ilyam8))
- add two more insignificant warnings to suppress in anomalies collector [\#10369](https://github.com/netdata/netdata/pull/10369) ([andrewm4894](https://github.com/andrewm4894))
- add paragraph in anomalies collector README to ask for feedback [\#10363](https://github.com/netdata/netdata/pull/10363) ([andrewm4894](https://github.com/andrewm4894))
- Fix hostname configuration in the exporting engine [\#10361](https://github.com/netdata/netdata/pull/10361) ([vlvkobal](https://github.com/vlvkobal))
- New ebpf charts [\#10360](https://github.com/netdata/netdata/pull/10360) ([thiagoftsm](https://github.com/thiagoftsm))
- installer: update go.d.plugin version to v0.26.2 [\#10355](https://github.com/netdata/netdata/pull/10355) ([ilyam8](https://github.com/ilyam8))
- Fixed handling of self-updating in updater script. [\#10352](https://github.com/netdata/netdata/pull/10352) ([Ferroin](https://github.com/Ferroin))
- update alarms collector readme image to use one from the related netdata PR [\#10348](https://github.com/netdata/netdata/pull/10348) ([andrewm4894](https://github.com/andrewm4894))
- Add documentation for time & date picker in Agent and Cloud [\#10347](https://github.com/netdata/netdata/pull/10347) ([joelhans](https://github.com/joelhans))
- Use `glibtoolize` on macOS instead of regular `libtoolize`. [\#10346](https://github.com/netdata/netdata/pull/10346) ([Ferroin](https://github.com/Ferroin))
- Fix handling of Python dependency for RPM package. [\#10345](https://github.com/netdata/netdata/pull/10345) ([Ferroin](https://github.com/Ferroin))
- Fixed handling of PowerTools repo on CentOS 8. [\#10344](https://github.com/netdata/netdata/pull/10344) ([Ferroin](https://github.com/Ferroin))
- Fix backend options [\#10343](https://github.com/netdata/netdata/pull/10343) ([vlvkobal](https://github.com/vlvkobal))
- Add guide: Monitor any process in real-time with Netdata [\#10338](https://github.com/netdata/netdata/pull/10338) ([joelhans](https://github.com/joelhans))
- Added patch to build LWS properly on macOS. [\#10333](https://github.com/netdata/netdata/pull/10333) ([Ferroin](https://github.com/Ferroin))
- Added number of allocated/stored objects within each Varnish storage [\#10329](https://github.com/netdata/netdata/pull/10329) ([ernestojpg](https://github.com/ernestojpg))
- health: disable 'used\_file\_descriptors' alarm [\#10328](https://github.com/netdata/netdata/pull/10328) ([ilyam8](https://github.com/ilyam8))
- Fix exporting config [\#10323](https://github.com/netdata/netdata/pull/10323) ([vlvkobal](https://github.com/vlvkobal))
- Fix a compilation warning [\#10320](https://github.com/netdata/netdata/pull/10320) ([vlvkobal](https://github.com/vlvkobal))
- installer: update go.d.plugin version to v0.26.1 [\#10319](https://github.com/netdata/netdata/pull/10319) ([ilyam8](https://github.com/ilyam8))
- Improve core documentation to align with recent Netdata Cloud releases [\#10318](https://github.com/netdata/netdata/pull/10318) ([joelhans](https://github.com/joelhans))
- Added support for MSE \(Massive Storage Engine\) in Varnish-Plus [\#10317](https://github.com/netdata/netdata/pull/10317) ([ernestojpg](https://github.com/ernestojpg))
- dashboard v2.10.1 [\#10314](https://github.com/netdata/netdata/pull/10314) ([jacekkolasa](https://github.com/jacekkolasa))
- fix UUID\_STR\_LEN undefined on MacOS [\#10313](https://github.com/netdata/netdata/pull/10313) ([underhood](https://github.com/underhood))
- python.d/nvidia\_smi: fix gpu data filtering [\#10312](https://github.com/netdata/netdata/pull/10312) ([ilyam8](https://github.com/ilyam8))
- Add new collectors to supported collectors list [\#10310](https://github.com/netdata/netdata/pull/10310) ([joelhans](https://github.com/joelhans))
- Added numerous improvements to our Docker image. [\#10308](https://github.com/netdata/netdata/pull/10308) ([Ferroin](https://github.com/Ferroin))
- Update README.md [\#10303](https://github.com/netdata/netdata/pull/10303) ([ktsaou](https://github.com/ktsaou))
- Add optional info on how to also add some default alarms as part of s… [\#10302](https://github.com/netdata/netdata/pull/10302) ([andrewm4894](https://github.com/andrewm4894))
- Update UPDATE.md [\#10301](https://github.com/netdata/netdata/pull/10301) ([ysamouhos](https://github.com/ysamouhos))
- HAProxy spelling correction [\#10300](https://github.com/netdata/netdata/pull/10300) ([autoalan](https://github.com/autoalan))
- eBPF synchronization [\#10299](https://github.com/netdata/netdata/pull/10299) ([thiagoftsm](https://github.com/thiagoftsm))
- python.d: always create a runtime chart on `create` call [\#10296](https://github.com/netdata/netdata/pull/10296) ([ilyam8](https://github.com/ilyam8))
- Update macOS instructions with cmake [\#10295](https://github.com/netdata/netdata/pull/10295) ([joelhans](https://github.com/joelhans))
- dbengine extent cache [\#10293](https://github.com/netdata/netdata/pull/10293) ([mfundul](https://github.com/mfundul))
- add privacy information about aclk connection [\#10292](https://github.com/netdata/netdata/pull/10292) ([OdysLam](https://github.com/OdysLam))
- Fixed the data endpoint so that the context param is correctly applied to children [\#10290](https://github.com/netdata/netdata/pull/10290) ([stelfrag](https://github.com/stelfrag))
- installer: update go.d.plugin version to v0.26.0 [\#10284](https://github.com/netdata/netdata/pull/10284) ([ilyam8](https://github.com/ilyam8))
- use new libmosquitto release \(with MacOS libMosq fix\) [\#10283](https://github.com/netdata/netdata/pull/10283) ([underhood](https://github.com/underhood))
- Address coverity errors \(CID 364045,364046\) [\#10282](https://github.com/netdata/netdata/pull/10282) ([stelfrag](https://github.com/stelfrag))
- health/web\_log: remove `crit` from unmatched alarms [\#10280](https://github.com/netdata/netdata/pull/10280) ([ilyam8](https://github.com/ilyam8))
- Fix compilation with https disabled. Fixes \#10278 [\#10279](https://github.com/netdata/netdata/pull/10279) ([KickerTom](https://github.com/KickerTom))
- Fix race condition in rrdset\_first\_entry\_t\(\) and rrdset\_last\_entry\_t\(\) [\#10276](https://github.com/netdata/netdata/pull/10276) ([mfundul](https://github.com/mfundul))
- Fix host name when syslog is used [\#10275](https://github.com/netdata/netdata/pull/10275) ([thiagoftsm](https://github.com/thiagoftsm))
- Add guide: How to optimize Netdata's performance [\#10271](https://github.com/netdata/netdata/pull/10271) ([joelhans](https://github.com/joelhans))
- Document the Agent reinstallation process [\#10270](https://github.com/netdata/netdata/pull/10270) ([joelhans](https://github.com/joelhans))
- fix bug\_report.md syntax error [\#10269](https://github.com/netdata/netdata/pull/10269) ([OdysLam](https://github.com/OdysLam))
- python.d/nvidia\_smi: use `pwd` lib to get username if not inside a container [\#10268](https://github.com/netdata/netdata/pull/10268) ([ilyam8](https://github.com/ilyam8))
- Add kernel to blacklist [\#10262](https://github.com/netdata/netdata/pull/10262) ([thiagoftsm](https://github.com/thiagoftsm))
- Made the update script significantly more robust and user friendly. [\#10261](https://github.com/netdata/netdata/pull/10261) ([Ferroin](https://github.com/Ferroin))
- new issue templates [\#10259](https://github.com/netdata/netdata/pull/10259) ([OdysLam](https://github.com/OdysLam))

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
