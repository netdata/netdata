## Real-time performance monitoring, just got better - v1.1.0

Live Demo: **http://netdata.firehol.org**

Full Specifications: https://github.com/firehol/netdata

Wiki: https://github.com/firehol/netdata/wiki

Download v1.1.0 release packages: https://github.com/firehol/netdata/releases/tag/v1.1.0

![main2](https://cloud.githubusercontent.com/assets/2662304/14703239/91270316-07b7-11e6-8c95-5614e75a474b.gif)

Dozens of commits that improve netdata in several ways:

### Data collection
 - added IPv6 monitoring
 - added SYNPROXY DDoS protection monitoring
 - apps.plugin: added charts for users and user groups
 - apps.plugin: grouping of processes now support patterns
 - apps.plugin: now it is faster, after the new features added
 - better auto-detection of partitions for disk monitoring
 - better fireqos intergation for QoS monitoring
 - squid monitoring now uses squidclient
 - SNMP monitoring now supports 64bit counters

### API
 - fixed issues in CSV output generation
 - netdata can now be restricted to listen on a specific IP (API and web server)

### Core
 - added error log flood protection

### Web Dashboard
 - better error handling when the netdata server is unreachable
 - each chart now has a toolbox
 - on-line help support
 - check for netdata updates button
 - added example /tv.html dashboard
 - now compiles with musl libc (alpine linux)

### Packaging
 - added debian packaging
 - support non-root installations
 - the installer generates uninstall script
