// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
#define NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"

#include <linux/capability.h>
#include <systemd/sd-journal.h>
#include <syslog.h>

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         60

extern netdata_mutex_t stdout_mutex;

void journal_init(void);
void function_systemd_journal(const char *transaction, char *function, int timeout, bool *cancelled);
void journal_files_registry_update(void);

#endif //NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
