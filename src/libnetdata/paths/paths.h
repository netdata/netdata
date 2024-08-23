// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PATHS_H
#define NETDATA_PATHS_H

#include "../libnetdata.h"

size_t filename_from_path_entry(char out[FILENAME_MAX], const char *path, const char *entry, const char *extension);
char *filename_from_path_entry_strdupz(const char *path, const char *entry);

bool filename_is_file(const char *filename);
bool filename_is_dir(const char *filename, bool create_it);

bool path_entry_is_file(const char *path, const char *entry);
bool path_entry_is_dir(const char *path, const char *entry, bool create_it);

void recursive_config_double_dir_load(
    const char *user_path
    , const char *stock_path
    , const char *entry
    , int (*callback)(const char *filename, void *data, bool stock_config)
    , void *data
    , size_t depth
);

#endif //NETDATA_PATHS_H
