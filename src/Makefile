ifndef BIN_DIR
BIN_DIR = "$(PWD)"
endif

ifndef CONFIG_DIR
CONFIG_DIR = "conf.d"
endif

ifndef LOG_DIR
LOG_DIR = "log"
endif

ifndef PLUGINS_DIR
PLUGINS_DIR = "plugins.d"
endif

COMMON_FLAGS = BIN_DIR='$(BIN_DIR)' CONFIG_DIR='$(CONFIG_DIR)' LOG_DIR='$(LOG_DIR)' PLUGINS_DIR='$(PLUGINS_DIR)'

ifdef debug
COMMON_FLAGS += debug=1
CFLAGS = -Wall -Wextra -ggdb -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'
else
CFLAGS = -Wall -Wextra -O3 -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'
endif

CC = gcc

proc_sources = proc_net_dev.c proc_net_ip_vs_stats.c proc_diskstats.c proc_meminfo.c proc_net_netstat.c proc_net_snmp.c proc_net_stat_conntrack.c proc_stat.c proc_vmstat.c proc_net_rpc_nfsd.c
sources = procfile.c common.c log.c popen.c url.c config.c web_buffer.c storage_number.c web_client.c global_statistics.c rrd.c rrd2json.c web_server.c plugins_d.c daemon.c plugin_tc.c plugin_checks.c plugin_idlejitter.c plugin_proc.c unit_test.c main.c
headers = $(patsubst %.c,%.h,$(sources))
objects = $(patsubst %.c,%.o,$(sources) $(proc_sources))

.PHONY: options
options:
	@echo "    COMPILING WITH OPTIONS: $(CFLAGS)"

all: options netdata plugins

netdata: $(objects)
	@echo "    $(CC) netdata"
	@$(CC) -o netdata $(objects) -pthread -lz

.PHONY: plugins
plugins:
	$(MAKE) -C plugins.d $(COMMON_FLAGS) all

%.o: %.c ${headers}
	@echo "    $(CC) $<"
	@$(CC) $(CFLAGS) -c $< -o $@

.PHONY: clean
clean:
	$(MAKE) -C plugins.d clean
	rm -f *.o netdata core

install: all
	$(MAKE) -C plugins.d $(COMMON_FLAGS) install
	@echo; \
	echo "    INSTALLING netdata to $(BIN_DIR)"; \
	if [ -f $(BIN_DIR)/netdata ]; \
	then \
		mv -f $(BIN_DIR)/netdata $(BIN_DIR)/netdata.old; \
	fi; \
	cp -f netdata $(BIN_DIR)/; \
	echo
