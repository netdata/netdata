#!/bin/bash

# Default configuration options
DEFAULT_BUILD_CLEAN_NETDATA=0
DEFAULT_BUILD_FOR_RELEASE=1
DEFAULT_NUM_LOG_SOURCES=0
DEFAULT_DELAY_BETWEEN_MSG_WRITE=1000000
DEFAULT_TOTAL_MSGS_PER_SOURCE=1000000
DEFAULT_QUERIES_DELAY=3600
DEFAULT_LOG_ROTATE_AFTER_SEC=3600
DEFAULT_DELAY_OPEN_TO_WRITE_SEC=6
DEFAULT_RUN_LOGS_MANAGEMENT_TESTS_ONLY=0

if [ "$1" == "-h" ] || [ "$1" == "--help" ]; then
  echo "Usage: $(basename "$0") [ARGS]..."
  echo "Example: $(basename "$0") 0 1 2 1000 1000000 10 6 6 0"
  echo "Build, install and run netdata with logs management "
  echo "functionality enabled and (optional) stress tests."
  echo ""
  echo "arg[1]: [build_clean_netdata]     		 Default: $DEFAULT_BUILD_CLEAN_NETDATA"
  echo "arg[2]: [build_for_release]      		 Default: $DEFAULT_BUILD_FOR_RELEASE"
  echo "arg[3]: [num_log_sources]         		 Default: $DEFAULT_NUM_LOG_SOURCES"
  echo "arg[4]: [delay_between_msg_write] 		 Default: $DEFAULT_DELAY_BETWEEN_MSG_WRITE us"
  echo "arg[5]: [total_msgs_per_source]   		 Default: $DEFAULT_TOTAL_MSGS_PER_SOURCE"
  echo "arg[6]: [queries_delay]           		 Default: $DEFAULT_QUERIES_DELAY s"
  echo "arg[7]: [log_rotate_after_sec]    		 Default: $DEFAULT_LOG_ROTATE_AFTER_SEC s"
  echo "arg[8]: [delay_open_to_write_sec]		 Default: $DEFAULT_DELAY_OPEN_TO_WRITE_SEC s"
  echo "arg[9]: [run_logs_management_tests_only]	 Default: $DEFAULT_RUN_LOGS_MANAGEMENT_TESTS_ONLY"
  exit 0
fi

build_clean_netdata="${1:-$DEFAULT_BUILD_CLEAN_NETDATA}"
build_for_release="${2:-$DEFAULT_BUILD_FOR_RELEASE}"
num_log_sources="${3:-$DEFAULT_NUM_LOG_SOURCES}"
delay_between_msg_write="${4:-$DEFAULT_DELAY_BETWEEN_MSG_WRITE}"
total_msgs_per_source="${5:-$DEFAULT_TOTAL_MSGS_PER_SOURCE}"
queries_delay="${6:-$DEFAULT_QUERIES_DELAY}"
log_rotate_after_sec="${7:-$DEFAULT_LOG_ROTATE_AFTER_SEC}"
delay_open_to_write_sec="${8:-$DEFAULT_DELAY_OPEN_TO_WRITE_SEC}"
run_logs_management_tests_only="${9:-$DEFAULT_RUN_LOGS_MANAGEMENT_TESTS_ONLY}"

if [ "$num_log_sources" -le 0 ]
then
	enable_stress_tests=0
else
	enable_stress_tests=1
fi

INSTALL_PATH="/tmp"

# Terminate running processes
sudo killall -s KILL netdata
sudo killall -s KILL stress_test
sudo killall -s KILL -u netdata

# Remove potentially persistent directories and files
sudo rm -f $INSTALL_PATH/netdata/var/log/netdata/error.log
sudo rm -rf $INSTALL_PATH/netdata/var/cache/netdata/logs_management_db 
sudo rm -rf $INSTALL_PATH/netdata_log_management_stress_test_data

CPU_CORES=$(grep ^cpu\\scores /proc/cpuinfo | uniq |  awk '{print $4}')

