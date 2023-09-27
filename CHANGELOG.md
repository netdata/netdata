# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.4...HEAD)

**Merged pull requests:**

- update bundled ui version to v6.41.1 [\#16054](https://github.com/netdata/netdata/pull/16054) ([ilyam8](https://github.com/ilyam8))
- Fix missing find command when installing/updating on Rocky Linux systems. [\#16052](https://github.com/netdata/netdata/pull/16052) ([Ferroin](https://github.com/Ferroin))
- Fix summary field in table [\#16050](https://github.com/netdata/netdata/pull/16050) ([MrZammler](https://github.com/MrZammler))
- Switch to uint64\_t to avoid overflow in 32bit systems [\#16048](https://github.com/netdata/netdata/pull/16048) ([stelfrag](https://github.com/stelfrag))
- Doc about running a local dashboard through Cloudflare \(community\) [\#16043](https://github.com/netdata/netdata/pull/16043) ([Ancairon](https://github.com/Ancairon))
- Remove discontinued Hangouts and StackPulse notification methods [\#16041](https://github.com/netdata/netdata/pull/16041) ([Ancairon](https://github.com/Ancairon))
- Disable mongodb exporter builds where broken. [\#16033](https://github.com/netdata/netdata/pull/16033) ([Ferroin](https://github.com/Ferroin))
- Run health queries from tier 0 [\#16032](https://github.com/netdata/netdata/pull/16032) ([MrZammler](https://github.com/MrZammler))
- use `status` as units for `anomaly_detection.detector_events` [\#16028](https://github.com/netdata/netdata/pull/16028) ([andrewm4894](https://github.com/andrewm4894))
- add description for Homebrew on Apple Silicon Mac\(netdata/learn/\#1789\) [\#16027](https://github.com/netdata/netdata/pull/16027) ([theggs](https://github.com/theggs))
- Fix package builds on Rocky Linux. [\#16026](https://github.com/netdata/netdata/pull/16026) ([Ferroin](https://github.com/Ferroin))
- add systemd-journal.plugin to apps\_groups.conf [\#16024](https://github.com/netdata/netdata/pull/16024) ([ilyam8](https://github.com/ilyam8))
- Fix handling of CI skipping. [\#16022](https://github.com/netdata/netdata/pull/16022) ([Ferroin](https://github.com/Ferroin))
- update bundled UI to v6.39.0 [\#16020](https://github.com/netdata/netdata/pull/16020) ([ilyam8](https://github.com/ilyam8))
- Update collector metadata for python collectors [\#16019](https://github.com/netdata/netdata/pull/16019) ([tkatsoulas](https://github.com/tkatsoulas))
- fix crash on setting thread name [\#16016](https://github.com/netdata/netdata/pull/16016) ([ilyam8](https://github.com/ilyam8))
- Systemd-Journal: fix crash when the uid or gid do not have names [\#16015](https://github.com/netdata/netdata/pull/16015) ([ktsaou](https://github.com/ktsaou))
- remove the line length limit from pluginsd [\#16013](https://github.com/netdata/netdata/pull/16013) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16011](https://github.com/netdata/netdata/pull/16011) ([netdatabot](https://github.com/netdatabot))
- Simplify the script for generating documentation from integrations [\#16009](https://github.com/netdata/netdata/pull/16009) ([Ancairon](https://github.com/Ancairon))
- some collector metadata improvements [\#16008](https://github.com/netdata/netdata/pull/16008) ([andrewm4894](https://github.com/andrewm4894))
- Fix compilation warnings [\#16006](https://github.com/netdata/netdata/pull/16006) ([stelfrag](https://github.com/stelfrag))
- Update CMakeLists.txt [\#16005](https://github.com/netdata/netdata/pull/16005) ([stelfrag](https://github.com/stelfrag))
- eBPF socket: function with event loop [\#16004](https://github.com/netdata/netdata/pull/16004) ([thiagoftsm](https://github.com/thiagoftsm))
- fix compilation warnings [\#16001](https://github.com/netdata/netdata/pull/16001) ([ktsaou](https://github.com/ktsaou))
- Update integrations/gen\_docs\_integrations.py [\#15997](https://github.com/netdata/netdata/pull/15997) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#15985](https://github.com/netdata/netdata/pull/15985) ([netdatabot](https://github.com/netdatabot))
- Re-store rrdvars on late dimensions [\#15984](https://github.com/netdata/netdata/pull/15984) ([MrZammler](https://github.com/MrZammler))
- Functions: allow collectors to be restarted [\#15983](https://github.com/netdata/netdata/pull/15983) ([ktsaou](https://github.com/ktsaou))
- Metadata fixes for some collectors [\#15982](https://github.com/netdata/netdata/pull/15982) ([Ancairon](https://github.com/Ancairon))
- update go.d.plugin to v0.55.0 [\#15981](https://github.com/netdata/netdata/pull/15981) ([ilyam8](https://github.com/ilyam8))
- bump UI to v6.37.1 [\#15980](https://github.com/netdata/netdata/pull/15980) ([ilyam8](https://github.com/ilyam8))
- Maintain node's last connected timestamp in the db [\#15979](https://github.com/netdata/netdata/pull/15979) ([stelfrag](https://github.com/stelfrag))
- apps.plugin function is not thread safe [\#15978](https://github.com/netdata/netdata/pull/15978) ([ktsaou](https://github.com/ktsaou))
- functions cancelling [\#15977](https://github.com/netdata/netdata/pull/15977) ([ktsaou](https://github.com/ktsaou))
- Facets: fixes 5 [\#15976](https://github.com/netdata/netdata/pull/15976) ([ktsaou](https://github.com/ktsaou))
- facets histogram: do not send db retention for facets [\#15974](https://github.com/netdata/netdata/pull/15974) ([ktsaou](https://github.com/ktsaou))
- extend ml default training from ~24 to ~48 hours [\#15971](https://github.com/netdata/netdata/pull/15971) ([andrewm4894](https://github.com/andrewm4894))
- facets histogram when empty [\#15970](https://github.com/netdata/netdata/pull/15970) ([ktsaou](https://github.com/ktsaou))
- facets: do not shadow local variable [\#15968](https://github.com/netdata/netdata/pull/15968) ([ktsaou](https://github.com/ktsaou))
- Skip trying to preserve file owners when bundling external code. [\#15966](https://github.com/netdata/netdata/pull/15966) ([Ferroin](https://github.com/Ferroin))
- fix using undefined var when loading job statuses in python.d [\#15965](https://github.com/netdata/netdata/pull/15965) ([ilyam8](https://github.com/ilyam8))
- facets: data-only queries [\#15961](https://github.com/netdata/netdata/pull/15961) ([ktsaou](https://github.com/ktsaou))
- Clarify shipping repos cases [\#15960](https://github.com/netdata/netdata/pull/15960) ([tkatsoulas](https://github.com/tkatsoulas))
- Clarifying the possible installation types [\#15958](https://github.com/netdata/netdata/pull/15958) ([tkatsoulas](https://github.com/tkatsoulas))
- fix journal direction parsing [\#15957](https://github.com/netdata/netdata/pull/15957) ([ktsaou](https://github.com/ktsaou))
- facets and journal improvements [\#15956](https://github.com/netdata/netdata/pull/15956) ([ktsaou](https://github.com/ktsaou))
- Preserve README info inside metadata.yaml for freeipmi.plugin [\#15955](https://github.com/netdata/netdata/pull/15955) ([Ancairon](https://github.com/Ancairon))
- Fix CID 400366 [\#15953](https://github.com/netdata/netdata/pull/15953) ([stelfrag](https://github.com/stelfrag))
- Update descriptions. [\#15952](https://github.com/netdata/netdata/pull/15952) ([thiagoftsm](https://github.com/thiagoftsm))
- Update slabinfo metadata [\#15951](https://github.com/netdata/netdata/pull/15951) ([thiagoftsm](https://github.com/thiagoftsm))
- Disk Labels [\#15949](https://github.com/netdata/netdata/pull/15949) ([ktsaou](https://github.com/ktsaou))
- streaming logs [\#15948](https://github.com/netdata/netdata/pull/15948) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15946](https://github.com/netdata/netdata/pull/15946) ([netdatabot](https://github.com/netdatabot))
- Integrations: Add a note to enable the collectors [\#15945](https://github.com/netdata/netdata/pull/15945) ([MrZammler](https://github.com/MrZammler))
- Change the build image of EL packages from alma to rocky [\#15944](https://github.com/netdata/netdata/pull/15944) ([tkatsoulas](https://github.com/tkatsoulas))
- Integrations: Add a note to install charts.d plugin [\#15943](https://github.com/netdata/netdata/pull/15943) ([MrZammler](https://github.com/MrZammler))
- Add description about packages [\#15941](https://github.com/netdata/netdata/pull/15941) ([thiagoftsm](https://github.com/thiagoftsm))
- facets optimizations [\#15940](https://github.com/netdata/netdata/pull/15940) ([ktsaou](https://github.com/ktsaou))
- improved facets info [\#15936](https://github.com/netdata/netdata/pull/15936) ([ktsaou](https://github.com/ktsaou))
- feat: Adds access control configuration for ntfy [\#15932](https://github.com/netdata/netdata/pull/15932) ([miversen33](https://github.com/miversen33))
- fix memory leak on prometheus exporter and code cleanup [\#15929](https://github.com/netdata/netdata/pull/15929) ([ktsaou](https://github.com/ktsaou))
- systemd-journal and facets: info and sources [\#15928](https://github.com/netdata/netdata/pull/15928) ([ktsaou](https://github.com/ktsaou))
- systemd-journal and facets improvements [\#15926](https://github.com/netdata/netdata/pull/15926) ([ktsaou](https://github.com/ktsaou))
- add specific info on how to access the dashboards [\#15925](https://github.com/netdata/netdata/pull/15925) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Reduce workload during cleanup [\#15919](https://github.com/netdata/netdata/pull/15919) ([stelfrag](https://github.com/stelfrag))
- Replace \_ with spaces for name variable for ntfy [\#15909](https://github.com/netdata/netdata/pull/15909) ([MAH69IK](https://github.com/MAH69IK))
- python.d/sensors: Increase voltage limits 127 -\> 400 [\#15905](https://github.com/netdata/netdata/pull/15905) ([kylemanna](https://github.com/kylemanna))
- Assorted Dockerfile cleanup. [\#15902](https://github.com/netdata/netdata/pull/15902) ([Ferroin](https://github.com/Ferroin))
- Improve shutdown of the metadata thread [\#15901](https://github.com/netdata/netdata/pull/15901) ([stelfrag](https://github.com/stelfrag))
- bump ui to v6.32.0 [\#15897](https://github.com/netdata/netdata/pull/15897) ([andrewm4894](https://github.com/andrewm4894))
- Update change-metrics-storage.md [\#15896](https://github.com/netdata/netdata/pull/15896) ([Ancairon](https://github.com/Ancairon))
- make `anomaly_detection.type_anomaly_rate` stacked [\#15895](https://github.com/netdata/netdata/pull/15895) ([andrewm4894](https://github.com/andrewm4894))
- Update pfsense.md [\#15894](https://github.com/netdata/netdata/pull/15894) ([Ancairon](https://github.com/Ancairon))
- Initial tooling for Integrations Documentation [\#15893](https://github.com/netdata/netdata/pull/15893) ([Ancairon](https://github.com/Ancairon))
- Reset the obsolete flag on service thread [\#15892](https://github.com/netdata/netdata/pull/15892) ([MrZammler](https://github.com/MrZammler))
- Add better recovery for corrupted metadata [\#15891](https://github.com/netdata/netdata/pull/15891) ([stelfrag](https://github.com/stelfrag))
- Add index to ACLK table to improve update statements [\#15890](https://github.com/netdata/netdata/pull/15890) ([stelfrag](https://github.com/stelfrag))
- Limit atomic operations for statistics [\#15887](https://github.com/netdata/netdata/pull/15887) ([ktsaou](https://github.com/ktsaou))
- Add a summary field to alerts [\#15886](https://github.com/netdata/netdata/pull/15886) ([MrZammler](https://github.com/MrZammler))
- Properly document issues with installing on hosts without IPv4. [\#15882](https://github.com/netdata/netdata/pull/15882) ([Ferroin](https://github.com/Ferroin))
- allow any field to be a facet [\#15880](https://github.com/netdata/netdata/pull/15880) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15879](https://github.com/netdata/netdata/pull/15879) ([netdatabot](https://github.com/netdatabot))
- use the newer XXH3 128bits algorithm, instead of the classic XXH128 [\#15878](https://github.com/netdata/netdata/pull/15878) ([ktsaou](https://github.com/ktsaou))
- Skip copying environment/install-type files when checking existing installs. [\#15876](https://github.com/netdata/netdata/pull/15876) ([Ferroin](https://github.com/Ferroin))
- ML add new `delete old models param` to readme [\#15873](https://github.com/netdata/netdata/pull/15873) ([andrewm4894](https://github.com/andrewm4894))
- Update SQLITE version to 3.42.0 [\#15870](https://github.com/netdata/netdata/pull/15870) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#15867](https://github.com/netdata/netdata/pull/15867) ([netdatabot](https://github.com/netdatabot))
- Add a fail reason to pinpoint exactly what went wrong [\#15866](https://github.com/netdata/netdata/pull/15866) ([stelfrag](https://github.com/stelfrag))
- Add plugin and module information to collector integrations. [\#15864](https://github.com/netdata/netdata/pull/15864) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#15862](https://github.com/netdata/netdata/pull/15862) ([netdatabot](https://github.com/netdatabot))
- Explicitly depend on version-matched plugins in native packages. [\#15861](https://github.com/netdata/netdata/pull/15861) ([Ferroin](https://github.com/Ferroin))
- Apply a label prefix for netdata labels [\#15860](https://github.com/netdata/netdata/pull/15860) ([kevin-fwu](https://github.com/kevin-fwu))
- fix proc meminfo cached calculation [\#15859](https://github.com/netdata/netdata/pull/15859) ([ilyam8](https://github.com/ilyam8))
- Fix compilation warnings [\#15858](https://github.com/netdata/netdata/pull/15858) ([stelfrag](https://github.com/stelfrag))
- packaging cleanup after \#15842 [\#15857](https://github.com/netdata/netdata/pull/15857) ([ilyam8](https://github.com/ilyam8))
- Add a chart that groups anomaly rate by chart type. [\#15856](https://github.com/netdata/netdata/pull/15856) ([vkalintiris](https://github.com/vkalintiris))
- fix packaging static build openssl 32bit [\#15855](https://github.com/netdata/netdata/pull/15855) ([ilyam8](https://github.com/ilyam8))
- fix packaging mark stable static build [\#15854](https://github.com/netdata/netdata/pull/15854) ([ilyam8](https://github.com/ilyam8))
- eBPF socket function [\#15850](https://github.com/netdata/netdata/pull/15850) ([thiagoftsm](https://github.com/thiagoftsm))
- Facets histograms [\#15846](https://github.com/netdata/netdata/pull/15846) ([ktsaou](https://github.com/ktsaou))
- reworked pluginsd caching of RDAs to avoid crashes [\#15845](https://github.com/netdata/netdata/pull/15845) ([ktsaou](https://github.com/ktsaou))
- Fix static build SSL [\#15842](https://github.com/netdata/netdata/pull/15842) ([ktsaou](https://github.com/ktsaou))
- bump bundled ui to v6.29.0 [\#15841](https://github.com/netdata/netdata/pull/15841) ([ilyam8](https://github.com/ilyam8))
- Fix configure: WARNING: unrecognized options: --with-zlib [\#15840](https://github.com/netdata/netdata/pull/15840) ([stelfrag](https://github.com/stelfrag))
- Fix compilation warning [\#15839](https://github.com/netdata/netdata/pull/15839) ([stelfrag](https://github.com/stelfrag))
- Fix warning when compiling with -flto [\#15838](https://github.com/netdata/netdata/pull/15838) ([stelfrag](https://github.com/stelfrag))
- workaround for systems that do not have SD\_JOURNAL\_OS\_ROOT [\#15837](https://github.com/netdata/netdata/pull/15837) ([ktsaou](https://github.com/ktsaou))
- added ilove.html [\#15836](https://github.com/netdata/netdata/pull/15836) ([ktsaou](https://github.com/ktsaou))
- Fix CID 382964:  Code maintainability issues  \(SIZEOF\_MISMATCH\) [\#15833](https://github.com/netdata/netdata/pull/15833) ([stelfrag](https://github.com/stelfrag))
- Fix coverity 393052:  API usage errors  \(LOCK\) [\#15832](https://github.com/netdata/netdata/pull/15832) ([stelfrag](https://github.com/stelfrag))
- systemd-journal in containers [\#15830](https://github.com/netdata/netdata/pull/15830) ([ktsaou](https://github.com/ktsaou))
- RPM: fixed attrs for conf.d dirs [\#15828](https://github.com/netdata/netdata/pull/15828) ([k0ste](https://github.com/k0ste))
- Avoid resource leak  [\#15827](https://github.com/netdata/netdata/pull/15827) ([stelfrag](https://github.com/stelfrag))
- Release fd if setsockopt or bind fails [\#15826](https://github.com/netdata/netdata/pull/15826) ([stelfrag](https://github.com/stelfrag))
- Fix use after free [\#15825](https://github.com/netdata/netdata/pull/15825) ([stelfrag](https://github.com/stelfrag))
- Improve dyncfg exit [\#15824](https://github.com/netdata/netdata/pull/15824) ([underhood](https://github.com/underhood))
- Release job message status to avoid memory leak [\#15822](https://github.com/netdata/netdata/pull/15822) ([stelfrag](https://github.com/stelfrag))
- ML improve init [\#15819](https://github.com/netdata/netdata/pull/15819) ([stelfrag](https://github.com/stelfrag))
- Update cmakelist [\#15817](https://github.com/netdata/netdata/pull/15817) ([stelfrag](https://github.com/stelfrag))
- added /api/v2/ilove.svg endpoint [\#15815](https://github.com/netdata/netdata/pull/15815) ([ktsaou](https://github.com/ktsaou))
- systemd-journal fixes [\#15814](https://github.com/netdata/netdata/pull/15814) ([ktsaou](https://github.com/ktsaou))
- fix packaging: link health.log to stdout [\#15813](https://github.com/netdata/netdata/pull/15813) ([ilyam8](https://github.com/ilyam8))
- docs rename alarm to alert [\#15812](https://github.com/netdata/netdata/pull/15812) ([ilyam8](https://github.com/ilyam8))
- bump ui to v6.28.0 [\#15810](https://github.com/netdata/netdata/pull/15810) ([ilyam8](https://github.com/ilyam8))
- return 412 instead of 403 when a bearer token is required [\#15808](https://github.com/netdata/netdata/pull/15808) ([ktsaou](https://github.com/ktsaou))
- installer setuid fallback for perf and slabinfo plugins [\#15807](https://github.com/netdata/netdata/pull/15807) ([ilyam8](https://github.com/ilyam8))
- fix api v1 mgmt/health [\#15806](https://github.com/netdata/netdata/pull/15806) ([underhood](https://github.com/underhood))
- Fix systemd journal build deps in DEB packages. [\#15805](https://github.com/netdata/netdata/pull/15805) ([Ferroin](https://github.com/Ferroin))
- Clean up python deps for RPM packages. [\#15804](https://github.com/netdata/netdata/pull/15804) ([Ferroin](https://github.com/Ferroin))
- Add proper SUID fallback for DEB plugin packages. [\#15803](https://github.com/netdata/netdata/pull/15803) ([Ferroin](https://github.com/Ferroin))
- nfacct.plugin increase restart time from 4 hours to 1 day [\#15801](https://github.com/netdata/netdata/pull/15801) ([ilyam8](https://github.com/ilyam8))
- Function systemd-journal: always have a nd\_journal\_process [\#15798](https://github.com/netdata/netdata/pull/15798) ([ktsaou](https://github.com/ktsaou))
- prevent reporting negative retention when the db is empty [\#15796](https://github.com/netdata/netdata/pull/15796) ([ktsaou](https://github.com/ktsaou))
- Fix typo in Readme [\#15794](https://github.com/netdata/netdata/pull/15794) ([shyamvalsan](https://github.com/shyamvalsan))
- fix hpssa handle unassigned drives [\#15793](https://github.com/netdata/netdata/pull/15793) ([ilyam8](https://github.com/ilyam8))
- count functions as collections, to restart plugins [\#15787](https://github.com/netdata/netdata/pull/15787) ([ktsaou](https://github.com/ktsaou))
- Set correct path for ansible-playbook in deployment tutorial [\#15786](https://github.com/netdata/netdata/pull/15786) ([novotnyJiri](https://github.com/novotnyJiri))
- minor Dyncfg mvp0 fixes [\#15785](https://github.com/netdata/netdata/pull/15785) ([underhood](https://github.com/underhood))
- fix docker-compose example [\#15784](https://github.com/netdata/netdata/pull/15784) ([zhqu1148980644](https://github.com/zhqu1148980644))
- mark integrations milestones as completed in README.md [\#15783](https://github.com/netdata/netdata/pull/15783) ([tkatsoulas](https://github.com/tkatsoulas))
- Update an oversight on the openSUSE 15.5 packages [\#15781](https://github.com/netdata/netdata/pull/15781) ([tkatsoulas](https://github.com/tkatsoulas))
- Bump openssl version of static builds to 1.1.1v [\#15779](https://github.com/netdata/netdata/pull/15779) ([tkatsoulas](https://github.com/tkatsoulas))
- fix: the cleanup was not performed during the kickstart.sh dry run [\#15775](https://github.com/netdata/netdata/pull/15775) ([ilyam8](https://github.com/ilyam8))
- don't return `-1` if the socket was closed [\#15771](https://github.com/netdata/netdata/pull/15771) ([moonbreon](https://github.com/moonbreon))
- Increase alert snapshot chunk size [\#15748](https://github.com/netdata/netdata/pull/15748) ([MrZammler](https://github.com/MrZammler))
- Added CentOS-Stream to distros [\#15742](https://github.com/netdata/netdata/pull/15742) ([k0ste](https://github.com/k0ste))
- Unconditionally delete very old models. [\#15720](https://github.com/netdata/netdata/pull/15720) ([vkalintiris](https://github.com/vkalintiris))
- Misc code cleanup [\#15665](https://github.com/netdata/netdata/pull/15665) ([stelfrag](https://github.com/stelfrag))
- Metadata cleanup improvements [\#15462](https://github.com/netdata/netdata/pull/15462) ([stelfrag](https://github.com/stelfrag))

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

**Merged pull requests:**

- ci: codacy exclude web/gui/v2/ [\#15780](https://github.com/netdata/netdata/pull/15780) ([ilyam8](https://github.com/ilyam8))
- update UI to v6.27.0 [\#15778](https://github.com/netdata/netdata/pull/15778) ([ilyam8](https://github.com/ilyam8))
- ci: fix labeler area/docs [\#15776](https://github.com/netdata/netdata/pull/15776) ([ilyam8](https://github.com/ilyam8))
- fix claiming via UI for static build [\#15774](https://github.com/netdata/netdata/pull/15774) ([ilyam8](https://github.com/ilyam8))
- extend the trimming window to avoid empty points at the end of queries [\#15773](https://github.com/netdata/netdata/pull/15773) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15772](https://github.com/netdata/netdata/pull/15772) ([netdatabot](https://github.com/netdatabot))
- Change FreeBSD / macOS system.swap\(io\) to mem.swap\(io\) [\#15769](https://github.com/netdata/netdata/pull/15769) ([Dim-P](https://github.com/Dim-P))
- update ui to v6.26.3 [\#15767](https://github.com/netdata/netdata/pull/15767) ([ilyam8](https://github.com/ilyam8))
- Fix CID 398318 [\#15766](https://github.com/netdata/netdata/pull/15766) ([underhood](https://github.com/underhood))
- Fix coverity issues introduced via drm proc module [\#15765](https://github.com/netdata/netdata/pull/15765) ([Dim-P](https://github.com/Dim-P))
- Regenerate integrations.js [\#15764](https://github.com/netdata/netdata/pull/15764) ([netdatabot](https://github.com/netdatabot))
- meta update proc drm icon [\#15763](https://github.com/netdata/netdata/pull/15763) ([ilyam8](https://github.com/ilyam8))
- Update metadata.yaml [\#15762](https://github.com/netdata/netdata/pull/15762) ([ktsaou](https://github.com/ktsaou))
- Update metadata.yaml [\#15761](https://github.com/netdata/netdata/pull/15761) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#15760](https://github.com/netdata/netdata/pull/15760) ([netdatabot](https://github.com/netdatabot))
- fix nvidia\_smi power\_readings for new drivers [\#15759](https://github.com/netdata/netdata/pull/15759) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v2.26.2 [\#15758](https://github.com/netdata/netdata/pull/15758) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#15751](https://github.com/netdata/netdata/pull/15751) ([netdatabot](https://github.com/netdatabot))
- ci labeler: remove integrations from area/docs [\#15750](https://github.com/netdata/netdata/pull/15750) ([ilyam8](https://github.com/ilyam8))
- meta: align left metrics, alerts, and config options [\#15749](https://github.com/netdata/netdata/pull/15749) ([ilyam8](https://github.com/ilyam8))
- Add dependencies for systemd journal plugin. [\#15747](https://github.com/netdata/netdata/pull/15747) ([Ferroin](https://github.com/Ferroin))
- prefer cap over setuid for sysetmd-journal in installer [\#15741](https://github.com/netdata/netdata/pull/15741) ([ilyam8](https://github.com/ilyam8))
- \[cloud-blocker\] https\_client add TLS ext. SNI + support chunked transfer encoding [\#15739](https://github.com/netdata/netdata/pull/15739) ([underhood](https://github.com/underhood))
- Don't overwrite my vscode settings! [\#15738](https://github.com/netdata/netdata/pull/15738) ([underhood](https://github.com/underhood))
- faster facets and journal fixes [\#15737](https://github.com/netdata/netdata/pull/15737) ([ktsaou](https://github.com/ktsaou))
- Adjust namespace used for sd\_journal\_open [\#15736](https://github.com/netdata/netdata/pull/15736) ([stelfrag](https://github.com/stelfrag))
- Update to latest copy of v2 dashboard. [\#15735](https://github.com/netdata/netdata/pull/15735) ([Ferroin](https://github.com/Ferroin))
- Add netdata-plugin-systemd-journal package. [\#15733](https://github.com/netdata/netdata/pull/15733) ([Ferroin](https://github.com/Ferroin))
- proc.plugin: dont log if pressure/irq does not exist [\#15732](https://github.com/netdata/netdata/pull/15732) ([ilyam8](https://github.com/ilyam8))
- ci: run "Generate Integrations" only in netdata/netdata [\#15731](https://github.com/netdata/netdata/pull/15731) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#15728](https://github.com/netdata/netdata/pull/15728) ([netdatabot](https://github.com/netdatabot))
- fix systemd-journal makefile [\#15727](https://github.com/netdata/netdata/pull/15727) ([ktsaou](https://github.com/ktsaou))
- disable systemdunits alarms [\#15726](https://github.com/netdata/netdata/pull/15726) ([ilyam8](https://github.com/ilyam8))
- Fix memory corruption [\#15724](https://github.com/netdata/netdata/pull/15724) ([stelfrag](https://github.com/stelfrag))
- Revert "Refactor RRD code. \(\#15423\)" [\#15723](https://github.com/netdata/netdata/pull/15723) ([vkalintiris](https://github.com/vkalintiris))
- Changes to the templates for integrations [\#15721](https://github.com/netdata/netdata/pull/15721) ([Ancairon](https://github.com/Ancairon))
- fix the freez pointer of dyncfg [\#15719](https://github.com/netdata/netdata/pull/15719) ([ktsaou](https://github.com/ktsaou))
- Update the bundled v2 dashboard to the latest release. [\#15718](https://github.com/netdata/netdata/pull/15718) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#15717](https://github.com/netdata/netdata/pull/15717) ([netdatabot](https://github.com/netdatabot))
- fix meta deploy docker swarm NC env var [\#15716](https://github.com/netdata/netdata/pull/15716) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#15713](https://github.com/netdata/netdata/pull/15713) ([netdatabot](https://github.com/netdatabot))
- Update metadata.yaml [\#15710](https://github.com/netdata/netdata/pull/15710) ([sashwathn](https://github.com/sashwathn))
- Regenerate integrations.js [\#15709](https://github.com/netdata/netdata/pull/15709) ([netdatabot](https://github.com/netdatabot))
- integrations: fix docker compose indent [\#15708](https://github.com/netdata/netdata/pull/15708) ([ilyam8](https://github.com/ilyam8))
- Better cleanup of aclk alert table entries [\#15706](https://github.com/netdata/netdata/pull/15706) ([MrZammler](https://github.com/MrZammler))
- Regenerate integrations.js [\#15705](https://github.com/netdata/netdata/pull/15705) ([netdatabot](https://github.com/netdatabot))
- Fix typo in categories for beanstalk collector metadata. [\#15703](https://github.com/netdata/netdata/pull/15703) ([Ferroin](https://github.com/Ferroin))
- Assorted fixes for integrations templates. [\#15702](https://github.com/netdata/netdata/pull/15702) ([Ferroin](https://github.com/Ferroin))
- integrations: fix metrics availability [\#15701](https://github.com/netdata/netdata/pull/15701) ([ilyam8](https://github.com/ilyam8))
- Fix handling of troubleshooting section in integrations. [\#15700](https://github.com/netdata/netdata/pull/15700) ([Ferroin](https://github.com/Ferroin))
- update vscode yaml schemas association [\#15697](https://github.com/netdata/netdata/pull/15697) ([ilyam8](https://github.com/ilyam8))
- Update categories.yaml [\#15696](https://github.com/netdata/netdata/pull/15696) ([sashwathn](https://github.com/sashwathn))
- Regenerate integrations.js [\#15695](https://github.com/netdata/netdata/pull/15695) ([netdatabot](https://github.com/netdatabot))
- Extend eBPF default shutdown [\#15694](https://github.com/netdata/netdata/pull/15694) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix integrations regen workflow [\#15693](https://github.com/netdata/netdata/pull/15693) ([Ferroin](https://github.com/Ferroin))
- bump go.d.plugin v0.54.1 [\#15692](https://github.com/netdata/netdata/pull/15692) ([ilyam8](https://github.com/ilyam8))
- Update names [\#15691](https://github.com/netdata/netdata/pull/15691) ([thiagoftsm](https://github.com/thiagoftsm))
- Update metadata.yaml [\#15690](https://github.com/netdata/netdata/pull/15690) ([sashwathn](https://github.com/sashwathn))
- Update categories.yaml [\#15689](https://github.com/netdata/netdata/pull/15689) ([sashwathn](https://github.com/sashwathn))
- Update metadata.yaml [\#15688](https://github.com/netdata/netdata/pull/15688) ([sashwathn](https://github.com/sashwathn))
- Update deploy.yaml [\#15687](https://github.com/netdata/netdata/pull/15687) ([sashwathn](https://github.com/sashwathn))
- Update categories.yaml [\#15686](https://github.com/netdata/netdata/pull/15686) ([sashwathn](https://github.com/sashwathn))
- Update categories.yaml [\#15685](https://github.com/netdata/netdata/pull/15685) ([sashwathn](https://github.com/sashwathn))
- Update metadata.yaml [\#15684](https://github.com/netdata/netdata/pull/15684) ([sashwathn](https://github.com/sashwathn))
- Update categories.yaml [\#15683](https://github.com/netdata/netdata/pull/15683) ([sashwathn](https://github.com/sashwathn))
- Update categories.yaml [\#15682](https://github.com/netdata/netdata/pull/15682) ([sashwathn](https://github.com/sashwathn))
- Update categories.yaml [\#15681](https://github.com/netdata/netdata/pull/15681) ([sashwathn](https://github.com/sashwathn))
- Update metadata.yaml [\#15680](https://github.com/netdata/netdata/pull/15680) ([sashwathn](https://github.com/sashwathn))
- Update metadata.yaml [\#15679](https://github.com/netdata/netdata/pull/15679) ([shyamvalsan](https://github.com/shyamvalsan))
- Update metadata.yaml [\#15678](https://github.com/netdata/netdata/pull/15678) ([sashwathn](https://github.com/sashwathn))
- Update Webhook icon [\#15677](https://github.com/netdata/netdata/pull/15677) ([sashwathn](https://github.com/sashwathn))
- Update deploy.yaml to fix Docker and Kubernetes commands [\#15676](https://github.com/netdata/netdata/pull/15676) ([sashwathn](https://github.com/sashwathn))
- meta MacOS =\> macOS [\#15675](https://github.com/netdata/netdata/pull/15675) ([ilyam8](https://github.com/ilyam8))
- Adapt Cloud notifications to the new schema [\#15674](https://github.com/netdata/netdata/pull/15674) ([sashwathn](https://github.com/sashwathn))
- Fix formatting [\#15673](https://github.com/netdata/netdata/pull/15673) ([shyamvalsan](https://github.com/shyamvalsan))
- Fixing tables \(aws sns\) [\#15671](https://github.com/netdata/netdata/pull/15671) ([shyamvalsan](https://github.com/shyamvalsan))
- Update metadata.yaml for Cloud Notifications [\#15670](https://github.com/netdata/netdata/pull/15670) ([sashwathn](https://github.com/sashwathn))
- remove " Metrics" from linux categories [\#15669](https://github.com/netdata/netdata/pull/15669) ([ilyam8](https://github.com/ilyam8))
- Fix table formatting \(custom exporter\) [\#15668](https://github.com/netdata/netdata/pull/15668) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix icon prometheus exporter icon [\#15666](https://github.com/netdata/netdata/pull/15666) ([hugovalente-pm](https://github.com/hugovalente-pm))
- freeipmi change restart message to info [\#15664](https://github.com/netdata/netdata/pull/15664) ([ilyam8](https://github.com/ilyam8))
- fix proc.plugin meta filename [\#15659](https://github.com/netdata/netdata/pull/15659) ([ilyam8](https://github.com/ilyam8))
- small improvements to README.md [\#15658](https://github.com/netdata/netdata/pull/15658) ([ilyam8](https://github.com/ilyam8))
- Fix icon for solarwinds [\#15657](https://github.com/netdata/netdata/pull/15657) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Fix Apps plugin icons [\#15655](https://github.com/netdata/netdata/pull/15655) ([hugovalente-pm](https://github.com/hugovalente-pm))
- fix pandas category [\#15654](https://github.com/netdata/netdata/pull/15654) ([andrewm4894](https://github.com/andrewm4894))
- Fix exporter icons [\#15652](https://github.com/netdata/netdata/pull/15652) ([shyamvalsan](https://github.com/shyamvalsan))
- disable freeipmi in docker by default [\#15651](https://github.com/netdata/netdata/pull/15651) ([ilyam8](https://github.com/ilyam8))
- Fixing FreeBSD icons [\#15650](https://github.com/netdata/netdata/pull/15650) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix exporter schema to support multiple entries per file. [\#15649](https://github.com/netdata/netdata/pull/15649) ([Ferroin](https://github.com/Ferroin))
- Fixing icons in netdata/netdata repo [\#15647](https://github.com/netdata/netdata/pull/15647) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix name in the yaml of example python collector [\#15646](https://github.com/netdata/netdata/pull/15646) ([Ancairon](https://github.com/Ancairon))
- Fix icons [\#15645](https://github.com/netdata/netdata/pull/15645) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Fix icons for notifications [\#15644](https://github.com/netdata/netdata/pull/15644) ([shyamvalsan](https://github.com/shyamvalsan))
- convert collectors meta files from single to multi [\#15642](https://github.com/netdata/netdata/pull/15642) ([ilyam8](https://github.com/ilyam8))
- fix edit-config for containerized Netdata when running from host [\#15641](https://github.com/netdata/netdata/pull/15641) ([ilyam8](https://github.com/ilyam8))
- fix: 🐛 docker bind-mount stock files creation [\#15639](https://github.com/netdata/netdata/pull/15639) ([Leny1996](https://github.com/Leny1996))
- The icon\_filename value was not in quotes - Fixed [\#15635](https://github.com/netdata/netdata/pull/15635) ([sashwathn](https://github.com/sashwathn))
- Update graphite metadata.yaml [\#15634](https://github.com/netdata/netdata/pull/15634) ([shyamvalsan](https://github.com/shyamvalsan))
- Debugfs yaml update [\#15633](https://github.com/netdata/netdata/pull/15633) ([thiagoftsm](https://github.com/thiagoftsm))
- Update metadata.yaml [\#15632](https://github.com/netdata/netdata/pull/15632) ([shyamvalsan](https://github.com/shyamvalsan))
- review images for integrations from security to windows systems [\#15630](https://github.com/netdata/netdata/pull/15630) ([hugovalente-pm](https://github.com/hugovalente-pm))
- bump ui to v6.23.0 [\#15629](https://github.com/netdata/netdata/pull/15629) ([ilyam8](https://github.com/ilyam8))
- Updated Cloud Notification Integrations with the new schema [\#15628](https://github.com/netdata/netdata/pull/15628) ([sashwathn](https://github.com/sashwathn))
- Add additional variable section to instance data in schema. [\#15627](https://github.com/netdata/netdata/pull/15627) ([Ferroin](https://github.com/Ferroin))
- fix icons for message brokers and hardware [\#15626](https://github.com/netdata/netdata/pull/15626) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Add key for notifications to control what global config options get displayed [\#15625](https://github.com/netdata/netdata/pull/15625) ([Ferroin](https://github.com/Ferroin))
- fix icons for webservers integrations [\#15624](https://github.com/netdata/netdata/pull/15624) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Add notification metadata for agent notifications [\#15622](https://github.com/netdata/netdata/pull/15622) ([shyamvalsan](https://github.com/shyamvalsan))
- fix icons for db integrations [\#15621](https://github.com/netdata/netdata/pull/15621) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Rename multi\_metadata.yaml to metadata.yaml [\#15619](https://github.com/netdata/netdata/pull/15619) ([shyamvalsan](https://github.com/shyamvalsan))
- Rename multi\_metadata.yaml to metadata.yaml [\#15618](https://github.com/netdata/netdata/pull/15618) ([shyamvalsan](https://github.com/shyamvalsan))
- Fix up notification schema to better support cloud notifications. [\#15616](https://github.com/netdata/netdata/pull/15616) ([Ferroin](https://github.com/Ferroin))
- Updated all cloud notifications except generic webhook [\#15615](https://github.com/netdata/netdata/pull/15615) ([sashwathn](https://github.com/sashwathn))
- prefer titles, families, units and priorities from collected charts [\#15614](https://github.com/netdata/netdata/pull/15614) ([ktsaou](https://github.com/ktsaou))
- Update categories.yaml to add notifications [\#15613](https://github.com/netdata/netdata/pull/15613) ([sashwathn](https://github.com/sashwathn))
- ci disable yamllint line-length check [\#15612](https://github.com/netdata/netdata/pull/15612) ([ilyam8](https://github.com/ilyam8))
- Fix descriptions in config objects, make them single line [\#15610](https://github.com/netdata/netdata/pull/15610) ([Ancairon](https://github.com/Ancairon))
- Update icons [\#15609](https://github.com/netdata/netdata/pull/15609) ([shyamvalsan](https://github.com/shyamvalsan))
- Update icon [\#15608](https://github.com/netdata/netdata/pull/15608) ([shyamvalsan](https://github.com/shyamvalsan))
- Update icon [\#15607](https://github.com/netdata/netdata/pull/15607) ([shyamvalsan](https://github.com/shyamvalsan))
- Update documentation [\#15606](https://github.com/netdata/netdata/pull/15606) ([kiela](https://github.com/kiela))
- fix potential crash bug.  [\#15605](https://github.com/netdata/netdata/pull/15605) ([icy17](https://github.com/icy17))
- FreeBSD yaml update [\#15603](https://github.com/netdata/netdata/pull/15603) ([thiagoftsm](https://github.com/thiagoftsm))
- Macos yaml update [\#15602](https://github.com/netdata/netdata/pull/15602) ([thiagoftsm](https://github.com/thiagoftsm))
- minor changes in README.md [\#15601](https://github.com/netdata/netdata/pull/15601) ([tkatsoulas](https://github.com/tkatsoulas))
- reviewed icos for a bunch of integrations [\#15599](https://github.com/netdata/netdata/pull/15599) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Sample Cloud Notifications metadata for Discord [\#15597](https://github.com/netdata/netdata/pull/15597) ([sashwathn](https://github.com/sashwathn))
- Updated icons in deploy section [\#15596](https://github.com/netdata/netdata/pull/15596) ([shyamvalsan](https://github.com/shyamvalsan))
- 10 points per query min [\#15595](https://github.com/netdata/netdata/pull/15595) ([ktsaou](https://github.com/ktsaou))
- CUPS yaml update [\#15594](https://github.com/netdata/netdata/pull/15594) ([thiagoftsm](https://github.com/thiagoftsm))
- remove metrics.csv files [\#15593](https://github.com/netdata/netdata/pull/15593) ([ilyam8](https://github.com/ilyam8))
- fix tomcat meta [\#15592](https://github.com/netdata/netdata/pull/15592) ([ilyam8](https://github.com/ilyam8))
- Added a sample metadata.yaml for Alerta [\#15591](https://github.com/netdata/netdata/pull/15591) ([sashwathn](https://github.com/sashwathn))
- remove the noise by silencing alerts that dont need to wake up people [\#15590](https://github.com/netdata/netdata/pull/15590) ([ktsaou](https://github.com/ktsaou))
- Fix health query [\#15589](https://github.com/netdata/netdata/pull/15589) ([stelfrag](https://github.com/stelfrag))
- Fix typo in notification schema. [\#15588](https://github.com/netdata/netdata/pull/15588) ([Ferroin](https://github.com/Ferroin))
- Update icons for relevant integrations in proc.plugin [\#15587](https://github.com/netdata/netdata/pull/15587) ([sashwathn](https://github.com/sashwathn))
- Update icon for power supply [\#15586](https://github.com/netdata/netdata/pull/15586) ([sashwathn](https://github.com/sashwathn))
- Update Slabinfo Logo [\#15585](https://github.com/netdata/netdata/pull/15585) ([sashwathn](https://github.com/sashwathn))
- fix cpu MHz from /proc/cpuinfo [\#15584](https://github.com/netdata/netdata/pull/15584) ([ilyam8](https://github.com/ilyam8))
- small readme icon fix [\#15583](https://github.com/netdata/netdata/pull/15583) ([andrewm4894](https://github.com/andrewm4894))
- update pandas collector metadata [\#15582](https://github.com/netdata/netdata/pull/15582) ([andrewm4894](https://github.com/andrewm4894))
- Update zscores metadata yaml [\#15581](https://github.com/netdata/netdata/pull/15581) ([andrewm4894](https://github.com/andrewm4894))
- Create metadata.yaml for MongoDB exporter [\#15580](https://github.com/netdata/netdata/pull/15580) ([shyamvalsan](https://github.com/shyamvalsan))
- Create metadata.yaml for JSON exporter [\#15579](https://github.com/netdata/netdata/pull/15579) ([shyamvalsan](https://github.com/shyamvalsan))
- Create metadata.yaml for Google PubSub exporter [\#15578](https://github.com/netdata/netdata/pull/15578) ([shyamvalsan](https://github.com/shyamvalsan))
- Create metadata.yaml for AWS kinesis exporter [\#15577](https://github.com/netdata/netdata/pull/15577) ([shyamvalsan](https://github.com/shyamvalsan))
- Create multi\_metadata.yaml for graphite exporters [\#15576](https://github.com/netdata/netdata/pull/15576) ([shyamvalsan](https://github.com/shyamvalsan))
- Create multi\_metadata.yaml [\#15575](https://github.com/netdata/netdata/pull/15575) ([shyamvalsan](https://github.com/shyamvalsan))
- Add missing file in CMakeLists.txt [\#15574](https://github.com/netdata/netdata/pull/15574) ([stelfrag](https://github.com/stelfrag))
- comment out anomalies metadata and add note [\#15573](https://github.com/netdata/netdata/pull/15573) ([andrewm4894](https://github.com/andrewm4894))
- Fixed deployment commands for Docker, Kubernetes and Linux [\#15572](https://github.com/netdata/netdata/pull/15572) ([sashwathn](https://github.com/sashwathn))
- filter out systemd-udevd.service/udevd [\#15571](https://github.com/netdata/netdata/pull/15571) ([ilyam8](https://github.com/ilyam8))
- Added FreeBSD integration and fixed Windows installation Steps [\#15570](https://github.com/netdata/netdata/pull/15570) ([sashwathn](https://github.com/sashwathn))
- fix schema validation for some meta files [\#15569](https://github.com/netdata/netdata/pull/15569) ([ilyam8](https://github.com/ilyam8))
- Drop duplicate / unused index [\#15568](https://github.com/netdata/netdata/pull/15568) ([stelfrag](https://github.com/stelfrag))
- Xen yaml update [\#15567](https://github.com/netdata/netdata/pull/15567) ([thiagoftsm](https://github.com/thiagoftsm))
- Timex yaml update [\#15565](https://github.com/netdata/netdata/pull/15565) ([thiagoftsm](https://github.com/thiagoftsm))
- Create metadata.yaml for OpenTSDB Exporter [\#15563](https://github.com/netdata/netdata/pull/15563) ([shyamvalsan](https://github.com/shyamvalsan))
- TC yaml update [\#15562](https://github.com/netdata/netdata/pull/15562) ([thiagoftsm](https://github.com/thiagoftsm))
- Added Exporter and Notifications categories and removed them from Data Collection [\#15561](https://github.com/netdata/netdata/pull/15561) ([sashwathn](https://github.com/sashwathn))
- Update slabinfo yaml [\#15560](https://github.com/netdata/netdata/pull/15560) ([thiagoftsm](https://github.com/thiagoftsm))
- Update metadata.yaml for charts.d collectors [\#15559](https://github.com/netdata/netdata/pull/15559) ([MrZammler](https://github.com/MrZammler))
- Perf yaml [\#15558](https://github.com/netdata/netdata/pull/15558) ([thiagoftsm](https://github.com/thiagoftsm))
- detect the path the netdata-claim.sh script is in [\#15556](https://github.com/netdata/netdata/pull/15556) ([ktsaou](https://github.com/ktsaou))
- Fixed typos in code blocks and added missing icons [\#15555](https://github.com/netdata/netdata/pull/15555) ([sashwathn](https://github.com/sashwathn))
- Remove temporarily from the CI Tumbleweed support [\#15554](https://github.com/netdata/netdata/pull/15554) ([tkatsoulas](https://github.com/tkatsoulas))
- fix ebpf.plugin system swapcalls [\#15553](https://github.com/netdata/netdata/pull/15553) ([ilyam8](https://github.com/ilyam8))
- Fixes for `deploy.yaml`. [\#15551](https://github.com/netdata/netdata/pull/15551) ([Ferroin](https://github.com/Ferroin))
- bump ui to v6.22.1 [\#15550](https://github.com/netdata/netdata/pull/15550) ([ilyam8](https://github.com/ilyam8))
- Add schema and examples for notification method metadata. [\#15549](https://github.com/netdata/netdata/pull/15549) ([Ferroin](https://github.com/Ferroin))
- Update python sensors metadata yaml [\#15548](https://github.com/netdata/netdata/pull/15548) ([andrewm4894](https://github.com/andrewm4894))
- fix yamls [\#15547](https://github.com/netdata/netdata/pull/15547) ([Ancairon](https://github.com/Ancairon))
- fix expiration dates for API responses [\#15546](https://github.com/netdata/netdata/pull/15546) ([ktsaou](https://github.com/ktsaou))
- Add exporter integration schema. [\#15545](https://github.com/netdata/netdata/pull/15545) ([Ferroin](https://github.com/Ferroin))
- postfix metadata.yaml - add links and some descriptions [\#15544](https://github.com/netdata/netdata/pull/15544) ([andrewm4894](https://github.com/andrewm4894))
- Update metadata for multiple python collectors. [\#15543](https://github.com/netdata/netdata/pull/15543) ([tkatsoulas](https://github.com/tkatsoulas))
- bump ui to v6.22.0 [\#15542](https://github.com/netdata/netdata/pull/15542) ([ilyam8](https://github.com/ilyam8))
- Fill in yaml files for some python collectors [\#15541](https://github.com/netdata/netdata/pull/15541) ([Ancairon](https://github.com/Ancairon))
- Fix deployment and categories [\#15540](https://github.com/netdata/netdata/pull/15540) ([sashwathn](https://github.com/sashwathn))
- docs: fix apps fd badges and typos [\#15539](https://github.com/netdata/netdata/pull/15539) ([ilyam8](https://github.com/ilyam8))
- change api.netdata.cloud to app.netdata.cloud [\#15538](https://github.com/netdata/netdata/pull/15538) ([ilyam8](https://github.com/ilyam8))
- Update metadata.yaml for some python collectors - 2 [\#15537](https://github.com/netdata/netdata/pull/15537) ([MrZammler](https://github.com/MrZammler))
- Change nvidia\_smi link to go version in COLLECTORS.md [\#15536](https://github.com/netdata/netdata/pull/15536) ([Ancairon](https://github.com/Ancairon))
- Update nfacct yaml [\#15535](https://github.com/netdata/netdata/pull/15535) ([thiagoftsm](https://github.com/thiagoftsm))
- Update ioping yaml [\#15534](https://github.com/netdata/netdata/pull/15534) ([thiagoftsm](https://github.com/thiagoftsm))
- Freeimpi yaml [\#15533](https://github.com/netdata/netdata/pull/15533) ([thiagoftsm](https://github.com/thiagoftsm))
- Updated all Linux distros, macOS and Docker [\#15532](https://github.com/netdata/netdata/pull/15532) ([sashwathn](https://github.com/sashwathn))
- Update platform support info and add a schema. [\#15531](https://github.com/netdata/netdata/pull/15531) ([Ferroin](https://github.com/Ferroin))
- added cloud status in registry?action=hello [\#15530](https://github.com/netdata/netdata/pull/15530) ([ktsaou](https://github.com/ktsaou))
- update memcached metadata.yaml [\#15529](https://github.com/netdata/netdata/pull/15529) ([andrewm4894](https://github.com/andrewm4894))
- Update python d varnish metadata [\#15528](https://github.com/netdata/netdata/pull/15528) ([andrewm4894](https://github.com/andrewm4894))
- Update yaml description \(diskspace\) [\#15527](https://github.com/netdata/netdata/pull/15527) ([thiagoftsm](https://github.com/thiagoftsm))
- wait for node\_id while claiming [\#15526](https://github.com/netdata/netdata/pull/15526) ([ktsaou](https://github.com/ktsaou))
- add `diskquota` collector to third party collectors list [\#15524](https://github.com/netdata/netdata/pull/15524) ([andrewm4894](https://github.com/andrewm4894))
- Add quick\_start key to deploy schema. [\#15522](https://github.com/netdata/netdata/pull/15522) ([Ferroin](https://github.com/Ferroin))
- Add a schema for the categories.yaml file. [\#15521](https://github.com/netdata/netdata/pull/15521) ([Ferroin](https://github.com/Ferroin))
- fix collector multi schema [\#15520](https://github.com/netdata/netdata/pull/15520) ([ilyam8](https://github.com/ilyam8))
- Allow to create alert hashes with --disable-cloud [\#15519](https://github.com/netdata/netdata/pull/15519) ([MrZammler](https://github.com/MrZammler))
- Python collector yaml updates [\#15517](https://github.com/netdata/netdata/pull/15517) ([Ancairon](https://github.com/Ancairon))
- eBPF Yaml complement [\#15516](https://github.com/netdata/netdata/pull/15516) ([thiagoftsm](https://github.com/thiagoftsm))
- Add AMD GPU collector  [\#15515](https://github.com/netdata/netdata/pull/15515) ([Dim-P](https://github.com/Dim-P))
- Update metadata.yaml for some python collectors [\#15513](https://github.com/netdata/netdata/pull/15513) ([MrZammler](https://github.com/MrZammler))
- Update metadata.yaml for some python collectors [\#15510](https://github.com/netdata/netdata/pull/15510) ([andrewm4894](https://github.com/andrewm4894))
- Add schema for deployment integrations and centralize integrations schemas. [\#15509](https://github.com/netdata/netdata/pull/15509) ([Ferroin](https://github.com/Ferroin))
- update gitignore to include vscode settings for schema validation [\#15508](https://github.com/netdata/netdata/pull/15508) ([andrewm4894](https://github.com/andrewm4894))
- Add Samba collector yaml [\#15507](https://github.com/netdata/netdata/pull/15507) ([Ancairon](https://github.com/Ancairon))
- Fill in metadata for idlejitter plugin. [\#15506](https://github.com/netdata/netdata/pull/15506) ([Ferroin](https://github.com/Ferroin))
- apps.plugin limits tracing [\#15504](https://github.com/netdata/netdata/pull/15504) ([ktsaou](https://github.com/ktsaou))
- Allow manage/health api call to be used without bearer [\#15503](https://github.com/netdata/netdata/pull/15503) ([MrZammler](https://github.com/MrZammler))
- Avoid an extra uuid\_copy when creating new MRG entries [\#15502](https://github.com/netdata/netdata/pull/15502) ([stelfrag](https://github.com/stelfrag))
- freeipmi flush keepalive msgs [\#15499](https://github.com/netdata/netdata/pull/15499) ([ilyam8](https://github.com/ilyam8))
- add required properties to multi-module schema [\#15496](https://github.com/netdata/netdata/pull/15496) ([ilyam8](https://github.com/ilyam8))
- proc integrations [\#15494](https://github.com/netdata/netdata/pull/15494) ([ktsaou](https://github.com/ktsaou))
- docs: clarify health percentage option [\#15492](https://github.com/netdata/netdata/pull/15492) ([ilyam8](https://github.com/ilyam8))
- Fix resource leak - CID 396310 [\#15491](https://github.com/netdata/netdata/pull/15491) ([stelfrag](https://github.com/stelfrag))
- Improve the update of the alert chart name in the database [\#15490](https://github.com/netdata/netdata/pull/15490) ([stelfrag](https://github.com/stelfrag))
- PCI Advanced Error Reporting \(AER\) [\#15488](https://github.com/netdata/netdata/pull/15488) ([ktsaou](https://github.com/ktsaou))
- Dynamic Config MVP0 [\#15486](https://github.com/netdata/netdata/pull/15486) ([underhood](https://github.com/underhood))
- Add a machine distinct id to analytics [\#15485](https://github.com/netdata/netdata/pull/15485) ([MrZammler](https://github.com/MrZammler))
- Add basic slabinfo metadata. [\#15484](https://github.com/netdata/netdata/pull/15484) ([Ferroin](https://github.com/Ferroin))
- Update charts.d.plugin yaml [\#15483](https://github.com/netdata/netdata/pull/15483) ([Ancairon](https://github.com/Ancairon))
- Make title reflect legacy agent dashboard [\#15479](https://github.com/netdata/netdata/pull/15479) ([Ancairon](https://github.com/Ancairon))
- docs: note that health foreach works only with template [\#15478](https://github.com/netdata/netdata/pull/15478) ([ilyam8](https://github.com/ilyam8))
- Yaml file updates [\#15477](https://github.com/netdata/netdata/pull/15477) ([Ancairon](https://github.com/Ancairon))
- Rename most-popular to most\_popular in categories.yaml [\#15476](https://github.com/netdata/netdata/pull/15476) ([Ancairon](https://github.com/Ancairon))
- Fix coverity issue [\#15475](https://github.com/netdata/netdata/pull/15475) ([stelfrag](https://github.com/stelfrag))
- eBPF Yaml [\#15474](https://github.com/netdata/netdata/pull/15474) ([thiagoftsm](https://github.com/thiagoftsm))
- Memory Controller \(MC\) and DIMM Error Detection And Correction \(EDAC\) [\#15473](https://github.com/netdata/netdata/pull/15473) ([ktsaou](https://github.com/ktsaou))
- meta schema change multi-instance to multi\_instance [\#15470](https://github.com/netdata/netdata/pull/15470) ([ilyam8](https://github.com/ilyam8))
- fix anchors [\#15469](https://github.com/netdata/netdata/pull/15469) ([Ancairon](https://github.com/Ancairon))
- fix the calculation of incremental-sum [\#15468](https://github.com/netdata/netdata/pull/15468) ([ktsaou](https://github.com/ktsaou))
- apps.plugin fds limits improvements [\#15467](https://github.com/netdata/netdata/pull/15467) ([ktsaou](https://github.com/ktsaou))
- Add community key in schema [\#15465](https://github.com/netdata/netdata/pull/15465) ([Ancairon](https://github.com/Ancairon))
- Overhaul deployment strategies documentation [\#15464](https://github.com/netdata/netdata/pull/15464) ([ralphm](https://github.com/ralphm))
- Update debugfs plugin metadata. [\#15463](https://github.com/netdata/netdata/pull/15463) ([Ferroin](https://github.com/Ferroin))
- Update proc plugin yaml [\#15460](https://github.com/netdata/netdata/pull/15460) ([Ancairon](https://github.com/Ancairon))
- Macos yaml updates [\#15459](https://github.com/netdata/netdata/pull/15459) ([Ancairon](https://github.com/Ancairon))
- Freeipmi yaml updates [\#15458](https://github.com/netdata/netdata/pull/15458) ([Ancairon](https://github.com/Ancairon))
- Add short descriptions to cgroups yaml [\#15457](https://github.com/netdata/netdata/pull/15457) ([Ancairon](https://github.com/Ancairon))
- readme: reorder cols in whats new and add links [\#15455](https://github.com/netdata/netdata/pull/15455) ([andrewm4894](https://github.com/andrewm4894))

## [v1.41.0](https://github.com/netdata/netdata/tree/v1.41.0) (2023-07-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.40.1...v1.41.0)

**Merged pull requests:**

- Include license for web v2 [\#15453](https://github.com/netdata/netdata/pull/15453) ([tkatsoulas](https://github.com/tkatsoulas))
- Updates to metadata.yaml [\#15452](https://github.com/netdata/netdata/pull/15452) ([shyamvalsan](https://github.com/shyamvalsan))
- Add apps yaml [\#15451](https://github.com/netdata/netdata/pull/15451) ([Ancairon](https://github.com/Ancairon))
- Add cgroups yaml [\#15450](https://github.com/netdata/netdata/pull/15450) ([Ancairon](https://github.com/Ancairon))
- Fix multiline [\#15449](https://github.com/netdata/netdata/pull/15449) ([Ancairon](https://github.com/Ancairon))
- bump v2 dashboard to v6.21.3 [\#15448](https://github.com/netdata/netdata/pull/15448) ([ilyam8](https://github.com/ilyam8))

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
