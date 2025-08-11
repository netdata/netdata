// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

#define NETDATA_CALCULATED_STATES 1

#include "libsensors/vendored/lib/sensors.h"
#include "libsensors/vendored/lib/error.h"

typedef short SENSOR_BUS_TYPE;
ENUM_STR_MAP_DEFINE(SENSOR_BUS_TYPE) = {
    { .id = SENSORS_BUS_TYPE_ANY, .name = "any", },
    { .id = SENSORS_BUS_TYPE_I2C, .name = "i2c", },
    { .id = SENSORS_BUS_TYPE_ISA, .name = "isa", },
    { .id = SENSORS_BUS_TYPE_PCI, .name = "pci", },
    { .id = SENSORS_BUS_TYPE_SPI, .name = "spi", },
    { .id = SENSORS_BUS_TYPE_VIRTUAL, .name = "virtual", },
    { .id = SENSORS_BUS_TYPE_ACPI, .name = "acpi", },
    { .id = SENSORS_BUS_TYPE_HID, .name = "hid", },
    { .id = SENSORS_BUS_TYPE_MDIO, .name = "mdio", },
    { .id = SENSORS_BUS_TYPE_SCSI, .name = "scsi", },

    // terminator
    {.id = 0, .name = NULL}
};
ENUM_STR_DEFINE_FUNCTIONS(SENSOR_BUS_TYPE, SENSORS_BUS_TYPE_ANY, "any");

typedef sensors_feature_type SENSOR_TYPE;
ENUM_STR_MAP_DEFINE(SENSOR_TYPE) = {
    { .id = SENSORS_FEATURE_IN, .name = "voltage", },
    { .id = SENSORS_FEATURE_FAN, .name = "fan", },
    { .id = SENSORS_FEATURE_TEMP, .name = "temperature", },
    { .id = SENSORS_FEATURE_POWER, .name = "power", },
    { .id = SENSORS_FEATURE_ENERGY, .name = "energy", },
    { .id = SENSORS_FEATURE_CURR, .name = "curr", },
    { .id = SENSORS_FEATURE_HUMIDITY, .name = "humidity", },
    { .id = SENSORS_FEATURE_VID, .name = "vid", },
    { .id = SENSORS_FEATURE_INTRUSION, .name = "intrusion", },
    { .id = SENSORS_FEATURE_BEEP_ENABLE, .name = "beep_enable", },
    { .id = SENSORS_FEATURE_UNKNOWN, .name = "unknown", },

    // terminator
    {.id = 0, .name = NULL}
};
ENUM_STR_DEFINE_FUNCTIONS(SENSOR_TYPE, SENSORS_FEATURE_UNKNOWN, "unknown");

