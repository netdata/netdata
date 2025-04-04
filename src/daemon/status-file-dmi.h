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
        char serial[64];  // System serial number
        char uuid[64];    // System UUID (hardware identifier)
        char asset_tag[64]; // System asset tag
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
        char serial[64];  // Board serial number
        char asset_tag[64]; // Board asset tag
    } board;

    struct {
        char type[16];
        char vendor[64];
        char version[64];
        char serial[64];  // Chassis serial number
        char asset_tag[64]; // Chassis asset tag
    } chassis;

    struct {
        char date[16];
        char release[64];
        char version[64];
        char vendor[64];
        char mode[16];    // Boot mode (UEFI/Legacy)
        bool secure_boot; // Secure boot status (true/false)
    } bios;
} DMI_INFO;

#include "status-file.h"

// Initialize DMI_INFO structure
void dmi_info_init(DMI_INFO *dmi);

// Platform-specific function to get DMI info 
void os_dmi_info_get(DMI_INFO *dmi);

#endif //NETDATA_STATUS_FILE_DMI_H
