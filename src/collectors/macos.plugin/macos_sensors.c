// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos_smc.h"
#include "plugin_macos.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <ctype.h>
#include <math.h>
#include <stdint.h>
#include <string.h>

#define MACOS_SENSORS_DEFAULT_DISCOVERY_EVERY 300
#define MACOS_SENSORS_MAX_SMC_KEYS 8192
#define MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE 3

typedef struct __IOHIDEventSystemClient *IOHIDEventSystemClientRef;
typedef struct __IOHIDServiceClient *IOHIDServiceClientRef;
typedef struct __IOHIDEvent *IOHIDEventRef;

extern IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);
extern int IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef matching);
extern CFArrayRef IOHIDEventSystemClientCopyServices(IOHIDEventSystemClientRef client);
extern CFTypeRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef key);
extern IOHIDEventRef IOHIDServiceClientCopyEvent(
    IOHIDServiceClientRef service,
    int64_t type,
    int32_t options,
    int64_t timestamp);
extern double IOHIDEventGetFloatValue(IOHIDEventRef event, int32_t field);

enum {
    MACOS_SENSORS_HID_PAGE_APPLE_VENDOR = 0xff00,
    MACOS_SENSORS_HID_USAGE_TEMPERATURE_SENSOR = 0x0005,
    MACOS_SENSORS_HID_EVENT_TYPE_TEMPERATURE = 15,
};

enum macos_sensor_kind {
    MACOS_SENSOR_TEMPERATURE,
    MACOS_SENSOR_FAN,
    MACOS_SENSOR_VOLTAGE,
    MACOS_SENSOR_CURRENT,
    MACOS_SENSOR_POWER,
    MACOS_SENSOR_KIND_COUNT,
};

struct macos_sensor_kind_def {
    const char *name;
    const char *family;
    const char *context;
    const char *units;
    collected_number divisor;
    int priority;
};

struct macos_sensor_chart {
    char id[RRD_ID_LENGTH_MAX + 1];
    enum macos_sensor_kind kind;
    char label[128];
    char feature[128];
    char path[192];
    char driver[64];
    char subsystem[64];
    char chip_id[128];
    char source[32];
    char component[64];

    bool seen;
    unsigned missing_cycles;
    RRDSET *st;
    RRDDIM *rd;

    struct macos_sensor_chart *next;
};

struct macos_smc_sensor_candidate {
    char key[MACOS_SMC_KEY_LEN + 1];
    enum macos_sensor_kind kind;
    char label[128];
    char component[64];
    bool discovered;

    struct macos_smc_sensor_candidate *next;
};

static const struct macos_sensor_kind_def macos_sensor_defs[MACOS_SENSOR_KIND_COUNT] = {
    [MACOS_SENSOR_TEMPERATURE] = {
        .name = "temperature",
        .family = "Temperature",
        .context = "system.hw.sensor.temperature.input",
        .units = "degrees Celsius",
        .divisor = 1000,
        .priority = NETDATA_CHART_PRIO_SENSORS,
    },
    [MACOS_SENSOR_FAN] = {
        .name = "fan",
        .family = "Fan",
        .context = "system.hw.sensor.fan.input",
        .units = "rotations per minute",
        .divisor = 1,
        .priority = NETDATA_CHART_PRIO_SENSORS + 5,
    },
    [MACOS_SENSOR_VOLTAGE] = {
        .name = "voltage",
        .family = "Voltage",
        .context = "system.hw.sensor.voltage.input",
        .units = "V",
        .divisor = 1000,
        .priority = NETDATA_CHART_PRIO_SENSORS + 2,
    },
    [MACOS_SENSOR_CURRENT] = {
        .name = "current",
        .family = "Current",
        .context = "system.hw.sensor.current.input",
        .units = "A",
        .divisor = 1000,
        .priority = NETDATA_CHART_PRIO_SENSORS + 3,
    },
    [MACOS_SENSOR_POWER] = {
        .name = "power",
        .family = "Power",
        .context = "system.hw.sensor.power.input",
        .units = "W",
        .divisor = 1000,
        .priority = NETDATA_CHART_PRIO_SENSORS + 6,
    },
};

