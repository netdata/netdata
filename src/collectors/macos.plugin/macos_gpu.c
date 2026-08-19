// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos_iohid.h"
#include "macos_smc.h"
#include "plugin_macos.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <ctype.h>
#include <dlfcn.h>
#include <math.h>
#include <stdint.h>
#include <string.h>
#include <strings.h>

#define MACOS_GPU_DVFS_KEY "voltage-states9"
#define MACOS_GPU_DVFS_SCALE_HZ_PER_MHZ 1000000.0
#define MACOS_GPU_MAX_DVFS_STATES 64
#define MACOS_GPU_SAMPLE_STALE_AFTER_SEC 30
#define MACOS_GPU_MAX_SMC_KEYS 64
#define MACOS_GPU_DEFAULT_SMC_TEMPERATURE_SAMPLE_EVERY 10
#define MACOS_GPU_SMC_TEMPERATURE_DISCOVERY_EVERY 300
#define MACOS_GPU_IOREPORT_RETRY_EVERY_SEC 10
#define MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE 3
#define MACOS_GPU_HID_TEMPERATURE_SENSOR_PREFIX "GPU MTR Temp Sensor"
#define MACOS_GPU_MATCH_STRING_MAX 256

typedef const void *IOReportSubscriptionRef;

enum {
    MACOS_GPU_HID_PAGE_APPLE_VENDOR = 0xff00,
    MACOS_GPU_HID_USAGE_TEMPERATURE_SENSOR = 0x0005,
    MACOS_GPU_HID_EVENT_TYPE_TEMPERATURE = 15,
};

enum macos_gpu_temperature_read_status {
    MACOS_GPU_TEMPERATURE_READ_FAILED,
    MACOS_GPU_TEMPERATURE_READ_SKIPPED,
    MACOS_GPU_TEMPERATURE_READ_RETRYING,
    MACOS_GPU_TEMPERATURE_READ_OK,
};

enum macos_gpu_temperature_source {
    MACOS_GPU_TEMPERATURE_SOURCE_NONE,
    MACOS_GPU_TEMPERATURE_SOURCE_SMC,
    MACOS_GPU_TEMPERATURE_SOURCE_IOHID,
};

struct macos_gpu_ioreport_funcs {
    CFDictionaryRef (*copy_all_channels)(uint64_t, uint64_t);
    IOReportSubscriptionRef (*create_subscription)(CFTypeRef, CFMutableDictionaryRef, CFMutableDictionaryRef *, uint64_t, CFTypeRef);
    CFDictionaryRef (*create_samples)(IOReportSubscriptionRef, CFMutableDictionaryRef, CFTypeRef);
    CFDictionaryRef (*create_samples_delta)(CFDictionaryRef, CFDictionaryRef, CFTypeRef);
    CFStringRef (*channel_get_group)(CFDictionaryRef);
    CFStringRef (*channel_get_subgroup)(CFDictionaryRef);
    CFStringRef (*channel_get_channel_name)(CFDictionaryRef);
    int64_t (*simple_get_integer_value)(CFDictionaryRef, int32_t);
    CFStringRef (*channel_get_unit_label)(CFDictionaryRef);
    int32_t (*state_get_count)(CFDictionaryRef);
    CFStringRef (*state_get_name_for_index)(CFDictionaryRef, int32_t);
    int64_t (*state_get_residency)(CFDictionaryRef, int32_t);
};

struct macos_gpu_metrics {
    bool has_active_residency;
    NETDATA_DOUBLE active_residency_perc;
    bool has_performance_state_residency;
    size_t performance_state_count;
    NETDATA_DOUBLE performance_state_residency_perc[MACOS_GPU_MAX_DVFS_STATES];
    bool has_frequency;
    NETDATA_DOUBLE frequency_mhz;
    bool has_power;
    NETDATA_DOUBLE power_w;
    bool has_temperature;
    NETDATA_DOUBLE temperature_c;
    enum macos_gpu_temperature_source temperature_source;
};

struct macos_gpu_smc_key {
    char key[MACOS_SMC_KEY_LEN + 1];
    struct macos_smc_key_info info;
};

struct macos_gpu_state {
    bool initialized;
    bool permanent_failure;
    bool temperature_available;
    bool ioreport_power_available;
    bool ioreport_power_source_available;
    bool logged_sample_error;
    bool logged_temperature_error;
    bool logged_malformed_ioreport;

    void *ioreport_handle;
    struct macos_gpu_ioreport_funcs io;
    IOReportSubscriptionRef subscription;
    CFMutableDictionaryRef channels;
    CFMutableDictionaryRef subscribed_channels;
    CFDictionaryRef all_channels;
    CFMutableArrayRef selected_channels;
    CFDictionaryRef previous_sample;
    usec_t previous_sample_ut;
    usec_t last_ioreport_open_attempt_ut;
    usec_t last_smc_temperature_read_ut;
    usec_t last_smc_temperature_discovery_ut;
    unsigned ioreport_consecutive_failures;
    unsigned ioreport_power_consecutive_misses;
    unsigned hid_temperature_consecutive_failures;
    unsigned smc_temperature_consecutive_failures;

    NETDATA_DOUBLE *gpu_freqs_mhz;
    size_t gpu_freqs_count;
    int smc_temperature_sample_every_s;
    int temperature_chart_update_every;
    enum macos_gpu_temperature_source active_temperature_source;

    io_connect_t smc_connection;
    struct macos_gpu_smc_key smc_gpu_keys[MACOS_GPU_MAX_SMC_KEYS];
    size_t smc_gpu_keys_count;
    struct macos_iohid_client hid_client;

    RRDSET *st_active_residency;
    RRDDIM *rd_active_residency;
    RRDSET *st_performance_state_residency;
    RRDDIM *rd_performance_state_residency[MACOS_GPU_MAX_DVFS_STATES];
    RRDSET *st_frequency;
    RRDDIM *rd_frequency;
    RRDSET *st_power;
    RRDDIM *rd_power;
    RRDSET *st_temperature;
    RRDDIM *rd_temperature;
};

