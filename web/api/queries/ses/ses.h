// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_SES_H
#define NETDATA_API_QUERIES_SES_H

#include "../query.h"
#include "../rrdr.h"

void grouping_init_ses(void);

void grouping_create_ses(RRDR *r, const char *options __maybe_unused);
void grouping_reset_ses(RRDR *r);
void grouping_free_ses(RRDR *r);
void grouping_add_ses(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_ses(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_SES_H