typedef sensors_subfeature_type SENSOR_SUBFEATURE_TYPE;
ENUM_STR_MAP_DEFINE(SENSOR_SUBFEATURE_TYPE) = {
    // Voltage input subfeatures
    { .id = SENSORS_SUBFEATURE_IN_INPUT, .name = "input", },
    { .id = SENSORS_SUBFEATURE_IN_MIN, .name = "minimum", },
    { .id = SENSORS_SUBFEATURE_IN_MAX, .name = "maximum", },
    { .id = SENSORS_SUBFEATURE_IN_LCRIT, .name = "critical low", },
    { .id = SENSORS_SUBFEATURE_IN_CRIT, .name = "critical high", },
    { .id = SENSORS_SUBFEATURE_IN_AVERAGE, .name = "average", },
    { .id = SENSORS_SUBFEATURE_IN_LOWEST, .name = "lowest", },
    { .id = SENSORS_SUBFEATURE_IN_HIGHEST, .name = "highest", },
    { .id = SENSORS_SUBFEATURE_IN_ALARM, .name = "alarm", },
    { .id = SENSORS_SUBFEATURE_IN_MIN_ALARM, .name = "alarm low", },
    { .id = SENSORS_SUBFEATURE_IN_MAX_ALARM, .name = "alarm high", },
    { .id = SENSORS_SUBFEATURE_IN_BEEP, .name = "beep", },
    { .id = SENSORS_SUBFEATURE_IN_LCRIT_ALARM, .name = "critical alarm low", },
    { .id = SENSORS_SUBFEATURE_IN_CRIT_ALARM, .name = "critical alarm high", },

    // Fan subfeatures
    { .id = SENSORS_SUBFEATURE_FAN_INPUT, .name = "input", },
    { .id = SENSORS_SUBFEATURE_FAN_MIN, .name = "minimum", },
    { .id = SENSORS_SUBFEATURE_FAN_MAX, .name = "maximum", },
    { .id = SENSORS_SUBFEATURE_FAN_ALARM, .name = "alarm", },
    { .id = SENSORS_SUBFEATURE_FAN_FAULT, .name = "fault", },
    { .id = SENSORS_SUBFEATURE_FAN_DIV, .name = "divisor", },
    { .id = SENSORS_SUBFEATURE_FAN_BEEP, .name = "beep", },
    { .id = SENSORS_SUBFEATURE_FAN_PULSES, .name = "pulses", },
    { .id = SENSORS_SUBFEATURE_FAN_MIN_ALARM, .name = "alarm low", },
    { .id = SENSORS_SUBFEATURE_FAN_MAX_ALARM, .name = "alarm high", },

    // Temperature subfeatures
    { .id = SENSORS_SUBFEATURE_TEMP_INPUT, .name = "input", },
    { .id = SENSORS_SUBFEATURE_TEMP_MAX, .name = "maximum", },
    { .id = SENSORS_SUBFEATURE_TEMP_MAX_HYST, .name = "maximum hysteresis", },
    { .id = SENSORS_SUBFEATURE_TEMP_MIN, .name = "minimum", },
    { .id = SENSORS_SUBFEATURE_TEMP_CRIT, .name = "critical high", },
    { .id = SENSORS_SUBFEATURE_TEMP_CRIT_HYST, .name = "critical hysteresis", },
    { .id = SENSORS_SUBFEATURE_TEMP_LCRIT, .name = "critical low", },
    { .id = SENSORS_SUBFEATURE_TEMP_EMERGENCY, .name = "emergency", },
    { .id = SENSORS_SUBFEATURE_TEMP_EMERGENCY_HYST, .name = "emergency hysteresis", },
    { .id = SENSORS_SUBFEATURE_TEMP_LOWEST, .name = "lowest", },
    { .id = SENSORS_SUBFEATURE_TEMP_HIGHEST, .name = "highest", },
    { .id = SENSORS_SUBFEATURE_TEMP_MIN_HYST, .name = "minimum hysteresis", },
    { .id = SENSORS_SUBFEATURE_TEMP_LCRIT_HYST, .name = "critical low hysteresis", },
    { .id = SENSORS_SUBFEATURE_TEMP_ALARM, .name = "alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_MAX_ALARM, .name = "alarm high", },
    { .id = SENSORS_SUBFEATURE_TEMP_MIN_ALARM, .name = "alarm low", },
    { .id = SENSORS_SUBFEATURE_TEMP_CRIT_ALARM, .name = "critical alarm high", },
    { .id = SENSORS_SUBFEATURE_TEMP_FAULT, .name = "fault", },
    { .id = SENSORS_SUBFEATURE_TEMP_TYPE, .name = "type", },
    { .id = SENSORS_SUBFEATURE_TEMP_OFFSET, .name = "offset", },
    { .id = SENSORS_SUBFEATURE_TEMP_BEEP, .name = "beep", },
    { .id = SENSORS_SUBFEATURE_TEMP_EMERGENCY_ALARM, .name = "emergency alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_LCRIT_ALARM, .name = "critical alarm low", },

    // Power subfeatures
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE, .name = "average", },
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE_HIGHEST, .name = "average highest", },
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE_LOWEST, .name = "average lowest", },
    { .id = SENSORS_SUBFEATURE_POWER_INPUT, .name = "input", },
    { .id = SENSORS_SUBFEATURE_POWER_INPUT_HIGHEST, .name = "input highest", },
    { .id = SENSORS_SUBFEATURE_POWER_INPUT_LOWEST, .name = "input lowest", },
    { .id = SENSORS_SUBFEATURE_POWER_CAP, .name = "cap", },
    { .id = SENSORS_SUBFEATURE_POWER_CAP_HYST, .name = "cap hysteresis", },
    { .id = SENSORS_SUBFEATURE_POWER_MAX, .name = "maximum", },
    { .id = SENSORS_SUBFEATURE_POWER_CRIT, .name = "critical high", },
    { .id = SENSORS_SUBFEATURE_POWER_MIN, .name = "minimum", },
    { .id = SENSORS_SUBFEATURE_POWER_LCRIT, .name = "critical low", },
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE_INTERVAL, .name = "average interval", },
    { .id = SENSORS_SUBFEATURE_POWER_ALARM, .name = "alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_CAP_ALARM, .name = "cap alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_MAX_ALARM, .name = "alarm high", },
    { .id = SENSORS_SUBFEATURE_POWER_CRIT_ALARM, .name = "critical alarm high", },
    { .id = SENSORS_SUBFEATURE_POWER_MIN_ALARM, .name = "alarm low", },
    { .id = SENSORS_SUBFEATURE_POWER_LCRIT_ALARM, .name = "critical alarm low", },

    // Energy subfeatures
    { .id = SENSORS_SUBFEATURE_ENERGY_INPUT, .name = "input", },

    // Current subfeatures
    { .id = SENSORS_SUBFEATURE_CURR_INPUT, .name = "input", },
    { .id = SENSORS_SUBFEATURE_CURR_MIN, .name = "minimum", },
    { .id = SENSORS_SUBFEATURE_CURR_MAX, .name = "maximum", },
    { .id = SENSORS_SUBFEATURE_CURR_LCRIT, .name = "critical low", },
    { .id = SENSORS_SUBFEATURE_CURR_CRIT, .name = "critical high", },
    { .id = SENSORS_SUBFEATURE_CURR_AVERAGE, .name = "average", },
    { .id = SENSORS_SUBFEATURE_CURR_LOWEST, .name = "lowest", },
    { .id = SENSORS_SUBFEATURE_CURR_HIGHEST, .name = "highest", },
    { .id = SENSORS_SUBFEATURE_CURR_ALARM, .name = "alarm", },
    { .id = SENSORS_SUBFEATURE_CURR_MIN_ALARM, .name = "alarm low", },
    { .id = SENSORS_SUBFEATURE_CURR_MAX_ALARM, .name = "alarm high", },
    { .id = SENSORS_SUBFEATURE_CURR_BEEP, .name = "beep", },
    { .id = SENSORS_SUBFEATURE_CURR_LCRIT_ALARM, .name = "critical alarm low", },
    { .id = SENSORS_SUBFEATURE_CURR_CRIT_ALARM, .name = "critical alarm high", },

    // Humidity subfeatures
    { .id = SENSORS_SUBFEATURE_HUMIDITY_INPUT, .name = "input", },

    // VID subfeatures
    { .id = SENSORS_SUBFEATURE_VID, .name = "value", },

    // Intrusion subfeatures
    { .id = SENSORS_SUBFEATURE_INTRUSION_ALARM, .name = "alarm", },
    { .id = SENSORS_SUBFEATURE_INTRUSION_BEEP, .name = "beep", },

    // Beep enable subfeatures
    { .id = SENSORS_SUBFEATURE_BEEP_ENABLE, .name = "enable", },

    // Unknown subfeature
    { .id = SENSORS_SUBFEATURE_UNKNOWN, .name = "unknown", },

    // terminator
    {.id = 0, .name = NULL}
};
ENUM_STR_DEFINE_FUNCTIONS(SENSOR_SUBFEATURE_TYPE, SENSORS_SUBFEATURE_UNKNOWN, "unknown");

typedef enum {
    SENSOR_STATE_NONE       = 0,        // unset
    SENSOR_STATE_CLEAR      = (1 << 0), // everything is good
    SENSOR_STATE_WARNING    = (1 << 1), // our own calculations indicate an alarm, but not the driver
    SENSOR_STATE_CAP        = (1 << 2), // our own calculations or the driver, indicate cap
    SENSOR_STATE_ALARM      = (1 << 3), // the kernel driver has raised an alarm
    SENSOR_STATE_CRITICAL   = (1 << 4), // our own calculations, or the driver, indicate a critical condition
    SENSOR_STATE_EMERGENCY  = (1 << 5), // our own calculations, or the driver, indicate an emergency
    SENSOR_STATE_FAULT      = (1 << 6), // our own calculations, or the driver, indicate a fault
} SENSOR_STATE;

ENUM_STR_MAP_DEFINE(SENSOR_STATE) = {
    { .id = SENSOR_STATE_CLEAR,     .name = "clear", },
    { .id = SENSOR_STATE_WARNING,   .name = "warning", },
    { .id = SENSOR_STATE_CAP,       .name = "cap", },
    { .id = SENSOR_STATE_ALARM,     .name = "alarm", },
    { .id = SENSOR_STATE_CRITICAL,  .name = "critical", },
    { .id = SENSOR_STATE_EMERGENCY, .name = "emergency", },
    { .id = SENSOR_STATE_FAULT,     .name = "fault", },

    // terminator
    { .id = 0, .name = NULL, },
};
ENUM_STR_DEFINE_FUNCTIONS(SENSOR_STATE, SENSOR_STATE_NONE, "unknown");

#define NOT_SUPPORTED (SENSORS_SUBFEATURE_UNKNOWN)

struct sensor_config {
    bool enabled;

    bool report_state;
    bool report_value;
    const char *title;
    const char *units;
    const char *context;
    const char *family;
    int priority;

