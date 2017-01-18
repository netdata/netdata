#ifndef NETDATA_SIMPLE_PATTERN_H
#define NETDATA_SIMPLE_PATTERN_H

/**
 * @file simple_pattern.h
 * @brief API for matching patterns
 */

/// Matching strategys
typedef enum {
    SIMPLE_PATTERN_EXACT,    /// only exact match
    SIMPLE_PATTERN_PREFIX,   /// only match a prefix
    SIMPLE_PATTERN_SUFFIX,   /// only match a suffix
    SIMPLE_PATTERN_SUBSTRING /// match anywhere
} SIMPLE_PREFIX_MODE;

/// A pattern generated with `simple_pattern_create()`
typedef void SIMPLE_PATTERN;

/**
 * Create a simple_pattern from the string given.
 * `default_mode` is used in cases where EXACT matches, without an asterisk,
 * should be considered PREFIX matches.
 *
 * @param list String to generate simple_pattern from.
 * @param default_mode to use.
 * @return created pattern
 */
extern SIMPLE_PATTERN *simple_pattern_create(const char *list, SIMPLE_PREFIX_MODE default_mode);

/** 
 * Test if string str is matched from the pattern.
 *
 * @param list a pattern created with `simple_pattern_create`
 * @param str to match
 * @return boolean matches
 */
extern int simple_pattern_matches(SIMPLE_PATTERN *list, const char *str);

/** 
 * Free a simple_pattern
 *
 * Free a simple_pattern that was created with `simple_pattern_create()`
 * list can be `NULL`, in which case, this does nothing.
 *
 * @param list pattern
 */
extern void simple_pattern_free(SIMPLE_PATTERN *list);

#endif //NETDATA_SIMPLE_PATTERN_H
