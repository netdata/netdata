// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_DMI_H
#define NETDATA_STATUS_FILE_DMI_H

#include "libnetdata/libnetdata.h"

#define safecpy(dst, src) do {                                                                  \
    _Static_assert(sizeof(dst) != sizeof(char *),                                               \
                   "safecpy: dst must not be a pointer, but a buffer (e.g., char dst[SIZE])");  \
                                                                                                \
    strcatz(dst, 0, src, sizeof(dst));                                                          \
} while (0)

typedef struct dmi_info {
    struct {
        char vendor[64];
    } sys;

    struct {
        char name[64];
        char version[64];
        char sku[64];
        char family[64];
    } product;

    struct {
        char name[64];
        char version[64];
        char vendor[64];
    } board;

    struct {
        char type[16];
        char vendor[64];
        char version[64];
    } chassis;

    struct {
        char date[16];
        char release[64];
        char version[64];
        char vendor[64];
    } bios;
} DMI_INFO;

#include "status-file.h"

bool dmi_is_virtual_machine(const DMI_INFO *dmi);
void dmi_normalize_vendor_field(char *buf, size_t buf_size);

void fill_dmi_info(DAEMON_STATUS_FILE *ds);

#endif //NETDATA_STATUS_FILE_DMI_H
