// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMPACTION_H
#define NETDATA_COMPACTION_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include "../rrdengine.h"

extern int compaction_failure_recovery(struct metalog_instance *ctx, struct metadata_logfile **metalogfiles,
                                       unsigned *matched_files);

#endif /* NETDATA_COMPACTION_H */