static struct macos_gpu_state gpu = {0};

bool macos_gpu_is_hid_temperature_sensor_name(const char *name)
{
    return name &&
           strncmp(
               name,
               MACOS_GPU_HID_TEMPERATURE_SENSOR_PREFIX,
               sizeof(MACOS_GPU_HID_TEMPERATURE_SENSOR_PREFIX) - 1) == 0;
}

static const char *macos_gpu_temperature_source_label(enum macos_gpu_temperature_source source)
{
    switch (source) {
        case MACOS_GPU_TEMPERATURE_SOURCE_SMC:
            return "smc";
        case MACOS_GPU_TEMPERATURE_SOURCE_IOHID:
            return "iohid";
        case MACOS_GPU_TEMPERATURE_SOURCE_NONE:
        default:
            return "unknown";
    }
}

static uint32_t macos_gpu_read_le32(const uint8_t *p)
{
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

static bool macos_gpu_cfstring_to_cstr(CFStringRef str, char *dst, size_t dst_size)
{
    if (!str || !dst || dst_size == 0)
        return false;

    if (!CFStringGetCString(str, dst, dst_size, kCFStringEncodingUTF8)) {
        dst[0] = '\0';
        return false;
    }

    dst[dst_size - 1] = '\0';
    return true;
}

static bool macos_gpu_ioreport_string(CFStringRef str, char *dst, size_t dst_size)
{
    if (!macos_gpu_cfstring_to_cstr(str, dst, dst_size))
        return false;

    char *start = dst;
    while (*start && isspace((unsigned char)*start))
        start++;

    size_t remaining = dst_size - (size_t)(start - dst);
    char *end = start + strnlen(start, remaining);
    while (end > start && isspace((unsigned char)end[-1]))
        *--end = '\0';

    if (start != dst)
        memmove(dst, start, (size_t)(end - start) + 1);

    return dst[0] != '\0';
}

static bool macos_gpu_token_matches_ci(const char *value, const char *needle)
{
    if (!value || !needle || !*needle)
        return false;

    size_t needle_len = strnlen(needle, MACOS_GPU_MATCH_STRING_MAX);
    if (needle_len == MACOS_GPU_MATCH_STRING_MAX)
        return false;

    const char *p = value;
    while (*p) {
        while (*p && !isalnum((unsigned char)*p))
            p++;

        const char *start = p;
        while (*p && isalnum((unsigned char)*p))
            p++;

        size_t len = (size_t)(p - start);
        if (len == needle_len && strncasecmp(start, needle, needle_len) == 0)
            return true;
    }

    return false;
}

static bool macos_gpu_ioreport_state_is_inactive(const char *name)
{
    return macos_gpu_token_matches_ci(name, "idle") ||
           macos_gpu_token_matches_ci(name, "down") ||
           macos_gpu_token_matches_ci(name, "off") ||
           macos_gpu_token_matches_ci(name, "sleep");
}

static bool macos_gpu_ioreport_item_strings(
    CFDictionaryRef item,
    char *group,
    size_t group_size,
    char *subgroup,
    size_t subgroup_size,
    char *channel,
    size_t channel_size,
    char *unit,
    size_t unit_size)
{
    group[0] = '\0';
    subgroup[0] = '\0';
    channel[0] = '\0';
    unit[0] = '\0';

    macos_gpu_ioreport_string(gpu.io.channel_get_group(item), group, group_size);
    macos_gpu_ioreport_string(gpu.io.channel_get_subgroup(item), subgroup, subgroup_size);
    macos_gpu_ioreport_string(gpu.io.channel_get_channel_name(item), channel, channel_size);
    macos_gpu_ioreport_string(gpu.io.channel_get_unit_label(item), unit, unit_size);

    return group[0] != '\0' && channel[0] != '\0';
}

static void macos_gpu_performance_state_dim_name(size_t index, char *dst, size_t dst_size)
{
    if (index < gpu.gpu_freqs_count && isfinite(gpu.gpu_freqs_mhz[index]) && gpu.gpu_freqs_mhz[index] > 0.0) {
        snprintfz(
            dst,
            dst_size,
            "pstate_%zu_%lldmhz",
            index,
            (long long)llround(gpu.gpu_freqs_mhz[index]));
        return;
    }

    snprintfz(dst, dst_size, "pstate_%zu", index);
}

static void macos_gpu_mark_charts_obsolete(void)
{
    if (gpu.st_active_residency)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_active_residency);
    if (gpu.st_performance_state_residency)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_performance_state_residency);
    if (gpu.st_frequency)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_frequency);
    if (gpu.st_power)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_power);
    if (gpu.st_temperature)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_temperature);
}

static void macos_gpu_ioreport_data_charts_obsolete(void)
{
    if (gpu.st_active_residency)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_active_residency);
    gpu.st_active_residency = NULL;
    gpu.rd_active_residency = NULL;

    if (gpu.st_performance_state_residency)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_performance_state_residency);
    gpu.st_performance_state_residency = NULL;
    memset(gpu.rd_performance_state_residency, 0, sizeof(gpu.rd_performance_state_residency));

    if (gpu.st_frequency)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_frequency);
    gpu.st_frequency = NULL;
    gpu.rd_frequency = NULL;
}

static void macos_gpu_temperature_chart_obsolete(void)
{
    if (gpu.st_temperature)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_temperature);

    gpu.st_temperature = NULL;
    gpu.rd_temperature = NULL;
    gpu.temperature_chart_update_every = 0;
}

static void macos_gpu_power_chart_obsolete(void)
{
    if (gpu.st_power)
        rrdset_is_obsolete___safe_from_collector_thread(gpu.st_power);

    gpu.st_power = NULL;
    gpu.rd_power = NULL;
}

static bool macos_gpu_dlsym(void **dst, const char *name)
{
    *dst = dlsym(gpu.ioreport_handle, name);
    return *dst != NULL;
}

static void macos_gpu_unload_ioreport(void)
{
    if (gpu.ioreport_handle) {
        dlclose(gpu.ioreport_handle);
        gpu.ioreport_handle = NULL;
    }

    memset(&gpu.io, 0, sizeof(gpu.io));
}

