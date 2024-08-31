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
    const snsec_t multiplier;
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
    { .unit = "M",   .formatter = false, .multiplier = NSEC_PER_MONTH },    // compatibility
    { .unit = "q",   .formatter = false, .multiplier = NSEC_PER_QUARTER },  // -
    { .unit = "y",   .formatter = true,  .multiplier = NSEC_PER_YEAR },     // -
    { .unit = "Y",   .formatter = false, .multiplier = NSEC_PER_YEAR },     // compatibility
    { .unit = "a",   .formatter = false, .multiplier = NSEC_PER_YEAR },     // UCUM
};

static inline const struct duration_unit *duration_find_unit(const char *unit) {
    if(!unit || !*unit) return NULL;

    for (size_t i = 0; i < sizeof(units) / sizeof(units[0]); i++) {
        const struct duration_unit *du = &units[i];
        if ((uint8_t)unit[0] == (uint8_t)du->unit[0] && strcmp(unit, du->unit) == 0)
            return du;
    }

    return NULL;
}

// -------------------------------------------------------------------------------------------------------------------
// parse a duration string

inline bool duration_str_to_nsec_t(const char *duration, snsec_t *result, const char *default_unit) {
    if (!duration || !*duration || !default_unit || !*default_unit)
        return false;

    const char *s = duration;
    snsec_t nsec = 0;

    while (*s) {
        // Skip leading spaces
        while (isspace((uint8_t)*s)) s++;

        if(*s == 'n' && strcmp(s, "never") == 0) {
            *result = 0;
            return true;
        }

        // Parse the number
        const char *number_start = s;
        NETDATA_DOUBLE value = str2ndd(s, (char **)&s);

        // If no valid number found, return default
        if (s == number_start)
            return false;

        // Skip spaces between number and unit
        while (isspace((uint8_t)*s)) s++;

        const char *unit_start = s;
        while (isalpha((uint8_t)*s)) s++;

        char unit[4];
        size_t unit_len = s - unit_start;
        if (unit_len == 0)
            strncpyz(unit, default_unit, sizeof(unit) - 1);
        else {
            if (unit_len >= sizeof(unit)) unit_len = sizeof(unit) - 1;
            strncpyz(unit, unit_start, unit_len);
        }

        const struct duration_unit *du = duration_find_unit(unit);
        if(!du) return false;
        nsec += (snsec_t)(value * (double)du->multiplier);
    }

    *result = nsec;
    return true;
}

bool duration_str_to_usec_t(const char *duration, susec_t *result) {
    snsec_t nsec;
    if(!duration_str_to_nsec_t(duration, &nsec, "us"))
        return false;

    nsec_t resolution = NSEC_PER_USEC;
    *result = (susec_t)((nsec + (resolution / 2)) / resolution);
    return true;
}

bool duration_str_to_time_t(const char *duration, time_t *result) {
    snsec_t nsec;
    if(!duration_str_to_nsec_t(duration, &nsec, "s"))
        return false;

    nsec_t resolution = NSEC_PER_SEC;
    *result = (time_t)((nsec + (resolution / 2)) / resolution);
    return true;
}

bool duration_str_to_days(const char *duration, int *result) {
    snsec_t nsec;
    if(!duration_str_to_nsec_t(duration, &nsec, "d"))
        return false;

    nsec_t resolution = NSEC_PER_DAY;
    *result = (int)((nsec + (resolution / 2)) / resolution);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// generate a string to represent a duration

inline size_t duration_str_from_nsec_t(char *dst, size_t dst_size, snsec_t nsec, const char *minimum_unit) {
    if (!dst || dst_size == 0) return 0;
    if (dst_size == 1) {
        dst[0] = '\0';
        return 0;
    }

    if(nsec == 0)
        return snprintfz(dst, dst_size, "never");

    const struct duration_unit *du_min = duration_find_unit(minimum_unit);
    size_t offset = 0;

    // Iterate through units from largest to smallest
    for (size_t i = sizeof(units) / sizeof(units[0]) - 1; i > 0 && nsec > 0; i--) {
        const struct duration_unit *du = &units[i];
        if(!units[i].formatter && du != du_min)
            continue;

        // IMPORTANT:
        // The week (7 days) is not aligned to the quarter (~91 days) or the year (365.25 days).
        // To make sure that the value returned can be parsed back without loss,
        // we have to round the value per unit (inside this loop), not globally.
        // Otherwise, we have to make sure that all larger units are integer multiples of the smaller ones.

        int64_t multiplier = units[i].multiplier;
        int64_t rounded = (du == du_min) ? (((nsec + (multiplier / 2LL)) / multiplier) * multiplier) : nsec;

        int64_t unit_count = rounded / multiplier;
        if (unit_count > 0) {
            int written = snprintfz(dst + offset, dst_size - offset,
                                    "%" PRIu64 "%s", unit_count, units[i].unit);

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
        offset = snprintfz(dst, dst_size, "never");

    return offset;
}

size_t duration_str_from_usec_t(char *dst, size_t size, susec_t value) {
    return duration_str_from_nsec_t(dst, size, (snsec_t)value * (snsec_t)NSEC_PER_USEC, "us");
}

size_t duration_str_from_time_t(char *dst, size_t size, time_t value) {
    return duration_str_from_nsec_t(dst, size, (snsec_t)value * (snsec_t)NSEC_PER_SEC, "s");
}

size_t duration_str_from_days(char *dst, size_t size, int value) {
    return duration_str_from_nsec_t(dst, size, (snsec_t)value * (snsec_t)NSEC_PER_DAY, "d");
}

// --------------------------------------------------------------------------------------------------------------------

int64_t nsec_to_unit(snsec_t nsec, const char *unit) {
    const struct duration_unit *du = duration_find_unit(unit);
    if(du)
        return (nsec + (du->multiplier / 2)) / du->multiplier;

    return 0;
}
