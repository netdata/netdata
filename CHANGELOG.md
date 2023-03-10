# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.38.1...HEAD)

**Merged pull requests:**

- Fix compilation error when --disable-cloud is specified [\#14695](https://github.com/netdata/netdata/pull/14695) ([stelfrag](https://github.com/stelfrag))
- fix: detect the host os in k8s on non-docker cri [\#14694](https://github.com/netdata/netdata/pull/14694) ([witalisoft](https://github.com/witalisoft))
- Remove google hangouts from list of integrations [\#14689](https://github.com/netdata/netdata/pull/14689) ([cakrit](https://github.com/cakrit))
- Fix Azure IMDS [\#14686](https://github.com/netdata/netdata/pull/14686) ([shyamvalsan](https://github.com/shyamvalsan))
- Send an EOF from charts.d.plugin before exit [\#14680](https://github.com/netdata/netdata/pull/14680) ([MrZammler](https://github.com/MrZammler))
- Fix conditionals for claim-only case in kickstart.sh. [\#14679](https://github.com/netdata/netdata/pull/14679) ([Ferroin](https://github.com/Ferroin))
- Fix kernel test script [\#14676](https://github.com/netdata/netdata/pull/14676) ([thiagoftsm](https://github.com/thiagoftsm))
- add note on readme on how to easily see all ml related blog posts [\#14675](https://github.com/netdata/netdata/pull/14675) ([andrewm4894](https://github.com/andrewm4894))
- Guard for null host when sending node instances [\#14673](https://github.com/netdata/netdata/pull/14673) ([MrZammler](https://github.com/MrZammler))
- reviewed role description to be according to app [\#14672](https://github.com/netdata/netdata/pull/14672) ([hugovalente-pm](https://github.com/hugovalente-pm))
- If a child is not streaming, send to the cloud last known version instead of unknown [\#14671](https://github.com/netdata/netdata/pull/14671) ([MrZammler](https://github.com/MrZammler))
- add FAQ information provided by Finance [\#14664](https://github.com/netdata/netdata/pull/14664) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Handle conffiles for DEB packages explicitly instead of automatically. [\#14662](https://github.com/netdata/netdata/pull/14662) ([Ferroin](https://github.com/Ferroin))
- Fix cloud node stale status when a virtual host is created [\#14660](https://github.com/netdata/netdata/pull/14660) ([stelfrag](https://github.com/stelfrag))
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
- Fix broken links [\#14634](https://github.com/netdata/netdata/pull/14634) ([Ancairon](https://github.com/Ancairon))
- Add link to native packages also on the list [\#14633](https://github.com/netdata/netdata/pull/14633) ([cakrit](https://github.com/cakrit))
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
- do not report dimensions that failed to be queried [\#14447](https://github.com/netdata/netdata/pull/14447) ([ktsaou](https://github.com/ktsaou))
- Fix agent build failure on FreeBSD 14.0 due to new tcpstat struct [\#14446](https://github.com/netdata/netdata/pull/14446) ([Dim-P](https://github.com/Dim-P))
- minor fix in the metadata of libnetdata/ebpf AND log documents [\#14445](https://github.com/netdata/netdata/pull/14445) ([tkatsoulas](https://github.com/tkatsoulas))
- Roles permissions docs [\#14444](https://github.com/netdata/netdata/pull/14444) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Only load required charts for rrdvars [\#14443](https://github.com/netdata/netdata/pull/14443) ([MrZammler](https://github.com/MrZammler))
- Typos in in notification docs [\#14440](https://github.com/netdata/netdata/pull/14440) ([iorvd](https://github.com/iorvd))
- Streaming interpolated values [\#14431](https://github.com/netdata/netdata/pull/14431) ([ktsaou](https://github.com/ktsaou))
- Fix compiler error when CLOSE\_RANGE\_CLOEXEC is missing [\#14430](https://github.com/netdata/netdata/pull/14430) ([Dim-P](https://github.com/Dim-P))
- Add .NET info [\#14429](https://github.com/netdata/netdata/pull/14429) ([thiagoftsm](https://github.com/thiagoftsm))
- Minor fix, convert metadata of the learn to hidden sections [\#14427](https://github.com/netdata/netdata/pull/14427) ([tkatsoulas](https://github.com/tkatsoulas))
- kickstart.sh: Fix `--release-channel` as `--nightly-channel` options [\#14424](https://github.com/netdata/netdata/pull/14424) ([vobruba-martin](https://github.com/vobruba-martin))
- Use curl from static builds if no system-wide copy exists. [\#14403](https://github.com/netdata/netdata/pull/14403) ([Ferroin](https://github.com/Ferroin))
- add @andrewm4894 as docs/ codeowner [\#14398](https://github.com/netdata/netdata/pull/14398) ([andrewm4894](https://github.com/andrewm4894))
- Roles permissions docs [\#14391](https://github.com/netdata/netdata/pull/14391) ([hugovalente-pm](https://github.com/hugovalente-pm))
- add note about not needing to have room id [\#14390](https://github.com/netdata/netdata/pull/14390) ([andrewm4894](https://github.com/andrewm4894))
- update the "Install Netdata with Docker" doc [\#14385](https://github.com/netdata/netdata/pull/14385) ([Ancairon](https://github.com/Ancairon))
- generates dual ksy for njfv2 + fix for padding after page blocks [\#14383](https://github.com/netdata/netdata/pull/14383) ([underhood](https://github.com/underhood))
- Delete docs/cloud/get-started.mdx [\#14382](https://github.com/netdata/netdata/pull/14382) ([Ancairon](https://github.com/Ancairon))
- Update the "Kubernetes visualizations" doc [\#14347](https://github.com/netdata/netdata/pull/14347) ([Ancairon](https://github.com/Ancairon))
- Update the "Deploy Kubernetes monitoring with Netdata" doc [\#14345](https://github.com/netdata/netdata/pull/14345) ([Ancairon](https://github.com/Ancairon))
- Events docs [\#14341](https://github.com/netdata/netdata/pull/14341) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Update the "Install Netdata with kickstart.sh" doc [\#14338](https://github.com/netdata/netdata/pull/14338) ([Ancairon](https://github.com/Ancairon))
- Misc SSL improvements 2 [\#14334](https://github.com/netdata/netdata/pull/14334) ([MrZammler](https://github.com/MrZammler))
- Indicate what root privileges are needed for in kickstart.sh. [\#14314](https://github.com/netdata/netdata/pull/14314) ([Ferroin](https://github.com/Ferroin))

## [v1.38.1](https://github.com/netdata/netdata/tree/v1.38.1) (2023-02-13)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.38.0...v1.38.1)

## [v1.38.0](https://github.com/netdata/netdata/tree/v1.38.0) (2023-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.37.1...v1.38.0)

**Merged pull requests:**

- Updated w1sensor.chart.py [\#14435](https://github.com/netdata/netdata/pull/14435) ([martindue](https://github.com/martindue))
- replication to streaming transition when there are gaps [\#14434](https://github.com/netdata/netdata/pull/14434) ([ktsaou](https://github.com/ktsaou))
- turn error\(\) to internal\_error\(\) [\#14428](https://github.com/netdata/netdata/pull/14428) ([ktsaou](https://github.com/ktsaou))
- Fix typo on the netdata-functions.md [\#14426](https://github.com/netdata/netdata/pull/14426) ([lokerhp](https://github.com/lokerhp))
- Update screenshot of timezone selector [\#14425](https://github.com/netdata/netdata/pull/14425) ([cakrit](https://github.com/cakrit))
- Stop training thread from processing training requests once cancelled. [\#14423](https://github.com/netdata/netdata/pull/14423) ([vkalintiris](https://github.com/vkalintiris))
- Check on parents the microseconds delta sent by agents [\#14422](https://github.com/netdata/netdata/pull/14422) ([ktsaou](https://github.com/ktsaou))
- better logging of invalid pages detected on dbengine files [\#14420](https://github.com/netdata/netdata/pull/14420) ([ktsaou](https://github.com/ktsaou))
- fix functions memory leak [\#14419](https://github.com/netdata/netdata/pull/14419) ([ktsaou](https://github.com/ktsaou))
- Move under Developer in Learn [\#14417](https://github.com/netdata/netdata/pull/14417) ([cakrit](https://github.com/cakrit))
- Libnetdata readmes learn [\#14416](https://github.com/netdata/netdata/pull/14416) ([cakrit](https://github.com/cakrit))
- Minor fixes in markdown links [\#14415](https://github.com/netdata/netdata/pull/14415) ([tkatsoulas](https://github.com/tkatsoulas))
- fix kubelet alarms [\#14414](https://github.com/netdata/netdata/pull/14414) ([ilyam8](https://github.com/ilyam8))
- DBENGINE v2 - bug fixes [\#14413](https://github.com/netdata/netdata/pull/14413) ([ktsaou](https://github.com/ktsaou))
- fix\(cgroups.plugin\): fix collecting full pressure stall time [\#14410](https://github.com/netdata/netdata/pull/14410) ([ilyam8](https://github.com/ilyam8))
- feat\(charts.d\): add load usage \(Watts\) to nuts collector [\#14407](https://github.com/netdata/netdata/pull/14407) ([ilyam8](https://github.com/ilyam8))
- fix link to ebpf collector [\#14405](https://github.com/netdata/netdata/pull/14405) ([ilyam8](https://github.com/ilyam8))
- Remove equality when deciding how to use point [\#14402](https://github.com/netdata/netdata/pull/14402) ([MrZammler](https://github.com/MrZammler))
- add help line to functions response [\#14399](https://github.com/netdata/netdata/pull/14399) ([ktsaou](https://github.com/ktsaou))
- Fix typo on the page [\#14397](https://github.com/netdata/netdata/pull/14397) ([iorvd](https://github.com/iorvd))
- Fix kickstart and updater not working with BusyBox wget [\#14392](https://github.com/netdata/netdata/pull/14392) ([Dim-P](https://github.com/Dim-P))
- Fix publishing Docker Images to secondary registries. [\#14389](https://github.com/netdata/netdata/pull/14389) ([Ferroin](https://github.com/Ferroin))
- Reduce service exit [\#14381](https://github.com/netdata/netdata/pull/14381) ([thiagoftsm](https://github.com/thiagoftsm))
- DBENGINE v2 - improvements part 12 [\#14379](https://github.com/netdata/netdata/pull/14379) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin v0.50.0 [\#14378](https://github.com/netdata/netdata/pull/14378) ([ilyam8](https://github.com/ilyam8))
- Patch master [\#14377](https://github.com/netdata/netdata/pull/14377) ([tkatsoulas](https://github.com/tkatsoulas))
- Revert "Delete libnetdata readme" [\#14374](https://github.com/netdata/netdata/pull/14374) ([cakrit](https://github.com/cakrit))
- Revert "Add libnetdata readmes to learn, delete empty" [\#14373](https://github.com/netdata/netdata/pull/14373) ([cakrit](https://github.com/cakrit))
- Publish Docker images to GHCR.io and Quay.io [\#14372](https://github.com/netdata/netdata/pull/14372) ([Ferroin](https://github.com/Ferroin))
- Add libnetdata readmes to learn, delete empty [\#14371](https://github.com/netdata/netdata/pull/14371) ([cakrit](https://github.com/cakrit))
- Add collectors main readme to learn [\#14370](https://github.com/netdata/netdata/pull/14370) ([cakrit](https://github.com/cakrit))
- Add collectors list to learn temporarily [\#14369](https://github.com/netdata/netdata/pull/14369) ([cakrit](https://github.com/cakrit))
- Add simple patterns readme to learn [\#14366](https://github.com/netdata/netdata/pull/14366) ([cakrit](https://github.com/cakrit))
- Add one way allocator readme to learn [\#14365](https://github.com/netdata/netdata/pull/14365) ([cakrit](https://github.com/cakrit))
- Add July README to learn [\#14364](https://github.com/netdata/netdata/pull/14364) ([cakrit](https://github.com/cakrit))
- Add ARL readme to learn [\#14363](https://github.com/netdata/netdata/pull/14363) ([cakrit](https://github.com/cakrit))
- Add BUFFER lib doc to learn [\#14362](https://github.com/netdata/netdata/pull/14362) ([cakrit](https://github.com/cakrit))
- Add dictionary readme to learn [\#14361](https://github.com/netdata/netdata/pull/14361) ([cakrit](https://github.com/cakrit))
- Add explanation of config files to learn [\#14360](https://github.com/netdata/netdata/pull/14360) ([cakrit](https://github.com/cakrit))
- Delete libnetdata readme [\#14357](https://github.com/netdata/netdata/pull/14357) ([cakrit](https://github.com/cakrit))
- Add main health readme to learn [\#14356](https://github.com/netdata/netdata/pull/14356) ([cakrit](https://github.com/cakrit))
- Delete QUICKSTART.md [\#14355](https://github.com/netdata/netdata/pull/14355) ([cakrit](https://github.com/cakrit))
- Delete data structures readme [\#14354](https://github.com/netdata/netdata/pull/14354) ([cakrit](https://github.com/cakrit))
- Delete BREAKING\_CHANGES.md [\#14353](https://github.com/netdata/netdata/pull/14353) ([cakrit](https://github.com/cakrit))
- Add redistributed to learn [\#14352](https://github.com/netdata/netdata/pull/14352) ([cakrit](https://github.com/cakrit))
- Add missing entries in README.md [\#14351](https://github.com/netdata/netdata/pull/14351) ([thiagoftsm](https://github.com/thiagoftsm))
- Add ansible.md to learn [\#14350](https://github.com/netdata/netdata/pull/14350) ([cakrit](https://github.com/cakrit))
- Delete BUILD.md [\#14348](https://github.com/netdata/netdata/pull/14348) ([cakrit](https://github.com/cakrit))
- Patch convert rel links [\#14344](https://github.com/netdata/netdata/pull/14344) ([tkatsoulas](https://github.com/tkatsoulas))
- Update dashboard [\#14342](https://github.com/netdata/netdata/pull/14342) ([thiagoftsm](https://github.com/thiagoftsm))
- minor fix on notification doc \(Discord\) [\#14339](https://github.com/netdata/netdata/pull/14339) ([tkatsoulas](https://github.com/tkatsoulas))
- DBENGINE v2 - improvements part 11 [\#14337](https://github.com/netdata/netdata/pull/14337) ([ktsaou](https://github.com/ktsaou))
- Update the Get started doc [\#14336](https://github.com/netdata/netdata/pull/14336) ([Ancairon](https://github.com/Ancairon))
- Notifications integration docs [\#14335](https://github.com/netdata/netdata/pull/14335) ([hugovalente-pm](https://github.com/hugovalente-pm))
- DBENGINE v2 - improvements part 10 [\#14332](https://github.com/netdata/netdata/pull/14332) ([ktsaou](https://github.com/ktsaou))
- reviewed the docs functions to fix broken links and other additions [\#14331](https://github.com/netdata/netdata/pull/14331) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Add |nowarn and |noclear notification modifiers [\#14330](https://github.com/netdata/netdata/pull/14330) ([vobruba-martin](https://github.com/vobruba-martin))
- Revert "Misc SSL improvements" [\#14327](https://github.com/netdata/netdata/pull/14327) ([MrZammler](https://github.com/MrZammler))
- DBENGINE v2 - improvements part 9 [\#14326](https://github.com/netdata/netdata/pull/14326) ([ktsaou](https://github.com/ktsaou))
- Don't send alert variables to the cloud [\#14325](https://github.com/netdata/netdata/pull/14325) ([MrZammler](https://github.com/MrZammler))
- fix\(proc.plugin\): add "cpu" label to per core util% charts [\#14322](https://github.com/netdata/netdata/pull/14322) ([ilyam8](https://github.com/ilyam8))
- DBENGINE v2 - improvements part 8 [\#14319](https://github.com/netdata/netdata/pull/14319) ([ktsaou](https://github.com/ktsaou))
- Misc SSL improvements [\#14317](https://github.com/netdata/netdata/pull/14317) ([MrZammler](https://github.com/MrZammler))
- Use "getent group" instead of reading "/etc/group" to get group information [\#14316](https://github.com/netdata/netdata/pull/14316) ([Dim-P](https://github.com/Dim-P))
- Add nvidia smi pci bandwidth percent collector [\#14315](https://github.com/netdata/netdata/pull/14315) ([ghanapunq](https://github.com/ghanapunq))
- minor - kaitai for netdata datafiles [\#14312](https://github.com/netdata/netdata/pull/14312) ([underhood](https://github.com/underhood))
- Add Collector log [\#14309](https://github.com/netdata/netdata/pull/14309) ([thiagoftsm](https://github.com/thiagoftsm))
- DBENGINE v2 - improvements part 7 [\#14307](https://github.com/netdata/netdata/pull/14307) ([ktsaou](https://github.com/ktsaou))
- Fix Exporiting compilaton error [\#14306](https://github.com/netdata/netdata/pull/14306) ([thiagoftsm](https://github.com/thiagoftsm))
- bump go.d.plugin to v0.49.2 [\#14305](https://github.com/netdata/netdata/pull/14305) ([ilyam8](https://github.com/ilyam8))
- Fixes required to make the agent work without crashes on MacOS [\#14304](https://github.com/netdata/netdata/pull/14304) ([vkalintiris](https://github.com/vkalintiris))
- Update kickstart script to use new DEB infrastructure. [\#14301](https://github.com/netdata/netdata/pull/14301) ([Ferroin](https://github.com/Ferroin))
- DBENGINE v2 - improvements part 6 [\#14299](https://github.com/netdata/netdata/pull/14299) ([ktsaou](https://github.com/ktsaou))
- add consul license expiration time alarm [\#14298](https://github.com/netdata/netdata/pull/14298) ([ilyam8](https://github.com/ilyam8))
- Fix macos struct definition. [\#14297](https://github.com/netdata/netdata/pull/14297) ([vkalintiris](https://github.com/vkalintiris))
- Remove archivedcharts endpoint, optimize indices [\#14296](https://github.com/netdata/netdata/pull/14296) ([stelfrag](https://github.com/stelfrag))
- track memory footprint of Netdata [\#14294](https://github.com/netdata/netdata/pull/14294) ([ktsaou](https://github.com/ktsaou))
- Switch to self-hosted infrastructure for DEB package distribution. [\#14290](https://github.com/netdata/netdata/pull/14290) ([Ferroin](https://github.com/Ferroin))
- DBENGINE v2 - improvements part 5 [\#14289](https://github.com/netdata/netdata/pull/14289) ([ktsaou](https://github.com/ktsaou))
- allow multiple local-build/static-install options in kickstart [\#14287](https://github.com/netdata/netdata/pull/14287) ([ilyam8](https://github.com/ilyam8))
- fix\(alarms\): treat 0 processors as unknown in load\_cpu\_number [\#14286](https://github.com/netdata/netdata/pull/14286) ([ilyam8](https://github.com/ilyam8))
- DBENGINE v2 - improvements part 4 [\#14285](https://github.com/netdata/netdata/pull/14285) ([ktsaou](https://github.com/ktsaou))
- fix for dbengine2 improvements part 3 [\#14284](https://github.com/netdata/netdata/pull/14284) ([ktsaou](https://github.com/ktsaou))
- Make sure variables are streamed after SENDER\_CONNECTED flag is set [\#14283](https://github.com/netdata/netdata/pull/14283) ([MrZammler](https://github.com/MrZammler))
- Update to SQLITE version 3.40.1 [\#14282](https://github.com/netdata/netdata/pull/14282) ([stelfrag](https://github.com/stelfrag))
- Check session variable before resuming it [\#14279](https://github.com/netdata/netdata/pull/14279) ([MrZammler](https://github.com/MrZammler))
- Update infographic image on main README [\#14276](https://github.com/netdata/netdata/pull/14276) ([cakrit](https://github.com/cakrit))
- bump go.d.plugin to v0.49.1 [\#14275](https://github.com/netdata/netdata/pull/14275) ([ilyam8](https://github.com/ilyam8))
- Improve ebpf exit [\#14270](https://github.com/netdata/netdata/pull/14270) ([thiagoftsm](https://github.com/thiagoftsm))
- DBENGINE v2 - improvements part 3 [\#14269](https://github.com/netdata/netdata/pull/14269) ([ktsaou](https://github.com/ktsaou))
- minor - add kaitaistruct for journal v2 files [\#14267](https://github.com/netdata/netdata/pull/14267) ([underhood](https://github.com/underhood))
- fix\(health\): don't assume 2 cores if the number is unknown [\#14265](https://github.com/netdata/netdata/pull/14265) ([ilyam8](https://github.com/ilyam8))
- More 32bit fixes [\#14264](https://github.com/netdata/netdata/pull/14264) ([ktsaou](https://github.com/ktsaou))
- Store host and claim info in sqlite as soon as possible [\#14263](https://github.com/netdata/netdata/pull/14263) ([MrZammler](https://github.com/MrZammler))
- Replace individual collector images/links on infographic [\#14262](https://github.com/netdata/netdata/pull/14262) ([cakrit](https://github.com/cakrit))
- Fix binpkg updates on OpenSUSE [\#14260](https://github.com/netdata/netdata/pull/14260) ([Dim-P](https://github.com/Dim-P))
- DBENGINE v2 - improvements 2 [\#14257](https://github.com/netdata/netdata/pull/14257) ([ktsaou](https://github.com/ktsaou))
- fix\(pacakging\): fix cpu/memory metrics when running inside LXC container as systemd service [\#14255](https://github.com/netdata/netdata/pull/14255) ([ilyam8](https://github.com/ilyam8))
- fix\(proc.plugin\): handle disabled IPv6 [\#14252](https://github.com/netdata/netdata/pull/14252) ([ilyam8](https://github.com/ilyam8))
- DBENGINE v2 - improvements part 1 [\#14251](https://github.com/netdata/netdata/pull/14251) ([ktsaou](https://github.com/ktsaou))
- Remove daemon/common.h header from libnetdata [\#14248](https://github.com/netdata/netdata/pull/14248) ([vkalintiris](https://github.com/vkalintiris))
- allow the cache to grow when huge queries are running that exceed the cache size [\#14247](https://github.com/netdata/netdata/pull/14247) ([ktsaou](https://github.com/ktsaou))
- Update netdata-overview.xml [\#14245](https://github.com/netdata/netdata/pull/14245) ([andrewm4894](https://github.com/andrewm4894))
- Revert health to run in a single thread [\#14244](https://github.com/netdata/netdata/pull/14244) ([MrZammler](https://github.com/MrZammler))
- profile startup and shutdown timings [\#14243](https://github.com/netdata/netdata/pull/14243) ([ktsaou](https://github.com/ktsaou))
- `ml - machine learning` to just `machine learning` [\#14242](https://github.com/netdata/netdata/pull/14242) ([andrewm4894](https://github.com/andrewm4894))
- cancel ml threads on shutdown and join them on host free [\#14240](https://github.com/netdata/netdata/pull/14240) ([ktsaou](https://github.com/ktsaou))
- pre gcc v5 support and allow building without dbengine [\#14239](https://github.com/netdata/netdata/pull/14239) ([ktsaou](https://github.com/ktsaou))
- Drop ARMv7 native packages for Fedora 36. [\#14233](https://github.com/netdata/netdata/pull/14233) ([Ferroin](https://github.com/Ferroin))
- fix consul\_raft\_leadership\_transitions alarm units [\#14232](https://github.com/netdata/netdata/pull/14232) ([ilyam8](https://github.com/ilyam8))
- readme updates [\#14224](https://github.com/netdata/netdata/pull/14224) ([andrewm4894](https://github.com/andrewm4894))
- bump go.d v0.49.0 [\#14220](https://github.com/netdata/netdata/pull/14220) ([ilyam8](https://github.com/ilyam8))
- remove lgtm.com [\#14216](https://github.com/netdata/netdata/pull/14216) ([ilyam8](https://github.com/ilyam8))
- Improve file descriptor closing loops [\#14213](https://github.com/netdata/netdata/pull/14213) ([Dim-P](https://github.com/Dim-P))
- Remove temporary allocations when preprocessing a samples buffer [\#14208](https://github.com/netdata/netdata/pull/14208) ([vkalintiris](https://github.com/vkalintiris))
- Create ML charts on child hosts. [\#14207](https://github.com/netdata/netdata/pull/14207) ([vkalintiris](https://github.com/vkalintiris))
- Use brackets around info variables [\#14206](https://github.com/netdata/netdata/pull/14206) ([MrZammler](https://github.com/MrZammler))
- Dont call worker\_utilization\_finish\(\) twice [\#14204](https://github.com/netdata/netdata/pull/14204) ([MrZammler](https://github.com/MrZammler))
- Switch to actions/labeler@v4 for labeling PRs. [\#14203](https://github.com/netdata/netdata/pull/14203) ([Ferroin](https://github.com/Ferroin))
- Refactor ML code and add support for multiple KMeans models [\#14198](https://github.com/netdata/netdata/pull/14198) ([vkalintiris](https://github.com/vkalintiris))
- Add few alarms for elasticsearch [\#14197](https://github.com/netdata/netdata/pull/14197) ([ilyam8](https://github.com/ilyam8))
- chore\(packaging\): remove python-pymongo [\#14196](https://github.com/netdata/netdata/pull/14196) ([ilyam8](https://github.com/ilyam8))
- bump go.d.plugin to v0.48.0 [\#14195](https://github.com/netdata/netdata/pull/14195) ([ilyam8](https://github.com/ilyam8))
- Fix typos [\#14194](https://github.com/netdata/netdata/pull/14194) ([rex4539](https://github.com/rex4539))
- add `telegraf` to `apps_groups.conf` monitoring section [\#14188](https://github.com/netdata/netdata/pull/14188) ([andrewm4894](https://github.com/andrewm4894))
- bump go.d.plugin to v0.47.0 [\#14182](https://github.com/netdata/netdata/pull/14182) ([ilyam8](https://github.com/ilyam8))
- remove mqtt-c from websockets [\#14181](https://github.com/netdata/netdata/pull/14181) ([underhood](https://github.com/underhood))
- fix logrotate postrotate [\#14180](https://github.com/netdata/netdata/pull/14180) ([ilyam8](https://github.com/ilyam8))
- docs: explicitly set the `nofile` limit for Netdata container and document the reason for this [\#14178](https://github.com/netdata/netdata/pull/14178) ([ilyam8](https://github.com/ilyam8))
- remove interface name from cgroup net family [\#14174](https://github.com/netdata/netdata/pull/14174) ([ilyam8](https://github.com/ilyam8))
- use specific charts labels instead of family in alarms [\#14173](https://github.com/netdata/netdata/pull/14173) ([ilyam8](https://github.com/ilyam8))
- Revert "Refactor ML code and add support for multiple KMeans models. … [\#14172](https://github.com/netdata/netdata/pull/14172) ([vkalintiris](https://github.com/vkalintiris))
- fix a typo in debian postinst [\#14171](https://github.com/netdata/netdata/pull/14171) ([ilyam8](https://github.com/ilyam8))
- feat\(packaging\): add netdata to www-data group on Proxmox [\#14168](https://github.com/netdata/netdata/pull/14168) ([ilyam8](https://github.com/ilyam8))
- minor - fix localhost nodeinstance fnc caps [\#14166](https://github.com/netdata/netdata/pull/14166) ([underhood](https://github.com/underhood))
- minor - Adds query type "function\[s\]" for aclk chart [\#14165](https://github.com/netdata/netdata/pull/14165) ([underhood](https://github.com/underhood))
- Fix race on query thread startup [\#14164](https://github.com/netdata/netdata/pull/14164) ([underhood](https://github.com/underhood))
- add alarms and dashboard info for Consul [\#14163](https://github.com/netdata/netdata/pull/14163) ([ilyam8](https://github.com/ilyam8))
- Ensure --claim-url for the claim script is a URL [\#14160](https://github.com/netdata/netdata/pull/14160) ([ralphm](https://github.com/ralphm))
- Finish switch to self-hosted RPM repositories. [\#14158](https://github.com/netdata/netdata/pull/14158) ([Ferroin](https://github.com/Ferroin))
- fix nodejs app detection [\#14156](https://github.com/netdata/netdata/pull/14156) ([ilyam8](https://github.com/ilyam8))
- Populate field values in send\_slack\(\) for Mattermost [\#14153](https://github.com/netdata/netdata/pull/14153) ([je2555](https://github.com/je2555))
- bump go.d.plugin to v0.46.1 [\#14151](https://github.com/netdata/netdata/pull/14151) ([ilyam8](https://github.com/ilyam8))
- Add a health configuration option of which alarms to load [\#14150](https://github.com/netdata/netdata/pull/14150) ([MrZammler](https://github.com/MrZammler))
- MQTT5 Topic Alias [\#14148](https://github.com/netdata/netdata/pull/14148) ([underhood](https://github.com/underhood))
- Disable integration by default \(eBPF \<-\> APPS\) [\#14147](https://github.com/netdata/netdata/pull/14147) ([thiagoftsm](https://github.com/thiagoftsm))
- Revert "MQTT 5 publish topic alias support" [\#14145](https://github.com/netdata/netdata/pull/14145) ([MrZammler](https://github.com/MrZammler))
- rename "Pid" to "PID" in functions [\#14144](https://github.com/netdata/netdata/pull/14144) ([andrewm4894](https://github.com/andrewm4894))
- Document memory mode alloc [\#14142](https://github.com/netdata/netdata/pull/14142) ([cakrit](https://github.com/cakrit))
- fix\(packaging\): add setuid for cgroup-network and ebpf.plugin in RPM [\#14140](https://github.com/netdata/netdata/pull/14140) ([ilyam8](https://github.com/ilyam8))
- use chart labels in portcheck alarms [\#14137](https://github.com/netdata/netdata/pull/14137) ([ilyam8](https://github.com/ilyam8))
- Remove Fedora 35 from the list of supported platforms. [\#14136](https://github.com/netdata/netdata/pull/14136) ([Ferroin](https://github.com/Ferroin))
- Fix conditions for uploading repoconfig packages to new infra. [\#14134](https://github.com/netdata/netdata/pull/14134) ([Ferroin](https://github.com/Ferroin))
- fix httpcheck alarms [\#14133](https://github.com/netdata/netdata/pull/14133) ([ilyam8](https://github.com/ilyam8))
- eBPF \(memory, NV, basis for functions\) [\#14131](https://github.com/netdata/netdata/pull/14131) ([thiagoftsm](https://github.com/thiagoftsm))
- DBENGINE v2 [\#14125](https://github.com/netdata/netdata/pull/14125) ([ktsaou](https://github.com/ktsaou))
- bump go.d.plugin to v0.46.0 [\#14124](https://github.com/netdata/netdata/pull/14124) ([ilyam8](https://github.com/ilyam8))
- heartbeat: don't log every discrepancy [\#14122](https://github.com/netdata/netdata/pull/14122) ([ktsaou](https://github.com/ktsaou))
- ARAL: add destroy function and optimize ifdefs [\#14121](https://github.com/netdata/netdata/pull/14121) ([ktsaou](https://github.com/ktsaou))
- Enable retries for SSL\_ERROR\_WANT\_READ [\#14120](https://github.com/netdata/netdata/pull/14120) ([MrZammler](https://github.com/MrZammler))
- ci: fix cgroup-parent name in packaging [\#14118](https://github.com/netdata/netdata/pull/14118) ([ilyam8](https://github.com/ilyam8))
- don't log too much about streaming connections [\#14117](https://github.com/netdata/netdata/pull/14117) ([ktsaou](https://github.com/ktsaou))
- fix get\_system\_cpus\(\) [\#14116](https://github.com/netdata/netdata/pull/14116) ([ktsaou](https://github.com/ktsaou))
- Fix minor typo. [\#14111](https://github.com/netdata/netdata/pull/14111) ([tkatsoulas](https://github.com/tkatsoulas))
- expose ACLK SSL KeyLog interface for developers [\#14109](https://github.com/netdata/netdata/pull/14109) ([underhood](https://github.com/underhood))
- add filtering options to functions table output [\#14108](https://github.com/netdata/netdata/pull/14108) ([ktsaou](https://github.com/ktsaou))
- fix health emphemerality labels src [\#14105](https://github.com/netdata/netdata/pull/14105) ([ilyam8](https://github.com/ilyam8))
- fix docker host editable config [\#14104](https://github.com/netdata/netdata/pull/14104) ([ilyam8](https://github.com/ilyam8))
- Switch to self-hosted infrastructure for RPM package distribution. [\#14100](https://github.com/netdata/netdata/pull/14100) ([Ferroin](https://github.com/Ferroin))
- Fix missing required package install of tar on FreeBSD [\#14095](https://github.com/netdata/netdata/pull/14095) ([Dim-P](https://github.com/Dim-P))
- Add version to netdatacli [\#14094](https://github.com/netdata/netdata/pull/14094) ([MrZammler](https://github.com/MrZammler))
- docs: add a note to set container nofile ulimit for Fedora users [\#14092](https://github.com/netdata/netdata/pull/14092) ([ilyam8](https://github.com/ilyam8))
- Fix eBPF load on RH 8.x family and improve code. [\#14090](https://github.com/netdata/netdata/pull/14090) ([thiagoftsm](https://github.com/thiagoftsm))
- fix v1.37 dbengine page alignment crashes [\#14086](https://github.com/netdata/netdata/pull/14086) ([ktsaou](https://github.com/ktsaou))
- Fix \_\_atomic\_compare\_exchange\_n\(\) atomics [\#14085](https://github.com/netdata/netdata/pull/14085) ([ktsaou](https://github.com/ktsaou))
- Fix 1.37 crashes [\#14081](https://github.com/netdata/netdata/pull/14081) ([stelfrag](https://github.com/stelfrag))
- add basic dashboard info for NGINX Plus [\#14080](https://github.com/netdata/netdata/pull/14080) ([ilyam8](https://github.com/ilyam8))
- replication fixes 9 [\#14079](https://github.com/netdata/netdata/pull/14079) ([ktsaou](https://github.com/ktsaou))
- optimize workers statistics performance [\#14077](https://github.com/netdata/netdata/pull/14077) ([ktsaou](https://github.com/ktsaou))
- fix SSL related crashes [\#14076](https://github.com/netdata/netdata/pull/14076) ([ktsaou](https://github.com/ktsaou))
- remove python.d/springboot [\#14075](https://github.com/netdata/netdata/pull/14075) ([ilyam8](https://github.com/ilyam8))
- fix backfilling statistics [\#14074](https://github.com/netdata/netdata/pull/14074) ([ktsaou](https://github.com/ktsaou))
- remove deprecated fping.plugin in accordance with v1.37.0 deprecation notice  [\#14073](https://github.com/netdata/netdata/pull/14073) ([ilyam8](https://github.com/ilyam8))

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
