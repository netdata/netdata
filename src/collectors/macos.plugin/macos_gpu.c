// SPDX-License-Identifier: GPL-3.0-or-later

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
#define MACOS_GPU_SMC_KEY_LEN 4

typedef const void *IOReportSubscriptionRef;
typedef struct __IOHIDEventSystemClient *IOHIDEventSystemClientRef;
typedef struct __IOHIDServiceClient *IOHIDServiceClientRef;
typedef struct __IOHIDEvent *IOHIDEventRef;

extern IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);
extern int IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef matching);
extern CFArrayRef IOHIDEventSystemClientCopyServices(IOHIDEventSystemClientRef client);
extern CFTypeRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef key);
extern IOHIDEventRef IOHIDServiceClientCopyEvent(IOHIDServiceClientRef service, int64_t type, int32_t options, int64_t timestamp);
extern double IOHIDEventGetFloatValue(IOHIDEventRef event, int32_t field);

enum {
    MACOS_GPU_HID_PAGE_APPLE_VENDOR = 0xff00,
    MACOS_GPU_HID_USAGE_TEMPERATURE_SENSOR = 0x0005,
    MACOS_GPU_HID_EVENT_TYPE_TEMPERATURE = 15,
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

struct macos_gpu_key_data_ver {
    uint8_t major;
    uint8_t minor;
    uint8_t build;
    uint8_t reserved;
    uint16_t release;
};

struct macos_gpu_p_limit_data {
    uint16_t version;
    uint16_t length;
    uint32_t cpu_p_limit;
    uint32_t gpu_p_limit;
    uint32_t mem_p_limit;
};

struct macos_gpu_key_info {
    uint32_t data_size;
    uint32_t data_type;
    uint8_t data_attributes;
};

struct macos_gpu_key_data {
    uint32_t key;
    struct macos_gpu_key_data_ver vers;
    struct macos_gpu_p_limit_data p_limit_data;
    struct macos_gpu_key_info key_info;
    uint8_t result;
    uint8_t status;
    uint8_t data8;
    uint32_t data32;
    uint8_t bytes[32];
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
};

struct macos_gpu_state {
    bool initialized;
    bool permanent_failure;
    bool temperature_available;
    bool logged_unavailable;
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

    NETDATA_DOUBLE *gpu_freqs_mhz;
    size_t gpu_freqs_count;