static bool macos_gpu_load_ioreport(void)
{
    gpu.ioreport_handle = dlopen("/usr/lib/libIOReport.dylib", RTLD_LAZY | RTLD_LOCAL);
    if (!gpu.ioreport_handle)
        return false;

#define LOAD_IOREPORT(symbol, field)                                                                                  \
    do {                                                                                                               \
        if (!macos_gpu_dlsym((void **)&gpu.io.field, symbol)) {                                                        \
            macos_gpu_unload_ioreport();                                                                               \
            return false;                                                                                              \
        }                                                                                                              \
    } while (0)

    LOAD_IOREPORT("IOReportCopyAllChannels", copy_all_channels);
    LOAD_IOREPORT("IOReportCreateSubscription", create_subscription);
    LOAD_IOREPORT("IOReportCreateSamples", create_samples);
    LOAD_IOREPORT("IOReportCreateSamplesDelta", create_samples_delta);
    LOAD_IOREPORT("IOReportChannelGetGroup", channel_get_group);
    LOAD_IOREPORT("IOReportChannelGetSubGroup", channel_get_subgroup);
    LOAD_IOREPORT("IOReportChannelGetChannelName", channel_get_channel_name);
    LOAD_IOREPORT("IOReportSimpleGetIntegerValue", simple_get_integer_value);
    LOAD_IOREPORT("IOReportChannelGetUnitLabel", channel_get_unit_label);
    LOAD_IOREPORT("IOReportStateGetCount", state_get_count);
    LOAD_IOREPORT("IOReportStateGetNameForIndex", state_get_name_for_index);
    LOAD_IOREPORT("IOReportStateGetResidency", state_get_residency);

#undef LOAD_IOREPORT

    return true;
}

static bool macos_gpu_read_pmgr_dvfs(void)
{
    io_iterator_t iter = IO_OBJECT_NULL;
    IOReturn kr = IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleARMIODevice"), &iter);
    if (kr != kIOReturnSuccess || iter == IO_OBJECT_NULL)
        return false;

    bool ok = false;
    io_registry_entry_t entry;
    while ((entry = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
        io_name_t name;
        if (IORegistryEntryGetName(entry, name) != kIOReturnSuccess || strcmp(name, "pmgr") != 0) {
            IOObjectRelease(entry);
            continue;
        }

        CFMutableDictionaryRef props = NULL;
        kr = IORegistryEntryCreateCFProperties(entry, &props, kCFAllocatorDefault, 0);
        IOObjectRelease(entry);
        if (kr != kIOReturnSuccess || !props)
            break;

        CFTypeRef value = CFDictionaryGetValue(props, CFSTR(MACOS_GPU_DVFS_KEY));
        if (value && CFGetTypeID(value) == CFDataGetTypeID()) {
            CFDataRef data = (CFDataRef)value;
            CFIndex len = CFDataGetLength(data);
            if (len >= 16 && len % 8 == 0) {
                size_t items = (size_t)len / 8;
                gpu.gpu_freqs_mhz = callocz(items - 1, sizeof(*gpu.gpu_freqs_mhz));
                gpu.gpu_freqs_count = items - 1;

                const UInt8 *bytes = CFDataGetBytePtr(data);
                for (size_t i = 1; i < items; i++)
                    gpu.gpu_freqs_mhz[i - 1] = (NETDATA_DOUBLE)macos_gpu_read_le32(&bytes[i * 8]) / MACOS_GPU_DVFS_SCALE_HZ_PER_MHZ;

                ok = gpu.gpu_freqs_count > 0;
            }
        }

        CFRelease(props);
        break;
    }

    IOObjectRelease(iter);
    return ok;
}

static bool macos_gpu_ioreport_channel_selected(const char *group, const char *subgroup, const char *channel)
{
    if (!strcmp(group, "GPU Stats") && !strcmp(subgroup, "GPU Performance States"))
        return true;

    if (!strcmp(group, "Energy Model") && (!strcmp(channel, "GPU Energy") || !strcmp(channel, "GPU")))
        return true;

    return false;
}

static void macos_gpu_ioreport_close_subscription(void)
{
    gpu.ioreport_power_available = false;
    gpu.ioreport_power_source_available = false;
    gpu.ioreport_power_consecutive_misses = 0;

    if (gpu.previous_sample) {
        CFRelease(gpu.previous_sample);
        gpu.previous_sample = NULL;
        gpu.previous_sample_ut = 0;
    }
    if (gpu.subscription) {
        CFRelease(gpu.subscription);
        gpu.subscription = NULL;
    }
    if (gpu.subscribed_channels) {
        CFRelease(gpu.subscribed_channels);
        gpu.subscribed_channels = NULL;
    }
    if (gpu.channels) {
        CFRelease(gpu.channels);
        gpu.channels = NULL;
    }
    if (gpu.selected_channels) {
        CFRelease(gpu.selected_channels);
        gpu.selected_channels = NULL;
    }
    if (gpu.all_channels) {
        CFRelease(gpu.all_channels);
        gpu.all_channels = NULL;
    }
}

static bool macos_gpu_ioreport_open_subscription(void)
{
    macos_gpu_ioreport_close_subscription();

    gpu.all_channels = gpu.io.copy_all_channels(0, 0);
    if (!gpu.all_channels)
        return false;

    CFTypeRef channels_obj = CFDictionaryGetValue(gpu.all_channels, CFSTR("IOReportChannels"));
    if (!channels_obj || CFGetTypeID(channels_obj) != CFArrayGetTypeID())
        goto failed;

    CFArrayRef all_channels_array = (CFArrayRef)channels_obj;
    CFIndex count = CFArrayGetCount(all_channels_array);
    gpu.channels = CFDictionaryCreateMutableCopy(kCFAllocatorDefault, CFDictionaryGetCount(gpu.all_channels), gpu.all_channels);
    gpu.selected_channels = CFArrayCreateMutable(kCFAllocatorDefault, count, &kCFTypeArrayCallBacks);
    if (!gpu.channels || !gpu.selected_channels)
        goto failed;

    gpu.ioreport_power_available = false;
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef item = (CFDictionaryRef)CFArrayGetValueAtIndex(all_channels_array, i);
        if (!item || CFGetTypeID(item) != CFDictionaryGetTypeID())
            continue;

        char group[128], subgroup[128], channel[128], unit[32];
        if (!macos_gpu_ioreport_item_strings(
                item, group, sizeof(group), subgroup, sizeof(subgroup), channel, sizeof(channel), unit, sizeof(unit)))
            continue;

        if (macos_gpu_ioreport_channel_selected(group, subgroup, channel)) {
            CFArrayAppendValue(gpu.selected_channels, item);
            if (!strcmp(group, "Energy Model") && (!strcmp(channel, "GPU Energy") || !strcmp(channel, "GPU")))
                gpu.ioreport_power_source_available = true;
        }
    }

    if (CFArrayGetCount(gpu.selected_channels) == 0)
        goto failed;

    CFDictionarySetValue(gpu.channels, CFSTR("IOReportChannels"), gpu.selected_channels);
    gpu.subscribed_channels = NULL;
    gpu.subscription = gpu.io.create_subscription(NULL, gpu.channels, &gpu.subscribed_channels, 0, NULL);
    if (!gpu.subscription)
        goto failed;

    return true;

failed:
    macos_gpu_ioreport_close_subscription();
    return false;
}

