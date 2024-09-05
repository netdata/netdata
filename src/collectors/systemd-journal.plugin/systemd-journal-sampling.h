// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEMD_JOURNAL_SAMPLING_H
#define NETDATA_SYSTEMD_JOURNAL_SAMPLING_H

// ----------------------------------------------------------------------------
// sampling support

static inline void sampling_query_init(LOGS_QUERY_STATUS *lqs, FACETS *facets) {
    if(!lqs->rq.sampling)
        return;

    if(!lqs->rq.slice) {
        // the user is doing a full data query
        // disable sampling
        lqs->rq.sampling = 0;
        return;
    }

    if(lqs->rq.data_only) {
        // the user is doing a data query
        // disable sampling
        lqs->rq.sampling = 0;
        return;
    }

    if(!lqs->c.files_matched) {
        // no files have been matched
        // disable sampling
        lqs->rq.sampling = 0;
        return;
    }

    lqs->c.samples.slots = facets_histogram_slots(facets);
    if(lqs->c.samples.slots < 2)
        lqs->c.samples.slots = 2;
    if(lqs->c.samples.slots > SYSTEMD_JOURNAL_SAMPLING_SLOTS)
        lqs->c.samples.slots = SYSTEMD_JOURNAL_SAMPLING_SLOTS;

    if(!lqs->rq.after_ut || !lqs->rq.before_ut || lqs->rq.after_ut >= lqs->rq.before_ut) {
        // we don't have enough information for sampling
        lqs->rq.sampling = 0;
        return;
    }

    usec_t delta = lqs->rq.before_ut - lqs->rq.after_ut;
    usec_t step = delta / facets_histogram_slots(facets) - 1;
    if(step < 1) step = 1;

    lqs->c.samples_per_time_slot.start_ut = lqs->rq.after_ut;
    lqs->c.samples_per_time_slot.end_ut = lqs->rq.before_ut;
    lqs->c.samples_per_time_slot.step_ut = step;

    // the minimum number of rows to enable sampling
    lqs->c.samples.enable_after_samples = lqs->rq.sampling / 2;

    size_t files_matched = lqs->c.files_matched;
    if(!files_matched)
        files_matched = 1;

    // the minimum number of rows per file to enable sampling
    lqs->c.samples_per_file.enable_after_samples = (lqs->rq.sampling / 4) / files_matched;
    if(lqs->c.samples_per_file.enable_after_samples < lqs->rq.entries)
        lqs->c.samples_per_file.enable_after_samples = lqs->rq.entries;

    // the minimum number of rows per time slot to enable sampling
    lqs->c.samples_per_time_slot.enable_after_samples = (lqs->rq.sampling / 4) / lqs->c.samples.slots;
    if(lqs->c.samples_per_time_slot.enable_after_samples < lqs->rq.entries)
        lqs->c.samples_per_time_slot.enable_after_samples = lqs->rq.entries;
}

static inline void sampling_file_init(LOGS_QUERY_STATUS *lqs, struct journal_file *jf __maybe_unused) {
    lqs->c.samples_per_file.sampled = 0;
    lqs->c.samples_per_file.unsampled = 0;
    lqs->c.samples_per_file.estimated = 0;
    lqs->c.samples_per_file.every = 0;
    lqs->c.samples_per_file.skipped = 0;
    lqs->c.samples_per_file.recalibrate = 0;
}

static inline size_t sampling_file_lines_scanned_so_far(LOGS_QUERY_STATUS *lqs) {
    size_t sampled = lqs->c.samples_per_file.sampled + lqs->c.samples_per_file.unsampled;
    if(!sampled) sampled = 1;
    return sampled;
}

