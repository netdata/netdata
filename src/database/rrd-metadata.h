// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_METADATA_H
#define NETDATA_RRD_METADATA_H

#include "libnetdata/libnetdata.h"

// struct to hold node, metrics, instances and contexts statistics
typedef struct rrdstats_metadata {
    struct {
        size_t total;
        size_t receiving;
        size_t sending;
        size_t archived;
    } nodes;

    struct {
        size_t collected;
        size_t available;
    } metrics;

    struct {
        size_t collected;
        size_t available;
    } instances;

    struct {
        size_t collected;
        size_t available;
    } contexts;
} RRDSTATS_METADATA;

// Function to collect all metadata statistics
RRDSTATS_METADATA rrdstats_metadata_collect(void);

#endif // NETDATA_RRD_METADATA_H