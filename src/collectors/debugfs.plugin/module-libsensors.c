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
    { .id = SENSORS_SUBFEATURE_IN_INPUT, .name = "in_input", },
    { .id = SENSORS_SUBFEATURE_IN_MIN, .name = "in_min", },
    { .id = SENSORS_SUBFEATURE_IN_MAX, .name = "in_max", },
    { .id = SENSORS_SUBFEATURE_IN_LCRIT, .name = "in_lcrit", },
    { .id = SENSORS_SUBFEATURE_IN_CRIT, .name = "in_crit", },
    { .id = SENSORS_SUBFEATURE_IN_AVERAGE, .name = "in_average", },
    { .id = SENSORS_SUBFEATURE_IN_LOWEST, .name = "in_lowest", },
    { .id = SENSORS_SUBFEATURE_IN_HIGHEST, .name = "in_highest", },
    { .id = SENSORS_SUBFEATURE_IN_ALARM, .name = "in_alarm", },
    { .id = SENSORS_SUBFEATURE_IN_MIN_ALARM, .name = "in_min_alarm", },
    { .id = SENSORS_SUBFEATURE_IN_MAX_ALARM, .name = "in_max_alarm", },
    { .id = SENSORS_SUBFEATURE_IN_BEEP, .name = "in_beep", },
    { .id = SENSORS_SUBFEATURE_IN_LCRIT_ALARM, .name = "in_lcrit_alarm", },
    { .id = SENSORS_SUBFEATURE_IN_CRIT_ALARM, .name = "in_crit_alarm", },

    // Fan subfeatures
    { .id = SENSORS_SUBFEATURE_FAN_INPUT, .name = "fan_input", },
    { .id = SENSORS_SUBFEATURE_FAN_MIN, .name = "fan_min", },
    { .id = SENSORS_SUBFEATURE_FAN_MAX, .name = "fan_max", },
    { .id = SENSORS_SUBFEATURE_FAN_ALARM, .name = "fan_alarm", },
    { .id = SENSORS_SUBFEATURE_FAN_FAULT, .name = "fan_fault", },
    { .id = SENSORS_SUBFEATURE_FAN_DIV, .name = "fan_div", },
    { .id = SENSORS_SUBFEATURE_FAN_BEEP, .name = "fan_beep", },
    { .id = SENSORS_SUBFEATURE_FAN_PULSES, .name = "fan_pulses", },
    { .id = SENSORS_SUBFEATURE_FAN_MIN_ALARM, .name = "fan_min_alarm", },
    { .id = SENSORS_SUBFEATURE_FAN_MAX_ALARM, .name = "fan_max_alarm", },

    // Temperature subfeatures
    { .id = SENSORS_SUBFEATURE_TEMP_INPUT, .name = "temp_input", },
    { .id = SENSORS_SUBFEATURE_TEMP_MAX, .name = "temp_max", },
    { .id = SENSORS_SUBFEATURE_TEMP_MAX_HYST, .name = "temp_max_hyst", },
    { .id = SENSORS_SUBFEATURE_TEMP_MIN, .name = "temp_min", },
    { .id = SENSORS_SUBFEATURE_TEMP_CRIT, .name = "temp_crit", },
    { .id = SENSORS_SUBFEATURE_TEMP_CRIT_HYST, .name = "temp_crit_hyst", },
    { .id = SENSORS_SUBFEATURE_TEMP_LCRIT, .name = "temp_lcrit", },
    { .id = SENSORS_SUBFEATURE_TEMP_EMERGENCY, .name = "temp_emergency", },
    { .id = SENSORS_SUBFEATURE_TEMP_EMERGENCY_HYST, .name = "temp_emergency_hyst", },
    { .id = SENSORS_SUBFEATURE_TEMP_LOWEST, .name = "temp_lowest", },
    { .id = SENSORS_SUBFEATURE_TEMP_HIGHEST, .name = "temp_highest", },
    { .id = SENSORS_SUBFEATURE_TEMP_MIN_HYST, .name = "temp_min_hyst", },
    { .id = SENSORS_SUBFEATURE_TEMP_LCRIT_HYST, .name = "temp_lcrit_hyst", },
    { .id = SENSORS_SUBFEATURE_TEMP_ALARM, .name = "temp_alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_MAX_ALARM, .name = "temp_max_alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_MIN_ALARM, .name = "temp_min_alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_CRIT_ALARM, .name = "temp_crit_alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_FAULT, .name = "temp_fault", },
    { .id = SENSORS_SUBFEATURE_TEMP_TYPE, .name = "temp_type", },
    { .id = SENSORS_SUBFEATURE_TEMP_OFFSET, .name = "temp_offset", },
    { .id = SENSORS_SUBFEATURE_TEMP_BEEP, .name = "temp_beep", },
    { .id = SENSORS_SUBFEATURE_TEMP_EMERGENCY_ALARM, .name = "temp_emergency_alarm", },
    { .id = SENSORS_SUBFEATURE_TEMP_LCRIT_ALARM, .name = "temp_lcrit_alarm", },

    // Power subfeatures
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE, .name = "power_average", },
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE_HIGHEST, .name = "power_average_highest", },
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE_LOWEST, .name = "power_average_lowest", },
    { .id = SENSORS_SUBFEATURE_POWER_INPUT, .name = "power_input", },
    { .id = SENSORS_SUBFEATURE_POWER_INPUT_HIGHEST, .name = "power_input_highest", },
    { .id = SENSORS_SUBFEATURE_POWER_INPUT_LOWEST, .name = "power_input_lowest", },
    { .id = SENSORS_SUBFEATURE_POWER_CAP, .name = "power_cap", },
    { .id = SENSORS_SUBFEATURE_POWER_CAP_HYST, .name = "power_cap_hyst", },
    { .id = SENSORS_SUBFEATURE_POWER_MAX, .name = "power_max", },
    { .id = SENSORS_SUBFEATURE_POWER_CRIT, .name = "power_crit", },
    { .id = SENSORS_SUBFEATURE_POWER_MIN, .name = "power_min", },
    { .id = SENSORS_SUBFEATURE_POWER_LCRIT, .name = "power_lcrit", },
    { .id = SENSORS_SUBFEATURE_POWER_AVERAGE_INTERVAL, .name = "power_average_interval", },
    { .id = SENSORS_SUBFEATURE_POWER_ALARM, .name = "power_alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_CAP_ALARM, .name = "power_cap_alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_MAX_ALARM, .name = "power_max_alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_CRIT_ALARM, .name = "power_crit_alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_MIN_ALARM, .name = "power_min_alarm", },
    { .id = SENSORS_SUBFEATURE_POWER_LCRIT_ALARM, .name = "power_lcrit_alarm", },

    // Energy subfeatures
    { .id = SENSORS_SUBFEATURE_ENERGY_INPUT, .name = "energy_input", },

    // Current subfeatures
    { .id = SENSORS_SUBFEATURE_CURR_INPUT, .name = "curr_input", },
    { .id = SENSORS_SUBFEATURE_CURR_MIN, .name = "curr_min", },
    { .id = SENSORS_SUBFEATURE_CURR_MAX, .name = "curr_max", },
    { .id = SENSORS_SUBFEATURE_CURR_LCRIT, .name = "curr_lcrit", },
    { .id = SENSORS_SUBFEATURE_CURR_CRIT, .name = "curr_crit", },
    { .id = SENSORS_SUBFEATURE_CURR_AVERAGE, .name = "curr_average", },
    { .id = SENSORS_SUBFEATURE_CURR_LOWEST, .name = "curr_lowest", },
    { .id = SENSORS_SUBFEATURE_CURR_HIGHEST, .name = "curr_highest", },
    { .id = SENSORS_SUBFEATURE_CURR_ALARM, .name = "curr_alarm", },
    { .id = SENSORS_SUBFEATURE_CURR_MIN_ALARM, .name = "curr_min_alarm", },
    { .id = SENSORS_SUBFEATURE_CURR_MAX_ALARM, .name = "curr_max_alarm", },
    { .id = SENSORS_SUBFEATURE_CURR_BEEP, .name = "curr_beep", },
    { .id = SENSORS_SUBFEATURE_CURR_LCRIT_ALARM, .name = "curr_lcrit_alarm", },
    { .id = SENSORS_SUBFEATURE_CURR_CRIT_ALARM, .name = "curr_crit_alarm", },

    // Humidity subfeatures
    { .id = SENSORS_SUBFEATURE_HUMIDITY_INPUT, .name = "humidity_input", },

    // VID subfeatures
    { .id = SENSORS_SUBFEATURE_VID, .name = "vid", },

    // Intrusion subfeatures
    { .id = SENSORS_SUBFEATURE_INTRUSION_ALARM, .name = "intrusion_alarm", },
    { .id = SENSORS_SUBFEATURE_INTRUSION_BEEP, .name = "intrusion_beep", },

    // Beep enable subfeatures
    { .id = SENSORS_SUBFEATURE_BEEP_ENABLE, .name = "beep_enable", },

    // Unknown subfeature
    { .id = SENSORS_SUBFEATURE_UNKNOWN, .name = "unknown", },

    // terminator
    {.id = 0, .name = NULL}
};

