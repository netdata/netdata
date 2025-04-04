// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_DMI_H
#define NETDATA_STATUS_FILE_DMI_H

#include "status-file.h"

#define safecpy(dst, src) do {                                                                  \
    _Static_assert(sizeof(dst) != sizeof(char *),                                               \
                   "safecpy: dst must not be a pointer, but a buffer (e.g., char dst[SIZE])");  \
                                                                                                \
    strcatz(dst, 0, src, sizeof(dst));                                                          \
} while (0)

void finalize_vendor_product_vm(DAEMON_STATUS_FILE *ds);
void fill_dmi_info(DAEMON_STATUS_FILE *ds);

#endif //NETDATA_STATUS_FILE_DMI_H
