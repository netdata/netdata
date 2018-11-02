#!/bin/bash

cd htmldoc/src
YML=../mkdocs.yml

# create yaml nav subtree with all the files directly under a specific directory
# arguments:
# directory - to get mds from to add them to the yaml
# file - can be * to include all files
# tabs - how deep do we show it in the hierarchy. Level 1 is the top level, max should probably be 3
# name - what do we call the main link on the navbar
# maxdepth - how many levels of subdirectories do I include in the yaml in this section. 1 means just the top level
navpart () {
 dir=$1
 file=$2
 tabs=$3
 name=$4
 maxdepth=$5
 spc=""
 
 i=1
 while [ ${i} -lt ${tabs} ] ; do
	spc="    $spc"
	i=$[$i + 1]
 done
 
 echo "$spc- $name:"
 for f in `find $dir -maxdepth $maxdepth -name "${file}.md"`; do 
	echo "$spc    - '$f'"
 done
}
echo 'site_name: NetData Documentation
repo_url: https://github.com/netdata/netdata
repo_name: GitHub
edit_uri: blob/htmldoc
site_description: NetData Documentation
copyright: NetData, 2018
docs_dir: src
site_dir: build
#use_directory_urls: false
theme:
    name: "material"
    custom_dir: themes/material
markdown_extensions:
 - extra
 - abbr
 - attr_list
 - def_list
 - fenced_code
 - footnotes
 - tables
 - admonition
 - codehilite
 - meta
 - nl2br
 - sane_lists
 - smarty
 - toc:
    permalink: True
    separator: "-"
 - wikilinks
nav:'

navpart . README 1 "Getting Started" 1

echo "    - 'doc/Introducing-netdata.md'
    - 'doc/Why-netdata.md'
    - 'doc/Performance-monitoring,-scaled-properly.md'
    - 'doc/netdata-for-IoT.md'
    - 'doc/Demo-Sites.md'"

navpart installer * 1 "Installation" 1

echo"- Using NetData"
    - 'doc/Getting-Started.md'
    - 'doc/Command-Line-Options.md'
    - 'doc/Log-Files.md'
    - 'doc/Tracing-Options.md'
    - 'doc/Performance.md'
    - 'doc/Memory-Requirements.md'
    - 'doc/netdata-security.md'
    - 'doc/Netdata-Security-and-Disclosure-Information.md'
    - 'doc/netdata-OOMScore.md'
    - 'doc/netdata-process-priority.md'
- Configuration:
    - 'doc/Configuration.md'
- Optimization:
    - 'doc/high-performance-netdata.md'
    - 'doc/Memory-Deduplication---Kernel-Same-Page-Merging---KSM.md'
    - 'doc/netdata-virtual-memory-size.md'
- Database-Replication-and-Mirroring:
    - 'doc/Replication-Overview.md'
    - 'doc/Monitoring-ephemeral-nodes.md'
    - 'doc/netdata-proxies.md'
- Backends-:
    - 'doc/netdata-backends.md'
    - 'doc/Using-Netdata-with-Prometheus.md'
    - 'doc/Netdata,-Prometheus,-and-Grafana-Stack.md'
"
navpart netdata/health README 1 "Health Monitoring" 1
navpart netdata/health/notifications * 2 "Supported Notifications" 2

echo "- Netdata-Registry:
    - 'doc/mynetdata-menu-item.md'
- Netdata-Badges:
    - 'doc/Generating-Badges.md'
- Data-Collection:
    - 'doc/Add-more-charts-to-netdata.md'
    - 'doc/Collectors-README.md'
    - Internal Plugins:
        - 'doc/Internal-Plugins.md'
        - 'doc/statsd.md'
        - 'doc/monitoring-cgroups.md'
        - 'doc/QoS.md'
        - 'doc/monitoring-systemd-services.md'
        - 'doc/Monitoring-disks.md'
    - External Plugins:
        - 'doc/External-Plugins.md'
        - 'doc/The-spectacles-of-a-web-server-log-file.md'
        - 'doc/monitoring-IPMI.md'
        - Binary-Modules:
            - 'doc/Apps-Plugin.md'
            - 'doc/fping-Plugin.md'
        - Python-Modules:
            - 'doc/Netdata-Python-Modules.md'
            - 'doc/Monitoring-Go-Applications.md'
            - 'doc/Monitoring-Java-Spring-Boot-Applications.md'
        - Nodejs-Modules:
            - 'doc/General-Info-node.d.md'
        - BASH-Modules:
            - 'doc/General-Info-charts.d.md'
        - Active-BASH-Modules:
            - 'doc/ap.chart.sh.md'
            - 'doc/apcupsd.chart.sh.md'
            - 'doc/example.chart.sh.md'
            - 'doc/charts.d.md'
            - 'doc/opensips.chart.sh.md'
        - Obsolete-BASH-Modules:
            - 'doc/Obsolete-BASH-Modules.md'
            - 'doc/apache.chart.sh.md'
            - 'doc/cpufreq.chart.sh.md'
            - 'doc/mysql.chart.sh.md'
            - 'doc/phpfpm.chart.sh.md'
            - 'doc/tomcat.chart.sh.md'
    - Third Party Plugins:
        - 'doc/Third-Party-Plugins.md'
- API-Documentation:
    - 'doc/REST-API-v1.md'
    - 'doc/receiving-netdata-metrics-from-shell-scripts.md'
- Web-Dashboards:
    - 'doc/Overview.md'
    - 'doc/Custom-Dashboards.md'
    - 'doc/Custom-Dashboard-with-Confluence.md'
- Chart-Libraries:
    - 'doc/Dygraph.md'
    - 'doc/EasyPieChart.md'
    - 'doc/Gauge.js.md'
    - 'doc/jQuery-Sparkline.md'
    - 'doc/Peity.md'
- Running-behind-another-web-server:
    - 'doc/Running-behind-nginx.md'
    - 'doc/Running-behind-apache.md'
    - 'doc/Running-behind-lighttpd.md'
    - 'doc/Running-behind-caddy.md'
- Blog:
    - 'doc/Donations-netdata-has-received.md'
    - 'doc/a-github-star-is-important.md'
    - 'doc/Release-Notes.md'
    - 'doc/CPU-Usage.md'
    - 'doc/DDOS.md'
"
