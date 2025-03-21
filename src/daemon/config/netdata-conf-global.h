// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETDATA_CONF_GLOBAL_H
#define NETDATA_NETDATA_CONF_GLOBAL_H

#include "libnetdata/libnetdata.h"

void netdata_conf_section_global(void);
void netdata_conf_section_global_run_as_user(const char **user);
void netdata_conf_section_global_hostname(void);

size_t netdata_conf_cpus(void);
void libuv_initialize(void);

void netdata_conf_glibc_malloc_initialize(size_t wanted_arenas, size_t trim_threshold);

#include "netdata-conf.h"

#endif //NETDATA_NETDATA_CONF_GLOBAL_H
