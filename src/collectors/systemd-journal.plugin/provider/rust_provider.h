#ifndef ND_SD_JOURNAL_PROVIDER_RUST_H
#define ND_SD_JOURNAL_PROVIDER_RUST_H

#include "crates/jf/journal_reader_ffi/journal_reader_ffi.h"

#define RSD_ID128_NULL ((const RsdId128){.bytes = {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}})
#define RSD_ID128_STRING_MAX 33U
#define RSD_ID128_UUID_STRING_MAX 37U

#define RSD_JOURNAL_FOREACH_DATA(j, data, l)                                                                            \
    for (rsd_journal_restart_data(j); rsd_journal_enumerate_available_data((j), &(data), &(l)) > 0;)

#define RSD_JOURNAL_FOREACH_UNIQUE(j, data, l)                                                                          \
    for (rsd_journal_restart_unique(j); rsd_journal_enumerate_available_unique((j), &(data), &(l)) > 0;)

#define RSD_JOURNAL_FOREACH_FIELD(j, field)                                                                             \
    for (rsd_journal_restart_fields(j); rsd_journal_enumerate_fields((j), &(field)) > 0;)

#ifndef HAVE_SD_JOURNAL_RESTART_FIELDS
#define HAVE_SD_JOURNAL_RESTART_FIELDS 1
#endif

#ifndef HAVE_SD_JOURNAL_GET_SEQNUM
#define HAVE_SD_JOURNAL_GET_SEQNUM 1
#endif

#endif /* ND_SD_JOURNAL_PROVIDER_RUST_H */
