// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

#if defined(HAVE_LIBSENSORS)

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
    { .id = SENSORS_FEATURE_IN, .name = "voltage", },
    { .id = SENSORS_FEATURE_FAN, .name = "fan", },
    { .id = SENSORS_FEATURE_TEMP, .name = "temperature", },
    { .id = SENSORS_FEATURE_POWER, .name = "power", },
    { .id = SENSORS_FEATURE_ENERGY, .name = "energy", },
    { .id = SENSORS_FEATURE_CURR, .name = "current", },
    { .id = SENSORS_FEATURE_HUMIDITY, .name = "humidity", },
    { .id = SENSORS_FEATURE_VID, .name = "voltage_identification", },
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
    FEATURE_STATE_CLEAR = 0,
    FEATURE_STATE_ALARM,
    FEATURE_STATE_CRITICAL,
    FEATURE_STATE_EMERGENCY,
    FEATURE_STATE_FAULT,
} FEATURE_STATE;

typedef struct feature {
    bool sent_to_netdata;
    SENSOR_FEATURE_TYPE type;
    FEATURE_STATE state;
    SUBFEATURES_JudyLSet values;
} FEATURE;

static inline double sft_value(FEATURE *ft, SENSOR_SUBFEATURE_TYPE type) {
    double value = NAN;

    SUBFEATURE *sft = SUBFEATURES_GET(&ft->values, type);
    if(sft && sft->read && !isinf(sft->value))
        value = sft->value;

    return value;
}

