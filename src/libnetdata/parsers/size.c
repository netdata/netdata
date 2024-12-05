// SPDX-License-Identifier: GPL-3.0-or-later

#include "size.h"

// Define multipliers for base 2 (binary) units
#define SIZE_MULTIPLIER_BASE2 1024ULL
#define SIZE_MULTIPLIER_KiB (SIZE_MULTIPLIER_BASE2)
#define SIZE_MULTIPLIER_MiB (SIZE_MULTIPLIER_KiB * SIZE_MULTIPLIER_BASE2)
#define SIZE_MULTIPLIER_GiB (SIZE_MULTIPLIER_MiB * SIZE_MULTIPLIER_BASE2)
#define SIZE_MULTIPLIER_TiB (SIZE_MULTIPLIER_GiB * SIZE_MULTIPLIER_BASE2)
#define SIZE_MULTIPLIER_PiB (SIZE_MULTIPLIER_TiB * SIZE_MULTIPLIER_BASE2)
//#define SIZE_MULTIPLIER_EiB (SIZE_MULTIPLIER_PiB * SIZE_MULTIPLIER_BASE2)
//#define SIZE_MULTIPLIER_ZiB (SIZE_MULTIPLIER_EiB * SIZE_MULTIPLIER_BASE2)
//#define SIZE_MULTIPLIER_YiB (SIZE_MULTIPLIER_ZiB * SIZE_MULTIPLIER_BASE2)

// Define multipliers for base 10 (decimal) units
#define SIZE_MULTIPLIER_BASE10 1000ULL
#define SIZE_MULTIPLIER_K (SIZE_MULTIPLIER_BASE10)
#define SIZE_MULTIPLIER_M (SIZE_MULTIPLIER_K * SIZE_MULTIPLIER_BASE10)
#define SIZE_MULTIPLIER_G (SIZE_MULTIPLIER_M * SIZE_MULTIPLIER_BASE10)
#define SIZE_MULTIPLIER_T (SIZE_MULTIPLIER_G * SIZE_MULTIPLIER_BASE10)
#define SIZE_MULTIPLIER_P (SIZE_MULTIPLIER_T * SIZE_MULTIPLIER_BASE10)
//#define SIZE_MULTIPLIER_E (SIZE_MULTIPLIER_P * SIZE_MULTIPLIER_BASE10)
//#define SIZE_MULTIPLIER_Z (SIZE_MULTIPLIER_E * SIZE_MULTIPLIER_BASE10)
//#define SIZE_MULTIPLIER_Y (SIZE_MULTIPLIER_Z * SIZE_MULTIPLIER_BASE10)

