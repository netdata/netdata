// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_H
#define NETDATA_UUID_H

static inline int uuid_memcmp(const uuid_t *uu1, const uuid_t *uu2) {
    return memcmp(uu1, uu2, sizeof(uuid_t));
}

#define UUID_COMPACT_STR_LEN 33
void uuid_unparse_lower_compact(const uuid_t uuid, char *out);

#endif //NETDATA_UUID_H
