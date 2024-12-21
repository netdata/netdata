// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

#if defined(HAVE_LIBSENSORS)

#define NETDATA_CALCULATED_STATES 1

#include <sensors/sensors.h>
#include <sensors/error.h>

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

typedef sensors_feature_type SENSOR_FEATURE_TYPE;
ENUM_STR_MAP_DEFINE(SENSOR_FEATURE_TYPE) = {
    { .id = SENSORS_FEATURE_IN, .name = "in", },
    { .id = SENSORS_FEATURE_FAN, .name = "fan", },
    { .id = SENSORS_FEATURE_TEMP, .name = "temp", },
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
ENUM_STR_DEFINE_FUNCTIONS(SENSOR_FEATURE_TYPE, SENSORS_FEATURE_UNKNOWN, "unknown");

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

typedef struct subfeature {
    STRING *name;
    bool read;
    double value;
} SUBFEATURE;

DEFINE_JUDYL_TYPED(SUBFEATURES, SUBFEATURE *);

typedef enum {
    FEATURE_STATE_NONE      = 0,
    FEATURE_STATE_CLEAR     = (1 << 0),
    FEATURE_STATE_CAP       = (1 << 1),
    FEATURE_STATE_ALARM     = (1 << 2),
    FEATURE_STATE_CRITICAL  = (1 << 3),
    FEATURE_STATE_EMERGENCY = (1 << 4),
    FEATURE_STATE_FAULT     = (1 << 5),
} FEATURE_STATE;

typedef struct sensor {
    bool sent_to_netdata;
    bool read;

    bool report_state;
    bool report_value;

    bool exposed_input;
    bool exposed_average;
    bool exposed_state;

    double input;
    double average;

    STRING *title;
    STRING *units;
    STRING *family;
    STRING *context;
    int priority;

    struct {
        STRING *id;
        STRING *name;
        STRING *adapter;
        STRING *path;
        short bus;
        int addr;
    } chip;

    struct {
        SENSOR_FEATURE_TYPE type;
        STRING *name;
        STRING *label;
        STRING *label_sanitized;
    } feature;

    FEATURE_STATE state;
    FEATURE_STATE supported_states;
    SUBFEATURES_JudyLSet values;
} SENSOR;

static inline double sft_value(SENSOR *ft, SENSOR_SUBFEATURE_TYPE type) {
    double value = NAN;

    SUBFEATURE *sft = SUBFEATURES_GET(&ft->values, type);
    if(sft && sft->read && !isinf(sft->value) && !isnan(sft->value))
        value = sft->value;

    return value;
}

static void set_sensor_state(SENSOR *ft) {
    ft->state = FEATURE_STATE_CLEAR;

#ifdef NETDATA_CALCULATED_STATES
    FEATURE_STATE custom_states = FEATURE_STATE_FAULT;
#else
    FEATURE_STATE custom_states = FEATURE_STATE_NONE;
#endif

    switch(ft->feature.type) {
        case SENSORS_FEATURE_IN: {
            ft->supported_states = FEATURE_STATE_CLEAR;

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_IN_INPUT);
            ft->average = sft_value(ft, SENSORS_SUBFEATURE_IN_AVERAGE);

            double lcrit_alarm = sft_value(ft, SENSORS_SUBFEATURE_IN_LCRIT_ALARM);
            double crit_alarm = sft_value(ft, SENSORS_SUBFEATURE_IN_CRIT_ALARM);
            if(!isnan(lcrit_alarm) || !isnan(crit_alarm))
                ft->supported_states |= FEATURE_STATE_CRITICAL;

            double alarm = sft_value(ft, SENSORS_SUBFEATURE_IN_ALARM);
            double min_alarm = sft_value(ft, SENSORS_SUBFEATURE_IN_MIN_ALARM);
            double max_alarm = sft_value(ft, SENSORS_SUBFEATURE_IN_MAX_ALARM);
            if(!isnan(alarm) || !isnan(min_alarm) || !isnan(max_alarm))
                ft->supported_states |= FEATURE_STATE_ALARM;

            // the following are kernel driver reported states
            if (lcrit_alarm > 0 || crit_alarm > 0)
                ft->state = FEATURE_STATE_CRITICAL;
            else if (alarm > 0 || min_alarm > 0 || max_alarm > 0)
                ft->state = FEATURE_STATE_ALARM;

#ifdef NETDATA_CALCULATED_STATES
            double lcrit = sft_value(ft, SENSORS_SUBFEATURE_IN_LCRIT);
            double crit = sft_value(ft, SENSORS_SUBFEATURE_IN_CRIT);
            double min = sft_value(ft, SENSORS_SUBFEATURE_IN_MIN);
            double max = sft_value(ft, SENSORS_SUBFEATURE_IN_MAX);

            if((isnan(lcrit_alarm) && !isnan(lcrit)) ||
                (isnan(crit_alarm) && !isnan(crit)))
                ft->supported_states |= FEATURE_STATE_CRITICAL;

            if((isnan(min_alarm) && !isnan(min)) ||
                (isnan(max_alarm) && !isnan(max)))
                ft->supported_states |= FEATURE_STATE_ALARM;

            ft->supported_states |= FEATURE_STATE_FAULT;

            if(ft->state == FEATURE_STATE_CLEAR) {
                if (ft->exposed_input && isnan(ft->input)) {
                    ft->state = FEATURE_STATE_FAULT;
                } else if (ft->exposed_average && isnan(ft->average)) {
                    ft->state = FEATURE_STATE_FAULT;
                } else if (!isnan(ft->input) && isnan(lcrit_alarm) && ft->input < lcrit)
                    ft->state = FEATURE_STATE_CRITICAL;
                else if (!isnan(ft->input) && isnan(crit_alarm) && ft->input >= crit)
                    ft->state = FEATURE_STATE_CRITICAL;
                else if (!isnan(ft->input) && isnan(min_alarm) && ft->input < min)
                    ft->state = FEATURE_STATE_ALARM;
                else if (!isnan(ft->input) && isnan(max_alarm) && ft->input >= max)
                    ft->state = FEATURE_STATE_ALARM;
            }
#endif
            break;
        }

        case SENSORS_FEATURE_FAN:
            ft->supported_states = (FEATURE_STATE_CLEAR|FEATURE_STATE_ALARM| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_FAN_INPUT);
            ft->average = NAN;

#ifdef NETDATA_CALCULATED_STATES
            if(isnan(ft->input))
                ft->state = FEATURE_STATE_FAULT;
#endif

            // the following are kernel driver reported states
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_FAN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_FAN_MIN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_FAN_MAX_ALARM) > 0)
                ft->state = FEATURE_STATE_ALARM;