    // sensor readings
    SENSOR_SUBFEATURE_TYPE input;
    SENSOR_SUBFEATURE_TYPE average;

    // thresholds
    SENSOR_SUBFEATURE_TYPE min;
    SENSOR_SUBFEATURE_TYPE max;
    SENSOR_SUBFEATURE_TYPE lcrit;
    SENSOR_SUBFEATURE_TYPE crit;
    SENSOR_SUBFEATURE_TYPE cap;
    SENSOR_SUBFEATURE_TYPE emergency;

    // alarms
    SENSOR_SUBFEATURE_TYPE fault;
    SENSOR_SUBFEATURE_TYPE alarm;
    SENSOR_SUBFEATURE_TYPE min_alarm;
    SENSOR_SUBFEATURE_TYPE max_alarm;
    SENSOR_SUBFEATURE_TYPE lcrit_alarm;
    SENSOR_SUBFEATURE_TYPE crit_alarm;
    SENSOR_SUBFEATURE_TYPE cap_alarm;
    SENSOR_SUBFEATURE_TYPE emergency_alarm;
} sensors_configurations[] = {
    [SENSORS_FEATURE_IN] = {
        .enabled = true,
        .title = "Sensor Voltage",
        .units = "Volts",
        .context = "system.hw.sensor.voltage",
        .family = "Voltage",
        .priority = 70002,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_IN_INPUT,
        .average = SENSORS_SUBFEATURE_IN_AVERAGE,

        .min = SENSORS_SUBFEATURE_IN_MIN,
        .max = SENSORS_SUBFEATURE_IN_MAX,
        .lcrit = SENSORS_SUBFEATURE_IN_LCRIT,
        .crit = SENSORS_SUBFEATURE_IN_CRIT,
        .cap = NOT_SUPPORTED,
        .emergency = NOT_SUPPORTED,

        .fault = NOT_SUPPORTED,
        .alarm = SENSORS_SUBFEATURE_IN_ALARM,
        .min_alarm = SENSORS_SUBFEATURE_IN_MIN_ALARM,
        .max_alarm = SENSORS_SUBFEATURE_IN_MAX_ALARM,
        .lcrit_alarm = SENSORS_SUBFEATURE_IN_LCRIT_ALARM,
        .crit_alarm = SENSORS_SUBFEATURE_IN_CRIT_ALARM,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = NOT_SUPPORTED,
    },

    [SENSORS_FEATURE_FAN] = {
        .enabled = true,
        .title = "Sensor Fan Speed",
        .units = "rotations per minute",
        .context = "system.hw.sensor.fan",
        .family = "Fan",
        .priority = 70005,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_FAN_INPUT,
        .average = NOT_SUPPORTED,

        .min = SENSORS_SUBFEATURE_FAN_MIN,
        .max = SENSORS_SUBFEATURE_FAN_MAX,
        .lcrit = NOT_SUPPORTED,
        .crit = NOT_SUPPORTED,
        .cap = NOT_SUPPORTED,
        .emergency = NOT_SUPPORTED,

        .fault = SENSORS_SUBFEATURE_FAN_FAULT,
        .alarm = SENSORS_SUBFEATURE_FAN_ALARM,
        .min_alarm = SENSORS_SUBFEATURE_FAN_MIN_ALARM,
        .max_alarm = SENSORS_SUBFEATURE_FAN_MAX_ALARM,
        .lcrit_alarm = NOT_SUPPORTED,
        .crit_alarm = NOT_SUPPORTED,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = NOT_SUPPORTED,
    },

    [SENSORS_FEATURE_TEMP] = {
        .enabled = true,
        .title = "Sensor Temperature",
        .units = "degrees Celsius",
        .context = "system.hw.sensor.temperature",
        .family = "Temperature",
        .priority = 70000,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_TEMP_INPUT,
        .average = NOT_SUPPORTED,

        .min = SENSORS_SUBFEATURE_TEMP_MIN,
        .max = SENSORS_SUBFEATURE_TEMP_MAX,
        .lcrit = SENSORS_SUBFEATURE_TEMP_LCRIT,
        .crit = SENSORS_SUBFEATURE_TEMP_CRIT,
        .cap = NOT_SUPPORTED,
        .emergency = SENSORS_SUBFEATURE_TEMP_EMERGENCY,

        .fault = SENSORS_SUBFEATURE_TEMP_FAULT,
        .alarm = SENSORS_SUBFEATURE_TEMP_ALARM,
        .min_alarm = SENSORS_SUBFEATURE_TEMP_MIN_ALARM,
        .max_alarm = SENSORS_SUBFEATURE_TEMP_MAX_ALARM,
        .lcrit_alarm = SENSORS_SUBFEATURE_TEMP_LCRIT_ALARM,
        .crit_alarm = SENSORS_SUBFEATURE_TEMP_CRIT_ALARM,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = SENSORS_SUBFEATURE_TEMP_EMERGENCY_ALARM,
    },

    [SENSORS_FEATURE_POWER] = {
        .enabled = true,
        .title = "Sensor Power",
        .units = "Watts",
        .context = "system.hw.sensor.power",
        .family = "Power",
        .priority = 70006,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_POWER_INPUT,
        .average = SENSORS_SUBFEATURE_POWER_AVERAGE,

        .min = SENSORS_SUBFEATURE_POWER_MIN,
        .max = SENSORS_SUBFEATURE_POWER_MAX,
        .lcrit = SENSORS_SUBFEATURE_POWER_LCRIT,
        .crit = SENSORS_SUBFEATURE_POWER_CRIT,
        .cap = SENSORS_SUBFEATURE_POWER_CAP,
        .emergency = NOT_SUPPORTED,

        .fault = NOT_SUPPORTED,
        .alarm = SENSORS_SUBFEATURE_POWER_ALARM,
        .min_alarm = SENSORS_SUBFEATURE_POWER_MIN_ALARM,
        .max_alarm = SENSORS_SUBFEATURE_POWER_MAX_ALARM,
        .lcrit_alarm = SENSORS_SUBFEATURE_POWER_LCRIT_ALARM,
        .crit_alarm = SENSORS_SUBFEATURE_POWER_CRIT_ALARM,
        .cap_alarm = SENSORS_SUBFEATURE_POWER_CAP_ALARM,
        .emergency_alarm = NOT_SUPPORTED,
    },

    [SENSORS_FEATURE_ENERGY] = {
        .enabled = true,
        .title = "Sensor Energy",
        .units = "Joules",
        .context = "system.hw.sensor.energy",
        .family = "Energy",
        .priority = 70007,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_ENERGY_INPUT,
        .average = NOT_SUPPORTED,

        .min = NOT_SUPPORTED,
        .max = NOT_SUPPORTED,
        .lcrit = NOT_SUPPORTED,
        .crit = NOT_SUPPORTED,
        .cap = NOT_SUPPORTED,
        .emergency = NOT_SUPPORTED,

        .fault = NOT_SUPPORTED,
        .alarm = NOT_SUPPORTED,
        .min_alarm = NOT_SUPPORTED,
        .max_alarm = NOT_SUPPORTED,
        .lcrit_alarm = NOT_SUPPORTED,
        .crit_alarm = NOT_SUPPORTED,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = NOT_SUPPORTED,
    },

    [SENSORS_FEATURE_CURR] = {
        .enabled = true,
        .title = "Sensor Current",
        .units = "Amperes",
        .context = "system.hw.sensor.current",
        .family = "Current",
        .priority = 70003,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_CURR_INPUT,
        .average = SENSORS_SUBFEATURE_CURR_AVERAGE,

        .min = SENSORS_SUBFEATURE_CURR_MIN,
        .max = SENSORS_SUBFEATURE_CURR_MAX,
        .lcrit = SENSORS_SUBFEATURE_CURR_LCRIT,
        .crit = SENSORS_SUBFEATURE_CURR_CRIT,
        .cap = NOT_SUPPORTED,
        .emergency = NOT_SUPPORTED,

        .fault = NOT_SUPPORTED,
        .alarm = SENSORS_SUBFEATURE_CURR_ALARM,
        .min_alarm = SENSORS_SUBFEATURE_CURR_MIN_ALARM,
        .max_alarm = SENSORS_SUBFEATURE_CURR_MAX_ALARM,
        .lcrit_alarm = SENSORS_SUBFEATURE_CURR_LCRIT_ALARM,
        .crit_alarm = SENSORS_SUBFEATURE_CURR_CRIT_ALARM,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = NOT_SUPPORTED,
    },

    [SENSORS_FEATURE_HUMIDITY] = {
        .enabled = true,
        .title = "Sensor Humidity",
        .units = "percentage",
        .context = "system.hw.sensor.humidity",
        .family = "Humidity",
        .priority = 70004,
        .report_value = true,
        .report_state = true,

        .input = SENSORS_SUBFEATURE_HUMIDITY_INPUT,
        .average = NOT_SUPPORTED,

        .min = NOT_SUPPORTED,
        .max = NOT_SUPPORTED,
        .lcrit = NOT_SUPPORTED,
        .crit = NOT_SUPPORTED,
        .cap = NOT_SUPPORTED,
        .emergency = NOT_SUPPORTED,

        .fault = NOT_SUPPORTED,
        .alarm = NOT_SUPPORTED,
        .min_alarm = NOT_SUPPORTED,
        .max_alarm = NOT_SUPPORTED,
        .lcrit_alarm = NOT_SUPPORTED,
        .crit_alarm = NOT_SUPPORTED,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = NOT_SUPPORTED,
    },

    [SENSORS_FEATURE_INTRUSION] = {
        .enabled = true,
        .title = "Sensor Intrusion",
        .units = "", // No specific unit, as this is a binary state
        .context = "system.hw.sensor.intrusion",
        .family = "Intrusion",
        .priority = 70008,
        .report_value = false, // there is not value in intrusion
        .report_state = true,

        .input = NOT_SUPPORTED,
        .average = NOT_SUPPORTED,

        .min = NOT_SUPPORTED,
        .max = NOT_SUPPORTED,
        .lcrit = NOT_SUPPORTED,
        .crit = NOT_SUPPORTED,
        .cap = NOT_SUPPORTED,
        .emergency = NOT_SUPPORTED,

        .fault = NOT_SUPPORTED,
        .alarm = SENSORS_SUBFEATURE_INTRUSION_ALARM,
        .min_alarm = NOT_SUPPORTED,
        .max_alarm = NOT_SUPPORTED,
        .lcrit_alarm = NOT_SUPPORTED,
        .crit_alarm = NOT_SUPPORTED,
        .cap_alarm = NOT_SUPPORTED,
        .emergency_alarm = NOT_SUPPORTED,
    },
};