static bool macos_gpu_open_smc(void)
{
    macos_smc_close(&gpu.smc_connection);
    gpu.smc_gpu_keys_count = 0;

    if (!macos_smc_open(&gpu.smc_connection))
        return false;

    uint32_t count = 0;
    if (!macos_smc_key_count(gpu.smc_connection, &count)) {
        macos_smc_close(&gpu.smc_connection);
        return false;
    }

    for (uint32_t i = 0; i < count && gpu.smc_gpu_keys_count < MACOS_GPU_MAX_SMC_KEYS; i++) {
        char key[MACOS_SMC_KEY_LEN + 1];
        if (!macos_smc_key_by_index(gpu.smc_connection, i, key))
            continue;

        if (strncmp(key, "Tg", 2) != 0 && strncmp(key, "TG", 2) != 0)
            continue;

        struct macos_smc_key_info info;
        if (!macos_smc_read_key_info(gpu.smc_connection, key, &info))
            continue;

        struct macos_smc_value value;
        NETDATA_DOUBLE ignored;
        if (!macos_smc_read_key_with_info(gpu.smc_connection, key, &info, &value) ||
            !macos_smc_decode_temperature(&value, &ignored))
            continue;

        snprintfz(gpu.smc_gpu_keys[gpu.smc_gpu_keys_count].key, sizeof(gpu.smc_gpu_keys[gpu.smc_gpu_keys_count].key), "%s", key);
        gpu.smc_gpu_keys[gpu.smc_gpu_keys_count].info = info;
        gpu.smc_gpu_keys_count++;
    }

    if (gpu.smc_gpu_keys_count == 0) {
        macos_smc_close(&gpu.smc_connection);
        return false;
    }

    return true;
}

static void macos_gpu_reset_smc_temperature_source(bool retry_next_smc_sample)
{
    macos_smc_close(&gpu.smc_connection);
    gpu.smc_gpu_keys_count = 0;

    if (retry_next_smc_sample)
        gpu.last_smc_temperature_discovery_ut = 0;
}

static bool macos_gpu_read_hid_temperature(NETDATA_DOUBLE *temperature_c)
{
    CFArrayRef services = macos_iohid_client_copy_services(&gpu.hid_client);
    if (!services)
        return false;

    NETDATA_DOUBLE sum = 0.0;
    size_t values = 0;
    CFIndex count = CFArrayGetCount(services);
    for (CFIndex i = 0; i < count; i++) {
        IOHIDServiceClientRef service = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (!service)
            continue;

        CFTypeRef product = macos_iohid_service_copy_property(service, CFSTR("Product"));
        char name[128] = "";
        if (product) {
            if (CFGetTypeID(product) == CFStringGetTypeID())
                macos_gpu_cfstring_to_cstr((CFStringRef)product, name, sizeof(name));
            CFRelease(product);
        }
        if (!macos_gpu_is_hid_temperature_sensor_name(name))
            continue;

        NETDATA_DOUBLE temp;
        if (!macos_iohid_service_copy_event_float(
                service,
                MACOS_GPU_HID_EVENT_TYPE_TEMPERATURE,
                MACOS_GPU_HID_EVENT_TYPE_TEMPERATURE << 16,
                &temp))
            continue;
        if (!isfinite(temp) || temp < -40.0 || temp > 150.0)
            continue;

        sum += temp;
        values++;
    }

    CFRelease(services);

    if (!values)
        return false;

    *temperature_c = sum / (NETDATA_DOUBLE)values;
    return true;
}

static bool macos_gpu_init_hid(void)
{
    if (!macos_iohid_client_set_matching(
            &gpu.hid_client,
            MACOS_GPU_HID_PAGE_APPLE_VENDOR,
            MACOS_GPU_HID_USAGE_TEMPERATURE_SENSOR))
        return false;

    NETDATA_DOUBLE probe;
    gpu.temperature_available = macos_gpu_read_hid_temperature(&probe);
    if (gpu.temperature_available)
        gpu.active_temperature_source = MACOS_GPU_TEMPERATURE_SOURCE_IOHID;
    return gpu.temperature_available;
}

static bool macos_gpu_read_smc_temperature(NETDATA_DOUBLE *temperature_c)
{
    if (!gpu.smc_connection || !gpu.smc_gpu_keys_count)
        return false;

    NETDATA_DOUBLE sum = 0.0;
    size_t values = 0;
    for (size_t i = 0; i < gpu.smc_gpu_keys_count; i++) {
        struct macos_smc_value value;
        NETDATA_DOUBLE temp;
        if (!macos_smc_read_key_with_info(
                gpu.smc_connection,
                gpu.smc_gpu_keys[i].key,
                &gpu.smc_gpu_keys[i].info,
                &value) ||
            !macos_smc_decode_temperature(&value, &temp))
            continue;
        sum += temp;
        values++;
    }

    if (!values)
        return false;

    *temperature_c = sum / (NETDATA_DOUBLE)values;
    return true;
}