static struct macos_sensor_chart *sensor_charts_root = NULL;
static struct macos_smc_sensor_candidate *smc_candidates_root = NULL;
static io_connect_t smc_connection = IO_OBJECT_NULL;
static CFDictionaryRef hid_matching = NULL;

static bool initialized = false;
static bool do_smc = true;
static bool do_hid = true;
static int discovery_every_s = MACOS_SENSORS_DEFAULT_DISCOVERY_EVERY;
static usec_t last_smc_discovery_ut = 0;

static bool macos_sensors_cfstring_to_cstr(CFStringRef str, char *dst, size_t dst_size)
{
    if (!str || !dst || dst_size == 0)
        return false;

    if (!CFStringGetCString(str, dst, dst_size, kCFStringEncodingUTF8)) {
        dst[0] = '\0';
        return false;
    }

    dst[dst_size - 1] = '\0';
    return dst[0] != '\0';
}

static CFNumberRef macos_sensors_cfnumber_int(int value)
{
    return CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &value);
}

static bool macos_sensors_smc_key_is_printable(const char key[MACOS_SMC_KEY_LEN + 1])
{
    for (size_t i = 0; i < MACOS_SMC_KEY_LEN; i++) {
        if (!isprint((unsigned char)key[i]))
            return false;
    }

    return key[MACOS_SMC_KEY_LEN] == '\0';
}

static bool macos_sensors_smc_kind_for_key(const char key[MACOS_SMC_KEY_LEN + 1], enum macos_sensor_kind *kind)
{
    if (!macos_sensors_smc_key_is_printable(key) || !kind)
        return false;

    if (key[0] == 'T') {
        *kind = MACOS_SENSOR_TEMPERATURE;
        return true;
    }

    if (key[0] == 'F' && isdigit((unsigned char)key[1]) && key[2] == 'A' && key[3] == 'c') {
        *kind = MACOS_SENSOR_FAN;
        return true;
    }

    if (key[0] == 'V') {
        *kind = MACOS_SENSOR_VOLTAGE;
        return true;
    }

    if (key[0] == 'I') {
        *kind = MACOS_SENSOR_CURRENT;
        return true;
    }

    if (key[0] == 'P') {
        *kind = MACOS_SENSOR_POWER;
        return true;
    }

    return false;
}

static void macos_sensors_smc_component_for_key(const char key[MACOS_SMC_KEY_LEN + 1], char *dst, size_t dst_size)
{
    const char *component = "hardware";

    switch (tolower((unsigned char)key[1])) {
        case 'a':
            component = "ambient";
            break;
        case 'b':
            component = "battery";
            break;
        case 'c':
            component = "cpu";
            break;
        case 'd':
        case 'h':
        case 'n':
            component = "storage";
            break;
        case 'f':
            component = "fan";
            break;
        case 'g':
            component = "gpu";
            break;
        case 'm':
            component = "memory";
            break;
        case 'p':
        case 's':
            component = "soc";
            break;
        case 'w':
            component = "wireless";
            break;
        case 'z':
            component = "power";
            break;
        default:
            break;
    }

    snprintfz(dst, dst_size, "%s", component);
}

static void macos_sensors_smc_label_for_key(
    const char key[MACOS_SMC_KEY_LEN + 1],
    enum macos_sensor_kind kind,
    char *dst,
    size_t dst_size)
{
    if (kind == MACOS_SENSOR_FAN) {
        snprintfz(dst, dst_size, "Fan %c Current Speed", key[1]);
        return;
    }

    if (kind == MACOS_SENSOR_TEMPERATURE) {
        if (!strncmp(key, "Tg", 2)) {
            snprintfz(dst, dst_size, "GPU Die Temperature");
            return;
        }
        if (!strncmp(key, "Tc", 2)) {
            snprintfz(dst, dst_size, "CPU Core Temperature");
            return;
        }
        if (!strncmp(key, "Tp", 2)) {
            snprintfz(dst, dst_size, "CPU Proximity Temperature");
            return;
        }
        if (!strncmp(key, "Tm", 2)) {
            snprintfz(dst, dst_size, "Memory Temperature");
            return;
        }
        if (!strncmp(key, "Ts", 2)) {
            snprintfz(dst, dst_size, "SoC Temperature");
            return;
        }
    } else if (kind == MACOS_SENSOR_POWER) {
        if (!strncmp(key, "PG", 2)) {
            snprintfz(dst, dst_size, "GPU Power");
            return;
        }
        if (!strncmp(key, "PC", 2)) {
            snprintfz(dst, dst_size, "CPU Power");
            return;
        }
    }

    snprintfz(dst, dst_size, "SMC %s %s", key, macos_sensor_defs[kind].family);
}