#ifdef NETDATA_CALCULATED_STATES
            // the following are Netdata calculated alarm states
            else if(
                isnan(sft_value(ft, SENSORS_SUBFEATURE_FAN_MIN_ALARM)) &&
                ft->input < sft_value(ft, SENSORS_SUBFEATURE_FAN_MIN))
                ft->state = FEATURE_STATE_ALARM;
            else if(
                isnan(sft_value(ft, SENSORS_SUBFEATURE_FAN_MAX_ALARM)) &&
                ft->input >= sft_value(ft, SENSORS_SUBFEATURE_FAN_MAX))
                ft->state = FEATURE_STATE_ALARM;
#endif
            break;

        case SENSORS_FEATURE_TEMP:
            ft->supported_states = (FEATURE_STATE_CLEAR|FEATURE_STATE_FAULT|FEATURE_STATE_EMERGENCY|FEATURE_STATE_CRITICAL|FEATURE_STATE_ALARM| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_TEMP_INPUT);
            ft->average = NAN;

#ifdef NETDATA_CALCULATED_STATES
            if(isnan(ft->input)) {
                ft->state = FEATURE_STATE_FAULT;
                return;
            }
#endif

            if(sft_value(ft, SENSORS_SUBFEATURE_TEMP_FAULT) > 0) {
                ft->input = NAN;
                ft->state = FEATURE_STATE_FAULT;
                return;
            }

            // the following are kernel driver reported states
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_EMERGENCY_ALARM) > 0)
                ft->state = FEATURE_STATE_EMERGENCY;
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_CRIT_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_LCRIT_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_CRIT_ALARM) > 0)
                ft->state = FEATURE_STATE_CRITICAL;
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_MIN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_MAX_ALARM) > 0)
                ft->state = FEATURE_STATE_ALARM;