static bool macos_gpu_ensure_smc_temperature_source(usec_t now_ut, bool force_discovery)
{
    if (gpu.smc_connection && gpu.smc_gpu_keys_count)
        return true;

    if (!force_discovery &&
        gpu.last_smc_temperature_discovery_ut &&
        now_ut - gpu.last_smc_temperature_discovery_ut <
            (usec_t)MACOS_GPU_SMC_TEMPERATURE_DISCOVERY_EVERY * USEC_PER_SEC)
        return false;

    gpu.last_smc_temperature_discovery_ut = now_ut;
    return macos_gpu_open_smc();
}

static enum macos_gpu_temperature_read_status macos_gpu_read_temperature(
    NETDATA_DOUBLE *temperature_c,
    enum macos_gpu_temperature_source *source)
{
    if (macos_gpu_read_hid_temperature(temperature_c)) {
        *source = MACOS_GPU_TEMPERATURE_SOURCE_IOHID;
        gpu.hid_temperature_consecutive_failures = 0;
        gpu.smc_temperature_consecutive_failures = 0;
        return MACOS_GPU_TEMPERATURE_READ_OK;
    }

    usec_t now_ut = now_monotonic_usec();
    bool force_smc_discovery = gpu.temperature_available &&
                               gpu.active_temperature_source == MACOS_GPU_TEMPERATURE_SOURCE_IOHID;
    if (!force_smc_discovery &&
        gpu.last_smc_temperature_read_ut &&
        now_ut - gpu.last_smc_temperature_read_ut <
            (usec_t)gpu.smc_temperature_sample_every_s * USEC_PER_SEC)
        return MACOS_GPU_TEMPERATURE_READ_SKIPPED;

    gpu.last_smc_temperature_read_ut = now_ut;
    if (!macos_gpu_ensure_smc_temperature_source(now_ut, force_smc_discovery)) {
        if (gpu.active_temperature_source == MACOS_GPU_TEMPERATURE_SOURCE_IOHID &&
            gpu.temperature_available &&
            ++gpu.hid_temperature_consecutive_failures < MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE)
            return MACOS_GPU_TEMPERATURE_READ_RETRYING;

        if (gpu.active_temperature_source == MACOS_GPU_TEMPERATURE_SOURCE_SMC &&
            gpu.temperature_available &&
            ++gpu.smc_temperature_consecutive_failures < MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE)
            return MACOS_GPU_TEMPERATURE_READ_RETRYING;

        return MACOS_GPU_TEMPERATURE_READ_FAILED;
    }

    if (macos_gpu_read_smc_temperature(temperature_c)) {
        *source = MACOS_GPU_TEMPERATURE_SOURCE_SMC;
        gpu.hid_temperature_consecutive_failures = 0;
        gpu.smc_temperature_consecutive_failures = 0;
        return MACOS_GPU_TEMPERATURE_READ_OK;
    }

    if (gpu.active_temperature_source == MACOS_GPU_TEMPERATURE_SOURCE_IOHID &&
        gpu.temperature_available &&
        ++gpu.hid_temperature_consecutive_failures < MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE)
        return MACOS_GPU_TEMPERATURE_READ_RETRYING;

    if (gpu.active_temperature_source == MACOS_GPU_TEMPERATURE_SOURCE_SMC &&
        gpu.temperature_available &&
        ++gpu.smc_temperature_consecutive_failures < MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE)
        return MACOS_GPU_TEMPERATURE_READ_RETRYING;

    macos_gpu_reset_smc_temperature_source(true);
    return MACOS_GPU_TEMPERATURE_READ_FAILED;
}

static bool macos_gpu_process_residency(CFDictionaryRef item, struct macos_gpu_metrics *metrics)
{
    int32_t count = gpu.io.state_get_count(item);
    if (count <= 0 || (size_t)count < gpu.gpu_freqs_count)
        return false;

    int64_t *residencies = callocz((size_t)count, sizeof(*residencies));
    NETDATA_DOUBLE total = 0.0;
    int offset = -1;

    for (int32_t i = 0; i < count; i++) {
        int64_t residency = gpu.io.state_get_residency(item, i);
        residencies[i] = residency > 0 ? residency : 0;
        total += (NETDATA_DOUBLE)residencies[i];

        char state_name[64] = "";
        macos_gpu_ioreport_string(gpu.io.state_get_name_for_index(item, i), state_name, sizeof(state_name));
        if (offset == -1 && !macos_gpu_ioreport_state_is_inactive(state_name))
            offset = i;
    }

    if (offset < 0 || total <= 0.0 || (size_t)(count - offset) < gpu.gpu_freqs_count) {
        freez(residencies);
        return false;
    }

    NETDATA_DOUBLE active = 0.0;
    for (int32_t i = offset; i < count; i++)
        active += (NETDATA_DOUBLE)residencies[i];

    if (gpu.gpu_freqs_count <= MACOS_GPU_MAX_DVFS_STATES) {
        metrics->has_performance_state_residency = true;
        metrics->performance_state_count = gpu.gpu_freqs_count;
        for (size_t i = 0; i < gpu.gpu_freqs_count; i++) {
            metrics->performance_state_residency_perc[i] =
                active > 0.0 ? ((NETDATA_DOUBLE)residencies[offset + (int)i] / active) * 100.0 : 0.0;
        }
    }

    NETDATA_DOUBLE average_mhz = 0.0;
    if (active > 0.0) {
        for (size_t i = 0; i < gpu.gpu_freqs_count; i++)
            average_mhz += ((NETDATA_DOUBLE)residencies[offset + (int)i] / active) * gpu.gpu_freqs_mhz[i];
    }

    metrics->has_active_residency = true;
    metrics->active_residency_perc = active > 0.0 ? (active / total) * 100.0 : 0.0;
    metrics->has_frequency = true;
    metrics->frequency_mhz = average_mhz;

    freez(residencies);
    return true;
}