typedef struct subfeature {
    STRING *name;
    bool read;
    double value;
} SUBFEATURE;
DEFINE_JUDYL_TYPED(SUBFEATURES, SUBFEATURE *);

typedef struct sensor {
    bool read;

    bool exposed_input;
    bool exposed_average;
    SENSOR_STATE exposed_states;

    // double divisor; // the divisor required to convert to base units
    double input;
    double average;

    STRING *id;

    struct {
        STRING *id;
        STRING *driver;
        STRING *adapter;
        STRING *path;
        STRING *device;
        STRING *subsystem;
        short bus;
        int addr;
    } chip;

    struct {
        SENSOR_TYPE type;
        STRING *name;
        STRING *label;
    } feature;

    SENSOR_STATE state;
    SENSOR_STATE state_logged;
    SENSOR_STATE supported_states;
    SUBFEATURES_JudyLSet values;

    struct sensor_config config;
    STRING *log_msg;
} SENSOR;

static inline msec_t chip_update_interval(const char *path, msec_t default_interval_ms) {
    char filename[FILENAME_MAX];
    snprintfz(filename, sizeof(filename), "%s/update_interval", path);

    unsigned long long result = 0;
    if(read_single_number_file(filename, &result) != 0)
        result = default_interval_ms;

    return result;
}

static STRING *get_device_name(const char *hwmon_path) {
    char device_path[FILENAME_MAX];
    char link_target[FILENAME_MAX];

    // Construct path to the device symlink
    snprintfz(device_path, sizeof(device_path), "%s/device", hwmon_path);

    // Read the symlink
    ssize_t len = readlink(device_path, link_target, sizeof(link_target) - 1);
    if (len < 0) return NULL;
    link_target[len] = '\0';

    // Extract the last component of the path
    char *last_slash = strrchr(link_target, '/');
    if (last_slash)
        return string_strdupz(last_slash + 1);

    return NULL;
}

static STRING *get_subsystem_name(const char *hwmon_path) {
    char device_path[FILENAME_MAX];
    char link_target[FILENAME_MAX];

    // Construct path to the device symlink
    snprintfz(device_path, sizeof(device_path), "%s/device/subsystem", hwmon_path);

    // Read the symlink
    ssize_t len = readlink(device_path, link_target, sizeof(link_target) - 1);
    if (len < 0) return NULL;
    link_target[len] = '\0';

    // Extract the last component of the path
    char *last_slash = strrchr(link_target, '/');
    if (last_slash)
        return string_strdupz(last_slash + 1);

    return NULL;
}

static inline bool sensor_subfeature_needed(SENSOR *s, SENSOR_SUBFEATURE_TYPE type) {
    return
        type != NOT_SUPPORTED &&
        (
            type == s->config.input ||
            type == s->config.average ||
            type == s->config.min ||
            type == s->config.max ||
            type == s->config.lcrit ||
            type == s->config.crit ||
            type == s->config.cap ||
            type == s->config.emergency ||
            type == s->config.fault ||
            type == s->config.alarm ||
            type == s->config.min_alarm ||
            type == s->config.max_alarm ||
            type == s->config.lcrit_alarm ||
            type == s->config.crit_alarm ||
            type == s->config.cap_alarm ||
            type == s->config.emergency_alarm
        );
}

