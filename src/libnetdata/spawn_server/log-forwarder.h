// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG_FORWARDER_H
#define NETDATA_LOG_FORWARDER_H

#include "../libnetdata.h"

typedef struct LOG_FORWARDER LOG_FORWARDER;

LOG_FORWARDER *log_forwarder_start(void); // done once, at spawn_server_create()
void log_forwarder_add_fd(LOG_FORWARDER *lf, int fd); // to add a new fd
void log_forwarder_annotate_fd_name(LOG_FORWARDER *lf, int fd, const char *cmd); // set the syslog identifier
void log_forwarder_annotate_fd_pid(LOG_FORWARDER *lf, int fd, pid_t pid); // set the pid of the child process
bool log_forwarder_del_and_close_fd(LOG_FORWARDER *lf, int fd); // to remove an fd
void log_forwarder_stop(LOG_FORWARDER *lf); // done once, at spawn_server_destroy()

#endif //NETDATA_LOG_FORWARDER_H
