// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos_iohid.h"

#include <dlfcn.h>

typedef struct __IOHIDEvent *IOHIDEventRef;

// These IOKit entry points are private API. Resolving them with dlsym instead of linking
// against them keeps the build working on SDKs that predate them (10.6 exports only
// IOHIDEventGetFloatValue of the seven below), and, on current macOS, turns a future Apple
// removal into a disabled collector rather than a dyld failure that stops netdata starting.
static struct {
    void *handle;
    bool attempted;

    IOHIDEventSystemClientRef (*client_create)(CFAllocatorRef allocator);
    void (*client_set_matching)(IOHIDEventSystemClientRef client, CFDictionaryRef matching);
    CFArrayRef (*client_copy_services)(IOHIDEventSystemClientRef client);
    CFTypeRef (*service_copy_property)(IOHIDServiceClientRef service, CFStringRef key);
    CFTypeRef (*service_get_registry_id)(IOHIDServiceClientRef service);
    IOHIDEventRef (*service_copy_event)(IOHIDServiceClientRef service, int64_t type, int32_t options, int64_t timestamp);
    double (*event_get_float_value)(IOHIDEventRef event, int32_t field);
} iohid_api = {0};

static bool macos_iohid_dlsym(void **dst, const char *name)
{
    *dst = dlsym(iohid_api.handle, name);
    return *dst != NULL;
}

// Only used to undo a partial load. A fully loaded handle is intentionally kept for the
// lifetime of the process: both the sensors and the gpu collector share it, netdata links
// IOKit directly anyway, and the `attempted` latch must survive a collector re-init so a
// known-missing symbol is not re-probed on every cycle.
static void macos_iohid_unload_api(void)
{
    if (iohid_api.handle) {
        dlclose(iohid_api.handle);
        iohid_api.handle = NULL;
    }

    // preserve `attempted` so a failed load is not retried on every collection cycle
    bool attempted = iohid_api.attempted;
    memset(&iohid_api, 0, sizeof(iohid_api));
    iohid_api.attempted = attempted;
}

static bool macos_iohid_load_api(void)
{
    if (iohid_api.attempted)
        return iohid_api.handle != NULL;

    iohid_api.attempted = true;

    iohid_api.handle = dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", RTLD_LAZY | RTLD_LOCAL);
    if (!iohid_api.handle)
        return false;

#define LOAD_IOHID(symbol, field)                                                                                      \
    do {                                                                                                               \
        if (!macos_iohid_dlsym((void **)&iohid_api.field, symbol)) {                                                    \
            collector_info("MACOS: IOKit does not export %s(); HID sensors are unavailable", symbol);                   \
            macos_iohid_unload_api();                                                                                   \
            return false;                                                                                              \
        }                                                                                                              \
    } while (0)

    LOAD_IOHID("IOHIDEventSystemClientCreate", client_create);
    LOAD_IOHID("IOHIDEventSystemClientSetMatching", client_set_matching);
    LOAD_IOHID("IOHIDEventSystemClientCopyServices", client_copy_services);
    LOAD_IOHID("IOHIDServiceClientCopyProperty", service_copy_property);
    LOAD_IOHID("IOHIDServiceClientGetRegistryID", service_get_registry_id);
    LOAD_IOHID("IOHIDServiceClientCopyEvent", service_copy_event);
    LOAD_IOHID("IOHIDEventGetFloatValue", event_get_float_value);

#undef LOAD_IOHID

    return true;
}

static void macos_iohid_client_invalidate(struct macos_iohid_client *hid)
{
    if (!hid)
        return;

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

    if (!macos_iohid_load_api())
        return NULL;

    if (!hid->client) {
        hid->client = iohid_api.client_create(kCFAllocatorDefault);
        if (!hid->client)
            return NULL;

        // Private API return semantics are not stable; CopyServices validates the configured client.
        iohid_api.client_set_matching(hid->client, hid->matching);
    }

    CFArrayRef services = iohid_api.client_copy_services(hid->client);
    if (!services) {
        macos_iohid_client_invalidate(hid);
        return NULL;
    }

    return services;
}

void macos_iohid_client_cleanup(struct macos_iohid_client *hid)
{
    if (!hid)
        return;

    macos_iohid_client_invalidate(hid);

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

    if (!macos_iohid_load_api())
        return NULL;

    return iohid_api.service_copy_property(service, key);
}

CFTypeRef macos_iohid_service_get_registry_id(IOHIDServiceClientRef service)
{
    if (!service)
        return NULL;

    if (!macos_iohid_load_api())
        return NULL;

    return iohid_api.service_get_registry_id(service);
}

bool macos_iohid_service_copy_event_float(
    IOHIDServiceClientRef service,
    int64_t type,
    int32_t field,
    NETDATA_DOUBLE *value)
{
    if (!service || !value)
        return false;

    if (!macos_iohid_load_api())
        return false;

    IOHIDEventRef event = iohid_api.service_copy_event(service, type, 0, 0);
    if (!event)
        return false;

    *value = iohid_api.event_get_float_value(event, field);
    CFRelease(event);
    return true;
}