// Define a structure to map size units to their multipliers
static const struct size_unit {
    const char *unit;
    const uint8_t base;
    const bool formatter; // true when this unit should be used when formatting to string
    const uint64_t multiplier;
} size_units[] = {
    // the order of this table is important: smaller to bigger units!

    { .unit = "B",   .base = 2,  .formatter = true,  .multiplier = 1ULL },
    { .unit = "k",   .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_K },
    { .unit = "K",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_K },
    { .unit = "KB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_K },
    { .unit = "KiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_KiB },
    { .unit = "M",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_M },
    { .unit = "MB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_M },
    { .unit = "MiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_MiB },
    { .unit = "G",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_G },
    { .unit = "GB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_G },
    { .unit = "GiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_GiB },
    { .unit = "T",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_T },
    { .unit = "TB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_T },
    { .unit = "TiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_TiB },
    { .unit = "P",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_P },
    { .unit = "PB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_P },
    { .unit = "PiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_PiB },
//    { .unit = "E",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_E },
//    { .unit = "EB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_E },
//    { .unit = "EiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_EiB },
//    { .unit = "Z",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_Z },
//    { .unit = "ZB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_Z },
//    { .unit = "ZiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_ZiB },
//    { .unit = "Y",   .base = 10, .formatter = true,  .multiplier = SIZE_MULTIPLIER_Y },
//    { .unit = "YB",  .base = 10, .formatter = false, .multiplier = SIZE_MULTIPLIER_Y },
//    { .unit = "YiB", .base = 2,  .formatter = true,  .multiplier = SIZE_MULTIPLIER_YiB },
};

static inline const struct size_unit *size_find_unit(const char *unit) {
    if (!unit || !*unit) unit = "B";

    for (size_t i = 0; i < sizeof(size_units) / sizeof(size_units[0]); i++) {
        const struct size_unit *su = &size_units[i];
        if ((uint8_t)unit[0] == (uint8_t)su->unit[0] && strcmp(unit, su->unit) == 0)
            return su;
    }

    return NULL;
}

static inline double size_round_to_resolution_dbl2(uint64_t value, uint64_t resolution) {
    double converted = (double)value / (double)resolution;
    return round(converted * 100.0) / 100.0;
}

static inline uint64_t size_round_to_resolution_int(uint64_t value, uint64_t resolution) {
    return (value + (resolution / 2)) / resolution;
}

// -------------------------------------------------------------------------------------------------------------------
// parse a size string

bool size_parse(const char *size_str, uint64_t *result, const char *default_unit) {
    if (!size_str || !*size_str) {
        *result = 0;
        return false;
    }

    const struct size_unit *su_def = size_find_unit(default_unit);
    if(!su_def) {
        *result = 0;
        return false;
    }

    const char *s = size_str;

    // Skip leading spaces
    while (isspace((uint8_t)*s)) s++;

    if(strcmp(s, "off") == 0) {
        *result = 0;
        return true;
    }

    // Parse the number
    const char *number_start = s;
    NETDATA_DOUBLE value = strtondd(s, (char **)&s);

    // If no valid number found, return false
    if (s == number_start || value < 0) {
        *result = 0;
        return false;
    }

    // Skip spaces between number and unit
    while (isspace((uint8_t)*s)) s++;

    const char *unit_start = s;
    while (isalpha((uint8_t)*s)) s++;

    char unit[4];
    size_t unit_len = s - unit_start;
    const struct size_unit *su;
    if (unit_len == 0)
        su = su_def;
    else {
        if (unit_len >= sizeof(unit)) unit_len = sizeof(unit) - 1;
        strncpy(unit, unit_start, unit_len);
        unit[unit_len] = '\0';
        su = size_find_unit(unit);
        if (!su) {
            *result = 0;
            return false;
        }
    }

    uint64_t bytes = (uint64_t)round(value * (NETDATA_DOUBLE)su->multiplier);
    *result = size_round_to_resolution_int(bytes, su_def->multiplier);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// generate a string to represent a size

ssize_t size_snprintf(char *dst, size_t dst_size, uint64_t value, const char *unit, bool accurate) {
    if (!dst || dst_size == 0) return -1;
    if (dst_size == 1) {
        dst[0] = '\0';
        return -2;
    }

    if (value == 0)
        return snprintfz(dst, dst_size, "off");

    const struct size_unit *su_def = size_find_unit(unit);
    if(!su_def) return -3;

    // use the units multiplier to find the units
    uint64_t bytes = value * su_def->multiplier;

    // Find the best unit to represent the size with up to 2 fractional digits
    const struct size_unit *su_best = su_def;
    for (size_t i = 0; i < sizeof(size_units) / sizeof(size_units[0]); i++) {
        const struct size_unit *su = &size_units[i];
        if (su->base != su_def->base                ||  // not the right base
            su->multiplier < su_def->multiplier     ||  // the multiplier is too small
            (!su->formatter && su != su_def)        ||  // it is not to be used in formatting (except our unit)
            (bytes < su->multiplier && su != su_def) )  // the converted value will be <1.0
            continue;

        double converted = size_round_to_resolution_dbl2(bytes, su->multiplier);
        uint64_t reversed_bytes = (uint64_t)round((converted * (double)su->multiplier));

        if(accurate) {
            // no precision loss is required
            if (reversed_bytes == bytes && converted > 1.0)
                // no precision loss, this is good to use
                su_best = su;
        }
        else {
            if(converted > 1.0)
                su_best = su;
        }
    }

    double converted = size_round_to_resolution_dbl2(bytes, su_best->multiplier);

    // print it either with 0, 1 or 2 fractional digits
    int written;
    if(converted == (double)((uint64_t)converted))
        written = snprintfz(dst, dst_size, "%.0f%s", converted, su_best->unit);
    else if(converted * 10.0 == (double)((uint64_t)(converted * 10.0)))
        written = snprintfz(dst, dst_size, "%.1f%s", converted, su_best->unit);
    else
        written = snprintfz(dst, dst_size, "%.2f%s", converted, su_best->unit);

    if (written < 0)
        return -4;

    if ((size_t)written >= dst_size)
        return (ssize_t)(dst_size - 1);

    return written;
}

