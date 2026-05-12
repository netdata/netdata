// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOCFPlugIn.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/storage/nvme/NVMeSMARTLibExternal.h>
#include <ctype.h>
#include <libkern/OSByteOrder.h>
#include <stdint.h>
#include <string.h>
#include <strings.h>

#define MACOS_NVME_DEFAULT_SAMPLE_EVERY 10
#define MACOS_NVME_DEFAULT_DISCOVERY_EVERY 300
#define MACOS_NVME_MAX_DEVICES 32
#define MACOS_NVME_NAME_MAX 32
#define MACOS_NVME_MODEL_MAX 80

enum macos_nvme_chart_priority {
    MACOS_NVME_PRIO_BASE = 2050,
    MACOS_NVME_PRIO_ENDURANCE,
    MACOS_NVME_PRIO_AVAILABLE_SPARE,
    MACOS_NVME_PRIO_TEMPERATURE,
    MACOS_NVME_PRIO_TRANSFERRED_DATA,
    MACOS_NVME_PRIO_POWER_CYCLES,
    MACOS_NVME_PRIO_POWER_ON_TIME,
    MACOS_NVME_PRIO_UNSAFE_SHUTDOWNS,
    MACOS_NVME_PRIO_CRITICAL_WARNINGS,
    MACOS_NVME_PRIO_MEDIA_ERRORS,
    MACOS_NVME_PRIO_ERROR_LOG,
};

struct macos_nvme_metrics {
    collected_number percentage_used;
    collected_number available_spare;
    collected_number temperature_c;
    collected_number data_units_read_bytes;
    collected_number data_units_written_bytes;
    collected_number power_cycles;
    collected_number power_on_time_s;
    collected_number unsafe_shutdowns;
    collected_number media_errors;
    collected_number error_log_entries;
    collected_number critical_warning_available_spare;
    collected_number critical_warning_temperature;
    collected_number critical_warning_reliability;
    collected_number critical_warning_read_only;
    collected_number critical_warning_volatile_memory_backup;
    collected_number critical_warning_persistent_memory;
};

struct macos_nvme_device {
    uint64_t registry_id;
    io_service_t service;
    char name[MACOS_NVME_NAME_MAX + 1];
    char model[MACOS_NVME_MODEL_MAX + 1];
    bool seen;

    RRDSET *st_endurance;
    RRDDIM *rd_percentage_used;

    RRDSET *st_available_spare;
    RRDDIM *rd_available_spare;

    RRDSET *st_temperature;
    RRDDIM *rd_temperature;

    RRDSET *st_transferred_data;
    RRDDIM *rd_data_units_read;
    RRDDIM *rd_data_units_written;

    RRDSET *st_power_cycles;
    RRDDIM *rd_power_cycles;

    RRDSET *st_power_on_time;
    RRDDIM *rd_power_on_time;

    RRDSET *st_unsafe_shutdowns;
    RRDDIM *rd_unsafe_shutdowns;

    RRDSET *st_critical_warnings;
    RRDDIM *rd_critical_warning_available_spare;
    RRDDIM *rd_critical_warning_temperature;
    RRDDIM *rd_critical_warning_reliability;
    RRDDIM *rd_critical_warning_read_only;
    RRDDIM *rd_critical_warning_volatile_memory_backup;
    RRDDIM *rd_critical_warning_persistent_memory;

    RRDSET *st_media_errors;
    RRDDIM *rd_media_errors;

    RRDSET *st_error_log;
    RRDDIM *rd_error_log_entries;

    struct macos_nvme_device *next;
};

static struct macos_nvme_device *nvme_devices_root = NULL;
static unsigned nvme_next_device_id = 0;
static bool nvme_logged_registry_error = false;
static bool nvme_logged_read_error = false;

static collected_number macos_nvme_le128_to_number(const uint64_t value[2], uint64_t multiplier)
{
    uint64_t lo = OSSwapLittleToHostInt64(value[0]);
    uint64_t hi = OSSwapLittleToHostInt64(value[1]);

    if (hi || (multiplier && lo > (uint64_t)INT64_MAX / multiplier))
        return (collected_number)INT64_MAX;

    return (collected_number)(lo * multiplier);
}

