// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_PATTERN_H
#define NETDATA_SIMPLE_PATTERN_H

#include "../libnetdata.h"

typedef enum __attribute__ ((__packed__)) {
    SIMPLE_PATTERN_EXACT,
    SIMPLE_PATTERN_PREFIX,
    SIMPLE_PATTERN_SUFFIX,
    SIMPLE_PATTERN_SUBSTRING
} SIMPLE_PREFIX_MODE;

typedef enum __attribute__ ((__packed__)) {
    SP_NOT_MATCHED,
    SP_MATCHED_NEGATIVE,
    SP_MATCHED_POSITIVE,
} SIMPLE_PATTERN_RESULT;

struct simple_pattern;
typedef struct simple_pattern SIMPLE_PATTERN;

typedef struct simple_pattern_index SIMPLE_PATTERN_INDEX;
typedef struct simple_pattern_index_matches SIMPLE_PATTERN_INDEX_MATCHES;

#define SIMPLE_PATTERN_NO_SEPARATORS (const char *)(0xFFFFFFFF)

// create a simple_pattern from the string given
// default_mode is used in cases where EXACT matches, without an asterisk,
// should be considered PREFIX matches.
SIMPLE_PATTERN *simple_pattern_create(const char *list, const char *separators, SIMPLE_PREFIX_MODE default_mode, bool case_sensitive);

struct netdata_string;

// test if string str is matched from the pattern and fill 'wildcarded' with the parts matched by '*'
SIMPLE_PATTERN_RESULT simple_pattern_matches_extract(SIMPLE_PATTERN *list, const char *str, char *wildcarded, size_t wildcarded_size);
SIMPLE_PATTERN_RESULT simple_pattern_matches_string_extract(SIMPLE_PATTERN *list, struct netdata_string *str, char *wildcarded, size_t wildcarded_size);
SIMPLE_PATTERN_RESULT simple_pattern_matches_buffer_extract(SIMPLE_PATTERN *list, BUFFER *str, char *wildcarded, size_t wildcarded_size);
SIMPLE_PATTERN_RESULT simple_pattern_matches_length_extract(SIMPLE_PATTERN *list, const char *str, size_t len, char *wildcarded, size_t wildcarded_size);

// test if string str is matched from the pattern
#define simple_pattern_matches(list, str) (simple_pattern_matches_extract(list, str, NULL, 0) == SP_MATCHED_POSITIVE)
#define simple_pattern_matches_string(list, str) (simple_pattern_matches_string_extract(list, str, NULL, 0) == SP_MATCHED_POSITIVE)
#define simple_pattern_matches_buffer(list, str) (simple_pattern_matches_buffer_extract(list, str, NULL, 0) == SP_MATCHED_POSITIVE)

// free a simple_pattern that was created with simple_pattern_create()
// list can be NULL, in which case, this does nothing.
void simple_pattern_free(SIMPLE_PATTERN *list);

void simple_pattern_dump(uint64_t debug_type, SIMPLE_PATTERN *p) ;
int simple_pattern_is_potential_name(SIMPLE_PATTERN *p) ;
char *simple_pattern_iterate(SIMPLE_PATTERN **p);

// check if string contains pattern wildcards (*, ! prefix, or separators)
bool simple_pattern_contains_wildcards(const char *str, const char *separators);

// A thread-safe many-to-many index from interned strings to non-NULL user pointers.
// The index owns one STRING reference per distinct key. Match results contain
// borrowed user pointers and preserve the caller's existing lifetime contract.
SIMPLE_PATTERN_INDEX *simple_pattern_index_create(void);
void simple_pattern_index_destroy(SIMPLE_PATTERN_INDEX *index);
bool simple_pattern_index_add(SIMPLE_PATTERN_INDEX *index, struct netdata_string *key, void *user);
bool simple_pattern_index_del(SIMPLE_PATTERN_INDEX *index, struct netdata_string *key, void *user);
size_t simple_pattern_index_del_user(SIMPLE_PATTERN_INDEX *index, void *user);
bool simple_pattern_index_replace_user(
    SIMPLE_PATTERN_INDEX *index, struct netdata_string *const *keys, size_t keys_count, void *user);

// A NULL pattern selects every indexed user. Positive rules select users and a
// negative match on any key is a permanent veto. Duplicate users are returned once.
SIMPLE_PATTERN_INDEX_MATCHES *simple_pattern_index_search(SIMPLE_PATTERN_INDEX *index, SIMPLE_PATTERN *pattern);
bool simple_pattern_index_matches_contains(SIMPLE_PATTERN_INDEX_MATCHES *matches, const void *user);
void *simple_pattern_index_matches_first(SIMPLE_PATTERN_INDEX_MATCHES *matches, Word_t *cursor);
void *simple_pattern_index_matches_next(SIMPLE_PATTERN_INDEX_MATCHES *matches, Word_t *cursor);
size_t simple_pattern_index_matches_count(SIMPLE_PATTERN_INDEX_MATCHES *matches);
void simple_pattern_index_matches_free(SIMPLE_PATTERN_INDEX_MATCHES *matches);

int simple_pattern_index_unittest(void);

#define SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS ",|\t\r\n\f\v"

#define is_valid_sp(x) ((x) && *(x) && !((x)[0] == '*' && (x)[1] == '\0'))

#define string_to_simple_pattern(str) (is_valid_sp(str) ? simple_pattern_create(str, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS, SIMPLE_PATTERN_EXACT, true) : NULL)
#define string_to_simple_pattern_nocase(str) (is_valid_sp(str) ? simple_pattern_create(str, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS, SIMPLE_PATTERN_EXACT, false) : NULL)
#define string_to_simple_pattern_nocase_substring(str) (is_valid_sp(str) ? simple_pattern_create(str, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS, SIMPLE_PATTERN_SUBSTRING, false) : NULL)

#endif //NETDATA_SIMPLE_PATTERN_H
