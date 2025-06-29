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
#define NSEC_PER_MONTH (NSEC_PER_DAY * 30ULL)
#define NSEC_PER_QUARTER (NSEC_PER_MONTH * 3ULL)

// more accurate, but not an integer multiple of days, weeks, months
#define NSEC_PER_YEAR (NSEC_PER_DAY * 365ULL)

// Define a structure to map time units to their multipliers
static const struct duration_unit {
    const char *unit;
    const bool formatter; // true when this unit should be used when formatting to string
    const snsec_t multiplier;
} units[] = {

    // IMPORTANT: the order of this array is crucial!
    // The array should be sorted from the smaller unit to the biggest unit.
    // For each multiplier value, the first entry with formatter=true is used for generation.

    { .unit = "ns",  .formatter = true,  .multiplier = 1 },                 // UCUM
    { .unit = "nanosecond",  .formatter = false, .multiplier = 1 },
    { .unit = "nanoseconds", .formatter = false, .multiplier = 1 },
    
    { .unit = "us",  .formatter = true,  .multiplier = NSEC_PER_USEC },     // UCUM
    { .unit = "microsecond",  .formatter = false, .multiplier = NSEC_PER_USEC },
    { .unit = "microseconds", .formatter = false, .multiplier = NSEC_PER_USEC },
    
    { .unit = "ms",  .formatter = true,  .multiplier = NSEC_PER_MS },       // UCUM
    { .unit = "millisecond",  .formatter = false, .multiplier = NSEC_PER_MS },
    { .unit = "milliseconds", .formatter = false, .multiplier = NSEC_PER_MS },
    
    { .unit = "s",   .formatter = true,  .multiplier = NSEC_PER_SEC },      // UCUM
    { .unit = "sec",  .formatter = false, .multiplier = NSEC_PER_SEC },
    { .unit = "secs", .formatter = false, .multiplier = NSEC_PER_SEC },
    { .unit = "second",  .formatter = false, .multiplier = NSEC_PER_SEC },
    { .unit = "seconds", .formatter = false, .multiplier = NSEC_PER_SEC },
    
    { .unit = "m",   .formatter = true,  .multiplier = NSEC_PER_MIN },      // -
    { .unit = "min", .formatter = false, .multiplier = NSEC_PER_MIN },      // UCUM
    { .unit = "minute",  .formatter = false, .multiplier = NSEC_PER_MIN },
    { .unit = "minutes", .formatter = false, .multiplier = NSEC_PER_MIN },
    
    { .unit = "h",   .formatter = true,  .multiplier = NSEC_PER_HOUR },     // UCUM
    { .unit = "hr",  .formatter = false, .multiplier = NSEC_PER_HOUR },
    { .unit = "hrs", .formatter = false, .multiplier = NSEC_PER_HOUR },
    { .unit = "hour",  .formatter = false, .multiplier = NSEC_PER_HOUR },
    { .unit = "hours", .formatter = false, .multiplier = NSEC_PER_HOUR },
    
    { .unit = "d",   .formatter = true,  .multiplier = NSEC_PER_DAY },      // UCUM
    { .unit = "day",  .formatter = false, .multiplier = NSEC_PER_DAY },
    { .unit = "days", .formatter = false, .multiplier = NSEC_PER_DAY },
    
    { .unit = "w",   .formatter = false, .multiplier = NSEC_PER_WEEK },     // -
    { .unit = "wk",  .formatter = false, .multiplier = NSEC_PER_WEEK },     // UCUM
    { .unit = "week",  .formatter = false, .multiplier = NSEC_PER_WEEK },
    { .unit = "weeks", .formatter = false, .multiplier = NSEC_PER_WEEK },
    
    { .unit = "mo",  .formatter = true,  .multiplier = NSEC_PER_MONTH },    // UCUM
    { .unit = "M",   .formatter = false, .multiplier = NSEC_PER_MONTH },    // compatibility
    { .unit = "month",  .formatter = false, .multiplier = NSEC_PER_MONTH },
    { .unit = "months", .formatter = false, .multiplier = NSEC_PER_MONTH },
    
    { .unit = "q",   .formatter = false, .multiplier = NSEC_PER_QUARTER },  // -
    { .unit = "quarter",  .formatter = false, .multiplier = NSEC_PER_QUARTER },
    { .unit = "quarters", .formatter = false, .multiplier = NSEC_PER_QUARTER },
    
    { .unit = "y",   .formatter = true,  .multiplier = NSEC_PER_YEAR },     // -
    { .unit = "Y",   .formatter = false, .multiplier = NSEC_PER_YEAR },     // compatibility
    { .unit = "a",   .formatter = false, .multiplier = NSEC_PER_YEAR },     // UCUM
    { .unit = "year",  .formatter = false, .multiplier = NSEC_PER_YEAR },
    { .unit = "years", .formatter = false, .multiplier = NSEC_PER_YEAR }
};