static inline double sensor_value(SENSOR *s, SENSOR_SUBFEATURE_TYPE type) {
    double value = NAN;

    SUBFEATURE *sft = SUBFEATURES_GET(&s->values, type);
    if(sft && sft->read && !isinf(sft->value) && !isnan(sft->value))
        value = sft->value;

    return value;
}

static inline void transition_to_state(SENSOR *s) {
    if(s->state_logged == s->state) {
        string_freez(s->log_msg);
        s->log_msg = NULL;
        return;
    }

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &sensors_state_transition_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    ND_LOG_FIELD_PRIORITY prio;
    switch(s->state) {
        default:
        case SENSOR_STATE_CLEAR:
            prio = NDLP_NOTICE;
            break;

        case SENSOR_STATE_CAP:
        case SENSOR_STATE_WARNING:
            prio = NDLP_WARNING;
            break;

        case SENSOR_STATE_FAULT:
        case SENSOR_STATE_ALARM:
            prio = NDLP_ERR;
            break;

        case SENSOR_STATE_CRITICAL:
            prio = NDLP_CRIT;
            break;

        case SENSOR_STATE_EMERGENCY:
            prio = NDLP_ALERT;
            break;
    }

    nd_log(NDLS_COLLECTORS, prio,
           "%s sensor '%s' transitioned from state '%s' to '%s' [device '%s', driver '%s', subsystem '%s', path '%s']%s%s",
           SENSOR_TYPE_2str(s->feature.type),
           string2str(s->id),
           SENSOR_STATE_2str(s->state_logged), SENSOR_STATE_2str(s->state),
           string2str(s->chip.device), string2str(s->chip.driver),
           string2str(s->chip.subsystem), string2str(s->chip.path),
           s->log_msg ? ": " : "", string2str(s->log_msg));

    string_freez(s->log_msg);
    s->log_msg = NULL;

    s->state_logged = s->state;
}

static inline void check_value_greater_than_zero(SENSOR *s, SENSOR_SUBFEATURE_TYPE *config, SENSOR_STATE state) {
    if(*config == NOT_SUPPORTED)
        return;

    double status = sensor_value(s, *config);
    if(isnan(status)) {
        // we cannot read this
        // exclude it from future iterations for this sensor
        *config = NOT_SUPPORTED;
    }
    else {
        // the sensor supports this state
        s->supported_states |= state;

        // set it to this state if it is raised
        if(status > 0 && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;

            string_freez(s->log_msg);
            char buf[1024];
            snprintf(buf, sizeof(buf), "%s == %f (kernel driver generated)",
                     SENSOR_SUBFEATURE_TYPE_2str(*config), status);
            s->log_msg = string_strdupz(buf);
        }
    }
}

static void userspace_evaluation_log_msg(SENSOR *s, const char *reading_txt, const char *condition, const char *threshold_txt, double reading, double threshold) {
    string_freez(s->log_msg);
    char buf[1024];
    snprintf(buf, sizeof(buf), "%s %f %s %s %f (userspace evaluation using kernel provided thresholds)",
             reading_txt, reading, condition, threshold_txt, threshold);
    s->log_msg = string_strdupz(buf);
}

static inline void check_smaller_than_threshold(SENSOR *s, SENSOR_SUBFEATURE_TYPE *config, SENSOR_STATE state) {
    if(*config == NOT_SUPPORTED)
        return;

    double threshold = sensor_value(s, *config);
    if(isnan(threshold)) {
        // we cannot read this
        // exclude it from future iterations for this sensor
        *config = NOT_SUPPORTED;
    }
    else {
        // the sensor supports this state
        s->supported_states |= state;

        // set it to this state if it is raised
        if(s->input < threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "input", "<", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->input, threshold);
        }
        else if(s->average < threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "average", "<", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->average, threshold);
        }
    }
}

static inline void check_greater_than_threshold(SENSOR *s, SENSOR_SUBFEATURE_TYPE *config, SENSOR_STATE state) {
    if(*config == NOT_SUPPORTED)
        return;

    double threshold = sensor_value(s, *config);
    if(isnan(threshold)) {
        // we cannot read this
        // exclude it from future iterations for this sensor
        *config = NOT_SUPPORTED;
    }
    else {
        // the sensor supports this state
        s->supported_states |= state;

        // set it to this state if it is raised
        if(s->input > threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "input", ">", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->input, threshold);
        }
        else if(s->average > threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "average", ">", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->average, threshold);
        }
    }
}

static inline void check_smaller_or_equal_to_threshold(SENSOR *s, SENSOR_SUBFEATURE_TYPE *config, SENSOR_STATE state) {
    if(*config == NOT_SUPPORTED)
        return;

    double threshold = sensor_value(s, *config);
    if(isnan(threshold)) {
        // we cannot read this
        // exclude it from future iterations for this sensor
        *config = NOT_SUPPORTED;
    }
    else {
        // the sensor supports this state
        s->supported_states |= state;

        // set it to this state if it is raised
        if(s->input <= threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "input", "<=", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->input, threshold);
        }
        else if(s->average <= threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "average", "<=", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->average, threshold);
        }
    }
}

static inline void check_greater_or_equal_to_threshold(SENSOR *s, SENSOR_SUBFEATURE_TYPE *config, SENSOR_STATE state) {
    if(*config == NOT_SUPPORTED)
        return;

    double threshold = sensor_value(s, *config);
    if(isnan(threshold)) {
        // we cannot read this
        // exclude it from future iterations for this sensor
        *config = NOT_SUPPORTED;
    }
    else {
        // the sensor supports this state
        s->supported_states |= state;

        // set it to this state if it is raised
        if(s->input >= threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "input", ">=", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->input, threshold);
        }
        else if(s->average >= threshold && s->state == SENSOR_STATE_CLEAR) {
            s->state = state;
            userspace_evaluation_log_msg(
                s, "average", ">=", SENSOR_SUBFEATURE_TYPE_2str(*config),
                s->average, threshold);
        }
    }
}

