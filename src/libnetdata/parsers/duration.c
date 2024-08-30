// SPDX-License-Identifier: GPL-3.0-or-later

#include "duration.h"

#ifdef NSEC_PER_USEC
#undef NSEC_PER_USEC
#endif
#define NSEC_PER_USEC (1000ULL)

#ifdef USEC_PER_MS
#undef USEC_PER_MS
#endif
#define USEC_PER_MS (1000ULL)

#ifdef NSEC_PER_SEC
#undef NSEC_PER_SEC
#endif
#define NSEC_PER_SEC (1000000000ULL)

#define NSEC_PER_MS (USEC_PER_MS * NSEC_PER_USEC)
#define NSEC_PER_MIN (NSEC_PER_SEC * 60ULL)
#define NSEC_PER_HOUR (NSEC_PER_MIN * 60ULL)
#define NSEC_PER_DAY (NSEC_PER_HOUR * 24ULL)
#define NSEC_PER_WEEK (NSEC_PER_DAY * 7ULL)

// more accurate, but not an integer multiple of days, weeks, months
#define NSEC_PER_YEAR ((NSEC_PER_DAY * 365ULL) + (NSEC_PER_DAY / 4ULL))

// more accurate, but not an integer multiple of days, weeks
#define NSEC_PER_MONTH (NSEC_PER_YEAR / 12ULL)

// more accurate, but not an integer multiple of days, weeks
#define NSEC_PER_QUARTER (NSEC_PER_YEAR / 4ULL)

// Define a structure to map time units to their multipliers
static const struct duration_unit {
    const char *unit;
    const bool formatter; // true when this unit should be used when formatting to string
    const uint64_t multiplier;
} units[] = {

    // IMPORTANT: the order of this array is crucial!
    // The array should be sorted from the smaller unit to the biggest unit.

    { .unit = "ns",  .formatter = true,  .multiplier = 1 },                 // UCUM
    { .unit = "us",  .formatter = true,  .multiplier = NSEC_PER_USEC },     // UCUM
    { .unit = "ms",  .formatter = true,  .multiplier = NSEC_PER_MS },       // UCUM
    { .unit = "s",   .formatter = true,  .multiplier = NSEC_PER_SEC },      // UCUM
    { .unit = "m",   .formatter = true,  .multiplier = NSEC_PER_MIN },      // -
    { .unit = "min", .formatter = false, .multiplier = NSEC_PER_MIN },      // UCUM
    { .unit = "h",   .formatter = true,  .multiplier = NSEC_PER_HOUR },     // UCUM
    { .unit = "d",   .formatter = true,  .multiplier = NSEC_PER_DAY },      // UCUM
    { .unit = "w",   .formatter = false, .multiplier = NSEC_PER_WEEK },     // -
    { .unit = "wk",  .formatter = false, .multiplier = NSEC_PER_WEEK },     // UCUM
    { .unit = "mo",  .formatter = true,  .multiplier = NSEC_PER_MONTH },    // UCUM
    { .unit = "q",   .formatter = false, .multiplier = NSEC_PER_QUARTER },  // -
    { .unit = "y",   .formatter = true,  .multiplier = NSEC_PER_YEAR },     // -
    { .unit = "a",   .formatter = false, .multiplier = NSEC_PER_YEAR },     // UCUM
};

static inline const struct duration_unit *duration_find_unit(const char *unit) {
    if(!unit || !*unit) return NULL;

    uint8_t first_char_lower = tolower((uint8_t)unit[0]);

    for (size_t i = 0; i < sizeof(units) / sizeof(units[0]); i++) {
        const struct duration_unit *du = &units[i];
        if (first_char_lower == (uint8_t)du->unit[0] && strcasecmp(unit, du->unit) == 0)
            return du;
    }

    return NULL;
}

// -------------------------------------------------------------------------------------------------------------------
// parse a duration string

