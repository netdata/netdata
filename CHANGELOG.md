# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.42.0...HEAD)

**Merged pull requests:**

- mark integrations milestones as completed in README.md [\#15783](https://github.com/netdata/netdata/pull/15783) ([tkatsoulas](https://github.com/tkatsoulas))
- Update an oversight on the openSUSE 15.5 packages [\#15781](https://github.com/netdata/netdata/pull/15781) ([tkatsoulas](https://github.com/tkatsoulas))

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
- Store and transmit chart\_name to cloud in alert events [\#15441](https://github.com/netdata/netdata/pull/15441) ([MrZammler](https://github.com/MrZammler))
- Refactor RRD code. [\#15423](https://github.com/netdata/netdata/pull/15423) ([vkalintiris](https://github.com/vkalintiris))
- Add initial tooling for generating integrations.js file. [\#15406](https://github.com/netdata/netdata/pull/15406) ([Ferroin](https://github.com/Ferroin))
- Add linux powercap metrics collector [\#15364](https://github.com/netdata/netdata/pull/15364) ([fhriley](https://github.com/fhriley))
- systemd-journal plugin [\#15363](https://github.com/netdata/netdata/pull/15363) ([ktsaou](https://github.com/ktsaou))
- Hash table charts [\#15323](https://github.com/netdata/netdata/pull/15323) ([thiagoftsm](https://github.com/thiagoftsm))
- Drop support for native packages of Ubuntu 22.10 [\#15292](https://github.com/netdata/netdata/pull/15292) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix non-interactive options for apt-get and zypper. [\#15288](https://github.com/netdata/netdata/pull/15288) ([zeylos](https://github.com/zeylos))

## [v1.41.0](https://github.com/netdata/netdata/tree/v1.41.0) (2023-07-19)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.40.1...v1.41.0)

**Merged pull requests:**

- Include license for web v2 [\#15453](https://github.com/netdata/netdata/pull/15453) ([tkatsoulas](https://github.com/tkatsoulas))
- Updates to metadata.yaml [\#15452](https://github.com/netdata/netdata/pull/15452) ([shyamvalsan](https://github.com/shyamvalsan))
- Add apps yaml [\#15451](https://github.com/netdata/netdata/pull/15451) ([Ancairon](https://github.com/Ancairon))
- Add cgroups yaml [\#15450](https://github.com/netdata/netdata/pull/15450) ([Ancairon](https://github.com/Ancairon))
- Fix multiline [\#15449](https://github.com/netdata/netdata/pull/15449) ([Ancairon](https://github.com/Ancairon))
- bump v2 dashboard to v6.21.3 [\#15448](https://github.com/netdata/netdata/pull/15448) ([ilyam8](https://github.com/ilyam8))
- fix alerts transitions search when something specific is asked for [\#15447](https://github.com/netdata/netdata/pull/15447) ([ktsaou](https://github.com/ktsaou))
- collector meta: remove meta.alternative\_monitored\_instances [\#15445](https://github.com/netdata/netdata/pull/15445) ([ilyam8](https://github.com/ilyam8))
- added missing fields to alerts instances [\#15442](https://github.com/netdata/netdata/pull/15442) ([ktsaou](https://github.com/ktsaou))
- removed dup categories [\#15440](https://github.com/netdata/netdata/pull/15440) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Create netdata-assistant docs [\#15438](https://github.com/netdata/netdata/pull/15438) ([shyamvalsan](https://github.com/shyamvalsan))
- apps.plugin fds limits improvements [\#15437](https://github.com/netdata/netdata/pull/15437) ([ktsaou](https://github.com/ktsaou))
- disable apps\_group\_file\_descriptors\_utilization alarm [\#15435](https://github.com/netdata/netdata/pull/15435) ([ilyam8](https://github.com/ilyam8))
- Add catch-all category entry in categories.yaml [\#15434](https://github.com/netdata/netdata/pull/15434) ([Ancairon](https://github.com/Ancairon))
- Update CODEOWNERS [\#15433](https://github.com/netdata/netdata/pull/15433) ([andrewm4894](https://github.com/andrewm4894))
- Remove duplicate category from categories.yaml [\#15432](https://github.com/netdata/netdata/pull/15432) ([Ancairon](https://github.com/Ancairon))
- readme: add link for netdata cloud and sign-in cta [\#15431](https://github.com/netdata/netdata/pull/15431) ([andrewm4894](https://github.com/andrewm4894))
- add chart id and name to alert instances and transitions [\#15430](https://github.com/netdata/netdata/pull/15430) ([ktsaou](https://github.com/ktsaou))
- update v2 dashboard [\#15427](https://github.com/netdata/netdata/pull/15427) ([ilyam8](https://github.com/ilyam8))
- fix unlocked registry access and add hostname to search response [\#15426](https://github.com/netdata/netdata/pull/15426) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#15424](https://github.com/netdata/netdata/pull/15424) ([christophidesp](https://github.com/christophidesp))
- Decode url before checking for question mark [\#15422](https://github.com/netdata/netdata/pull/15422) ([MrZammler](https://github.com/MrZammler))
- use real-time clock for http response headers [\#15421](https://github.com/netdata/netdata/pull/15421) ([ktsaou](https://github.com/ktsaou))
- Bugfix on alerts generation for yamls [\#15420](https://github.com/netdata/netdata/pull/15420) ([Ancairon](https://github.com/Ancairon))
- Minor typo fix on consul.conf [\#15419](https://github.com/netdata/netdata/pull/15419) ([Ancairon](https://github.com/Ancairon))
- monitor applications file descriptor limits [\#15417](https://github.com/netdata/netdata/pull/15417) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#15416](https://github.com/netdata/netdata/pull/15416) ([ktsaou](https://github.com/ktsaou))
- Update README.md [\#15414](https://github.com/netdata/netdata/pull/15414) ([ktsaou](https://github.com/ktsaou))
- collector meta: restrict chart\_type to known values [\#15413](https://github.com/netdata/netdata/pull/15413) ([ilyam8](https://github.com/ilyam8))
- Update README.md [\#15412](https://github.com/netdata/netdata/pull/15412) ([tkatsoulas](https://github.com/tkatsoulas))
- add reference to cncf [\#15408](https://github.com/netdata/netdata/pull/15408) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Make skipped CI run even faster. [\#15407](https://github.com/netdata/netdata/pull/15407) ([Ferroin](https://github.com/Ferroin))
- Pre release fixes [\#15405](https://github.com/netdata/netdata/pull/15405) ([ktsaou](https://github.com/ktsaou))
- Hide eBPF functions [\#15404](https://github.com/netdata/netdata/pull/15404) ([thiagoftsm](https://github.com/thiagoftsm))
- remove collector meta definitions [\#15403](https://github.com/netdata/netdata/pull/15403) ([ilyam8](https://github.com/ilyam8))
- bump v2 dashboard to latest prod [\#15402](https://github.com/netdata/netdata/pull/15402) ([ilyam8](https://github.com/ilyam8))
- Make yamls pass the schema, and use decided temporary naming scheme [\#15401](https://github.com/netdata/netdata/pull/15401) ([Ancairon](https://github.com/Ancairon))
- collector meta schema: global config examples folding + per example [\#15398](https://github.com/netdata/netdata/pull/15398) ([ilyam8](https://github.com/ilyam8))
- packaging: fix arch detection in update\_static [\#15396](https://github.com/netdata/netdata/pull/15396) ([ilyam8](https://github.com/ilyam8))
- add expiration to bearer token response [\#15392](https://github.com/netdata/netdata/pull/15392) ([ktsaou](https://github.com/ktsaou))
- dont add all nodes to registry action hello [\#15390](https://github.com/netdata/netdata/pull/15390) ([ktsaou](https://github.com/ktsaou))
- Revert "dont add all nodes to registry action hello" [\#15389](https://github.com/netdata/netdata/pull/15389) ([ktsaou](https://github.com/ktsaou))
- dont add all nodes to registry action hello [\#15388](https://github.com/netdata/netdata/pull/15388) ([ktsaou](https://github.com/ktsaou))
- update bundled v2 dashboard; make v2 the default dashboard [\#15386](https://github.com/netdata/netdata/pull/15386) ([ilyam8](https://github.com/ilyam8))
- Create categories.yaml [\#15385](https://github.com/netdata/netdata/pull/15385) ([Ancairon](https://github.com/Ancairon))
- Fix CodeQL alert  [\#15384](https://github.com/netdata/netdata/pull/15384) ([stelfrag](https://github.com/stelfrag))
- Add missing files to web/gui/Makefile.am. [\#15383](https://github.com/netdata/netdata/pull/15383) ([Ferroin](https://github.com/Ferroin))
- Updates on JSON schemas [\#15382](https://github.com/netdata/netdata/pull/15382) ([Ancairon](https://github.com/Ancairon))
- Build optimizations [\#15381](https://github.com/netdata/netdata/pull/15381) ([tkatsoulas](https://github.com/tkatsoulas))
- update http response code descriptions [\#15379](https://github.com/netdata/netdata/pull/15379) ([ktsaou](https://github.com/ktsaou))
- Suppress H2O compilation warnings [\#15378](https://github.com/netdata/netdata/pull/15378) ([stelfrag](https://github.com/stelfrag))
- update bundled v2 dashboard [\#15377](https://github.com/netdata/netdata/pull/15377) ([ilyam8](https://github.com/ilyam8))
- health: fix windows alarms for vnodes [\#15376](https://github.com/netdata/netdata/pull/15376) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#15375](https://github.com/netdata/netdata/pull/15375) ([stelfrag](https://github.com/stelfrag))
- Update bundled v2 dashboard. [\#15374](https://github.com/netdata/netdata/pull/15374) ([Ferroin](https://github.com/Ferroin))
- Update libbpf version \(1.2.2\) [\#15373](https://github.com/netdata/netdata/pull/15373) ([thiagoftsm](https://github.com/thiagoftsm))
- simplify collector schema by moving some props under meta [\#15372](https://github.com/netdata/netdata/pull/15372) ([ilyam8](https://github.com/ilyam8))
- dont log error on opening .environment [\#15371](https://github.com/netdata/netdata/pull/15371) ([ilyam8](https://github.com/ilyam8))
- Add most-popular entry in oneOf of categories in definitions.json [\#15370](https://github.com/netdata/netdata/pull/15370) ([Ancairon](https://github.com/Ancairon))
- Rename log\_access and log\_health [\#15368](https://github.com/netdata/netdata/pull/15368) ([MrZammler](https://github.com/MrZammler))
- move not really related props single-module.json -\> definitions.json [\#15366](https://github.com/netdata/netdata/pull/15366) ([ilyam8](https://github.com/ilyam8))
- Add keys to integrations schema, categories, icon path, plus some fixes [\#15365](https://github.com/netdata/netdata/pull/15365) ([Ancairon](https://github.com/Ancairon))
- format the sdr cache filenames [\#15361](https://github.com/netdata/netdata/pull/15361) ([ktsaou](https://github.com/ktsaou))
- fix\(freeipmi\): set sensor state on every reading [\#15360](https://github.com/netdata/netdata/pull/15360) ([ilyam8](https://github.com/ilyam8))
- documentation update for the release of the new UI [\#15359](https://github.com/netdata/netdata/pull/15359) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Rename multi module yamls to same name but wuth prefix [\#15356](https://github.com/netdata/netdata/pull/15356) ([Ancairon](https://github.com/Ancairon))
- Update dashboard to version v3.0.1. [\#15352](https://github.com/netdata/netdata/pull/15352) ([netdatabot](https://github.com/netdatabot))
- Fix installation type command [\#15351](https://github.com/netdata/netdata/pull/15351) ([hugovalente-pm](https://github.com/hugovalente-pm))
- agent alert notifications redirect [\#15350](https://github.com/netdata/netdata/pull/15350) ([ktsaou](https://github.com/ktsaou))
- bearer protection - additions [\#15349](https://github.com/netdata/netdata/pull/15349) ([ktsaou](https://github.com/ktsaou))
- health: fix evaluating expression with `nan` [\#15348](https://github.com/netdata/netdata/pull/15348) ([ilyam8](https://github.com/ilyam8))
- add missing labels to freeipmi metrics csv [\#15347](https://github.com/netdata/netdata/pull/15347) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#15345](https://github.com/netdata/netdata/pull/15345) ([stelfrag](https://github.com/stelfrag))
- Update libbpf on netdata repo [\#15343](https://github.com/netdata/netdata/pull/15343) ([thiagoftsm](https://github.com/thiagoftsm))
- bearer improvements [\#15342](https://github.com/netdata/netdata/pull/15342) ([ktsaou](https://github.com/ktsaou))
- Attempt to more aggressively skip CI jobs on PRs if those jobs are irrelevant to the PR. [\#15341](https://github.com/netdata/netdata/pull/15341) ([Ferroin](https://github.com/Ferroin))
- Remove availability from required fields on metric level [\#15340](https://github.com/netdata/netdata/pull/15340) ([Ancairon](https://github.com/Ancairon))
- docs: make the default Docker installation provide the full feature set [\#15339](https://github.com/netdata/netdata/pull/15339) ([ilyam8](https://github.com/ilyam8))
- add internal stats metrics csv [\#15337](https://github.com/netdata/netdata/pull/15337) ([ilyam8](https://github.com/ilyam8))
- Add missing required field in schema [\#15335](https://github.com/netdata/netdata/pull/15335) ([Ancairon](https://github.com/Ancairon))
- Fix compilation on BSD [\#15331](https://github.com/netdata/netdata/pull/15331) ([thiagoftsm](https://github.com/thiagoftsm))
- alerts\_transitions outputs hostnames and items statistics [\#15329](https://github.com/netdata/netdata/pull/15329) ([ktsaou](https://github.com/ktsaou))
- Use spinlock in host and chart [\#15328](https://github.com/netdata/netdata/pull/15328) ([stelfrag](https://github.com/stelfrag))
- multi-threaded version of freeipmi.plugin [\#15327](https://github.com/netdata/netdata/pull/15327) ([ktsaou](https://github.com/ktsaou))
- Single module schema, add required properties [\#15326](https://github.com/netdata/netdata/pull/15326) ([Ancairon](https://github.com/Ancairon))
- Fix coverity issue 394862 - Argument cannot be negative [\#15324](https://github.com/netdata/netdata/pull/15324) ([stelfrag](https://github.com/stelfrag))
- Rename log Macros \(debug\) [\#15322](https://github.com/netdata/netdata/pull/15322) ([thiagoftsm](https://github.com/thiagoftsm))
- bearer authorization API [\#15321](https://github.com/netdata/netdata/pull/15321) ([ktsaou](https://github.com/ktsaou))
- local-listeners: use host prefix in read\_cmdline [\#15320](https://github.com/netdata/netdata/pull/15320) ([ilyam8](https://github.com/ilyam8))
- local-listener using libnetdata [\#15319](https://github.com/netdata/netdata/pull/15319) ([ktsaou](https://github.com/ktsaou))
- avoid memory allocations for alert transitions facets processing [\#15318](https://github.com/netdata/netdata/pull/15318) ([ktsaou](https://github.com/ktsaou))
- add add summary linking to alert instances \(ati\) when options=summary,values is requested [\#15317](https://github.com/netdata/netdata/pull/15317) ([ktsaou](https://github.com/ktsaou))
- fix alerts transitions sorting [\#15315](https://github.com/netdata/netdata/pull/15315) ([ktsaou](https://github.com/ktsaou))
- Keep health log history in seconds [\#15314](https://github.com/netdata/netdata/pull/15314) ([MrZammler](https://github.com/MrZammler))
- stale vitual hosts [\#15313](https://github.com/netdata/netdata/pull/15313) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin to v0.54.0 [\#15312](https://github.com/netdata/netdata/pull/15312) ([ilyam8](https://github.com/ilyam8))
- health: respect overriding nc binary for IRC notifications [\#15310](https://github.com/netdata/netdata/pull/15310) ([ilyam8](https://github.com/ilyam8))
- hide not available for viewers charts when exporting in shell format [\#15309](https://github.com/netdata/netdata/pull/15309) ([ilyam8](https://github.com/ilyam8))
- move collectors meta to metadata/ [\#15308](https://github.com/netdata/netdata/pull/15308) ([ilyam8](https://github.com/ilyam8))
- Release acquired dimensions [\#15307](https://github.com/netdata/netdata/pull/15307) ([stelfrag](https://github.com/stelfrag))
- Check for source field when requesting /api/v1/alarm\_log [\#15306](https://github.com/netdata/netdata/pull/15306) ([MrZammler](https://github.com/MrZammler))
- ci: disable clang format [\#15305](https://github.com/netdata/netdata/pull/15305) ([ilyam8](https://github.com/ilyam8))
- Change info to netdata\_log\_info in sqlite\_db\_migration.c [\#15303](https://github.com/netdata/netdata/pull/15303) ([MrZammler](https://github.com/MrZammler))
- Create integrations JSON schema [\#15302](https://github.com/netdata/netdata/pull/15302) ([Ancairon](https://github.com/Ancairon))
- Change query to store host system info values [\#15300](https://github.com/netdata/netdata/pull/15300) ([MrZammler](https://github.com/MrZammler))
- s/info/netdata\_log\_info/ [\#15299](https://github.com/netdata/netdata/pull/15299) ([vkalintiris](https://github.com/vkalintiris))
- Fix wording issue in Docker README [\#15298](https://github.com/netdata/netdata/pull/15298) ([Ancairon](https://github.com/Ancairon))
- Update metrics-streaming-and-replication.md [\#15297](https://github.com/netdata/netdata/pull/15297) ([Ancairon](https://github.com/Ancairon))
- Rename generic `error` function [\#15296](https://github.com/netdata/netdata/pull/15296) ([thiagoftsm](https://github.com/thiagoftsm))
- Code reorg and cleanup - enrichment of /api/v2 [\#15294](https://github.com/netdata/netdata/pull/15294) ([ktsaou](https://github.com/ktsaou))
- Optimizations part 3 [\#15293](https://github.com/netdata/netdata/pull/15293) ([ktsaou](https://github.com/ktsaou))
- docs: update stream.conf "health enabled by default" description [\#15291](https://github.com/netdata/netdata/pull/15291) ([ilyam8](https://github.com/ilyam8))
- Remove extra parenthesis from doc [\#15290](https://github.com/netdata/netdata/pull/15290) ([Ancairon](https://github.com/Ancairon))
- merged spaces, war rooms and invite your team to one place [\#15289](https://github.com/netdata/netdata/pull/15289) ([hugovalente-pm](https://github.com/hugovalente-pm))
- use stat\(\) instead of lstat\(\) [\#15287](https://github.com/netdata/netdata/pull/15287) ([ktsaou](https://github.com/ktsaou))
- Only try to enable \_FORTIFY\_SOURCE if the user has not disabled optimizations [\#15284](https://github.com/netdata/netdata/pull/15284) ([Ferroin](https://github.com/Ferroin))
- Send alert chart labels config key to cloud [\#15283](https://github.com/netdata/netdata/pull/15283) ([MrZammler](https://github.com/MrZammler))
- Fixed mistype for 'send automatic labels' Prometheus option [\#15282](https://github.com/netdata/netdata/pull/15282) ([k0ste](https://github.com/k0ste))
- Optimizations part 2 [\#15280](https://github.com/netdata/netdata/pull/15280) ([ktsaou](https://github.com/ktsaou))
- Revert "Optimizations Part 2" [\#15279](https://github.com/netdata/netdata/pull/15279) ([ktsaou](https://github.com/ktsaou))
- exporting: change priority to synchronous when calculating value [\#15276](https://github.com/netdata/netdata/pull/15276) ([ilyam8](https://github.com/ilyam8))
- expose CmdLine in apps function [\#15275](https://github.com/netdata/netdata/pull/15275) ([ilyam8](https://github.com/ilyam8))
- Misc alert fixes [\#15274](https://github.com/netdata/netdata/pull/15274) ([MrZammler](https://github.com/MrZammler))
- Small readme improvements [\#15270](https://github.com/netdata/netdata/pull/15270) ([andrewm4894](https://github.com/andrewm4894))
- Optimizations Part 2 [\#15267](https://github.com/netdata/netdata/pull/15267) ([ktsaou](https://github.com/ktsaou))
- Replace `info` macro with a less generic name [\#15266](https://github.com/netdata/netdata/pull/15266) ([carlocab](https://github.com/carlocab))
- Yaml template finalization [\#15265](https://github.com/netdata/netdata/pull/15265) ([Ancairon](https://github.com/Ancairon))
- fix tc.plugin charts labels [\#15262](https://github.com/netdata/netdata/pull/15262) ([ilyam8](https://github.com/ilyam8))
- Update libbpf version [\#15258](https://github.com/netdata/netdata/pull/15258) ([thiagoftsm](https://github.com/thiagoftsm))
- rewrite /api/v2/alerts [\#15257](https://github.com/netdata/netdata/pull/15257) ([ktsaou](https://github.com/ktsaou))
- Fix $\(libh2o\_dir\) not expanded properly sometimes. [\#15253](https://github.com/netdata/netdata/pull/15253) ([Dim-P](https://github.com/Dim-P))
- use gperf for the pluginsd/streaming parser hashtable [\#15251](https://github.com/netdata/netdata/pull/15251) ([ktsaou](https://github.com/ktsaou))
- Update pfsense.md package install instructions [\#15250](https://github.com/netdata/netdata/pull/15250) ([MYanello](https://github.com/MYanello))
- URL rewrite at the agent web server to support multiple dashboard versions [\#15247](https://github.com/netdata/netdata/pull/15247) ([ktsaou](https://github.com/ktsaou))
- delay collecting virtual network interfaces [\#15244](https://github.com/netdata/netdata/pull/15244) ([ilyam8](https://github.com/ilyam8))
- Assorted kickstart script improvements. [\#15243](https://github.com/netdata/netdata/pull/15243) ([Ferroin](https://github.com/Ferroin))
- Install the correct systemd unit file on older RPM systems. [\#15240](https://github.com/netdata/netdata/pull/15240) ([Ferroin](https://github.com/Ferroin))
- Add yaml metadata for metrics.csv files [\#15238](https://github.com/netdata/netdata/pull/15238) ([Ancairon](https://github.com/Ancairon))
- Add module column to apps.plugin csv [\#15235](https://github.com/netdata/netdata/pull/15235) ([Ancairon](https://github.com/Ancairon))
- Fix coverity 393183 & 393182 [\#15234](https://github.com/netdata/netdata/pull/15234) ([MrZammler](https://github.com/MrZammler))
- Create index for health log migration [\#15233](https://github.com/netdata/netdata/pull/15233) ([stelfrag](https://github.com/stelfrag))
- New alerts endpoint [\#15232](https://github.com/netdata/netdata/pull/15232) ([stelfrag](https://github.com/stelfrag))
- fix not handling N/A value in python.d/nvidia\_smi [\#15231](https://github.com/netdata/netdata/pull/15231) ([ilyam8](https://github.com/ilyam8))
- Fix handling of plugin ownership in static builds. [\#15230](https://github.com/netdata/netdata/pull/15230) ([Ferroin](https://github.com/Ferroin))
- /api/v2 improvements [\#15227](https://github.com/netdata/netdata/pull/15227) ([ktsaou](https://github.com/ktsaou))
- Remove erroneous space for unit [\#15226](https://github.com/netdata/netdata/pull/15226) ([ralphm](https://github.com/ralphm))
- Relax jnfv2 caching [\#15224](https://github.com/netdata/netdata/pull/15224) ([ktsaou](https://github.com/ktsaou))
- Fix /api/v2/contexts,nodes,nodes\_instances,q before match [\#15223](https://github.com/netdata/netdata/pull/15223) ([ktsaou](https://github.com/ktsaou))
- Fix SSL non-blocking retry handling in the web server [\#15222](https://github.com/netdata/netdata/pull/15222) ([ktsaou](https://github.com/ktsaou))
- Update dashboard to version v3.0.0. [\#15219](https://github.com/netdata/netdata/pull/15219) ([netdatabot](https://github.com/netdatabot))
- fix arch detection on i386 \(native packages\) [\#15218](https://github.com/netdata/netdata/pull/15218) ([ilyam8](https://github.com/ilyam8))
- RW\_SPINLOCK: recursive readers support [\#15217](https://github.com/netdata/netdata/pull/15217) ([ktsaou](https://github.com/ktsaou))
- cgroups: remove pod\_uid and container\_id labels in k8s [\#15216](https://github.com/netdata/netdata/pull/15216) ([ilyam8](https://github.com/ilyam8))
- Allow overriding pipename from env [\#15215](https://github.com/netdata/netdata/pull/15215) ([vkalintiris](https://github.com/vkalintiris))
- eBPF Functions \(enable/disable threads\) [\#15214](https://github.com/netdata/netdata/pull/15214) ([thiagoftsm](https://github.com/thiagoftsm))

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