static inline void sampling_running_file_query_overlapping_timeframe_ut(
    LOGS_QUERY_STATUS *lqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction,
    usec_t msg_ut, usec_t *after_ut, usec_t *before_ut) {

    // find the overlap of the query and file timeframes
    // taking into account the first message we encountered

    usec_t oldest_ut, newest_ut;
    if(direction == FACETS_ANCHOR_DIRECTION_FORWARD) {
        // the first message we know (oldest)
        oldest_ut = lqs->c.query_file.first_msg_ut ? lqs->c.query_file.first_msg_ut : jf->msg_first_ut;
        if(!oldest_ut) oldest_ut = lqs->c.query_file.start_ut;

        if(jf->msg_last_ut)
            newest_ut = MIN(lqs->c.query_file.stop_ut, jf->msg_last_ut);
        else if(jf->file_last_modified_ut)
            newest_ut = MIN(lqs->c.query_file.stop_ut, jf->file_last_modified_ut);
        else
            newest_ut = lqs->c.query_file.stop_ut;

        if(msg_ut < oldest_ut)
            oldest_ut = msg_ut - 1;
    }
    else /* BACKWARD */ {
        // the latest message we know (newest)
        newest_ut = lqs->c.query_file.first_msg_ut ? lqs->c.query_file.first_msg_ut : jf->msg_last_ut;
        if(!newest_ut) newest_ut = lqs->c.query_file.start_ut;

        if(jf->msg_first_ut)
            oldest_ut = MAX(lqs->c.query_file.stop_ut, jf->msg_first_ut);
        else
            oldest_ut = lqs->c.query_file.stop_ut;

        if(newest_ut < msg_ut)
            newest_ut = msg_ut + 1;
    }

    *after_ut = oldest_ut;
    *before_ut = newest_ut;
}

