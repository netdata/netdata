
CONFIG_DIR = "conf.d"
LOG_DIR = "log"
PLUGINS_DIR = "plugins.d"

COMMON_FLAGS = -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'

ifdef debug
CFLAGS = $(COMMON_FLAGS) -Wall -Wextra -ggdb
else
CFLAGS = $(COMMON_FLAGS) -Wall -Wextra -O3
endif

all: netdata plugins
	@echo
	@echo "Compilation Done!"
	@echo "You can 'make getconf' to get the config file conf.d/netdata.conf from the netdata server."
	@echo

plugins: plugins.d/apps.plugin

netdata: netdata.c
	@echo
	@echo "Compiling netdata server..."
	$(CC) $(CFLAGS) -o netdata netdata.c -lpthread -lz

plugins.d/apps.plugin: apps_plugin.c
	@echo
	@echo "Compiling apps.plugin..."
	$(CC) $(CFLAGS) -o plugins.d/apps.plugin apps_plugin.c
	@-if [ ! "$$USER" = "root" ]; \
	then \
		echo; \
		echo " >>> apps.plugin requires root access to access files in /proc"; \
		echo " >>> Please authorize it!"; \
		echo; \
		sudo chown root plugins.d/apps.plugin; \
		sudo chmod 4775 plugins.d/apps.plugin; \
	else \
		chown root plugins.d/apps.plugin; \
		chmod 4775 plugins.d/apps.plugin; \
	fi


clean:
	-rm -f *.o netdata plugins.d/apps.plugin core

getconf:
	wget -O conf.d/netdata.conf.new "http://localhost:19999/netdata.conf"
	mv conf.d/netdata.conf conf.d/netdata.conf.old
	mv conf.d/netdata.conf.new conf.d/netdata.conf