static bool macos_gpu_process_power(CFDictionaryRef item, const char *unit, usec_t elapsed_ut, NETDATA_DOUBLE *power_w)
{
    int64_t raw = gpu.io.simple_get_integer_value(item, 0);
    if (raw < 0 || elapsed_ut == 0)
        return false;

    NETDATA_DOUBLE factor;
    if (!strcmp(unit, "mJ"))
        factor = 1e3;
    else if (!strcmp(unit, "uJ"))
        factor = 1e6;
    else if (!strcmp(unit, "nJ"))
        factor = 1e9;
    else
        return false;

    NETDATA_DOUBLE seconds = (NETDATA_DOUBLE)elapsed_ut / (NETDATA_DOUBLE)USEC_PER_SEC;
    if (seconds <= 0.0)
        return false;

    *power_w = ((NETDATA_DOUBLE)raw / seconds) / factor;
    return true;
}

static bool macos_gpu_process_ioreport_delta(CFDictionaryRef delta, usec_t elapsed_ut, struct macos_gpu_metrics *metrics)
{
    CFTypeRef channels_obj = CFDictionaryGetValue(delta, CFSTR("IOReportChannels"));
    if (!channels_obj || CFGetTypeID(channels_obj) != CFArrayGetTypeID())
        return false;

    CFArrayRef channels = (CFArrayRef)channels_obj;
    CFIndex count = CFArrayGetCount(channels);
    bool saw_gpu = false;
    bool has_canonical_power = false;
    bool has_alias_power = false;
    NETDATA_DOUBLE canonical_power_w = 0.0;
    NETDATA_DOUBLE alias_power_w = 0.0;
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef item = (CFDictionaryRef)CFArrayGetValueAtIndex(channels, i);
        if (!item || CFGetTypeID(item) != CFDictionaryGetTypeID())
            continue;

        char group[128], subgroup[128], channel[128], unit[32];
        if (!macos_gpu_ioreport_item_strings(
                item, group, sizeof(group), subgroup, sizeof(subgroup), channel, sizeof(channel), unit, sizeof(unit)))
            continue;

        if (!strcmp(group, "GPU Stats") && !strcmp(subgroup, "GPU Performance States") && !strcmp(channel, "GPUPH")) {
            if (macos_gpu_process_residency(item, metrics)) {
                saw_gpu = true;
                gpu.logged_malformed_ioreport = false;
            } else if (!gpu.logged_malformed_ioreport) {
                collector_error("MACOS: IOReport GPU residency sample is malformed; GPU utilization/frequency will resume when valid data is available");
                gpu.logged_malformed_ioreport = true;
            }
            continue;
        }

        if (!strcmp(group, "Energy Model") && !strcmp(channel, "GPU Energy")) {
            NETDATA_DOUBLE power_w;
            if (macos_gpu_process_power(item, unit, elapsed_ut, &power_w)) {
                has_canonical_power = true;
                canonical_power_w += power_w;
            }
            continue;
        }

        if (!strcmp(group, "Energy Model") && !strcmp(channel, "GPU")) {
            NETDATA_DOUBLE power_w;
            if (macos_gpu_process_power(item, unit, elapsed_ut, &power_w)) {
                has_alias_power = true;
                alias_power_w += power_w;
            }
        }
    }

    if (has_canonical_power) {
        metrics->has_power = true;
        metrics->power_w = canonical_power_w;
    } else if (has_alias_power) {
        metrics->has_power = true;
        metrics->power_w = alias_power_w;
    }

    return saw_gpu || metrics->has_power;
}

static bool macos_gpu_collect_ioreport(struct macos_gpu_metrics *metrics)
{
    usec_t now_ut = now_monotonic_usec();

    if (!gpu.subscription) {
        if (gpu.last_ioreport_open_attempt_ut &&
            now_ut - gpu.last_ioreport_open_attempt_ut <
                (usec_t)MACOS_GPU_IOREPORT_RETRY_EVERY_SEC * USEC_PER_SEC)
            return false;

        gpu.last_ioreport_open_attempt_ut = now_ut;
        if (!macos_gpu_ioreport_open_subscription())
            return false;
    }

    CFDictionaryRef current = gpu.io.create_samples(gpu.subscription, gpu.channels, NULL);
    if (!current)
        return false;

    if (!gpu.previous_sample) {
        gpu.previous_sample = current;
        gpu.previous_sample_ut = now_ut;
        return true;
    }

    usec_t elapsed_ut = now_ut - gpu.previous_sample_ut;
    if (elapsed_ut > (usec_t)MACOS_GPU_SAMPLE_STALE_AFTER_SEC * USEC_PER_SEC) {
        CFRelease(gpu.previous_sample);
        gpu.previous_sample = current;
        gpu.previous_sample_ut = now_ut;
        return true;
    }

    CFDictionaryRef delta = gpu.io.create_samples_delta(gpu.previous_sample, current, NULL);
    CFRelease(gpu.previous_sample);
    gpu.previous_sample = current;
    gpu.previous_sample_ut = now_ut;

    if (!delta)
        return false;

    bool ok = macos_gpu_process_ioreport_delta(delta, elapsed_ut, metrics);
    CFRelease(delta);
    return ok;
}

