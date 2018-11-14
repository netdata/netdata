#!/bin/bash

# Doc link checker
# Validates and tries to fix all links that will cause issues either in the repo, or in the html site

replace_wikilink () {
	f=$1
	lnk=$2
	
	# The list for this case statement was created by executing in nedata.wiki repo the following:
	# grep 'Moved to ' * | sed 's/\.md.*http/ http/g' | sed 's/).*//g' | awk '{printf("*%s* ) newlnk=\"%s\" ;;\n",$1,$2);}' 
	case "$lnk" in
		*Add-more-charts-to-netdata* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Add-more-charts-to-netdata.md#add-more-charts-to-netdata" ;;
		*alarm-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications" ;;
		*Alerta-monitoring-system* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/alerta" ;;
		*Amazon-SNS-Notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/awssns" ;;
		*apache.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/apache" ;;
		*ap.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/ap" ;;
		*Apps-Plugin* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/apps.plugin" ;;
		*Article:-Introducing-netdata* ) newlnk="https://github.com/netdata/netdata/tree/master/README.md" ;;
		*Chart-Libraries* ) newlnk="https://github.com/netdata/netdata/tree/master/web/gui/#netdata-agent-web-gui" ;;
		*Command-Line-Options* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon#command-line-options" ;;
		*Configuration* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon/config#configuration-guide" ;;
		*cpufreq.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/cpufreq" ;;
		*Custom-Dashboards* ) newlnk="https://github.com/netdata/netdata/tree/master/web/gui/custom#custom-dashboards" ;;
		*Custom-Dashboard-with-Confluence* ) newlnk="https://github.com/netdata/netdata/tree/master/web/gui/confluence#atlassian-confluence-dashboards" ;;
		*discord-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/discord" ;;
		*Donations-netdata-has-received* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Donations-netdata-has-received.md#donations-received" ;;
		*Dygraph* ) newlnk="https://github.com/netdata/netdata/blob/master/web/gui/README.md#Dygraph" ;;
		*EasyPieChart* ) newlnk="https://github.com/netdata/netdata/blob/master/web/gui/README.md#EasyPieChart" ;;
		*email-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/email" ;;
		*example.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/example" ;;
		*External-Plugins* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/plugins.d" ;;
		*flock-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/flock" ;;
		*fping-Plugin* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/fping.plugin" ;;
		*General-Info---charts.d* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin" ;;
		*General-Info---node.d* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/node.d.plugin" ;;
		*Generating-Badges* ) newlnk="https://github.com/netdata/netdata/tree/master/web/api/badges" ;;
		*health-API-calls* ) newlnk="https://github.com/netdata/netdata/tree/master/web/api/health#health-api-calls" ;;
		*health-configuration-examples* ) newlnk="https://github.com/netdata/netdata/tree/master/health#examples" ;;
		*health-configuration-reference* ) newlnk="https://github.com/netdata/netdata/tree/master/health" ;;
		*health-monitoring* ) newlnk="https://github.com/netdata/netdata/tree/master/health" ;;
		*high-performance-netdata* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/high-performance-netdata.md#high-performance-netdata" ;;
		*Installation* ) newlnk="https://github.com/netdata/netdata/tree/master/installer#installation" ;;
		*Install-netdata-with-Docker* ) newlnk="https://github.com/netdata/netdata/tree/master/docker#install-netdata-with-docker" ;;
		*Internal-Plugins* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors" ;;
		*IRC-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/irc" ;;
		*kavenegar-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/kavenegar" ;;
		*Linux-console-tools,-fail-to-report-per-process-CPU-usage-properly* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/apps.plugin#comparison-with-console-tools" ;;
		*Log-Files* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon#log-files" ;;
		*Memory-Deduplication---Kernel-Same-Page-Merging---KSM* ) newlnk="https://github.com/netdata/netdata/tree/master/database#ksm" ;;
		*Memory-Requirements* ) newlnk="https://github.com/netdata/netdata/tree/master/database" ;;
		*messagebird-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/messagebird" ;;
		*Monitor-application-bandwidth-with-Linux-QoS* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/tc.plugin" ;;
		*monitoring-cgroups* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/cgroups.plugin" ;;
		*Monitoring-disks* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/proc.plugin#monitoring-disks" ;;
		*monitoring-ephemeral-containers* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/cgroups.plugin#monitoring-ephemeral-containers" ;;
		*Monitoring-ephemeral-nodes* ) newlnk="https://github.com/netdata/netdata/tree/master/streaming#monitoring-ephemeral-nodes" ;;
		*Monitoring-Go-Applications* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin/go_expvar" ;;
		*monitoring-IPMI* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/freeipmi.plugin" ;;
		*Monitoring-Java-Spring-Boot-Applications* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin/springboot#monitoring-java-spring-boot-applications" ;;
		*Monitoring-SYNPROXY* ) newlnk="https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md#linux-anti-ddos" ;;
		*monitoring-systemd-services* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/cgroups.plugin#monitoring-systemd-services" ;;
		*mynetdata-menu-item* ) newlnk="https://github.com/netdata/netdata/tree/master/registry" ;;
		*mysql.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/mysql" ;;
		*named.node.js* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/node.d.plugin/named" ;;
		*netdata-backends* ) newlnk="https://github.com/netdata/netdata/tree/master/backends" ;;
		*netdata-for-IoT* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/netdata-for-IoT.md#netdata-for-iot" ;;
		*netdata-OOMScore* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon#oom-score" ;;
		*netdata-package-maintainers* ) newlnk="https://github.com/netdata/netdata/tree/master/packaging/maintainers#package-maintainers" ;;
		*netdata-process-priority* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon#netdata-process-scheduling-policy" ;;
		*Netdata,-Prometheus,-and-Grafana-Stack* ) newlnk="https://github.com/netdata/netdata/blob/master/backends/WALKTHROUGH.md#netdata-prometheus-grafana-stack" ;;
		*netdata-proxies* ) newlnk="https://github.com/netdata/netdata/tree/master/streaming#proxies" ;;
		*netdata-security* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/netdata-security.md#netdata-security" ;;
		*netdata-virtual-memory-size* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" ;;
		*Overview* ) newlnk="https://github.com/netdata/netdata/tree/master/web#web-dashboards-overview" ;;
		*pagerduty-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/pagerduty" ;;
		*Performance* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Performance.md#netdata-performance" ;;
		*phpfpm.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/phpfpm" ;;
		*pushbullet-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/pushbullet" ;;
		*pushover-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/pushover" ;;
		*receiving-netdata-metrics-from-shell-scripts* ) newlnk="https://github.com/netdata/netdata/tree/master/web/api#using-the-api-from-shell-scripts" ;;
		*Replication-Overview* ) newlnk="https://github.com/netdata/netdata/tree/master/streaming" ;;
		*REST-API-v1* ) newlnk="https://github.com/netdata/netdata/tree/master/web/api" ;;
		*RocketChat-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/rocketchat" ;;
		*Running-behind-apache* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Running-behind-apache.md#netdata-via-apaches-mod_proxy" ;;
		*Running-behind-caddy* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Running-behind-caddy.md#netdata-via-caddy" ;;
		*Running-behind-lighttpd* ) newlnk="httpd-v14x" ;;
		*Running-behind-nginx* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Running-behind-nginx.md#netdata-via-nginx" ;;
		*slack-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/slack" ;;
		*sma_webbox.node.js* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/node.d.plugin/sma_webbox" ;;
		*snmp.node.js* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/node.d.plugin/snmp" ;;
		*statsd* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/statsd.plugin" ;;
		*Syslog-Notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/syslog" ;;
		*telegram-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/telegram" ;;
		*The-spectacles-of-a-web-server-log-file* ) newlnk="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log/README.md#web_log" ;;
		*Third-Party-Plugins* ) newlnk="https://github.com/netdata/netdata/blob/master/doc/Third-Party-Plugins.md#third-party-plugins" ;;
		*tomcat.chart.sh* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/charts.d.plugin/tomcat" ;;
		*Tracing-Options* ) newlnk="https://github.com/netdata/netdata/tree/master/daemon#debugging" ;;
		*troubleshooting-alarms* ) newlnk="https://github.com/netdata/netdata/tree/master/health#troubleshooting" ;;
		*twilio-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/twilio" ;;
		*Using-Netdata-with-Prometheus* ) newlnk="https://github.com/netdata/netdata/tree/master/backends/prometheus" ;;
		*web-browser-notifications* ) newlnk="https://github.com/netdata/netdata/tree/master/health/notifications/web" ;;
		*Why-netdata* ) newlnk="https://github.com/netdata/netdata/tree/master/doc/Why-Netdata.md" ;;
		*Writing-Plugins* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/plugins.d" ;;
		*You-should-install-QoS-on-all-your-servers* ) newlnk="https://github.com/netdata/netdata/tree/master/collectors/tc.plugin#tcplugin" ;;
		* ) echo " WARNING: Couldn't map $lnk to a known replacement" ;;
	esac
	srch=$(echo $lnk | sed 's/\//\\\//g' )
	rplc=$(echo $newlnk | sed 's/\//\\\//g' )
	printf " FIX: sed -i \'s/%s/%s/g\' %s\n" "$srch" "$rplc" $f
}

