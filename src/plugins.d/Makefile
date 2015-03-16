
CONFIG_DIR = "conf.d"
LOG_DIR = "log"
PLUGINS_DIR = "plugins.d"

COMMON_FLAGS = -DCONFIG_DIR='$(CONFIG_DIR)' -DLOG_DIR='$(LOG_DIR)' -DPLUGINS_DIR='$(PLUGINS_DIR)'

ifdef debug
CFLAGS = $(COMMON_FLAGS) -Wall -Wextra -ggdb
else
CFLAGS = $(COMMON_FLAGS) -Wall -Wextra -O3
endif

all: plugins

plugins: apps.plugin

apps.plugin: apps_plugin.c
	@echo
	@echo "Compiling apps.plugin..."
	$(CC) $(CFLAGS) -o apps.plugin apps_plugin.c
	@-if [ ! "$$USER" = "root" ]; \
	then \
		echo; \
		echo " >>> apps.plugin requires root access to access files in /proc"; \
		echo " >>> Please authorize it!"; \
		echo; \
		sudo chown root apps.plugin; \
		sudo chmod 4775 apps.plugin; \
	else \
		chown root apps.plugin; \
		chmod 4775 apps.plugin; \
	fi


clean:
	-rm -f *.o apps.plugin core