static void macos_nvme_trim_ascii_field(const uint8_t *src, size_t src_len, char *dst, size_t dst_len)
{
    if (!src || !dst || dst_len == 0)
        return;

    size_t start = 0;
    while (start < src_len && (src[start] == '\0' || isspace((unsigned char)src[start])))
        start++;

    size_t end = src_len;
    while (end > start && (src[end - 1] == '\0' || isspace((unsigned char)src[end - 1])))
        end--;

    size_t used = 0;
    for (size_t i = start; i < end && used + 1 < dst_len; i++) {
        unsigned char c = src[i];
        dst[used++] = isprint(c) ? (char)c : '_';
    }
    dst[used] = '\0';
}

static bool macos_nvme_service_is_smart_capable(io_registry_entry_t entry)
{
    CFTypeRef prop = IORegistryEntryCreateCFProperty(
        entry,
        CFSTR(kIOPropertyNVMeSMARTCapableKey),
        kCFAllocatorDefault,
        0);
    if (!prop)
        return false;

    bool capable = false;
    CFTypeID type = CFGetTypeID(prop);
    if (type == CFBooleanGetTypeID()) {
        capable = CFBooleanGetValue((CFBooleanRef)prop);
    } else if (type == CFNumberGetTypeID()) {
        int value = 0;
        capable = CFNumberGetValue((CFNumberRef)prop, kCFNumberIntType, &value) && value != 0;
    } else if (type == CFStringGetTypeID()) {
        char value[16] = "";
        capable =
            CFStringGetCString((CFStringRef)prop, value, sizeof(value), kCFStringEncodingUTF8) &&
            (!strcasecmp(value, "yes") || !strcasecmp(value, "true") || !strcmp(value, "1"));
    }

    CFRelease(prop);
    return capable;
}

static bool macos_nvme_open_interface(io_service_t service, IONVMeSMARTInterface ***smart_interface)
{
    *smart_interface = NULL;

    IOCFPlugInInterface **plugin = NULL;
    SInt32 score = 0;
    IOReturn kr = IOCreatePlugInInterfaceForService(
        service,
        kIONVMeSMARTUserClientTypeID,
        kIOCFPlugInInterfaceID,
        &plugin,
        &score);
    if (kr != kIOReturnSuccess || !plugin)
        return false;

    IONVMeSMARTInterface **smart = NULL;
    HRESULT hres = (*plugin)->QueryInterface(
        plugin,
        CFUUIDGetUUIDBytes(kIONVMeSMARTInterfaceID),
        (LPVOID *)&smart);

    (*plugin)->Release(plugin);

    if (hres != S_OK || !smart)
        return false;

    *smart_interface = smart;
    return true;
}

static void macos_nvme_close_interface(IONVMeSMARTInterface **smart)
{
    if (smart)
        (*smart)->Release(smart);
}

static bool macos_nvme_read_model(io_service_t service, char *dst, size_t dst_size)
{
    IONVMeSMARTInterface **smart = NULL;
    if (!macos_nvme_open_interface(service, &smart))
        return false;

    NVMeIdentifyControllerStruct identify;
    memset(&identify, 0, sizeof(identify));
    IOReturn kr = (*smart)->GetIdentifyData(smart, &identify, 0);
    macos_nvme_close_interface(smart);

    if (kr != kIOReturnSuccess)
        return false;

    macos_nvme_trim_ascii_field(identify.MODEL_NUMBER, sizeof(identify.MODEL_NUMBER), dst, dst_size);
    return dst && dst[0] != '\0';
}

