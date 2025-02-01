// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-replication-receiver.h"
#include "stream-receiver-internals.h"

struct replication_request_details {
    struct {
        send_command callback;
        struct parser *parser;
    } caller;

    RRDHOST *host;
    RRDSET *st;

    struct {
        time_t first_entry_t;               // the first entry time the child has
        time_t last_entry_t;                // the last entry time the child has
        time_t wall_clock_time;             // the current time of the child
        bool fixed_last_entry;              // when set we set the last entry to wall clock time
    } child_db;

    struct {
        time_t first_entry_t;               // the first entry time we have
        time_t last_entry_t;                // the last entry time we have
        time_t wall_clock_time;                         // the current local world clock time
    } local_db;

    struct {
        time_t from;                        // the starting time of the entire gap we have
        time_t to;                          // the ending time of the entire gap we have
    } gap;

    struct {
        time_t after;                       // the start time we requested previously from this child
        time_t before;                      // the end time we requested previously from this child
    } last_request;

    struct {
        time_t after;                       // the start time of this replication request - the child will add 1 second
        time_t before;                      // the end time of this replication request
        bool start_streaming;               // true when we want the child to send anything remaining and start streaming - the child will overwrite 'before'
    } wanted;
};

static void replicate_log_request(struct replication_request_details *r, const char *msg) {
#ifdef NETDATA_INTERNAL_CHECKS
    internal_error(true,
#else
    nd_log_limit_static_global_var(erl, 1, 0);
    nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
#endif
                   "STREAM SND REPLAY ERROR: 'host:%s/chart:%s' child sent: "
                   "db from %ld to %ld%s, wall clock time %ld, "
                   "last request from %ld to %ld, "
                   "issue: %s - "
                   "sending replication request from %ld to %ld, start streaming %s",
                   rrdhost_hostname(r->st->rrdhost), rrdset_id(r->st),
                   r->child_db.first_entry_t,
                   r->child_db.last_entry_t, r->child_db.fixed_last_entry ? " (fixed)" : "",
                   r->child_db.wall_clock_time,
                   r->last_request.after,
                   r->last_request.before,
                   msg,
                   r->wanted.after,
                   r->wanted.before,
                   r->wanted.start_streaming ? "true" : "false");
}

static bool send_replay_chart_cmd(struct replication_request_details *r, const char *msg, bool log) {
    RRDSET *st = r->st;

    if(log)
        replicate_log_request(r, msg);

    if(st->rrdhost->receiver && (!st->rrdhost->receiver->replication.first_time_s || r->wanted.after < st->rrdhost->receiver->replication.first_time_s))
        st->rrdhost->receiver->replication.first_time_s = r->wanted.after;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    st->replay.log_next_data_collection = true;

    char wanted_after_buf[LOG_DATE_LENGTH + 1] = "", wanted_before_buf[LOG_DATE_LENGTH + 1] = "";

    if(r->wanted.after)
        log_date(wanted_after_buf, LOG_DATE_LENGTH, r->wanted.after);

    if(r->wanted.before)
        log_date(wanted_before_buf, LOG_DATE_LENGTH, r->wanted.before);

    internal_error(true,
                   "STREAM SND REPLAY: 'host:%s/chart:%s' sending replication request %ld [%s] to %ld [%s], start streaming '%s': %s: "
                   "last[%ld - %ld] child[%ld - %ld, now %ld %s] local[%ld - %ld, now %ld] gap[%ld - %ld %s] %s"
                   , rrdhost_hostname(r->host), rrdset_id(r->st)
                       , r->wanted.after, wanted_after_buf
                   , r->wanted.before, wanted_before_buf
                   , r->wanted.start_streaming ? "YES" : "NO"
                   , msg
                   , r->last_request.after, r->last_request.before
                   , r->child_db.first_entry_t, r->child_db.last_entry_t
                   , r->child_db.wall_clock_time, (r->child_db.wall_clock_time == r->local_db.wall_clock_time) ? "SAME" : (r->child_db.wall_clock_time < r->local_db.wall_clock_time) ? "BEHIND" : "AHEAD"
                   , r->local_db.first_entry_t, r->local_db.last_entry_t
                   , r->local_db.wall_clock_time
                   , r->gap.from, r->gap.to
                   , (r->gap.from == r->wanted.after) ? "FULL" : "PARTIAL"
                   , (st->replay.after != 0 || st->replay.before != 0) ? "OVERLAPPING" : ""
    );

    st->replay.start_streaming = r->wanted.start_streaming;
    st->replay.after = r->wanted.after;
    st->replay.before = r->wanted.before;
#endif // NETDATA_LOG_REPLICATION_REQUESTS

    char buffer[2048 + 1];
    snprintfz(buffer, sizeof(buffer) - 1, PLUGINSD_KEYWORD_REPLAY_CHART " \"%s\" \"%s\" %llu %llu\n",
              rrdset_id(st), r->wanted.start_streaming ? "true" : "false",
              (unsigned long long)r->wanted.after, (unsigned long long)r->wanted.before);

    ssize_t ret = r->caller.callback(buffer, r->caller.parser, STREAM_TRAFFIC_TYPE_REPLICATION);
    if (ret < 0) {
        netdata_log_error("STREAM SND REPLAY ERROR: 'host:%s/chart:%s' failed to send replication request to child (error %zd)",
                          rrdhost_hostname(r->host), rrdset_id(r->st), ret);
        return false;
    }

    __atomic_add_fetch(&st->rrdhost->stream.rcv.status.replication.counter_out, 1, __ATOMIC_RELAXED);

#ifdef REPLICATION_TRACKING
    st->stream.rcv.who = REPLAY_WHO_THEM;
#endif

    return true;
}

