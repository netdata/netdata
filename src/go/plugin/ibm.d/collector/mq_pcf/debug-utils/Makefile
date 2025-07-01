# Makefile for MQ PCF debug utilities
CC = gcc
CFLAGS = -Wall -Wextra -g
LDFLAGS = -lmqm

# Find MQ include and lib directories
MQ_INCLUDE = /usr/lib/netdata/ibm-mqclient/inc
MQ_LIB = /usr/lib/netdata/ibm-mqclient/lib64

# Add MQ paths
CFLAGS += -I$(MQ_INCLUDE)
LDFLAGS += -L$(MQ_LIB) -Wl,-rpath,$(MQ_LIB)

# List of utilities
UTILS = dump_queue_attrs dump_channel_attrs_simple test_queue_stats test_queue_status test_queue_stats_attrs test_reset_q_stats_detailed

# Default target
all: $(UTILS)

# Generic rule for building utilities
%: %.c
	$(CC) $(CFLAGS) -o $@ $< $(LDFLAGS)

# Clean target
clean:
	rm -f $(UTILS)

# Help target
help:
	@echo "Available targets:"
	@echo "  all     - Build all utilities"
	@echo "  clean   - Remove built utilities"
	@echo "  help    - Show this help message"
	@echo ""
	@echo "Individual utilities:"
	@for util in $(UTILS); do echo "  $$util"; done

.PHONY: all clean help