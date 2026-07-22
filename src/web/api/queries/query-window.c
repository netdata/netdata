// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

// #define DEBUG_QUERY_LOGIC 1

#ifdef DEBUG_QUERY_LOGIC
#define query_debug_log_init() BUFFER *debug_log = buffer_create(1000)
#define query_debug_log(args...) buffer_sprintf(debug_log, ##args)
#define query_debug_log_fin() { \
        netdata_log_info("QUERY: '%s', after:%ld, before:%ld, duration:%ld, points:%zu, res:%ld - wanted => after:%ld, before:%ld, points:%zu, group:%zu, granularity:%ld, resgroup:%ld, resdiv:" NETDATA_DOUBLE_FORMAT_AUTO " %s", qt->id, after_requested, before_requested, before_requested - after_requested, points_requested, resampling_time_requested, after_wanted, before_wanted, points_wanted, group, query_granularity, resampling_group, resampling_divisor, buffer_tostring(debug_log)); \
        buffer_free(debug_log); \
        debug_log = NULL; \
    }
#define query_debug_log_free() do { buffer_free(debug_log); } while(0)
#else
#define query_debug_log_init() debug_dummy()
#define query_debug_log(args...) debug_dummy()
#define query_debug_log_fin() debug_dummy()
#define query_debug_log_free() debug_dummy()
#endif

static inline bool query_window_uint_to_size(uintmax_t value, size_t *result) {
    if(unlikely(value > SIZE_MAX))
        return false;

    *result = (size_t)value;
    return true;
}

static inline bool query_window_uint_to_time(uintmax_t value, time_t *result) {
    const uintmax_t maximum = ((uintmax_t)1 << (sizeof(time_t) * CHAR_BIT - 1)) - 1;

    if(unlikely(value > maximum))
        return false;

    *result = (time_t)value;
    return true;
}

