// SPDX-License-Identifier: GPL-3.0-or-later

#include "h2o_utils.h"

#include "h2o/string_.h"

#include "libnetdata/libnetdata.h"

char *iovec_to_cstr(h2o_iovec_t *str)
{
    char *c_str = mallocz(str->len + 1);
    memcpy(c_str, str->base, str->len);
    c_str[str->len] = 0;
    return c_str;
}

#define KEY_VAL_BUFFER_GROWTH_STEP 5
h2o_iovec_pair_vector_t *parse_URL_params(h2o_mem_pool_t *pool, h2o_iovec_t params_string)
{
    h2o_iovec_pair_vector_t *params_vec = h2o_mem_alloc_shared(pool, sizeof(h2o_iovec_pair_vector_t), NULL);
    memset(params_vec, 0, sizeof(h2o_iovec_pair_vector_t));

    h2o_iovec_pair_t param;
    while ((param.name.base = (char*)h2o_next_token(&params_string, '&', &param.name.len, &param.value)) != NULL) {
        if (params_vec->capacity == params_vec->size)
            h2o_vector_reserve(pool, params_vec, params_vec->capacity + KEY_VAL_BUFFER_GROWTH_STEP);

        params_vec->entries[params_vec->size++] = param;
    }

    return params_vec;
}

h2o_iovec_pair_t *get_URL_param_by_name(h2o_iovec_pair_vector_t *params_vec, const void *needle, size_t needle_len)
{
    for (size_t i = 0; i < params_vec->size; i++) {
        h2o_iovec_pair_t *ret = &params_vec->entries[i];
        if (h2o_memis(ret->name.base, ret->name.len, needle, needle_len))
            return ret;
    }
    return NULL;
}

char *url_unescape(const char *url)
{
    char *result = mallocz(strlen(url) + 1);

    int i, j;
    for (i = 0, j = 0; url[i] != 0; i++, j++) {
        if (url[i] == '%' && isxdigit(url[i+1]) && isxdigit(url[i+2])) {
            char hex[3] = { url[i+1], url[i+2], 0 };
            result[j] = strtol(hex, NULL, 16);
            i += 2;
        } else
            result[j] = url[i];
    }
    result[j] = 0;

    return result;
}