static void set_sensor_state(SENSOR *s) {
    s->supported_states = SENSOR_STATE_CLEAR;
    s->state = SENSOR_STATE_CLEAR;

    // ----------------------------------------------------------------------------------------------------------------
    // read the values

    if(s->config.input != NOT_SUPPORTED) {
        s->input = sensor_value(s, s->config.input);
        if(isnan(s->input) && !s->exposed_input) {
            s->config.input = NOT_SUPPORTED;
            s->input = NAN;
        }
    }
    
    if(s->config.average != NOT_SUPPORTED) {
        s->average = sensor_value(s, s->config.average);
        if(isnan(s->average) && !s->exposed_average) {
            s->config.average = NOT_SUPPORTED;
            s->average = NAN;
        }
    }

    // ----------------------------------------------------------------------------------------------------------------
    // read the sensor alarms as exposed by the kernel driver

    check_value_greater_than_zero(s, &s->config.fault, SENSOR_STATE_FAULT);
    check_value_greater_than_zero(s, &s->config.emergency_alarm, SENSOR_STATE_EMERGENCY);
    check_value_greater_than_zero(s, &s->config.crit_alarm, SENSOR_STATE_CRITICAL);
    check_value_greater_than_zero(s, &s->config.lcrit_alarm, SENSOR_STATE_CRITICAL);
    check_value_greater_than_zero(s, &s->config.max_alarm, SENSOR_STATE_ALARM);
    check_value_greater_than_zero(s, &s->config.min_alarm, SENSOR_STATE_ALARM);
    check_value_greater_than_zero(s, &s->config.alarm, SENSOR_STATE_ALARM);
    check_value_greater_than_zero(s, &s->config.cap_alarm, SENSOR_STATE_CAP);

#ifdef NETDATA_CALCULATED_STATES

    // ----------------------------------------------------------------------------------------------------------------
    // our custom logic for triggering state changes

    // if the sensor is already exposed to netdata, but now it cannot give values,
    // set it to faulty state
    s->supported_states |= SENSOR_STATE_FAULT;
    if(isnan(s->input) && isnan(s->average) && (s->exposed_input || s->exposed_average) &&
        s->state == SENSOR_STATE_CLEAR) {
        s->state = SENSOR_STATE_FAULT;
    }

    check_greater_or_equal_to_threshold(s, &s->config.emergency, SENSOR_STATE_EMERGENCY);
    check_greater_or_equal_to_threshold(s, &s->config.crit, SENSOR_STATE_CRITICAL);
    check_smaller_or_equal_to_threshold(s, &s->config.lcrit, SENSOR_STATE_CRITICAL);
    check_greater_than_threshold(s, &s->config.cap, SENSOR_STATE_CAP);
    check_greater_than_threshold(s, &s->config.max, SENSOR_STATE_WARNING);
    check_smaller_than_threshold(s, &s->config.min, SENSOR_STATE_WARNING);

#endif

    // ----------------------------------------------------------------------------------------------------------------
    // log any transitions

    transition_to_state(s);
}

static SENSOR *sensor_get_or_create(DICTIONARY *dict, const sensors_chip_name *chip, const sensors_feature *feature) {
    static __thread char buf[4096];

    struct sensor_config *config = NULL;
    if(feature->type < _countof(sensors_configurations))
        config = &sensors_configurations[feature->type];

    if(!config || !config->enabled)
        return NULL;

    snprintfz(buf, sizeof(buf),
              "%s|%s-%d-%d-%s",
              chip->path, chip->prefix, chip->bus.type, chip->addr, feature->name);

    SENSOR *s = dictionary_get(dict, buf);
    if(s) return s;

    s = dictionary_set(dict, buf, NULL, sizeof(SENSOR));
    s->config = *config;
    s->state_logged = SENSOR_STATE_CLEAR;
    s->input = NAN;
    s->average = NAN;
    s->feature.label = NULL;

    sensors_snprintf_chip_name(buf, sizeof(buf), chip);
    s->chip.id = string_strdupz(buf);
    s->chip.driver = string_strdupz(chip->prefix);
    s->chip.adapter = string_strdupz(sensors_get_adapter_name(&chip->bus));
    s->chip.path = string_strdupz(chip->path);
    s->chip.device = get_device_name(chip->path);
    s->chip.subsystem = get_subsystem_name(chip->path);
    s->chip.bus = chip->bus.type;
    s->chip.addr = chip->addr;

    s->feature.name = string_strdupz(feature->name);
    s->feature.type = feature->type;

    // returns feature->name if no label
    // https://github.com/lm-sensors/lm-sensors/blob/master/lib/access.c#L199-L200
    const char *label = sensors_get_label(chip, feature);

    if (label && strcmp(label, string2str(s->feature.name)) != 0) {
        s->feature.label = string_strdupz(label);
        snprintfz(
            buf,
            sizeof(buf),
            "%s_%s_%s_%s",
            SENSOR_TYPE_2str(s->feature.type),
            string2str(s->chip.id),
            string2str(s->feature.name),
            string2str(s->feature.label));
    } else {
        snprintfz(
            buf,
            sizeof(buf),
            "%s_%s_%s",
            SENSOR_TYPE_2str(s->feature.type),
            string2str(s->chip.id),
            string2str(s->feature.name));
    }

    // we have to free this, because it is malloced from libsensors
    if(label)
        free((void *)label); // do not use freez() here - libsensors uses malloc()

    netdata_fix_chart_id(buf);
    s->id = string_strdupz(buf);

    return s;
}

static void sensor_labels(SENSOR *ft) {
    printf(PLUGINSD_KEYWORD_CLABEL " feature '%s' 1\n", string2str(ft->feature.name));
    printf(PLUGINSD_KEYWORD_CLABEL " label '%s' 1\n", string2str(ft->feature.label));
    printf(PLUGINSD_KEYWORD_CLABEL " chip_id '%s' 1\n", string2str(ft->chip.id));
    printf(PLUGINSD_KEYWORD_CLABEL " path '%s' 1\n", string2str(ft->chip.path));
    printf(PLUGINSD_KEYWORD_CLABEL " subsystem '%s' 1\n", string2str(ft->chip.subsystem));
    printf(PLUGINSD_KEYWORD_CLABEL " driver '%s' 1\n", string2str(ft->chip.driver));

//    printf(
//        PLUGINSD_KEYWORD_CLABEL " sensor '%s - %s' 1\n",
//        string2str(ft->chip.name),
//        string2str(ft->feature.label));

    printf(PLUGINSD_KEYWORD_CLABEL_COMMIT "\n");
}

static size_t states_count(SENSOR_STATE state) {
    // the gcc way
    return __builtin_popcount(state);

//    size_t count = 0;
//    while (state) {
//        state &= (state - 1);  // Clear the least significant set bit
//        count++;
//    }
//    return count;
}