static void macos_gpu_update_gpu_charts(const struct macos_gpu_metrics *metrics, int update_every)
{
    if (metrics->has_active_residency) {
        if (!gpu.st_active_residency) {
            gpu.st_active_residency = rrdset_create_localhost(
                "macos",
                "gpu_utilization",
                NULL,
                "gpu",
                "macos.gpu_utilization",
                "GPU Utilization",
                "percentage",
                "macos.plugin",
                "gpu",
                NETDATA_CHART_PRIO_SENSORS - 20,
                update_every,
                RRDSET_TYPE_LINE);
            rrdlabels_add(gpu.st_active_residency->rrdlabels, "source", "ioreport", RRDLABEL_SRC_AUTO);
            gpu.rd_active_residency =
                rrddim_add(gpu.st_active_residency, "utilization", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(gpu.st_active_residency, gpu.rd_active_residency, (collected_number)llround(metrics->active_residency_perc * 1000.0));
        rrdset_done(gpu.st_active_residency);
    }

    if (metrics->has_performance_state_residency && metrics->performance_state_count > 0) {
        size_t state_count = metrics->performance_state_count;
        if (state_count > MACOS_GPU_MAX_DVFS_STATES)
            state_count = MACOS_GPU_MAX_DVFS_STATES;

        if (!gpu.st_performance_state_residency) {
            gpu.st_performance_state_residency = rrdset_create_localhost(
                "macos",
                "gpu_performance_state_residency",
                NULL,
                "gpu",
                "macos.gpu_performance_state_residency",
                "GPU Performance State Residency",
                "percentage",
                "macos.plugin",
                "gpu",
                NETDATA_CHART_PRIO_SENSORS - 17,
                update_every,
                RRDSET_TYPE_STACKED);
            rrdlabels_add(
                gpu.st_performance_state_residency->rrdlabels,
                "source",
                "ioreport",
                RRDLABEL_SRC_AUTO);
        }

        for (size_t i = 0; i < state_count; i++) {
            if (!gpu.rd_performance_state_residency[i]) {
                char dim_name[64];
                macos_gpu_performance_state_dim_name(i, dim_name, sizeof(dim_name));
                gpu.rd_performance_state_residency[i] =
                    rrddim_add(
                        gpu.st_performance_state_residency,
                        dim_name,
                        NULL,
                        1,
                        1000,
                        RRD_ALGORITHM_ABSOLUTE);
            }
            rrddim_set_by_pointer(
                gpu.st_performance_state_residency,
                gpu.rd_performance_state_residency[i],
                (collected_number)llround(metrics->performance_state_residency_perc[i] * 1000.0));
        }

        for (size_t i = state_count; i < MACOS_GPU_MAX_DVFS_STATES; i++) {
            if (gpu.rd_performance_state_residency[i])
                rrddim_set_by_pointer(
                    gpu.st_performance_state_residency,
                    gpu.rd_performance_state_residency[i],
                    0);
        }

        rrdset_done(gpu.st_performance_state_residency);
    }

    if (metrics->has_frequency) {
        if (!gpu.st_frequency) {
            gpu.st_frequency = rrdset_create_localhost(
                "macos",
                "gpu_clock_freq",
                NULL,
                "gpu",
                "macos.gpu_clock_freq",
                "GPU Clock Frequency",
                "MHz",
                "macos.plugin",
                "gpu",
                NETDATA_CHART_PRIO_SENSORS - 19,
                update_every,
                RRDSET_TYPE_LINE);
            rrdlabels_add(gpu.st_frequency->rrdlabels, "source", "ioreport", RRDLABEL_SRC_AUTO);
            gpu.rd_frequency = rrddim_add(gpu.st_frequency, "frequency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(gpu.st_frequency, gpu.rd_frequency, (collected_number)llround(metrics->frequency_mhz * 1000.0));
        rrdset_done(gpu.st_frequency);
    }

    if (metrics->has_power) {
        macos_powermetrics_release_gpu_power_fallback();

        if (!gpu.st_power) {
            gpu.st_power = rrdset_create_localhost(
                "macos",
                "gpu_power_draw",
                NULL,
                "gpu",
                "macos.gpu_power_draw",
                "GPU Power Draw",
                "W",
                "macos.plugin",
                "gpu",
                NETDATA_CHART_PRIO_SENSORS - 18,
                update_every,
                RRDSET_TYPE_LINE);
            gpu.rd_power = rrddim_add(gpu.st_power, "power_draw", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        rrdlabels_add(gpu.st_power->rrdlabels, "source", "ioreport", RRDLABEL_SRC_AUTO);
        rrddim_set_by_pointer(gpu.st_power, gpu.rd_power, (collected_number)llround(metrics->power_w * 1000.0));
        rrdset_done(gpu.st_power);
    }
}

static void macos_gpu_update_temperature(const struct macos_gpu_metrics *metrics, int update_every)
{
    if (!metrics->has_temperature)
        return;

    macos_powermetrics_release_gpu_temperature_fallback();

    int chart_update_every = update_every;
    if (metrics->temperature_source == MACOS_GPU_TEMPERATURE_SOURCE_SMC &&
        gpu.smc_temperature_sample_every_s > chart_update_every)
        chart_update_every = gpu.smc_temperature_sample_every_s;

    if (gpu.st_temperature &&
        gpu.temperature_chart_update_every &&
        gpu.temperature_chart_update_every != chart_update_every)
        macos_gpu_temperature_chart_obsolete();

    if (!gpu.st_temperature) {
        gpu.st_temperature = rrdset_create_localhost(
            "macos",
            "gpu_temperature",
            NULL,
            "gpu",
            "macos.gpu_temperature",
            "GPU Temperature",
            "degrees Celsius",
            "macos.plugin",
            "gpu",
            NETDATA_CHART_PRIO_SENSORS - 16,
            chart_update_every,
            RRDSET_TYPE_LINE);
        gpu.temperature_chart_update_every = chart_update_every;
        gpu.rd_temperature =
            rrddim_add(gpu.st_temperature, "temperature", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdlabels_add(
        gpu.st_temperature->rrdlabels,
        "source",
        macos_gpu_temperature_source_label(metrics->temperature_source),
        RRDLABEL_SRC_AUTO);
    rrddim_set_by_pointer(
        gpu.st_temperature,
        gpu.rd_temperature,
        (collected_number)llround(metrics->temperature_c * 1000.0));
    rrdset_done(gpu.st_temperature);
}

static bool macos_gpu_init(void)
{
    if (gpu.initialized)
        return !gpu.permanent_failure;

    if (!macos_gpu_load_ioreport()) {
        collector_error("MACOS: cannot load private IOReport framework; Apple Silicon GPU monitoring is disabled");
        gpu.permanent_failure = true;
        gpu.initialized = true;
        return false;
    }

    if (!macos_gpu_read_pmgr_dvfs()) {
        collector_error("MACOS: Apple Silicon GPU DVFS table not found in IORegistry; GPU monitoring is disabled on this hardware");
        gpu.permanent_failure = true;
        gpu.initialized = true;
        return false;
    }

    if (!macos_gpu_init_hid())
        macos_gpu_open_smc();

    gpu.initialized = true;
    return true;
}

bool macos_gpu_temperature_available(void)
{
    return gpu.temperature_available;
}

bool macos_gpu_power_available(void)
{
    return gpu.initialized && !gpu.permanent_failure && gpu.ioreport_handle && gpu.gpu_freqs_count > 0 &&
           (gpu.ioreport_power_available ||
            (gpu.ioreport_power_source_available &&
             gpu.ioreport_power_consecutive_misses < MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE));
}

bool macos_gpu_power_source_available(void)
{
    return gpu.initialized && !gpu.permanent_failure && gpu.ioreport_handle && gpu.gpu_freqs_count > 0 &&
           gpu.ioreport_power_source_available;
}

int do_macos_gpu(int update_every, usec_t dt __maybe_unused)
{
    static int enabled = -1;
    if (unlikely(enabled == -1)) {
        enabled = inicfg_get_boolean(&netdata_config, "plugin:macos:gpu", "enabled", 1);
        if (!enabled)
            return 1;
    }

    gpu.smc_temperature_sample_every_s = (int)inicfg_get_duration_seconds(
        &netdata_config,
        "plugin:macos:gpu",
        "SMC temperature sample every",
        MACOS_GPU_DEFAULT_SMC_TEMPERATURE_SAMPLE_EVERY);
    if (gpu.smc_temperature_sample_every_s < 1)
        gpu.smc_temperature_sample_every_s = 1;

    if (!macos_gpu_init())
        return gpu.permanent_failure ? 1 : 0;

    struct macos_gpu_metrics metrics = {0};
    if (!macos_gpu_collect_ioreport(&metrics)) {
        macos_gpu_ioreport_close_subscription();
        macos_gpu_power_chart_obsolete();
        gpu.ioreport_power_available = false;
        if (++gpu.ioreport_consecutive_failures >= MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE)
            macos_gpu_ioreport_data_charts_obsolete();
        if (!gpu.logged_sample_error) {
            collector_error("MACOS: cannot collect IOReport GPU sample; GPU charts will resume when IOReport sampling recovers");
            gpu.logged_sample_error = true;
        }
    } else {
        gpu.logged_sample_error = false;
        gpu.ioreport_consecutive_failures = 0;
        if (metrics.has_power) {
            gpu.ioreport_power_consecutive_misses = 0;
            gpu.ioreport_power_available = true;
        } else if (++gpu.ioreport_power_consecutive_misses >= MACOS_GPU_MISSING_CYCLES_BEFORE_OBSOLETE) {
            macos_gpu_power_chart_obsolete();
            gpu.ioreport_power_available = false;
        }
    }

    NETDATA_DOUBLE temp;
    enum macos_gpu_temperature_source temperature_source = MACOS_GPU_TEMPERATURE_SOURCE_NONE;
    enum macos_gpu_temperature_read_status temperature_status =
        macos_gpu_read_temperature(&temp, &temperature_source);
    if (temperature_status == MACOS_GPU_TEMPERATURE_READ_OK) {
        gpu.temperature_available = true;
        gpu.logged_temperature_error = false;
        metrics.has_temperature = true;
        metrics.temperature_c = temp;
        metrics.temperature_source = temperature_source;
        gpu.active_temperature_source = temperature_source;
    } else if (temperature_status == MACOS_GPU_TEMPERATURE_READ_FAILED &&
               gpu.temperature_available && !gpu.logged_temperature_error) {
        collector_error("MACOS: cannot read GPU temperature through SMC/IOHID; GPU temperature chart will resume when sensors recover");
        gpu.logged_temperature_error = true;
        gpu.temperature_available = false;
        gpu.active_temperature_source = MACOS_GPU_TEMPERATURE_SOURCE_NONE;
        macos_gpu_temperature_chart_obsolete();
    } else if (temperature_status == MACOS_GPU_TEMPERATURE_READ_SKIPPED ||
               temperature_status == MACOS_GPU_TEMPERATURE_READ_RETRYING) {
        // SMC temperature is intentionally sampled at a slower cadence; the
        // chart update_every is raised to the same cadence when SMC is active,
        // and short SMC read glitches retain the chart until the source fails repeatedly.
    }

    macos_gpu_update_gpu_charts(&metrics, update_every);
    macos_gpu_update_temperature(&metrics, update_every);

    return 0;
}

void macos_gpu_cleanup(void)
{
    macos_gpu_ioreport_close_subscription();

    macos_smc_close(&gpu.smc_connection);
    macos_iohid_client_cleanup(&gpu.hid_client);
    macos_gpu_unload_ioreport();

    freez(gpu.gpu_freqs_mhz);
    gpu.gpu_freqs_count = 0;
    gpu.last_ioreport_open_attempt_ut = 0;
    gpu.last_smc_temperature_read_ut = 0;
    gpu.last_smc_temperature_discovery_ut = 0;
    gpu.ioreport_consecutive_failures = 0;
    gpu.ioreport_power_consecutive_misses = 0;
    gpu.hid_temperature_consecutive_failures = 0;
    gpu.smc_temperature_consecutive_failures = 0;
    gpu.smc_temperature_sample_every_s = 0;
    gpu.temperature_chart_update_every = 0;
    gpu.active_temperature_source = MACOS_GPU_TEMPERATURE_SOURCE_NONE;
    macos_gpu_mark_charts_obsolete();
    gpu.initialized = false;
    gpu.permanent_failure = false;
    gpu.temperature_available = false;
    gpu.ioreport_power_available = false;
    gpu.ioreport_power_source_available = false;
}
