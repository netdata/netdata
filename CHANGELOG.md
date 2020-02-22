# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.20.0...HEAD)

**Merged pull requests:**

- Update the manual install documentation with better info. [\#8088](https://github.com/netdata/netdata/pull/8088) ([Ferroin](https://github.com/Ferroin))
- Tutorials to support v1.20 release [\#7943](https://github.com/netdata/netdata/pull/7943) ([joelhans](https://github.com/joelhans))
- Cross-host docker builds [\#7754](https://github.com/netdata/netdata/pull/7754) ([amoss](https://github.com/amoss))

## [v1.20.0](https://github.com/netdata/netdata/tree/v1.20.0) (2020-02-21)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.19.0...v1.20.0)

**Merged pull requests:**

- Ebpf coverity [\#8135](https://github.com/netdata/netdata/pull/8135) ([thiagoftsm](https://github.com/thiagoftsm))
- Small ebpf fixes [\#8133](https://github.com/netdata/netdata/pull/8133) ([thiagoftsm](https://github.com/thiagoftsm))
- Update eBPF docs with better install/enable instructions [\#8125](https://github.com/netdata/netdata/pull/8125) ([joelhans](https://github.com/joelhans))
- Fixes NetData installer on \*BSD systems post libmosquitto / eBPF [\#8121](https://github.com/netdata/netdata/pull/8121) ([prologic](https://github.com/prologic))
- Fix nightly RPM builds. [\#8109](https://github.com/netdata/netdata/pull/8109) ([Ferroin](https://github.com/Ferroin))
- Add Patti to docs and .md codeowners [\#8092](https://github.com/netdata/netdata/pull/8092) ([joelhans](https://github.com/joelhans))
- Docs: Fix Node and Bash collector module titles [\#8086](https://github.com/netdata/netdata/pull/8086) ([joelhans](https://github.com/joelhans))
- Add handling of libmosquitto to binary packages. [\#8085](https://github.com/netdata/netdata/pull/8085) ([Ferroin](https://github.com/Ferroin))
- Fixes text if current version is \>= latest version and already installed [\#8078](https://github.com/netdata/netdata/pull/8078) ([prologic](https://github.com/prologic))
- Revert "Added hack to sha256sums.txt to force users of NetData on v1.19.0-483 with a broken updater to update to latest" [\#8076](https://github.com/netdata/netdata/pull/8076) ([prologic](https://github.com/prologic))
- Integrates the eBPF Kernel Collector with the NetData Installer [\#8075](https://github.com/netdata/netdata/pull/8075) ([prologic](https://github.com/prologic))
- updates for issue 8006 [\#8074](https://github.com/netdata/netdata/pull/8074) ([shortpatti](https://github.com/shortpatti))
- github/actions: update labeler config and bump version [\#8071](https://github.com/netdata/netdata/pull/8071) ([ilyam8](https://github.com/ilyam8))
- Adding testing section to the PR template. [\#8068](https://github.com/netdata/netdata/pull/8068) ([amoss](https://github.com/amoss))
- Update to new release of libmosquitto fork. [\#8067](https://github.com/netdata/netdata/pull/8067) ([Ferroin](https://github.com/Ferroin))
- Make temporary directories relative to $TEMPDIR. [\#8066](https://github.com/netdata/netdata/pull/8066) ([Ferroin](https://github.com/Ferroin))
- collectors: apps.plugin: apps\_groups: create 'dns' group. [\#8058](https://github.com/netdata/netdata/pull/8058) ([k0ste](https://github.com/k0ste))
- Added hack to sha256sums.txt to force users of NetData on v1.19.0-483 with a broken updater to update to latest [\#8057](https://github.com/netdata/netdata/pull/8057) ([prologic](https://github.com/prologic))
- Add config instructions to each collector module README  [\#8052](https://github.com/netdata/netdata/pull/8052) ([joelhans](https://github.com/joelhans))
- Add required build dep for ACLK dependencies to Dockerfile. [\#8047](https://github.com/netdata/netdata/pull/8047) ([Ferroin](https://github.com/Ferroin))
- github/workflow: change labeler [\#8032](https://github.com/netdata/netdata/pull/8032) ([ilyam8](https://github.com/ilyam8))
- Docs: Promote DB engine/long-term metrics storage more heavily [\#8031](https://github.com/netdata/netdata/pull/8031) ([joelhans](https://github.com/joelhans))
- Check if ACLK can be built [\#8030](https://github.com/netdata/netdata/pull/8030) ([underhood](https://github.com/underhood))
- Fixes conditional for NetData Updater when checking for new updates [\#8028](https://github.com/netdata/netdata/pull/8028) ([gmeszaros](https://github.com/gmeszaros))
- Add support for libmosquitto to netdata-installer.sh for ACLK support. [\#8025](https://github.com/netdata/netdata/pull/8025) ([Ferroin](https://github.com/Ferroin))
- Fix: Documentation states in one place default is dbengine and in another save is default [\#8017](https://github.com/netdata/netdata/pull/8017) ([underhood](https://github.com/underhood))
- invalid literal for float\(\): NN.NNt [\#8013](https://github.com/netdata/netdata/pull/8013) ([blaines](https://github.com/blaines))
- Fix missing variables on stream [\#8011](https://github.com/netdata/netdata/pull/8011) ([thiagoftsm](https://github.com/thiagoftsm))
- /docs/generator: use h1 heading as page title for collectors pages [\#8009](https://github.com/netdata/netdata/pull/8009) ([ilyam8](https://github.com/ilyam8))
- /docs/generator: build docs only for go.d itself and its modules [\#8005](https://github.com/netdata/netdata/pull/8005) ([ilyam8](https://github.com/ilyam8))
- Update database servers group in apps\_groups.conf [\#8004](https://github.com/netdata/netdata/pull/8004) ([DefauIt](https://github.com/DefauIt))
- /collectors/charts.d.plugin: fix `os\_id` detection in `run` [\#8002](https://github.com/netdata/netdata/pull/8002) ([ilyam8](https://github.com/ilyam8))
- /docs/generator: add an option to use last dir name for page name  [\#7997](https://github.com/netdata/netdata/pull/7997) ([ilyam8](https://github.com/ilyam8))
- Refactor collectors documentation [\#7996](https://github.com/netdata/netdata/pull/7996) ([joelhans](https://github.com/joelhans))
- Docs: Change sed to allow parentheses in heading links [\#7995](https://github.com/netdata/netdata/pull/7995) ([joelhans](https://github.com/joelhans))
- Fix CentOS 7 RPM build failures. [\#7993](https://github.com/netdata/netdata/pull/7993) ([Ferroin](https://github.com/Ferroin))
- Don't pretend we're building i386 RPM packages when we aren't. [\#7989](https://github.com/netdata/netdata/pull/7989) ([Ferroin](https://github.com/Ferroin))
- initial MQTT over Secure Websockets support for ACLK [\#7988](https://github.com/netdata/netdata/pull/7988) ([underhood](https://github.com/underhood))
- Fix permissions issues caused by 986bc2052. [\#7984](https://github.com/netdata/netdata/pull/7984) ([Ferroin](https://github.com/Ferroin))
- collectors: apps.plugin: apps\_groups: update ceph & samba collections. [\#7982](https://github.com/netdata/netdata/pull/7982) ([k0ste](https://github.com/k0ste))
- /collectors/python.d.plugin/third\_party: patch monotonic synology6 [\#7980](https://github.com/netdata/netdata/pull/7980) ([ilyam8](https://github.com/ilyam8))
- eBPF process plugin [\#7979](https://github.com/netdata/netdata/pull/7979) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix typos in docs/step-by-step/step-06.md [\#7978](https://github.com/netdata/netdata/pull/7978) ([joelhans](https://github.com/joelhans))
- /collectors/charts.d.plugin: format modules [\#7973](https://github.com/netdata/netdata/pull/7973) ([ilyam8](https://github.com/ilyam8))
- Fixes static builds and nightlies [\#7971](https://github.com/netdata/netdata/pull/7971) ([prologic](https://github.com/prologic))
- Adds GHA Workflow to actually Build the Agent across all the OS/Distro\(s\) we support today [\#7969](https://github.com/netdata/netdata/pull/7969) ([prologic](https://github.com/prologic))
- Indicate FreeIPMI supported in Docker image [\#7964](https://github.com/netdata/netdata/pull/7964) ([lassebm](https://github.com/lassebm))
- CODEOWNERS: change collectors/charts.d.plugin/ owners [\#7963](https://github.com/netdata/netdata/pull/7963) ([ilyam8](https://github.com/ilyam8))
- /collectors/charts.d.plugin: remove deprecated modules [\#7962](https://github.com/netdata/netdata/pull/7962) ([ilyam8](https://github.com/ilyam8))
- Fix cmake build error [\#7960](https://github.com/netdata/netdata/pull/7960) ([mfundul](https://github.com/mfundul))
- /pythond.d/UrlService.py: add body [\#7956](https://github.com/netdata/netdata/pull/7956) ([ilyam8](https://github.com/ilyam8))
- netdata-updater.sh: explicitly return 0 from update [\#7955](https://github.com/netdata/netdata/pull/7955) ([ilyam8](https://github.com/ilyam8))
- update httpcheck.conf [\#7952](https://github.com/netdata/netdata/pull/7952) ([yasharne](https://github.com/yasharne))
- Fix wrong code fragments in signing in [\#7950](https://github.com/netdata/netdata/pull/7950) ([cakrit](https://github.com/cakrit))
- Adds a GHA workflow to test install-required-packages [\#7949](https://github.com/netdata/netdata/pull/7949) ([prologic](https://github.com/prologic))
- install go.d.plugin only if there is new version [\#7946](https://github.com/netdata/netdata/pull/7946) ([ilyam8](https://github.com/ilyam8))
- Fix variety of linter errors across docs [\#7944](https://github.com/netdata/netdata/pull/7944) ([joelhans](https://github.com/joelhans))
- Adds support for only performing updates if there is a newer version [\#7939](https://github.com/netdata/netdata/pull/7939) ([prologic](https://github.com/prologic))
- Fixes error/warnings found by shellcheck for ./packaging/installer/netdata-updater.sh [\#7938](https://github.com/netdata/netdata/pull/7938) ([prologic](https://github.com/prologic))
- Re-formatts ./packaging/installer/netdata-updater.sh with shfmt -w -i 2 -ci -sr [\#7937](https://github.com/netdata/netdata/pull/7937) ([prologic](https://github.com/prologic))
- Fixes a typo in ./packaging/installer/functions.sh with wrong message for updater cron script [\#7934](https://github.com/netdata/netdata/pull/7934) ([prologic](https://github.com/prologic))
- Fixes support for editing configuration when NetData is installed to a symlinked /opt [\#7933](https://github.com/netdata/netdata/pull/7933) ([prologic](https://github.com/prologic))
- Re-formats ./system/edit-config.in with shfmt -w -i 2 -ci -sr [\#7932](https://github.com/netdata/netdata/pull/7932) ([prologic](https://github.com/prologic))
- Fixes a bug in DO\_NOT\_TRACK expression [\#7929](https://github.com/netdata/netdata/pull/7929) ([prologic](https://github.com/prologic))
- Assorted cleanup items in the RPM spec file. [\#7927](https://github.com/netdata/netdata/pull/7927) ([Ferroin](https://github.com/Ferroin))
- /collectors/python.d/varnish: collect smf metrics [\#7926](https://github.com/netdata/netdata/pull/7926) ([ilyam8](https://github.com/ilyam8))
- Cleanup of macOS installation docs [\#7925](https://github.com/netdata/netdata/pull/7925) ([joelhans](https://github.com/joelhans))
- Fix typo in PULL\_REQUEST\_TEMPLATE [\#7924](https://github.com/netdata/netdata/pull/7924) ([joelhans](https://github.com/joelhans))
- Set ownership correctly for plugins in netdata-installer.sh [\#7923](https://github.com/netdata/netdata/pull/7923) ([Ferroin](https://github.com/Ferroin))
- Reformats ./packaging/installer/install-required-packages.sh with: shfmt -w -i 2 -ci -sr [\#7915](https://github.com/netdata/netdata/pull/7915) ([prologic](https://github.com/prologic))
- Adds new simpler \(Alpine based\) Dockerfile for quick dev and testing [\#7914](https://github.com/netdata/netdata/pull/7914) ([prologic](https://github.com/prologic))
- Add doc with post-install instructions for GCP [\#7912](https://github.com/netdata/netdata/pull/7912) ([joelhans](https://github.com/joelhans))
- health\_doc\_name: Clarify the rules to create an alarm name [\#7911](https://github.com/netdata/netdata/pull/7911) ([thiagoftsm](https://github.com/thiagoftsm))
- Add docs about using caching proxies with our package repos. [\#7909](https://github.com/netdata/netdata/pull/7909) ([Ferroin](https://github.com/Ferroin))
- ACLK agent 1 [\#7894](https://github.com/netdata/netdata/pull/7894) ([stelfrag](https://github.com/stelfrag))
- Adds docs for how to build/install NetData on CentOS 8.x [\#7890](https://github.com/netdata/netdata/pull/7890) ([prologic](https://github.com/prologic))
- Clarify editing health config files in health quickstart [\#7883](https://github.com/netdata/netdata/pull/7883) ([joelhans](https://github.com/joelhans))
- installer: include go.d.plugin version v0.15.0 [\#7882](https://github.com/netdata/netdata/pull/7882) ([ilyam8](https://github.com/ilyam8))
- Missing extern [\#7877](https://github.com/netdata/netdata/pull/7877) ([thiagoftsm](https://github.com/thiagoftsm))
- collectors/python.d/phpfpm: fix readme and per process chart titles [\#7876](https://github.com/netdata/netdata/pull/7876) ([ilyam8](https://github.com/ilyam8))
- .travis.yml: Add -fno-common to CFLAGS [\#7870](https://github.com/netdata/netdata/pull/7870) ([candrews](https://github.com/candrews))
- Add disk size detection to system-info.sh. [\#7866](https://github.com/netdata/netdata/pull/7866) ([Ferroin](https://github.com/Ferroin))
- Update `api/v1/info ` [\#7862](https://github.com/netdata/netdata/pull/7862) ([thiagoftsm](https://github.com/thiagoftsm))
- /collectors/python.d: remove unbound module [\#7853](https://github.com/netdata/netdata/pull/7853) ([ilyam8](https://github.com/ilyam8))
- Stream with version [\#7851](https://github.com/netdata/netdata/pull/7851) ([thiagoftsm](https://github.com/thiagoftsm))
- python.d/retroshare: add readme [\#7849](https://github.com/netdata/netdata/pull/7849) ([ilyam8](https://github.com/ilyam8))
- Fixes and improves the installer/updater shell scripts. [\#7847](https://github.com/netdata/netdata/pull/7847) ([prologic](https://github.com/prologic))
- Adds support for opting out of telemetry via the DO\_NOT\_TRACK envirnment variable [\#7846](https://github.com/netdata/netdata/pull/7846) ([prologic](https://github.com/prologic))
- Fixed typo in README [\#7843](https://github.com/netdata/netdata/pull/7843) ([Jiab77](https://github.com/Jiab77))
- Docs: Overhaul of installation documentation [\#7841](https://github.com/netdata/netdata/pull/7841) ([joelhans](https://github.com/joelhans))
- alarms\_values: New endpoint [\#7836](https://github.com/netdata/netdata/pull/7836) ([thiagoftsm](https://github.com/thiagoftsm))
- Update collect-apache-nginx-web-logs.md to deprecated [\#7835](https://github.com/netdata/netdata/pull/7835) ([joelhans](https://github.com/joelhans))
- collectors/python.d: format modules code [\#7832](https://github.com/netdata/netdata/pull/7832) ([ilyam8](https://github.com/ilyam8))
- Remove all refernces to .keep files [\#7829](https://github.com/netdata/netdata/pull/7829) ([prologic](https://github.com/prologic))
- Adds ReviewDog CI checks for JavaScript [\#7828](https://github.com/netdata/netdata/pull/7828) ([prologic](https://github.com/prologic))
- Adds ReviewDog CI checks for Golang [\#7827](https://github.com/netdata/netdata/pull/7827) ([prologic](https://github.com/prologic))
- Don't remove groups/users in Debian postrm [\#7817](https://github.com/netdata/netdata/pull/7817) ([prologic](https://github.com/prologic))
- node.d/snmp.node.js: format code [\#7816](https://github.com/netdata/netdata/pull/7816) ([ilyam8](https://github.com/ilyam8))
- Improve the system-info.sh script to report CPU and RAM meta-data. [\#7815](https://github.com/netdata/netdata/pull/7815) ([Ferroin](https://github.com/Ferroin))
- Attempt to use system service manager to shut down Netdata. [\#7814](https://github.com/netdata/netdata/pull/7814) ([Ferroin](https://github.com/Ferroin))
- add possibility to change badge text font color [\#7809](https://github.com/netdata/netdata/pull/7809) ([underhood](https://github.com/underhood))
- bug\_report improvements [\#7805](https://github.com/netdata/netdata/pull/7805) ([ilyam8](https://github.com/ilyam8))
- node.d/snmp: snmpv3 support [\#7802](https://github.com/netdata/netdata/pull/7802) ([ilyam8](https://github.com/ilyam8))
- Fixes install on FreeBSD systems with non GNU sed \(do't use -i\) [\#7796](https://github.com/netdata/netdata/pull/7796) ([prologic](https://github.com/prologic))
- Adds reviewdog/shellcheck to CI via Github Actions on changed shell scripts in PRs [\#7795](https://github.com/netdata/netdata/pull/7795) ([prologic](https://github.com/prologic))
- Fixes Source0 URL in RPM spec [\#7794](https://github.com/netdata/netdata/pull/7794) ([prologic](https://github.com/prologic))
- Better systemd service file [\#7790](https://github.com/netdata/netdata/pull/7790) ([amishmm](https://github.com/amishmm))
- Fix unit tests for the exporting engine [\#7784](https://github.com/netdata/netdata/pull/7784) ([vlvkobal](https://github.com/vlvkobal))
- Remove unnessecary `echo` call in updater. [\#7783](https://github.com/netdata/netdata/pull/7783) ([Ferroin](https://github.com/Ferroin))
- Fix CSV -\> SSV in docs [\#7782](https://github.com/netdata/netdata/pull/7782) ([cosmix](https://github.com/cosmix))
- Fix a Coverity issue [\#7780](https://github.com/netdata/netdata/pull/7780) ([vlvkobal](https://github.com/vlvkobal))
- Fix libuv IPC pipe cleanup problem [\#7778](https://github.com/netdata/netdata/pull/7778) ([mfundul](https://github.com/mfundul))
- Drop dirty dbengine pages if disk cannot keep up [\#7777](https://github.com/netdata/netdata/pull/7777) ([mfundul](https://github.com/mfundul))
- Add a missing parameter to the allmetrics endpoint in Swagger Editor [\#7776](https://github.com/netdata/netdata/pull/7776) ([vlvkobal](https://github.com/vlvkobal))
- Issue 7488 docker labels [\#7770](https://github.com/netdata/netdata/pull/7770) ([amoss](https://github.com/amoss))
- Fix notification report [\#7769](https://github.com/netdata/netdata/pull/7769) ([thiagoftsm](https://github.com/thiagoftsm))
- Limit PR labeler runs to the main repo. [\#7768](https://github.com/netdata/netdata/pull/7768) ([Ferroin](https://github.com/Ferroin))
- Add an environment variable check to Travis configuration to allow disabling nightlies. [\#7765](https://github.com/netdata/netdata/pull/7765) ([Ferroin](https://github.com/Ferroin))
- add swagger docu for `fixed\_width\_lbl` and `fixed\_width\_val` [\#7764](https://github.com/netdata/netdata/pull/7764) ([underhood](https://github.com/underhood))
- Fix the formatting of the trailer line in the Debian changelog template. [\#7763](https://github.com/netdata/netdata/pull/7763) ([Ferroin](https://github.com/Ferroin))
- Filter out lxc cgroups which are not useful [\#7760](https://github.com/netdata/netdata/pull/7760) ([vlvkobal](https://github.com/vlvkobal))
- Small updates to dash.html [\#7757](https://github.com/netdata/netdata/pull/7757) ([tnyeanderson](https://github.com/tnyeanderson))
- Improve styling of documentation site and use Algolia search [\#7753](https://github.com/netdata/netdata/pull/7753) ([joelhans](https://github.com/joelhans))
- multiple files: fix typos [\#7752](https://github.com/netdata/netdata/pull/7752) ([schneiderl](https://github.com/schneiderl))
- on cloud error, inform user to update their netdata. [\#7750](https://github.com/netdata/netdata/pull/7750) ([jacekkolasa](https://github.com/jacekkolasa))
- Update stop-notifications-alarms.md [\#7737](https://github.com/netdata/netdata/pull/7737) ([yasharne](https://github.com/yasharne))
- Adds Docker based build system for Binary Packages, CI/CD, Smoke Testing and Development. [\#7735](https://github.com/netdata/netdata/pull/7735) ([prologic](https://github.com/prologic))
- Do not alert the \#automation channel on checksum failures that will fail a PR in CI anyway [\#7733](https://github.com/netdata/netdata/pull/7733) ([prologic](https://github.com/prologic))
- installer: include go.d.plugin version v0.14.1 [\#7732](https://github.com/netdata/netdata/pull/7732) ([ilyam8](https://github.com/ilyam8))
- Fix a check for nfnetlink\_conntrack.h [\#7727](https://github.com/netdata/netdata/pull/7727) ([vlvkobal](https://github.com/vlvkobal))
- Fixes support for read-only /lib on SystemD systems like CoreOS in  kickstart static64 [\#7726](https://github.com/netdata/netdata/pull/7726) ([prologic](https://github.com/prologic))
- Cleanup packaging/makeself/build-x86\_64-static.sh to use /bin/sh and remove use of sudo [\#7725](https://github.com/netdata/netdata/pull/7725) ([prologic](https://github.com/prologic))
- Add Korean translation to docs [\#7723](https://github.com/netdata/netdata/pull/7723) ([cakrit](https://github.com/cakrit))
- Control introduction of new languages in docs translation [\#7722](https://github.com/netdata/netdata/pull/7722) ([cakrit](https://github.com/cakrit))
- .travis.yml: Reduce notifications [\#7714](https://github.com/netdata/netdata/pull/7714) ([knatsakis](https://github.com/knatsakis))
- Make auto-updates work on kickstart-static64 installs. [\#7704](https://github.com/netdata/netdata/pull/7704) ([Ferroin](https://github.com/Ferroin))
- Parse host tags [\#7702](https://github.com/netdata/netdata/pull/7702) ([vlvkobal](https://github.com/vlvkobal))
- Support static builds for Prometheus remote write [\#7691](https://github.com/netdata/netdata/pull/7691) ([Ehekatl](https://github.com/Ehekatl))
- Adds a Dockerfile.docs for more easily and reproducibly building/rebuilding docs [\#7688](https://github.com/netdata/netdata/pull/7688) ([prologic](https://github.com/prologic))
- Fix setuid for freeipmi.plugin in Docker images [\#7684](https://github.com/netdata/netdata/pull/7684) ([lassebm](https://github.com/lassebm))
- Fixes for pfSense Installation [\#7665](https://github.com/netdata/netdata/pull/7665) ([prologic](https://github.com/prologic))
- Fix install permissions [\#7632](https://github.com/netdata/netdata/pull/7632) ([Ferroin](https://github.com/Ferroin))
- Restrict quotes in label values [\#7594](https://github.com/netdata/netdata/pull/7594) ([thiagoftsm](https://github.com/thiagoftsm))
- Reduce broken pipe errors [\#7588](https://github.com/netdata/netdata/pull/7588) ([thiagoftsm](https://github.com/thiagoftsm))
- Move the script for installing required packages into the main repo. [\#7563](https://github.com/netdata/netdata/pull/7563) ([Ferroin](https://github.com/Ferroin))
- Stream with labels [\#7549](https://github.com/netdata/netdata/pull/7549) ([thiagoftsm](https://github.com/thiagoftsm))
- Alarm Log labels [\#7548](https://github.com/netdata/netdata/pull/7548) ([thiagoftsm](https://github.com/thiagoftsm))
- Docs: Linter fixes for main README [\#7526](https://github.com/netdata/netdata/pull/7526) ([joelhans](https://github.com/joelhans))

## [v1.19.0](https://github.com/netdata/netdata/tree/v1.19.0) (2019-11-27)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.18.1...v1.19.0)

## [v1.18.1](https://github.com/netdata/netdata/tree/v1.18.1) (2019-10-18)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.18.0...v1.18.1)

## [v1.18.0](https://github.com/netdata/netdata/tree/v1.18.0) (2019-10-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.17.1...v1.18.0)

## [v1.17.1](https://github.com/netdata/netdata/tree/v1.17.1) (2019-09-12)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.17.0...v1.17.1)

## [v1.17.0](https://github.com/netdata/netdata/tree/v1.17.0) (2019-09-03)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.16.1...v1.17.0)

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