#ifdef NETDATA_CALCULATED_STATES
            // the following are Netdata calculated alarm states
            else if(
                isnan(sft_value(ft, SENSORS_SUBFEATURE_TEMP_LCRIT_ALARM)) &&
                ft->input < sft_value(ft, SENSORS_SUBFEATURE_TEMP_LCRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if(
                isnan(sft_value(ft, SENSORS_SUBFEATURE_TEMP_CRIT_ALARM)) &&
                ft->input >= sft_value(ft, SENSORS_SUBFEATURE_TEMP_CRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if(
                isnan(sft_value(ft, SENSORS_SUBFEATURE_TEMP_MIN_ALARM)) &&
                ft->input < sft_value(ft, SENSORS_SUBFEATURE_TEMP_MIN))
                ft->state = FEATURE_STATE_ALARM;
            else if(
                isnan(sft_value(ft, SENSORS_SUBFEATURE_TEMP_MAX_ALARM)) &&
                ft->input >= sft_value(ft, SENSORS_SUBFEATURE_TEMP_MAX))
                ft->state = FEATURE_STATE_ALARM;
#endif
            break;

        case SENSORS_FEATURE_POWER: {
            ft->supported_states = (FEATURE_STATE_CLEAR|FEATURE_STATE_CRITICAL|FEATURE_STATE_ALARM|FEATURE_STATE_CAP| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_POWER_INPUT);
            ft->average = sft_value(ft, SENSORS_SUBFEATURE_POWER_AVERAGE);

#ifdef NETDATA_CALCULATED_STATES
            if(ft->exposed_input && isnan(ft->input)) {
                ft->state = FEATURE_STATE_FAULT;
                return;
            }

            if(ft->exposed_average && isnan(ft->average)) {
                ft->state = FEATURE_STATE_FAULT;
                return;
            }
#endif

            // the following are kernel driver reported states
            if (
                sft_value(ft, SENSORS_SUBFEATURE_POWER_LCRIT_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_POWER_CRIT_ALARM) > 0)
                ft->state = FEATURE_STATE_CRITICAL;
            else if (
                sft_value(ft, SENSORS_SUBFEATURE_POWER_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_POWER_MIN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_POWER_MAX_ALARM) > 0)
                ft->state = FEATURE_STATE_ALARM;
            else if (
                sft_value(ft, SENSORS_SUBFEATURE_POWER_CAP_ALARM) > 0)
                ft->state = FEATURE_STATE_CAP;

#ifdef NETDATA_CALCULATED_STATES
            // the following are Netdata calculated alarm states
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_POWER_LCRIT_ALARM)) &&
                     ft->input < sft_value(ft, SENSORS_SUBFEATURE_POWER_LCRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_POWER_CRIT_ALARM)) &&
                     ft->input >= sft_value(ft, SENSORS_SUBFEATURE_POWER_CRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_POWER_MIN_ALARM)) &&
                     ft->input < sft_value(ft, SENSORS_SUBFEATURE_POWER_MIN))
                ft->state = FEATURE_STATE_ALARM;
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_POWER_MAX_ALARM)) &&
                     ft->input >= sft_value(ft, SENSORS_SUBFEATURE_POWER_MAX))
                ft->state = FEATURE_STATE_ALARM;
#endif
            break;
        }

        case SENSORS_FEATURE_ENERGY:
            ft->supported_states = (FEATURE_STATE_CLEAR| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_ENERGY_INPUT);
            ft->average = NAN;

