// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JOURNALFILE_H
#define NETDATA_JOURNALFILE_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdengine_instance;
struct rrdengine_worker_config;
struct rrdengine_datafile;
struct rrdengine_journalfile;

#define WALFILE_PREFIX "journalfile-"
#define WALFILE_EXTENSION ".njf"


/* only one event loop is supported for now */
struct rrdengine_journalfile {
    uv_file file;
    uint64_t pos;
    void *data;
    struct rrdengine_datafile *datafile;
};

/* only one event loop is supported for now */
struct transaction_commit_log {
    uint64_t transaction_id;

    /* outstanding transaction buffer */
    void *buf;
    unsigned buf_pos;
    unsigned buf_size;
};

void generate_journalfilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
void journalfile_init(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
void *wal_get_transaction_buffer(struct rrdengine_worker_config* wc, unsigned size);
void wal_flush_transaction_buffer(struct rrdengine_worker_config* wc);
int close_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int unlink_journal_file(struct rrdengine_journalfile *journalfile);
int destroy_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int create_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int load_journal_file(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                             struct rrdengine_datafile *datafile);
void init_commit_log(struct rrdengine_instance *ctx);


#endif /* NETDATA_JOURNALFILE_H */