testlink () {
	if [ $TESTURLS -eq 0 ] ; then return ; fi
	if [ $VERBOSE -eq 1 ] ; then echo " Testing URL $1" ; fi
	curl -sS $1 > /dev/null
	if [ $? -gt 0 ] ; then
		echo " ERROR: $1 is a broken link"
	fi
}

testinternal () {
	# Check if the header referred to by the internal link exists in the same file
	f=${1}
	lnk=${2}
	header=$(echo $lnk | sed 's/#/# /g' | sed 's/-/\[ -\]/g')
	if [ $VERBOSE -eq 1 ] ; then echo " Searching for \"$header\" in $f"; fi
	grep -i "^\#*$header\$" $f >/dev/null
	if [ $? -eq 0 ] ; then
		if [ $VERBOSE -eq 1 ] ; then echo " $lnk found in $f"; fi
	else
		echo " ERROR: $lnk header not found in $f"
	fi

}

ck_netdata_relative () {
	f=${1}
	lnk=${2}
	if [ $VERBOSE -eq 1 ] ; then echo " Checking relative link $lnk"; fi
	if [ $ISREADME -eq 0 ] ; then
		case "$lnk" in
			\#* ) testinternal $f $lnk ;;
			* ) echo " ERROR: $lnk is a relative link in a Non-README file. Either convert the file to a README under a directory, or replace the relative link with an absolute one" ;;
		esac
	else
		case "$lnk" in
			\#* ) testinternal $f $lnk ;;
			*/*[^/]#* ) 
				echo " ERROR: $lnk - relative directory followed immediately by \# will break html." 
				newlnk=$(echo $lnk | sed 's/#/\/#/g')
				echo " FIX: Replace $lnk with $newlnk"
			;;
		esac
	fi
}

ck_netdata_absolute () {
	f=${1}
	lnk=${2}
	
	testlink $lnk
	case "$lnk" in
		*\/*.md* ) echo "ERROR: $lnk points to an md file, html will break" ;;
		* ) echo " WARNING: $lnk is an absolute link. Should replace it with relative" ;;
	esac
	if [[ $l =~ \(https://github.com/netdata/netdata/..../master/([^\(\) ]*)\) ]] ; then
		abspath="${BASH_REMATCH[1]}"
		echo " abspath: $abspath"
	fi
	# Convert it to a well-formed relative link ending, if necessary
	case "$abspath" in 
		*[^/]\#* ) abspath=$(echo $abspath | ses 's/\#/\/\#/g') ;;
		*[^/] ) abspath="${abspath}/";;
	esac
	# LOTS MORE TO DO HERE
}

checklinks () {
	f=$1
	echo "Processing $f"
	if [[ $f =~ .*README\.md ]] ; then
		ISREADME=1
		if [ $VERBOSE -eq 1 ] ; then echo "README file" ; fi
	else
		ISREADME=0
		if [ $VERBOSE -eq 1 ] ; then echo "WARNING: Not a README file. Will be converted to a directory with a README in htmldoc/src"; fi
	fi
	while read l ; do
		if [[ $l =~ .*\[[^\[]*\]\(([^\(\) ]*)\).* ]] ; then
			lnk="${BASH_REMATCH[1]}"
			if [ $VERBOSE -eq 1 ] ; then echo "$lnk"; fi
			case "$lnk" in
				https://github.com/netdata/netdata/wiki* ) 
					testlink $lnk
					echo " WARNING: $lnk points to the wiki" 
					replace_wikilink $f $lnk
				;;
				https://github.com/netdata/netdata/* ) ck_netdata_absolute $f $lnk;;
				http* ) testlink $lnk ;;
				* ) ck_netdata_relative $f $lnk ;;
			esac
		fi
	done < $f
}

REPLACE=0
TESTURLS=0
VERBOSE=0
while getopts :vtf:r: option
do
    case "$option" in
    f)
         file=$OPTARG
         ;;
	r)
		REPLACE=1
		;;
	t) 
		TESTURLS=1
		;;
	v)
		VERBOSE=1
		;;
	*)
		echo "Usage: htmldoc/checklinks.sh [-f <fname>] [-r]
	If no file is passed, recursively checks all files.
	-r option causes the link to be replaced with a proper link, where possible
	-t tests all absolute URLs
"
		;;
	esac
done

if [ -z ${file} ] ; then 
	for f in $(find . -type d \( -path ./htmldoc -o -path ./node_modules \) -prune -o -name "*.md" -print); do
		checklinks $f
	done
else
	checklinks $file
fi

