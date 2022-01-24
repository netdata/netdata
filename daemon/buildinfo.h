// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BUILDINFO_H
#define NETDATA_BUILDINFO_H 1

struct install_type_info {
    char *install_type;
    char *prebuilt_arch;
    char *prebuilt_distro;
};

extern void print_build_info(void);

extern void print_build_info_json(void);

extern char *get_value_from_key(char *buffer, char *key);

extern struct install_type_info get_install_type();

#endif // NETDATA_BUILDINFO_H
