ifndef BIN_DIR
BIN_DIR = "$(PWD)"
endif

ifndef CONFIG_DIR
CONFIG_DIR = "$(PWD)/conf.d"
endif

ifndef LOG_DIR
LOG_DIR = "$(PWD)/log"
endif

ifndef PLUGINS_DIR
PLUGINS_DIR = "$(PWD)/plugins.d"
endif

COMMON_FLAGS = BIN_DIR='$(BIN_DIR)' CONFIG_DIR='$(CONFIG_DIR)' LOG_DIR='$(LOG_DIR)' PLUGINS_DIR='$(PLUGINS_DIR)'

ifdef debug
COMMON_FLAGS += debug=1
endif

.PHONY: all
all: 
	$(MAKE) -C src $(COMMON_FLAGS) all

.PHONY: clean
clean:
	$(MAKE) -C src clean
	-rm -f netdata netdata.old plugins.d/apps.plugin plugins.d/apps.plugin.old

.PHONY: install
install:
	$(MAKE) -C src $(COMMON_FLAGS) install

