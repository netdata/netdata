// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_IO_H
#define NETDATA_STATUS_FILE_IO_H

#include "libnetdata/libnetdata.h"
#include "status-file.h"

bool status_file_io_load(const char *filename, bool (*cb)(const char *, void *), void *data, bool log);
void status_file_io_remove(const char *filename);
// sync_file: fsync the file before publishing it. Without it the save survives a process kill (the data is in the
// kernel page cache), but a power loss may leave the file empty or zero-filled.
bool status_file_io_save(const char *filename, const void *data, size_t size, bool log, bool sync_file);

#endif //NETDATA_STATUS_FILE_IO_H
