// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_H2O_UTILS_H
#define NETDATA_H2O_UTILS_H

#include "h2o/memory.h"

#define __HAS_URL_PARAMS(reqptr) ((reqptr)->query_at != SIZE_MAX && ((reqptr)->path.len - (reqptr)->query_at > 1))
#define IF_HAS_URL_PARAMS(reqptr) if __HAS_URL_PARAMS(reqptr)
#define UNLESS_HAS_URL_PARAMS(reqptr) if (!__HAS_URL_PARAMS(reqptr))
#define URL_PARAMS_IOVEC_INIT(reqptr) { .base = &(reqptr)->path.base[(reqptr)->query_at + 1], \
            .len = (reqptr)->path.len - (reqptr)->query_at - 1 }
#define URL_PARAMS_IOVEC_INIT_WITH_QUESTIONMARK(reqptr) { .base = &(reqptr)->path.base[(reqptr)->query_at], \
            .len = (reqptr)->path.len - (reqptr)->query_at }

#define PRINTF_H2O_IOVEC_FMT "%.*s"
#define PRINTF_H2O_IOVEC(iovec) ((int)(iovec)->len), ((iovec)->base)

char *iovec_to_cstr(h2o_iovec_t *str);

typedef struct h2o_iovec_pair {
    h2o_iovec_t name;
    h2o_iovec_t value;
} h2o_iovec_pair_t;

typedef H2O_VECTOR(h2o_iovec_pair_t) h2o_iovec_pair_vector_t;

// Takes the part of url behind ? (the url encoded parameters)
// and parse it to vector of name/value pairs without copying the actual strings
h2o_iovec_pair_vector_t *parse_URL_params(h2o_mem_pool_t *pool, h2o_iovec_t params_string);

// Searches for parameter by name (provided in needle)
// returns pointer to it or NULL
h2o_iovec_pair_t *get_URL_param_by_name(h2o_iovec_pair_vector_t *params_vec, const void *needle, size_t needle_len);

char *url_unescape(const char *url);

#endif /* NETDATA_H2O_UTILS_H */