static bool macos_sensors_validate_value(enum macos_sensor_kind kind, NETDATA_DOUBLE value)
{
    if (!isfinite(value))
        return false;

    switch (kind) {
        case MACOS_SENSOR_TEMPERATURE:
            return value > 0.0 && value <= 150.0;
        case MACOS_SENSOR_FAN:
            return value >= 0.0 && value <= 30000.0;
        case MACOS_SENSOR_VOLTAGE:
            return value > 0.0 && value <= 1000.0;
        case MACOS_SENSOR_CURRENT:
            return value >= -10000.0 && value <= 10000.0;
        case MACOS_SENSOR_POWER:
            return value >= 0.0 && value <= 10000.0;
        default:
            return false;
    }
}

static bool macos_sensors_decode_smc_value(
    enum macos_sensor_kind kind,
    const struct macos_smc_value *value,
    NETDATA_DOUBLE *decoded)
{
    bool ok;

    if (kind == MACOS_SENSOR_TEMPERATURE)
        ok = macos_smc_decode_temperature(value, decoded);
    else
        ok = macos_smc_decode_numeric(value, decoded);

    return ok && macos_sensors_validate_value(kind, *decoded);
}

static struct macos_sensor_chart *macos_sensors_find_chart(const char *id)
{
    for (struct macos_sensor_chart *s = sensor_charts_root; s; s = s->next) {
        if (!strcmp(s->id, id))
            return s;
    }

    return NULL;
}

static struct macos_sensor_chart *macos_sensors_get_or_create_chart(
    const char *id,
    enum macos_sensor_kind kind,
    const char *label,
    const char *feature,
    const char *path,
    const char *driver,
    const char *subsystem,
    const char *chip_id,
    const char *source,
    const char *component)
{
    struct macos_sensor_chart *s = macos_sensors_find_chart(id);
    if (s)
        return s;

    s = callocz(1, sizeof(*s));
    snprintfz(s->id, sizeof(s->id), "%s", id);
    s->kind = kind;
    snprintfz(s->label, sizeof(s->label), "%s", label);
    snprintfz(s->feature, sizeof(s->feature), "%s", feature);
    snprintfz(s->path, sizeof(s->path), "%s", path);
    snprintfz(s->driver, sizeof(s->driver), "%s", driver);
    snprintfz(s->subsystem, sizeof(s->subsystem), "%s", subsystem);
    snprintfz(s->chip_id, sizeof(s->chip_id), "%s", chip_id);
    snprintfz(s->source, sizeof(s->source), "%s", source);
    snprintfz(s->component, sizeof(s->component), "%s", component);

    s->next = sensor_charts_root;
    sensor_charts_root = s;

    return s;
}

static void macos_sensors_obsolete_chart(struct macos_sensor_chart *s)
{
    if (s->st)
        rrdset_is_obsolete___safe_from_collector_thread(s->st);
}

static void macos_sensors_free_chart(struct macos_sensor_chart *s)
{
    if (!s)
        return;

    macos_sensors_obsolete_chart(s);
    freez(s);
}

static void macos_sensors_begin_cycle(void)
{
    for (struct macos_sensor_chart *s = sensor_charts_root; s; s = s->next)
        s->seen = false;
}