#ifdef NETDATA_CALCULATED_STATES
            if(isnan(ft->input))
                ft->state = FEATURE_STATE_FAULT;
#endif
            break;

        case SENSORS_FEATURE_CURR: {
            ft->supported_states = (FEATURE_STATE_CLEAR|FEATURE_STATE_CRITICAL|FEATURE_STATE_ALARM| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_CURR_INPUT);
            ft->average = sft_value(ft, SENSORS_SUBFEATURE_CURR_AVERAGE);

#ifdef NETDATA_CALCULATED_STATES
            if(ft->exposed_input && isnan(ft->input)) {
                ft->state = FEATURE_STATE_FAULT;
                return;
            }

            if(ft->exposed_average && isnan(ft->average)) {
                ft->state = FEATURE_STATE_FAULT;
                return;
            }
#endif

            // the following are kernel driver reported states
            if (
                sft_value(ft, SENSORS_SUBFEATURE_CURR_LCRIT_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_CURR_CRIT_ALARM) > 0)
                ft->state = FEATURE_STATE_CRITICAL;
            else if (
                sft_value(ft, SENSORS_SUBFEATURE_CURR_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_CURR_MIN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_CURR_MAX_ALARM) > 0)
                ft->state = FEATURE_STATE_ALARM;

#ifdef NETDATA_CALCULATED_STATES
            // the following are Netdata calculated alarm states
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_CURR_LCRIT_ALARM)) &&
                     ft->input < sft_value(ft, SENSORS_SUBFEATURE_CURR_LCRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_CURR_CRIT_ALARM)) &&
                     ft->input >= sft_value(ft, SENSORS_SUBFEATURE_CURR_CRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_CURR_MIN_ALARM)) &&
                     ft->input < sft_value(ft, SENSORS_SUBFEATURE_CURR_MIN))
                ft->state = FEATURE_STATE_ALARM;
            else if (!isnan(ft->input) &&
                     isnan(sft_value(ft, SENSORS_SUBFEATURE_CURR_MAX_ALARM)) &&
                     ft->input >= sft_value(ft, SENSORS_SUBFEATURE_CURR_MAX))
                ft->state = FEATURE_STATE_ALARM;
#endif
            break;
        }

        case SENSORS_FEATURE_HUMIDITY:
            ft->supported_states = (FEATURE_STATE_CLEAR| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_HUMIDITY_INPUT);
            ft->average = NAN;

#ifdef NETDATA_CALCULATED_STATES
            if(isnan(ft->input))
                ft->state = FEATURE_STATE_FAULT;
#endif
            break;

        case SENSORS_FEATURE_INTRUSION:
            ft->supported_states = (FEATURE_STATE_CLEAR|FEATURE_STATE_ALARM| custom_states);

            ft->input = sft_value(ft, SENSORS_SUBFEATURE_INTRUSION_ALARM);
            ft->average = NAN;

#ifdef NETDATA_CALCULATED_STATES
            if(isnan(ft->input)) {
                ft->state = FEATURE_STATE_FAULT;
                return;
            }
#endif
            if(ft->input > 0)
                ft->state = FEATURE_STATE_ALARM;
            break;

        default:
            break;
    }
}

