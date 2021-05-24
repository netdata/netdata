// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_H
#define NETDATA_SQLITE_ACLK_H

#include "sqlite3.h"

//#include "../../aclk/aclk.h"
#include "../../aclk/schema-wrappers/chart_stream.h"

static inline void uuid_unparse_lower_fix(uuid_t *uuid, char *out)
{
    uuid_unparse_lower(*uuid, out);
    out[8] = '_';
    out[13] = '_';
    out[18] = '_';
    out[23] = '_';
}

static inline char *get_str_from_uuid(uuid_t *uuid)
{
    char uuid_str[GUID_LEN + 1];
    if (unlikely(!uuid)) {
        uuid_t zero_uuid;
        uuid_clear(zero_uuid);
        uuid_unparse_lower(zero_uuid, uuid_str);
    }
    else
        uuid_unparse_lower(*uuid, uuid_str);
    return strdupz(uuid_str);
}

#define TABLE_ACLK_CHART "create table if not exists aclk_chart_%s (sequence_id integer primary key, " \
        "date_created, date_updated, date_submitted, status, chart_id, unique_id, " \
        "update_count default 1, unique(chart_id, status));"

#define TABLE_ACLK_CHART_PAYLOAD "create table if not exists aclk_chart_payload_%s (unique_id blob primary key, " \
        "chart_id, type, date_created, payload);"

#define TRIGGER_ACLK_CHART_PAYLOAD "create trigger if not exists aclk_tr_chart_payload_%s " \
        "after insert on aclk_chart_payload_%s " \
        "begin insert into aclk_chart_%s (chart_id, unique_id, status, date_created) values " \
        " (new.chart_id, new.unique_id, 'pending', strftime('%%s')) on conflict(chart_id, status) " \
        " do update set unique_id = new.unique_id, update_count = update_count + 1; " \
        "end;"

#define TABLE_ACLK_DIMENSION "create table if not exists aclk_dimension_%s (sequence_id integer primary key, " \
        "date_created, date_updated, date_submitted, status, dim_id, unique_id, " \
        "update_count default 1, unique(dim_id, status));"

#define TABLE_ACLK_DIMENSION_PAYLOAD "create table if not exists aclk_dimension_payload_%s (unique_id blob primary key, " \
        "dim_id, type, date_created, payload);"

#define TRIGGER_ACLK_DIMENSION_PAYLOAD "create trigger if not exists aclk_tr_dimension_payload_%s " \
        "after insert on aclk_dimension_payload_%s " \
        "begin insert into aclk_dimension_%s (dim_id, unique_id, status, date_created) values " \
        " (new.dim_id, new.unique_id, 'pending', strftime('%%s')) on conflict(dim_id, status) " \
        " do update set unique_id = new.unique_id, update_count = update_count + 1; " \
        "end;"

#define TABLE_ACLK_ALERT "create table if not exists aclk_alert_%s (sequence_id integer primary key, " \
                 "date_created, date_updated, date_submitted, status, alarm_id, unique_id, " \
                 "update_count default 1, unique(alarm_id, status));"

#define TABLE_ACLK_ALERT_PAYLOAD "create table if not exists aclk_alert_payload_%s (unique_id blob primary key, " \
                 "ae_unique_id, alarm_id, type, date_created, payload);"

#define TRIGGER_ACLK_ALERT_PAYLOAD "create trigger if not exists aclk_tr_alert_payload_%s " \
        "after insert on aclk_alert_payload_%s " \
        "begin insert into aclk_alert_%s (alarm_id, unique_id, status, date_created) values " \
        " (new.alarm_id, new.unique_id, 'pending', strftime('%%s')) on conflict(alarm_id, status) " \
        " do update set unique_id = new.unique_id, update_count = update_count + 1; " \
        "end;"


enum aclk_database_opcode {
    /* can be used to return empty status or flush the command queue */
    ACLK_DATABASE_NOOP = 0,
    ACLK_DATABASE_CLEANUP,
    ACLK_DATABASE_TIMER,
    ACLK_DATABASE_ADD_CHART,
    ACLK_DATABASE_ADD_DIMENSION,
    ACLK_DATABASE_FETCH_CHART,
    ACLK_DATABASE_PUSH_CHART,
    ACLK_DATABASE_PUSH_CHART_CONFIG,
    ACLK_DATABASE_CHART_ACK,
    ACLK_DATABASE_ADD_CHART_CONFIG,
    ACLK_DATABASE_PUSH_DIMENSION,
    ACLK_DATABASE_RESET_CHART,
    ACLK_DATABASE_STATUS_CHART,
    ACLK_DATABASE_RESET_NODE,
    ACLK_DATABASE_UPD_CHART,
    ACLK_DATABASE_UPD_ALERT,
    ACLK_DATABASE_SHUTDOWN,
    ACLK_DATABASE_ADD_ALARM,
    ACLK_DATABASE_MAX_OPCODE
};

struct aclk_chart_payload_t {
    long sequence_id;
    long last_sequence_id;
    char *payload;
    struct aclk_chart_payload_t *next;
};


struct aclk_database_cmd {
    enum aclk_database_opcode opcode;
    void *data;
    void *data1;
    void *data_param;
    int count;
    uint64_t param1;
    union {
//        struct rrdeng_read_page {
//            struct rrdeng_page_descr *page_cache_descr;
//        } read_page;
//        struct rrdeng_read_extent {
//            struct rrdeng_page_descr *page_cache_descr[MAX_PAGES_PER_EXTENT];
//            int page_count;
//        } read_extent;
        struct completion *completion;
    };
};

#define ACLK_DATABASE_CMD_Q_MAX_SIZE (2048)

struct aclk_database_cmdqueue {
    unsigned head, tail;
    struct aclk_database_cmd cmd_array[ACLK_DATABASE_CMD_Q_MAX_SIZE];
};

struct aclk_database_worker_config {
    uv_thread_t thread;
    char uuid_str[GUID_LEN + 1];
    uv_loop_t *loop;
    RRDHOST *host;
    uv_async_t async;
    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct aclk_database_cmdqueue cmd_queue;
    int error;
    int chart_updates;
    int alert_updates;
};

//extern void sqlite_worker(void* arg);
extern void aclk_database_enq_cmd(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd);

extern void sql_queue_chart_to_aclk(RRDSET *st, int cmd);
void sql_queue_dimension_to_aclk(RRDHOST *host, RRDDIM *rd);
extern void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae);
extern sqlite3 *db_meta;
extern void sql_create_aclk_table(RRDHOST *host);
extern void sql_create_aclk_table(RRDHOST *host);
int aclk_add_chart_event(RRDSET *st, char *payload_type, struct completion *completion);
int aclk_push_chart_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_add_dimension_event(RRDDIM *st, char *payload_type, struct completion *completion);
int aclk_add_alarm_event(RRDHOST *host, ALARM_ENTRY *ae, char *payload_type, struct completion *completion);
void aclk_fetch_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_status_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_reset_node_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
//void aclk_fetch_chart_event_proto(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_drop_host_aclk_table_list(uuid_t *host_uuid);
void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id);
void aclk_get_chart_config(char **hash_id_list);
void aclk_start_streaming(char *node_id);
void sql_aclk_drop_all_table_list();
void sql_set_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
extern void aclk_set_architecture(int mode);
#endif //NETDATA_SQLITE_ACLK_H