static void sensor_process(SENSOR *s, int update_every, const char *name) {
    // evaluate the state of the feature
    set_sensor_state(s);
    internal_fatal(s->state == 0,
                   "SENSORS: state %u is not a valid state", s->state);

    internal_fatal((s->state & s->supported_states) == 0,
                   "SENSORS: state %u is not in the supported list of states %u",
        s->state,
        s->supported_states);

    bool do_input = s->config.report_value && !isnan(s->input);
    bool do_average = s->config.report_value && !isnan(s->average);
    bool do_state = s->config.report_state && states_count(s->supported_states) > 1;

    // send the feature data to netdata
    if(do_input && !s->exposed_input) {
        printf(
            PLUGINSD_KEYWORD_CHART " 'sensors.%s_input' '' '%s' '%s' '%s' '%s.input' line %d %d '' debugfs %s\n",
            string2str(s->id),
            s->config.title,
            s->config.units,
            s->config.family,
            s->config.context,
            s->config.priority,
            update_every,
            name);

        printf(PLUGINSD_KEYWORD_DIMENSION " input '' absolute 1 10000 ''\n");
        sensor_labels(s);
        s->exposed_input = true;
    }

    if(do_average && !s->exposed_average) {
        printf(
            PLUGINSD_KEYWORD_CHART " 'sensors.%s_average' '' '%s Average' '%s' '%s' '%s.average' line %d %d '' debugfs %s\n",
            string2str(s->id),
            s->config.title,
            s->config.units,
            s->config.family,
            s->config.context,
            s->config.priority + 1,
            update_every,
            name);

        printf(PLUGINSD_KEYWORD_DIMENSION " average '' absolute 1 10000 ''\n");
        sensor_labels(s);
        s->exposed_average = true;
    }

    if(do_state && s->exposed_states != s->supported_states) {
        printf(
            PLUGINSD_KEYWORD_CHART " 'sensors.%s_alarm' '' '%s Alarm Status' '%s' '%s' '%s.alarm' line %d %d '' debugfs %s\n",
            string2str(s->id),
            s->config.title,
            "status",
            s->config.family,
            s->config.context,
            s->config.priority + 2,
            update_every,
            name);

        if(s->supported_states & SENSOR_STATE_CLEAR)
            printf(PLUGINSD_KEYWORD_DIMENSION " clear '' absolute 1 1 ''\n");
        if(s->supported_states & SENSOR_STATE_WARNING)
            printf(PLUGINSD_KEYWORD_DIMENSION " warning '' absolute 1 1 ''\n");
        if(s->supported_states & SENSOR_STATE_CAP)
            printf(PLUGINSD_KEYWORD_DIMENSION " cap '' absolute 1 1 ''\n");
        if(s->supported_states & SENSOR_STATE_ALARM)
            printf(PLUGINSD_KEYWORD_DIMENSION " alarm '' absolute 1 1 ''\n");
        if(s->supported_states & SENSOR_STATE_CRITICAL)
            printf(PLUGINSD_KEYWORD_DIMENSION " critical '' absolute 1 1 ''\n");
        if(s->supported_states & SENSOR_STATE_EMERGENCY)
            printf(PLUGINSD_KEYWORD_DIMENSION " emergency '' absolute 1 1 ''\n");
        if(s->supported_states & SENSOR_STATE_FAULT)
            printf(PLUGINSD_KEYWORD_DIMENSION " fault '' absolute 1 1 ''\n");

        sensor_labels(s);
        s->exposed_states = s->supported_states;
    }

#if 0
    // ----------------------------------------------------------------------------------------------------------------
    // debugging

    fprintf(stderr,
            "SENSORS: "
            "{ chip id '%s', name '%s', addr %d }, "
            "{ adapter '%s', bus '%s', path '%s'}, "
            "{ feature label '%s', name '%s', type '%s' }\n",
            string2str(s->chip.id),
            string2str(s->chip.driver),
        s->chip.addr,
            string2str(s->chip.adapter),
            SENSOR_BUS_TYPE_2str(s->chip.bus),
            string2str(s->chip.path),
            string2str(s->feature.label),
            string2str(s->feature.name),
            SENSOR_TYPE_2str(s->feature.type));

    Word_t idx = 0;
    for(SUBFEATURE *sft = SUBFEATURES_FIRST(&s->values, &idx);
         sft;
         sft = SUBFEATURES_NEXT(&s->values, &idx)) {
        fprintf(stderr,
                " ------------ >>> "
                "{ subfeature '%s', type '%s' } "
                "value %f, %s\n",
                string2str(sft->name), SENSOR_SUBFEATURE_TYPE_2str(idx),
                sft->value, sft->read ? "OK" : "FAILED");
    }

    if(do_input)
        fprintf(stderr, " ------------ >>> %f (input)\n", s->input);

    if(do_average)
        fprintf(stderr, " ------------ >>> %f (average)\n", s->average);

    if(do_state)
        fprintf(stderr, " ------------ >>> %u (state)\n", s->state);
#endif

    // ----------------------------------------------------------------------------------------------------------------
    // send the data

    if(do_input) {
        printf(PLUGINSD_KEYWORD_BEGIN " 'sensors.%s_input'\n", string2str(s->id));

        printf(PLUGINSD_KEYWORD_SET " input = %lld\n", (long long)(s->input * 10000.0));
        printf(PLUGINSD_KEYWORD_END "\n");
    }

    if(do_average) {
        printf(PLUGINSD_KEYWORD_BEGIN " 'sensors.%s_average'\n", string2str(s->id));

        printf(PLUGINSD_KEYWORD_SET " average = %lld\n", (long long)(s->average * 10000.0));
        printf(PLUGINSD_KEYWORD_END "\n");
    }

    if(do_state) {
        printf(
            PLUGINSD_KEYWORD_BEGIN " 'sensors.%s_alarm'\n",
            string2str(s->id));

        if(s->supported_states & SENSOR_STATE_CLEAR)
            printf(PLUGINSD_KEYWORD_SET " clear = %d\n", s->state == SENSOR_STATE_CLEAR ? 1 : 0);
        if(s->supported_states & SENSOR_STATE_WARNING)
            printf(PLUGINSD_KEYWORD_SET " warning = %d\n", s->state == SENSOR_STATE_WARNING ? 1 : 0);
        if(s->supported_states & SENSOR_STATE_CAP)
            printf(PLUGINSD_KEYWORD_SET " cap = %d\n", s->state == SENSOR_STATE_CAP ? 1 : 0);
        if(s->supported_states & SENSOR_STATE_ALARM)
            printf(PLUGINSD_KEYWORD_SET " alarm = %d\n", s->state == SENSOR_STATE_ALARM ? 1 : 0);
        if(s->supported_states & SENSOR_STATE_CRITICAL)
            printf(PLUGINSD_KEYWORD_SET " critical = %d\n", s->state == SENSOR_STATE_CRITICAL ? 1 : 0);
        if(s->supported_states & SENSOR_STATE_EMERGENCY)
            printf(PLUGINSD_KEYWORD_SET " emergency = %d\n", s->state == SENSOR_STATE_EMERGENCY ? 1 : 0);
        if(s->supported_states & SENSOR_STATE_FAULT)
            printf(PLUGINSD_KEYWORD_SET " fault = %d\n", s->state == SENSOR_STATE_FAULT ? 1 : 0);

        printf(PLUGINSD_KEYWORD_END "\n");
    }
}