static SENSOR *sensor_get_or_create(DICTIONARY *dict, const sensors_chip_name *chip, const sensors_feature *feature) {
    static __thread char buf[1024];

    snprintfz(buf, sizeof(buf),
              "%s|%s-%d-%d-%s",
              chip->path, chip->prefix, chip->bus.type, chip->addr, feature->name);

    SENSOR *ft = dictionary_get(dict, buf);
    if(ft) return ft;

    bool report_state = true;
    bool report_value = true;
    const char *title;
    const char *units;
    const char *context;
    const char *family;
    int priority;

    switch(feature->type) {
        case SENSORS_FEATURE_IN:
            title = "Sensor Voltage";
            units = "Volts";
            context = "sensors.hw.voltage";
            family = "Voltages";
            priority = 70002;
            break;
        case SENSORS_FEATURE_FAN:
            title = "Sensor Fan Speed";
            units = "rotations per minute";
            context = "sensors.hw.fan";
            family = "Fans";
            priority = 70005;
            break;
        case SENSORS_FEATURE_TEMP:
            title = "Sensor Temperature";
            units = "degrees Celsius";
            context = "sensors.hw.temperature";
            family = "Temperatures";
            priority = 70000;
            break;
        case SENSORS_FEATURE_POWER:
            title = "Sensor Power";
            units = "Watts";
            context = "sensors.hw.power";
            family = "Power";
            priority = 70006;
            break;
        case SENSORS_FEATURE_ENERGY:
            title = "Sensor Energy";
            units = "Joules";
            context = "sensors.hw.energy";
            family = "Energy";
            priority = 70007;
            break;
        case SENSORS_FEATURE_CURR:
            title = "Sensor Current";
            units = "Amperes";
            context = "sensors.hw.current";
            family = "Currents";
            priority = 70003;
            break;
        case SENSORS_FEATURE_HUMIDITY:
            title = "Sensor Humidity";
            units = "percentage";
            context = "sensors.hw.humidity";
            family = "Humidity";
            priority = 70004;
            break;
        case SENSORS_FEATURE_INTRUSION:
            report_value = false;
            title = "Sensor Intrusion";
            units = ""; // No specific unit, as this is a binary state
            context = "sensors.hw.intrusion";
            family = "Intrusion";
            priority = 70008;
            break;

        default:
        case SENSORS_FEATURE_UNKNOWN:
            return NULL;
    }

    ft = dictionary_set(dict, buf, NULL, sizeof(SENSOR));
    ft->report_state = report_state;
    ft->report_value = report_value;
    ft->title = string_strdupz(title);
    ft->units = string_strdupz(units);
    ft->family = string_strdupz(family);
    ft->context = string_strdupz(context);
    ft->priority = priority;

    sensors_snprintf_chip_name(buf, sizeof(buf), chip);
    ft->chip.id = string_strdupz(buf);
    ft->chip.name = string_strdupz(chip->prefix);
    ft->chip.adapter = string_strdupz(sensors_get_adapter_name(&chip->bus));
    ft->chip.path = string_strdupz(chip->path);
    ft->chip.bus = chip->bus.type;
    ft->chip.addr = chip->addr;

    ft->feature.name = string_strdupz(feature->name);
    const char *label = sensors_get_label(chip, feature);
    ft->feature.label = string_strdupz(label ? label : feature->name);
    ft->feature.type = feature->type;

    char *label_sanitized = strdupz(string2str(ft->feature.label));
    netdata_fix_chart_id(label_sanitized);
    ft->feature.label_sanitized = string_strdupz(label_sanitized);
    freez(label_sanitized);

    return ft;
}

static void sensor_labels(SENSOR *ft) {
    printf(PLUGINSD_KEYWORD_CLABEL " label '%s' 1\n", string2str(ft->feature.label));
    printf(PLUGINSD_KEYWORD_CLABEL " adapter '%s' 1\n", string2str(ft->chip.adapter));
    printf(PLUGINSD_KEYWORD_CLABEL " bus '%s' 1\n", SENSOR_BUS_TYPE_2str(ft->chip.bus));
    printf(PLUGINSD_KEYWORD_CLABEL " chip '%s' 1\n", string2str(ft->chip.name));
    printf(PLUGINSD_KEYWORD_CLABEL " chip_id '%s' 1\n", string2str(ft->chip.id));
    printf(PLUGINSD_KEYWORD_CLABEL " path '%s' 1\n", string2str(ft->chip.path));

    printf(
        PLUGINSD_KEYWORD_CLABEL " sensor '%s - %s' 1\n",
        string2str(ft->chip.name),
        string2str(ft->feature.label));

    printf(PLUGINSD_KEYWORD_CLABEL_COMMIT "\n");
}

