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

extern void generate_journalfilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
extern void journalfile_init(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
extern void *wal_get_transaction_buffer(struct rrdengine_worker_config* wc, unsigned size);
extern void wal_flush_transaction_buffer(struct rrdengine_worker_config* wc);
extern int close_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
extern int destroy_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
extern int create_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
extern int load_journal_file(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                             struct rrdengine_datafile *datafile);
extern void init_commit_log(struct rrdengine_instance *ctx);


#endif /* NETDATA_JOURNALFILE_H */