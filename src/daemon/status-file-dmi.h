// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_DMI_H
#define NETDATA_STATUS_FILE_DMI_H

#include "status-file.h"

void finalize_vendor_product_vm(DAEMON_STATUS_FILE *ds);
void fill_dmi_info(DAEMON_STATUS_FILE *ds);

#endif //NETDATA_STATUS_FILE_DMI_H
