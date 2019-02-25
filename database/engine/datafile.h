// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DATAFILE_H
#define NETDATA_DATAFILE_H

#include "rrdengine.h"

#define DATAFILE "/tmp/datafile.ndf"

/* only one event loop is supported for now */
struct rrdengine_datafile {
    uv_file file;
    uint64_t pos;
};

extern struct rrdengine_datafile datafile;

extern int init_data_files(void);

#endif /* NETDATA_DATAFILE_H */