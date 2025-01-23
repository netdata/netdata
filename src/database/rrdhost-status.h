// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_STATUS_H
#define NETDATA_RRDHOST_STATUS_H

#include "libnetdata/libnetdata.h"

typedef enum __attribute__((packed)) {
    RRDHOST_STATUS_BASIC    = 0,
    RRDHOST_STATUS_STREAM   = (1 << 0),
    RRDHOST_STATUS_ML       = (1 << 1),
    RRDHOST_STATUS_DYNCFG   = (1 << 2),
    RRDHOST_STATUS_HEALTH   = (1 << 3),
} RRDHOST_STATUS_INFO;

#define RRDHOST_STATUS_ALL (RRDHOST_STATUS_BASIC|RRDHOST_STATUS_STREAM|RRDHOST_STATUS_ML|RRDHOST_STATUS_DYNCFG|RRDHOST_STATUS_HEALTH)

typedef enum __attribute__((packed)) {
    RRDHOST_DB_STATUS_INITIALIZING = 0,
    RRDHOST_DB_STATUS_QUERYABLE,
} RRDHOST_DB_STATUS;

typedef enum __attribute__((packed)) {
    RRDHOST_DB_LIVENESS_STALE = 0,
    RRDHOST_DB_LIVENESS_LIVE,
} RRDHOST_DB_LIVENESS;

typedef enum __attribute__((packed)) {
    RRDHOST_INGEST_STATUS_ARCHIVED = 0,     // an old host in the database (never connected during this session)
    RRDHOST_INGEST_STATUS_INITIALIZING,     // contexts are still loading
    RRDHOST_INGEST_STATUS_REPLICATING,      // receiving replication
    RRDHOST_INGEST_STATUS_ONLINE,           // currently collecting data
    RRDHOST_INGEST_STATUS_OFFLINE,          // a disconnected node
} RRDHOST_INGEST_STATUS;

typedef enum __attribute__((packed)) {
    RRDHOST_INGEST_TYPE_LOCALHOST = 0,
    RRDHOST_INGEST_TYPE_VIRTUAL,
    RRDHOST_INGEST_TYPE_CHILD,
    RRDHOST_INGEST_TYPE_ARCHIVED,
} RRDHOST_INGEST_TYPE;

typedef enum __attribute__((packed)) {
    RRDHOST_STREAM_STATUS_DISABLED = 0,
    RRDHOST_STREAM_STATUS_REPLICATING,
    RRDHOST_STREAM_STATUS_ONLINE,
    RRDHOST_STREAM_STATUS_OFFLINE,
} RRDHOST_STREAMING_STATUS;

typedef enum __attribute__((packed)) {
    RRDHOST_ML_STATUS_DISABLED = 0,
    RRDHOST_ML_STATUS_OFFLINE,
    RRDHOST_ML_STATUS_RUNNING,
} RRDHOST_ML_STATUS;

typedef enum __attribute__((packed)) {
    RRDHOST_ML_TYPE_DISABLED = 0,
    RRDHOST_ML_TYPE_SELF,
    RRDHOST_ML_TYPE_RECEIVED,
} RRDHOST_ML_TYPE;

typedef enum __attribute__((packed)) {
    RRDHOST_HEALTH_STATUS_DISABLED = 0,
    RRDHOST_HEALTH_STATUS_INITIALIZING,
    RRDHOST_HEALTH_STATUS_RUNNING,
} RRDHOST_HEALTH_STATUS;

typedef enum __attribute__((packed)) {
    RRDHOST_DYNCFG_STATUS_UNAVAILABLE = 0,
    RRDHOST_DYNCFG_STATUS_AVAILABLE,
} RRDHOST_DYNCFG_STATUS;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_DB_STATUS);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_DB_LIVENESS);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_INGEST_STATUS);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_INGEST_TYPE);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_STREAMING_STATUS);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_ML_STATUS);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_ML_TYPE);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_HEALTH_STATUS);
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDHOST_DYNCFG_STATUS);

#define rrdhost_db_status_to_string(status) RRDHOST_DB_STATUS_2str(status)
#define rrdhost_db_liveness_to_string(status) RRDHOST_DB_LIVENESS_2str(status)
#define rrdhost_ingest_status_to_string(status) RRDHOST_INGEST_STATUS_2str(status)
#define rrdhost_ingest_type_to_string(type) RRDHOST_INGEST_TYPE_2str(type)
#define rrdhost_streaming_status_to_string(status) RRDHOST_STREAMING_STATUS_2str(status)
#define rrdhost_ml_status_to_string(status) RRDHOST_ML_STATUS_2str(status)
#define rrdhost_ml_type_to_string(type) RRDHOST_ML_TYPE_2str(type)
#define rrdhost_health_status_to_string(status) RRDHOST_HEALTH_STATUS_2str(status)
#define rrdhost_dyncfg_status_to_string(status) RRDHOST_DYNCFG_STATUS_2str(status)

#include "streaming/stream-handshake.h"
#include "streaming/stream-capabilities.h"
#include "rrd.h"

typedef struct rrdhost_status_t {
    RRDHOST *host;
    time_t now;

    struct {
        RRDHOST_DYNCFG_STATUS status;
    } dyncfg;

    struct {
        RRDHOST_DB_STATUS status;
        RRDHOST_DB_LIVENESS liveness;
        RRD_DB_MODE mode;
        time_t first_time_s;
        time_t last_time_s;
        size_t metrics;
        size_t instances;
        size_t contexts;
    } db;

    struct {
        RRDHOST_ML_STATUS status;
        RRDHOST_ML_TYPE type;
        struct ml_metrics_statistics metrics;
    } ml;

    struct {
        int16_t hops;
        RRDHOST_INGEST_TYPE  type;
        RRDHOST_INGEST_STATUS status;
        SOCKET_PEERS peers;
        bool ssl;
        STREAM_CAPABILITIES capabilities;
        uint32_t id;
        time_t since;
        STREAM_HANDSHAKE reason;

        struct {
            uint32_t metrics; // currently collected
            uint32_t instances; // currently collected
            uint32_t contexts; // currently collected
        } collected;

        struct {
            bool in_progress;
            NETDATA_DOUBLE completion;
            uint32_t instances;
        } replication;
    } ingest;

    struct {
        int16_t hops;
        RRDHOST_STREAMING_STATUS status;
        SOCKET_PEERS peers;
        bool ssl;
        bool compression;
        STREAM_CAPABILITIES capabilities;
        uint32_t id;
        time_t since;
        STREAM_HANDSHAKE reason;

        struct {
            bool in_progress;
            NETDATA_DOUBLE completion;
            size_t instances;
        } replication;

        size_t sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];
    } stream;

    struct {
        RRDHOST_HEALTH_STATUS status;
        struct {
            uint32_t undefined;
            uint32_t uninitialized;
            uint32_t clear;
            uint32_t warning;
            uint32_t critical;
        } alerts;
    } health;
} RRDHOST_STATUS;

void rrdhost_status(RRDHOST *host, time_t now, RRDHOST_STATUS *s, RRDHOST_STATUS_INFO info);
RRDHOST_INGEST_STATUS rrdhost_get_ingest_status(RRDHOST *host, time_t now);
RRDHOST_INGEST_STATUS rrdhost_ingestion_status(RRDHOST *host);
int16_t rrdhost_ingestion_hops(RRDHOST *host);

#endif //NETDATA_RRDHOST_STATUS_H