static bool macos_nvme_read_metrics(io_service_t service, struct macos_nvme_metrics *metrics)
{
    IONVMeSMARTInterface **smart = NULL;
    if (!macos_nvme_open_interface(service, &smart))
        return false;

    NVMeSMARTData data;
    memset(&data, 0, sizeof(data));
    IOReturn kr = (*smart)->SMARTReadData(smart, &data);
    macos_nvme_close_interface(smart);

    if (kr != kIOReturnSuccess)
        return false;

    uint16_t kelvin = OSSwapLittleToHostInt16(data.TEMPERATURE);
    uint8_t warnings = data.CRITICAL_WARNING;

    metrics->percentage_used = (collected_number)data.PERCENTAGE_USED;
    metrics->available_spare = (collected_number)data.AVAILABLE_SPARE;
    metrics->temperature_c = kelvin > 273 ? (collected_number)(kelvin - 273) : 0;
    metrics->data_units_read_bytes = macos_nvme_le128_to_number(data.DATA_UNITS_READ, 1000ULL * 512ULL);
    metrics->data_units_written_bytes = macos_nvme_le128_to_number(data.DATA_UNITS_WRITTEN, 1000ULL * 512ULL);
    metrics->power_cycles = macos_nvme_le128_to_number(data.POWER_CYCLES, 1);
    metrics->power_on_time_s = macos_nvme_le128_to_number(data.POWER_ON_HOURS, 3600);
    metrics->unsafe_shutdowns = macos_nvme_le128_to_number(data.UNSAFE_SHUTDOWNS, 1);
    metrics->media_errors = macos_nvme_le128_to_number(data.MEDIA_ERRORS, 1);
    metrics->error_log_entries = macos_nvme_le128_to_number(data.NUM_ERROR_INFO_LOG_ENTRIES, 1);
    metrics->critical_warning_available_spare = (warnings & (1 << 0)) ? 1 : 0;
    metrics->critical_warning_temperature = (warnings & (1 << 1)) ? 1 : 0;
    metrics->critical_warning_reliability = (warnings & (1 << 2)) ? 1 : 0;
    metrics->critical_warning_read_only = (warnings & (1 << 3)) ? 1 : 0;
    metrics->critical_warning_volatile_memory_backup = (warnings & (1 << 4)) ? 1 : 0;
    metrics->critical_warning_persistent_memory = (warnings & (1 << 5)) ? 1 : 0;

    return true;
}

static struct macos_nvme_device *macos_nvme_find_device(uint64_t registry_id)
{
    for (struct macos_nvme_device *d = nvme_devices_root; d; d = d->next) {
        if (d->registry_id == registry_id)
            return d;
    }

    return NULL;
}

static struct macos_nvme_device *macos_nvme_add_device(uint64_t registry_id, io_service_t service)
{
    struct macos_nvme_device *d = callocz(1, sizeof(*d));

    d->registry_id = registry_id;
    d->service = service;
    snprintfz(d->name, sizeof(d->name), "nvme%u", nvme_next_device_id++);
    snprintfz(d->model, sizeof(d->model), "unknown");

    d->next = nvme_devices_root;
    nvme_devices_root = d;

    return d;
}

static void macos_nvme_mark_charts_obsolete(struct macos_nvme_device *d)
{
    if (d->st_endurance)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_endurance);
    if (d->st_available_spare)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_available_spare);
    if (d->st_temperature)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_temperature);
    if (d->st_transferred_data)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_transferred_data);
    if (d->st_power_cycles)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_power_cycles);
    if (d->st_power_on_time)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_power_on_time);
    if (d->st_unsafe_shutdowns)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_unsafe_shutdowns);
    if (d->st_critical_warnings)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_critical_warnings);
    if (d->st_media_errors)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_media_errors);
    if (d->st_error_log)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_error_log);
}

static void macos_nvme_free_device(struct macos_nvme_device *d)
{
    if (!d)
        return;

    macos_nvme_mark_charts_obsolete(d);
    if (d->service)
        IOObjectRelease(d->service);
    freez(d);
}

static void macos_nvme_prune_missing_devices(void)
{
    struct macos_nvme_device **pp = &nvme_devices_root;
    while (*pp) {
        struct macos_nvme_device *d = *pp;
        if (!d->seen) {
            *pp = d->next;
            macos_nvme_free_device(d);
        } else {
            pp = &d->next;
        }
    }
}

