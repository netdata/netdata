// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

#if defined(HAVE_LIBSENSORS)

#include <sensors/sensors.h>
#include <sensors/error.h>

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

    const sensors_chip_name *chip;
    int chip_nr = 0;

    // Iterate over all detected chips
    while ((chip = sensors_get_detected_chips(NULL, &chip_nr)) != NULL) {
        printf("Chip: %s\n", sensors_get_adapter_name(&chip->bus));

        const sensors_feature *feature;
        int feature_nr = 0;

        // Iterate over all features of the chip
        while ((feature = sensors_get_features(chip, &feature_nr)) != NULL) {
            printf("  Feature: %s\n", sensors_get_label(chip, feature));

            const sensors_subfeature *subfeature;
            int subfeature_nr = 0;

            // Iterate over all subfeatures of the feature
            while ((subfeature = sensors_get_all_subfeatures(chip, feature, &subfeature_nr)) != NULL) {
                if (subfeature->flags & SENSORS_MODE_R) { // If subfeature is readable
                    double value;
                    if (sensors_get_value(chip, subfeature->number, &value) == 0) {
                        printf("    Subfeature: %s, Value: %.2f\n", sensors_get_label(chip, feature), value);
                    } else {
                        fprintf(stderr, "    Failed to read value for %s\n", sensors_get_label(chip, feature));
                    }
                }

                if (subfeature->type == SENSORS_SUBFEATURE_IN_ALARM) { // Check for alarms
                    double alarm_value;
                    if (sensors_get_value(chip, subfeature->number, &alarm_value) == 0) {
                        printf("    Alarm: %s, Status: %s\n",
                               sensors_get_label(chip, feature),
                               alarm_value == 1.0 ? "ACTIVE" : "INACTIVE");
                    } else {
                        fprintf(stderr, "    Failed to read alarm status for %s\n", sensors_get_label(chip, feature));
                    }
                }
            }
        }
    }

    return 1;
}

#else
int do_module_libsensors(int update_every __maybe_unused, const char *name __maybe_unused) { return 1; }
#endif
