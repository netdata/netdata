# COMMON_FLAGS =

CONFIG_DIR = "conf.d"
LOG_DIR = "log"
PLUGINS_DIR = "plugins.d"

COMMON_FLAGS = -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'

ifdef final
CFLAGS = -Wall -Wextra -O3 $(COMMON_FLAGS)
else
CFLAGS = -Wall -Wextra -ggdb $(COMMON_FLAGS)
endif

CC = gcc $(CFLAGS)

proc_sources = proc_net_dev.c proc_net_ip_vs_stats.c proc_diskstats.c proc_meminfo.c proc_net_netstat.c proc_net_snmp.c proc_net_stat_conntrack.c proc_stat.c proc_vmstat.c
sources = procfile.c common.c log.c popen.c url.c config.c web_buffer.c storage_number.c web_client.c global_statistics.c rrd.c rrd2json.c web_server.c plugins_d.c daemon.c plugin_tc.c plugin_checks.c plugin_idlejitter.c plugin_proc.c unit_test.c main.c
headers = $(patsubst %.c,%.h,$(sources))
objects = $(patsubst %.c,%.o,$(sources) $(proc_sources))

all: netdata

netdata: $(objects)
	$(CC) -o netdata $(objects) -pthread -lz

%.o: %.c ${headers}
	$(CC) -c $< -o $@

clean:
	rm -rfv *.o netdata core