static inline const struct duration_unit *duration_find_unit(const char *unit) {
    if(!unit || !*unit)
        unit = "ns";

    for (size_t i = 0; i < sizeof(units) / sizeof(units[0]); i++) {
        const struct duration_unit *du = &units[i];
        if (strcasecmp(unit, du->unit) == 0)
            return du;
    }

    return NULL;
}

inline int64_t duration_round_to_resolution(int64_t value, int64_t resolution) {
    if(value > 0)
        return (value + ((resolution - 1) / 2)) / resolution;

    if(value < 0)
        return (value - ((resolution - 1) / 2)) / resolution;

    return 0;
}

// -------------------------------------------------------------------------------------------------------------------
// parse a duration string

bool duration_parse(const char *duration, int64_t *result, const char *default_unit, const char *output_unit) {
    if (!duration || !*duration) {
        *result = 0;
        return false;
    }

    const struct duration_unit *du_def = duration_find_unit(default_unit);
    if(!du_def) {
        *result = 0;
        return false;
    }

    const struct duration_unit *du_out = duration_find_unit(output_unit);
    if(!du_out) {
        *result = 0;
        return false;
    }

    int64_t sign = 1;
    const char *s = duration;
    while (isspace((uint8_t)*s)) s++;
    if(*s == '-') {
        s++;
        sign = -1;
    }

    int64_t v = 0;
    bool found_ago = false;
    bool parsed_any_duration = false;

    while (*s) {
        // Skip leading spaces
        while (isspace((uint8_t)*s)) s++;
        
        // If no more content, break out of the loop
        if (!*s) break;

        // compatibility - case insensitive
        if(*s == 'n' || *s == 'N') {
            if(strcasecmp(s, "never") == 0) {
                *result = 0;
                return true;
            }
        }

        if(*s == 'o' || *s == 'O') {
            if(strcasecmp(s, "off") == 0) {
                *result = 0;
                return true;
            }
        }

        // Parse the number
        const char *number_start = s;
        NETDATA_DOUBLE value = str2ndd(s, (char **)&s);

        // If no valid number found, check if it's "ago"
        if (s == number_start) {
            // Maybe it's the "ago" suffix
            if (strcasecmp(s, "ago") == 0) {
                found_ago = true;
                s += 3;  // Skip "ago"
                break;   // Exit the loop
            }
            *result = 0;
            return false;
        }

        // Skip spaces between number and unit
        while (isspace((uint8_t)*s)) s++;

        const char *unit_start = s;
        while (isalpha((uint8_t)*s)) s++;

        char unit[16];  // Increased to handle "microseconds" (12 chars)
        size_t unit_len = s - unit_start;
        const struct duration_unit *du = NULL;
        
        if (unit_len == 0) {
            du = du_def;
        }
        else {
            // First check if we have "ago" at the end of the alphabetic sequence
            if (unit_len >= 3 && strncasecmp(s - 3, "ago", 3) == 0) {
                // We might have something like "daysago"
                // Try to parse the unit without "ago"
                unit_len -= 3;
                if (unit_len > 0 && unit_len < sizeof(unit)) {
                    strncpyz(unit, unit_start, unit_len);
                    du = duration_find_unit(unit);
                    if (du) {
                        // Successfully found the unit, mark that we found "ago"
                        found_ago = true;
                        s -= 3;  // Back up to just after the unit, before "ago"
                    }
                }
            }
            
            // If we didn't find a unit with "ago" suffix, try the whole thing
            if (!du) {
                unit_len = s - unit_start;
                if (unit_len >= sizeof(unit)) unit_len = sizeof(unit) - 1;
                strncpyz(unit, unit_start, unit_len);
                
                // Check if this might be "ago" by itself
                if (strcasecmp(unit, "ago") == 0) {
                    // Found "ago" - this ends the duration parsing
                    found_ago = true;
                    break;
                }
                
                du = duration_find_unit(unit);
                if(!du) {
                    *result = 0;
                    return false;
                }
            }
        }

        v += (int64_t)round(value * (NETDATA_DOUBLE)du->multiplier);
        parsed_any_duration = true;
    }

    v *= sign;

    // Check for "ago" suffix to negate the result if not already found
    if (!found_ago) {
        // Skip any trailing whitespace
        while (isspace((uint8_t)*s)) s++;
        
        // Check if the remaining string is "ago" (case-insensitive)
        if (*s) {
            if (strcasecmp(s, "ago") == 0) {
                found_ago = true;
                s += 3;  // Skip past "ago"
            }
            else {
                // If there's any other trailing text, it's an error
                *result = 0;
                return false;
            }
        }
    }
    
    // Apply "ago" negation if found
    if (found_ago) {
        // But only if we actually parsed some duration
        if (!parsed_any_duration) {
            // "ago" without any duration is an error
            *result = 0;
            return false;
        }
        
        // If the original sign was negative, "ago" is redundant
        // For example: "-7 days ago" means the same as "-7 days"
        // We keep it negative (don't apply double negative)
        if (sign > 0) {
            v = -v;  // Only negate if originally positive
        }
        // If sign < 0, v is already negative, so we keep it that way
        
        // Check for any trailing content after "ago"
        while (isspace((uint8_t)*s)) s++;
        if (*s) {
            // Extra text after "ago" is an error
            *result = 0;
            return false;
        }
    }

    // Convert the final value from nanoseconds to the desired output unit
    // and apply appropriate rounding
    if(du_out->multiplier == 1)
        *result = v;
    else {
        // First convert to the output unit
        NETDATA_DOUBLE converted = (NETDATA_DOUBLE)v / (NETDATA_DOUBLE)du_out->multiplier;
        // Then round to nearest integer in the output unit
        *result = (int64_t)round(converted);
    }

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// generate a string to represent a duration

ssize_t duration_snprintf(char *dst, size_t dst_size, int64_t value, const char *unit, bool add_spaces) {
    if (!dst || dst_size == 0) return -1;
    if (dst_size == 1) {
        dst[0] = '\0';
        return -2;
    }

    if(value == 0)
        return snprintfz(dst, dst_size, "off");

    const char *sign = "";
    if(value < 0) {
        sign = "-";
        value = -value;
    }

    const struct duration_unit *du_min = duration_find_unit(unit);
    size_t offset = 0;

    int64_t nsec = value * du_min->multiplier;

    // Iterate through units from largest to smallest
    for (ssize_t i = (ssize_t)(sizeof(units) / sizeof(units[0])) - 1; i >= 0 && nsec > 0; i--) {
        const struct duration_unit *du = &units[i];
        if(!units[i].formatter && du != du_min)
            continue;

        // IMPORTANT:
        // The week (7 days) is not aligned to the quarter (~91 days) or the year (365.25 days).
        // To make sure that the value returned can be parsed back without loss,
        // we have to round the value per unit (inside this loop), not globally.
        // Otherwise, we have to make sure that all larger units are integer multiples of the smaller ones.

        int64_t multiplier = units[i].multiplier;
        int64_t rounded = (du == du_min) ? (duration_round_to_resolution(nsec, multiplier) * multiplier) : nsec;

        int64_t unit_count = rounded / multiplier;
        if (unit_count > 0) {
            const char *space = (add_spaces && offset) ? " " : "";
            int written = snprintfz(dst + offset, dst_size - offset,
                                    "%s%s%" PRIi64 "%s", space, sign, unit_count, units[i].unit);

            if (written < 0)
                return -3;

            sign = "";
            offset += written;

            if (offset >= dst_size) {
                // buffer overflow
                return (ssize_t)offset;
            }

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
        offset = snprintfz(dst, dst_size, "off");

    return (ssize_t)offset;
}

// --------------------------------------------------------------------------------------------------------------------
// compatibility for parsing seconds in int.

bool duration_parse_seconds(const char *str, int *result) {
    int64_t v;

    if(duration_parse_time_t(str, &v)) {
        *result = (int)v;
        return true;
    }

    return false;
}