    io_connect_t smc_connection;
    char smc_gpu_keys[MACOS_GPU_MAX_SMC_KEYS][MACOS_GPU_SMC_KEY_LEN + 1];
    size_t smc_gpu_keys_count;
    CFDictionaryRef hid_matching;

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

static uint32_t macos_gpu_read_le32(const uint8_t *p)
{
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

static uint32_t macos_gpu_read_be32(const uint8_t *p)
{
    return ((uint32_t)p[0] << 24) | ((uint32_t)p[1] << 16) | ((uint32_t)p[2] << 8) | (uint32_t)p[3];
}

static uint32_t macos_gpu_key_from_cstr(const char key[MACOS_GPU_SMC_KEY_LEN + 1])
{
    return ((uint32_t)(uint8_t)key[0] << 24) | ((uint32_t)(uint8_t)key[1] << 16) |
           ((uint32_t)(uint8_t)key[2] << 8) | (uint32_t)(uint8_t)key[3];
}

static void macos_gpu_key_to_cstr(uint32_t key, char dst[MACOS_GPU_SMC_KEY_LEN + 1])
{
    dst[0] = (char)((key >> 24) & 0xff);
    dst[1] = (char)((key >> 16) & 0xff);
    dst[2] = (char)((key >> 8) & 0xff);
    dst[3] = (char)(key & 0xff);
    dst[4] = '\0';
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

    char *end = start + strlen(start);
    while (end > start && isspace((unsigned char)end[-1]))
        *--end = '\0';

    if (start != dst)
        memmove(dst, start, strlen(start) + 1);

    return dst[0] != '\0';
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

    if (!strcmp(group, "Energy Model") && !strcmp(channel, "GPU Energy"))
        return true;

    return false;
}

static void macos_gpu_ioreport_close_subscription(void)
{
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

static bool macos_gpu_ioreport_open_subscription(bool permanent_on_empty)
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

    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef item = (CFDictionaryRef)CFArrayGetValueAtIndex(all_channels_array, i);
        if (!item || CFGetTypeID(item) != CFDictionaryGetTypeID())
            continue;

        char group[128], subgroup[128], channel[128], unit[32];
        if (!macos_gpu_ioreport_item_strings(
                item, group, sizeof(group), subgroup, sizeof(subgroup), channel, sizeof(channel), unit, sizeof(unit)))
            continue;

        if (macos_gpu_ioreport_channel_selected(group, subgroup, channel))
            CFArrayAppendValue(gpu.selected_channels, item);
    }

    if (CFArrayGetCount(gpu.selected_channels) == 0) {
        if (permanent_on_empty)
            gpu.permanent_failure = true;
        goto failed;
    }

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

static bool macos_gpu_smc_call(const struct macos_gpu_key_data *input, struct macos_gpu_key_data *output)
{
    size_t output_size = sizeof(*output);
    memset(output, 0, sizeof(*output));

    IOReturn kr = IOConnectCallStructMethod(
        gpu.smc_connection,
        2,
        input,
        sizeof(*input),
        output,
        &output_size);

    if (kr != kIOReturnSuccess)
        return false;

    return output->result == 0;
}

static bool macos_gpu_smc_read_key_info(uint32_t key, struct macos_gpu_key_info *info)
{
    struct macos_gpu_key_data input = {
        .key = key,
        .data8 = 9,
    };
    struct macos_gpu_key_data output;

    if (!macos_gpu_smc_call(&input, &output))
        return false;

    *info = output.key_info;
    return true;
}

static bool macos_gpu_smc_read_value(uint32_t key, const struct macos_gpu_key_info *info, struct macos_gpu_key_data *output)
{
    struct macos_gpu_key_data input = {
        .key = key,
        .key_info = *info,
        .data8 = 5,
    };

    return macos_gpu_smc_call(&input, output);
}

static bool macos_gpu_smc_read_float_key(const char key_name[MACOS_GPU_SMC_KEY_LEN + 1], NETDATA_DOUBLE *value)
{
    uint32_t key = macos_gpu_key_from_cstr(key_name);
    struct macos_gpu_key_info info;
    if (!macos_gpu_smc_read_key_info(key, &info))
        return false;

    if (info.data_size != 4 || info.data_type != macos_gpu_key_from_cstr("flt "))
        return false;

    struct macos_gpu_key_data output;
    if (!macos_gpu_smc_read_value(key, &info, &output))
        return false;

    uint32_t raw = macos_gpu_read_le32(output.bytes);
    float temp;
    memcpy(&temp, &raw, sizeof(temp));
    if (!isfinite(temp) || temp <= 0.0 || temp > 150.0)
        return false;

    *value = (NETDATA_DOUBLE)temp;
    return true;
}

static bool macos_gpu_smc_key_by_index(uint32_t index, char key_name[MACOS_GPU_SMC_KEY_LEN + 1])
{
    struct macos_gpu_key_data input = {
        .data8 = 8,
        .data32 = index,
    };
    struct macos_gpu_key_data output;

    if (!macos_gpu_smc_call(&input, &output))
        return false;

    macos_gpu_key_to_cstr(output.key, key_name);
    return true;
}

static bool macos_gpu_smc_key_count(uint32_t *count)
{
    uint32_t key = macos_gpu_key_from_cstr("#KEY");
    struct macos_gpu_key_info info;
    if (!macos_gpu_smc_read_key_info(key, &info))
        return false;

    struct macos_gpu_key_data output;
    if (!macos_gpu_smc_read_value(key, &info, &output))
        return false;

    *count = macos_gpu_read_be32(output.bytes);
    return true;
}

static bool macos_gpu_open_smc(void)
{
    io_iterator_t iter = IO_OBJECT_NULL;
    IOReturn kr = IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleSMC"), &iter);
    if (kr != kIOReturnSuccess || iter == IO_OBJECT_NULL)
        return false;

    bool opened = false;
    io_service_t service;
    while ((service = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
        io_name_t name;
        if (IORegistryEntryGetName(service, name) == kIOReturnSuccess && !strcmp(name, "AppleSMCKeysEndpoint")) {
            kr = IOServiceOpen(service, mach_task_self(), 0, &gpu.smc_connection);
            opened = kr == kIOReturnSuccess;
        }
        IOObjectRelease(service);
        if (opened)
            break;
    }
    IOObjectRelease(iter);

    if (!opened)
        return false;

    uint32_t count = 0;
    if (!macos_gpu_smc_key_count(&count)) {
        IOServiceClose(gpu.smc_connection);
        gpu.smc_connection = IO_OBJECT_NULL;
        return false;
    }

    for (uint32_t i = 0; i < count && gpu.smc_gpu_keys_count < MACOS_GPU_MAX_SMC_KEYS; i++) {
        char key[MACOS_GPU_SMC_KEY_LEN + 1];
        if (!macos_gpu_smc_key_by_index(i, key))
            continue;

        if (strncmp(key, "Tg", 2) != 0)
            continue;

        NETDATA_DOUBLE ignored;
        if (!macos_gpu_smc_read_float_key(key, &ignored))
            continue;

        snprintfz(gpu.smc_gpu_keys[gpu.smc_gpu_keys_count], sizeof(gpu.smc_gpu_keys[gpu.smc_gpu_keys_count]), "%s", key);
        gpu.smc_gpu_keys_count++;
    }

    if (gpu.smc_gpu_keys_count == 0) {
        IOServiceClose(gpu.smc_connection);
        gpu.smc_connection = IO_OBJECT_NULL;
        return false;
    }

    gpu.temperature_available = true;
    return true;
}

static CFNumberRef macos_gpu_cfnumber_int(int value)
{
    return CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &value);
}

static bool macos_gpu_read_hid_temperature(NETDATA_DOUBLE *temperature_c)
{
    if (!gpu.hid_matching)
        return false;

    IOHIDEventSystemClientRef client = IOHIDEventSystemClientCreate(kCFAllocatorDefault);
    if (!client)
        return false;

    IOHIDEventSystemClientSetMatching(client, gpu.hid_matching);
    CFArrayRef services = IOHIDEventSystemClientCopyServices(client);
    if (!services) {
        CFRelease(client);
        return false;
    }

    NETDATA_DOUBLE sum = 0.0;
    size_t values = 0;
    CFIndex count = CFArrayGetCount(services);
    for (CFIndex i = 0; i < count; i++) {
        IOHIDServiceClientRef service = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (!service)
            continue;

        CFTypeRef product = IOHIDServiceClientCopyProperty(service, CFSTR("Product"));
        char name[128] = "";
        if (product) {
            if (CFGetTypeID(product) == CFStringGetTypeID())
                macos_gpu_cfstring_to_cstr((CFStringRef)product, name, sizeof(name));
            CFRelease(product);
        }
        if (strncmp(name, "GPU MTR Temp Sensor", strlen("GPU MTR Temp Sensor")) != 0)
            continue;

        IOHIDEventRef event = IOHIDServiceClientCopyEvent(service, MACOS_GPU_HID_EVENT_TYPE_TEMPERATURE, 0, 0);
        if (!event)
            continue;

        NETDATA_DOUBLE temp = IOHIDEventGetFloatValue(event, MACOS_GPU_HID_EVENT_TYPE_TEMPERATURE << 16);
        CFRelease(event);
        if (!isfinite(temp) || temp <= 0.0 || temp > 150.0)
            continue;

        sum += temp;
        values++;
    }

    CFRelease(services);
    CFRelease(client);

    if (!values)
        return false;

    *temperature_c = sum / (NETDATA_DOUBLE)values;
    return true;
}

static bool macos_gpu_init_hid(void)
{
    CFStringRef keys[] = {CFSTR("PrimaryUsagePage"), CFSTR("PrimaryUsage")};
    CFNumberRef values[] = {
        macos_gpu_cfnumber_int(MACOS_GPU_HID_PAGE_APPLE_VENDOR),
        macos_gpu_cfnumber_int(MACOS_GPU_HID_USAGE_TEMPERATURE_SENSOR),
    };
    if (!values[0] || !values[1]) {
        if (values[0])
            CFRelease(values[0]);
        if (values[1])
            CFRelease(values[1]);
        return false;
    }

    gpu.hid_matching = CFDictionaryCreate(
        kCFAllocatorDefault,
        (const void **)keys,
        (const void **)values,
        2,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);

    CFRelease(values[0]);
    CFRelease(values[1]);

    if (!gpu.hid_matching)
        return false;

    NETDATA_DOUBLE probe;
    gpu.temperature_available = macos_gpu_read_hid_temperature(&probe);
    return gpu.temperature_available;
}

static bool macos_gpu_read_temperature(NETDATA_DOUBLE *temperature_c)
{
    if (gpu.smc_connection && gpu.smc_gpu_keys_count) {
        NETDATA_DOUBLE sum = 0.0;
        size_t values = 0;
        for (size_t i = 0; i < gpu.smc_gpu_keys_count; i++) {
            NETDATA_DOUBLE temp;
            if (!macos_gpu_smc_read_float_key(gpu.smc_gpu_keys[i], &temp))
                continue;
            sum += temp;
            values++;
        }

        if (values) {
            *temperature_c = sum / (NETDATA_DOUBLE)values;
            return true;
        }
    }

    return macos_gpu_read_hid_temperature(temperature_c);
}

static bool macos_gpu_process_residency(CFDictionaryRef item, struct macos_gpu_metrics *metrics)
{
    int32_t count = gpu.io.state_get_count(item);
    if (count <= 0 || (size_t)count <= gpu.gpu_freqs_count)
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
        if (offset == -1 && strcmp(state_name, "IDLE") != 0 && strcmp(state_name, "DOWN") != 0 && strcmp(state_name, "OFF") != 0)
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

static bool macos_gpu_process_power(CFDictionaryRef item, const char *unit, usec_t elapsed_ut, struct macos_gpu_metrics *metrics)
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

    metrics->has_power = true;
    metrics->power_w += ((NETDATA_DOUBLE)raw / seconds) / factor;
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
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef item = (CFDictionaryRef)CFArrayGetValueAtIndex(channels, i);
        if (!item || CFGetTypeID(item) != CFDictionaryGetTypeID())
            continue;

        char group[128], subgroup[128], channel[128], unit[32];
        if (!macos_gpu_ioreport_item_strings(
                item, group, sizeof(group), subgroup, sizeof(subgroup), channel, sizeof(channel), unit, sizeof(unit)))
            continue;

        if (!strcmp(group, "GPU Stats") && !strcmp(subgroup, "GPU Performance States") && !strcmp(channel, "GPUPH")) {
            if (macos_gpu_process_residency(item, metrics))
                saw_gpu = true;
            else if (!gpu.logged_malformed_ioreport) {
                collector_error("MACOS: IOReport GPU residency sample is malformed; GPU utilization/frequency will resume when valid data is available");
                gpu.logged_malformed_ioreport = true;
            }
            continue;
        }

        if (!strcmp(group, "Energy Model") && !strcmp(channel, "GPU Energy"))
            macos_gpu_process_power(item, unit, elapsed_ut, metrics);
    }

    return saw_gpu || metrics->has_power;
}

static bool macos_gpu_collect_ioreport(struct macos_gpu_metrics *metrics)
{
    if (!gpu.subscription && !macos_gpu_ioreport_open_subscription(false))
        return false;

    usec_t now_ut = now_monotonic_usec();
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
            rrdlabels_add(gpu.st_power->rrdlabels, "source", "ioreport", RRDLABEL_SRC_AUTO);
            gpu.rd_power = rrddim_add(gpu.st_power, "power_draw", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(gpu.st_power, gpu.rd_power, (collected_number)llround(metrics->power_w * 1000.0));
        rrdset_done(gpu.st_power);
    }
}

static void macos_gpu_update_temperature(const struct macos_gpu_metrics *metrics, int update_every)
{
    if (!metrics->has_temperature)
        return;

    if (!gpu.st_temperature) {
        gpu.st_temperature = rrdset_create_localhost(
            "sensors",
            "macos_gpu_die_temperature",
            NULL,
            "Temperature",
            "system.hw.sensor.temperature.input",
            "GPU Die Temperature",
            "degrees Celsius",
            "macos.plugin",
            "gpu",
            NETDATA_CHART_PRIO_SENSORS + 1,
            update_every,
            RRDSET_TYPE_LINE);
        rrdlabels_add(gpu.st_temperature->rrdlabels, "source", "iokit", RRDLABEL_SRC_AUTO);
        rrdlabels_add(gpu.st_temperature->rrdlabels, "sensor", "gpu_die", RRDLABEL_SRC_AUTO);
        gpu.rd_temperature = rrddim_add(gpu.st_temperature, "input", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(gpu.st_temperature, gpu.rd_temperature, (collected_number)llround(metrics->temperature_c * 1000.0));
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

    if (!macos_gpu_ioreport_open_subscription(true)) {
        collector_error("MACOS: IOReport GPU channels are unavailable; GPU monitoring is disabled on this hardware");
        gpu.permanent_failure = true;
        gpu.initialized = true;
        return false;
    }

    if (!macos_gpu_open_smc())
        macos_gpu_init_hid();

    gpu.initialized = true;
    return true;
}

bool macos_gpu_temperature_available(void)
{
    return gpu.temperature_available;
}

bool macos_gpu_ioreport_available(void)
{
    return gpu.initialized && !gpu.permanent_failure && gpu.ioreport_handle && gpu.gpu_freqs_count > 0;
}

int do_macos_gpu(int update_every, usec_t dt __maybe_unused)
{
    static int enabled = -1;
    if (unlikely(enabled == -1)) {
        enabled = inicfg_get_boolean(&netdata_config, "plugin:macos:gpu", "enabled", 1);
        if (!enabled)
            return 1;
    }

    if (!macos_gpu_init())
        return gpu.permanent_failure ? 1 : 0;

    struct macos_gpu_metrics metrics = {0};
    if (!macos_gpu_collect_ioreport(&metrics)) {
        macos_gpu_ioreport_close_subscription();
        if (!gpu.logged_sample_error) {
            collector_error("MACOS: cannot collect IOReport GPU sample; GPU charts will resume when IOReport sampling recovers");
            gpu.logged_sample_error = true;
        }
        return 0;
    }

    NETDATA_DOUBLE temp;
    if (macos_gpu_read_temperature(&temp)) {
        metrics.has_temperature = true;
        metrics.temperature_c = temp;
    } else if (gpu.temperature_available && !gpu.logged_temperature_error) {
        collector_error("MACOS: cannot read GPU temperature through SMC/IOHID; GPU temperature chart will resume when sensors recover");
        gpu.logged_temperature_error = true;
    }

    macos_gpu_update_gpu_charts(&metrics, update_every);
    macos_gpu_update_temperature(&metrics, update_every);

    return 0;
}

void macos_gpu_cleanup(void)
{
    macos_gpu_ioreport_close_subscription();

    if (gpu.smc_connection) {
        IOServiceClose(gpu.smc_connection);
        gpu.smc_connection = IO_OBJECT_NULL;
    }
    if (gpu.hid_matching) {
        CFRelease(gpu.hid_matching);
        gpu.hid_matching = NULL;
    }
    macos_gpu_unload_ioreport();

    freez(gpu.gpu_freqs_mhz);
    gpu.gpu_freqs_count = 0;
    macos_gpu_mark_charts_obsolete();
    gpu.initialized = false;
    gpu.permanent_failure = false;
    gpu.temperature_available = false;
}