static FILE *sensors_open_file(const char *env_var, const char *def_dir, const char *file) {
    const char *dir = getenv(env_var);
    if(!dir || !*dir)
        dir = def_dir;

    if (dir && *dir) {
        char filename[FILENAME_MAX];
        snprintfz(filename, sizeof(filename), "%s/%s", dir, file);
        return fopen(filename, "r");
    }

    return NULL;
}

static DICTIONARY *sensors_dict = NULL;

static int sensors_collect_data(void) {
    // ----------------------------------------------------------------------------------------------------------------
    // reset all sensors to unread

    SENSOR *s;
    dfe_start_read(sensors_dict, s) {
        s->read = false;
    }
    dfe_done(s);

    // ----------------------------------------------------------------------------------------------------------------
    // Iterate over all detected chips

    size_t subfeatures_collected = 0;

    const sensors_chip_name *chip;
    int chip_nr = 0;
    while ((chip = sensors_get_detected_chips(NULL, &chip_nr)) != NULL) {

        // Iterate over all features of the chip
        const sensors_feature *feature;
        int feature_nr = 0;
        while ((feature = sensors_get_features(chip, &feature_nr)) != NULL) {
            s = sensor_get_or_create(sensors_dict, chip, feature);
            if(!s) continue;

            internal_fatal(s->read, "The features key is not unique!");
            s->read = true;

            // --------------------------------------------------------------------------------------------------------
            // mark all existing subfeatures as unread

            Word_t idx = 0;
            for(SUBFEATURE *sf = SUBFEATURES_FIRST(&s->values, &idx); sf; sf = SUBFEATURES_NEXT(&s->values, &idx)) {
                sf->read = false;
                sf->value = NAN;
            }

            // --------------------------------------------------------------------------------------------------------
            // iterate over all subfeatures of the feature

            const sensors_subfeature *subfeature;
            int subfeature_nr = 0;
            while ((subfeature = sensors_get_all_subfeatures(chip, feature, &subfeature_nr)) != NULL) {
                if(!(subfeature->flags & SENSORS_MODE_R) ||         // not readable
                    !sensor_subfeature_needed(s, subfeature->type)) // we don't need it
                    continue;

                SUBFEATURE *sft = SUBFEATURES_GET(&s->values, subfeature->type);
                if(!sft) {
                    sft = callocz(1, sizeof(*sft));
                    sft->name = string_strdupz(subfeature->name);
                    SUBFEATURES_SET(&s->values, subfeature->type, sft);
                }

                if (sensors_get_value(chip, subfeature->number, &sft->value) == 0) {
                    sft->read = true;
                    subfeatures_collected++;
                }
                else {
                    sft->value = NAN;
                    sft->read = false;
                }
            }
        }
    }

    return subfeatures_collected ? 0 : 1;
}

static bool libsensors_running = false;
static int libsensors_update_every = 1;

void libsensors_thread(void *ptr __maybe_unused) {
    int update_every = libsensors_update_every;

    FILE *fp = NULL;
    if(access("/etc/sensors3.conf", R_OK) != 0 &&
        access("/etc/sensors.conf", R_OK) != 0 &&
        access("/etc/sensors.d", R_OK | X_OK) != 0) {
                fp = sensors_open_file("NETDATA_CONFIG_DIR", CONFIG_DIR, "../sensors3.conf");
        if(!fp) fp = sensors_open_file("NETDATA_CONFIG_DIR", CONFIG_DIR, "sensors3.conf");
        if(!fp) fp = sensors_open_file("NETDATA_STOCK_CONFIG_DIR", LIBCONFIG_DIR, "sensors3.conf");
    }

    if (sensors_init(fp) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot initialize libsensors - disabling sensors monitoring");
        if(fp) fclose(fp);
        goto cleanup;
    }
    if(fp) fclose(fp);

    sensors_dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE, NULL, sizeof(SENSOR));

    // preflight to check data collection latency
    {
        SENSOR *s;

        sensors_collect_data(); // do the first before starting measurements
        dfe_start_read(sensors_dict, s) { set_sensor_state(s); } dfe_done(s);

        usec_t max_ut = 0;
        size_t samples = 0;
        usec_t started_ut = now_monotonic_usec();
        for(size_t i = 0; i < 5; i++) {
            usec_t before_ut = now_monotonic_usec();
            sensors_collect_data();
            dfe_start_read(sensors_dict, s) { set_sensor_state(s); } dfe_done(s);
            usec_t after_ut = now_monotonic_usec();
            max_ut = MAX(max_ut, after_ut - before_ut);
            samples++;
        }
        usec_t ended_ut = now_monotonic_usec();
        usec_t average_ut = (ended_ut - started_ut) / samples;

        if(average_ut < 1)
            average_ut = 1;

        if(max_ut < 1)
            max_ut = 1;

        // List of valid intervals in seconds (divisors and multiples of 60)
        static const int valid_update_every_intervals[] = {1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60, 120, 180, 240, 300, 600, 900, 1200, 1800, 3600};

        // Find the smallest valid interval that satisfies our timing requirement
        int best_update_every = update_every;
        for(size_t i = 0; i < _countof(valid_update_every_intervals); i++) {
            if(valid_update_every_intervals[i] >= update_every &&
                max_ut <= (valid_update_every_intervals[i] * USEC_PER_SEC / 5)) {
                best_update_every = valid_update_every_intervals[i];
                break;
            }
        }

        char avg[64], max[64];
        duration_snprintf(avg, sizeof(avg), (int64_t)average_ut, "us", false);
        duration_snprintf(max, sizeof(max), (int64_t)max_ut, "us", false);

        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "SENSORS max data collection latency is %s (average %s), setting update_every to %ds (default is %ds)",
               max, avg, best_update_every, update_every);

        update_every = best_update_every;
    }

    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);

    while(!nd_thread_signaled_to_cancel()) {
        heartbeat_next(&hb);

        if(sensors_collect_data())
            break;

        netdata_mutex_lock(&stdout_mutex);

        SENSOR *s;
        dfe_start_read(sensors_dict, s) {
            sensor_process(s, update_every, "sensors");
        }
        dfe_done(s);

        fflush(stdout);
        netdata_mutex_unlock(&stdout_mutex);
    }

cleanup:
    libsensors_running = false;

    dictionary_destroy(sensors_dict);
    sensors_dict = NULL;
}

static ND_THREAD *libsensors = NULL;
int do_module_libsensors(int update_every, const char *name __maybe_unused) {
    if(!libsensors) {
        libsensors_update_every = update_every;
        libsensors_running = true;
        libsensors = nd_thread_create("LIBSENSORS", NETDATA_THREAD_OPTION_DEFAULT, libsensors_thread, NULL);
    }

    return libsensors && libsensors_running ? 0 : 1;
}

void module_libsensors_cleanup(void) {
    nd_thread_signal_cancel(libsensors);
    nd_thread_join(libsensors);
}
