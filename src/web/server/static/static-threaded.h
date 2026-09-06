// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_SERVER_STATIC_THREADED_H
#define NETDATA_WEB_SERVER_STATIC_THREADED_H

#include "web/server/web_server.h"

void socket_listen_main_static_threaded(void *ptr);
void web_server_remove_current_socket_from_poll(void);
bool web_server_startup_wait_acquire(void);
void web_server_startup_wait_release(void);

#endif //NETDATA_WEB_SERVER_STATIC_THREADED_H
