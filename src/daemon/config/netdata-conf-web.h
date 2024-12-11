// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETDATA_CONF_WEB_H
#define NETDATA_NETDATA_CONF_WEB_H

#include "netdata-conf.h"

void netdata_conf_section_web(void);
void web_server_threading_selection(void);
void netdata_conf_web_security_init(void);

#endif //NETDATA_NETDATA_CONF_WEB_H