# Build or rebuild Netdata
if [ "$build_clean_netdata" -eq 1 ]
then
	cd ../..
	sudo $INSTALL_PATH/netdata/usr/libexec/netdata/netdata-uninstaller.sh -y -f -e $INSTALL_PATH/netdata/etc/netdata/.environment
	sudo rm -rf $INSTALL_PATH/netdata/etc/netdata # Remove /etc/netdata if it persists for some reason
	sudo git clean -dxff && git submodule update --init --recursive --force 

	if [ "$build_for_release" -eq 0 ]
	then
		c_flags="-O1 -ggdb -Wall -Wextra "
		c_flags+="-fno-omit-frame-pointer -Wformat-signedness -fstack-protector-all -Wformat-truncation=2 -Wunused-result "
		c_flags+="-DNETDATA_INTERNAL_CHECKS=1 -DNETDATA_DEV_MODE=1 -DLOGS_MANAGEMENT_STRESS_TEST=$enable_stress_tests "
		# c_flags+="-Wl,--no-as-needed -ldl "
		sudo CFLAGS="$c_flags" ./netdata-installer.sh \
					--dont-start-it \
					--dont-wait \
					--disable-lto \
					--disable-telemetry \
					--disable-go \
					--disable-ebpf \
					--disable-ml \
					--enable-logsmanagement-tests \
					--install-prefix $INSTALL_PATH
	else
		c_flags="-DLOGS_MANAGEMENT_STRESS_TEST=$enable_stress_tests "
		# c_flags+="-Wl,--no-as-needed -ldl "
		sudo CFLAGS="$c_flags" ./netdata-installer.sh \
					--dont-start-it \
					--dont-wait \
					--disable-telemetry \
					--install-prefix $INSTALL_PATH
	fi

	sudo cp logsmanagement/stress_test/logs_query.html "$INSTALL_PATH/netdata/usr/share/netdata/web"
	sudo chown -R netdata:netdata "$INSTALL_PATH/netdata/usr/share/netdata/web/logs_query.html"

else
	cd ../.. && sudo make -j"$CPU_CORES" || exit 1 && sudo make install 
	sudo chown -R netdata:netdata "$INSTALL_PATH/netdata/usr/share/netdata/web"
fi

cd logsmanagement/stress_test || exit

if [ "$run_logs_management_tests_only" -eq 0 ]
then
	# Rebuild and run stress test
	if [ "$num_log_sources" -gt 0 ]
	then
		sudo -u netdata -g netdata mkdir $INSTALL_PATH/netdata_log_management_stress_test_data 
		gcc stress_test.c -DNUM_LOG_SOURCES="$num_log_sources" \
						-DDELAY_BETWEEN_MSG_WRITE="$delay_between_msg_write" \
						-DTOTAL_MSGS_PER_SOURCE="$total_msgs_per_source" \
						-DQUERIES_DELAY="$queries_delay" \
						-DLOG_ROTATE_AFTER_SEC="$log_rotate_after_sec" \
						-DDELAY_OPEN_TO_WRITE_SEC="$delay_open_to_write_sec" \
						-luv -Og -g -o stress_test 
		sudo -u netdata -g netdata ./stress_test &
		sleep 1
	fi

	# Run Netdata
	if [ "$build_for_release" -eq 0 ]
	then
		sudo -u netdata -g netdata -s gdb -ex="set confirm off" -ex=run --args $INSTALL_PATH/netdata/usr/sbin/netdata -D
	elif [ "$build_for_release" -eq 2 ]
	then
		sudo -u netdata -g netdata -s gdb -ex="set confirm off" -ex=run --args $INSTALL_PATH/netdata/usr/libexec/netdata/plugins.d/logs-management.plugin
	else
		sudo -u netdata -g netdata ASAN_OPTIONS=log_path=stdout $INSTALL_PATH/netdata/usr/sbin/netdata -D
	fi
else
	if [[ $($INSTALL_PATH/netdata/usr/sbin/netdata -W buildinfo | grep -Fc DLOGS_MANAGEMENT_STRESS_TEST) -eq 1 ]]
	then
		sudo -u netdata -g netdata ASAN_OPTIONS=log_path=/dev/null $INSTALL_PATH/netdata/usr/libexec/netdata/plugins.d/logs-management.plugin --unittest
	else
		echo "======================================================================="
		echo "run_logs_management_tests_only=1 but logs management tests cannot run."
		echo "Netdata must be configured with --enable-logsmanagement-tests."
		echo "Please rerun script with build_clean_netdata=1 and build_for_release=0."
		echo "======================================================================="
	fi
fi
