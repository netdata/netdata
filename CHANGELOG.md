# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.45.6...HEAD)

**Merged pull requests:**

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
- Regenerate integrations.js [\#17660](https://github.com/netdata/netdata/pull/17660) ([netdatabot](https://github.com/netdatabot))
- go.d cockroachdb fix calculation [\#17659](https://github.com/netdata/netdata/pull/17659) ([ilyam8](https://github.com/ilyam8))
- Fall back to querying libc.so.6 if ldd can’t detect libc implementation. [\#17657](https://github.com/netdata/netdata/pull/17657) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17655](https://github.com/netdata/netdata/pull/17655) ([netdatabot](https://github.com/netdatabot))
- go.d hpssa fix cache battery chart ctx [\#17654](https://github.com/netdata/netdata/pull/17654) ([ilyam8](https://github.com/ilyam8))
- files movearound [\#17653](https://github.com/netdata/netdata/pull/17653) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#17652](https://github.com/netdata/netdata/pull/17652) ([netdatabot](https://github.com/netdatabot))
- Update libbpf [\#17650](https://github.com/netdata/netdata/pull/17650) ([thiagoftsm](https://github.com/thiagoftsm))
- Support offline installs in the updater code. [\#17648](https://github.com/netdata/netdata/pull/17648) ([Ferroin](https://github.com/Ferroin))
- Update the claim readme [\#17646](https://github.com/netdata/netdata/pull/17646) ([Ancairon](https://github.com/Ancairon))
- remove doc and consolidate info to main section page [\#17645](https://github.com/netdata/netdata/pull/17645) ([Ancairon](https://github.com/Ancairon))
- Bump github.com/vmware/govmomi from 0.37.1 to 0.37.2 in /src/go/collectors/go.d.plugin [\#17644](https://github.com/netdata/netdata/pull/17644) ([dependabot[bot]](https://github.com/apps/dependabot))
- Fix incorrect bind failure warning [\#17643](https://github.com/netdata/netdata/pull/17643) ([stelfrag](https://github.com/stelfrag))
- go.d ping add missing setting to schema [\#17642](https://github.com/netdata/netdata/pull/17642) ([ilyam8](https://github.com/ilyam8))
- logs: add ND\_ALERT\_STATUS to facets [\#17641](https://github.com/netdata/netdata/pull/17641) ([ktsaou](https://github.com/ktsaou))
- Add valkey to apps\_groups.conf [\#17639](https://github.com/netdata/netdata/pull/17639) ([mohd-akram](https://github.com/mohd-akram))
- python.d remove hpssa [\#17638](https://github.com/netdata/netdata/pull/17638) ([ilyam8](https://github.com/ilyam8))
- go.d hpssa [\#17637](https://github.com/netdata/netdata/pull/17637) ([ilyam8](https://github.com/ilyam8))
- ndsudo add ssacli [\#17635](https://github.com/netdata/netdata/pull/17635) ([ilyam8](https://github.com/ilyam8))
- health update isc dhcp alarms [\#17634](https://github.com/netdata/netdata/pull/17634) ([ilyam8](https://github.com/ilyam8))
- add pcre2 to install-required-packages "netdata" for macOS [\#17633](https://github.com/netdata/netdata/pull/17633) ([ilyam8](https://github.com/ilyam8))
- Bump github.com/docker/docker from 26.1.1+incompatible to 26.1.2+incompatible in /src/go/collectors/go.d.plugin [\#17631](https://github.com/netdata/netdata/pull/17631) ([dependabot[bot]](https://github.com/apps/dependabot))
- Regenerate integrations.js [\#17630](https://github.com/netdata/netdata/pull/17630) ([netdatabot](https://github.com/netdatabot))
- go.d isc\_dhcpd create a chart for each pool [\#17629](https://github.com/netdata/netdata/pull/17629) ([ilyam8](https://github.com/ilyam8))
- python.d remove bind\_rndc [\#17628](https://github.com/netdata/netdata/pull/17628) ([ilyam8](https://github.com/ilyam8))
- Add Sentry support to new CPack packages. [\#17627](https://github.com/netdata/netdata/pull/17627) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17626](https://github.com/netdata/netdata/pull/17626) ([netdatabot](https://github.com/netdatabot))
- go.d filecheck update to create a chart per instance [\#17624](https://github.com/netdata/netdata/pull/17624) ([ilyam8](https://github.com/ilyam8))
- go.d systemdunits fix unit files selector [\#17622](https://github.com/netdata/netdata/pull/17622) ([ilyam8](https://github.com/ilyam8))
- Improve handling of an alert that transitions to the REMOVED state [\#17621](https://github.com/netdata/netdata/pull/17621) ([stelfrag](https://github.com/stelfrag))
- log to journal all transitions [\#17618](https://github.com/netdata/netdata/pull/17618) ([ktsaou](https://github.com/ktsaou))
- Remove contrib now that we use cpack for DEB packages [\#17614](https://github.com/netdata/netdata/pull/17614) ([vkalintiris](https://github.com/vkalintiris))
- add update every to json schema [\#17613](https://github.com/netdata/netdata/pull/17613) ([ktsaou](https://github.com/ktsaou))
- reset health when children disconnect [\#17612](https://github.com/netdata/netdata/pull/17612) ([ktsaou](https://github.com/ktsaou))
- go.d dyncfg return 200 on Enable for running jobs [\#17611](https://github.com/netdata/netdata/pull/17611) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#17610](https://github.com/netdata/netdata/pull/17610) ([netdatabot](https://github.com/netdatabot))
- Bump golang.org/x/net from 0.24.0 to 0.25.0 in /src/go/collectors/go.d.plugin [\#17609](https://github.com/netdata/netdata/pull/17609) ([dependabot[bot]](https://github.com/apps/dependabot))
- Bump jinja2 from 3.1.3 to 3.1.4 in /packaging/dag [\#17607](https://github.com/netdata/netdata/pull/17607) ([dependabot[bot]](https://github.com/apps/dependabot))
- go.d systemdunits add unit files state [\#17606](https://github.com/netdata/netdata/pull/17606) ([ilyam8](https://github.com/ilyam8))
- Add improved handling for TLS certificates for static builds. [\#17605](https://github.com/netdata/netdata/pull/17605) ([Ferroin](https://github.com/Ferroin))
- Add option to limit architectures for offline installs. [\#17604](https://github.com/netdata/netdata/pull/17604) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#17603](https://github.com/netdata/netdata/pull/17603) ([netdatabot](https://github.com/netdatabot))
- Make offline installs properly offline again. [\#17602](https://github.com/netdata/netdata/pull/17602) ([Ferroin](https://github.com/Ferroin))
- remove python.d/smartd\_log [\#17600](https://github.com/netdata/netdata/pull/17600) ([ilyam8](https://github.com/ilyam8))
- Remove CentOS Stream 8 from CI. [\#17599](https://github.com/netdata/netdata/pull/17599) ([Ferroin](https://github.com/Ferroin))
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
- Remove Fedora 38 from CI. [\#17548](https://github.com/netdata/netdata/pull/17548) ([Ferroin](https://github.com/Ferroin))
- Remove Alpine 3.16 from CI. [\#17547](https://github.com/netdata/netdata/pull/17547) ([Ferroin](https://github.com/Ferroin))
- Fix platform EOL check issue assignment. [\#17544](https://github.com/netdata/netdata/pull/17544) ([Ferroin](https://github.com/Ferroin))
- refresh the ML documentation and consolidate the two docs [\#17543](https://github.com/netdata/netdata/pull/17543) ([Ancairon](https://github.com/Ancairon))
- Bump github.com/likexian/whois from 1.15.2 to 1.15.3 in /src/go/collectors/go.d.plugin [\#17542](https://github.com/netdata/netdata/pull/17542) ([dependabot[bot]](https://github.com/apps/dependabot))
- Additional code cleanup [\#17541](https://github.com/netdata/netdata/pull/17541) ([stelfrag](https://github.com/stelfrag))
- docs: Add Ubuntu AArch64 that is missing from the list [\#17538](https://github.com/netdata/netdata/pull/17538) ([dgibbs64](https://github.com/dgibbs64))
- go.d smartctl [\#17536](https://github.com/netdata/netdata/pull/17536) ([ilyam8](https://github.com/ilyam8))
- Fix handling of OpenSSL linking on macOS [\#17535](https://github.com/netdata/netdata/pull/17535) ([Ferroin](https://github.com/Ferroin))
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
- Make CMake options for platform-dependent plugins depend on being build for a supported platform. [\#17517](https://github.com/netdata/netdata/pull/17517) ([Ferroin](https://github.com/Ferroin))
- Support to WolfSSL \(Step 1\) [\#17516](https://github.com/netdata/netdata/pull/17516) ([thiagoftsm](https://github.com/thiagoftsm))
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
- Windows Support Phase 1 [\#17497](https://github.com/netdata/netdata/pull/17497) ([ktsaou](https://github.com/ktsaou))
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
- Support time based retention [\#17413](https://github.com/netdata/netdata/pull/17413) ([stelfrag](https://github.com/stelfrag))
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
