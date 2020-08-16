// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMPACTION_H
#define NETDATA_COMPACTION_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include "../rrdengine.h"

struct logfile_compaction_state {
    unsigned fileno; /* Starts at 1 */
    unsigned starting_fileno; /* 0 for normal files, staring number during compaction */

    struct metadata_record_commit_log records_log;
    struct metadata_logfile_list new_metadata_logfiles;
    struct metadata_logfile *last_original_logfile; /* Marks the end of compaction */
    uint8_t throttle; /* set non-zero to throttle compaction */
};

extern int compaction_failure_recovery(struct metalog_instance *ctx, struct metadata_logfile **metalogfiles,
                                       unsigned *matched_files);
extern void metalog_do_compaction(struct metalog_worker_config *wc);
extern void after_compact_old_records(struct metalog_worker_config* wc);

#endif /* NETDATA_COMPACTION_H */
