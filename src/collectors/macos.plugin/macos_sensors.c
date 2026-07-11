// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos_iohid.h"
#include "macos_smc.h"
#include "plugin_macos.h"

#define _COMMON_PLUGIN_NAME "macos.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "sensors"
#include "../common-contexts/common-contexts.h"
#include "../common-contexts/hw-sensors-function.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <ctype.h>
#include <math.h>
#include <stdint.h>
#include <string.h>
#include <strings.h>

#define MACOS_SENSORS_DEFAULT_DISCOVERY_EVERY 300
#define MACOS_SENSORS_DEFAULT_SMC_COLLECTION_EVERY 10
#define MACOS_SENSORS_MAX_SMC_KEYS 8192
#define MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE 3
#define MACOS_SENSORS_MATCH_STRING_MAX 256

enum {
    MACOS_SENSORS_HID_PAGE_APPLE_VENDOR = 0xff00,
    MACOS_SENSORS_HID_USAGE_TEMPERATURE_SENSOR = 0x0005,
    MACOS_SENSORS_HID_EVENT_TYPE_TEMPERATURE = 15,
    MACOS_SENSORS_HID_PAGE_APPLE_VENDOR_POWER = 0xff08,
    MACOS_SENSORS_HID_USAGE_CURRENT_SENSOR = 0x0002,
    MACOS_SENSORS_HID_USAGE_VOLTAGE_SENSOR = 0x0003,
    MACOS_SENSORS_HID_EVENT_TYPE_POWER = 25,
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

    bool seen;
    unsigned missing_cycles;
    NETDATA_DOUBLE last_value;
    bool has_value;
    RRDSET *st;
    RRDDIM *rd;

    struct macos_sensor_chart *next;
};

struct macos_smc_sensor_candidate {
    char key[MACOS_SMC_KEY_LEN + 1];
    enum macos_sensor_kind kind;
    char label[128];
    char subsystem[64];
    struct macos_smc_key_info info;
    bool has_info;
    bool discovered;
    bool quarantined;
    unsigned consecutive_failures;

    struct macos_smc_sensor_candidate *next;
};

struct macos_hid_sensor_source {
    enum macos_sensor_kind kind;
    int primary_usage_page;
    int primary_usage;
    int64_t event_type;
    int32_t event_field;
    NETDATA_DOUBLE divisor;
    const char *subsystem;
    const char *default_label;
    const char *feature_prefix;
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
static struct macos_iohid_client hid_clients[MACOS_SENSOR_KIND_COUNT] = {0};

static bool initialized = false;
static bool do_smc = true;
static bool do_hid = true;
static int discovery_every_s = MACOS_SENSORS_DEFAULT_DISCOVERY_EVERY;
static int smc_collection_every_s = MACOS_SENSORS_DEFAULT_SMC_COLLECTION_EVERY;
static usec_t last_smc_discovery_ut = 0;
static usec_t last_smc_collection_ut = 0;
static bool smc_gpu_temperature_available = false;
static bool hid_gpu_temperature_available = false;
static bool smc_fan_available = false;
static bool last_gpu_temperature_available = false;
static bool last_gpu_power_available = false;

// per-channel charts are opt-in for high-cardinality kinds; the default view
// is the summary charts (per-subsystem rollups + the temperature histogram)
static bool do_per_sensor_charts[MACOS_SENSOR_KIND_COUNT] = {
    [MACOS_SENSOR_TEMPERATURE] = false,
    [MACOS_SENSOR_FAN] = true,
    [MACOS_SENSOR_VOLTAGE] = false,
    [MACOS_SENSOR_CURRENT] = false,
    [MACOS_SENSOR_POWER] = false,
};

static HW_SENSORS_TEMPERATURE_HISTOGRAM temperature_histogram = {0};

struct macos_sensor_rollup {
    enum macos_sensor_kind kind;
    char subsystem[64];

    RRDSET *st;
    RRDDIM *rd_min;
    RRDDIM *rd_avg;
    RRDDIM *rd_max;
    RRDDIM *rd_sum;
    bool obsolete;

    unsigned count;
    NETDATA_DOUBLE min;
    NETDATA_DOUBLE max;
    NETDATA_DOUBLE sum;