ENUM_STR_DEFINE_FUNCTIONS(SENSOR_SUBFEATURE_TYPE, SENSORS_SUBFEATURE_UNKNOWN, "unknown");

int do_module_libsensors(int update_every, const char *name) {
    static bool libsensors_initialized = false;
    static size_t iteration = 0;

    if(!libsensors_initialized || iteration % 600 == 0) {
        if(libsensors_initialized)
            sensors_cleanup();

        if (sensors_init(NULL) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot initialize libsensors - disabling sensors monitoring");
            return 0;
        }

        libsensors_initialized = true;
    }

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

            // Iterate over all subfeatures of the feature
            const sensors_subfeature *subfeature;
            int subfeature_nr = 0;
            while ((subfeature = sensors_get_all_subfeatures(chip, feature, &subfeature_nr)) != NULL) {
                const char *subfeature_name = subfeature->name;
                const char *subfeature_type = SENSOR_SUBFEATURE_TYPE_2str(subfeature->type);
                bool readable = subfeature->flags & SENSORS_MODE_R;

                const char *msg = "READ OK";

                double value = NAN;
                if (readable) { // If subfeature is readable
                    if (sensors_get_value(chip, subfeature->number, &value) != 0) {
                        value = NAN;
                        msg = "READ FAILED";
                    }
                }
                else
                    msg = "NOT READABLE";

                fprintf(stderr,
                        "LIBSENSORS: { chip '%s', bus '%s', prefix '%s', path '%s'}, { feature '%s' (%s), type '%s' }, { subfeature '%s', type '%s' }, value %0.4f, %s\n",
                        adapter_name, bus_type, chip->prefix, chip->path,
                        feature_name, feature->name, feature_type,
                        subfeature_name, subfeature_type,
                        value, msg);
            }
        }
    }

    return 1;
}

#else
int do_module_libsensors(int update_every __maybe_unused, const char *name __maybe_unused) { return 1; }
#endif