static size_t states_count(FEATURE_STATE state) {
    // the gcc way
    return __builtin_popcount(state);

//    size_t count = 0;
//    while (state) {
//        state &= (state - 1);  // Clear the least significant set bit
//        count++;
//    }
//    return count;
}

static void sensor_process(SENSOR *ft, int update_every, const char *name) {
    // evaluate the state of the feature
    set_sensor_state(ft);
    internal_fatal(ft->state == 0,
                   "LIBSENSORS: state %u is not a valid state",
                   ft->state);

    internal_fatal((ft->state & ft->supported_states) == 0,
                   "LIBSENSORS: state %u is not in the supported list of states %u",
                   ft->state, ft->supported_states);

    bool do_input = ft->report_value && !isnan(ft->input);
    bool do_average = ft->report_value && !isnan(ft->average);
    bool do_state = ft->report_state && states_count(ft->supported_states) > 1;

    // send the feature data to netdata
    if(do_input && !ft->exposed_input) {
        printf(
            PLUGINSD_KEYWORD_CHART "'sensors.%s_%s_%s_%s' '' '%s' '%s' '%s' '%s' line %d %d '' debugfs %s\n",
            SENSOR_FEATURE_TYPE_2str(ft->feature.type),
            string2str(ft->chip.id),
            string2str(ft->feature.name),
            string2str(ft->feature.label_sanitized),
            string2str(ft->title),
            string2str(ft->units),
            string2str(ft->family),
            string2str(ft->context),
            ft->priority,
            update_every,
            name);

        printf(PLUGINSD_KEYWORD_DIMENSION " 'input' 1 10000 \n");
        sensor_labels(ft);
        ft->exposed_input = true;
    }

    if(do_average && !ft->exposed_average) {
        printf(
            PLUGINSD_KEYWORD_CHART "'sensors.%s_%s_%s_%s_average' '' '%s Average' '%s' '%s' '%s_average' line %d %d '' debugfs %s\n",
            SENSOR_FEATURE_TYPE_2str(ft->feature.type),
            string2str(ft->chip.id),
            string2str(ft->feature.name),
            string2str(ft->feature.label_sanitized),
            string2str(ft->title),
            string2str(ft->units),
            string2str(ft->family),
            string2str(ft->context),
            ft->priority,
            update_every,
            name);

        printf(PLUGINSD_KEYWORD_DIMENSION " 'average' 1 10000 \n");
        sensor_labels(ft);
        ft->exposed_average = true;
    }

    if(do_state && !ft->exposed_state) {
        printf(
            PLUGINSD_KEYWORD_CHART "'sensors.%s_%s_%s_%s_state' '' '%s State' '%s' '%s' '%s_states' line %d %d '' debugfs %s\n",
            SENSOR_FEATURE_TYPE_2str(ft->feature.type),
            string2str(ft->chip.id),
            string2str(ft->feature.name),
            string2str(ft->feature.label_sanitized),
            string2str(ft->title),
            string2str(ft->units),
            string2str(ft->family),
            string2str(ft->context),
            ft->priority,
            update_every,
            name);

        if(ft->supported_states & FEATURE_STATE_CLEAR)
            printf(PLUGINSD_KEYWORD_DIMENSION " clear 1 1 \n");
        if(ft->supported_states & FEATURE_STATE_CAP)
            printf(PLUGINSD_KEYWORD_DIMENSION " cap 1 1 \n");
        if(ft->supported_states & FEATURE_STATE_ALARM)
            printf(PLUGINSD_KEYWORD_DIMENSION " alarm 1 1 \n");
        if(ft->supported_states & FEATURE_STATE_CRITICAL)
            printf(PLUGINSD_KEYWORD_DIMENSION " critical 1 1 \n");
        if(ft->supported_states & FEATURE_STATE_EMERGENCY)
            printf(PLUGINSD_KEYWORD_DIMENSION " emergency 1 1 \n");
        if(ft->supported_states & FEATURE_STATE_FAULT)
            printf(PLUGINSD_KEYWORD_DIMENSION " fault 1 1 \n");

        sensor_labels(ft);
        ft->exposed_state = true;
    }

    // ----------------------------------------------------------------------------------------------------------------
    // debugging

    fprintf(stderr,
            "LIBSENSORS: "
            "{ chip id '%s', name '%s', addr %d }, "
            "{ adapter '%s', bus '%s', path '%s'}, "
            "{ feature label '%s', name '%s', type '%s' }\n",
            string2str(ft->chip.id),
            string2str(ft->chip.name), ft->chip.addr,
            string2str(ft->chip.adapter),
            SENSOR_BUS_TYPE_2str(ft->chip.bus),
            string2str(ft->chip.path),
            string2str(ft->feature.label),
            string2str(ft->feature.name),
            SENSOR_FEATURE_TYPE_2str(ft->feature.type));

    Word_t idx = 0;
    for(SUBFEATURE *sft = SUBFEATURES_FIRST(&ft->values, &idx);
         sft;
         sft = SUBFEATURES_NEXT(&ft->values, &idx)) {
        fprintf(stderr,
                " ------------ >>> "
                "{ subfeature '%s', type '%s' } "
                "value %0.4f, %s\n",
                string2str(sft->name), SENSOR_SUBFEATURE_TYPE_2str(idx),
                sft->value, sft->read ? "OK" : "FAILED");
    }

    if(do_input)
        fprintf(stderr, " ------------ >>> %0.4f (input)\n", ft->input);

    if(do_average)
        fprintf(stderr, " ------------ >>> %0.4f (average)\n", ft->average);

    if(do_state)
        fprintf(stderr, " ------------ >>> %u (state)\n", ft->state);

    // ----------------------------------------------------------------------------------------------------------------
    // send the data

    if(do_input) {
        printf(
            PLUGINSD_KEYWORD_BEGIN " 'sensors.%s_%s_%s_%s'\n",
            SENSOR_FEATURE_TYPE_2str(ft->feature.type),
            string2str(ft->chip.id),
            string2str(ft->feature.name),
            string2str(ft->feature.label_sanitized));

        printf(PLUGINSD_KEYWORD_SET " input = %lld\n", (long long)(ft->input * 10000.0));
        printf(PLUGINSD_KEYWORD_END "\n");
    }

    if(do_average) {
        printf(
            PLUGINSD_KEYWORD_BEGIN " 'sensors.%s_%s_%s_%s_average'\n",
            SENSOR_FEATURE_TYPE_2str(ft->feature.type),
            string2str(ft->chip.id),
            string2str(ft->feature.name),
            string2str(ft->feature.label_sanitized));

        printf(PLUGINSD_KEYWORD_SET " average = %lld\n", (long long)(ft->average * 10000.0));
        printf(PLUGINSD_KEYWORD_END "\n");
    }

    if(do_state) {
        printf(
            PLUGINSD_KEYWORD_BEGIN " 'sensors.%s_%s_%s_%s_state'\n",
            SENSOR_FEATURE_TYPE_2str(ft->feature.type),
            string2str(ft->chip.id),
            string2str(ft->feature.name),
            string2str(ft->feature.label_sanitized));

        if(ft->supported_states & FEATURE_STATE_CLEAR)
            printf(PLUGINSD_KEYWORD_DIMENSION " clear = %d\n", ft->state == FEATURE_STATE_CLEAR ? 1 : 0);
        if(ft->supported_states & FEATURE_STATE_CAP)
            printf(PLUGINSD_KEYWORD_DIMENSION " cap = %d\n", ft->state == FEATURE_STATE_CAP ? 1 : 0);
        if(ft->supported_states & FEATURE_STATE_ALARM)
            printf(PLUGINSD_KEYWORD_DIMENSION " alarm = %d\n", ft->state == FEATURE_STATE_ALARM ? 1 : 0);
        if(ft->supported_states & FEATURE_STATE_CRITICAL)
            printf(PLUGINSD_KEYWORD_DIMENSION " critical = %d\n", ft->state == FEATURE_STATE_CRITICAL ? 1 : 0);
        if(ft->supported_states & FEATURE_STATE_EMERGENCY)
            printf(PLUGINSD_KEYWORD_DIMENSION " emergency = %d\n", ft->state == FEATURE_STATE_EMERGENCY ? 1 : 0);
        if(ft->supported_states & FEATURE_STATE_FAULT)
            printf(PLUGINSD_KEYWORD_DIMENSION " fault = %d\n", ft->state == FEATURE_STATE_FAULT ? 1 : 0);

        printf(PLUGINSD_KEYWORD_END "\n");
    }
}

