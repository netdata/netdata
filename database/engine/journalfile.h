// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JOURNALFILE_H
#define NETDATA_JOURNALFILE_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdengine_worker_config;

#define WALFILE "/tmp/journal.njf"

/* only one event loop is supported for now */
struct rrdengine_journalfile {
    uv_file file;
    uint64_t pos;
};

/* only one event loop is supported for now */
struct transaction_commit_log {
    uint64_t transaction_id;

    /* outstanding transaction buffer */
    void *buf;
    unsigned buf_pos;
    unsigned buf_size;
};

extern struct transaction_commit_log commit_log;
extern struct rrdengine_journalfile journalfile;

extern int init_journal_files(uint64_t data_file_size);
extern void *wal_get_transaction_buffer(struct rrdengine_worker_config* wc, unsigned size);
extern void wal_flush_transaction_buffer(struct rrdengine_worker_config* wc);

#endif /* NETDATA_JOURNALFILE_H */