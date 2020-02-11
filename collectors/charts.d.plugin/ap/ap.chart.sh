# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
ap_update_every=
ap_priority=6900

declare -A ap_devs=()

# _check is called once, to find out if this chart should be enabled or not
ap_check() {
  require_cmd iw || return 1
  local ev
  ev=$(run iw dev | awk '
		BEGIN {
			i = "";
			ssid = "";
			ap = 0;
		}
		/^[ \t]+Interface / {
			if( ap == 1 ) {
				print "ap_devs[" i "]=\"" ssid "\""
			}

			i = $2;
			ssid = "";
			ap = 0;
		}
		/^[ \t]+ssid / { ssid = $2; }
		/^[ \t]+type AP$/ { ap = 1; }
		END {
			if( ap == 1 ) {
				print "ap_devs[" i "]=\"" ssid "\""
			}
		}
	')
  eval "${ev}"

  # this should return:
  #  - 0 to enable the chart
  #  - 1 to disable the chart

  [ ${#ap_devs[@]} -gt 0 ] && return 0
  error "no devices found in AP mode, with 'iw dev'"
  return 1
}

# _create is called once, to create the charts
ap_create() {
  local ssid dev

  for dev in "${!ap_devs[@]}"; do
    ssid="${ap_devs[${dev}]}"

    # create the chart with 3 dimensions
    cat << EOF
CHART ap_clients.${dev} '' "Connected clients to ${ssid} on ${dev}" "clients" ${dev} ap.clients line $((ap_priority + 1)) $ap_update_every
DIMENSION clients '' absolute 1 1

CHART ap_bandwidth.${dev} '' "Bandwidth for ${ssid} on ${dev}" "kilobits/s" ${dev} ap.net area $((ap_priority + 2)) $ap_update_every
DIMENSION received '' incremental 8 1024
DIMENSION sent '' incremental -8 1024

CHART ap_packets.${dev} '' "Packets for ${ssid} on ${dev}" "packets/s" ${dev} ap.packets line $((ap_priority + 3)) $ap_update_every
DIMENSION received '' incremental 1 1
DIMENSION sent '' incremental -1 1

CHART ap_issues.${dev} '' "Transmit Issues for ${ssid} on ${dev}" "issues/s" ${dev} ap.issues line $((ap_priority + 4)) $ap_update_every
DIMENSION retries 'tx retries' incremental 1 1
DIMENSION failures 'tx failures' incremental -1 1

CHART ap_signal.${dev} '' "Average Signal for ${ssid} on ${dev}" "dBm" ${dev} ap.signal line $((ap_priority + 5)) $ap_update_every
DIMENSION signal 'average signal' absolute 1 1000

CHART ap_bitrate.${dev} '' "Bitrate for ${ssid} on ${dev}" "Mbps" ${dev} ap.bitrate line $((ap_priority + 6)) $ap_update_every
DIMENSION receive '' absolute 1 1000
DIMENSION transmit '' absolute -1 1000
DIMENSION expected 'expected throughput' absolute 1 1000
EOF
  done

  return 0
}

# _update is called continuously, to collect the values
ap_update() {
  # the first argument to this function is the microseconds since last update
  # pass this parameter to the BEGIN statement (see bellow).

  # do all the work to collect / calculate the values
  # for each dimension
  # remember: KEEP IT SIMPLE AND SHORT

  for dev in "${!ap_devs[@]}"; do
    echo
    echo "DEVICE ${dev}"
    iw "${dev}" station dump
  done | awk '
        function zero_data() {
            dev = "";
            c = 0;
            rb = 0;
            tb = 0;
            rp = 0;
            tp = 0;
            tr = 0;
            tf = 0;
            tt = 0;
            rt = 0;
            s = 0;
            g = 0;
            e = 0;
        }
        function print_device() {
            if(dev != "" && length(dev) > 0) {
                print "BEGIN ap_clients." dev;
                print "SET clients = " c;
                print "END";
                print "BEGIN ap_bandwidth." dev;
                print "SET received = " rb;
                print "SET sent = " tb;
                print "END";
                print "BEGIN ap_packets." dev;
                print "SET received = " rp;
                print "SET sent = " tp;
                print "END";
                print "BEGIN ap_issues." dev;
                print "SET retries = " tr;
                print "SET failures = " tf;
                print "END";

                if( c == 0 ) c = 1;
                print "BEGIN ap_signal." dev;
                print "SET signal = " int(s / c);
                print "END";
                print "BEGIN ap_bitrate." dev;
                print "SET receive = " int(rt / c);
                print "SET transmit = " int(tt / c);
                print "SET expected = " int(e / c);
                print "END";
            }
            zero_data();
        }
        BEGIN {
            zero_data();
        }
        /^DEVICE / {
            print_device();
            dev = $2;
        }
        /^Station/            { c++; }
        /^[ \t]+rx bytes:/   { rb += $3; }
        /^[ \t]+tx bytes:/   { tb += $3; }
        /^[ \t]+rx packets:/ { rp += $3; }
        /^[ \t]+tx packets:/ { tp += $3; }
        /^[ \t]+tx retries:/ { tr += $3; }
        /^[ \t]+tx failed:/  { tf += $3; }
        /^[ \t]+signal:/     { x = $2; s  += x * 1000; }
        /^[ \t]+rx bitrate:/ { x = $3; rt += x * 1000; }
        /^[ \t]+tx bitrate:/ { x = $3; tt += x * 1000; }
        /^[ \t]+expected throughput:(.*)Mbps/ {
            x=$3;
            sub(/Mbps/, "", x);
            e += x * 1000;
        }
        END {
            print_device();
        }
    '

  return 0
}
