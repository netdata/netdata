// SPDX-License-Identifier: GPL-3.0-or-later

#include "parsers.h"

#define MINUTE_IN_SECONDS (60)
#define HOUR_IN_SECONDS (MINUTE_IN_SECONDS * 60)
#define DAY_IN_SECONDS (HOUR_IN_SECONDS * 24)
#define WEEK_IN_SECONDS (DAY_IN_SECONDS * 7)
#define YEAR_IN_SECONDS (DAY_IN_SECONDS * 365)
#define MONTH_IN_SECONDS (YEAR_IN_SECONDS / 12)
#define QUARTER_IN_SECONDS (YEAR_IN_SECONDS / 4)

// Define a structure to map time units to their multipliers
struct {
    const char *unit;
    nsec_t multiplier;
} units[] = {
    { .unit = "ns",             .multiplier = 1 },                                                      // UCUM
    { .unit = "us",             .multiplier = 1 * NSEC_PER_USEC },                                      // UCUM
    { .unit = "ms",             .multiplier = 1 * USEC_PER_MS * NSEC_PER_USEC },                        // UCUM
    { .unit = "s",              .multiplier = 1 * USEC_PER_SEC * NSEC_PER_USEC },                       // UCUM
    { .unit = "m",              .multiplier = MINUTE_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },       // -
    { .unit = "min",            .multiplier = MINUTE_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },       // UCUM
    { .unit = "h",              .multiplier = HOUR_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },         // UCUM
    { .unit = "d",              .multiplier = DAY_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },          // UCUM
    { .unit = "w",              .multiplier = WEEK_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },         // -
    { .unit = "wk",              .multiplier = WEEK_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },        // UCUM
    { .unit = "mo",             .multiplier = MONTH_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },        // UCUM
    { .unit = "q",              .multiplier = QUARTER_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },      // -
    { .unit = "y",              .multiplier = YEAR_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },         // -
    { .unit = "a",              .multiplier = YEAR_IN_SECONDS * USEC_PER_SEC * NSEC_PER_USEC },         // UCUM
};

// -------------------------------------------------------------------------------------------------------------------
// parse a duration string

nsec_t duration_to_nsec_t(const char *duration, nsec_t default_value, const char *default_unit) {
    if(!duration || !*duration)
        return default_value;

    if(!default_unit)
        default_unit = "ns";

    const char *end = NULL;
    double value = strtod(duration, (char **)&end);

    if (!end || end == duration)
        return default_value;

    while(isspace(*end))
        end++;

    if (!*end)
        end = default_unit;

    for (size_t i = 0; i < sizeof(units) / sizeof(units[0]); i++) {
        if (strcmp(end, units[i].unit) == 0)
            return (nsec_t)(value * (double)units[i].multiplier);
    }

    return (nsec_t)value;
}

usec_t duration_to_usec_t(const char *duration, usec_t default_value, const char *default_unit) {
    if(!default_unit)
        default_unit = "us";

    return (time_t)(duration_to_nsec_t(duration, default_value, default_unit) / NSEC_PER_USEC);
}

time_t duration_to_time_t(const char *duration, time_t default_value, const char *default_unit) {
    if(!default_unit)
        default_unit = "s";

    return (time_t)(duration_to_nsec_t(duration, default_value, default_unit) / NSEC_PER_SEC);
}

// --------------------------------------------------------------------------------------------------------------------
// generate a string to represent a duration

size_t duration_from_nsec_t(char *dst, size_t size, nsec_t value, const char *default_unit) {
    if (!dst || !size) return 0;
    if (size == 1) {
        *dst = '\0';
        return 0;
    }

    if(!default_unit)
        default_unit = "ns";

    for (int i = sizeof(units) / sizeof(units[0]) - 1; i >= 0; i--) {
        if ((value % units[i].multiplier) == 0) {
            uint64_t num_units = value / units[i].multiplier;
            return snprintf(dst, size, "%"PRIu64"%s", num_units, units[i].unit);
        }
    }

    for (size_t i = 0; i < sizeof(units) / sizeof(units[0]); i++) {
        if (strcmp(default_unit, units[i].unit) == 0) {
            double num_units = (double)value / (double)units[i].multiplier;
            return snprintf(dst, size, "%.2f%s", num_units, units[i].unit);
        }
    }

    // If no suitable unit is found, default to nanoseconds
    return snprintf(dst, size, "%"PRIu64"ns", (uint64_t)value);
}

size_t duration_from_usec_t(char *dst, size_t size, usec_t value, const char *default_unit) {
    if(!default_unit) default_unit = "us";
    return duration_from_nsec_t(dst, size, value * NSEC_PER_USEC, default_unit);
}

size_t duration_from_time_t(char *dst, size_t size, time_t value, const char *default_unit) {
    if(!default_unit) default_unit = "s";
    return duration_from_nsec_t(dst, size, value * NSEC_PER_SEC, default_unit);
}