static void macos_sensors_finish_cycle(void)
{
    struct macos_sensor_chart **pp = &sensor_charts_root;
    while (*pp) {
        struct macos_sensor_chart *s = *pp;
        if (!s->seen && ++s->missing_cycles >= MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE) {
            *pp = s->next;
            macos_sensors_free_chart(s);
        } else
            pp = &s->next;
    }
}

static void macos_sensors_update_chart(
    const char *id,
    enum macos_sensor_kind kind,
    const char *label,
    const char *feature,
    const char *path,
    const char *driver,
    const char *subsystem,
    const char *chip_id,
    const char *source,
    const char *component,
    NETDATA_DOUBLE value,
    int update_every)
{
    struct macos_sensor_chart *s = macos_sensors_get_or_create_chart(
        id, kind, label, feature, path, driver, subsystem, chip_id, source, component);
    const struct macos_sensor_kind_def *def = &macos_sensor_defs[kind];

    s->seen = true;
    s->missing_cycles = 0;

    if (!s->st) {
        s->st = rrdset_create_localhost(
            "sensors",
            s->id,
            NULL,
            def->family,
            def->context,
            s->label,
            def->units,
            "macos.plugin",
            "sensors",
            def->priority,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(s->st->rrdlabels, "driver", s->driver, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "subsystem", s->subsystem, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "chip_id", s->chip_id, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "feature", s->feature, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "label", s->label, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "path", s->path, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "source", s->source, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "component", s->component, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "sensor", s->feature, RRDLABEL_SRC_AUTO);
    }

    if (!s->rd)
        s->rd = rrddim_add(s->st, "input", NULL, 1, def->divisor, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(s->st, s->rd, (collected_number)llround(value * (NETDATA_DOUBLE)def->divisor));
    rrdset_done(s->st);
}

static struct macos_smc_sensor_candidate *macos_sensors_find_smc_candidate(const char key[MACOS_SMC_KEY_LEN + 1])
{
    for (struct macos_smc_sensor_candidate *c = smc_candidates_root; c; c = c->next) {
        if (!strcmp(c->key, key))
            return c;
    }

    return NULL;
}

static struct macos_smc_sensor_candidate *macos_sensors_get_or_create_smc_candidate(
    const char key[MACOS_SMC_KEY_LEN + 1],
    enum macos_sensor_kind kind)
{
    struct macos_smc_sensor_candidate *c = macos_sensors_find_smc_candidate(key);
    if (c) {
        c->discovered = true;
        return c;
    }

    c = callocz(1, sizeof(*c));
    snprintfz(c->key, sizeof(c->key), "%s", key);
    c->kind = kind;
    c->discovered = true;
    macos_sensors_smc_label_for_key(key, kind, c->label, sizeof(c->label));
    macos_sensors_smc_component_for_key(key, c->component, sizeof(c->component));

    c->next = smc_candidates_root;
    smc_candidates_root = c;
    return c;
}

static void macos_sensors_prune_smc_candidates(void)
{
    struct macos_smc_sensor_candidate **pp = &smc_candidates_root;
    while (*pp) {
        struct macos_smc_sensor_candidate *c = *pp;
        if (!c->discovered) {
            *pp = c->next;
            freez(c);
        } else
            pp = &c->next;
    }
}

static bool macos_sensors_smc_ensure_open(void)
{
    if (smc_connection != IO_OBJECT_NULL)
        return true;

    return macos_smc_open(&smc_connection);
}

static bool macos_sensors_discover_smc(void)
{
    if (!macos_sensors_smc_ensure_open())
        return false;

    for (struct macos_smc_sensor_candidate *c = smc_candidates_root; c; c = c->next)
        c->discovered = false;

    uint32_t count = 0;
    if (!macos_smc_key_count(smc_connection, &count)) {
        macos_smc_close(&smc_connection);
        return false;
    }

    if (count > MACOS_SENSORS_MAX_SMC_KEYS)
        count = MACOS_SENSORS_MAX_SMC_KEYS;

    for (uint32_t i = 0; i < count; i++) {
        char key[MACOS_SMC_KEY_LEN + 1];
        enum macos_sensor_kind kind;
        if (!macos_smc_key_by_index(smc_connection, i, key) || !macos_sensors_smc_kind_for_key(key, &kind))
            continue;

        struct macos_smc_value value;
        NETDATA_DOUBLE decoded;
        if (!macos_smc_read_key(smc_connection, key, &value) ||
            !macos_sensors_decode_smc_value(kind, &value, &decoded))
            continue;

        macos_sensors_get_or_create_smc_candidate(key, kind);
    }

    macos_sensors_prune_smc_candidates();
    return smc_candidates_root != NULL;
}

static void macos_sensors_collect_smc(int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    if (!last_smc_discovery_ut || now_ut - last_smc_discovery_ut >= (usec_t)discovery_every_s * USEC_PER_SEC) {
        macos_sensors_discover_smc();
        last_smc_discovery_ut = now_ut;
    }

    if (smc_connection == IO_OBJECT_NULL || !smc_candidates_root)
        return;

    for (struct macos_smc_sensor_candidate *c = smc_candidates_root; c; c = c->next) {
        struct macos_smc_value value;
        NETDATA_DOUBLE decoded;
        if (!macos_smc_read_key(smc_connection, c->key, &value) ||
            !macos_sensors_decode_smc_value(c->kind, &value, &decoded))
            continue;

        char chart_id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(chart_id, sizeof(chart_id), "macos_smc_%s_%s", macos_sensor_defs[c->kind].name, c->key);
        netdata_fix_chart_id(chart_id);

        char path[192];
        snprintfz(path, sizeof(path), "AppleSMC/%s", c->key);

        macos_sensors_update_chart(
            chart_id,
            c->kind,
            c->label,
            c->key,
            path,
            "AppleSMC",
            "platform",
            "AppleSMC",
            "smc",
            c->component,
            decoded,
            update_every);
    }
}

static bool macos_sensors_hid_matching_init(void)
{
    if (hid_matching)
        return true;

    CFStringRef keys[] = {CFSTR("PrimaryUsagePage"), CFSTR("PrimaryUsage")};
    CFNumberRef values[] = {
        macos_sensors_cfnumber_int(MACOS_SENSORS_HID_PAGE_APPLE_VENDOR),
        macos_sensors_cfnumber_int(MACOS_SENSORS_HID_USAGE_TEMPERATURE_SENSOR),
    };
    if (!values[0] || !values[1]) {
        if (values[0])
            CFRelease(values[0]);
        if (values[1])
            CFRelease(values[1]);
        return false;
    }

    hid_matching = CFDictionaryCreate(
        kCFAllocatorDefault,
        (const void **)keys,
        (const void **)values,
        2,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);

    CFRelease(values[0]);
    CFRelease(values[1]);
    return hid_matching != NULL;
}

static bool macos_sensors_hid_product_name(IOHIDServiceClientRef service, char *name, size_t name_size)
{
    name[0] = '\0';

    CFTypeRef product = IOHIDServiceClientCopyProperty(service, CFSTR("Product"));
    if (!product)
        return false;

    bool ok = false;
    if (CFGetTypeID(product) == CFStringGetTypeID())
        ok = macos_sensors_cfstring_to_cstr((CFStringRef)product, name, name_size);

    CFRelease(product);
    return ok;
}

static unsigned macos_sensors_hid_duplicate_index(CFArrayRef services, CFIndex current, const char *name)
{
    unsigned duplicates = 0;

    for (CFIndex i = 0; i < current; i++) {
        IOHIDServiceClientRef service = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (!service)
            continue;

        char other[128];
        if (macos_sensors_hid_product_name(service, other, sizeof(other)) && !strcmp(other, name))
            duplicates++;
    }

    return duplicates;
}

static void macos_sensors_collect_hid(int update_every)
{
    if (!macos_sensors_hid_matching_init())
        return;

    IOHIDEventSystemClientRef client = IOHIDEventSystemClientCreate(kCFAllocatorDefault);
    if (!client)
        return;

    IOHIDEventSystemClientSetMatching(client, hid_matching);
    CFArrayRef services = IOHIDEventSystemClientCopyServices(client);
    if (!services) {
        CFRelease(client);
        return;
    }

    CFIndex count = CFArrayGetCount(services);
    for (CFIndex i = 0; i < count; i++) {
        IOHIDServiceClientRef service = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (!service)
            continue;

        IOHIDEventRef event =
            IOHIDServiceClientCopyEvent(service, MACOS_SENSORS_HID_EVENT_TYPE_TEMPERATURE, 0, 0);
        if (!event)
            continue;

        NETDATA_DOUBLE temp =
            IOHIDEventGetFloatValue(event, MACOS_SENSORS_HID_EVENT_TYPE_TEMPERATURE << 16);
        CFRelease(event);

        if (!macos_sensors_validate_value(MACOS_SENSOR_TEMPERATURE, temp))
            continue;

        char label[128];
        bool has_product = macos_sensors_hid_product_name(service, label, sizeof(label));
        if (!has_product)
            snprintfz(label, sizeof(label), "IOHID Temperature Sensor");

        char feature[128];
        if (has_product) {
            snprintfz(feature, sizeof(feature), "%s", label);
            netdata_fix_chart_id(feature);
        } else
            snprintfz(feature, sizeof(feature), "temperature_%ld", (long)i);

        unsigned duplicate_index = has_product ? macos_sensors_hid_duplicate_index(services, i, label) : 0;
        if (duplicate_index) {
            char duplicate_feature[128];
            snprintfz(duplicate_feature, sizeof(duplicate_feature), "%s_%u", feature, duplicate_index);
            snprintfz(feature, sizeof(feature), "%s", duplicate_feature);
        }

        char chart_id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(chart_id, sizeof(chart_id), "macos_iohid_temperature_%s", feature);
        netdata_fix_chart_id(chart_id);

        char path[192];
        snprintfz(path, sizeof(path), "IOHID/%s", feature);

        macos_sensors_update_chart(
            chart_id,
            MACOS_SENSOR_TEMPERATURE,
            label,
            feature,
            path,
            "IOHID",
            "hid",
            "IOHIDEventSystemClient",
            "iohid",
            "thermal",
            temp,
            update_every);
    }

    CFRelease(services);
    CFRelease(client);
}

static void macos_sensors_init(void)
{
    if (initialized)
        return;

    do_smc = inicfg_get_boolean(&netdata_config, "plugin:macos:sensors", "SMC sensors", 1);
    do_hid = inicfg_get_boolean(&netdata_config, "plugin:macos:sensors", "IOHID sensors", 1);
    discovery_every_s = (int)inicfg_get_duration_seconds(
        &netdata_config,
        "plugin:macos:sensors",
        "discovery every",
        MACOS_SENSORS_DEFAULT_DISCOVERY_EVERY);
    if (discovery_every_s < 10)
        discovery_every_s = 10;

    initialized = true;
}

int do_macos_sensors(int update_every, usec_t dt __maybe_unused)
{
    static int enabled = -1;
    if (unlikely(enabled == -1)) {
        enabled = inicfg_get_boolean(&netdata_config, "plugin:macos:sensors", "enabled", 1);
        if (!enabled)
            return 1;
    }

    macos_sensors_init();

    macos_sensors_begin_cycle();
    if (do_smc)
        macos_sensors_collect_smc(update_every);
    if (do_hid)
        macos_sensors_collect_hid(update_every);
    macos_sensors_finish_cycle();

    return 0;
}

void macos_sensors_cleanup(void)
{
    while (sensor_charts_root) {
        struct macos_sensor_chart *s = sensor_charts_root;
        sensor_charts_root = s->next;
        macos_sensors_free_chart(s);
    }

    while (smc_candidates_root) {
        struct macos_smc_sensor_candidate *c = smc_candidates_root;
        smc_candidates_root = c->next;
        freez(c);
    }

    macos_smc_close(&smc_connection);

    if (hid_matching) {
        CFRelease(hid_matching);
        hid_matching = NULL;
    }

    initialized = false;
    last_smc_discovery_ut = 0;
}