static void set_feature_state(FEATURE *ft) {
    ft->state = FEATURE_STATE_CLEAR;

    double value;
    switch(ft->type) {
        case SENSORS_FEATURE_IN: {
            bool is_average = false;
            value = sft_value(ft, SENSORS_SUBFEATURE_IN_INPUT);
            if (isnan(value)) {
                is_average = true;
                value = sft_value(ft, SENSORS_SUBFEATURE_IN_AVERAGE);
                if (isnan(value)) {
                    ft->state = FEATURE_STATE_FAULT;
                    return;
                }
            }

            if (
                sft_value(ft, SENSORS_SUBFEATURE_IN_LCRIT_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_IN_CRIT_ALARM) > 0)
                ft->state = FEATURE_STATE_CRITICAL;
            else if (
                sft_value(ft, SENSORS_SUBFEATURE_IN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_IN_MIN_ALARM) > 0 ||
                sft_value(ft, SENSORS_SUBFEATURE_IN_MAX_ALARM) > 0)
                ft->state = FEATURE_STATE_ALARM;
            else if (!is_average &&
                isnan(sft_value(ft, SENSORS_SUBFEATURE_IN_LCRIT_ALARM)) &&
                value < sft_value(ft, SENSORS_SUBFEATURE_IN_LCRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if (!is_average &&
                isnan(sft_value(ft, SENSORS_SUBFEATURE_IN_CRIT_ALARM)) &&
                value >= sft_value(ft, SENSORS_SUBFEATURE_IN_CRIT))
                ft->state = FEATURE_STATE_CRITICAL;
            else if (!is_average &&
                isnan(sft_value(ft, SENSORS_SUBFEATURE_IN_MIN_ALARM)) &&
                value < sft_value(ft, SENSORS_SUBFEATURE_IN_MIN))
                ft->state = FEATURE_STATE_ALARM;
            else if (!is_average &&
                isnan(sft_value(ft, SENSORS_SUBFEATURE_IN_MAX_ALARM)) &&
                value >= sft_value(ft, SENSORS_SUBFEATURE_IN_MAX))
                ft->state = FEATURE_STATE_ALARM;
            break;
        }

        case SENSORS_FEATURE_FAN:
            value = sft_value(ft, SENSORS_SUBFEATURE_FAN_INPUT);
            if(isnan(value))
                ft->state = FEATURE_STATE_FAULT;
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_FAN_ALARM) > 0 ||
                     sft_value(ft, SENSORS_SUBFEATURE_FAN_MIN_ALARM) > 0 ||
                     sft_value(ft, SENSORS_SUBFEATURE_FAN_MAX_ALARM) > 0)
                ft->state = FEATURE_STATE_ALARM;
            break;

        case SENSORS_FEATURE_TEMP:
            value = sft_value(ft, SENSORS_SUBFEATURE_TEMP_INPUT);
            if(isnan(value))
                ft->state = FEATURE_STATE_FAULT;
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_TEMP_CRIT_ALARM) > 0 ||
                     value < sft_value(ft, SENSORS_SUBFEATURE_IN_LCRIT_ALARM) ||
                     value >= sft_value(ft, SENSORS_SUBFEATURE_IN_CRIT_ALARM))
                ft->state = FEATURE_STATE_CRITICAL;
            else if(
                sft_value(ft, SENSORS_SUBFEATURE_IN_ALARM) > 0 ||
                     value < sft_value(ft, SENSORS_SUBFEATURE_TEMP_MIN_ALARM) ||
                     value >= sft_value(ft, SENSORS_SUBFEATURE_TEMP_MAX_ALARM))
                ft->state = FEATURE_STATE_ALARM;
            break;

        case SENSORS_FEATURE_POWER:
            break;

        case SENSORS_FEATURE_ENERGY:
            break;

        case SENSORS_FEATURE_CURR:
            break;

        case SENSORS_FEATURE_HUMIDITY:
            break;

        case SENSORS_FEATURE_MAX_MAIN:
            break;

        case SENSORS_FEATURE_VID:
            break;

        case SENSORS_FEATURE_INTRUSION:
            break;

        case SENSORS_FEATURE_MAX_OTHER:
            break;

        case SENSORS_FEATURE_BEEP_ENABLE:
            break;

        case SENSORS_FEATURE_MAX:
            break;

        case SENSORS_FEATURE_UNKNOWN:
            break;

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
            return 0;
        }

        if(fp)
            fclose(fp);

        features = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_SINGLE_THREADED);
        libsensors_initialized = true;
    }

    // ----------------------------------------------------------------------------------------------------------------
    // read all sensor values

    CLEAN_BUFFER *key = buffer_create(0, NULL);

    // Iterate over all detected chips
    const sensors_chip_name *chip;
    int chip_nr = 0;
    while ((chip = sensors_get_detected_chips(NULL, &chip_nr)) != NULL) {
        const char *adapter_name = sensors_get_adapter_name(&chip->bus);
        const char *bus_type = SENSOR_BUS_TYPE_2str(chip->bus.type);

        // Iterate over all features of the chip
        const sensors_feature *feature;
        int feature_nr = 0;
        while ((feature = sensors_get_features(chip, &feature_nr)) != NULL) {
            const char *feature_name = sensors_get_label(chip, feature);
            const char *feature_type = SENSOR_FEATURE_TYPE_2str(feature->type);

            // create the dictionary key
            buffer_flush(key);
            buffer_sprintf(key, "%s|%s|%s|%s|%s|%s",
                           adapter_name, bus_type, chip->prefix, chip->path, feature_name, feature_type);

            FEATURE *ft = dictionary_get(features, buffer_tostring(key));
            if(!ft) {
                ft = dictionary_set(features, buffer_tostring(key), NULL, sizeof(FEATURE));
                ft->type = feature->type;
            }
            ft->state = FEATURE_STATE_CLEAR;

            // mark all existing subfeatures as unread
            Word_t idx = 0;
            for(SUBFEATURE *sft = SUBFEATURES_FIRST(&ft->values, &idx);
                 sft;
                 sft = SUBFEATURES_NEXT(&ft->values, &idx)) {
                sft->read = false;
                sft->value = NAN;
            }

            // iterate over all subfeatures of the feature
            const sensors_subfeature *subfeature;
            int subfeature_nr = 0;
            while ((subfeature = sensors_get_all_subfeatures(chip, feature, &subfeature_nr)) != NULL) {
                if(!(subfeature->flags & SENSORS_MODE_R))
                    continue;

                const char *subfeature_name = subfeature->name;
                const char *subfeature_type = SENSOR_SUBFEATURE_TYPE_2str(subfeature->type);

                SUBFEATURE *sft = SUBFEATURES_GET(&ft->values, subfeature->type);
                if(!sft) {
                    sft = callocz(1, sizeof(*sft));
                    sft->name = string_strdupz(subfeature_name);
                    SUBFEATURES_SET(&ft->values, subfeature->type, sft);
                }

                if (sensors_get_value(chip, subfeature->number, &sft->value) == 0)
                    sft->read = true;
                else {
                    sft->value = NAN;
                    sft->read = false;
                }

                fprintf(stderr,
                        "LIBSENSORS: { chip '%s', bus '%s', prefix '%s', path '%s'}, { feature '%s' (%s), type '%s' }, { subfeature '%s', type '%s' }, value %0.4f, %s\n",
                        adapter_name, bus_type, chip->prefix, chip->path,
                        feature_name, feature->name, feature_type,
                        subfeature_name, subfeature_type,
                        sft->value, sft->read ? "OK" : "FAILED");
            }

            set_feature_state(ft);

            // --------------------------------------------------------------------------------------------------------
            // send the feature data to netdata


        }
    }

    return 1;
}

#else
int do_module_libsensors(int update_every __maybe_unused, const char *name __maybe_unused) { return 1; }
#endif
