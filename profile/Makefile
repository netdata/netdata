# SPDX-License-Identifier: GPL-3.0+

COMMON_CFLAGS=-I ../src/ -I ../
PROFILE_CFLAGS=-O1 -ggdb -Wall -Wextra 

benchmark-procfile-parser: benchmark-procfile-parser.c
	gcc ${PROFILE_CFLAGS} ${COMMON_CFLAGS} -o $@ $^ ../src/log.o ../src/procfile.o ../src/threads.o ../src/locks.o ../src/common.o ../src/clocks.o ../src/web_buffer.o ../src/storage_number.o -pthread -lm

statsd-stress: statsd-stress.c
	gcc -O2 -Wall -Wextra -o $@ $^ -pthread

all: statsd-stress benchmark-procfile-parser
