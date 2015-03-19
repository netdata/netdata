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

CC = gcc

ifdef debug
COMMON_FLAGS += debug=1
CFLAGS = -Wall -Wextra -ggdb -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'
else
CFLAGS = -Wall -Wextra -O3 -DBIN_DIR='$(BIN_DIR)' -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'
endif

all: options plugins

options:
	@echo "    COMPILING WITH OPTIONS: $(CFLAGS)"

plugins: apps.plugin

apps.plugin: apps_plugin.c
	@echo "    $(CC) $@"
	@$(CC) $(CFLAGS) -o $@ apps_plugin.c

clean:
	-rm -f *.o apps.plugin core

install: all
	@echo; \
	echo "    INSTALLING apps.plugin to $(PLUGINS_DIR)"; \
	if [ -f $(PLUGINS_DIR)/apps.plugin ]; \
	then \
		mv -f $(PLUGINS_DIR)/apps.plugin $(PLUGINS_DIR)/apps.plugin.old; \
	fi; \
	cp apps.plugin $(PLUGINS_DIR)/; \
	if [ ! "$$USER" = "root" ]; \
	then \
		echo; \
		echo " >>> apps.plugin requires root access to access files in /proc"; \
		echo " >>> Please authorize it!"; \
		echo; \
		sudo chown root $(PLUGINS_DIR)/apps.plugin; \
		sudo chmod 4775 $(PLUGINS_DIR)/apps.plugin; \
	else \
		chown root $(PLUGINS_DIR)/apps.plugin; \
		chmod 4775 $(PLUGINS_DIR)/apps.plugin; \
	fi; \
	echo;

.PHONY: options all plugins clean install
