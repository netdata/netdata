//
// Created by costa on 4/4/25.
//

#ifndef NETDATA_STATUS_FILE_PRODUCT_H
#define NETDATA_STATUS_FILE_PRODUCT_H

#include "status-file.h"
#include "status-file-dmi.h"

void product_name_vendor_type(DAEMON_STATUS_FILE *ds);
bool dmi_is_virtual_machine(const DMI_INFO *dmi);

#endif //NETDATA_STATUS_FILE_PRODUCT_H
