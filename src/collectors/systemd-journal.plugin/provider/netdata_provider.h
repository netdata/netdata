#ifndef ND_SD_JOURNAL_PROVIDER_NETDATA_H
#define ND_SD_JOURNAL_PROVIDER_NETDATA_H

#ifdef __cplusplus
extern "C" {
#endif /* __cplusplus */

#include "config.h"

#if defined(HAVE_RUST_PROVIDER)
    #include "rust_provider.h"
#else
    #include <systemd/sd-journal.h>
#endif

#if defined(HAVE_RUST_PROVIDER)
    typedef struct RsdJournal NsdJournal;

    #define NSD_JOURNAL_FOREACH_DATA(j, data, l) RSD_JOURNAL_FOREACH_DATA(j, data, l)
    #define NSD_JOURNAL_FOREACH_UNIQUE(j, data, l) RSD_JOURNAL_FOREACH_UNIQUE(j, data, l)
    #define NSD_JOURNAL_FOREACH_FIELD(j, field) RSD_JOURNAL_FOREACH_FIELD(j, field)

    typedef struct RsdId128 NsdId128;

    #define NSD_ID128_NULL RSD_ID128_NULL
    #define NSD_ID128_STRING_MAX RSD_ID128_STRING_MAX
    #define NSD_ID128_UUID_STRING_MAX RSD_ID128_UUID_STRING_MAX
#else
    typedef struct sd_journal NsdJournal;
    typedef sd_id128_t NsdId128;

    #define NSD_ID128_NULL SD_ID128_NULL
    #define NSD_ID128_STRING_MAX SD_ID128_STRING_MAX
    #define NSD_ID128_UUID_STRING_MAX SD_ID128_UUID_STRING_MAX

    #define NSD_JOURNAL_FOREACH_DATA(j, data, l) SD_JOURNAL_FOREACH_DATA(j, data, l)
    #if defined(HAVE_SD_JOURNAL_RESTART_FIELDS)
        #define NSD_JOURNAL_FOREACH_UNIQUE(j, data, l) SD_JOURNAL_FOREACH_UNIQUE(j, data, l)
        #define NSD_JOURNAL_FOREACH_FIELD(j, field) SD_JOURNAL_FOREACH_FIELD(j, field)
    #endif
#endif

int32_t nsd_id128_from_string(const char *s, NsdId128 *ret);
int32_t nsd_id128_equal(NsdId128 a, NsdId128 b);

int nsd_journal_open_files(NsdJournal **ret, const char *const *paths, int flags);
void nsd_journal_close(NsdJournal *j);

int nsd_journal_seek_head(NsdJournal *j);
int nsd_journal_seek_tail(NsdJournal *j);
int nsd_journal_seek_realtime_usec(NsdJournal *j, uint64_t usec);

int nsd_journal_next(NsdJournal *j);
int nsd_journal_previous(NsdJournal *j);

#if defined(HAVE_SD_JOURNAL_GET_SEQNUM)
int nsd_journal_get_seqnum(NsdJournal *j, uint64_t *ret_seqnum, NsdId128 *ret_seqnum_id);
#endif
int nsd_journal_get_realtime_usec(NsdJournal *j, uint64_t *ret);

#if defined(HAVE_SD_JOURNAL_RESTART_FIELDS)
int nsd_journal_enumerate_fields(NsdJournal *j, const char **field);
void nsd_journal_restart_fields(NsdJournal *j);

int nsd_journal_query_unique(NsdJournal *j, const char *field);
void nsd_journal_restart_unique(NsdJournal *j);
#endif /* HAVE_SD_JOURNAL_RESTART_FIELDS */

int nsd_journal_add_match(NsdJournal *j, const void *data, uintptr_t size);
int nsd_journal_add_conjunction(NsdJournal *j);
int nsd_journal_add_disjunction(NsdJournal *j);
void nsd_journal_flush_matches(NsdJournal *j);

#ifdef __cplusplus
}
#endif /* __cplusplus */

#endif /* ND_SD_JOURNAL_PROVIDER_NETDATA_H */
