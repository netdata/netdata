// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_DES_H
#define NETDATA_API_QUERIES_DES_H

#include "../query.h"
#include "../rrdr.h"

extern void grouping_init_des(void);

extern void grouping_create_des(RRDR *r, const char *options __maybe_unused);
extern void grouping_reset_des(RRDR *r);
extern void grouping_free_des(RRDR *r);
extern void grouping_add_des(RRDR *r, NETDATA_DOUBLE value);
extern NETDATA_DOUBLE grouping_flush_des(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_DES_H
