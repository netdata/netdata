# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.3...HEAD)

**Merged pull requests:**

- reset health when children disconnect [\#17612](https://github.com/netdata/netdata/pull/17612) ([ktsaou](https://github.com/ktsaou))
- go.d dyncfg return 200 on Enable for running jobs [\#17611](https://github.com/netdata/netdata/pull/17611) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17610](https://github.com/netdata/netdata/pull/17610) ([netdatabot](https://github.com/netdatabot))
- Bump golang.org/x/net from 0.24.0 to 0.25.0 in /src/go/collectors/go.d.plugin [\#17609](https://github.com/netdata/netdata/pull/17609) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump jinja2 from 3.1.3 to 3.1.4 in /packaging/dag [\#17607](https://github.com/netdata/netdata/pull/17607) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d systemdunits add unit files state [\#17606](https://github.com/netdata/netdata/pull/17606) ([ilyam8](https://github.com/ilyam8))
- Add option to limit architectures for offline installs. [\#17604](https://github.com/netdata/netdata/pull/17604) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17603](https://github.com/netdata/netdata/pull/17603) ([netdatabot](https://github.com/netdatabot))
- Make offline installs properly offline again. [\#17602](https://github.com/netdata/netdata/pull/17602) ([Ferroin](https://github.com/Ferroin))
- remove python.d/smartd\_log [\#17600](https://github.com/netdata/netdata/pull/17600) ([ilyam8](https://github.com/ilyam8))
- go.d postgres: reset table/index bloat stats before querying [\#17598](https://github.com/netdata/netdata/pull/17598) ([ilyam8](https://github.com/ilyam8))
- Bump golang.org/x/text from 0.14.0 to 0.15.0 in /src/go/collectors/go.d.plugin [\#17596](https://github.com/netdata/netdata/pull/17596) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 26.1.0+incompatible to 26.1.1+incompatible in /src/go/collectors/go.d.plugin [\#17592](https://github.com/netdata/netdata/pull/17592) ([dependabot[bot]](https://github.com/apps/dependabot))
- Update eBPF code to v1.4.1. [\#17591](https://github.com/netdata/netdata/pull/17591) ([Ferroin](https://github.com/Ferroin))
- Fix handling of service startup in DEB packages. [\#17589](https://github.com/netdata/netdata/pull/17589) ([Ferroin](https://github.com/Ferroin))
- go.d/python.d respect all netdata log levels [\#17587](https://github.com/netdata/netdata/pull/17587) ([ilyam8](https://github.com/ilyam8))
- Fix DEB package conflict entries. [\#17584](https://github.com/netdata/netdata/pull/17584) ([Ferroin](https://github.com/Ferroin))
- fix ndsudo setuid bit for static builds [\#17583](https://github.com/netdata/netdata/pull/17583) ([ilyam8](https://github.com/ilyam8))
- fix table [\#17581](https://github.com/netdata/netdata/pull/17581) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Fix invalid item in postinst script for Netdata package. [\#17580](https://github.com/netdata/netdata/pull/17580) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17578](https://github.com/netdata/netdata/pull/17578) ([netdatabot](https://github.com/netdatabot))
- Cpack fixes [\#17576](https://github.com/netdata/netdata/pull/17576) ([vkalintiris](https://github.com/vkalintiris))
- Fix compilation without `dbengine` [\#17575](https://github.com/netdata/netdata/pull/17575) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix handling of netdata.conf on install in build system. [\#17572](https://github.com/netdata/netdata/pull/17572) ([Ferroin](https://github.com/Ferroin))
- Update Netdata subscription plans documentation [\#17571](https://github.com/netdata/netdata/pull/17571) ([Ancairon](https://github.com/Ancairon))
- go.d prometheus remove apostrophe in label values [\#17570](https://github.com/netdata/netdata/pull/17570) ([ilyam8](https://github.com/ilyam8))
- remove go.d symbol/debug info with RelWithDebInfo [\#17569](https://github.com/netdata/netdata/pull/17569) ([ilyam8](https://github.com/ilyam8))
- go.d smartctl add meta setup prerequisites [\#17568](https://github.com/netdata/netdata/pull/17568) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17567](https://github.com/netdata/netdata/pull/17567) ([netdatabot](https://github.com/netdatabot))
- Increase the message size to the spawn server [\#17566](https://github.com/netdata/netdata/pull/17566) ([stelfrag](https://github.com/stelfrag))
- go.d smartctl small improvements [\#17565](https://github.com/netdata/netdata/pull/17565) ([ilyam8](https://github.com/ilyam8))
- go.d smartctl improve units [\#17564](https://github.com/netdata/netdata/pull/17564) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17561](https://github.com/netdata/netdata/pull/17561) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#17560](https://github.com/netdata/netdata/pull/17560) ([netdatabot](https://github.com/netdatabot))
- Add OIDC docs [\#17557](https://github.com/netdata/netdata/pull/17557) ([car12o](https://github.com/car12o))
- Fix handling of vendored eBPF code in CMake. [\#17556](https://github.com/netdata/netdata/pull/17556) ([Ferroin](https://github.com/Ferroin))
- try hardcode docs links [\#17553](https://github.com/netdata/netdata/pull/17553) ([Ancairon](https://github.com/Ancairon))
- Notification section updates [\#17551](https://github.com/netdata/netdata/pull/17551) ([Ancairon](https://github.com/Ancairon))
- Adjust eBPF code. [\#17550](https://github.com/netdata/netdata/pull/17550) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix platform EOL check issue assignment. [\#17544](https://github.com/netdata/netdata/pull/17544) ([Ferroin](https://github.com/Ferroin))
- refresh the ML documentation and consolidate the two docs [\#17543](https://github.com/netdata/netdata/pull/17543) ([Ancairon](https://github.com/Ancairon))
- Bump github.com/likexian/whois from 1.15.2 to 1.15.3 in /src/go/collectors/go.d.plugin [\#17542](https://github.com/netdata/netdata/pull/17542) ([dependabot[bot]](https://github.com/apps/dependabot))
- Additional code cleanup [\#17541](https://github.com/netdata/netdata/pull/17541) ([stelfrag](https://github.com/stelfrag))
- docs: Add Ubuntu AArch64 that is missing from the list [\#17538](https://github.com/netdata/netdata/pull/17538) ([dgibbs64](https://github.com/dgibbs64))
- go.d smartctl [\#17536](https://github.com/netdata/netdata/pull/17536) ([ilyam8](https://github.com/ilyam8))
- Detect and use ld.mold instead of the system linker. [\#17534](https://github.com/netdata/netdata/pull/17534) ([Ferroin](https://github.com/Ferroin))
- Significantly simplify the protobuf handling in CMake. [\#17533](https://github.com/netdata/netdata/pull/17533) ([Ferroin](https://github.com/Ferroin))
- Clean up handling of compiler flags in CMake. [\#17532](https://github.com/netdata/netdata/pull/17532) ([Ferroin](https://github.com/Ferroin))
- add features section requested on Okta review [\#17531](https://github.com/netdata/netdata/pull/17531) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Don’t unnescesarily clean repo during static builds. [\#17530](https://github.com/netdata/netdata/pull/17530) ([Ferroin](https://github.com/Ferroin))
- Revert changes to ENABLE\_CLOUD option. [\#17528](https://github.com/netdata/netdata/pull/17528) ([Ferroin](https://github.com/Ferroin))
- fix \_ndpath in detect\_existing\_install\(\) [\#17527](https://github.com/netdata/netdata/pull/17527) ([ilyam8](https://github.com/ilyam8))
- go.d prometheus fix units for snmp\_exporter [\#17524](https://github.com/netdata/netdata/pull/17524) ([ilyam8](https://github.com/ilyam8))
- update the file [\#17522](https://github.com/netdata/netdata/pull/17522) ([Ancairon](https://github.com/Ancairon))
- Events tab documentation updates [\#17521](https://github.com/netdata/netdata/pull/17521) ([Ancairon](https://github.com/Ancairon))
- Bump github.com/docker/docker from 26.0.2+incompatible to 26.1.0+incompatible in /src/go/collectors/go.d.plugin [\#17520](https://github.com/netdata/netdata/pull/17520) ([dependabot[bot]](https://github.com/apps/dependabot))
- Anomaly Advisor documentation edits [\#17518](https://github.com/netdata/netdata/pull/17518) ([Ancairon](https://github.com/Ancairon))
- add smartctl to ndsudo [\#17515](https://github.com/netdata/netdata/pull/17515) ([ilyam8](https://github.com/ilyam8))
- Fix handling of kernel version detection in CMake. [\#17514](https://github.com/netdata/netdata/pull/17514) ([Ferroin](https://github.com/Ferroin))
- Work around MS’s broken infra in CI. [\#17513](https://github.com/netdata/netdata/pull/17513) ([Ferroin](https://github.com/Ferroin))
- Move handling of legacy eBPF programs into CMake. [\#17512](https://github.com/netdata/netdata/pull/17512) ([Ferroin](https://github.com/Ferroin))
- go.d traefik fix "got a SET but dimension does not exist" [\#17511](https://github.com/netdata/netdata/pull/17511) ([ilyam8](https://github.com/ilyam8))
- Documentation edits [\#17509](https://github.com/netdata/netdata/pull/17509) ([Ancairon](https://github.com/Ancairon))
- Report correct error code when data insert fails [\#17508](https://github.com/netdata/netdata/pull/17508) ([stelfrag](https://github.com/stelfrag))
- Sso improvements [\#17506](https://github.com/netdata/netdata/pull/17506) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Regenerate integrations.js [\#17505](https://github.com/netdata/netdata/pull/17505) ([netdatabot](https://github.com/netdatabot))
- Additional SQL code cleanup [\#17503](https://github.com/netdata/netdata/pull/17503) ([stelfrag](https://github.com/stelfrag))
- remove python.d/fail2ban [\#17502](https://github.com/netdata/netdata/pull/17502) ([ilyam8](https://github.com/ilyam8))
- add go.d fail2ban [\#17501](https://github.com/netdata/netdata/pull/17501) ([ilyam8](https://github.com/ilyam8))
- better redirect [\#17500](https://github.com/netdata/netdata/pull/17500) ([hugovalente-pm](https://github.com/hugovalente-pm))
- add fail2ban-client to ndsudo [\#17499](https://github.com/netdata/netdata/pull/17499) ([ilyam8](https://github.com/ilyam8))
- Update CMake to request new behavior for all policies through v3.28.0. [\#17496](https://github.com/netdata/netdata/pull/17496) ([Ferroin](https://github.com/Ferroin))
- Fix usage of sha256sum in static builds. [\#17495](https://github.com/netdata/netdata/pull/17495) ([Ferroin](https://github.com/Ferroin))
- add generic sso authenciation page and SP-initiated SSO on Okta [\#17494](https://github.com/netdata/netdata/pull/17494) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Public NC Spaces access fix [\#17492](https://github.com/netdata/netdata/pull/17492) ([ktsaou](https://github.com/ktsaou))
- go.d fix intelgpu with update\_every \> 3 [\#17491](https://github.com/netdata/netdata/pull/17491) ([ilyam8](https://github.com/ilyam8))
- move node-filter.md [\#17490](https://github.com/netdata/netdata/pull/17490) ([Ancairon](https://github.com/Ancairon))
- go.d update go.mod 1.22.0 and update some packages [\#17489](https://github.com/netdata/netdata/pull/17489) ([ilyam8](https://github.com/ilyam8))
- move netdata charts documentation to proper folder [\#17488](https://github.com/netdata/netdata/pull/17488) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17487](https://github.com/netdata/netdata/pull/17487) ([netdatabot](https://github.com/netdatabot))
- move netdata charts documentation to proper folder [\#17486](https://github.com/netdata/netdata/pull/17486) ([Ancairon](https://github.com/Ancairon))
- Move libbpf and eBPF CO-RE bundling into CMake. [\#17484](https://github.com/netdata/netdata/pull/17484) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17483](https://github.com/netdata/netdata/pull/17483) ([netdatabot](https://github.com/netdatabot))
- Fix labels name-only matching [\#17482](https://github.com/netdata/netdata/pull/17482) ([stelfrag](https://github.com/stelfrag))
- go.d nvidia\_smi: use XML format by default [\#17481](https://github.com/netdata/netdata/pull/17481) ([ilyam8](https://github.com/ilyam8))
- go.d pkg prometheus improve parsing err msg [\#17480](https://github.com/netdata/netdata/pull/17480) ([ilyam8](https://github.com/ilyam8))
- move dashboards file [\#17479](https://github.com/netdata/netdata/pull/17479) ([Ancairon](https://github.com/Ancairon))
- go.d windows add "vnode" to config schema [\#17478](https://github.com/netdata/netdata/pull/17478) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17477](https://github.com/netdata/netdata/pull/17477) ([netdatabot](https://github.com/netdatabot))
- move dashboards file [\#17476](https://github.com/netdata/netdata/pull/17476) ([Ancairon](https://github.com/Ancairon))
- Use CPack to generate Debian packages [\#17475](https://github.com/netdata/netdata/pull/17475) ([vkalintiris](https://github.com/vkalintiris))
- bump go toolchain to v1.22.0 in check-for-go-toolchain.sh [\#17474](https://github.com/netdata/netdata/pull/17474) ([ilyam8](https://github.com/ilyam8))
- remove python.d/sensors [\#17473](https://github.com/netdata/netdata/pull/17473) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17472](https://github.com/netdata/netdata/pull/17472) ([netdatabot](https://github.com/netdatabot))
- k8s doc edits [\#17471](https://github.com/netdata/netdata/pull/17471) ([Ancairon](https://github.com/Ancairon))
- Update Libbpf to 1.4 [\#17470](https://github.com/netdata/netdata/pull/17470) ([thiagoftsm](https://github.com/thiagoftsm))
- Bump github.com/likexian/whois-parser from 1.24.12 to 1.24.15 in /src/go/collectors/go.d.plugin [\#17469](https://github.com/netdata/netdata/pull/17469) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d add sensors [\#17466](https://github.com/netdata/netdata/pull/17466) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17465](https://github.com/netdata/netdata/pull/17465) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#17464](https://github.com/netdata/netdata/pull/17464) ([netdatabot](https://github.com/netdatabot))
- remove python.d/hddtemp [\#17463](https://github.com/netdata/netdata/pull/17463) ([ilyam8](https://github.com/ilyam8))
- go.d add hddtemp [\#17462](https://github.com/netdata/netdata/pull/17462) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17461](https://github.com/netdata/netdata/pull/17461) ([netdatabot](https://github.com/netdatabot))
- go.d storcli update [\#17460](https://github.com/netdata/netdata/pull/17460) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17458](https://github.com/netdata/netdata/pull/17458) ([netdatabot](https://github.com/netdatabot))
- add section for regenerate claiming token [\#17457](https://github.com/netdata/netdata/pull/17457) ([hugovalente-pm](https://github.com/hugovalente-pm))
- ndsudo add storcli [\#17455](https://github.com/netdata/netdata/pull/17455) ([ilyam8](https://github.com/ilyam8))
- go.d add storcli collector [\#17454](https://github.com/netdata/netdata/pull/17454) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/vmware/govmomi from 0.37.0 to 0.37.1 in /src/go/collectors/go.d.plugin [\#17451](https://github.com/netdata/netdata/pull/17451) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/miekg/dns from 1.1.58 to 1.1.59 in /src/go/collectors/go.d.plugin [\#17449](https://github.com/netdata/netdata/pull/17449) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.52.3 to 0.53.0 in /src/go/collectors/go.d.plugin [\#17448](https://github.com/netdata/netdata/pull/17448) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 26.0.1+incompatible to 26.0.2+incompatible in /src/go/collectors/go.d.plugin [\#17447](https://github.com/netdata/netdata/pull/17447) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#17446](https://github.com/netdata/netdata/pull/17446) ([netdatabot](https://github.com/netdatabot))
- Add documentation for VictorOps cloud notifications [\#17445](https://github.com/netdata/netdata/pull/17445) ([juacker](https://github.com/juacker))
- Reconnect to the cloud when resuming from suspension [\#17444](https://github.com/netdata/netdata/pull/17444) ([stelfrag](https://github.com/stelfrag))
- timex is not supported on windows. [\#17443](https://github.com/netdata/netdata/pull/17443) ([vkalintiris](https://github.com/vkalintiris))
- Clean up CMake build options. [\#17442](https://github.com/netdata/netdata/pull/17442) ([Ferroin](https://github.com/Ferroin))
- Fix maintainer documentation to reflect the new build system. [\#17441](https://github.com/netdata/netdata/pull/17441) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17439](https://github.com/netdata/netdata/pull/17439) ([netdatabot](https://github.com/netdatabot))
- go.d mega/adaptec meta add alerts [\#17438](https://github.com/netdata/netdata/pull/17438) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17437](https://github.com/netdata/netdata/pull/17437) ([netdatabot](https://github.com/netdatabot))
- Start watcher thread after fork [\#17436](https://github.com/netdata/netdata/pull/17436) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#17434](https://github.com/netdata/netdata/pull/17434) ([netdatabot](https://github.com/netdatabot))
- go.d fix adaptec/megacli meta name [\#17433](https://github.com/netdata/netdata/pull/17433) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17432](https://github.com/netdata/netdata/pull/17432) ([netdatabot](https://github.com/netdatabot))
- go.d adaptec fix meta [\#17430](https://github.com/netdata/netdata/pull/17430) ([ilyam8](https://github.com/ilyam8))
- remove python.d/adaptec\_raid [\#17429](https://github.com/netdata/netdata/pull/17429) ([ilyam8](https://github.com/ilyam8))
- go.d rewrite python.d/adaptec\_raid [\#17428](https://github.com/netdata/netdata/pull/17428) ([ilyam8](https://github.com/ilyam8))
- cncf changed the url [\#17427](https://github.com/netdata/netdata/pull/17427) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Regenerate integrations.js [\#17425](https://github.com/netdata/netdata/pull/17425) ([netdatabot](https://github.com/netdatabot))
- Fix coverity issue 425241 [\#17424](https://github.com/netdata/netdata/pull/17424) ([stelfrag](https://github.com/stelfrag))
- Indent generated files [\#17423](https://github.com/netdata/netdata/pull/17423) ([vkalintiris](https://github.com/vkalintiris))
- Associate sentry events with guid. [\#17420](https://github.com/netdata/netdata/pull/17420) ([vkalintiris](https://github.com/vkalintiris))
- go.d megacli health fix megacli\_phys\_drive\_media\_errors [\#17419](https://github.com/netdata/netdata/pull/17419) ([ilyam8](https://github.com/ilyam8))
- go.d megacli fix meta metrics\_description [\#17418](https://github.com/netdata/netdata/pull/17418) ([ilyam8](https://github.com/ilyam8))
- remove python.d/megacli [\#17417](https://github.com/netdata/netdata/pull/17417) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17416](https://github.com/netdata/netdata/pull/17416) ([netdatabot](https://github.com/netdatabot))
- go.d sd ll set max\_time\_series for prometheus/clickhouse [\#17415](https://github.com/netdata/netdata/pull/17415) ([ilyam8](https://github.com/ilyam8))
- rewrite megacli in go [\#17410](https://github.com/netdata/netdata/pull/17410) ([ilyam8](https://github.com/ilyam8))
- dashboards doc edits [\#17409](https://github.com/netdata/netdata/pull/17409) ([Ancairon](https://github.com/Ancairon))
- Logs tab docs in dashboard section [\#17408](https://github.com/netdata/netdata/pull/17408) ([Ancairon](https://github.com/Ancairon))
- gh labeler: go.d.plugin rm suffix [\#17407](https://github.com/netdata/netdata/pull/17407) ([ilyam8](https://github.com/ilyam8))
- Bump go.mongodb.org/mongo-driver from 1.14.0 to 1.15.0 in /src/go/collectors/go.d.plugin [\#17406](https://github.com/netdata/netdata/pull/17406) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.52.2 to 0.52.3 in /src/go/collectors/go.d.plugin [\#17405](https://github.com/netdata/netdata/pull/17405) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.36.3 to 0.37.0 in /src/go/collectors/go.d.plugin [\#17404](https://github.com/netdata/netdata/pull/17404) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/docker/docker from 26.0.0+incompatible to 26.0.1+incompatible in /src/go/collectors/go.d.plugin [\#17403](https://github.com/netdata/netdata/pull/17403) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#17399](https://github.com/netdata/netdata/pull/17399) ([netdatabot](https://github.com/netdatabot))
- apply first alarms, then alarm templates [\#17398](https://github.com/netdata/netdata/pull/17398) ([ktsaou](https://github.com/ktsaou))
- Fix publishing Docker images to Docker Hub. [\#17397](https://github.com/netdata/netdata/pull/17397) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17396](https://github.com/netdata/netdata/pull/17396) ([netdatabot](https://github.com/netdatabot))
- go.d nvme meta: remove sudoers prereq [\#17395](https://github.com/netdata/netdata/pull/17395) ([ilyam8](https://github.com/ilyam8))
- add simple collector to monitor lvm thin volumes space usage [\#17394](https://github.com/netdata/netdata/pull/17394) ([ilyam8](https://github.com/ilyam8))
- fix percentages on alerts [\#17391](https://github.com/netdata/netdata/pull/17391) ([ktsaou](https://github.com/ktsaou))
- Function docs edits [\#17390](https://github.com/netdata/netdata/pull/17390) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17387](https://github.com/netdata/netdata/pull/17387) ([netdatabot](https://github.com/netdatabot))
- go.d nvme: drop using `nvme` directly [\#17386](https://github.com/netdata/netdata/pull/17386) ([ilyam8](https://github.com/ilyam8))
- Add option to cleanup health\_log table  [\#17385](https://github.com/netdata/netdata/pull/17385) ([stelfrag](https://github.com/stelfrag))
- go.d intelgpu: use cmd.Wait instead of process.Wait [\#17384](https://github.com/netdata/netdata/pull/17384) ([ilyam8](https://github.com/ilyam8))
- Split Sentry enablement to be per-architecture. [\#17383](https://github.com/netdata/netdata/pull/17383) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17382](https://github.com/netdata/netdata/pull/17382) ([netdatabot](https://github.com/netdatabot))
- lowercase word [\#17381](https://github.com/netdata/netdata/pull/17381) ([Ancairon](https://github.com/Ancairon))
- go.d intelgpu switch to using ndsudo [\#17380](https://github.com/netdata/netdata/pull/17380) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17379](https://github.com/netdata/netdata/pull/17379) ([netdatabot](https://github.com/netdatabot))
- go.d intelgpu meta update icon [\#17378](https://github.com/netdata/netdata/pull/17378) ([ilyam8](https://github.com/ilyam8))
- hardcode ndsudo PATH [\#17377](https://github.com/netdata/netdata/pull/17377) ([ilyam8](https://github.com/ilyam8))
- add intel\_gpu\_top to ndsudo [\#17376](https://github.com/netdata/netdata/pull/17376) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17375](https://github.com/netdata/netdata/pull/17375) ([netdatabot](https://github.com/netdatabot))
- Canonicalize paths before comparison when checking for multiple installs. [\#17373](https://github.com/netdata/netdata/pull/17373) ([Ferroin](https://github.com/Ferroin))
- go.d zfspool minor schema and meta fixes [\#17372](https://github.com/netdata/netdata/pull/17372) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17371](https://github.com/netdata/netdata/pull/17371) ([netdatabot](https://github.com/netdatabot))
- go.d intelgpu: uncomment stock config and update meta setup prerequisites [\#17370](https://github.com/netdata/netdata/pull/17370) ([ilyam8](https://github.com/ilyam8))
- Fix logic in detection of multiple installs. [\#17369](https://github.com/netdata/netdata/pull/17369) ([Ferroin](https://github.com/Ferroin))
- add collector to monitor ZFS pools space usage [\#17367](https://github.com/netdata/netdata/pull/17367) ([ilyam8](https://github.com/ilyam8))
- Fix required toolchain version for Go code. [\#17366](https://github.com/netdata/netdata/pull/17366) ([Ferroin](https://github.com/Ferroin))
- Metrics tab doc [\#17365](https://github.com/netdata/netdata/pull/17365) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17364](https://github.com/netdata/netdata/pull/17364) ([netdatabot](https://github.com/netdatabot))
- Regenerate integrations.js [\#17362](https://github.com/netdata/netdata/pull/17362) ([netdatabot](https://github.com/netdatabot))
- go.d intelgpu: rename label engine to engine\_class [\#17361](https://github.com/netdata/netdata/pull/17361) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17360](https://github.com/netdata/netdata/pull/17360) ([netdatabot](https://github.com/netdatabot))
- make file if not found [\#17359](https://github.com/netdata/netdata/pull/17359) ([Ancairon](https://github.com/Ancairon))
- Move vendoring of Sentry to it’s own module and switch to using Git instead of the releases page. [\#17358](https://github.com/netdata/netdata/pull/17358) ([Ferroin](https://github.com/Ferroin))
- uninstaller: remove LaunchDaemons plist file \(macOS\) [\#17357](https://github.com/netdata/netdata/pull/17357) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17356](https://github.com/netdata/netdata/pull/17356) ([netdatabot](https://github.com/netdatabot))
- add prerequisites to go.d/intelgpu meta [\#17355](https://github.com/netdata/netdata/pull/17355) ([ilyam8](https://github.com/ilyam8))
- add intel\_gpu\_top to apps\_groups.conf [\#17354](https://github.com/netdata/netdata/pull/17354) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17353](https://github.com/netdata/netdata/pull/17353) ([netdatabot](https://github.com/netdatabot))
- add try except [\#17352](https://github.com/netdata/netdata/pull/17352) ([Ancairon](https://github.com/Ancairon))
- add Okta SSO integration [\#17351](https://github.com/netdata/netdata/pull/17351) ([hugovalente-pm](https://github.com/hugovalente-pm))
- revised node-tab document [\#17348](https://github.com/netdata/netdata/pull/17348) ([Ancairon](https://github.com/Ancairon))
- add intel\_gpu\_top collector [\#17344](https://github.com/netdata/netdata/pull/17344) ([ilyam8](https://github.com/ilyam8))
- Increase number of pages per extent in dbengine [\#17343](https://github.com/netdata/netdata/pull/17343) ([stelfrag](https://github.com/stelfrag))
- fix invalid var in prepare\_offline\_install\_source\(\) [\#17342](https://github.com/netdata/netdata/pull/17342) ([ilyam8](https://github.com/ilyam8))
- remove unused install\_go.sh [\#17339](https://github.com/netdata/netdata/pull/17339) ([ilyam8](https://github.com/ilyam8))
- fix proc-power-supply charts family [\#17338](https://github.com/netdata/netdata/pull/17338) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/likexian/whois from 1.15.1 to 1.15.2 in /src/go/collectors/go.d.plugin [\#17337](https://github.com/netdata/netdata/pull/17337) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.36.2 to 0.36.3 in /src/go/collectors/go.d.plugin [\#17336](https://github.com/netdata/netdata/pull/17336) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.51.1 to 0.52.2 in /src/go/collectors/go.d.plugin [\#17335](https://github.com/netdata/netdata/pull/17335) ([dependabot[bot]](https://github.com/apps/dependabot))
- Add repo config for Amazon Linux 2023. [\#17330](https://github.com/netdata/netdata/pull/17330) ([PaulSzymanski](https://github.com/PaulSzymanski))
- Add power consumption metric to power supply monitoring module [\#17329](https://github.com/netdata/netdata/pull/17329) ([eyusupov](https://github.com/eyusupov))
- Enable Sentry for Ubuntu and Debian native packages. [\#17327](https://github.com/netdata/netdata/pull/17327) ([Ferroin](https://github.com/Ferroin))
- go.d: schema windows: fix url placeholder scheme [\#17326](https://github.com/netdata/netdata/pull/17326) ([ilyam8](https://github.com/ilyam8))
- go.d: schemas: add missing "body" and "method" [\#17325](https://github.com/netdata/netdata/pull/17325) ([ilyam8](https://github.com/ilyam8))
- remove old overview infrastructure and add home tab doc [\#17323](https://github.com/netdata/netdata/pull/17323) ([Ancairon](https://github.com/Ancairon))
- Drop generic bitmap implementation. [\#17322](https://github.com/netdata/netdata/pull/17322) ([vkalintiris](https://github.com/vkalintiris))
- Remove seemingly dead logging related code from libnetdata. [\#17320](https://github.com/netdata/netdata/pull/17320) ([Ferroin](https://github.com/Ferroin))
- Check for Snappy only when required. [\#17319](https://github.com/netdata/netdata/pull/17319) ([Ferroin](https://github.com/Ferroin))
- set min thread stack size to 1 MB [\#17317](https://github.com/netdata/netdata/pull/17317) ([ilyam8](https://github.com/ilyam8))
- Call with resize true when dictionary has DICT\_OPTION\_INDEX\_HASHTABLE [\#17316](https://github.com/netdata/netdata/pull/17316) ([stelfrag](https://github.com/stelfrag))
- Drop legacy dbengine support [\#17315](https://github.com/netdata/netdata/pull/17315) ([stelfrag](https://github.com/stelfrag))
- Address cmake compilation [\#17314](https://github.com/netdata/netdata/pull/17314) ([thiagoftsm](https://github.com/thiagoftsm))
- Explicitly require systemd for systemd journal plugin. [\#17313](https://github.com/netdata/netdata/pull/17313) ([Ferroin](https://github.com/Ferroin))
- Fix assorted issues in the Docker build process. [\#17312](https://github.com/netdata/netdata/pull/17312) ([Ferroin](https://github.com/Ferroin))
- dyncfg function on parents should not require any access rights [\#17310](https://github.com/netdata/netdata/pull/17310) ([ktsaou](https://github.com/ktsaou))
- Add a build option to disable all optional features by default. [\#17309](https://github.com/netdata/netdata/pull/17309) ([Ferroin](https://github.com/Ferroin))
- Update metadata frequency [\#17307](https://github.com/netdata/netdata/pull/17307) ([stelfrag](https://github.com/stelfrag))
- Fix handling of post-release workflows triggered by Docker workflow. [\#17306](https://github.com/netdata/netdata/pull/17306) ([Ferroin](https://github.com/Ferroin))
- go.d: sd ll: add mysql socket jobs [\#17305](https://github.com/netdata/netdata/pull/17305) ([ilyam8](https://github.com/ilyam8))
- go.d: sd local listeners: add unix socket job [\#17304](https://github.com/netdata/netdata/pull/17304) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/vmware/govmomi from 0.36.1 to 0.36.2 in /src/go/collectors/go.d.plugin [\#17300](https://github.com/netdata/netdata/pull/17300) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/go-sql-driver/mysql from 1.8.0 to 1.8.1 in /src/go/collectors/go.d.plugin [\#17299](https://github.com/netdata/netdata/pull/17299) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix SWAP pages [\#17295](https://github.com/netdata/netdata/pull/17295) ([thiagoftsm](https://github.com/thiagoftsm))
- Update hpssa.chart.py [\#17294](https://github.com/netdata/netdata/pull/17294) ([Metric-Void](https://github.com/Metric-Void))
- fix rrdlabels traversal [\#17292](https://github.com/netdata/netdata/pull/17292) ([ktsaou](https://github.com/ktsaou))
- fix positive and negative matches on labels [\#17290](https://github.com/netdata/netdata/pull/17290) ([ktsaou](https://github.com/ktsaou))
- go.d: don't create jobs with unknown module [\#17289](https://github.com/netdata/netdata/pull/17289) ([ilyam8](https://github.com/ilyam8))
- Fix repoconfig publishing. [\#17288](https://github.com/netdata/netdata/pull/17288) ([Ferroin](https://github.com/Ferroin))
- go.d: set User-Agent automatically when creating HTTP req [\#17286](https://github.com/netdata/netdata/pull/17286) ([ilyam8](https://github.com/ilyam8))
- go.d: sd docker: create multiple nginx configs [\#17285](https://github.com/netdata/netdata/pull/17285) ([ilyam8](https://github.com/ilyam8))
- go.d: socket package: don't set client on connect\(\) err [\#17283](https://github.com/netdata/netdata/pull/17283) ([ilyam8](https://github.com/ilyam8))
- Add Fedora 40 to CI, packages, and support policy. [\#17282](https://github.com/netdata/netdata/pull/17282) ([Ferroin](https://github.com/Ferroin))
- Add Ubuntu 24.04 to CI, package builds, and support policy. [\#17281](https://github.com/netdata/netdata/pull/17281) ([Ferroin](https://github.com/Ferroin))
- Revert "Enable sentry on all Debian and Ubuntu versions." [\#17279](https://github.com/netdata/netdata/pull/17279) ([tkatsoulas](https://github.com/tkatsoulas))
- Correctly handle libyaml linking for log2journal. [\#17276](https://github.com/netdata/netdata/pull/17276) ([Ferroin](https://github.com/Ferroin))
- REFERENCE - Fix small unligned typo [\#17274](https://github.com/netdata/netdata/pull/17274) ([sepek](https://github.com/sepek))
- Regenerate integrations.js [\#17273](https://github.com/netdata/netdata/pull/17273) ([netdatabot](https://github.com/netdatabot))
- go.d: dyncfg: allow "name" additional property [\#17272](https://github.com/netdata/netdata/pull/17272) ([ilyam8](https://github.com/ilyam8))
- add additional fields to webhook reachability notifications [\#17271](https://github.com/netdata/netdata/pull/17271) ([juacker](https://github.com/juacker))
- Update Codeowners [\#17270](https://github.com/netdata/netdata/pull/17270) ([tkatsoulas](https://github.com/tkatsoulas))
- go.d: config schemas update: prohibit additional properties [\#17269](https://github.com/netdata/netdata/pull/17269) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17268](https://github.com/netdata/netdata/pull/17268) ([netdatabot](https://github.com/netdatabot))
- fix duplicate chart context [\#17267](https://github.com/netdata/netdata/pull/17267) ([ilyam8](https://github.com/ilyam8))
- Reset database connection handle on close [\#17266](https://github.com/netdata/netdata/pull/17266) ([stelfrag](https://github.com/stelfrag))
- docs: update pagerduty meta [\#17264](https://github.com/netdata/netdata/pull/17264) ([ilyam8](https://github.com/ilyam8))
- Correctly propagate errors from child scripts in kickstart.sh. [\#17263](https://github.com/netdata/netdata/pull/17263) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17261](https://github.com/netdata/netdata/pull/17261) ([netdatabot](https://github.com/netdatabot))
- Enable sentry on all Debian and Ubuntu versions. [\#17259](https://github.com/netdata/netdata/pull/17259) ([vkalintiris](https://github.com/vkalintiris))
- include reachability alert fields [\#17258](https://github.com/netdata/netdata/pull/17258) ([juacker](https://github.com/juacker))
- Add dbengine compression info in -W buildinfo [\#17257](https://github.com/netdata/netdata/pull/17257) ([stelfrag](https://github.com/stelfrag))
- minor fix in monitor releases workflow [\#17256](https://github.com/netdata/netdata/pull/17256) ([tkatsoulas](https://github.com/tkatsoulas))
- go.d: job manager: set config defaults [\#17255](https://github.com/netdata/netdata/pull/17255) ([ilyam8](https://github.com/ilyam8))
- go.d: sd local-listeners: drop docker-proxy targets [\#17254](https://github.com/netdata/netdata/pull/17254) ([ilyam8](https://github.com/ilyam8))
- go.d: sd local-listeners: discover /proc/net/tcp6 only apps [\#17252](https://github.com/netdata/netdata/pull/17252) ([ilyam8](https://github.com/ilyam8))
- update whats new [\#17251](https://github.com/netdata/netdata/pull/17251) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Try finding OpenSSL using pkg-config first on macOS. [\#17250](https://github.com/netdata/netdata/pull/17250) ([Ferroin](https://github.com/Ferroin))
- remove USR1 "Save internal DB to disk" [\#17249](https://github.com/netdata/netdata/pull/17249) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/docker/docker from 25.0.5+incompatible to 26.0.0+incompatible in /src/go/collectors/go.d.plugin [\#17248](https://github.com/netdata/netdata/pull/17248) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.50.0 to 0.51.1 in /src/go/collectors/go.d.plugin [\#17247](https://github.com/netdata/netdata/pull/17247) ([dependabot[bot]](https://github.com/apps/dependabot))
- DBENGINE: support ZSTD compression [\#17244](https://github.com/netdata/netdata/pull/17244) ([ktsaou](https://github.com/ktsaou))
- apps\_proc\_pid\_fd: ignore kf\_sock\_inpcb on modern FreeBSD [\#17243](https://github.com/netdata/netdata/pull/17243) ([glebius](https://github.com/glebius))
- Fix MRG Metric refcount issue [\#17239](https://github.com/netdata/netdata/pull/17239) ([ktsaou](https://github.com/ktsaou))
- Code cleanup [\#17237](https://github.com/netdata/netdata/pull/17237) ([ktsaou](https://github.com/ktsaou))
- DBENGINE: use gorilla by default [\#17234](https://github.com/netdata/netdata/pull/17234) ([ktsaou](https://github.com/ktsaou))
- updated dbengine unittest [\#17232](https://github.com/netdata/netdata/pull/17232) ([ktsaou](https://github.com/ktsaou))
- dbengine: cache bug-fix when under pressure [\#17231](https://github.com/netdata/netdata/pull/17231) ([ktsaou](https://github.com/ktsaou))
- fix html [\#17228](https://github.com/netdata/netdata/pull/17228) ([Ancairon](https://github.com/Ancairon))
- update flowchart cloud-onprem [\#17227](https://github.com/netdata/netdata/pull/17227) ([M4itee](https://github.com/M4itee))
- feat: add netdata cloud api-tokens docs [\#17225](https://github.com/netdata/netdata/pull/17225) ([witalisoft](https://github.com/witalisoft))
- Trigger subsequent workflows for Helmchart and MSI [\#17224](https://github.com/netdata/netdata/pull/17224) ([tkatsoulas](https://github.com/tkatsoulas))
- fixing the helm login command for the onprem installation [\#17222](https://github.com/netdata/netdata/pull/17222) ([M4itee](https://github.com/M4itee))
- Reduce flush operations during journal build [\#17220](https://github.com/netdata/netdata/pull/17220) ([stelfrag](https://github.com/stelfrag))
- go.d: mysql: disable session query log and slow query log [\#17219](https://github.com/netdata/netdata/pull/17219) ([ilyam8](https://github.com/ilyam8))
- go.d: local-listeners sd: fix mariadbd comm [\#17218](https://github.com/netdata/netdata/pull/17218) ([ilyam8](https://github.com/ilyam8))
- Assorted macOS build fixes. [\#17216](https://github.com/netdata/netdata/pull/17216) ([Ferroin](https://github.com/Ferroin))
- Fix job depdendencies in Docker workflow. [\#17215](https://github.com/netdata/netdata/pull/17215) ([Ferroin](https://github.com/Ferroin))
- add warning on old custom dashboards and rephrase existing page [\#17214](https://github.com/netdata/netdata/pull/17214) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Bump github.com/docker/docker from 25.0.4+incompatible to 25.0.5+incompatible in /src/go/collectors/go.d.plugin [\#17211](https://github.com/netdata/netdata/pull/17211) ([dependabot[bot]](https://github.com/apps/dependabot))
- Add -Wno-builtin-macro-redefined to compiler flags. [\#17209](https://github.com/netdata/netdata/pull/17209) ([Ferroin](https://github.com/Ferroin))
- Move bundling of JSON-C to CMake. [\#17207](https://github.com/netdata/netdata/pull/17207) ([Ferroin](https://github.com/Ferroin))
- Compatibility with Prometheus HELP [\#17191](https://github.com/netdata/netdata/pull/17191) ([thiagoftsm](https://github.com/thiagoftsm))
- Add label for cgroup [\#17156](https://github.com/netdata/netdata/pull/17156) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix action lints [\#17120](https://github.com/netdata/netdata/pull/17120) ([tkatsoulas](https://github.com/tkatsoulas))
- Alert transitions code cleanup [\#17103](https://github.com/netdata/netdata/pull/17103) ([stelfrag](https://github.com/stelfrag))
- Skip Go code in CI if it hasn’t changed. [\#17077](https://github.com/netdata/netdata/pull/17077) ([Ferroin](https://github.com/Ferroin))
- Fix conditional for Amazon Linux 2023 in repoconfig spec file. [\#17056](https://github.com/netdata/netdata/pull/17056) ([PaulSzymanski](https://github.com/PaulSzymanski))
- Add fallback logic in installer for fetching files. [\#17045](https://github.com/netdata/netdata/pull/17045) ([Ferroin](https://github.com/Ferroin))

## [v1.45.3](https://github.com/netdata/netdata/tree/v1.45.3) (2024-04-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.2...v1.45.3)

## [v1.45.2](https://github.com/netdata/netdata/tree/v1.45.2) (2024-04-01)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.1...v1.45.2)

## [v1.45.1](https://github.com/netdata/netdata/tree/v1.45.1) (2024-03-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.0...v1.45.1)

## [v1.45.0](https://github.com/netdata/netdata/tree/v1.45.0) (2024-03-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.3...v1.45.0)

**Merged pull requests:**

- Dynamic configuration switch to version 2 [\#17212](https://github.com/netdata/netdata/pull/17212) ([stelfrag](https://github.com/stelfrag))
- update bundled UI to v6.104.1 [\#17208](https://github.com/netdata/netdata/pull/17208) ([ilyam8](https://github.com/ilyam8))
- go.d: adjust dyncfg return codes [\#17206](https://github.com/netdata/netdata/pull/17206) ([ilyam8](https://github.com/ilyam8))
- go.d: local-listeners sd: trust known ports to identify an app [\#17205](https://github.com/netdata/netdata/pull/17205) ([ilyam8](https://github.com/ilyam8))
- go.d: weblog allow PURGE HTTP method [\#17204](https://github.com/netdata/netdata/pull/17204) ([ilyam8](https://github.com/ilyam8))
- go.d: local-listeners sd: use "ip:port" as address instead of "localhost" [\#17203](https://github.com/netdata/netdata/pull/17203) ([ilyam8](https://github.com/ilyam8))
- Fix issues with permissions when installing from source on macOS [\#17198](https://github.com/netdata/netdata/pull/17198) ([ilyam8](https://github.com/ilyam8))
- Handle agents will wrong alert\_hash table definition [\#17197](https://github.com/netdata/netdata/pull/17197) ([stelfrag](https://github.com/stelfrag))
- Fix alert hash table definition [\#17196](https://github.com/netdata/netdata/pull/17196) ([stelfrag](https://github.com/stelfrag))
- health: unsilence cpu % alarm [\#17194](https://github.com/netdata/netdata/pull/17194) ([ilyam8](https://github.com/ilyam8))
- Fix sum calculation in rrdr2value [\#17193](https://github.com/netdata/netdata/pull/17193) ([stelfrag](https://github.com/stelfrag))
- Move bundling of libyaml to CMake. [\#17190](https://github.com/netdata/netdata/pull/17190) ([Ferroin](https://github.com/Ferroin))
- add callout that snapshots only available on v1 [\#17189](https://github.com/netdata/netdata/pull/17189) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Bump github.com/vmware/govmomi from 0.36.0 to 0.36.1 in /src/go/collectors/go.d.plugin [\#17185](https://github.com/netdata/netdata/pull/17185) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump k8s.io/client-go from 0.29.2 to 0.29.3 in /src/go/collectors/go.d.plugin [\#17184](https://github.com/netdata/netdata/pull/17184) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus/common from 0.48.0 to 0.50.0 in /src/go/collectors/go.d.plugin [\#17182](https://github.com/netdata/netdata/pull/17182) ([dependabot[bot]](https://github.com/apps/dependabot))
- split apps.plugin into multiple files and support MacOS [\#17180](https://github.com/netdata/netdata/pull/17180) ([ktsaou](https://github.com/ktsaou))
- Update themes.md [\#17176](https://github.com/netdata/netdata/pull/17176) ([Ancairon](https://github.com/Ancairon))
- go.d sd docker use well-known port for app identification too [\#17174](https://github.com/netdata/netdata/pull/17174) ([ilyam8](https://github.com/ilyam8))
- go.d sd docker add mongodb-community-server [\#17173](https://github.com/netdata/netdata/pull/17173) ([ilyam8](https://github.com/ilyam8))
- Update themes.md [\#17172](https://github.com/netdata/netdata/pull/17172) ([Ancairon](https://github.com/Ancairon))
- go.d sd config add "disabled" [\#17171](https://github.com/netdata/netdata/pull/17171) ([ilyam8](https://github.com/ilyam8))
- docs: add "With NVIDIA GPUs monitoring" to docker install [\#17167](https://github.com/netdata/netdata/pull/17167) ([ilyam8](https://github.com/ilyam8))
- go.d.plugin: jsonschema allow array/object to be null [\#17166](https://github.com/netdata/netdata/pull/17166) ([ilyam8](https://github.com/ilyam8))
- DYNCFG: alerts improvements [\#17165](https://github.com/netdata/netdata/pull/17165) ([ktsaou](https://github.com/ktsaou))
- go.d.plugin: update file path pattern in jsonschema [\#17164](https://github.com/netdata/netdata/pull/17164) ([ilyam8](https://github.com/ilyam8))
- Announce dynamic configuration capability to the cloud [\#17162](https://github.com/netdata/netdata/pull/17162) ([stelfrag](https://github.com/stelfrag))
- go.d.plugin: execute local-listeners periodically [\#17160](https://github.com/netdata/netdata/pull/17160) ([ilyam8](https://github.com/ilyam8))
- Install the correct service file based on systemd version [\#17159](https://github.com/netdata/netdata/pull/17159) ([tkatsoulas](https://github.com/tkatsoulas))
- go.d.plugin: sd compose: allow multi config template [\#17157](https://github.com/netdata/netdata/pull/17157) ([ilyam8](https://github.com/ilyam8))
- Bump google.golang.org/protobuf from 1.32.0 to 1.33.0 in /src/go/collectors/go.d.plugin [\#17154](https://github.com/netdata/netdata/pull/17154) ([dependabot[bot]](https://github.com/apps/dependabot))
- Improve offline install error handling. [\#17153](https://github.com/netdata/netdata/pull/17153) ([Ferroin](https://github.com/Ferroin))
- go.d.plugin: add docker service discovery [\#17152](https://github.com/netdata/netdata/pull/17152) ([ilyam8](https://github.com/ilyam8))
- Fix macOS issue with SOCK\_CLOEXEC [\#17151](https://github.com/netdata/netdata/pull/17151) ([stelfrag](https://github.com/stelfrag))
- Document new field on PagerDuty cloud integration [\#17149](https://github.com/netdata/netdata/pull/17149) ([juacker](https://github.com/juacker))
- bring back old docs that were containing missing information [\#17146](https://github.com/netdata/netdata/pull/17146) ([Ancairon](https://github.com/Ancairon))
- Update go.d.plugin packages [\#17145](https://github.com/netdata/netdata/pull/17145) ([ilyam8](https://github.com/ilyam8))
- Check alert duration on submission to the cloud [\#17144](https://github.com/netdata/netdata/pull/17144) ([stelfrag](https://github.com/stelfrag))
- Bump github.com/go-sql-driver/mysql from 1.7.1 to 1.8.0 in /src/go/collectors/go.d.plugin [\#17142](https://github.com/netdata/netdata/pull/17142) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/prometheus-community/pro-bing from 0.3.0 to 0.4.0 in /src/go/collectors/go.d.plugin [\#17141](https://github.com/netdata/netdata/pull/17141) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/vmware/govmomi from 0.35.0 to 0.36.0 in /src/go/collectors/go.d.plugin [\#17140](https://github.com/netdata/netdata/pull/17140) ([ilyam8](https://github.com/ilyam8))
- Add macos check \(build from source\) [\#17139](https://github.com/netdata/netdata/pull/17139) ([tkatsoulas](https://github.com/tkatsoulas))
- Bump github.com/likexian/whois-parser from 1.24.10 to 1.24.11 in /src/go/collectors/go.d.plugin [\#17137](https://github.com/netdata/netdata/pull/17137) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/cloudflare/cfssl from 1.6.4 to 1.6.5 in /src/go/collectors/go.d.plugin [\#17136](https://github.com/netdata/netdata/pull/17136) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump github.com/jackc/pgx/v4 from 4.18.1 to 4.18.3 in /src/go/collectors/go.d.plugin [\#17135](https://github.com/netdata/netdata/pull/17135) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump golang.org/x/net from 0.21.0 to 0.22.0 in /src/go/collectors/go.d.plugin [\#17134](https://github.com/netdata/netdata/pull/17134) ([dependabot[bot]](https://github.com/apps/dependabot))
- remove repetitive words [\#17131](https://github.com/netdata/netdata/pull/17131) ([carrychair](https://github.com/carrychair))
- packaging: remove Suggests nut [\#17129](https://github.com/netdata/netdata/pull/17129) ([ilyam8](https://github.com/ilyam8))
- Prefer Protobuf’s own CMake config over CMake's FindProtobuf. [\#17128](https://github.com/netdata/netdata/pull/17128) ([Ferroin](https://github.com/Ferroin))
- Fix login to GHCR when publishing Docker images. [\#17127](https://github.com/netdata/netdata/pull/17127) ([Ferroin](https://github.com/Ferroin))
- Detect self thread when exiting. [\#17126](https://github.com/netdata/netdata/pull/17126) ([vkalintiris](https://github.com/vkalintiris))
- fix health alert dyncfg schema fullPage option [\#17125](https://github.com/netdata/netdata/pull/17125) ([ilyam8](https://github.com/ilyam8))
- improve go.d.plugin dyncfg config schemas [\#17124](https://github.com/netdata/netdata/pull/17124) ([ilyam8](https://github.com/ilyam8))
- minor fix; broken link on on prem installation doc [\#17118](https://github.com/netdata/netdata/pull/17118) ([tkatsoulas](https://github.com/tkatsoulas))
- very minor docs update [\#17117](https://github.com/netdata/netdata/pull/17117) ([Ancairon](https://github.com/Ancairon))
- remove deprecated settings from the health ref doc [\#17116](https://github.com/netdata/netdata/pull/17116) ([ilyam8](https://github.com/ilyam8))
- fix discovered config default values [\#17115](https://github.com/netdata/netdata/pull/17115) ([ilyam8](https://github.com/ilyam8))
- Fix memory leak [\#17114](https://github.com/netdata/netdata/pull/17114) ([stelfrag](https://github.com/stelfrag))
- remove "os" "hosts" "plugin" and "module" from stock alarms [\#17113](https://github.com/netdata/netdata/pull/17113) ([ilyam8](https://github.com/ilyam8))
- go.d.plugin add notice log level [\#17112](https://github.com/netdata/netdata/pull/17112) ([ilyam8](https://github.com/ilyam8))
- Second pass at reworking Docker CI. [\#17111](https://github.com/netdata/netdata/pull/17111) ([Ferroin](https://github.com/Ferroin))
- rm unused files from go.d.plugin [\#17110](https://github.com/netdata/netdata/pull/17110) ([ilyam8](https://github.com/ilyam8))
- fix links in go.d.plugin [\#17108](https://github.com/netdata/netdata/pull/17108) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17107](https://github.com/netdata/netdata/pull/17107) ([netdatabot](https://github.com/netdatabot))
- remove "foreach" from health REFERENCE.md [\#17106](https://github.com/netdata/netdata/pull/17106) ([ilyam8](https://github.com/ilyam8))
- Improve cleanup of ephemeral hosts during agent startup [\#17104](https://github.com/netdata/netdata/pull/17104) ([stelfrag](https://github.com/stelfrag))
- Reorganize and cleanup database related code [\#17101](https://github.com/netdata/netdata/pull/17101) ([stelfrag](https://github.com/stelfrag))
- Fix ebpf compilation warnings [\#17100](https://github.com/netdata/netdata/pull/17100) ([stelfrag](https://github.com/stelfrag))
- Remove distributed-data-architecture.md and omit mentions to it [\#17097](https://github.com/netdata/netdata/pull/17097) ([Ancairon](https://github.com/Ancairon))
- Remove deployment-strategies [\#17096](https://github.com/netdata/netdata/pull/17096) ([Ancairon](https://github.com/Ancairon))
- fix links [\#17095](https://github.com/netdata/netdata/pull/17095) ([Ancairon](https://github.com/Ancairon))
- delete docs/netdata-security.md and replace links to proper points [\#17094](https://github.com/netdata/netdata/pull/17094) ([Ancairon](https://github.com/Ancairon))
- fix go.d.plugin/pulsar tests [\#17093](https://github.com/netdata/netdata/pull/17093) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/stretchr/testify from 1.8.4 to 1.9.0 in /src/go/collectors/go.d.plugin [\#17092](https://github.com/netdata/netdata/pull/17092) ([dependabot[bot]](https://github.com/apps/dependabot))
- Rework Docker CI to build each platform in it's own runner. [\#17088](https://github.com/netdata/netdata/pull/17088) ([Ferroin](https://github.com/Ferroin))
- Fix cups plugin group owner [\#17087](https://github.com/netdata/netdata/pull/17087) ([tkatsoulas](https://github.com/tkatsoulas))
- deb packages fix on ioping perms [\#17086](https://github.com/netdata/netdata/pull/17086) ([tkatsoulas](https://github.com/tkatsoulas))
- Amend the logic of ebpf-plugin package suggestion for network-viewer plugin [\#17085](https://github.com/netdata/netdata/pull/17085) ([tkatsoulas](https://github.com/tkatsoulas))
- rename network plugin post and pre install actions [\#17084](https://github.com/netdata/netdata/pull/17084) ([tkatsoulas](https://github.com/tkatsoulas))
- Improve message in kickstart if a static build can’t be found. [\#17081](https://github.com/netdata/netdata/pull/17081) ([Ferroin](https://github.com/Ferroin))
- Make watcher thread wait for explicit steps. [\#17079](https://github.com/netdata/netdata/pull/17079) ([vkalintiris](https://github.com/vkalintiris))
- Update functions tables docs [\#17071](https://github.com/netdata/netdata/pull/17071) ([car12o](https://github.com/car12o))
- add missing "gotify" to list of notification methods in alarm-notify.sh [\#17069](https://github.com/netdata/netdata/pull/17069) ([ilyam8](https://github.com/ilyam8))
- Add CI checks for Go code. [\#17066](https://github.com/netdata/netdata/pull/17066) ([Ferroin](https://github.com/Ferroin))
- go.d.plugin dyncfgv2 [\#17064](https://github.com/netdata/netdata/pull/17064) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17063](https://github.com/netdata/netdata/pull/17063) ([netdatabot](https://github.com/netdatabot))
- go.d.plugin: set max chart id length to 1200 [\#17062](https://github.com/netdata/netdata/pull/17062) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17061](https://github.com/netdata/netdata/pull/17061) ([netdatabot](https://github.com/netdatabot))
- Abort the agent if a single shutdown step takes more than 60 seconds. [\#17060](https://github.com/netdata/netdata/pull/17060) ([vkalintiris](https://github.com/vkalintiris))
- Fix typo [\#17059](https://github.com/netdata/netdata/pull/17059) ([vkalintiris](https://github.com/vkalintiris))
- updated sizing netdata [\#17057](https://github.com/netdata/netdata/pull/17057) ([ktsaou](https://github.com/ktsaou))
- fix zpool state chart family [\#17054](https://github.com/netdata/netdata/pull/17054) ([ilyam8](https://github.com/ilyam8))
- DYNCFG: call the interceptor when a test is made on a new job [\#17052](https://github.com/netdata/netdata/pull/17052) ([ktsaou](https://github.com/ktsaou))
- Fix a few minor bits of build-related infrastructure. [\#17051](https://github.com/netdata/netdata/pull/17051) ([Ferroin](https://github.com/Ferroin))
- HEALTH: eliminate fields that should be labels [\#17048](https://github.com/netdata/netdata/pull/17048) ([ktsaou](https://github.com/ktsaou))
- fix alerts jsonschema prototype for latest dyncfg [\#17047](https://github.com/netdata/netdata/pull/17047) ([ktsaou](https://github.com/ktsaou))
- Protect type anomaly rate map [\#17044](https://github.com/netdata/netdata/pull/17044) ([vkalintiris](https://github.com/vkalintiris))
- Do not use backtrace when sentry is enabled. [\#17043](https://github.com/netdata/netdata/pull/17043) ([vkalintiris](https://github.com/vkalintiris))
- Keep a count of metrics and samples collected [\#17042](https://github.com/netdata/netdata/pull/17042) ([stelfrag](https://github.com/stelfrag))
- Fix links pointing to old go.d repo and update the integrations [\#17040](https://github.com/netdata/netdata/pull/17040) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17039](https://github.com/netdata/netdata/pull/17039) ([netdatabot](https://github.com/netdatabot))
- Improved query target cleanup [\#17038](https://github.com/netdata/netdata/pull/17038) ([stelfrag](https://github.com/stelfrag))
- Liquify start-stop-restart doc [\#17037](https://github.com/netdata/netdata/pull/17037) ([Ancairon](https://github.com/Ancairon))
- Code cleanup [\#17036](https://github.com/netdata/netdata/pull/17036) ([stelfrag](https://github.com/stelfrag))
- Populate the SSL section in Observability and centralization points -… [\#17035](https://github.com/netdata/netdata/pull/17035) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17034](https://github.com/netdata/netdata/pull/17034) ([netdatabot](https://github.com/netdatabot))
- Metric release does not need to fetch retention [\#17033](https://github.com/netdata/netdata/pull/17033) ([stelfrag](https://github.com/stelfrag))
- Bump go.mongodb.org/mongo-driver from 1.13.1 to 1.14.0 in /src/go/collectors/go.d.plugin [\#17030](https://github.com/netdata/netdata/pull/17030) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump k8s.io/client-go from 0.29.1 to 0.29.2 in /src/go/collectors/go.d.plugin [\#17029](https://github.com/netdata/netdata/pull/17029) ([dependabot[bot]](https://github.com/apps/dependabot))
- Increase RRD\_ID\_LENGTH\_MAX to 1200 [\#17028](https://github.com/netdata/netdata/pull/17028) ([stelfrag](https://github.com/stelfrag))
- Fix determining repo root in Coverity scan script. [\#17024](https://github.com/netdata/netdata/pull/17024) ([Ferroin](https://github.com/Ferroin))
- DYNCFG support deleting orphan configurations [\#17023](https://github.com/netdata/netdata/pull/17023) ([ktsaou](https://github.com/ktsaou))
- More concretely utilize local modules in our CMake code. [\#17022](https://github.com/netdata/netdata/pull/17022) ([Ferroin](https://github.com/Ferroin))
- Correctly mark protobuf as required in find\_package. [\#17021](https://github.com/netdata/netdata/pull/17021) ([Ferroin](https://github.com/Ferroin))
- Protect metric release in dimension delete callback [\#17020](https://github.com/netdata/netdata/pull/17020) ([stelfrag](https://github.com/stelfrag))

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
