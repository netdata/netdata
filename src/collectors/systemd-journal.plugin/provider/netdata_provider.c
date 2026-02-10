#include "netdata_provider.h"

int32_t nsd_id128_from_string(const char *s, NsdId128 *ret)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_id128_from_string(s, (struct RsdId128 *) ret);
#else
    return sd_id128_from_string(s, (sd_id128_t *) ret);
#endif
}

int32_t nsd_id128_equal(NsdId128 a, NsdId128 b)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_id128_equal(a, b);
#else
    return sd_id128_equal(a, b);
#endif
}

int nsd_journal_open_files(NsdJournal **ret, const char *const *paths, int flags)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_open_files(ret, paths, flags);
#else
    return sd_journal_open_files(ret, paths, flags);
#endif
}

void nsd_journal_close(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    rsd_journal_close(j);
#else
    sd_journal_close(j);
#endif
}

int nsd_journal_seek_head(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_seek_head(j);
#else
    return sd_journal_seek_head(j);
#endif
}

int nsd_journal_seek_tail(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_seek_tail(j);
#else
    return sd_journal_seek_tail(j);
#endif
}

int nsd_journal_seek_realtime_usec(NsdJournal *j, uint64_t usec)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_seek_realtime_usec(j, usec);
#else
    return sd_journal_seek_realtime_usec(j, usec);
#endif
}

int nsd_journal_next(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_next(j);
#else
    return sd_journal_next(j);
#endif
}

int nsd_journal_previous(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_previous(j);
#else
    return sd_journal_previous(j);
#endif
}

#if defined(HAVE_SD_JOURNAL_GET_SEQNUM)
int nsd_journal_get_seqnum(NsdJournal *j, uint64_t *ret_seqnum, NsdId128 *ret_seqnum_id)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_get_seqnum(j, ret_seqnum, ret_seqnum_id);
#else
    return sd_journal_get_seqnum(j, ret_seqnum, ret_seqnum_id);
#endif
}
#endif /* HAVE_SD_JOURNAL_GET_SEQNUM */

int nsd_journal_get_realtime_usec(NsdJournal *j, uint64_t *ret)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_get_realtime_usec(j, ret);
#else
    return sd_journal_get_realtime_usec(j, ret);
#endif
}

#if defined(HAVE_SD_JOURNAL_RESTART_FIELDS)
int nsd_journal_enumerate_fields(NsdJournal *j, const char **field)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_enumerate_fields(j, field);
#else
    return sd_journal_enumerate_fields(j, field);
#endif
}
#endif /* HAVE_SD_JOURNAL_RESTART_FIELDS */

#if defined(HAVE_SD_JOURNAL_RESTART_FIELDS)
void nsd_journal_restart_fields(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    rsd_journal_restart_fields(j);
#else
    sd_journal_restart_fields(j);
#endif
}
#endif /* HAVE_SD_JOURNAL_RESTART_FIELDS */

#if defined(HAVE_SD_JOURNAL_RESTART_FIELDS)
int nsd_journal_query_unique(NsdJournal *j, const char *field)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_query_unique(j, field);
#else
    return sd_journal_query_unique(j, field);
#endif
}
#endif /* HAVE_SD_JOURNAL_RESTART_FIELDS */

#if defined(HAVE_SD_JOURNAL_RESTART_FIELDS)
void nsd_journal_restart_unique(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    rsd_journal_restart_unique(j);
#else
    sd_journal_restart_unique(j);
#endif
}
#endif /* HAVE_SD_JOURNAL_RESTART_FIELDS */

int nsd_journal_add_match(NsdJournal *j, const void *data, uintptr_t size)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_add_match(j, data, size);
#else
    return sd_journal_add_match(j, data, size);
#endif
}

int nsd_journal_add_conjunction(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_add_conjunction(j);
#else
    return sd_journal_add_conjunction(j);
#endif
}

int nsd_journal_add_disjunction(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    return rsd_journal_add_disjunction(j);
#else
    return sd_journal_add_disjunction(j);
#endif
}

void nsd_journal_flush_matches(NsdJournal *j)
{
#if defined(HAVE_RUST_PROVIDER)
    rsd_journal_flush_matches(j);
#else
    sd_journal_flush_matches(j);
#endif
}
