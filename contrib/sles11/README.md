# spec to build netdata RPM for sles 11

based on [opensuse rpm spec](https://build.opensuse.org/package/show/network/netdata) with some 
changes and additions for sles 11 backport, namely:
- init.d script 
- run-time dependency on python ordereddict backport
- patch for netdata python.d plugin to work with older python
- crude hack of notification script to work with bash 3 (mails only)
