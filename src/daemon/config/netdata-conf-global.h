// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETDATA_CONF_GLOBAL_H
#define NETDATA_NETDATA_CONF_GLOBAL_H

#include "netdata-conf.h"

void netdata_conf_section_global(void);
void netdata_conf_section_global_run_as_user(const char **user);

#endif //NETDATA_NETDATA_CONF_GLOBAL_H
