// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos_iohid.h"

typedef struct __IOHIDEvent *IOHIDEventRef;

extern IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);
extern void IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef matching);
extern CFArrayRef IOHIDEventSystemClientCopyServices(IOHIDEventSystemClientRef client);
extern CFTypeRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef key);
extern CFTypeRef IOHIDServiceClientGetRegistryID(IOHIDServiceClientRef service);
extern IOHIDEventRef IOHIDServiceClientCopyEvent(
    IOHIDServiceClientRef service,
    int64_t type,
    int32_t options,
    int64_t timestamp);
extern double IOHIDEventGetFloatValue(IOHIDEventRef event, int32_t field);

#define MACOS_IOHID_SERVICES_CACHE_TTL_SEC 300

static void macos_iohid_services_invalidate(struct macos_iohid_client *hid)
{
    if (!hid)
        return;

    if (hid->services) {
        CFRelease(hid->services);
        hid->services = NULL;
    }
    hid->services_collected_ut = 0;
}

static void macos_iohid_client_invalidate(struct macos_iohid_client *hid)
{
    if (!hid)
        return;

    macos_iohid_services_invalidate(hid);
    if (!hid->client)
        return;

    CFRelease(hid->client);
    hid->client = NULL;
}

static CFNumberRef macos_iohid_cfnumber_int(int value)
{
    return CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &value);
}

static bool macos_iohid_matching_create(struct macos_iohid_client *hid, int primary_usage_page, int primary_usage)
{
    CFStringRef keys[] = {CFSTR("PrimaryUsagePage"), CFSTR("PrimaryUsage")};
    CFNumberRef values[] = {
        macos_iohid_cfnumber_int(primary_usage_page),
        macos_iohid_cfnumber_int(primary_usage),
    };
    if (!values[0] || !values[1]) {
        if (values[0])
            CFRelease(values[0]);
        if (values[1])
            CFRelease(values[1]);
        return false;
    }

    CFDictionaryRef matching = CFDictionaryCreate(
        kCFAllocatorDefault,
        (const void **)keys,
        (const void **)values,
        2,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);

    CFRelease(values[0]);
    CFRelease(values[1]);

    if (!matching)
        return false;

    hid->matching = matching;
    hid->primary_usage_page = primary_usage_page;
    hid->primary_usage = primary_usage;
    hid->matching_configured = true;
    return true;
}

bool macos_iohid_client_set_matching(
    struct macos_iohid_client *hid,
    int primary_usage_page,
    int primary_usage)
{
    if (!hid)
        return false;

    if (hid->matching_configured &&
        hid->primary_usage_page == primary_usage_page &&
        hid->primary_usage == primary_usage)
        return true;

    macos_iohid_client_invalidate(hid);
    macos_iohid_services_invalidate(hid);

    if (hid->matching) {
        CFRelease(hid->matching);
        hid->matching = NULL;
    }

    hid->matching_configured = false;
    return macos_iohid_matching_create(hid, primary_usage_page, primary_usage);
}

CFArrayRef macos_iohid_client_copy_services(struct macos_iohid_client *hid)
{
    if (!hid || !hid->matching_configured || !hid->matching)
        return NULL;

    usec_t now_ut = now_monotonic_usec();
    if (hid->services &&
        now_ut - hid->services_collected_ut < (usec_t)MACOS_IOHID_SERVICES_CACHE_TTL_SEC * USEC_PER_SEC) {
        CFRetain(hid->services);
        return hid->services;
    }

    macos_iohid_services_invalidate(hid);

    if (!hid->client) {
        hid->client = IOHIDEventSystemClientCreate(kCFAllocatorDefault);
        if (!hid->client)
            return NULL;

        // Private API return semantics are not stable; CopyServices validates the configured client.
        IOHIDEventSystemClientSetMatching(hid->client, hid->matching);
    }

    CFArrayRef services = IOHIDEventSystemClientCopyServices(hid->client);
    if (!services) {
        macos_iohid_client_invalidate(hid);
        return NULL;
    }

    hid->services = services;
    hid->services_collected_ut = now_ut;
    CFRetain(hid->services);

    return services;
}

void macos_iohid_client_cleanup(struct macos_iohid_client *hid)
{
    if (!hid)
        return;

    macos_iohid_client_invalidate(hid);
    macos_iohid_services_invalidate(hid);

    if (hid->matching) {
        CFRelease(hid->matching);
        hid->matching = NULL;
    }

    hid->primary_usage_page = 0;
    hid->primary_usage = 0;
    hid->matching_configured = false;
}

CFTypeRef macos_iohid_service_copy_property(IOHIDServiceClientRef service, CFStringRef key)
{
    if (!service || !key)
        return NULL;

    return IOHIDServiceClientCopyProperty(service, key);
}

CFTypeRef macos_iohid_service_get_registry_id(IOHIDServiceClientRef service)
{
    if (!service)
        return NULL;

    return IOHIDServiceClientGetRegistryID(service);
}

bool macos_iohid_service_copy_event_float(
    IOHIDServiceClientRef service,
    int64_t type,
    int32_t field,
    NETDATA_DOUBLE *value)
{
    if (!service || !value)
        return false;

    IOHIDEventRef event = IOHIDServiceClientCopyEvent(service, type, 0, 0);
    if (!event)
        return false;

    *value = IOHIDEventGetFloatValue(event, field);
    CFRelease(event);
    return true;
}