static unsigned macos_nvme_discover_devices(void)
{
    for (struct macos_nvme_device *d = nvme_devices_root; d; d = d->next)
        d->seen = false;

    io_iterator_t iter = IO_OBJECT_NULL;
    IOReturn kr = IORegistryCreateIterator(kIOMainPortDefault, kIOServicePlane, kIORegistryIterateRecursively, &iter);
    if (unlikely(kr != kIOReturnSuccess || iter == IO_OBJECT_NULL)) {
        if (!nvme_logged_registry_error) {
            collector_error("MACOS: cannot scan IORegistry for NVMe SMART-capable services");
            nvme_logged_registry_error = true;
        }
        return 0;
    }

    unsigned found = 0;
    io_registry_entry_t entry;
    while ((entry = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
        if (!macos_nvme_service_is_smart_capable(entry)) {
            IOObjectRelease(entry);
            continue;
        }

        uint64_t registry_id = 0;
        if (IORegistryEntryGetRegistryEntryID(entry, &registry_id) != kIOReturnSuccess || registry_id == 0) {
            IOObjectRelease(entry);
            continue;
        }

        struct macos_nvme_device *d = macos_nvme_find_device(registry_id);
        if (d) {
            if (d->service)
                IOObjectRelease(d->service);
            d->service = entry;
        } else if (found < MACOS_NVME_MAX_DEVICES) {
            d = macos_nvme_add_device(registry_id, entry);
        } else {
            IOObjectRelease(entry);
            continue;
        }

        d->seen = true;
        found++;

        char model[sizeof(d->model)] = "";
        if (macos_nvme_read_model(d->service, model, sizeof(model)))
            snprintfz(d->model, sizeof(d->model), "%s", model);
    }

    IOObjectRelease(iter);
    macos_nvme_prune_missing_devices();

    return found;
}

static void macos_nvme_add_labels(RRDSET *st, const struct macos_nvme_device *d)
{
    rrdlabels_add(st->rrdlabels, "device", d->name, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "model_number", d->model, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "source", "iokit", RRDLABEL_SRC_AUTO);
}

static RRDSET *macos_nvme_create_chart(
    const struct macos_nvme_device *d,
    const char *suffix,
    const char *family,
    const char *context,
    const char *title,
    const char *units,
    int priority,
    int update_every,
    RRDSET_TYPE chart_type)
{
    char chart_id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(chart_id, sizeof(chart_id), "device_%s_%s", d->name, suffix);

    RRDSET *st = rrdset_create_localhost(
        "nvme",
        chart_id,
        NULL,
        family,
        context,
        title,
        units,
        "macos.plugin",
        "nvme_smart",
        priority,
        update_every,
        chart_type);

    macos_nvme_add_labels(st, d);
    return st;
}

static RRDDIM *macos_nvme_add_dim(
    RRDSET *st,
    const struct macos_nvme_device *d,
    const char *suffix,
    const char *name,
    collected_number multiplier,
    collected_number divisor,
    RRD_ALGORITHM algorithm)
{
    char dim_id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(dim_id, sizeof(dim_id), "device_%s_%s", d->name, suffix);

    return rrddim_add(st, dim_id, name, multiplier, divisor, algorithm);
}

static void macos_nvme_update_charts(struct macos_nvme_device *d, const struct macos_nvme_metrics *m, int update_every)
{
    if (!d->st_endurance) {
        d->st_endurance = macos_nvme_create_chart(
            d,
            "estimated_endurance_perc",
            "endurance",
            "nvme.device_estimated_endurance_perc",
            "Estimated endurance",
            "percentage",
            MACOS_NVME_PRIO_ENDURANCE,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_percentage_used = macos_nvme_add_dim(
            d->st_endurance, d, "percentage_used", "used", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_endurance, d->rd_percentage_used, m->percentage_used);
    rrdset_done(d->st_endurance);

    if (!d->st_available_spare) {
        d->st_available_spare = macos_nvme_create_chart(
            d,
            "available_spare_perc",
            "spare",
            "nvme.device_available_spare_perc",
            "Remaining spare capacity",
            "percentage",
            MACOS_NVME_PRIO_AVAILABLE_SPARE,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_available_spare = macos_nvme_add_dim(
            d->st_available_spare, d, "available_spare", "spare", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_available_spare, d->rd_available_spare, m->available_spare);
    rrdset_done(d->st_available_spare);

    if (!d->st_temperature) {
        d->st_temperature = macos_nvme_create_chart(
            d,
            "temperature",
            "temperature",
            "nvme.device_composite_temperature",
            "Composite temperature",
            "celsius",
            MACOS_NVME_PRIO_TEMPERATURE,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_temperature = macos_nvme_add_dim(
            d->st_temperature, d, "temperature", "temperature", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_temperature, d->rd_temperature, m->temperature_c);
    rrdset_done(d->st_temperature);

    if (!d->st_transferred_data) {
        d->st_transferred_data = macos_nvme_create_chart(
            d,
            "io_transferred_count",
            "transferred data",
            "nvme.device_io_transferred_count",
            "Amount of data transferred to and from device",
            "bytes",
            MACOS_NVME_PRIO_TRANSFERRED_DATA,
            update_every,
            RRDSET_TYPE_AREA);
        d->rd_data_units_read = macos_nvme_add_dim(
            d->st_transferred_data, d, "data_units_read", "read", 1, 1, RRD_ALGORITHM_ABSOLUTE);
        d->rd_data_units_written = macos_nvme_add_dim(
            d->st_transferred_data, d, "data_units_written", "written", -1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_transferred_data, d->rd_data_units_read, m->data_units_read_bytes);
    rrddim_set_by_pointer(d->st_transferred_data, d->rd_data_units_written, m->data_units_written_bytes);
    rrdset_done(d->st_transferred_data);

    if (!d->st_power_cycles) {
        d->st_power_cycles = macos_nvme_create_chart(
            d,
            "power_cycles_count",
            "power cycles",
            "nvme.device_power_cycles_count",
            "Power cycles",
            "cycles",
            MACOS_NVME_PRIO_POWER_CYCLES,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_power_cycles = macos_nvme_add_dim(
            d->st_power_cycles, d, "power_cycles", "power", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_power_cycles, d->rd_power_cycles, m->power_cycles);
    rrdset_done(d->st_power_cycles);

    if (!d->st_power_on_time) {
        d->st_power_on_time = macos_nvme_create_chart(
            d,
            "power_on_time",
            "power-on time",
            "nvme.device_power_on_time",
            "Power-on time",
            "seconds",
            MACOS_NVME_PRIO_POWER_ON_TIME,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_power_on_time = macos_nvme_add_dim(
            d->st_power_on_time, d, "power_on_time", "power-on", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_power_on_time, d->rd_power_on_time, m->power_on_time_s);
    rrdset_done(d->st_power_on_time);

    if (!d->st_unsafe_shutdowns) {
        d->st_unsafe_shutdowns = macos_nvme_create_chart(
            d,
            "unsafe_shutdowns_count",
            "shutdowns",
            "nvme.device_unsafe_shutdowns_count",
            "Unsafe shutdowns",
            "shutdowns",
            MACOS_NVME_PRIO_UNSAFE_SHUTDOWNS,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_unsafe_shutdowns = macos_nvme_add_dim(
            d->st_unsafe_shutdowns, d, "unsafe_shutdowns", "unsafe", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(d->st_unsafe_shutdowns, d->rd_unsafe_shutdowns, m->unsafe_shutdowns);
    rrdset_done(d->st_unsafe_shutdowns);

    if (!d->st_critical_warnings) {
        d->st_critical_warnings = macos_nvme_create_chart(
            d,
            "critical_warnings_state",
            "critical warnings",
            "nvme.device_critical_warnings_state",
            "Critical warnings state",
            "state",
            MACOS_NVME_PRIO_CRITICAL_WARNINGS,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_critical_warning_available_spare = macos_nvme_add_dim(
            d->st_critical_warnings,
            d,
            "critical_warning_available_spare",
            "available_spare",
            1,
            1,
            RRD_ALGORITHM_ABSOLUTE);
        d->rd_critical_warning_temperature = macos_nvme_add_dim(
            d->st_critical_warnings,
            d,
            "critical_warning_temp_threshold",
            "temp_threshold",
            1,
            1,
            RRD_ALGORITHM_ABSOLUTE);
        d->rd_critical_warning_reliability = macos_nvme_add_dim(
            d->st_critical_warnings,
            d,
            "critical_warning_nvm_subsystem_reliability",
            "nvm_subsystem_reliability",
            1,
            1,
            RRD_ALGORITHM_ABSOLUTE);
        d->rd_critical_warning_read_only = macos_nvme_add_dim(
            d->st_critical_warnings,
            d,
            "critical_warning_read_only",
            "read_only",
            1,
            1,
            RRD_ALGORITHM_ABSOLUTE);
        d->rd_critical_warning_volatile_memory_backup = macos_nvme_add_dim(
            d->st_critical_warnings,
            d,
            "critical_warning_volatile_mem_backup_failed",
            "volatile_mem_backup_failed",
            1,
            1,
            RRD_ALGORITHM_ABSOLUTE);
        d->rd_critical_warning_persistent_memory = macos_nvme_add_dim(
            d->st_critical_warnings,
            d,
            "critical_warning_persistent_memory_read_only",
            "persistent_memory_read_only",
            1,
            1,
            RRD_ALGORITHM_ABSOLUTE);
    }
    rrddim_set_by_pointer(
        d->st_critical_warnings,
        d->rd_critical_warning_available_spare,
        m->critical_warning_available_spare);
    rrddim_set_by_pointer(
        d->st_critical_warnings,
        d->rd_critical_warning_temperature,
        m->critical_warning_temperature);
    rrddim_set_by_pointer(
        d->st_critical_warnings,
        d->rd_critical_warning_reliability,
        m->critical_warning_reliability);
    rrddim_set_by_pointer(d->st_critical_warnings, d->rd_critical_warning_read_only, m->critical_warning_read_only);
    rrddim_set_by_pointer(
        d->st_critical_warnings,
        d->rd_critical_warning_volatile_memory_backup,
        m->critical_warning_volatile_memory_backup);
    rrddim_set_by_pointer(
        d->st_critical_warnings,
        d->rd_critical_warning_persistent_memory,
        m->critical_warning_persistent_memory);
    rrdset_done(d->st_critical_warnings);

    if (!d->st_media_errors) {
        d->st_media_errors = macos_nvme_create_chart(
            d,
            "media_errors_rate",
            "media errors",
            "nvme.device_media_errors_rate",
            "Media and data integrity errors",
            "errors/s",
            MACOS_NVME_PRIO_MEDIA_ERRORS,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_media_errors = macos_nvme_add_dim(
            d->st_media_errors, d, "media_errors", "media", 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    rrddim_set_by_pointer(d->st_media_errors, d->rd_media_errors, m->media_errors);
    rrdset_done(d->st_media_errors);

    if (!d->st_error_log) {
        d->st_error_log = macos_nvme_create_chart(
            d,
            "error_log_entries_rate",
            "error log",
            "nvme.device_error_log_entries_rate",
            "Error log entries",
            "entries/s",
            MACOS_NVME_PRIO_ERROR_LOG,
            update_every,
            RRDSET_TYPE_LINE);
        d->rd_error_log_entries = macos_nvme_add_dim(
            d->st_error_log, d, "num_err_log_entries", "error_log", 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    rrddim_set_by_pointer(d->st_error_log, d->rd_error_log_entries, m->error_log_entries);
    rrdset_done(d->st_error_log);
}

int do_macos_nvme_smart(int update_every __maybe_unused, usec_t dt __maybe_unused)
{
    static int initialized = 0;
    static int sample_every_s = MACOS_NVME_DEFAULT_SAMPLE_EVERY;
    static int discovery_every_s = MACOS_NVME_DEFAULT_DISCOVERY_EVERY;
    static usec_t last_sample_ut = 0;
    static usec_t last_discovery_ut = 0;
    static unsigned consecutive_read_failures = 0;

    if (unlikely(!initialized)) {
        sample_every_s = (int)inicfg_get_duration_seconds(
            &netdata_config,
            "plugin:macos:nvme_smart",
            "sample every",
            MACOS_NVME_DEFAULT_SAMPLE_EVERY);
        if (sample_every_s < 10)
            sample_every_s = 10;

        discovery_every_s = (int)inicfg_get_duration_seconds(
            &netdata_config,
            "plugin:macos:nvme_smart",
            "discovery every",
            MACOS_NVME_DEFAULT_DISCOVERY_EVERY);
        if (discovery_every_s < sample_every_s)
            discovery_every_s = sample_every_s;

        initialized = 1;
    }

    usec_t now_ut = now_monotonic_usec();
    if (last_sample_ut && now_ut - last_sample_ut < (usec_t)sample_every_s * USEC_PER_SEC)
        return 0;
    last_sample_ut = now_ut;

    if (!last_discovery_ut || now_ut - last_discovery_ut >= (usec_t)discovery_every_s * USEC_PER_SEC) {
        macos_nvme_discover_devices();
        last_discovery_ut = now_ut;
    }

    unsigned devices = 0, collected = 0;
    for (struct macos_nvme_device *d = nvme_devices_root; d; d = d->next) {
        devices++;

        struct macos_nvme_metrics metrics = {0};
        if (!macos_nvme_read_metrics(d->service, &metrics))
            continue;

        macos_nvme_update_charts(d, &metrics, sample_every_s);
        collected++;
    }

    if (devices && !collected) {
        if (++consecutive_read_failures >= 3 && !nvme_logged_read_error) {
            collector_error(
                "MACOS: cannot read NVMe SMART data through IOKit; "
                "NVMe health charts will appear when macOS exposes readable NVMe SMART data");
            nvme_logged_read_error = true;
        }
    } else {
        consecutive_read_failures = 0;
    }

    return 0;
}

void macos_nvme_smart_cleanup(void)
{
    while (nvme_devices_root) {
        struct macos_nvme_device *d = nvme_devices_root;
        nvme_devices_root = d->next;
        macos_nvme_free_device(d);
    }

    nvme_next_device_id = 0;
}
