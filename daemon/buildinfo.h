// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BUILDINFO_H
#define NETDATA_BUILDINFO_H 1

void print_build_info(void);

void print_build_info_json(void);

char *get_value_from_key(char *buffer, char *key);

void get_install_type(char **install_type, char **prebuilt_arch, char **prebuilt_dist);

#endif // NETDATA_BUILDINFO_H