bool query_target_calculate_window(QUERY_TARGET *qt) {
    if (unlikely(!qt)) return false;

    size_t points_requested = qt->request.points;
    time_t after_requested = qt->request.after;
    time_t before_requested = qt->request.before;
    RRDR_TIME_GROUPING group_method = qt->request.time_group_method;
    time_t resampling_time_requested = qt->request.resampling_time;
    RRDR_OPTIONS options = qt->window.options;
    size_t tier = qt->request.tier;
    time_t update_every = qt->db.minimum_latest_update_every_s ? qt->db.minimum_latest_update_every_s : 1;

    // RULES
    // points_requested = 0
    // the user wants all the natural points the database has
    //
    // after_requested = 0
    // the user wants to start the query from the oldest point in our database
    //
    // before_requested = 0
    // the user wants the query to end to the latest point in our database
    //
    // when natural points are wanted, the query has to be aligned to the update_every
    // of the database

    size_t points_wanted = points_requested;
    time_t after_wanted = after_requested;
    time_t before_wanted = before_requested;

    bool aligned = !(options & RRDR_OPTION_NOT_ALIGNED);
    bool automatic_natural_points = (points_wanted == 0);
    bool relative_period_requested = false;
    bool natural_points = (options & RRDR_OPTION_NATURAL_POINTS) || automatic_natural_points;
    bool before_is_aligned_to_db_end = false;

    query_debug_log_init();

    if (rrdr_relative_window_value_is_relative(before_requested) ||
        rrdr_relative_window_value_is_relative(after_requested)) {
        relative_period_requested = true;
        natural_points = true;
        options |= RRDR_OPTION_NATURAL_POINTS;
        query_debug_log(":relative+natural");
    }

    // if the user wants virtual points, make sure we do it
    if (options & RRDR_OPTION_VIRTUAL_POINTS)
        natural_points = false;

    // set the right flag about natural and virtual points
    if (natural_points) {
        options |= RRDR_OPTION_NATURAL_POINTS;

        if (options & RRDR_OPTION_VIRTUAL_POINTS)
            options &= ~RRDR_OPTION_VIRTUAL_POINTS;
    }
    else {
        options |= RRDR_OPTION_VIRTUAL_POINTS;

        if (options & RRDR_OPTION_NATURAL_POINTS)
            options &= ~RRDR_OPTION_NATURAL_POINTS;
    }

    if (after_wanted == 0 || before_wanted == 0) {
        relative_period_requested = true;

        time_t first_entry_s = qt->db.first_time_s;
        time_t last_entry_s = qt->db.last_time_s;

        if (first_entry_s == 0 || last_entry_s == 0) {
            internal_error(true, "QUERY: no data detected on query '%s' (db first_entry_t = %ld, last_entry_t = %ld)", qt->id, first_entry_s, last_entry_s);
            after_wanted = qt->window.after;
            before_wanted = qt->window.before;

            if(after_wanted == before_wanted)
                after_wanted = before_wanted - update_every;

            if (points_wanted == 0) {
                points_wanted = (before_wanted - after_wanted) / update_every;
                query_debug_log(":zero points_wanted %zu", points_wanted);
            }
        }
        else {
            query_debug_log(":first_entry_t %ld, last_entry_t %ld", first_entry_s, last_entry_s);

            if (after_wanted == 0) {
                after_wanted = first_entry_s;
                query_debug_log(":zero after_wanted %ld", after_wanted);
            }

            if (before_wanted == 0) {
                before_wanted = last_entry_s;
                before_is_aligned_to_db_end = true;
                query_debug_log(":zero before_wanted %ld", before_wanted);
            }

            if (points_wanted == 0) {
                points_wanted = (last_entry_s - first_entry_s) / update_every;
                query_debug_log(":zero points_wanted %zu", points_wanted);
            }
        }
    }

    if (points_wanted == 0) {
        points_wanted = 600;
        query_debug_log(":zero600 points_wanted %zu", points_wanted);
    }

    // convert our before_wanted and after_wanted to absolute
    rrdr_relative_window_to_absolute_query(&after_wanted, &before_wanted, NULL, unittest_running);
    query_debug_log(":relative2absolute after %ld, before %ld", after_wanted, before_wanted);

    if (natural_points && (options & RRDR_OPTION_SELECTED_TIER) &&
        tier > 0 && tier < nd_profile.storage_tiers && nd_profile.storage_tiers > 1) {
        update_every = query_target_min_update_every_for_tier(qt, tier);

        if (update_every <= 0) update_every = qt->db.minimum_latest_update_every_s;
        query_debug_log(":natural update every %ld", update_every);
    }

    // this is the update_every of the query
    // it may be different to the update_every of the database
    time_t query_granularity = (natural_points) ? update_every : 1;
    if (query_granularity <= 0) query_granularity = 1;
    query_debug_log(":query_granularity %ld", query_granularity);

    // align before_wanted and after_wanted to query_granularity
    if (before_wanted % query_granularity) {
        before_wanted -= before_wanted % query_granularity;
        query_debug_log(":granularity align before_wanted %ld", before_wanted);
    }

    if (after_wanted % query_granularity) {
        after_wanted -= after_wanted % query_granularity;
        query_debug_log(":granularity align after_wanted %ld", after_wanted);
    }

    // automatic_natural_points is set when the user wants all the points available in the database
    if (automatic_natural_points) {
        points_wanted = (before_wanted - after_wanted + 1) / query_granularity;
        if (unlikely(points_wanted <= 0)) points_wanted = 1;
        query_debug_log(":auto natural points_wanted %zu", points_wanted);
    }

    time_t duration;
    if(unlikely(__builtin_sub_overflow(before_wanted, after_wanted, &duration) || duration < 0))
        goto failed;

    // if the resampling time is too big, extend the duration to the past
    if (unlikely(resampling_time_requested > duration)) {
        if(unlikely(__builtin_sub_overflow(before_wanted, resampling_time_requested, &after_wanted)))
            goto failed;

        duration = resampling_time_requested;
        query_debug_log(":resampling after_wanted %ld", after_wanted);
    }

    // if the duration is not aligned to resampling time
    // extend the duration to the past, to avoid a gap at the chart
    // only when the missing duration is above 1/10th of a point
    if (resampling_time_requested > query_granularity && duration % resampling_time_requested) {
        time_t delta = duration % resampling_time_requested;
        if (delta > resampling_time_requested / 10) {
            time_t extension = resampling_time_requested - delta;
            if(unlikely(__builtin_sub_overflow(after_wanted, extension, &after_wanted) ||
                        __builtin_add_overflow(duration, extension, &duration)))
                goto failed;

            query_debug_log(":resampling2 after_wanted %ld", after_wanted);
        }
    }

    // the available points of the query
    uintmax_t duration_units = (uintmax_t)(duration / query_granularity);
    if(duration % query_granularity == query_granularity - 1 &&
       unlikely(__builtin_add_overflow(duration_units, (uintmax_t)1, &duration_units)))
        goto failed;

    size_t points_available;
    if(unlikely(!query_window_uint_to_size(duration_units, &points_available)))
        goto failed;

    if (unlikely(points_available <= 0)) points_available = 1;
    query_debug_log(":points_available %zu", points_available);

    if (points_wanted > points_available) {
        points_wanted = points_available;
        query_debug_log(":max points_wanted %zu", points_wanted);
    }

    if(points_wanted > 86400 && !unittest_running) {
        points_wanted = 86400;
        query_debug_log(":absolute max points_wanted %zu", points_wanted);
    }

    // calculate the desired grouping of source data points
    size_t group = points_available / points_wanted;
    if (group == 0) group = 1;

    // round "group" to the closest integer
    if (points_available % points_wanted > points_wanted / 2)
        group++;

    query_debug_log(":group %zu", group);

    uintmax_t grouped_points;
    bool grouped_points_fit = !__builtin_mul_overflow(
        (uintmax_t)points_wanted, (uintmax_t)group, &grouped_points);
    uintmax_t required_grouped_points = (uintmax_t)(duration / query_granularity) +
                                        (duration % query_granularity != 0);

    if (grouped_points_fit && grouped_points < required_grouped_points) {
        // the grouping we are going to do, is not enough
        // to cover the entire duration requested, so
        // we have to change the number of points, to make sure we will
        // respect the timeframe as closely as possibly

        // let's see how many points are the optimal
        points_wanted = points_available / group;

        if (points_wanted * group < points_available)
            points_wanted++;

        if (unlikely(points_wanted == 0))
            points_wanted = 1;

        query_debug_log(":optimal points %zu", points_wanted);
    }

    // resampling_time_requested enforces a certain grouping multiple
    NETDATA_DOUBLE resampling_divisor = 1.0;
    size_t resampling_group = 1;
    if (unlikely(resampling_time_requested > query_granularity)) {
        // the points we should group to satisfy gtime
        uintmax_t resampling_group_wide = (uintmax_t)(resampling_time_requested / query_granularity) +
                                         (resampling_time_requested % query_granularity != 0);
        if(unlikely(!query_window_uint_to_size(resampling_group_wide, &resampling_group)))
            goto failed;

        query_debug_log(":resampling group %zu", resampling_group);

        // adapt group according to resampling_group
        if (unlikely(group < resampling_group)) {
            group = resampling_group; // do not allow grouping below the desired one
            query_debug_log(":group less res %zu", group);
        }
        if (unlikely(group % resampling_group)) {
            if(unlikely(__builtin_add_overflow(
                    group, resampling_group - (group % resampling_group), &group)))
                goto failed;

            query_debug_log(":group mod res %zu", group);
        }
    }

    uintmax_t view_update_every_wide;
    time_t view_update_every;
    if(unlikely(__builtin_mul_overflow(
                    (uintmax_t)group, (uintmax_t)query_granularity, &view_update_every_wide) ||
                !query_window_uint_to_time(view_update_every_wide, &view_update_every)))
        goto failed;

    if (unlikely(resampling_time_requested > query_granularity)) {
        // resampling_divisor = group / resampling_group;
        resampling_divisor = (NETDATA_DOUBLE)view_update_every / (NETDATA_DOUBLE)resampling_time_requested;
        query_debug_log(":resampling divisor " NETDATA_DOUBLE_FORMAT, resampling_divisor);
    }

    // now that we have group, align the requested timeframe to fit it.
    // The product is proven representable above; retain the original conversions for negative timestamps.
    bool alignment_needed = aligned && before_wanted % (group * query_granularity);
    time_t alignment = aligned ? before_wanted % view_update_every : 0;
    if (alignment_needed) {
        if (before_is_aligned_to_db_end) {
            if(unlikely(__builtin_sub_overflow(before_wanted, alignment, &before_wanted)))
                goto failed;
        }
        else if(alignment > 0) {
            time_t adjustment = view_update_every - alignment;
            if(unlikely(__builtin_add_overflow(before_wanted, adjustment, &before_wanted)))
                goto failed;
        }
        else {
            if(unlikely(__builtin_add_overflow(before_wanted, view_update_every, &before_wanted) ||
                        __builtin_sub_overflow(before_wanted, alignment, &before_wanted)))
                goto failed;
        }

        query_debug_log(":align before_wanted %ld", before_wanted);
    }

    uintmax_t preceding_points_duration, final_duration;
    if(unlikely(__builtin_mul_overflow(
                    (uintmax_t)(points_wanted - 1), view_update_every_wide, &preceding_points_duration) ||
                __builtin_add_overflow(preceding_points_duration,
                                       view_update_every_wide - (uintmax_t)query_granularity,
                                       &final_duration) ||
                !query_window_uint_to_time(final_duration, &duration) ||
                __builtin_sub_overflow(before_wanted, duration, &after_wanted)))
        goto failed;

    time_t query_start;
    if(unlikely(__builtin_sub_overflow(after_wanted, query_granularity, &query_start)))
        goto failed;
    (void)query_start;

    query_debug_log(":final after_wanted %ld", after_wanted);

    query_debug_log(":final duration %ju", final_duration);

    query_debug_log_fin();

    uintmax_t calculated_points = final_duration / view_update_every_wide + 1;
    internal_error((uintmax_t)points_wanted != calculated_points,
                   "QUERY: points_wanted %zu is not points %zu",
                   points_wanted, (size_t)calculated_points);

    internal_error(group < resampling_group,
                   "QUERY: group %zu is less than the desired group points %zu",
                   group, resampling_group);

    internal_error(group > resampling_group && group % resampling_group,
                   "QUERY: group %zu is not a multiple of the desired group points %zu",
                   group, resampling_group);

    // -------------------------------------------------------------------------
    // update QUERY_TARGET with our calculations

    qt->window.after = after_wanted;
    qt->window.before = before_wanted;
    qt->window.relative = relative_period_requested;
    qt->window.points = points_wanted;
    qt->window.group = group;
    qt->window.time_group_method = group_method;
    qt->window.time_group_options = qt->request.time_group_options;
    qt->window.query_granularity = query_granularity;
    qt->window.resampling_group = resampling_group;
    qt->window.resampling_divisor = resampling_divisor;
    qt->window.options = options;
    qt->window.tier = tier;
    qt->window.aligned = aligned;

    return true;

failed:
    query_debug_log_free();
    return false;
}
