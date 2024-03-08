# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.3...HEAD)

**Merged pull requests:**

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
- eBPF - Network Viewer \(Move code\) [\#17018](https://github.com/netdata/netdata/pull/17018) ([thiagoftsm](https://github.com/thiagoftsm))
- dyncfg: allow tree for individual IDs [\#17017](https://github.com/netdata/netdata/pull/17017) ([ktsaou](https://github.com/ktsaou))
- Documentation changes, new files and restructuring the hierarchy [\#17014](https://github.com/netdata/netdata/pull/17014) ([Ancairon](https://github.com/Ancairon))
- eBPF & NV \(update packages\) [\#17012](https://github.com/netdata/netdata/pull/17012) ([thiagoftsm](https://github.com/thiagoftsm))
- Add watcher thread to report shutdown steps. [\#17010](https://github.com/netdata/netdata/pull/17010) ([vkalintiris](https://github.com/vkalintiris))
- dyncfg: fix support for testing new jobs [\#17009](https://github.com/netdata/netdata/pull/17009) ([ktsaou](https://github.com/ktsaou))
- Abort on non-zero rc. [\#17008](https://github.com/netdata/netdata/pull/17008) ([vkalintiris](https://github.com/vkalintiris))
- Netdata Agent: Backup restore documentation [\#17006](https://github.com/netdata/netdata/pull/17006) ([luisj1983](https://github.com/luisj1983))
- Integrate Go plugin with build system. [\#17005](https://github.com/netdata/netdata/pull/17005) ([Ferroin](https://github.com/Ferroin))
- Bump the version of the installed Go toolchain to 1.22.0. [\#17004](https://github.com/netdata/netdata/pull/17004) ([Ferroin](https://github.com/Ferroin))
- Misc improvements [\#17001](https://github.com/netdata/netdata/pull/17001) ([stelfrag](https://github.com/stelfrag))
- Adjust storage tiers if we fail to create the requested number of tiers [\#16999](https://github.com/netdata/netdata/pull/16999) ([stelfrag](https://github.com/stelfrag))
- Move diagrams/ under docs/ [\#16998](https://github.com/netdata/netdata/pull/16998) ([vkalintiris](https://github.com/vkalintiris))
- Include Go plugin sources in main repository. [\#16997](https://github.com/netdata/netdata/pull/16997) ([Ferroin](https://github.com/Ferroin))
- Small cleanup [\#16996](https://github.com/netdata/netdata/pull/16996) ([vkalintiris](https://github.com/vkalintiris))
- Remove historical changelog and cppcheck [\#16995](https://github.com/netdata/netdata/pull/16995) ([vkalintiris](https://github.com/vkalintiris))
- Remove config macros that are always set. [\#16994](https://github.com/netdata/netdata/pull/16994) ([vkalintiris](https://github.com/vkalintiris))
- Use changed files in check-files workflow [\#16993](https://github.com/netdata/netdata/pull/16993) ([tkatsoulas](https://github.com/tkatsoulas))
- Move web/ under src/ [\#16992](https://github.com/netdata/netdata/pull/16992) ([vkalintiris](https://github.com/vkalintiris))
- Add spinlock to protect metric release [\#16989](https://github.com/netdata/netdata/pull/16989) ([stelfrag](https://github.com/stelfrag))
- updated message ids for systemd and dbus [\#16987](https://github.com/netdata/netdata/pull/16987) ([ktsaou](https://github.com/ktsaou))
- Cache key wasn't taking account changes in the version of bundled software [\#16985](https://github.com/netdata/netdata/pull/16985) ([tkatsoulas](https://github.com/tkatsoulas))
- Update input skip patterns [\#16984](https://github.com/netdata/netdata/pull/16984) ([tkatsoulas](https://github.com/tkatsoulas))
- Update input paths for tj-actions/changed-files [\#16982](https://github.com/netdata/netdata/pull/16982) ([tkatsoulas](https://github.com/tkatsoulas))
- Update synology.md [\#16980](https://github.com/netdata/netdata/pull/16980) ([pschaer](https://github.com/pschaer))
- Detect machine GUID change [\#16979](https://github.com/netdata/netdata/pull/16979) ([stelfrag](https://github.com/stelfrag))
- Move CO-RE headers \(integration between eBPF and Network Viewer\) [\#16978](https://github.com/netdata/netdata/pull/16978) ([thiagoftsm](https://github.com/thiagoftsm))
- Regenerate integrations.js [\#16974](https://github.com/netdata/netdata/pull/16974) ([netdatabot](https://github.com/netdatabot))
- Use C++14 by default when building on systems that support it. [\#16972](https://github.com/netdata/netdata/pull/16972) ([Ferroin](https://github.com/Ferroin))
- change edac ecc errors from incremental to absolute [\#16970](https://github.com/netdata/netdata/pull/16970) ([ilyam8](https://github.com/ilyam8))
- update go.d.plugin version to v0.58.1 [\#16968](https://github.com/netdata/netdata/pull/16968) ([ilyam8](https://github.com/ilyam8))
- fix move collectors to src/ leftovers [\#16967](https://github.com/netdata/netdata/pull/16967) ([ilyam8](https://github.com/ilyam8))
- necessary changes for integrations to work after moving collectors/ i… [\#16966](https://github.com/netdata/netdata/pull/16966) ([Ancairon](https://github.com/Ancairon))
- Move collectors/ under src/ [\#16965](https://github.com/netdata/netdata/pull/16965) ([vkalintiris](https://github.com/vkalintiris))
- network-viewer: aggregated view improvements [\#16960](https://github.com/netdata/netdata/pull/16960) ([ktsaou](https://github.com/ktsaou))
- Improve agent shutdown [\#16959](https://github.com/netdata/netdata/pull/16959) ([stelfrag](https://github.com/stelfrag))
- DYNCFG: support test on new jobs [\#16958](https://github.com/netdata/netdata/pull/16958) ([ktsaou](https://github.com/ktsaou))
- Updated txt - Prometheus section README.md [\#16957](https://github.com/netdata/netdata/pull/16957) ([Aliki92](https://github.com/Aliki92))
- Fix path in health integrations [\#16956](https://github.com/netdata/netdata/pull/16956) ([Ancairon](https://github.com/Ancairon))
- Regenerate integrations.js [\#16955](https://github.com/netdata/netdata/pull/16955) ([netdatabot](https://github.com/netdatabot))
- Move health/ under src/ [\#16954](https://github.com/netdata/netdata/pull/16954) ([vkalintiris](https://github.com/vkalintiris))
- fix linking of a markdown file [\#16952](https://github.com/netdata/netdata/pull/16952) ([tkatsoulas](https://github.com/tkatsoulas))
- Do not declare struct meant for internal usage [\#16951](https://github.com/netdata/netdata/pull/16951) ([vkalintiris](https://github.com/vkalintiris))
- fix wrong sizeof [\#16950](https://github.com/netdata/netdata/pull/16950) ([ktsaou](https://github.com/ktsaou))
- Split network viewer plugin to it’s own package. [\#16949](https://github.com/netdata/netdata/pull/16949) ([Ferroin](https://github.com/Ferroin))
- update bundled UI to v6.85.0 [\#16948](https://github.com/netdata/netdata/pull/16948) ([ilyam8](https://github.com/ilyam8))
- Update codeowners and cleanup .gitignore [\#16946](https://github.com/netdata/netdata/pull/16946) ([vkalintiris](https://github.com/vkalintiris))
- Remove cleanup\_destroyed\_dictionaries call during shutdown [\#16944](https://github.com/netdata/netdata/pull/16944) ([stelfrag](https://github.com/stelfrag))
- respect NETDATA\_LOG\_LEVEL if set [\#16943](https://github.com/netdata/netdata/pull/16943) ([ilyam8](https://github.com/ilyam8))
- enable network-viewer aggregated views [\#16940](https://github.com/netdata/netdata/pull/16940) ([ktsaou](https://github.com/ktsaou))
- fix charts.d.plugin configuration directory names [\#16939](https://github.com/netdata/netdata/pull/16939) ([ilyam8](https://github.com/ilyam8))
- Assorted cleanup of CI/packaging related code. [\#16938](https://github.com/netdata/netdata/pull/16938) ([Ferroin](https://github.com/Ferroin))
- Check for agent already running [\#16937](https://github.com/netdata/netdata/pull/16937) ([stelfrag](https://github.com/stelfrag))
- Remove duplicate check [\#16936](https://github.com/netdata/netdata/pull/16936) ([stelfrag](https://github.com/stelfrag))
- Drop ESLint CI jobs and config. [\#16935](https://github.com/netdata/netdata/pull/16935) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#16934](https://github.com/netdata/netdata/pull/16934) ([netdatabot](https://github.com/netdatabot))
- Move daemon/ under src/ [\#16933](https://github.com/netdata/netdata/pull/16933) ([vkalintiris](https://github.com/vkalintiris))
- Exporting moved, so changes needed for integrations, + CODEOWNERS change [\#16932](https://github.com/netdata/netdata/pull/16932) ([Ancairon](https://github.com/Ancairon))
- Make sure the duration is not negative [\#16931](https://github.com/netdata/netdata/pull/16931) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16930](https://github.com/netdata/netdata/pull/16930) ([netdatabot](https://github.com/netdatabot))
- Protect analytics set data [\#16929](https://github.com/netdata/netdata/pull/16929) ([stelfrag](https://github.com/stelfrag))
- Minor rework on document optimize performance guide [\#16925](https://github.com/netdata/netdata/pull/16925) ([tkatsoulas](https://github.com/tkatsoulas))
- fix installation of `libfluent-bit.so` [\#16924](https://github.com/netdata/netdata/pull/16924) ([ilyam8](https://github.com/ilyam8))
- respect log level for all sources [\#16922](https://github.com/netdata/netdata/pull/16922) ([ilyam8](https://github.com/ilyam8))
- split dictionary into multiple files [\#16920](https://github.com/netdata/netdata/pull/16920) ([ktsaou](https://github.com/ktsaou))
- Update file match patterns in CI jobs. [\#16917](https://github.com/netdata/netdata/pull/16917) ([Ferroin](https://github.com/Ferroin))
- Release label key if already in use [\#16916](https://github.com/netdata/netdata/pull/16916) ([stelfrag](https://github.com/stelfrag))
- add support for the info parameter to all external plugin functions [\#16915](https://github.com/netdata/netdata/pull/16915) ([ktsaou](https://github.com/ktsaou))
- Move exporting/ under src/ [\#16913](https://github.com/netdata/netdata/pull/16913) ([vkalintiris](https://github.com/vkalintiris))
- Network viewer: filter by username [\#16911](https://github.com/netdata/netdata/pull/16911) ([ktsaou](https://github.com/ktsaou))
- Build network-viewer only on linux [\#16910](https://github.com/netdata/netdata/pull/16910) ([vkalintiris](https://github.com/vkalintiris))
- rename network functions [\#16908](https://github.com/netdata/netdata/pull/16908) ([ktsaou](https://github.com/ktsaou))
- Update CODEOWNERS [\#16907](https://github.com/netdata/netdata/pull/16907) ([tkatsoulas](https://github.com/tkatsoulas))
- Assorted build-related changes. [\#16906](https://github.com/netdata/netdata/pull/16906) ([vkalintiris](https://github.com/vkalintiris))
- Remove markdown linter [\#16905](https://github.com/netdata/netdata/pull/16905) ([vkalintiris](https://github.com/vkalintiris))
- Update README.md [\#16904](https://github.com/netdata/netdata/pull/16904) ([Aliki92](https://github.com/Aliki92))
- fluent-bit & logsmanagement under src/ [\#16903](https://github.com/netdata/netdata/pull/16903) ([vkalintiris](https://github.com/vkalintiris))
- updated permissions map comment [\#16902](https://github.com/netdata/netdata/pull/16902) ([ktsaou](https://github.com/ktsaou))
- Use spinlock for reference counting. [\#16901](https://github.com/netdata/netdata/pull/16901) ([vkalintiris](https://github.com/vkalintiris))
- network-viewer: show unknown container [\#16900](https://github.com/netdata/netdata/pull/16900) ([ktsaou](https://github.com/ktsaou))
- Move aclk/ under src/ [\#16899](https://github.com/netdata/netdata/pull/16899) ([vkalintiris](https://github.com/vkalintiris))
- Enable sentry sessions [\#16898](https://github.com/netdata/netdata/pull/16898) ([vkalintiris](https://github.com/vkalintiris))
- Do not cancel detection thread. [\#16897](https://github.com/netdata/netdata/pull/16897) ([vkalintiris](https://github.com/vkalintiris))
- Create a top-level directory to contain source code. [\#16896](https://github.com/netdata/netdata/pull/16896) ([vkalintiris](https://github.com/vkalintiris))
- updating What's new based on Office Hours shared plans [\#16895](https://github.com/netdata/netdata/pull/16895) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Remove tags field from RRD hosts. [\#16894](https://github.com/netdata/netdata/pull/16894) ([vkalintiris](https://github.com/vkalintiris))
- local-sockets: use netlink when libmnl is available [\#16893](https://github.com/netdata/netdata/pull/16893) ([ktsaou](https://github.com/ktsaou))
- Add a constant env var for Sentry's  DSN when someone wants to build Agent and doesn't have access to GH secrets.  [\#16892](https://github.com/netdata/netdata/pull/16892) ([tkatsoulas](https://github.com/tkatsoulas))
- fixed missing collisions and drag on newly added apps [\#16891](https://github.com/netdata/netdata/pull/16891) ([ktsaou](https://github.com/ktsaou))
- Set build type to release with debug info. [\#16889](https://github.com/netdata/netdata/pull/16889) ([vkalintiris](https://github.com/vkalintiris))
- fix order of openning a file and checking its inode [\#16887](https://github.com/netdata/netdata/pull/16887) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16886](https://github.com/netdata/netdata/pull/16886) ([netdatabot](https://github.com/netdatabot))
- fix crash on query\_progress initializer [\#16885](https://github.com/netdata/netdata/pull/16885) ([ktsaou](https://github.com/ktsaou))
- highlight Challenge Secret title to be more visible [\#16882](https://github.com/netdata/netdata/pull/16882) ([juacker](https://github.com/juacker))
- add the CLOEXEC flag to all sockets and files [\#16881](https://github.com/netdata/netdata/pull/16881) ([ktsaou](https://github.com/ktsaou))
- Network viewer UI minor fixes [\#16880](https://github.com/netdata/netdata/pull/16880) ([ktsaou](https://github.com/ktsaou))
- Network viewer fixes [\#16877](https://github.com/netdata/netdata/pull/16877) ([ktsaou](https://github.com/ktsaou))
- Add requirements.txt for dag [\#16875](https://github.com/netdata/netdata/pull/16875) ([vkalintiris](https://github.com/vkalintiris))
- Rm refs to map and save modes [\#16874](https://github.com/netdata/netdata/pull/16874) ([vkalintiris](https://github.com/vkalintiris))
- Fix coverity issues [\#16873](https://github.com/netdata/netdata/pull/16873) ([stelfrag](https://github.com/stelfrag))
- Network Viewer \(local-sockets version\) [\#16872](https://github.com/netdata/netdata/pull/16872) ([ktsaou](https://github.com/ktsaou))
- Limit what we upload to GCS for nightlies. [\#16870](https://github.com/netdata/netdata/pull/16870) ([Ferroin](https://github.com/Ferroin))
- Use dagger to build and test the agent. [\#16868](https://github.com/netdata/netdata/pull/16868) ([vkalintiris](https://github.com/vkalintiris))
- Local sockets for network namespaces [\#16867](https://github.com/netdata/netdata/pull/16867) ([ktsaou](https://github.com/ktsaou))
- Fix coverity issue [\#16866](https://github.com/netdata/netdata/pull/16866) ([stelfrag](https://github.com/stelfrag))
- update alpine 3.16 fts-dev [\#16865](https://github.com/netdata/netdata/pull/16865) ([ilyam8](https://github.com/ilyam8))
- Remove old mention of save db mode [\#16864](https://github.com/netdata/netdata/pull/16864) ([Ancairon](https://github.com/Ancairon))
- port useful code from incomplete PRs [\#16863](https://github.com/netdata/netdata/pull/16863) ([ktsaou](https://github.com/ktsaou))
- apply the right prototype to instances [\#16862](https://github.com/netdata/netdata/pull/16862) ([ktsaou](https://github.com/ktsaou))
- detect sockets direction [\#16861](https://github.com/netdata/netdata/pull/16861) ([ktsaou](https://github.com/ktsaou))
- add freebsd jail detection to system-info.sh [\#16858](https://github.com/netdata/netdata/pull/16858) ([ilyam8](https://github.com/ilyam8))
- Add ARMv6 static builds. [\#16853](https://github.com/netdata/netdata/pull/16853) ([Ferroin](https://github.com/Ferroin))
- CNCF link fix [\#16851](https://github.com/netdata/netdata/pull/16851) ([hugovalente-pm](https://github.com/hugovalente-pm))
- Disable local Go builds in our RPM packages. [\#16848](https://github.com/netdata/netdata/pull/16848) ([Ferroin](https://github.com/Ferroin))
- Include timer units failed state alert as default [\#16845](https://github.com/netdata/netdata/pull/16845) ([tkatsoulas](https://github.com/tkatsoulas))
- kickstart: use extended-regexp in `get_redirect()` [\#16844](https://github.com/netdata/netdata/pull/16844) ([ilyam8](https://github.com/ilyam8))
- Make the kickstart checksum's placeholder value more concrete [\#16843](https://github.com/netdata/netdata/pull/16843) ([tkatsoulas](https://github.com/tkatsoulas))
- Improve service thread shutdown [\#16841](https://github.com/netdata/netdata/pull/16841) ([stelfrag](https://github.com/stelfrag))
- Update statistics to address slow queries [\#16838](https://github.com/netdata/netdata/pull/16838) ([stelfrag](https://github.com/stelfrag))
- New Permissions System [\#16837](https://github.com/netdata/netdata/pull/16837) ([ktsaou](https://github.com/ktsaou))
- Regenerate integrations.js [\#16835](https://github.com/netdata/netdata/pull/16835) ([netdatabot](https://github.com/netdatabot))
- adds docs for cloud MS Teams integration [\#16834](https://github.com/netdata/netdata/pull/16834) ([papazach](https://github.com/papazach))
- Fix coverity issue [\#16831](https://github.com/netdata/netdata/pull/16831) ([stelfrag](https://github.com/stelfrag))
- add brotli and libyaml to buildinfo [\#16830](https://github.com/netdata/netdata/pull/16830) ([ktsaou](https://github.com/ktsaou))
- Fix directory handling in Go toolchain handling script. [\#16828](https://github.com/netdata/netdata/pull/16828) ([Ferroin](https://github.com/Ferroin))
- Change query label matching logic [\#16827](https://github.com/netdata/netdata/pull/16827) ([stelfrag](https://github.com/stelfrag))
- Improve container detection logic for edit-config. [\#16825](https://github.com/netdata/netdata/pull/16825) ([Ferroin](https://github.com/Ferroin))
- Preserve label source during migration [\#16821](https://github.com/netdata/netdata/pull/16821) ([stelfrag](https://github.com/stelfrag))
- Add explicit callback types for readability. [\#16820](https://github.com/netdata/netdata/pull/16820) ([vkalintiris](https://github.com/vkalintiris))
- Update documentation \(Replication DB\) [\#16816](https://github.com/netdata/netdata/pull/16816) ([thiagoftsm](https://github.com/thiagoftsm))
- Add script to ensure a usable Go toolchain is installed. [\#16815](https://github.com/netdata/netdata/pull/16815) ([Ferroin](https://github.com/Ferroin))
- fix verify\_netdata\_host\_prefix log spam [\#16814](https://github.com/netdata/netdata/pull/16814) ([ilyam8](https://github.com/ilyam8))
- use fs type to veryfiy procfs/sysfs [\#16813](https://github.com/netdata/netdata/pull/16813) ([ilyam8](https://github.com/ilyam8))
- updates to light onprem docs [\#16811](https://github.com/netdata/netdata/pull/16811) ([M4itee](https://github.com/M4itee))
- set app\_group label automatically [\#16810](https://github.com/netdata/netdata/pull/16810) ([boxjan](https://github.com/boxjan))
- Apply ASCII-based comparisons to commands in kickstart script that rely on a particular language setting [\#16806](https://github.com/netdata/netdata/pull/16806) ([tkatsoulas](https://github.com/tkatsoulas))
- Remove help text that no longer applies. [\#16805](https://github.com/netdata/netdata/pull/16805) ([vkalintiris](https://github.com/vkalintiris))
- Move mqtt\_websockets under aclk/ [\#16804](https://github.com/netdata/netdata/pull/16804) ([vkalintiris](https://github.com/vkalintiris))
- Fix incorrect major version check in updater. [\#16803](https://github.com/netdata/netdata/pull/16803) ([Ferroin](https://github.com/Ferroin))
- cgroups: containers-vms add CPU throttling % [\#16800](https://github.com/netdata/netdata/pull/16800) ([ilyam8](https://github.com/ilyam8))
- Setup sentry-native SDK. [\#16798](https://github.com/netdata/netdata/pull/16798) ([vkalintiris](https://github.com/vkalintiris))
- Add additional fail reason and source during database initialization [\#16794](https://github.com/netdata/netdata/pull/16794) ([stelfrag](https://github.com/stelfrag))
- Use original summary for alert transition [\#16793](https://github.com/netdata/netdata/pull/16793) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16792](https://github.com/netdata/netdata/pull/16792) ([netdatabot](https://github.com/netdatabot))
- Update role-based-access.md [\#16791](https://github.com/netdata/netdata/pull/16791) ([vkuznecovas](https://github.com/vkuznecovas))
- Free key and search, replace patterns [\#16789](https://github.com/netdata/netdata/pull/16789) ([stelfrag](https://github.com/stelfrag))
- Prepare to functions \(eBPF\) [\#16788](https://github.com/netdata/netdata/pull/16788) ([thiagoftsm](https://github.com/thiagoftsm))
- Use named constants for keyword tokens. [\#16787](https://github.com/netdata/netdata/pull/16787) ([vkalintiris](https://github.com/vkalintiris))
- diskspace: reworked the cleanup to fix race conditions [\#16786](https://github.com/netdata/netdata/pull/16786) ([ktsaou](https://github.com/ktsaou))
- diskspace missing mutex use [\#16784](https://github.com/netdata/netdata/pull/16784) ([ktsaou](https://github.com/ktsaou))
- Remove h2o header from libnetdata [\#16780](https://github.com/netdata/netdata/pull/16780) ([vkalintiris](https://github.com/vkalintiris))
- DYNCFG: dynamically configured alerts [\#16779](https://github.com/netdata/netdata/pull/16779) ([ktsaou](https://github.com/ktsaou))
- Update replication documentation [\#16778](https://github.com/netdata/netdata/pull/16778) ([thiagoftsm](https://github.com/thiagoftsm))
- Update telegram documentation [\#16777](https://github.com/netdata/netdata/pull/16777) ([thiagoftsm](https://github.com/thiagoftsm))
- Delete unused variable. [\#16776](https://github.com/netdata/netdata/pull/16776) ([vkalintiris](https://github.com/vkalintiris))
- Use unsigned char for binary data in mqtt. [\#16775](https://github.com/netdata/netdata/pull/16775) ([vkalintiris](https://github.com/vkalintiris))
- Fix warning. [\#16774](https://github.com/netdata/netdata/pull/16774) ([vkalintiris](https://github.com/vkalintiris))
- fix thread name on fatal and cgroup netdev rename crash [\#16771](https://github.com/netdata/netdata/pull/16771) ([ktsaou](https://github.com/ktsaou))
- allow POST requests to be received from ACLK [\#16770](https://github.com/netdata/netdata/pull/16770) ([ktsaou](https://github.com/ktsaou))
- Keep transaction id of request headers [\#16769](https://github.com/netdata/netdata/pull/16769) ([ktsaou](https://github.com/ktsaou))
- Change default build directory in installer to `build`. [\#16768](https://github.com/netdata/netdata/pull/16768) ([Ferroin](https://github.com/Ferroin))
- Fix coverity issues [\#16766](https://github.com/netdata/netdata/pull/16766) ([stelfrag](https://github.com/stelfrag))
- Add missing call for aral\_freez \(eBPF\) [\#16765](https://github.com/netdata/netdata/pull/16765) ([thiagoftsm](https://github.com/thiagoftsm))
- /api/v1/config tree improvements and swagger documentation [\#16764](https://github.com/netdata/netdata/pull/16764) ([ktsaou](https://github.com/ktsaou))
- fix compiler warnings [\#16763](https://github.com/netdata/netdata/pull/16763) ([ktsaou](https://github.com/ktsaou))
- fix cmake \_GNU\_SOURCE warnings [\#16761](https://github.com/netdata/netdata/pull/16761) ([ktsaou](https://github.com/ktsaou))
- fix phtread-detatch\(\) call [\#16760](https://github.com/netdata/netdata/pull/16760) ([ktsaou](https://github.com/ktsaou))
- Fix sanitizer errors [\#16759](https://github.com/netdata/netdata/pull/16759) ([ktsaou](https://github.com/ktsaou))
- report timestamps with progress [\#16758](https://github.com/netdata/netdata/pull/16758) ([ktsaou](https://github.com/ktsaou))
- add schemas to /usr/lib/netdata/conf.d/schema.d [\#16757](https://github.com/netdata/netdata/pull/16757) ([ktsaou](https://github.com/ktsaou))
- Add netdata\_os\_info metric [\#16756](https://github.com/netdata/netdata/pull/16756) ([colinleroy](https://github.com/colinleroy))
- Recursively merge mqtt\_websockets [\#16755](https://github.com/netdata/netdata/pull/16755) ([vkalintiris](https://github.com/vkalintiris))
- packaging: add cap\_dac\_read\_search to go.d.plugin [\#16754](https://github.com/netdata/netdata/pull/16754) ([ilyam8](https://github.com/ilyam8))
- Name storage engine variables consistently. [\#16753](https://github.com/netdata/netdata/pull/16753) ([vkalintiris](https://github.com/vkalintiris))
- minor - fix/update codeowners [\#16750](https://github.com/netdata/netdata/pull/16750) ([underhood](https://github.com/underhood))
- fix cpu per core charts priority [\#16749](https://github.com/netdata/netdata/pull/16749) ([ilyam8](https://github.com/ilyam8))
- Address sanitizer through CMake and use it for unit tests. [\#16748](https://github.com/netdata/netdata/pull/16748) ([vkalintiris](https://github.com/vkalintiris))
- Remove unused file [\#16747](https://github.com/netdata/netdata/pull/16747) ([stelfrag](https://github.com/stelfrag))
- cleanup proc net-dev renames [\#16745](https://github.com/netdata/netdata/pull/16745) ([ktsaou](https://github.com/ktsaou))
- uninstaller: improve removing `netdata` from groups [\#16742](https://github.com/netdata/netdata/pull/16742) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16739](https://github.com/netdata/netdata/pull/16739) ([netdatabot](https://github.com/netdatabot))
- change get kickstart url to https://get.netdata.cloud/kickstart.sh [\#16738](https://github.com/netdata/netdata/pull/16738) ([ilyam8](https://github.com/ilyam8))
- health: add httpcheck bad header alert [\#16736](https://github.com/netdata/netdata/pull/16736) ([ilyam8](https://github.com/ilyam8))
- update default netdata.conf used for native packages [\#16734](https://github.com/netdata/netdata/pull/16734) ([ilyam8](https://github.com/ilyam8))
- fix missing CPU frequency [\#16732](https://github.com/netdata/netdata/pull/16732) ([ilyam8](https://github.com/ilyam8))
- Fix handling of hardening flags with Clang [\#16731](https://github.com/netdata/netdata/pull/16731) ([Ferroin](https://github.com/Ferroin))
- fix excessive "maximum number of cgroups reached" log messages [\#16730](https://github.com/netdata/netdata/pull/16730) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16728](https://github.com/netdata/netdata/pull/16728) ([netdatabot](https://github.com/netdatabot))
- update ebpf-socket function name and columns [\#16727](https://github.com/netdata/netdata/pull/16727) ([ilyam8](https://github.com/ilyam8))
- Fix --distro-override parameter name in docs [\#16726](https://github.com/netdata/netdata/pull/16726) ([moschlar](https://github.com/moschlar))
- update go.d.plugin to v0.58.0 [\#16725](https://github.com/netdata/netdata/pull/16725) ([ilyam8](https://github.com/ilyam8))
- Add GHA workflow to upload kickstart script to our repo server. [\#16724](https://github.com/netdata/netdata/pull/16724) ([Ferroin](https://github.com/Ferroin))
- Add Netdata Mobile App to issue template config [\#16723](https://github.com/netdata/netdata/pull/16723) ([ilyam8](https://github.com/ilyam8))
- fix clock resolution detection [\#16720](https://github.com/netdata/netdata/pull/16720) ([ktsaou](https://github.com/ktsaou))
- cgroups: don't multiply cgroup\_check\_for\_new\_every by update\_every [\#16719](https://github.com/netdata/netdata/pull/16719) ([ilyam8](https://github.com/ilyam8))
- Add info to distros.yml for handling of legacy platforms. [\#16718](https://github.com/netdata/netdata/pull/16718) ([Ferroin](https://github.com/Ferroin))
- Regenerate integrations.js [\#16716](https://github.com/netdata/netdata/pull/16716) ([netdatabot](https://github.com/netdatabot))
- Add the Mobile App notification Integration [\#16715](https://github.com/netdata/netdata/pull/16715) ([sashwathn](https://github.com/sashwathn))
- Update GHA steps that handle artifacts to use latest versions of upload/download actions. [\#16714](https://github.com/netdata/netdata/pull/16714) ([Ferroin](https://github.com/Ferroin))
- CI runtime check cleanup [\#16713](https://github.com/netdata/netdata/pull/16713) ([Ferroin](https://github.com/Ferroin))
- disable logsmanagement when installing on macOS [\#16708](https://github.com/netdata/netdata/pull/16708) ([ilyam8](https://github.com/ilyam8))
- dyncfg v2 [\#16702](https://github.com/netdata/netdata/pull/16702) ([ktsaou](https://github.com/ktsaou))
- delay collecting double linked network interfaces [\#16701](https://github.com/netdata/netdata/pull/16701) ([ilyam8](https://github.com/ilyam8))
- fix quota calculation when the the db is empty [\#16699](https://github.com/netdata/netdata/pull/16699) ([ktsaou](https://github.com/ktsaou))
- disable logsmanagement when installing on macOS [\#16697](https://github.com/netdata/netdata/pull/16697) ([ilyam8](https://github.com/ilyam8))
- Remove Ubuntu 23.04 from the CI [\#16694](https://github.com/netdata/netdata/pull/16694) ([tkatsoulas](https://github.com/tkatsoulas))
- fix installing service file and start/stop ND using `launchctl` on macOS [\#16693](https://github.com/netdata/netdata/pull/16693) ([ilyam8](https://github.com/ilyam8))
- improve the error message when accessing functions [\#16692](https://github.com/netdata/netdata/pull/16692) ([ktsaou](https://github.com/ktsaou))
- cups exit on sigpipe [\#16691](https://github.com/netdata/netdata/pull/16691) ([ilyam8](https://github.com/ilyam8))
- fix minor omission on netdata-installers arguments [\#16690](https://github.com/netdata/netdata/pull/16690) ([tkatsoulas](https://github.com/tkatsoulas))
- Revert "Update artifact-handling actions to latest version." [\#16689](https://github.com/netdata/netdata/pull/16689) ([tkatsoulas](https://github.com/tkatsoulas))
- cmake log2journal netdatacli [\#16688](https://github.com/netdata/netdata/pull/16688) ([ktsaou](https://github.com/ktsaou))
- atomically load the metric reference count [\#16687](https://github.com/netdata/netdata/pull/16687) ([ktsaou](https://github.com/ktsaou))
- fix claiming on macOS [\#16686](https://github.com/netdata/netdata/pull/16686) ([ilyam8](https://github.com/ilyam8))
- add --disable-logsmanagement when building static [\#16684](https://github.com/netdata/netdata/pull/16684) ([ilyam8](https://github.com/ilyam8))
- fix exporting internal charts context and family [\#16683](https://github.com/netdata/netdata/pull/16683) ([ilyam8](https://github.com/ilyam8))
- Fatal relaxation of unknown page types. [\#16682](https://github.com/netdata/netdata/pull/16682) ([vkalintiris](https://github.com/vkalintiris))
- docs: add "Require Cloud" column to functions table [\#16681](https://github.com/netdata/netdata/pull/16681) ([ilyam8](https://github.com/ilyam8))
- cmake missing defines [\#16680](https://github.com/netdata/netdata/pull/16680) ([ktsaou](https://github.com/ktsaou))
- Fix alerts-configuration-manager.md [\#16679](https://github.com/netdata/netdata/pull/16679) ([Ancairon](https://github.com/Ancairon))
- kickstart: dont run install-required-packages.sh as root on macOS [\#16675](https://github.com/netdata/netdata/pull/16675) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.75.2 [\#16674](https://github.com/netdata/netdata/pull/16674) ([ilyam8](https://github.com/ilyam8))
- kickstart: add a note on how to access the UI to the success banner [\#16673](https://github.com/netdata/netdata/pull/16673) ([ilyam8](https://github.com/ilyam8))
- remove contrib/rhel [\#16672](https://github.com/netdata/netdata/pull/16672) ([ilyam8](https://github.com/ilyam8))
- Update binaries \(eBPF\) [\#16671](https://github.com/netdata/netdata/pull/16671) ([thiagoftsm](https://github.com/thiagoftsm))
- eBPF socket \(eBPF\) [\#16669](https://github.com/netdata/netdata/pull/16669) ([thiagoftsm](https://github.com/thiagoftsm))
- fix compiler warnings [\#16665](https://github.com/netdata/netdata/pull/16665) ([ktsaou](https://github.com/ktsaou))
- dont exceed buffer boundaries, when the buffer is empty [\#16664](https://github.com/netdata/netdata/pull/16664) ([ktsaou](https://github.com/ktsaou))
- set log level of too-old-data message to debug  [\#16663](https://github.com/netdata/netdata/pull/16663) ([ilyam8](https://github.com/ilyam8))
- Improve context load  [\#16659](https://github.com/netdata/netdata/pull/16659) ([stelfrag](https://github.com/stelfrag))
- Shutdown dbengine event loop properly [\#16658](https://github.com/netdata/netdata/pull/16658) ([stelfrag](https://github.com/stelfrag))
- docs: Correct chart\_labels summary [\#16656](https://github.com/netdata/netdata/pull/16656) ([sepek](https://github.com/sepek))
- Fix coverity issues [\#16655](https://github.com/netdata/netdata/pull/16655) ([stelfrag](https://github.com/stelfrag))
- Fix overrun in crc32set [\#16654](https://github.com/netdata/netdata/pull/16654) ([stelfrag](https://github.com/stelfrag))
- Necessary changes for Learn [\#16651](https://github.com/netdata/netdata/pull/16651) ([Ancairon](https://github.com/Ancairon))
- docs: add a few examples how to query Netdata logs using journalctl [\#16650](https://github.com/netdata/netdata/pull/16650) ([ilyam8](https://github.com/ilyam8))
- increase max response size to 100MiB [\#16649](https://github.com/netdata/netdata/pull/16649) ([ktsaou](https://github.com/ktsaou))
- rename bundle dashboard scripts [\#16648](https://github.com/netdata/netdata/pull/16648) ([ilyam8](https://github.com/ilyam8))
- update bundled UI to v6.72.0 [\#16647](https://github.com/netdata/netdata/pull/16647) ([ilyam8](https://github.com/ilyam8))
- Fix compilation error when using --disable-dbengine [\#16645](https://github.com/netdata/netdata/pull/16645) ([stelfrag](https://github.com/stelfrag))
- Create alerts-configuration-manager.md [\#16642](https://github.com/netdata/netdata/pull/16642) ([sashwathn](https://github.com/sashwathn))
- Add extra build flags to CMakeLists.txt. [\#16641](https://github.com/netdata/netdata/pull/16641) ([Ferroin](https://github.com/Ferroin))
- Assorted cleanup of native packaging code. [\#16640](https://github.com/netdata/netdata/pull/16640) ([Ferroin](https://github.com/Ferroin))
- Update artifact-handling actions to latest version. [\#16639](https://github.com/netdata/netdata/pull/16639) ([Ferroin](https://github.com/Ferroin))
- cmake: make WEB\_DIR configurable [\#16638](https://github.com/netdata/netdata/pull/16638) ([ilyam8](https://github.com/ilyam8))
- Remove code relying on autotools. [\#16634](https://github.com/netdata/netdata/pull/16634) ([vkalintiris](https://github.com/vkalintiris))
- docs: add "Rootless mode" to Docker install guide [\#16632](https://github.com/netdata/netdata/pull/16632) ([ilyam8](https://github.com/ilyam8))
- eBPF cgroup update [\#16630](https://github.com/netdata/netdata/pull/16630) ([thiagoftsm](https://github.com/thiagoftsm))
- Correctly handle basic permissions for most scripts on install. [\#16629](https://github.com/netdata/netdata/pull/16629) ([Ferroin](https://github.com/Ferroin))
- Fix UB of unaligned loads/stores and signed shifts. [\#16628](https://github.com/netdata/netdata/pull/16628) ([vkalintiris](https://github.com/vkalintiris))
- cgroups: filter lxcfs.service/.control [\#16620](https://github.com/netdata/netdata/pull/16620) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues, logically dead code and error checking [\#16618](https://github.com/netdata/netdata/pull/16618) ([stelfrag](https://github.com/stelfrag))
- Added energy efficiency img README.md [\#16617](https://github.com/netdata/netdata/pull/16617) ([Aliki92](https://github.com/Aliki92))
- Fix small coverity issue [\#16616](https://github.com/netdata/netdata/pull/16616) ([stelfrag](https://github.com/stelfrag))
- ndsudo - a helper to run privileged commands [\#16614](https://github.com/netdata/netdata/pull/16614) ([ktsaou](https://github.com/ktsaou))
- Robustness improvements to netdata-updater.sh [\#16613](https://github.com/netdata/netdata/pull/16613) ([candlerb](https://github.com/candlerb))
- Remove assert [\#16611](https://github.com/netdata/netdata/pull/16611) ([stelfrag](https://github.com/stelfrag))
- Remove CPack stuff from CMake [\#16608](https://github.com/netdata/netdata/pull/16608) ([vkalintiris](https://github.com/vkalintiris))
- Remove includes outside of libnetdata. [\#16607](https://github.com/netdata/netdata/pull/16607) ([vkalintiris](https://github.com/vkalintiris))
- Fix \(and improve\) Coverity scanning. [\#16605](https://github.com/netdata/netdata/pull/16605) ([Ferroin](https://github.com/Ferroin))
- Delete memory mode "map" and "save". [\#16604](https://github.com/netdata/netdata/pull/16604) ([vkalintiris](https://github.com/vkalintiris))
- remove v1 dashboard version check from installer [\#16603](https://github.com/netdata/netdata/pull/16603) ([ilyam8](https://github.com/ilyam8))
- fix not assigned proc\_count in installer [\#16602](https://github.com/netdata/netdata/pull/16602) ([ilyam8](https://github.com/ilyam8))
- improve enable\_feature function in the installer [\#16601](https://github.com/netdata/netdata/pull/16601) ([ilyam8](https://github.com/ilyam8))
- Remove build/ [\#16600](https://github.com/netdata/netdata/pull/16600) ([vkalintiris](https://github.com/vkalintiris))
- Allow passing cmake options with NETDATA\_CMAKE\_OPTIONS. [\#16598](https://github.com/netdata/netdata/pull/16598) ([vkalintiris](https://github.com/vkalintiris))
- Cleanup am files [\#16597](https://github.com/netdata/netdata/pull/16597) ([vkalintiris](https://github.com/vkalintiris))
- Fix coverity issues [\#16596](https://github.com/netdata/netdata/pull/16596) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16595](https://github.com/netdata/netdata/pull/16595) ([netdatabot](https://github.com/netdatabot))
- fix: use black version icon for Splunk in order to make it visible [\#16593](https://github.com/netdata/netdata/pull/16593) ([juacker](https://github.com/juacker))
- systemd-journal: exit if unable to locate journal data directories [\#16592](https://github.com/netdata/netdata/pull/16592) ([ilyam8](https://github.com/ilyam8))
- Fix coverity issues [\#16589](https://github.com/netdata/netdata/pull/16589) ([stelfrag](https://github.com/stelfrag))
- Regenerate integrations.js [\#16587](https://github.com/netdata/netdata/pull/16587) ([netdatabot](https://github.com/netdatabot))
- Adds docs for Splunk cloud notifications [\#16586](https://github.com/netdata/netdata/pull/16586) ([juacker](https://github.com/juacker))
- uninstaller remove log2journal and systemd-cat-native [\#16585](https://github.com/netdata/netdata/pull/16585) ([ilyam8](https://github.com/ilyam8))
- Handle coverity issues related to Y2K38\_SAFETY [\#16583](https://github.com/netdata/netdata/pull/16583) ([stelfrag](https://github.com/stelfrag))
- Update categories.yaml to add Logs [\#16582](https://github.com/netdata/netdata/pull/16582) ([sashwathn](https://github.com/sashwathn))
- Add Alpine Linux 3.19 to CI. [\#16579](https://github.com/netdata/netdata/pull/16579) ([Ferroin](https://github.com/Ferroin))
- Queries Progress [\#16574](https://github.com/netdata/netdata/pull/16574) ([ktsaou](https://github.com/ktsaou))
- disable cpu per core metrics by default [\#16572](https://github.com/netdata/netdata/pull/16572) ([ilyam8](https://github.com/ilyam8))
- make debugfs exit on sigpipe [\#16569](https://github.com/netdata/netdata/pull/16569) ([ilyam8](https://github.com/ilyam8))
- Fix memory leak during host chart label cleanup [\#16568](https://github.com/netdata/netdata/pull/16568) ([stelfrag](https://github.com/stelfrag))
- fix cpu arch/ram/disk values in buildinfo [\#16567](https://github.com/netdata/netdata/pull/16567) ([ilyam8](https://github.com/ilyam8))
- Remove Netdata packages from APT cache when attempting to install. [\#16566](https://github.com/netdata/netdata/pull/16566) ([Ferroin](https://github.com/Ferroin))
- Resolve issue on startup in servers with 1 core [\#16565](https://github.com/netdata/netdata/pull/16565) ([stelfrag](https://github.com/stelfrag))
- Update naming for swagger api [\#16564](https://github.com/netdata/netdata/pull/16564) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix release metadata workflow [\#16563](https://github.com/netdata/netdata/pull/16563) ([tkatsoulas](https://github.com/tkatsoulas))
- Make  the systemd-journal mandatory package on Centos 7  and Amazon linux 2 [\#16562](https://github.com/netdata/netdata/pull/16562) ([tkatsoulas](https://github.com/tkatsoulas))
- Fix for AMD GPU drm different format proc file [\#16561](https://github.com/netdata/netdata/pull/16561) ([MrZammler](https://github.com/MrZammler))
- Revert "remove discourse badge from readme" [\#16560](https://github.com/netdata/netdata/pull/16560) ([ilyam8](https://github.com/ilyam8))
- Change the workflow on how we set the right permissions for perf-plugin [\#16558](https://github.com/netdata/netdata/pull/16558) ([tkatsoulas](https://github.com/tkatsoulas))
- Add README for gorilla [\#16553](https://github.com/netdata/netdata/pull/16553) ([vkalintiris](https://github.com/vkalintiris))
- Bump new version on the release changelog GHA [\#16551](https://github.com/netdata/netdata/pull/16551) ([tkatsoulas](https://github.com/tkatsoulas))
- set "HOME" after switching to netdata user [\#16548](https://github.com/netdata/netdata/pull/16548) ([ilyam8](https://github.com/ilyam8))
- code cleanup [\#16542](https://github.com/netdata/netdata/pull/16542) ([ktsaou](https://github.com/ktsaou))
- Assorted kickstart script fixes. [\#16537](https://github.com/netdata/netdata/pull/16537) ([Ferroin](https://github.com/Ferroin))
- wip documentation about functions table [\#16535](https://github.com/netdata/netdata/pull/16535) ([ktsaou](https://github.com/ktsaou))

## [v1.44.3](https://github.com/netdata/netdata/tree/v1.44.3) (2024-02-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.2...v1.44.3)

## [v1.44.2](https://github.com/netdata/netdata/tree/v1.44.2) (2024-02-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.1...v1.44.2)

## [v1.44.1](https://github.com/netdata/netdata/tree/v1.44.1) (2023-12-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.44.0...v1.44.1)

## [v1.44.0](https://github.com/netdata/netdata/tree/v1.44.0) (2023-12-06)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.43.2...v1.44.0)

**Merged pull requests:**

- update bundled UI to v6.66.1 [\#16554](https://github.com/netdata/netdata/pull/16554) ([ilyam8](https://github.com/ilyam8))
- Improve page validity check during database extent load [\#16552](https://github.com/netdata/netdata/pull/16552) ([stelfrag](https://github.com/stelfrag))
- Proper Learn-friendly links [\#16547](https://github.com/netdata/netdata/pull/16547) ([Ancairon](https://github.com/Ancairon))
- docs required for release [\#16546](https://github.com/netdata/netdata/pull/16546) ([ktsaou](https://github.com/ktsaou))
- Add option to change page type for tier 0 to gorilla [\#16545](https://github.com/netdata/netdata/pull/16545) ([vkalintiris](https://github.com/vkalintiris))
- fix alpine deps [\#16543](https://github.com/netdata/netdata/pull/16543) ([tkatsoulas](https://github.com/tkatsoulas))
- change level to debug "took too long to be updated" [\#16540](https://github.com/netdata/netdata/pull/16540) ([ilyam8](https://github.com/ilyam8))
- apps: fix uptime for groups with 0 processes [\#16538](https://github.com/netdata/netdata/pull/16538) ([ilyam8](https://github.com/ilyam8))
- Regenerate integrations.js [\#16536](https://github.com/netdata/netdata/pull/16536) ([netdatabot](https://github.com/netdatabot))
- Reorg kickstart guide's steps [\#16534](https://github.com/netdata/netdata/pull/16534) ([tkatsoulas](https://github.com/tkatsoulas))
- update go.d plugin to v0.57.2 [\#16533](https://github.com/netdata/netdata/pull/16533) ([ilyam8](https://github.com/ilyam8))
- Update getting-started-light-poc.md [\#16532](https://github.com/netdata/netdata/pull/16532) ([M4itee](https://github.com/M4itee))
- Acquire receiver\_lock to to avoid race condition [\#16531](https://github.com/netdata/netdata/pull/16531) ([stelfrag](https://github.com/stelfrag))
- link aclk.log to stdout in docker [\#16529](https://github.com/netdata/netdata/pull/16529) ([ilyam8](https://github.com/ilyam8))
- Update getting-started.md [\#16528](https://github.com/netdata/netdata/pull/16528) ([Ancairon](https://github.com/Ancairon))
- Make image available to Learn + add a category overview page for new … [\#16527](https://github.com/netdata/netdata/pull/16527) ([Ancairon](https://github.com/Ancairon))
- logs-management: Disable logs management monitoring section [\#16525](https://github.com/netdata/netdata/pull/16525) ([Dim-P](https://github.com/Dim-P))

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
