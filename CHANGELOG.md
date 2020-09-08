# Changelog

## [**Next release**](https://github.com/netdata/netdata/tree/HEAD)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.24.0...HEAD)

**Merged pull requests:**

- Fix lock order reversal. \(Coverity defect CID 361629\) [\#9888](https://github.com/netdata/netdata/pull/9888) ([mfundul](https://github.com/mfundul))
- fix lack macOS ram info in system-info.sh [\#9882](https://github.com/netdata/netdata/pull/9882) ([weijing24](https://github.com/weijing24))
- installer: update go.d.plugin version to v0.21.0 [\#9881](https://github.com/netdata/netdata/pull/9881) ([ilyam8](https://github.com/ilyam8))
- Fixed handling of libJudy bundling for RPM packages. [\#9875](https://github.com/netdata/netdata/pull/9875) ([Ferroin](https://github.com/Ferroin))
- python.d/dnsdist: fix latency-avg chart units [\#9871](https://github.com/netdata/netdata/pull/9871) ([scottymuse](https://github.com/scottymuse))
- labeler: add `area/exporting` [\#9866](https://github.com/netdata/netdata/pull/9866) ([ilyam8](https://github.com/ilyam8))
- Fix race condition with orphan hosts [\#9862](https://github.com/netdata/netdata/pull/9862) ([mfundul](https://github.com/mfundul))
- health\_fix\_typo: Fix typo [\#9860](https://github.com/netdata/netdata/pull/9860) ([thiagoftsm](https://github.com/thiagoftsm))
- Removed dependency on libJudy for systems which don't have it. [\#9859](https://github.com/netdata/netdata/pull/9859) ([Ferroin](https://github.com/Ferroin))
- Fix multi-host DB corruption when legacy metrics reside in localhost. [\#9855](https://github.com/netdata/netdata/pull/9855) ([mfundul](https://github.com/mfundul))
- python.d/openldap: fix tls over ldap [\#9853](https://github.com/netdata/netdata/pull/9853) ([scatenag](https://github.com/scatenag))
- Fix broken `Edit this page` link in simple patterns doc [\#9847](https://github.com/netdata/netdata/pull/9847) ([joelhans](https://github.com/joelhans))
- fixes compilation warnings [\#9845](https://github.com/netdata/netdata/pull/9845) ([underhood](https://github.com/underhood))
- Don't install eBPF plugin components when they shouldn't be installed [\#9844](https://github.com/netdata/netdata/pull/9844) ([vlvkobal](https://github.com/vlvkobal))
- collect active processes limit feature v2 [\#9843](https://github.com/netdata/netdata/pull/9843) ([Ancairon](https://github.com/Ancairon))
- Fixed tmpdir handling failure on macOS/FreeBSD. [\#9842](https://github.com/netdata/netdata/pull/9842) ([Ferroin](https://github.com/Ferroin))
- Fixed bugs in handling of Python 3 dependencies on install. [\#9839](https://github.com/netdata/netdata/pull/9839) ([Ferroin](https://github.com/Ferroin))
- dashboard v1.4.2 [\#9837](https://github.com/netdata/netdata/pull/9837) ([jacekkolasa](https://github.com/jacekkolasa))
- Change a log level in cgroup-network helper [\#9836](https://github.com/netdata/netdata/pull/9836) ([vlvkobal](https://github.com/vlvkobal))
- Don't falsely report that the group was not deleted [\#9835](https://github.com/netdata/netdata/pull/9835) ([michmach](https://github.com/michmach))
- Fixed updater bug introduced by incomplete variable rename in \#8808. [\#9834](https://github.com/netdata/netdata/pull/9834) ([Ferroin](https://github.com/Ferroin))
- Fixes proxy forwarding claim\_id to old parent [\#9828](https://github.com/netdata/netdata/pull/9828) ([underhood](https://github.com/underhood))
- Remove Google Charts info from API doc [\#9826](https://github.com/netdata/netdata/pull/9826) ([joelhans](https://github.com/joelhans))
- Fix empty dbengine files [\#9820](https://github.com/netdata/netdata/pull/9820) ([mfundul](https://github.com/mfundul))
- Improve dbengine docs and add new multihost setting [\#9817](https://github.com/netdata/netdata/pull/9817) ([joelhans](https://github.com/joelhans))
- Fix old dashboard third-party packaging [\#9814](https://github.com/netdata/netdata/pull/9814) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix broken link and clean up frontmatter in health docs [\#9813](https://github.com/netdata/netdata/pull/9813) ([joelhans](https://github.com/joelhans))
- Fix docker packaging caddyserver basicauth link [\#9812](https://github.com/netdata/netdata/pull/9812) ([pando85](https://github.com/pando85))
- Fix install if system does not have ebpf.plugin [\#9809](https://github.com/netdata/netdata/pull/9809) ([roedie](https://github.com/roedie))
- rpm: Fix rpm build script version issues [\#9808](https://github.com/netdata/netdata/pull/9808) ([Saruspete](https://github.com/Saruspete))
- Fixed handling of offline installs. [\#9805](https://github.com/netdata/netdata/pull/9805) ([Ferroin](https://github.com/Ferroin))
- Add `claimed\_id` for child nodes streamed to their parents [\#9804](https://github.com/netdata/netdata/pull/9804) ([underhood](https://github.com/underhood))
- Improved temporary directory checking in installer and updater. [\#9797](https://github.com/netdata/netdata/pull/9797) ([Ferroin](https://github.com/Ferroin))
- Fix loading custom dashboard\_info in /old dashboard [\#9792](https://github.com/netdata/netdata/pull/9792) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix redirect with parameters [\#9790](https://github.com/netdata/netdata/pull/9790) ([thiagoftsm](https://github.com/thiagoftsm))
- dashboard v1.3.1 [\#9786](https://github.com/netdata/netdata/pull/9786) ([jacekkolasa](https://github.com/jacekkolasa))
- Fix long stats.d chart names \(suggested by @vince-lessbits\) [\#9783](https://github.com/netdata/netdata/pull/9783) ([amoss](https://github.com/amoss))
- Fix Travis CI builds and skip Fedora 31 i386 build/test cycles [\#9781](https://github.com/netdata/netdata/pull/9781) ([prologic](https://github.com/prologic))
- Fix UNIX socket access with kickstart-static64 [\#9780](https://github.com/netdata/netdata/pull/9780) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix timestamps for global variables in Prometheus output [\#9779](https://github.com/netdata/netdata/pull/9779) ([vlvkobal](https://github.com/vlvkobal))
- Added code to bundle libJudy on systems which do not provide a usable copy of it. [\#9776](https://github.com/netdata/netdata/pull/9776) ([Ferroin](https://github.com/Ferroin))
- Fix HTTP header for the remote write exporting connector [\#9775](https://github.com/netdata/netdata/pull/9775) ([vlvkobal](https://github.com/vlvkobal))
- Fix broken link in privacy policy [\#9771](https://github.com/netdata/netdata/pull/9771) ([joelhans](https://github.com/joelhans))
- Fix numerous bugs in duplicate install handling [\#9769](https://github.com/netdata/netdata/pull/9769) ([Ferroin](https://github.com/Ferroin))
- Add collecting `maxmemory` to the Python-based Redis collector [\#9767](https://github.com/netdata/netdata/pull/9767) ([ilyam8](https://github.com/ilyam8))
- Fix unit tests for exporting engine [\#9766](https://github.com/netdata/netdata/pull/9766) ([vlvkobal](https://github.com/vlvkobal))
- fix\_broken\_pipe: Fix netfilter for it closes when sigpipe happens [\#9756](https://github.com/netdata/netdata/pull/9756) ([thiagoftsm](https://github.com/thiagoftsm))
- Add support for IP ranges to Python-based isc\_dhcpd collector [\#9755](https://github.com/netdata/netdata/pull/9755) ([vsc55](https://github.com/vsc55))
- Improve eBPF plugin by removing unnecessary debug messages [\#9754](https://github.com/netdata/netdata/pull/9754) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix packaging to enable eBPF collector only if enabled in config.h [\#9752](https://github.com/netdata/netdata/pull/9752) ([Saruspete](https://github.com/Saruspete))
- Add check for spurious wakeups [\#9751](https://github.com/netdata/netdata/pull/9751) ([vlvkobal](https://github.com/vlvkobal))
- Fix code formatting for the mdstat collector [\#9749](https://github.com/netdata/netdata/pull/9749) ([vlvkobal](https://github.com/vlvkobal))
- Fix exporting update point [\#9748](https://github.com/netdata/netdata/pull/9748) ([vlvkobal](https://github.com/vlvkobal))
- Clarify which notifications are received when the "|critical" limit is set [\#9740](https://github.com/netdata/netdata/pull/9740) ([cakrit](https://github.com/cakrit))
- Fix flushing errors [\#9738](https://github.com/netdata/netdata/pull/9738) ([mfundul](https://github.com/mfundul))
- Added proper certificate handling cURL in our static build. [\#9733](https://github.com/netdata/netdata/pull/9733) ([Ferroin](https://github.com/Ferroin))
- Added code to release memory used by the global GUID map [\#9729](https://github.com/netdata/netdata/pull/9729) ([stelfrag](https://github.com/stelfrag))
- Add CAP\_SYS\_CHROOT for netdata service [\#9726](https://github.com/netdata/netdata/pull/9726) ([vlvkobal](https://github.com/vlvkobal))
- Fixed global GUID map memory leak [\#9725](https://github.com/netdata/netdata/pull/9725) ([stelfrag](https://github.com/stelfrag))
- Fix proxy redirect [\#9722](https://github.com/netdata/netdata/pull/9722) ([thiagoftsm](https://github.com/thiagoftsm))
- Add v1.24 news to main README [\#9721](https://github.com/netdata/netdata/pull/9721) ([aabatangle](https://github.com/aabatangle))
- Fix crash when receiving malformed labels via streaming. [\#9715](https://github.com/netdata/netdata/pull/9715) ([mfundul](https://github.com/mfundul))
- Fixed issue with missing alarms [\#9712](https://github.com/netdata/netdata/pull/9712) ([stelfrag](https://github.com/stelfrag))
- Set the default value of the home directory to the environment's HOME [\#9711](https://github.com/netdata/netdata/pull/9711) ([cakrit](https://github.com/cakrit))
- Remove broken optimization in the sender thread [\#9703](https://github.com/netdata/netdata/pull/9703) ([amoss](https://github.com/amoss))
- Send follow up alarms when the initial status matches the notification [\#9698](https://github.com/netdata/netdata/pull/9698) ([cakrit](https://github.com/cakrit))
- Improve and correct vulnerability reporting instructions [\#9696](https://github.com/netdata/netdata/pull/9696) ([cakrit](https://github.com/cakrit))
- Fix collectors on MacOS and FreeBSD to ignore archived charts. [\#9695](https://github.com/netdata/netdata/pull/9695) ([mfundul](https://github.com/mfundul))
- Fix print message when building for Ubuntu Focal [\#9694](https://github.com/netdata/netdata/pull/9694) ([devinrsmith](https://github.com/devinrsmith))
- Fix alarm redirection link for Cloud to stop showing 404 [\#9688](https://github.com/netdata/netdata/pull/9688) ([cakrit](https://github.com/cakrit))
- Fix high CPU in IPFS collector by disabling call to the `/api/v0/stats/repo` endpoint by default [\#9687](https://github.com/netdata/netdata/pull/9687) ([ilyam8](https://github.com/ilyam8))
- Fix netdata/netdata Docker image size [\#9669](https://github.com/netdata/netdata/pull/9669) ([prologic](https://github.com/prologic))
- Add option for multiple storage backends in Python-based Varnish collector [\#9668](https://github.com/netdata/netdata/pull/9668) ([florianmagnin](https://github.com/florianmagnin))
- Fix for ignored LXC containers [\#9645](https://github.com/netdata/netdata/pull/9645) ([vlvkobal](https://github.com/vlvkobal))
- Fix systemd journal logs to remove PrivateMounts [\#9619](https://github.com/netdata/netdata/pull/9619) ([Steve8291](https://github.com/Steve8291))
- Add community link to readme [\#9602](https://github.com/netdata/netdata/pull/9602) ([zack-shoylev](https://github.com/zack-shoylev))
- Network viewer charts [\#9591](https://github.com/netdata/netdata/pull/9591) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix MySQL collector documentation to mention `netdata` user [\#9555](https://github.com/netdata/netdata/pull/9555) ([mrbarletta](https://github.com/mrbarletta))
- Add and document support for reading container names from Podman in cgroups.plugin [\#9474](https://github.com/netdata/netdata/pull/9474) ([K900](https://github.com/K900))

## [v1.24.0](https://github.com/netdata/netdata/tree/v1.24.0) (2020-08-10)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.2...v1.24.0)

**Merged pull requests:**

- Stop multi-host DB statistics from being counted multiple times. [\#9685](https://github.com/netdata/netdata/pull/9685) ([mfundul](https://github.com/mfundul))
- Hide archived chart from mdstat collector. [\#9667](https://github.com/netdata/netdata/pull/9667) ([mfundul](https://github.com/mfundul))
- Remove obsoleted libraries from install/uninstall scripts [\#9661](https://github.com/netdata/netdata/pull/9661) ([vlvkobal](https://github.com/vlvkobal))
- Fix missing comma. [\#9656](https://github.com/netdata/netdata/pull/9656) ([mfundul](https://github.com/mfundul))
- Fix Travis config [\#9655](https://github.com/netdata/netdata/pull/9655) ([prologic](https://github.com/prologic))
- Fix warning when compiled with gcc-10.1 [\#9651](https://github.com/netdata/netdata/pull/9651) ([thiagoftsm](https://github.com/thiagoftsm))
- Detect a buggy Ubuntu kernel [\#9648](https://github.com/netdata/netdata/pull/9648) ([vlvkobal](https://github.com/vlvkobal))
- installer: fix `govercomp` [\#9646](https://github.com/netdata/netdata/pull/9646) ([ilyam8](https://github.com/ilyam8))
- installer: update `go.d.plugin` version to v0.20.0 [\#9644](https://github.com/netdata/netdata/pull/9644) ([ilyam8](https://github.com/ilyam8))
- python.d: fix `find\_binary` [\#9641](https://github.com/netdata/netdata/pull/9641) ([ilyam8](https://github.com/ilyam8))
- dashboard v1.0.26 [\#9639](https://github.com/netdata/netdata/pull/9639) ([jacekkolasa](https://github.com/jacekkolasa))
- Fetch libbpf from netdata fork [\#9637](https://github.com/netdata/netdata/pull/9637) ([vlvkobal](https://github.com/vlvkobal))
- charts.d: fix `current\_time\_ms\_from\_date` on macOS [\#9636](https://github.com/netdata/netdata/pull/9636) ([ilyam8](https://github.com/ilyam8))
- Adjust check-kernel-config.sh to run in bash [\#9633](https://github.com/netdata/netdata/pull/9633) ([Steve8291](https://github.com/Steve8291))
- Fix Travis CI and remove deprecated/removed builds that have no upstream LXC image [\#9630](https://github.com/netdata/netdata/pull/9630) ([prologic](https://github.com/prologic))
- Added eBPF collector support to DEB and RPM packages. [\#9628](https://github.com/netdata/netdata/pull/9628) ([Ferroin](https://github.com/Ferroin))
- Fixed RPM default permissions for /usr/libexec/netdata [\#9621](https://github.com/netdata/netdata/pull/9621) ([Saruspete](https://github.com/Saruspete))
- Added sandboxing exception for `/run/netdata`. [\#9613](https://github.com/netdata/netdata/pull/9613) ([Ferroin](https://github.com/Ferroin))
- python.d/gearmand: handle func prefixes in `status\n` response [\#9610](https://github.com/netdata/netdata/pull/9610) ([ilyam8](https://github.com/ilyam8))
- Add support for DEB packages for Ubuntu 20.04 \(focal\) [\#9592](https://github.com/netdata/netdata/pull/9592) ([prologic](https://github.com/prologic))
- Removed delay in updater script for non-interactive runs from install scripts. [\#9589](https://github.com/netdata/netdata/pull/9589) ([Ferroin](https://github.com/Ferroin))
- Added proper handling for autogen on Ubuntu 18.04 [\#9586](https://github.com/netdata/netdata/pull/9586) ([Ferroin](https://github.com/Ferroin))
- Added lock dir [\#9584](https://github.com/netdata/netdata/pull/9584) ([vlvkobal](https://github.com/vlvkobal))
- Stop mdstat collector from looking up archived charts. [\#9583](https://github.com/netdata/netdata/pull/9583) ([mfundul](https://github.com/mfundul))
- Fixes mempcpy-\>memcpy [\#9575](https://github.com/netdata/netdata/pull/9575) ([underhood](https://github.com/underhood))
- Sends netdata.public.unique.id \(machine GUID\) with claim [\#9574](https://github.com/netdata/netdata/pull/9574) ([underhood](https://github.com/underhood))
- Added libbpf patch to make dist. [\#9571](https://github.com/netdata/netdata/pull/9571) ([Ferroin](https://github.com/Ferroin))
- Added CAP\_SYS\_RESOURCE to capability bounding set. [\#9569](https://github.com/netdata/netdata/pull/9569) ([Ferroin](https://github.com/Ferroin))
- charts.d.plugin: never use `-t` option for `timeout` [\#9568](https://github.com/netdata/netdata/pull/9568) ([ilyam8](https://github.com/ilyam8))
- Removed runtime support for Polymorphic Linux from our Docker containers. [\#9566](https://github.com/netdata/netdata/pull/9566) ([Ferroin](https://github.com/Ferroin))
- python.d: add job file lock registry [\#9564](https://github.com/netdata/netdata/pull/9564) ([ilyam8](https://github.com/ilyam8))
- Adding pihole to the dns app group [\#9557](https://github.com/netdata/netdata/pull/9557) ([bmatheny](https://github.com/bmatheny))
- Implemented multihost database [\#9556](https://github.com/netdata/netdata/pull/9556) ([stelfrag](https://github.com/stelfrag))
- health/megacli: change all instances of alarm to template [\#9553](https://github.com/netdata/netdata/pull/9553) ([tinyhammers](https://github.com/tinyhammers))
- Revert the eBPF package bundling that breaks the release and DEB packages. [\#9552](https://github.com/netdata/netdata/pull/9552) ([prologic](https://github.com/prologic))
- Read socket information from kernel ring [\#9549](https://github.com/netdata/netdata/pull/9549) ([thiagoftsm](https://github.com/thiagoftsm))
- Suppress warning -Wformat-truncation in ACLK [\#9547](https://github.com/netdata/netdata/pull/9547) ([underhood](https://github.com/underhood))
- Add 1.23.1 and 1.23.0 to news section [\#9518](https://github.com/netdata/netdata/pull/9518) ([joelhans](https://github.com/joelhans))
- Implemented default disk space size calculation for multihost db [\#9504](https://github.com/netdata/netdata/pull/9504) ([stelfrag](https://github.com/stelfrag))
- Network Viewer options [\#9495](https://github.com/netdata/netdata/pull/9495) ([thiagoftsm](https://github.com/thiagoftsm))
- Use the libbpf library for the eBPF plugin [\#9490](https://github.com/netdata/netdata/pull/9490) ([vlvkobal](https://github.com/vlvkobal))
- Implemented the HOST command in metadata log replay [\#9489](https://github.com/netdata/netdata/pull/9489) ([stelfrag](https://github.com/stelfrag))
- Add documentation to provide a comprehensive guide for package maintainers [\#9467](https://github.com/netdata/netdata/pull/9467) ([Ferroin](https://github.com/Ferroin))
- Add better checks for existing installs to the kickstart scripts. [\#9408](https://github.com/netdata/netdata/pull/9408) ([Ferroin](https://github.com/Ferroin))
- Fix Static Netdata to correctly build with Netdata Cloud support. [\#9381](https://github.com/netdata/netdata/pull/9381) ([prologic](https://github.com/prologic))
- nvidia\_smi: charts for memory used by each user and number of distinct users [\#9372](https://github.com/netdata/netdata/pull/9372) ([scatenag](https://github.com/scatenag))

## [v1.23.2](https://github.com/netdata/netdata/tree/v1.23.2) (2020-07-16)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.1...v1.23.2)

**Merged pull requests:**

- Fix SHA256 handling in eBPF bundling code. [\#9546](https://github.com/netdata/netdata/pull/9546) ([Ferroin](https://github.com/Ferroin))
- Disable failing unit tests in CMake build [\#9545](https://github.com/netdata/netdata/pull/9545) ([vlvkobal](https://github.com/vlvkobal))
- Fix compilation warnings [\#9544](https://github.com/netdata/netdata/pull/9544) ([vlvkobal](https://github.com/vlvkobal))
- Fixed stored number accuracy [\#9540](https://github.com/netdata/netdata/pull/9540) ([stelfrag](https://github.com/stelfrag))
- Add eBPF bundling script to `make dist`. [\#9539](https://github.com/netdata/netdata/pull/9539) ([Ferroin](https://github.com/Ferroin))
- Fix CMake build failing if ACLK is disabled [\#9537](https://github.com/netdata/netdata/pull/9537) ([underhood](https://github.com/underhood))
- Fix transition from archived to active charts not generating alarms [\#9536](https://github.com/netdata/netdata/pull/9536) ([mfundul](https://github.com/mfundul))
- Update apps\_groups.conf [\#9535](https://github.com/netdata/netdata/pull/9535) ([AliMickey](https://github.com/AliMickey))
- Fix PyMySQL library to respect `my.cnf` parameter [\#9526](https://github.com/netdata/netdata/pull/9526) ([anirudhdggl](https://github.com/anirudhdggl))
- Remove health from archived metrics [\#9520](https://github.com/netdata/netdata/pull/9520) ([mfundul](https://github.com/mfundul))
- Fix now\_ms in charts.d collector to prevent tc-qos-helper crashes [\#9510](https://github.com/netdata/netdata/pull/9510) ([ilyam8](https://github.com/ilyam8))
- Fix an issue with random crashes when updating a chart's metadata on the fly [\#9509](https://github.com/netdata/netdata/pull/9509) ([stelfrag](https://github.com/stelfrag))
- Fix python.d crashes by adding a lock to stdout write function [\#9508](https://github.com/netdata/netdata/pull/9508) ([ilyam8](https://github.com/ilyam8))
- Fix the check condition for chart name change [\#9503](https://github.com/netdata/netdata/pull/9503) ([stelfrag](https://github.com/stelfrag))
- Fix ACLK protocol version always parsed as 0 [\#9502](https://github.com/netdata/netdata/pull/9502) ([underhood](https://github.com/underhood))
- Fix broken link in Kavenegar notification doc [\#9492](https://github.com/netdata/netdata/pull/9492) ([joelhans](https://github.com/joelhans))
- Fix vulnerability in JSON parsing [\#9491](https://github.com/netdata/netdata/pull/9491) ([underhood](https://github.com/underhood))
- Fix potential memory leak in ebpf.plugin [\#9484](https://github.com/netdata/netdata/pull/9484) ([thiagoftsm](https://github.com/thiagoftsm))
- Add guide for monitoring a k8s cluster with Netdata [\#9466](https://github.com/netdata/netdata/pull/9466) ([joelhans](https://github.com/joelhans))
- Add codeowners to exporting engine folder [\#9465](https://github.com/netdata/netdata/pull/9465) ([thiagoftsm](https://github.com/thiagoftsm))
- Update exporting engine to read the prefix option from instance config sections [\#9463](https://github.com/netdata/netdata/pull/9463) ([vlvkobal](https://github.com/vlvkobal))
- Fix a Coverity defect for resource leaks [\#9462](https://github.com/netdata/netdata/pull/9462) ([vlvkobal](https://github.com/vlvkobal))
- Fix the exporting engine unit tests [\#9460](https://github.com/netdata/netdata/pull/9460) ([vlvkobal](https://github.com/vlvkobal))
- Wrap exporting engine header definitions in compilation conditions [\#9458](https://github.com/netdata/netdata/pull/9458) ([candrews](https://github.com/candrews))
- Properly include eBPF collector in binary packages. [\#9450](https://github.com/netdata/netdata/pull/9450) ([Ferroin](https://github.com/Ferroin))
- Fix display error in Swagger API documentation [\#9417](https://github.com/netdata/netdata/pull/9417) ([underhood](https://github.com/underhood))
- Added missing caps letters [\#9379](https://github.com/netdata/netdata/pull/9379) ([Jiab77](https://github.com/Jiab77))
- Fixed typo in the streaming readme [\#9378](https://github.com/netdata/netdata/pull/9378) ([Jiab77](https://github.com/Jiab77))
- Add support for multiple ACLK query processing threads [\#9355](https://github.com/netdata/netdata/pull/9355) ([underhood](https://github.com/underhood))
- Change the HTTP method to make the IPFS collector compatible with 0.5.0+ [\#9248](https://github.com/netdata/netdata/pull/9248) ([RubenKelevra](https://github.com/RubenKelevra))

## [v1.23.1](https://github.com/netdata/netdata/tree/v1.23.1) (2020-07-01)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.23.0...v1.23.1)

**Merged pull requests:**

-  Fix the unittest execution [\#9445](https://github.com/netdata/netdata/pull/9445) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix children version on stream [\#9438](https://github.com/netdata/netdata/pull/9438) ([thiagoftsm](https://github.com/thiagoftsm))
- Disallow dimensions and chart being obsolete and archived simultaneously. [\#9436](https://github.com/netdata/netdata/pull/9436) ([mfundul](https://github.com/mfundul))
- Fix internal registry  [\#9434](https://github.com/netdata/netdata/pull/9434) ([thiagoftsm](https://github.com/thiagoftsm))
- Fixed duplicate alarm ids in health-log.db [\#9428](https://github.com/netdata/netdata/pull/9428) ([stelfrag](https://github.com/stelfrag))
- Correct virtualization detection in system-info.sh [\#9425](https://github.com/netdata/netdata/pull/9425) ([Ferroin](https://github.com/Ferroin))
- Stop reading from /proc/sys/kernel/osrelease at trailing newline [\#9374](https://github.com/netdata/netdata/pull/9374) ([sjuxax](https://github.com/sjuxax))
- Show cgroups/containers ran by Kubelet without access to Kubernetes cluster information [\#9321](https://github.com/netdata/netdata/pull/9321) ([cakrit](https://github.com/cakrit))

## [v1.23.0](https://github.com/netdata/netdata/tree/v1.23.0) (2020-06-25)

[Full Changelog](https://github.com/netdata/netdata/compare/v1.22.1...v1.23.0)

**Merged pull requests:**

- Fix Coverity Defect CID 304732 [\#9402](https://github.com/netdata/netdata/pull/9402) ([amoss](https://github.com/amoss))
- update synology.md [\#9400](https://github.com/netdata/netdata/pull/9400) ([pkrasam](https://github.com/pkrasam))
- Added OpenSSL to list of dependencies for Netdata Cloud. [\#9398](https://github.com/netdata/netdata/pull/9398) ([Ferroin](https://github.com/Ferroin))
- Fix missing host variables on stream [\#9396](https://github.com/netdata/netdata/pull/9396) ([thiagoftsm](https://github.com/thiagoftsm))
- Fix a bug in the simple exporting connector [\#9389](https://github.com/netdata/netdata/pull/9389) ([vlvkobal](https://github.com/vlvkobal))
- Fixes the race-hazard in streaming during the shutdown sequence [\#9370](https://github.com/netdata/netdata/pull/9370) ([amoss](https://github.com/amoss))
- Fixed ACLK shutdown sequence [\#9367](https://github.com/netdata/netdata/pull/9367) ([underhood](https://github.com/underhood))
- Improved error handling and recovery during compaction and metadata log replay [\#9354](https://github.com/netdata/netdata/pull/9354) ([stelfrag](https://github.com/stelfrag))
- Add guide for troubleshooting with eBPF metrics [\#9352](https://github.com/netdata/netdata/pull/9352) ([joelhans](https://github.com/joelhans))
- dashboard v1.0.14\_2 [\#9350](https://github.com/netdata/netdata/pull/9350) ([jacekkolasa](https://github.com/jacekkolasa))
- Revert "Override linker and include paths for static builds." [\#9343](https://github.com/netdata/netdata/pull/9343) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.19.2 [\#9340](https://github.com/netdata/netdata/pull/9340) ([ilyam8](https://github.com/ilyam8))
- Get netdata execution path early to avoid user permission issues [\#9339](https://github.com/netdata/netdata/pull/9339) ([mfundul](https://github.com/mfundul))
- Fix issues on ebpf.plugin [\#9333](https://github.com/netdata/netdata/pull/9333) ([thiagoftsm](https://github.com/thiagoftsm))
- Remove Gentoo from CI [\#9327](https://github.com/netdata/netdata/pull/9327) ([prologic](https://github.com/prologic))
- Fixed invalid memory access [\#9326](https://github.com/netdata/netdata/pull/9326) ([stelfrag](https://github.com/stelfrag))
- Add support for persistent metadata [\#9324](https://github.com/netdata/netdata/pull/9324) ([stelfrag](https://github.com/stelfrag))
- Change streaming terminology to parent-child in the code [\#9323](https://github.com/netdata/netdata/pull/9323) ([amoss](https://github.com/amoss))
- Fix check for remote write header in unit tests [\#9318](https://github.com/netdata/netdata/pull/9318) ([vlvkobal](https://github.com/vlvkobal))
- Adds missing files for streaming changes into cmake build [\#9316](https://github.com/netdata/netdata/pull/9316) ([underhood](https://github.com/underhood))
- apps\_groups.conf: add agent-service-discovery [\#9315](https://github.com/netdata/netdata/pull/9315) ([ilyam8](https://github.com/ilyam8))
- Change streaming terminology to parent/child in docs [\#9312](https://github.com/netdata/netdata/pull/9312) ([joelhans](https://github.com/joelhans))
- Added dummy `--enable-ebpf` flag to avoid breaking updates. [\#9310](https://github.com/netdata/netdata/pull/9310) ([Ferroin](https://github.com/Ferroin))
- installer: update go.d.plugin version to v0.19.1 [\#9309](https://github.com/netdata/netdata/pull/9309) ([ilyam8](https://github.com/ilyam8))
- Correct the repo in the docs for CentOS 8. [\#9308](https://github.com/netdata/netdata/pull/9308) ([Ferroin](https://github.com/Ferroin))
- Fix consistency of kubernetes cgroup names [\#9303](https://github.com/netdata/netdata/pull/9303) ([cakrit](https://github.com/cakrit))
- Fix remote write HTTP header [\#9302](https://github.com/netdata/netdata/pull/9302) ([vlvkobal](https://github.com/vlvkobal))
- minor copy edits [\#9298](https://github.com/netdata/netdata/pull/9298) ([MeganBishopMoore](https://github.com/MeganBishopMoore))
- Fix crash in \#9291 [\#9297](https://github.com/netdata/netdata/pull/9297) ([amoss](https://github.com/amoss))
- Add frontmatter to Matrix notifications doc [\#9295](https://github.com/netdata/netdata/pull/9295) ([joelhans](https://github.com/joelhans))
- installer: update go.d.plugin version to v0.19.0 [\#9294](https://github.com/netdata/netdata/pull/9294) ([ilyam8](https://github.com/ilyam8))
- dashboard\_info.js: ebpf: fix close code block [\#9293](https://github.com/netdata/netdata/pull/9293) ([ilyam8](https://github.com/ilyam8))
- Add guide to exporting metrics to Graphite [\#9285](https://github.com/netdata/netdata/pull/9285) ([joelhans](https://github.com/joelhans))
- update\_apps\_groups: Bring imunify and lsphp to apps groups [\#9284](https://github.com/netdata/netdata/pull/9284) ([thiagoftsm](https://github.com/thiagoftsm))
- Adds metrics for ACLK performance and status [\#9269](https://github.com/netdata/netdata/pull/9269) ([underhood](https://github.com/underhood))
- Fix Coverity defects 359164, 359165 and 358989. [\#9268](https://github.com/netdata/netdata/pull/9268) ([amoss](https://github.com/amoss))
- Move/refactor docs to accomodate new Guides section on Learn [\#9266](https://github.com/netdata/netdata/pull/9266) ([joelhans](https://github.com/joelhans))
- Cleanup of main README and registry doc [\#9265](https://github.com/netdata/netdata/pull/9265) ([joelhans](https://github.com/joelhans))
- Fixed handling of OpenSSL on CentOS/RHEL by bundling a static copy and selecting a configuration directory at install time. [\#9263](https://github.com/netdata/netdata/pull/9263) ([Ferroin](https://github.com/Ferroin))
- Fix frontmatter in circular\_buffer README [\#9262](https://github.com/netdata/netdata/pull/9262) ([joelhans](https://github.com/joelhans))
- Doc: Add missing slash [\#9257](https://github.com/netdata/netdata/pull/9257) ([oneoneonepig](https://github.com/oneoneonepig))
- Fixes documentation ambiguity leading into issue \#8239 [\#9255](https://github.com/netdata/netdata/pull/9255) ([underhood](https://github.com/underhood))
- Add new exporting "home base" document [\#9246](https://github.com/netdata/netdata/pull/9246) ([joelhans](https://github.com/joelhans))
- Add a random offset to the update script when running non-interactively. [\#9245](https://github.com/netdata/netdata/pull/9245) ([Ferroin](https://github.com/Ferroin))

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
