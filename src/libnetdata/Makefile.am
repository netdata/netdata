# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in

AM_CFLAGS = \
	$(OPTIONAL_MATH_CFLAGS) \
	$(OPTIONAL_ZLIB_CFLAGS) \
	$(OPTIONAL_UUID_CFLAGS) \
	$(NULL)

noinst_LIBRARIES=libnetdata.a

libnetdata_a_SOURCES = \
	adaptive_resortable_list.c \
	adaptive_resortable_list.h \
	appconfig.c \
	appconfig.h \
	avl.c \
	avl.h \
	clocks.c \
	clocks.h \
	common.c \
	dictionary.c \
	dictionary.h \
	eval.c \
	eval.h \
	inlined.h \
	locks.c \
	locks.h \
	log.c \
	log.h \
	popen.c \
	popen.h \
	procfile.c \
	procfile.h \
	simple_pattern.c \
	simple_pattern.h \
	socket.c \
	socket.h \
	statistical.c \
	statistical.h \
	storage_number.c \
	storage_number.h \
	threads.c \
	thread.h \
	web_buffer.c \
	web_buffer.h \
	$(NULL)
