#ifndef ND_SD_JOURNAL_PROVIDER_NETDATA_H
#define ND_SD_JOURNAL_PROVIDER_NETDATA_H

#include "crates/jf/journal_reader_ffi/journal_reader_ffi.h"

#define SD_ID128_NULL ((const sd_id128_t){.bytes = {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}})
#define SD_ID128_STRING_MAX 33U
#define SD_ID128_UUID_STRING_MAX 37U

#define SD_JOURNAL_FOREACH_DATA(j, data, l)                                                                            \
    for (sd_journal_restart_data(j); sd_journal_enumerate_available_data((j), &(data), &(l)) > 0;)

#define SD_JOURNAL_FOREACH_UNIQUE(j, data, l)                                                                          \
    for (sd_journal_restart_unique(j); sd_journal_enumerate_available_unique((j), &(data), &(l)) > 0;)

#define SD_JOURNAL_FOREACH_FIELD(j, field)                                                                             \
    for (sd_journal_restart_fields(j); sd_journal_enumerate_fields((j), &(field)) > 0;)

#undef HAVE_SD_JOURNAL_RESTART_FIELDS
#define HAVE_SD_JOURNAL_RESTART_FIELDS 1

#undef HAVE_SD_JOURNAL_GET_SEQNUM
#define HAVE_SD_JOURNAL_GET_SEQNUM 1

#endif /* ND_SD_JOURNAL_PROVIDER_NETDATA_H */
