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

// Initialize DMI_INFO structure
void dmi_info_init(DMI_INFO *dmi);

// Platform-specific function to get DMI info 
void os_dmi_info_get(DMI_INFO *dmi);

#endif //NETDATA_STATUS_FILE_DMI_H
