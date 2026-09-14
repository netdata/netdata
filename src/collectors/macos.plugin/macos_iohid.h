// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MACOS_IOHID_H
#define NETDATA_MACOS_IOHID_H 1

#include "plugin_macos.h"

#include <CoreFoundation/CoreFoundation.h>
#include <stdint.h>

typedef struct __IOHIDEventSystemClient *IOHIDEventSystemClientRef;
typedef struct __IOHIDServiceClient *IOHIDServiceClientRef;

struct macos_iohid_client {
    IOHIDEventSystemClientRef client;
    CFDictionaryRef matching;
    int primary_usage_page;
    int primary_usage;
    bool matching_configured;
};

bool macos_iohid_client_set_matching(
    struct macos_iohid_client *hid,
    int primary_usage_page,
    int primary_usage);
CFArrayRef macos_iohid_client_copy_services(struct macos_iohid_client *hid);
void macos_iohid_client_cleanup(struct macos_iohid_client *hid);

CFTypeRef macos_iohid_service_copy_property(IOHIDServiceClientRef service, CFStringRef key);
CFTypeRef macos_iohid_service_get_registry_id(IOHIDServiceClientRef service);
bool macos_iohid_service_copy_event_float(
    IOHIDServiceClientRef service,
    int64_t type,
    int32_t field,
    NETDATA_DOUBLE *value);

#endif /* NETDATA_MACOS_IOHID_H */
