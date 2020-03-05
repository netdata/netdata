# SPDX-License-Identifier: GPL-3.0-or-later

COMMON_CFLAGS = -I ../../ -DTARGET_OS=1  -Wall -Wextra
PROFILE_CFLAGS = -O1 -ggdb $(COMMON_CFLAGS)
PERFORMANCE_CFLAGS = -O2 $(COMMON_CFLAGS)

CFLAGS = $(PERFORMANCE_CFLAGS)

LIBNETDATA_FILES = \
    ../../libnetdata/popen/popen.o \
    ../../libnetdata/storage_number/storage_number.o \
    ../../libnetdata/avl/avl.o \
    ../../libnetdata/socket/socket.o \
    ../../libnetdata/os.o \
    ../../libnetdata/clocks/clocks.o \
    ../../libnetdata/procfile/procfile.o \
    ../../libnetdata/statistical/statistical.o \
    ../../libnetdata/eval/eval.o \
    ../../libnetdata/threads/threads.o \
    ../../libnetdata/dictionary/dictionary.o \
    ../../libnetdata/simple_pattern/simple_pattern.o \
    ../../libnetdata/url/url.o \
    ../../libnetdata/config/appconfig.o \
    ../../libnetdata/libnetdata.o \
    ../../libnetdata/buffer/buffer.o \
    ../../libnetdata/adaptive_resortable_list/adaptive_resortable_list.o \
    ../../libnetdata/locks/locks.o \
    ../../libnetdata/log/log.o \
    $(NULL)

COMMON_LDFLAGS = $(LIBNETDATA_FILES) -pthread -lm

all: statsd-stress benchmark-procfile-parser test-eval benchmark-dictionary benchmark-value-pairs

benchmark-procfile-parser: benchmark-procfile-parser.c
	gcc ${CFLAGS} -o $@ $^ ${COMMON_LDFLAGS}

benchmark-dictionary: benchmark-dictionary.c
	gcc ${CFLAGS} -o $@ $^ ${COMMON_LDFLAGS}

benchmark-value-pairs: benchmark-value-pairs.c
	gcc ${CFLAGS} -o $@ $^ ${COMMON_LDFLAGS}

statsd-stress: statsd-stress.c
	gcc ${CFLAGS} -o $@ $^ ${COMMON_LDFLAGS}

test-eval: test-eval.c
	gcc ${CFLAGS} -o $@ $^ ${COMMON_LDFLAGS}

clean:
	rm -f benchmark-procfile-parser statsd-stress test-eval benchmark-dictionary benchmark-value-pairs