inline nsec_t duration_str_to_nsec_t(const char *duration, nsec_t default_value, const char *default_unit) {
    if (!duration || !*duration)
        return default_value;

    if (!default_unit)
        default_unit = "ns";

    const char *ptr = duration;
    double value = 0;
    nsec_t nsec = 0;

    while (*ptr) {
        // Skip leading spaces
        while (isspace(*ptr)) ptr++;

        // Parse the number
        value = strtod(ptr, (char **)&ptr);

        // If no valid number found, return default
        if (ptr == duration || value == 0)
            return default_value;

        // Skip spaces between number and unit
        while (isspace(*ptr)) ptr++;

        const char *unit_start = ptr;
        while (isalpha(*ptr)) ptr++;

        char unit[4];
        size_t unit_len = ptr - unit_start;
        if (unit_len == 0)
            strncpyz(unit, default_unit, sizeof(unit) - 1);
        else {
            if (unit_len >= sizeof(unit)) unit_len = sizeof(unit) - 1;
            strncpyz(unit, unit_start, unit_len);
        }

        const struct duration_unit *du = duration_find_unit(unit);
        if(du)
            nsec += (nsec_t)(value * (double)du->multiplier);
        else
            return default_value;
    }

    return nsec;
}

usec_t duration_str_to_usec_t(const char *duration, usec_t default_value) {
    nsec_t nsec = duration_str_to_nsec_t(duration, default_value, "us");
    nsec_t resolution = NSEC_PER_USEC;
    return (usec_t)((nsec + (resolution / 2)) / resolution);
}

time_t duration_str_to_time_t(const char *duration, time_t default_value) {
    nsec_t nsec = duration_str_to_nsec_t(duration, default_value, "s");
    nsec_t resolution = NSEC_PER_SEC;
    return (time_t)((nsec + (resolution / 2)) / resolution);
}

unsigned duration_str_to_days(const char *duration, unsigned default_value) {
    nsec_t nsec = duration_str_to_nsec_t(duration, default_value, "d");
    nsec_t resolution = NSEC_PER_DAY;
    return (unsigned)((nsec + (resolution / 2)) / resolution);
}

// --------------------------------------------------------------------------------------------------------------------
// generate a string to represent a duration

inline size_t duration_str_from_nsec_t(char *dst, size_t dst_size, nsec_t nsec, const char *minimum_unit) {
    if (!dst || dst_size == 0) return 0;
    if (dst_size == 1) {
        dst[0] = '\0';
        return 0;
    }

    const struct duration_unit *du_min = duration_find_unit(minimum_unit);
    size_t offset = 0;

    // Iterate through units from largest to smallest
    for (size_t i = sizeof(units) / sizeof(units[0]) - 1; i > 0 && nsec > 0; i--) {
        const struct duration_unit *du = &units[i];
        if(!units[i].formatter && du != du_min)
            continue;

        // IMPORTANT:
        // The week (7days) is not aligned to the quarter (~91days) or the year (365days).
        // To make sure that the value returned can be parsed back without loss,
        // we have to round the value per unit (inside this loop), not globally.
        // Otherwise, we have to make sure that all larger units, are integer multiples of the smaller ones.

        uint64_t multiplier = units[i].multiplier;
        uint64_t rounded = (du == du_min) ? (((nsec + (multiplier / 2ULL)) / multiplier) * multiplier) : nsec;

        uint64_t unit_count = rounded / multiplier;
        if (unit_count > 0) {
            int written = snprintfz(dst + offset, dst_size - offset,
                                    "%" PRIu64 "%s ", unit_count, units[i].unit);

            if (written < 0 || (size_t)written >= dst_size - offset) {
                // buffer overflow or snprintfz() error
                break;
            }
            offset += written;

            if(unit_count * multiplier >= nsec)
                break;
            else
                nsec -= unit_count * multiplier;
        }

        if(du == du_min)
            // we should not go to smaller units
            break;
    }

    if (offset == 0)
        // nothing has been written
        offset = snprintfz(dst, dst_size, "0%s ", du_min ? du_min->unit : "");

    if (offset > 0 && dst[offset - 1] == ' ') {
        // Remove trailing space
        dst[--offset] = '\0';
    }

    return offset;
}

size_t duration_str_from_usec_t(char *dst, size_t size, usec_t value) {
    return duration_str_from_nsec_t(dst, size, value * NSEC_PER_USEC, "us");
}

size_t duration_str_from_time_t(char *dst, size_t size, time_t value) {
    return duration_str_from_nsec_t(dst, size, value * NSEC_PER_SEC, "s");
}

size_t duration_str_from_days(char *dst, size_t size, unsigned value) {
    return duration_str_from_nsec_t(dst, size, value * NSEC_PER_DAY, "d");
}

// --------------------------------------------------------------------------------------------------------------------

uint64_t nsec_to_unit(nsec_t nsec, const char *unit) {
    const struct duration_unit *du = duration_find_unit(unit);
    if(du)
        return (nsec + (du->multiplier / 2)) / du->multiplier;

    return 0;
}
