// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_IO_H
#define NETDATA_STATUS_FILE_IO_H

#include "libnetdata/libnetdata.h"
#include "status-file.h"

bool status_file_io_load(const char *filename, bool (*cb)(const char *, void *), void *data, bool log);
bool status_file_io_save(const char *filename, const void *data, size_t size, bool log);

// Starts the worker thread that performs async-signal-safe deferred
// renames of status files on UCRT64. No-op on POSIX where rename(2) is
// already async-signal-safe. Must be called once during startup before any
// signal handler can publish a status file.
void status_file_io_init(void);

// Drains any pending rename before clean shutdown so the final status
// snapshot is not lost. No-op on POSIX.
void status_file_io_shutdown(void);

#endif //NETDATA_STATUS_FILE_IO_H