static inline double sampling_running_file_query_progress_by_time(
    LOGS_QUERY_STATUS *lqs, struct journal_file *jf,
    FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {

    usec_t after_ut, before_ut, elapsed_ut;
    sampling_running_file_query_overlapping_timeframe_ut(lqs, jf, direction, msg_ut, &after_ut, &before_ut);

    if(direction == FACETS_ANCHOR_DIRECTION_FORWARD)
        elapsed_ut = msg_ut - after_ut;
    else
        elapsed_ut = before_ut - msg_ut;

    usec_t total_ut = before_ut - after_ut;
    double progress = (double)elapsed_ut / (double)total_ut;

    return progress;
}

static inline usec_t sampling_running_file_query_remaining_time(
    LOGS_QUERY_STATUS *lqs, struct journal_file *jf,
    FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut,
    usec_t *total_time_ut, usec_t *remaining_start_ut,
    usec_t *remaining_end_ut) {
    usec_t after_ut, before_ut;
    sampling_running_file_query_overlapping_timeframe_ut(lqs, jf, direction, msg_ut, &after_ut, &before_ut);

    // since we have a timestamp in msg_ut
    // this timestamp can extend the overlap
    if(msg_ut <= after_ut)
        after_ut = msg_ut - 1;

    if(msg_ut >= before_ut)
        before_ut = msg_ut + 1;

    // return the remaining duration
    usec_t remaining_from_ut, remaining_to_ut;
    if(direction == FACETS_ANCHOR_DIRECTION_FORWARD) {
        remaining_from_ut = msg_ut;
        remaining_to_ut = before_ut;
    }
    else {
        remaining_from_ut = after_ut;
        remaining_to_ut = msg_ut;
    }

    usec_t remaining_ut = remaining_to_ut - remaining_from_ut;

    if(total_time_ut)
        *total_time_ut = (before_ut > after_ut) ? before_ut - after_ut : 1;

    if(remaining_start_ut)
        *remaining_start_ut = remaining_from_ut;

    if(remaining_end_ut)
        *remaining_end_ut = remaining_to_ut;

    return remaining_ut;
}

static inline size_t sampling_running_file_query_estimate_remaining_lines_by_time(
    LOGS_QUERY_STATUS *lqs,
    struct journal_file *jf,
    FACETS_ANCHOR_DIRECTION direction,
    usec_t msg_ut) {
    size_t scanned_lines = sampling_file_lines_scanned_so_far(lqs);

    // Calculate the proportion of time covered
    usec_t total_time_ut, remaining_start_ut, remaining_end_ut;
    usec_t remaining_time_ut = sampling_running_file_query_remaining_time(
        lqs, jf, direction, msg_ut, &total_time_ut, &remaining_start_ut, &remaining_end_ut);
    if (total_time_ut == 0) total_time_ut = 1;

    double proportion_by_time = (double) (total_time_ut - remaining_time_ut) / (double) total_time_ut;

    if (proportion_by_time == 0 || proportion_by_time > 1.0 || !isfinite(proportion_by_time))
        proportion_by_time = 1.0;

    // Estimate the total number of lines in the file
    size_t expected_matching_logs_by_time = (size_t)((double)scanned_lines / proportion_by_time);

    if(jf->messages_in_file && expected_matching_logs_by_time > jf->messages_in_file)
        expected_matching_logs_by_time = jf->messages_in_file;

    // Calculate the estimated number of remaining lines
    size_t remaining_logs_by_time = expected_matching_logs_by_time - scanned_lines;
    if (remaining_logs_by_time < 1) remaining_logs_by_time = 1;

    //    nd_log(NDLS_COLLECTORS, NDLP_INFO,
    //           "JOURNAL ESTIMATION: '%s' "
    //           "scanned_lines=%zu [sampled=%zu, unsampled=%zu, estimated=%zu], "
    //           "file [%"PRIu64" - %"PRIu64", duration %"PRId64", known lines in file %zu], "
    //           "query [%"PRIu64" - %"PRIu64", duration %"PRId64"], "
    //           "first message read from the file at %"PRIu64", current message at %"PRIu64", "
    //           "proportion of time %.2f %%, "
    //           "expected total lines in file %zu, "
    //           "remaining lines %zu, "
    //           "remaining time %"PRIu64" [%"PRIu64" - %"PRIu64", duration %"PRId64"]"
    //           , jf->filename
    //           , scanned_lines, fqs->samples_per_file.sampled, fqs->samples_per_file.unsampled, fqs->samples_per_file.estimated
    //           , jf->msg_first_ut, jf->msg_last_ut, jf->msg_last_ut - jf->msg_first_ut, jf->messages_in_file
    //           , fqs->query_file.start_ut, fqs->query_file.stop_ut, fqs->query_file.stop_ut - fqs->query_file.start_ut
    //           , fqs->query_file.first_msg_ut, msg_ut
    //           , proportion_by_time * 100.0
    //           , expected_matching_logs_by_time
    //           , remaining_logs_by_time
    //           , remaining_time_ut, remaining_start_ut, remaining_end_ut, remaining_end_ut - remaining_start_ut
    //           );

    return remaining_logs_by_time;
}

static inline size_t sampling_running_file_query_estimate_remaining_lines(
    sd_journal *j __maybe_unused, LOGS_QUERY_STATUS *lqs, struct journal_file *jf,
    FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {
    size_t remaining_logs_by_seqnum = 0;

#ifdef HAVE_SD_JOURNAL_GET_SEQNUM
    size_t expected_matching_logs_by_seqnum = 0;
    double proportion_by_seqnum = 0.0;
    uint64_t current_msg_seqnum;
    sd_id128_t current_msg_writer;
    if(!lqs->c.query_file.first_msg_seqnum || sd_journal_get_seqnum(j, &current_msg_seqnum, &current_msg_writer) < 0) {
        lqs->c.query_file.first_msg_seqnum = 0;
        lqs->c.query_file.first_msg_writer = SD_ID128_NULL;
    }
    else if(jf->messages_in_file) {
        size_t scanned_lines = sampling_file_lines_scanned_so_far(lqs);

        double proportion_of_all_lines_so_far;
        if(direction == FACETS_ANCHOR_DIRECTION_FORWARD)
            proportion_of_all_lines_so_far = (double)scanned_lines / (double)(current_msg_seqnum - jf->first_seqnum);
        else
            proportion_of_all_lines_so_far = (double)scanned_lines / (double)(jf->last_seqnum - current_msg_seqnum);

        if(proportion_of_all_lines_so_far > 1.0)
            proportion_of_all_lines_so_far = 1.0;

        expected_matching_logs_by_seqnum = (size_t)(proportion_of_all_lines_so_far * (double)jf->messages_in_file);

        proportion_by_seqnum = (double)scanned_lines / (double)expected_matching_logs_by_seqnum;

        if (proportion_by_seqnum == 0 || proportion_by_seqnum > 1.0 || !isfinite(proportion_by_seqnum))
            proportion_by_seqnum = 1.0;

        remaining_logs_by_seqnum = expected_matching_logs_by_seqnum - scanned_lines;
        if(!remaining_logs_by_seqnum) remaining_logs_by_seqnum = 1;
    }
#endif

    if(remaining_logs_by_seqnum)
        return remaining_logs_by_seqnum;

    return sampling_running_file_query_estimate_remaining_lines_by_time(lqs, jf, direction, msg_ut);
}

static inline void sampling_decide_file_sampling_every(sd_journal *j,
                                                       LOGS_QUERY_STATUS *lqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {
    size_t files_matched = lqs->c.files_matched;
    if(!files_matched) files_matched = 1;

    size_t remaining_lines = sampling_running_file_query_estimate_remaining_lines(j, lqs, jf, direction, msg_ut);
    size_t wanted_samples = (lqs->rq.sampling / 2) / files_matched;
    if(!wanted_samples) wanted_samples = 1;

    lqs->c.samples_per_file.every = remaining_lines / wanted_samples;

    if(lqs->c.samples_per_file.every < 1)
        lqs->c.samples_per_file.every = 1;
}

typedef enum {
    SAMPLING_STOP_AND_ESTIMATE = -1,
    SAMPLING_FULL = 0,
    SAMPLING_SKIP_FIELDS = 1,
} sampling_t;

static inline sampling_t is_row_in_sample(
    sd_journal *j, LOGS_QUERY_STATUS *lqs, struct journal_file *jf,
    usec_t msg_ut, FACETS_ANCHOR_DIRECTION direction, bool candidate_to_keep) {
    if(!lqs->rq.sampling || candidate_to_keep)
        return SAMPLING_FULL;

    if(unlikely(msg_ut < lqs->c.samples_per_time_slot.start_ut))
        msg_ut = lqs->c.samples_per_time_slot.start_ut;
    if(unlikely(msg_ut > lqs->c.samples_per_time_slot.end_ut))
        msg_ut = lqs->c.samples_per_time_slot.end_ut;

    size_t slot = (msg_ut - lqs->c.samples_per_time_slot.start_ut) / lqs->c.samples_per_time_slot.step_ut;
    if(slot >= lqs->c.samples.slots)
        slot = lqs->c.samples.slots - 1;

    bool should_sample = false;

    if(lqs->c.samples.sampled < lqs->c.samples.enable_after_samples ||
        lqs->c.samples_per_file.sampled < lqs->c.samples_per_file.enable_after_samples ||
        lqs->c.samples_per_time_slot.sampled[slot] < lqs->c.samples_per_time_slot.enable_after_samples)
        should_sample = true;

    else if(lqs->c.samples_per_file.recalibrate >= SYSTEMD_JOURNAL_SAMPLING_RECALIBRATE || !lqs->c.samples_per_file.every) {
        // this is the first to be unsampled for this file
        sampling_decide_file_sampling_every(j, lqs, jf, direction, msg_ut);
        lqs->c.samples_per_file.recalibrate = 0;
        should_sample = true;
    }
    else {
        // we sample 1 every fqs->samples_per_file.every
        if(lqs->c.samples_per_file.skipped >= lqs->c.samples_per_file.every) {
            lqs->c.samples_per_file.skipped = 0;
            should_sample = true;
        }
        else
            lqs->c.samples_per_file.skipped++;
    }

    if(should_sample) {
        lqs->c.samples.sampled++;
        lqs->c.samples_per_file.sampled++;
        lqs->c.samples_per_time_slot.sampled[slot]++;

        return SAMPLING_FULL;
    }

    lqs->c.samples_per_file.recalibrate++;

    lqs->c.samples.unsampled++;
    lqs->c.samples_per_file.unsampled++;
    lqs->c.samples_per_time_slot.unsampled[slot]++;

    if(lqs->c.samples_per_file.unsampled > lqs->c.samples_per_file.sampled) {
        double progress_by_time = sampling_running_file_query_progress_by_time(lqs, jf, direction, msg_ut);

        if(progress_by_time > SYSTEMD_JOURNAL_ENABLE_ESTIMATIONS_FILE_PERCENTAGE)
            return SAMPLING_STOP_AND_ESTIMATE;
    }

    return SAMPLING_SKIP_FIELDS;
}

static inline void sampling_update_running_query_file_estimates(
    FACETS *facets, sd_journal *j,
    LOGS_QUERY_STATUS *lqs, struct journal_file *jf, usec_t msg_ut, FACETS_ANCHOR_DIRECTION direction) {
    usec_t total_time_ut, remaining_start_ut, remaining_end_ut;
    sampling_running_file_query_remaining_time(
        lqs, jf, direction, msg_ut, &total_time_ut, &remaining_start_ut, &remaining_end_ut);
    size_t remaining_lines = sampling_running_file_query_estimate_remaining_lines(j, lqs, jf, direction, msg_ut);
    facets_update_estimations(facets, remaining_start_ut, remaining_end_ut, remaining_lines);
    lqs->c.samples.estimated += remaining_lines;
    lqs->c.samples_per_file.estimated += remaining_lines;
}

#endif //NETDATA_SYSTEMD_JOURNAL_SAMPLING_H