int do_module_libsensors(int update_every, const char *name) {
    static bool libsensors_initialized = false;
    static DICTIONARY *features = NULL;

    // ----------------------------------------------------------------------------------------------------------------
    // initialize it, if it is not initialized already

    if(!libsensors_initialized) {
        if(libsensors_initialized)
            sensors_cleanup();

        FILE *fp = NULL;
        const char *etc_netdata_dir = getenv("NETDATA_CONFIG_DIR");
        if(etc_netdata_dir) {
            // In static installs, we copy the file sensors3.conf to /opt/netdata/etc
            char filename[FILENAME_MAX];
            snprintfz(filename, sizeof(filename), "%s/../sensors3.conf", etc_netdata_dir);
            fp = fopen(filename, "r");
        }

        if (sensors_init(fp) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot initialize libsensors - disabling sensors monitoring");
            return 1;
        }

        if(fp)
            fclose(fp);

        features = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_SINGLE_THREADED);
        libsensors_initialized = true;
    }

    // ----------------------------------------------------------------------------------------------------------------
    // reset all sensors to unread

    SENSOR *ft;
    dfe_start_read(features, ft) {
        ft->read = false;
    }
    dfe_done(ft);

    // ----------------------------------------------------------------------------------------------------------------
    // Iterate over all detected chips

    const sensors_chip_name *chip;
    int chip_nr = 0;
    while ((chip = sensors_get_detected_chips(NULL, &chip_nr)) != NULL) {

        // Iterate over all features of the chip
        const sensors_feature *feature;
        int feature_nr = 0;
        while ((feature = sensors_get_features(chip, &feature_nr)) != NULL) {
            ft = sensor_get_or_create(features, chip, feature);
            if(!ft) continue;
            internal_fatal(ft->read, "The features key is not unique!");
            ft->read = true;

            // --------------------------------------------------------------------------------------------------------
            // mark all existing subfeatures as unread

            Word_t idx = 0;
            for(SUBFEATURE *sft = SUBFEATURES_FIRST(&ft->values, &idx);
                 sft;
                 sft = SUBFEATURES_NEXT(&ft->values, &idx)) {
                sft->read = false;
                sft->value = NAN;
            }

            // --------------------------------------------------------------------------------------------------------
            // iterate over all subfeatures of the feature

            const sensors_subfeature *subfeature;
            int subfeature_nr = 0;
            while ((subfeature = sensors_get_all_subfeatures(chip, feature, &subfeature_nr)) != NULL) {
                if(!(subfeature->flags & SENSORS_MODE_R))
                    continue;

                SUBFEATURE *sft = SUBFEATURES_GET(&ft->values, subfeature->type);
                if(!sft) {
                    sft = callocz(1, sizeof(*sft));
                    sft->name = string_strdupz(subfeature->name);
                    SUBFEATURES_SET(&ft->values, subfeature->type, sft);
                }

                if (sensors_get_value(chip, subfeature->number, &sft->value) == 0)
                    sft->read = true;
                else {
                    sft->value = NAN;
                    sft->read = false;
                }
            }

            sensor_process(ft, update_every, name);
        }
    }

    return 0;
}

#else
int do_module_libsensors(int update_every __maybe_unused, const char *name __maybe_unused) { return 1; }
#endif