    struct macos_sensor_rollup *next;
};

static struct macos_sensor_rollup *sensor_rollups_root = NULL;

// guards sensor_charts_root and per-sensor values between the collection
// cycle, the "sensors" function threads and cleanup
// (netdata_mutex_t is uv_mutex_t - it has no static initializer)
static netdata_mutex_t macos_sensors_mutex;

static void __attribute__((constructor)) macos_sensors_mutex_init(void) {
    netdata_mutex_init(&macos_sensors_mutex);
}

static const struct macos_hid_sensor_source macos_hid_sensor_sources[] = {
    {
        .kind = MACOS_SENSOR_TEMPERATURE,
        .primary_usage_page = MACOS_SENSORS_HID_PAGE_APPLE_VENDOR,
        .primary_usage = MACOS_SENSORS_HID_USAGE_TEMPERATURE_SENSOR,
        .event_type = MACOS_SENSORS_HID_EVENT_TYPE_TEMPERATURE,
        .event_field = MACOS_SENSORS_HID_EVENT_TYPE_TEMPERATURE << 16,
        .divisor = 1.0,
        .subsystem = "thermal",
        .default_label = "IOHID Temperature Sensor",
        .feature_prefix = "temperature",
    },
    {
        .kind = MACOS_SENSOR_CURRENT,
        .primary_usage_page = MACOS_SENSORS_HID_PAGE_APPLE_VENDOR_POWER,
        .primary_usage = MACOS_SENSORS_HID_USAGE_CURRENT_SENSOR,
        .event_type = MACOS_SENSORS_HID_EVENT_TYPE_POWER,
        .event_field = MACOS_SENSORS_HID_EVENT_TYPE_POWER << 16,
        .divisor = 1000.0,
        .subsystem = "pmu",
        .default_label = "IOHID Current Sensor",
        .feature_prefix = "current",
    },
    {
        .kind = MACOS_SENSOR_VOLTAGE,
        .primary_usage_page = MACOS_SENSORS_HID_PAGE_APPLE_VENDOR_POWER,
        .primary_usage = MACOS_SENSORS_HID_USAGE_VOLTAGE_SENSOR,
        .event_type = MACOS_SENSORS_HID_EVENT_TYPE_POWER,
        .event_field = MACOS_SENSORS_HID_EVENT_TYPE_POWER << 16,
        .divisor = 1000.0,
        .subsystem = "pmu",
        .default_label = "IOHID Voltage Sensor",
        .feature_prefix = "voltage",
    },
};

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

static bool macos_sensors_smc_kind_for_key(const char key[MACOS_SMC_KEY_LEN + 1], enum macos_sensor_kind *kind)
{
    if (!macos_smc_key_is_valid(key) || !kind)
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

static bool macos_sensors_smc_key_is_gpu_temperature(const char key[MACOS_SMC_KEY_LEN + 1])
{
    return key[0] == 'T' && (key[1] == 'g' || key[1] == 'G');
}

static bool macos_sensors_smc_key_is_gpu_power(const char key[MACOS_SMC_KEY_LEN + 1])
{
    return key[0] == 'P' && (key[1] == 'g' || key[1] == 'G');
}

static bool macos_sensors_smc_key_suppressed_by_better_source(const char key[MACOS_SMC_KEY_LEN + 1])
{
    if (macos_sensors_smc_key_is_gpu_temperature(key) && macos_gpu_temperature_available())
        return true;

    if (macos_sensors_smc_key_is_gpu_power(key) && macos_gpu_power_source_available())
        return true;

    return false;
}

static bool macos_sensors_starts_with_ci(const char *value, const char *prefix)
{
    if (!value || !prefix || !*prefix)
        return false;

    size_t prefix_len = strnlen(prefix, MACOS_SENSORS_MATCH_STRING_MAX);
    if (prefix_len == MACOS_SENSORS_MATCH_STRING_MAX)
        return false;

    return strncasecmp(value, prefix, prefix_len) == 0;
}

static bool macos_sensors_contains_ci(const char *value, const char *needle)
{
    if (!value || !needle || !*needle)
        return false;

    size_t needle_len = strnlen(needle, MACOS_SENSORS_MATCH_STRING_MAX);
    if (needle_len == MACOS_SENSORS_MATCH_STRING_MAX)
        return false;

    for (const char *p = value; *p; p++) {
        if (strncasecmp(p, needle, needle_len) == 0)
            return true;
    }

    return false;
}

static bool macos_sensors_token_matches_ci(const char *value, const char *token)
{
    if (!value || !token || !*token)
        return false;

    size_t token_len = strnlen(token, MACOS_SENSORS_MATCH_STRING_MAX);
    if (token_len == MACOS_SENSORS_MATCH_STRING_MAX)
        return false;

    for (const char *p = value; *p;) {
        while (*p && !isalnum((unsigned char)*p))
            p++;

        const char *start = p;
        while (*p && isalnum((unsigned char)*p))
            p++;

        size_t len = (size_t)(p - start);
        if (len == token_len && strncasecmp(start, token, token_len) == 0)
            return true;
    }

    return false;
}

static const char *macos_sensors_hid_subsystem_for_product(
    const struct macos_hid_sensor_source *source,
    const char *label)
{
    if (source->kind != MACOS_SENSOR_TEMPERATURE || !label || !*label)
        return source->subsystem;

    if (macos_gpu_is_hid_temperature_sensor_name(label) ||
        macos_sensors_token_matches_ci(label, "gpu"))
        return "gpu";

    if (macos_sensors_starts_with_ci(label, "NAND") ||
        macos_sensors_token_matches_ci(label, "SSD") ||
        macos_sensors_token_matches_ci(label, "NVMe"))
        return "storage";

    if (macos_sensors_starts_with_ci(label, "PMU") ||
        macos_sensors_starts_with_ci(label, "sACC") ||
        macos_sensors_token_matches_ci(label, "SoC") ||
        macos_sensors_token_matches_ci(label, "PMGR"))
        return "soc";

    if (macos_sensors_token_matches_ci(label, "CPU") ||
        macos_sensors_starts_with_ci(label, "eACC") ||
        macos_sensors_starts_with_ci(label, "pACC"))
        return "cpu";

    if (macos_sensors_starts_with_ci(label, "mACC") ||
        macos_sensors_token_matches_ci(label, "DRAM") ||
        macos_sensors_token_matches_ci(label, "memory"))
        return "memory";

    if (macos_sensors_token_matches_ci(label, "ambient"))
        return "ambient";

    if (macos_sensors_token_matches_ci(label, "battery"))
        return "battery";

    if (macos_sensors_token_matches_ci(label, "airport") ||
        macos_sensors_token_matches_ci(label, "wireless") ||
        macos_sensors_token_matches_ci(label, "RF"))
        return "wireless";

    if (macos_sensors_contains_ci(label, "power delivery") ||
        macos_sensors_starts_with_ci(label, "PD "))
        return "power";

    return source->subsystem;
}

static void macos_sensors_smc_subsystem_for_key(const char key[MACOS_SMC_KEY_LEN + 1], char *dst, size_t dst_size)
{
    const char *subsystem = "hardware";

    if (key[0] == 'F' && isdigit((unsigned char)key[1]) && key[2] == 'A' && key[3] == 'c') {
        snprintfz(dst, dst_size, "fan");
        return;
    }

    char kind = (char)tolower((unsigned char)key[0]);
    char component = (char)tolower((unsigned char)key[1]);

    if (kind == 't') {
        switch (component) {
            case 'a':
                subsystem = "ambient";
                break;
            case 'b':
                subsystem = "battery";
                break;
            case 'c':
            case 'p':
                subsystem = "cpu";
                break;
            case 'd':
            case 'n':
                subsystem = "soc";
                break;
            case 'h':
                subsystem = "storage";
                break;
            case 'g':
                subsystem = "gpu";
                break;
            case 'm':
                subsystem = "memory";
                break;
            case 's':
                subsystem = "soc";
                break;
            case 'w':
                subsystem = "wireless";
                break;
            case 'z':
                subsystem = "power";
                break;
            default:
                break;
        }

        snprintfz(dst, dst_size, "%s", subsystem);
        return;
    }

    if (kind == 'p') {
        switch (component) {
            case 'c':
            case 'p':
                subsystem = "cpu";
                break;
            case 'g':
                subsystem = "gpu";
                break;
            case 'm':
                subsystem = "memory";
                break;
            case 's':
                subsystem = "soc";
                break;
            case 'z':
                subsystem = "power";
                break;
            default:
                subsystem = "power";
                break;
        }
    }

    snprintfz(dst, dst_size, "%s", subsystem);
    return;
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
            return value >= -40.0 && value <= 150.0;
        case MACOS_SENSOR_FAN:
            return value >= 0.0 && value <= 30000.0;
        case MACOS_SENSOR_VOLTAGE:
            return value >= 0.0 && value <= 1000.0;
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
    const char *source)
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

static struct macos_sensor_rollup *macos_sensors_get_or_create_rollup(enum macos_sensor_kind kind, const char *subsystem)
{
    for (struct macos_sensor_rollup *r = sensor_rollups_root; r; r = r->next) {
        if (r->kind == kind && !strcmp(r->subsystem, subsystem))
            return r;
    }

    struct macos_sensor_rollup *r = callocz(1, sizeof(*r));
    r->kind = kind;
    snprintfz(r->subsystem, sizeof(r->subsystem), "%s", subsystem);
    r->next = sensor_rollups_root;
    sensor_rollups_root = r;
    return r;
}

static void macos_sensors_update_rollup_chart(struct macos_sensor_rollup *r, int update_every)
{
    if (!r->count) {
        if (r->st && !r->obsolete) {
            rrdset_is_obsolete___safe_from_collector_thread(r->st);
            r->obsolete = true;
        }
        return;
    }

    const struct macos_sensor_kind_def *def = &macos_sensor_defs[r->kind];

    if (!r->st) {
        char id[RRD_ID_LENGTH_MAX + 1];
        const char *context;
        const char *title;
        int priority;

        if (r->kind == MACOS_SENSOR_TEMPERATURE) {
            snprintfz(id, sizeof(id), "temperature_subsystem_%s", r->subsystem);
            context = "system.hw.sensor.temperature.subsystem";
            title = "Temperature Summary";
            priority = NETDATA_CHART_PRIO_SENSORS - 8;
        }
        else {
            snprintfz(id, sizeof(id), "power_subsystem_%s", r->subsystem);
            context = "system.hw.sensor.power.subsystem";
            title = "Power Summary";
            priority = NETDATA_CHART_PRIO_SENSORS - 6;
        }

        netdata_fix_chart_id(id);

        r->st = rrdset_create_localhost(
            "sensors",
            id,
            NULL,
            def->family,
            context,
            title,
            def->units,
            "macos.plugin",
            "sensors",
            priority,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(r->st->rrdlabels, "subsystem", r->subsystem, RRDLABEL_SRC_AUTO);

        if (r->kind == MACOS_SENSOR_TEMPERATURE) {
            r->rd_min = rrddim_add(r->st, "min", NULL, 1, def->divisor, RRD_ALGORITHM_ABSOLUTE);
            r->rd_avg = rrddim_add(r->st, "avg", NULL, 1, def->divisor, RRD_ALGORITHM_ABSOLUTE);
            r->rd_max = rrddim_add(r->st, "max", NULL, 1, def->divisor, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            r->rd_sum = rrddim_add(r->st, "power", NULL, 1, def->divisor, RRD_ALGORITHM_ABSOLUTE);
    }

    if (r->obsolete) {
        rrdset_isnot_obsolete___safe_from_collector_thread(r->st);
        r->obsolete = false;
    }

    if (r->kind == MACOS_SENSOR_TEMPERATURE) {
        rrddim_set_by_pointer(r->st, r->rd_min, (collected_number)llround(r->min * (NETDATA_DOUBLE)def->divisor));
        rrddim_set_by_pointer(
            r->st, r->rd_avg,
            (collected_number)llround(r->sum / (NETDATA_DOUBLE)r->count * (NETDATA_DOUBLE)def->divisor));
        rrddim_set_by_pointer(r->st, r->rd_max, (collected_number)llround(r->max * (NETDATA_DOUBLE)def->divisor));
    }
    else
        rrddim_set_by_pointer(r->st, r->rd_sum, (collected_number)llround(r->sum * (NETDATA_DOUBLE)def->divisor));

    rrdset_done(r->st);
}

static void macos_sensors_begin_cycle(void)
{
    for (struct macos_sensor_chart *s = sensor_charts_root; s; s = s->next)
        s->seen = false;
}

static void macos_sensors_finish_cycle(bool smc_attempted, bool hid_attempted, int update_every)
{
    struct macos_sensor_chart **pp = &sensor_charts_root;
    while (*pp) {
        struct macos_sensor_chart *s = *pp;
        bool source_attempted = true;

        if (!strcmp(s->source, "smc"))
            source_attempted = smc_attempted;
        else if (!strcmp(s->source, "iohid"))
            source_attempted = hid_attempted;

        if (!source_attempted) {
            pp = &s->next;
            continue;
        }

        if (!s->seen) {
            if (s->missing_cycles < MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE &&
                ++s->missing_cycles == MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE) {
                macos_sensors_obsolete_chart(s);
                s->has_value = false;
            }

            pp = &s->next;
            continue;
        }

        pp = &s->next;
    }

    smc_gpu_temperature_available = false;
    smc_fan_available = false;
    for (struct macos_sensor_chart *s = sensor_charts_root; s; s = s->next) {
        if (strcmp(s->source, "smc") != 0)
            continue;

        if (s->kind == MACOS_SENSOR_FAN)
            smc_fan_available = true;
        else if (s->kind == MACOS_SENSOR_TEMPERATURE && strcmp(s->subsystem, "gpu") == 0)
            smc_gpu_temperature_available = true;
    }

    // ------------------------------------------------------------------
    // summary views: temperature histogram + per-subsystem rollups.
    // SMC values refresh on their own cadence; the last known value of each
    // sensor is used until the sensor is missing long enough to be obsoleted.

    hw_sensors_temperature_histogram_reset(&temperature_histogram);

    for (struct macos_sensor_rollup *r = sensor_rollups_root; r; r = r->next) {
        r->count = 0;
        r->min = r->max = r->sum = 0.0;
    }

    for (struct macos_sensor_chart *s = sensor_charts_root; s; s = s->next) {
        if (!s->has_value || s->missing_cycles >= MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE)
            continue;

        if (s->kind != MACOS_SENSOR_TEMPERATURE && s->kind != MACOS_SENSOR_POWER)
            continue;

        if (s->kind == MACOS_SENSOR_TEMPERATURE)
            hw_sensors_temperature_histogram_add(&temperature_histogram, s->last_value);

        struct macos_sensor_rollup *r = macos_sensors_get_or_create_rollup(s->kind, s->subsystem);
        if (!r->count || s->last_value < r->min)
            r->min = s->last_value;
        if (!r->count || s->last_value > r->max)
            r->max = s->last_value;
        r->sum += s->last_value;
        r->count++;
    }

    common_hw_sensors_temperature_histogram(&temperature_histogram, update_every, "macos.plugin", "sensors");

    for (struct macos_sensor_rollup *r = sensor_rollups_root; r; r = r->next)
        macos_sensors_update_rollup_chart(r, update_every);
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
    NETDATA_DOUBLE value,
    int update_every)
{
    const struct macos_sensor_kind_def *def = &macos_sensor_defs[kind];
    collected_number collected_value = (collected_number)llround(value * (NETDATA_DOUBLE)def->divisor);
    struct macos_sensor_chart *s = macos_sensors_find_chart(id);
    if (!s && kind == MACOS_SENSOR_VOLTAGE && collected_value == 0)
        return;

    if (!s)
        s = macos_sensors_get_or_create_chart(id, kind, label, feature, path, driver, subsystem, chip_id, source);

    if (s->st && s->missing_cycles >= MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE)
        rrdset_isnot_obsolete___safe_from_collector_thread(s->st);

    s->seen = true;
    s->missing_cycles = 0;
    s->last_value = value;
    s->has_value = true;

    // high-cardinality kinds chart per-channel only when enabled; the value
    // is still collected above to feed the summary charts
    if (!do_per_sensor_charts[s->kind])
        return;

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
        rrdlabels_add(s->st->rrdlabels, "device", s->chip_id, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "feature", s->feature, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "label", s->label, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "path", s->path, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "source", s->source, RRDLABEL_SRC_AUTO);
        rrdlabels_add(s->st->rrdlabels, "sensor", s->feature, RRDLABEL_SRC_AUTO);
    }

    if (!s->rd)
        s->rd = rrddim_add(s->st, "input", NULL, 1, def->divisor, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(s->st, s->rd, collected_value);
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
    enum macos_sensor_kind kind,
    const struct macos_smc_key_info *info)
{
    struct macos_smc_sensor_candidate *c = macos_sensors_find_smc_candidate(key);
    if (c) {
        c->discovered = true;
        c->kind = kind;
        if (info) {
            c->info = *info;
            c->has_info = true;
        }
        c->quarantined = false;
        c->consecutive_failures = 0;
        return c;
    }

    c = callocz(1, sizeof(*c));
    snprintfz(c->key, sizeof(c->key), "%s", key);
    c->kind = kind;
    c->discovered = true;
    if (info) {
        c->info = *info;
        c->has_info = true;
    }
    macos_sensors_smc_label_for_key(key, kind, c->label, sizeof(c->label));
    macos_sensors_smc_subsystem_for_key(key, c->subsystem, sizeof(c->subsystem));

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

        if (macos_sensors_smc_key_suppressed_by_better_source(key))
            continue;

        struct macos_smc_key_info info;
        if (!macos_smc_read_key_info(smc_connection, key, &info))
            continue;

        struct macos_smc_value value;
        NETDATA_DOUBLE decoded;
        if (!macos_smc_read_key_with_info(smc_connection, key, &info, &value) ||
            !macos_sensors_decode_smc_value(kind, &value, &decoded))
            continue;

        macos_sensors_get_or_create_smc_candidate(key, kind, &info);
    }

    macos_sensors_prune_smc_candidates();
    return true;
}

static int macos_sensors_smc_chart_update_every(int plugin_update_every)
{
    if (smc_collection_every_s > plugin_update_every)
        return smc_collection_every_s;

    return plugin_update_every > 0 ? plugin_update_every : 1;
}

static bool macos_sensors_read_smc_candidate(
    struct macos_smc_sensor_candidate *c,
    struct macos_smc_value *value)
{
    if (!c->has_info) {
        if (!macos_smc_read_key_info(smc_connection, c->key, &c->info))
            return false;

        c->has_info = true;
    }

    return macos_smc_read_key_with_info(smc_connection, c->key, &c->info, value);
}

static bool macos_sensors_collect_smc(int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    bool gpu_temperature_available = macos_gpu_temperature_available();
    bool gpu_power_available = macos_gpu_power_source_available();
    bool collection_due = !last_smc_collection_ut ||
                          now_ut - last_smc_collection_ut >= (usec_t)smc_collection_every_s * USEC_PER_SEC;
    if (gpu_temperature_available != last_gpu_temperature_available ||
        gpu_power_available != last_gpu_power_available) {
        last_smc_discovery_ut = 0;
        last_gpu_temperature_available = gpu_temperature_available;
        last_gpu_power_available = gpu_power_available;
    }

    if (!last_smc_discovery_ut || now_ut - last_smc_discovery_ut >= (usec_t)discovery_every_s * USEC_PER_SEC) {
        if (macos_sensors_discover_smc())
            last_smc_discovery_ut = now_ut;
    }

    if (!collection_due)
        return false;

    last_smc_collection_ut = now_ut;
    smc_gpu_temperature_available = false;
    smc_fan_available = false;

    if (smc_connection == IO_OBJECT_NULL || !smc_candidates_root)
        return true;

    int chart_update_every = macos_sensors_smc_chart_update_every(update_every);

    for (struct macos_smc_sensor_candidate *c = smc_candidates_root; c; c = c->next) {
        if (c->quarantined || macos_sensors_smc_key_suppressed_by_better_source(c->key))
            continue;

        struct macos_smc_value value;
        NETDATA_DOUBLE decoded;
        if (!macos_sensors_read_smc_candidate(c, &value) ||
            !macos_sensors_decode_smc_value(c->kind, &value, &decoded)) {
            if (++c->consecutive_failures >= MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE)
                c->quarantined = true;
            continue;
        }

        c->consecutive_failures = 0;
        if (c->kind == MACOS_SENSOR_FAN)
            smc_fan_available = true;
        else if (c->kind == MACOS_SENSOR_TEMPERATURE && macos_sensors_smc_key_is_gpu_temperature(c->key))
            smc_gpu_temperature_available = true;

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
            c->subsystem,
            "AppleSMC",
            "smc",
            decoded,
            chart_update_every);
    }

    return true;
}

static bool macos_sensors_hid_product_name(IOHIDServiceClientRef service, char *name, size_t name_size)
{
    name[0] = '\0';

    CFTypeRef product = macos_iohid_service_copy_property(service, CFSTR("Product"));
    if (!product)
        return false;

    bool ok = false;
    if (CFGetTypeID(product) == CFStringGetTypeID())
        ok = macos_sensors_cfstring_to_cstr((CFStringRef)product, name, name_size);

    CFRelease(product);
    return ok;
}

static bool macos_sensors_cf_value_identifier(CFTypeRef value, char *dst, size_t dst_size)
{
    if (!value || !dst || dst_size == 0)
        return false;

    dst[0] = '\0';

    if (CFGetTypeID(value) == CFNumberGetTypeID()) {
        int64_t signed_id = 0;
        if (!CFNumberGetValue((CFNumberRef)value, kCFNumberSInt64Type, &signed_id))
            return false;

        uint64_t id = (uint64_t)signed_id;
        snprintfz(dst, dst_size, "%llx", (unsigned long long)id);
        return dst[0] != '\0';
    }

    if (CFGetTypeID(value) == CFStringGetTypeID()) {
        if (!macos_sensors_cfstring_to_cstr((CFStringRef)value, dst, dst_size))
            return false;

        netdata_fix_chart_id(dst);
        return dst[0] != '\0';
    }

    return false;
}

static bool macos_sensors_hid_service_identifier(IOHIDServiceClientRef service, char *dst, size_t dst_size)
{
    // Prefer LocationID: it derives from the hardware topology and is stable
    // across reboots. IORegistryEntryID is unique per boot, so identities
    // built from it would mint new time-series on every system restart.
    CFTypeRef location_id = macos_iohid_service_copy_property(service, CFSTR("LocationID"));
    if (location_id) {
        bool ok = macos_sensors_cf_value_identifier(location_id, dst, dst_size);
        CFRelease(location_id);
        if (ok) {
            char prefixed[128];
            snprintfz(prefixed, sizeof(prefixed), "location_%s", dst);
            snprintfz(dst, dst_size, "%s", prefixed);
            return true;
        }
    }

    CFTypeRef registry_id = macos_iohid_service_get_registry_id(service);
    if (macos_sensors_cf_value_identifier(registry_id, dst, dst_size)) {
        char prefixed[128];
        snprintfz(prefixed, sizeof(prefixed), "registry_%s", dst);
        snprintfz(dst, dst_size, "%s", prefixed);
        return true;
    }

    return false;
}

static bool macos_sensors_collect_hid_source(const struct macos_hid_sensor_source *source, int update_every)
{
    struct macos_iohid_client *client = &hid_clients[source->kind];
    if (!macos_iohid_client_set_matching(
            client,
            source->primary_usage_page,
            source->primary_usage))
        return true;

    CFArrayRef services = macos_iohid_client_copy_services(client);
    if (!services)
        return true;

    CFIndex count = CFArrayGetCount(services);
    if (!count) {
        CFRelease(services);
        return true;
    }

    // Identity stability: when a service's product name is unique within this
    // kind, the product name alone is the sensor identity. Appending per-boot
    // identifiers unconditionally (as done before) minted new time-series on
    // every reboot. Only true duplicates get a service-specific suffix.
    // Known trade-off: if a duplicate product appears mid-run (hot-plug), the
    // first device's identity changes from bare product to a suffixed one -
    // acceptable, since built-in sensors do not hot-plug in practice.
    // the O(count^2) duplicate scan below is fine: Apple Silicon exposes at
    // most a few dozen IOHID services per kind
    char (*products)[128] = callocz((size_t)count, sizeof(*products));
    for (CFIndex i = 0; i < count; i++) {
        IOHIDServiceClientRef svc = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (svc && macos_sensors_hid_product_name(svc, products[i], sizeof(products[i])))
            netdata_fix_chart_id(products[i]);
        else
            products[i][0] = '\0';
    }

    for (CFIndex i = 0; i < count; i++) {
        IOHIDServiceClientRef service = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (!service)
            continue;

        char label[128];
        bool has_product = macos_sensors_hid_product_name(service, label, sizeof(label));
        bool is_gpu_temperature_sensor = has_product &&
                                         source->kind == MACOS_SENSOR_TEMPERATURE &&
                                         macos_gpu_is_hid_temperature_sensor_name(label);
        if (is_gpu_temperature_sensor && macos_gpu_temperature_available())
            continue;
        if (!has_product)
            snprintfz(label, sizeof(label), "%s", source->default_label);

        NETDATA_DOUBLE value;
        if (!macos_iohid_service_copy_event_float(
                service,
                source->event_type,
                source->event_field,
                &value))
            continue;

        value /= source->divisor;

        if (!macos_sensors_validate_value(source->kind, value))
            continue;

        bool product_is_unique = false;
        if (products[i][0]) {
            product_is_unique = true;
            for (CFIndex j = 0; j < count; j++) {
                if (j != i && strcmp(products[j], products[i]) == 0) {
                    product_is_unique = false;
                    break;
                }
            }
        }

        char feature[128];
        if (product_is_unique) {
            snprintfz(feature, sizeof(feature), "%s", products[i]);
        }
        else {
            char service_identifier[128] = "";
            if (!macos_sensors_hid_service_identifier(service, service_identifier, sizeof(service_identifier)))
                continue;

            if (products[i][0])
                snprintfz(feature, sizeof(feature), "%s_%s", products[i], service_identifier);
            else
                snprintfz(feature, sizeof(feature), "%s_%s", source->feature_prefix, service_identifier);
        }

        netdata_fix_chart_id(feature);

        char chart_id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(chart_id, sizeof(chart_id), "macos_iohid_%s_%s", macos_sensor_defs[source->kind].name, feature);
        netdata_fix_chart_id(chart_id);

        char path[192];
        snprintfz(path, sizeof(path), "IOHID/%s/%s", macos_sensor_defs[source->kind].name, feature);
        const char *subsystem = macos_sensors_hid_subsystem_for_product(source, label);

        macos_sensors_update_chart(
            chart_id,
            source->kind,
            label,
            feature,
            path,
            "IOHID",
            subsystem,
            "IOHIDEventSystemClient",
            "iohid",
            value,
            update_every);

        if (is_gpu_temperature_sensor)
            hid_gpu_temperature_available = true;
    }

    freez(products);
    CFRelease(services);
    return true;
}

static bool macos_sensors_collect_hid(int update_every)
{
    bool attempted = false;
    hid_gpu_temperature_available = false;

    for (size_t i = 0; i < _countof(macos_hid_sensor_sources); i++)
        attempted |= macos_sensors_collect_hid_source(&macos_hid_sensor_sources[i], update_every);

    return attempted;
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

    smc_collection_every_s = (int)inicfg_get_duration_seconds(
        &netdata_config,
        "plugin:macos:sensors",
        "SMC sample every",
        MACOS_SENSORS_DEFAULT_SMC_COLLECTION_EVERY);
    if (smc_collection_every_s < 1)
        smc_collection_every_s = 1;

    do_per_sensor_charts[MACOS_SENSOR_TEMPERATURE] = inicfg_get_boolean(
        &netdata_config, "plugin:macos:sensors", "per sensor temperature charts",
        do_per_sensor_charts[MACOS_SENSOR_TEMPERATURE]);
    do_per_sensor_charts[MACOS_SENSOR_FAN] = inicfg_get_boolean(
        &netdata_config, "plugin:macos:sensors", "per sensor fan charts",
        do_per_sensor_charts[MACOS_SENSOR_FAN]);
    do_per_sensor_charts[MACOS_SENSOR_VOLTAGE] = inicfg_get_boolean(
        &netdata_config, "plugin:macos:sensors", "per sensor voltage charts",
        do_per_sensor_charts[MACOS_SENSOR_VOLTAGE]);
    do_per_sensor_charts[MACOS_SENSOR_CURRENT] = inicfg_get_boolean(
        &netdata_config, "plugin:macos:sensors", "per sensor current charts",
        do_per_sensor_charts[MACOS_SENSOR_CURRENT]);
    do_per_sensor_charts[MACOS_SENSOR_POWER] = inicfg_get_boolean(
        &netdata_config, "plugin:macos:sensors", "per sensor power charts",
        do_per_sensor_charts[MACOS_SENSOR_POWER]);

    initialized = true;
}

#define MACOS_SENSORS_FUNCTION_HELP                                                                                    \
    "Lists all hardware sensors discovered on this host (SMC and IOHID), with their current readings - including "    \
    "the sensors that are aggregated into the summary charts and not charted individually."

static int macos_sensors_function(
    BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused)
{
    netdata_mutex_lock(&macos_sensors_mutex);

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", MACOS_SENSORS_FUNCTION_HELP);
    buffer_json_member_add_array(wb, "data");

    for (struct macos_sensor_chart *s = sensor_charts_root; s; s = s->next) {
        const struct macos_sensor_kind_def *def = &macos_sensor_defs[s->kind];
        bool fresh = s->has_value && s->missing_cycles < MACOS_SENSORS_MISSING_CYCLES_BEFORE_OBSOLETE;
        NETDATA_DOUBLE reading = fresh ? s->last_value : NAN;

        // the chart id this sensor has (or would have) in per-sensor mode,
        // so users can correlate function rows with dashboard charts
        char chart_id[RRD_ID_LENGTH_MAX + 16];
        snprintfz(chart_id, sizeof(chart_id), "sensors.%s", s->id);

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, chart_id);
        buffer_json_add_array_item_string(wb, s->label);
        buffer_json_add_array_item_string(wb, def->name);
        buffer_json_add_array_item_string(wb, s->subsystem);
        buffer_json_add_array_item_string(wb, s->source);
        buffer_json_add_array_item_string(wb, s->chip_id);
        buffer_json_add_array_item_string(wb, s->feature);
        buffer_json_add_array_item_double(wb, reading);
        buffer_json_add_array_item_string(wb, def->units);
        buffer_json_add_array_item_string(wb, fresh ? "ok" : "missing");
        buffer_json_add_array_item_string(wb, do_per_sensor_charts[s->kind] ? "per-sensor" : "summary");
        buffer_json_add_array_item_uint64(wb, 1); // Count - enables grouped sensor-count charts

        // per-kind value columns - they power the per-kind aggregation tiles
        buffer_json_add_array_item_double(wb, s->kind == MACOS_SENSOR_TEMPERATURE ? reading : NAN);
        buffer_json_add_array_item_double(wb, s->kind == MACOS_SENSOR_FAN ? reading : NAN);
        buffer_json_add_array_item_double(wb, s->kind == MACOS_SENSOR_VOLTAGE ? reading : NAN);
        buffer_json_add_array_item_double(wb, s->kind == MACOS_SENSOR_CURRENT ? reading : NAN);
        buffer_json_add_array_item_double(wb, s->kind == MACOS_SENSOR_POWER ? reading : NAN);

        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb); // data

    hw_sensors_function_columns(wb);
    hw_sensors_function_presentation(wb);
    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    netdata_mutex_unlock(&macos_sensors_mutex);
    return HTTP_RESP_OK;
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

    netdata_mutex_lock(&macos_sensors_mutex);

    macos_sensors_begin_cycle();
    bool smc_attempted = false;
    bool hid_attempted = false;
    if (do_smc)
        smc_attempted = macos_sensors_collect_smc(update_every);
    if (do_hid)
        hid_attempted = macos_sensors_collect_hid(update_every);
    macos_sensors_finish_cycle(smc_attempted, hid_attempted, update_every);

    netdata_mutex_unlock(&macos_sensors_mutex);

    // expose the function only on hosts that actually have sensors
    // (only this collection thread mutates the sensors list)
    static bool function_registered = false;
    if (unlikely(!function_registered && sensor_charts_root)) {
        rrd_function_add_inline(localhost, NULL, "sensors", 10,
                                RRDFUNCTIONS_PRIORITY_DEFAULT, RRDFUNCTIONS_VERSION_DEFAULT,
                                MACOS_SENSORS_FUNCTION_HELP,
                                "top", HTTP_ACCESS_ANONYMOUS_DATA,
                                macos_sensors_function);
        function_registered = true;
    }

    return 0;
}

bool macos_sensors_gpu_temperature_available(void)
{
    return smc_gpu_temperature_available || hid_gpu_temperature_available;
}

bool macos_sensors_fan_available(void)
{
    return smc_fan_available;
}

void macos_sensors_cleanup(void)
{
    // serialize with in-flight "sensors" function requests: they must not
    // observe the list while it is being freed
    netdata_mutex_lock(&macos_sensors_mutex);

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

    while (sensor_rollups_root) {
        struct macos_sensor_rollup *r = sensor_rollups_root;
        sensor_rollups_root = r->next;
        if (r->st)
            rrdset_is_obsolete___safe_from_collector_thread(r->st);
        freez(r);
    }

    if (temperature_histogram.st) {
        rrdset_is_obsolete___safe_from_collector_thread(temperature_histogram.st);
        memset(&temperature_histogram, 0, sizeof(temperature_histogram));
    }

    macos_smc_close(&smc_connection);

    for (size_t i = 0; i < _countof(hid_clients); i++)
        macos_iohid_client_cleanup(&hid_clients[i]);

    initialized = false;
    last_smc_discovery_ut = 0;
    last_smc_collection_ut = 0;
    smc_gpu_temperature_available = false;
    hid_gpu_temperature_available = false;
    smc_fan_available = false;

    netdata_mutex_unlock(&macos_sensors_mutex);
}
