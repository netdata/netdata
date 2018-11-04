#!/bin/bash

cd htmldoc/src

# create yaml nav subtree with all the files directly under a specific directory
# arguments:
# tabs - how deep do we show it in the hierarchy. Level 1 is the top level, max should probably be 3
# directory - to get mds from to add them to the yaml
# file - can be left empty to include all files
# name - what do we call the relevant section on the navbar. Empty if no new section is required
# maxdepth - how many levels of subdirectories do I include in the yaml in this section. 1 means just the top level and is the default if left empty
# excludefirstlevel - Optional param. If passed, mindepth is set to 2, to exclude the READMEs in the first directory level

navpart () {
 tabs=$1
 dir=$2
 file=$3
 section=$4
 maxdepth=$5
 excludefirstlevel=$6
 spc=""
 
 i=1
 while [ ${i} -lt ${tabs} ] ; do
	spc="    $spc"
	i=$[$i + 1]
 done
 
 if [ -z "$file" ] ; then file='*' ; fi
 if [[ ! -z "$section" ]] ; then echo "$spc- ${section}:" ; fi
 if [ -z "$maxdepth" ] ; then maxdepth=1; fi
 if [[ ! -z "$excludefirstlevel" ]] ; then mindepth=2 ; else mindepth=1; fi
 
 for f in `find $dir -mindepth $mindepth -maxdepth $maxdepth -name "${file}.md" -printf '%h\0%d\0%p\n' | sort -t '\0' -n | awk -F '\0' '{print $3}'`; do 
	# If I'm adding a section, I need the child links to be one level deeper than the requested level in "tabs"
	if [ -z "$section" ] ; then 
		echo "$spc- '$f'"
	else
		echo "$spc    - '$f'"
	fi
 done
}


echo -e 'site_name: NetData Documentation
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

navpart 1 . README "Getting Started"

echo -ne "    - 'doc/Why-netdata.md'
    - 'doc/Demo-Sites.md'
    - Installation:
        - 'installer/README.md'
        - 'installer/DOCKER.md'
        - 'installer/LAUNCH.md'
        - 'installer/UNINSTALL.md'
        - 'installer/UPDATE.md'
        - 'installer/MAINTAINERS.md'
    "

# Configuration
navpart 2 system

navpart 2 registry
navpart 2 streaming "" "" 4
navpart 2 backends "" "Backends" 3

echo -ne "- Using NetData:
    - 'doc/Getting-Started.md'
    - 'doc/Command-Line-Options.md'
    - 'doc/Log-Files.md'
    - 'doc/Tracing-Options.md'
    - 'doc/Performance.md'
    - 'doc/netdata-for-IoT.md'
    - 'doc/Memory-Requirements.md'
    - 'doc/netdata-security.md'
    - 'doc/Netdata-Security-and-Disclosure-Information.md'
    - 'doc/netdata-OOMScore.md'
    - 'doc/netdata-process-priority.md'
    - 'doc/high-performance-netdata.md'
    - 'doc/Memory-Deduplication---Kernel-Same-Page-Merging---KSM.md'
    - 'doc/netdata-virtual-memory-size.md'
"

navpart 1 health README "Health Monitoring"
navpart 2 health/notifications "" "" 1
navpart 2 health/notifications "" "Supported Notifications" 2 excludefirstlevel
navpart 2 doc "health-API-calls"
navpart 2 doc "troubleshooting-alarms"

navpart 1 collectors "" "Data Collection" 1
echo -ne "    - 'doc/Add-more-charts-to-netdata.md'
    - Internal Plugins:
"
navpart 3 collectors/proc.plugin
navpart 3 collectors/statsd.plugin
navpart 3 collectors/cgroups.plugin
navpart 3 collectors/idlejitter.plugin
navpart 3 collectors/tc.plugin
navpart 3 collectors/nfacct.plugin
navpart 3 collectors/checks.plugin
navpart 3 collectors/diskspace.plugin
navpart 3 collectors/freebsd.plugin
navpart 3 collectors/macos.plugin

navpart 2 collectors/plugins.d "" "External Plugins"
navpart 3 collectors/python.d.plugin "" "Python Plugins" 3
navpart 3 collectors/node.d.plugin "" "Node.js Plugins" 3
navpart 3 collectors/charts.d.plugin "" "BASH Plugins" 3
navpart 3 collectors/apps.plugin
navpart 3 collectors/fping.plugin
navpart 3 collectors/freeipmi.plugin

echo -ne "    - Third Party Plugins:
        - 'doc/Third-Party-Plugins.md'
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
"
navpart 2 web "" "Web"
navpart 3 web/gui "" "Web GUI" 4
navpart 3 web/server "" "Web Server" 4
navpart 3 web/api "" "Web API" 4

echo -ne "- Running-behind-another-web-server:
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
echo -ne "- Hacking netdata:
"
navpart 2 makeself "" "" 4
navpart 2 packaging "" "" 4
navpart 2 libnetdata "" "libnetdata" 4
