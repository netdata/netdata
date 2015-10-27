ifndef BIN_DIR
BIN_DIR = "."
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

ifndef nomath
STORAGE_WITH_MATH = 1
endif

ifdef nofork
NETDATA_NO_DAEMON = 1
endif

ifdef nozlib
NETDATA_WITHOUT_ZLIB = 1
endif

COMMON_FLAGS = BIN_DIR='$(BIN_DIR)' CONFIG_DIR='$(CONFIG_DIR)' LOG_DIR='$(LOG_DIR)' PLUGINS_DIR='$(PLUGINS_DIR)'

proc_sources = proc_net_dev.c proc_net_ip_vs_stats.c proc_diskstats.c proc_meminfo.c proc_net_netstat.c proc_net_snmp.c proc_net_stat_conntrack.c proc_stat.c proc_vmstat.c proc_net_rpc_nfsd.c proc_sys_kernel_random_entropy_avail.c proc_interrupts.c
sources = rrd.c rrd2json.c avl.c dictionary.c procfile.c common.c log.c popen.c url.c config.c web_buffer.c storage_number.c web_client.c global_statistics.c web_server.c plugins_d.c daemon.c plugin_tc.c plugin_checks.c plugin_idlejitter.c plugin_proc.c unit_test.c main.c
libs    = -pthread

ifdef debug
COMMON_FLAGS += debug=1
CFLAGS = -Wall -Wextra -O -ggdb -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)' -fno-omit-frame-pointer
# CFLAGS := -Wall -Wextra -O -g -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)' -fsanitize=thread -fno-omit-frame-pointer -fPIE
# libs += -pie -ltsan
libs += -rdynamic
else
CFLAGS := -Wall -Wextra -O3 -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)' -fomit-frame-pointer
endif

CC = gcc

ifdef STORAGE_WITH_MATH
CFLAGS += -DSTORAGE_WITH_MATH=1
libs += -lm
endif

ifdef NETDATA_NO_DAEMON
CFLAGS += -DNETDATA_NO_DAEMON=1
endif

ifdef NETDATA_WITHOUT_ZLIB
CFLAGS += -DNETDATA_WITHOUT_ZLIB=1
else
libs += -lz
endif

# nfacct requires root access, so we prefer it as a plugin.d external plugin
ifdef INTERNAL_PLUGIN_NFACCT
CFLAGS += -DINTERNAL_PLUGIN_NFACCT=1
sources += plugin_nfacct.c
libs += -lmnl -lnetfilter_acct
endif

headers = $(patsubst %.c,%.h,$(sources))
objects = $(patsubst %.c,%.o,$(sources) $(proc_sources))

all: show_options netdata plugins

%.o: %.c ${headers}
	$(CC) $(CFLAGS) -c $< -o $@

netdata: $(objects)
	$(CC) -o netdata $(objects) $(libs)

show_options:
	@echo "    COMPILING WITH OPTIONS: $(CFLAGS)"

plugins:
	$(MAKE) -C plugins.d $(COMMON_FLAGS) all

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

.PHONY: all show_options plugins clean install