bool replicate_chart_request(send_command callback, struct parser *parser, RRDHOST *host, RRDSET *st,
                             time_t child_first_entry, time_t child_last_entry, time_t child_wall_clock_time,
                             time_t prev_first_entry_wanted, time_t prev_last_entry_wanted)
{
    struct replication_request_details r = {
        .caller = {
            .callback = callback,
            .parser = parser,
        },

        .host = host,
        .st = st,

        .child_db = {
            .first_entry_t = child_first_entry,
            .last_entry_t = child_last_entry,
            .wall_clock_time = child_wall_clock_time,
            .fixed_last_entry = false,
        },

        .local_db = {
            .first_entry_t = 0,
            .last_entry_t = 0,
            .wall_clock_time  = now_realtime_sec(),
        },

        .last_request = {
            .after = prev_first_entry_wanted,
            .before = prev_last_entry_wanted,
        },

        .wanted = {
            .after = 0,
            .before = 0,
            .start_streaming = true,
        },
    };

    if(r.child_db.last_entry_t > r.child_db.wall_clock_time) {
        replicate_log_request(&r, "child's db last entry > child's wall clock time");
        r.child_db.last_entry_t = r.child_db.wall_clock_time;
        r.child_db.fixed_last_entry = true;
    }

    rrdset_get_retention_of_tier_for_collected_chart(r.st, &r.local_db.first_entry_t, &r.local_db.last_entry_t, r.local_db.wall_clock_time, 0);

    // let's find the GAP we have
    if(!r.last_request.after || !r.last_request.before) {
        // there is no previous request

        if(r.local_db.last_entry_t)
            // we have some data, let's continue from the last point we have
            r.gap.from = r.local_db.last_entry_t;
        else
            // we don't have any data, the gap is the max timeframe we are allowed to replicate
            r.gap.from = r.local_db.wall_clock_time - r.host->stream.replication.period;

    }
    else {
        // we had sent a request - let's continue at the point we left it
        // for this we don't take into account the actual data in our db
        // because the child may also have gaps, and we need to get over it
        r.gap.from = r.last_request.before;
    }

    // we want all the data up to now
    r.gap.to = r.local_db.wall_clock_time;

    // The gap is now r.gap.from -> r.gap.to

    if (unlikely(!rrdhost_option_check(host, RRDHOST_OPTION_REPLICATION)))
        return send_replay_chart_cmd(&r, "sending empty replication request, replication is disabled", false);

    if (unlikely(!rrdset_number_of_dimensions(st)))
        return send_replay_chart_cmd(&r, "sending empty replication request, chart has no dimensions", false);

    if (unlikely(!r.child_db.first_entry_t || !r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "sending empty replication request, child has no stored data", false);

    if (unlikely(r.child_db.first_entry_t < 0 || r.child_db.last_entry_t < 0))
        return send_replay_chart_cmd(&r, "sending empty replication request, child db timestamps are invalid", true);

    if (unlikely(r.child_db.first_entry_t > r.child_db.wall_clock_time))
        return send_replay_chart_cmd(&r, "sending empty replication request, child db first entry is after its wall clock time", true);

    if (unlikely(r.child_db.first_entry_t > r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "sending empty replication request, child timings are invalid (first entry > last entry)", true);

    if (unlikely(r.local_db.last_entry_t > r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "sending empty replication request, local last entry is later than the child one", false);

    // let's find what the child can provide to fill that gap

    if(r.child_db.first_entry_t > r.gap.from)
        // the child does not have all the data - let's get what it has
        r.wanted.after = r.child_db.first_entry_t;
    else
        // ok, the child can fill the entire gap we have
        r.wanted.after = r.gap.from;

    if(r.gap.to - r.wanted.after > host->stream.replication.step)
        // the duration is too big for one request - let's take the first step
        r.wanted.before = r.wanted.after + host->stream.replication.step;
    else
        // wow, we can do it in one request
        r.wanted.before = r.gap.to;

    // don't ask from the child more than it has
    if(r.wanted.before > r.child_db.last_entry_t)
        r.wanted.before = r.child_db.last_entry_t;

    if(r.wanted.after > r.wanted.before) {
        r.wanted.after = 0;
        r.wanted.before = 0;
        r.wanted.start_streaming = true;
        return send_replay_chart_cmd(&r, "sending empty replication request, because wanted 'after' computed bigger than wanted 'before'", true);
    }

    // the child should start streaming immediately if the wanted duration is small, or we reached the last entry of the child
    r.wanted.start_streaming = (r.local_db.wall_clock_time - r.wanted.after <= host->stream.replication.step ||
                                r.wanted.before >= r.child_db.last_entry_t ||
                                r.wanted.before >= r.child_db.wall_clock_time ||
                                r.wanted.before >= r.local_db.wall_clock_time);

    // the wanted timeframe is now r.wanted.after -> r.wanted.before
    // send it
    return send_replay_chart_cmd(&r, "OK", false);
}

ALWAYS_INLINE bool stream_parse_enable_streaming(const char *start_streaming_txt) {
    bool start_streaming;

    if(unlikely(!start_streaming_txt || !*start_streaming_txt)) {
        start_streaming = false;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "REPLAY: malformed start_streaming boolean value empty");
    }
    else if(likely(strcmp(start_streaming_txt, "false") == 0))
        start_streaming = false;
    else if(likely(strcmp(start_streaming_txt, "true") == 0))
        start_streaming = true;
    else {
        start_streaming = false;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "REPLAY: malformed start_streaming boolean value '%s'", start_streaming_txt);
    }

    return start_streaming;
}
