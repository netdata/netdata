// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"
#include "database/rrdset-collection.h"
#include "daemon/unit_test_bridge.h"

// Charts fed through the plugin protocol with their collection times (`END <seconds> <microseconds> 1`), as Go
// plugins send every sample. A sample whose time is not after the chart's last collection time must leave the chart
// as if it had not been sent: no point is stored, the chart's clock and counter baseline stay where the last newer
// sample left them, and the next newer sample is placed at its own time with its own rate. Each case feeds such
// samples to one chart and the same feed without them to a control chart, and checks both against the points the
// Agent must store.

// a whole multiple of every update_every used below, far from the wall clock
#define SAMPLE_TIMES_BASE_S 200000000LL

struct sample_times_sample {
    int64_t at_us;              // collection time, microseconds after SAMPLE_TIMES_BASE_S (may be negative)
    collected_number value;     // counter value of dimension 'events'
    bool behind;                // not after the chart's clock: the control chart does not receive it
    bool restart;               // the plugin restarts before this sample; the Agent keeps the chart
    bool add_late;              // the plugin adds dimension 'late' before this sample (both charts)
    bool set_late;              // the sample sets dimension 'late' to late_value
    collected_number late_value;
};

struct sample_times_point {
    int64_t at_s;               // slot time, seconds after SAMPLE_TIMES_BASE_S
    NETDATA_DOUBLE value;       // NAN for a gap
};

struct sample_times_case {
    const char *name;
    int update_every;
    bool store_first;
    const struct sample_times_sample *samples;
    size_t samples_count;
    const struct sample_times_point *points;    // 'events': every slot from the first stored point to the last one
    size_t points_count;
    const struct sample_times_point *late_points;   // 'late', when the case adds it
    size_t late_points_count;
    bool late_float;            // 'late' is a `type=float` dimension
};

struct sample_times_plugin {
    struct plugind cd;
    PARSER *parser;
};

static void sample_times_plugin_start(struct sample_times_plugin *p, int update_every) {
    *p = (struct sample_times_plugin){ .cd = { .update_every = update_every } };
    p->cd.filename = string_strdupz("sample-times-unittest");

    struct parser_user_object user = { .host = localhost, .enabled = 1, .cd = &p->cd };
    p->parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(p->parser, PARSER_INIT_PLUGINSD);
}

// what the Agent does when a plugin exits: its charts become obsolete, its parser goes away
static void sample_times_plugin_stop(struct sample_times_plugin *p, RRDSET *st) {
    if(st)
        rrdset_is_obsolete___safe_from_collector_thread(st);

    pluginsd_process_cleanup(p->parser);
    string_freez(p->cd.filename);
    *p = (struct sample_times_plugin){ 0 };
}

static bool sample_times_command(struct sample_times_plugin *p, const char *fmt, ...) PRINTFLIKE(2, 3);
static bool sample_times_command(struct sample_times_plugin *p, const char *fmt, ...) {
    char line[512];
    va_list args;
    va_start(args, fmt);
    vsnprintfz(line, sizeof(line), fmt, args);
    va_end(args);

    char *input = strdupz(line);
    int rc = parser_action(p->parser, input);
    freez(input);

    if(rc)
        fprintf(stderr, "SAMPLE TIMES unittest: command failed: %s\n", line);

    return rc == 0;
}

static RRDSET *sample_times_define(struct sample_times_plugin *p, const char *id, const struct sample_times_case *tc,
                                   bool with_late) {
    if(!sample_times_command(p, "CHART sample_times.%s '' 'Sample times' events/s sample_times sample_times.events "
                                "line 1000 %d '%s'", id, tc->update_every, tc->store_first ? "store_first" : "") ||
       !sample_times_command(p, "DIMENSION events '' incremental 1 1") ||
       (with_late && !sample_times_command(p, "DIMENSION late '' incremental 1 1 '%s'",
                                           tc->late_float ? "type=float" : "")))
        return NULL;

    char full_id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(full_id, sizeof(full_id), "sample_times.%s", id);
    return rrdset_find(localhost, full_id, false);
}

static usec_t sample_times_abs_ut(int64_t at_us) {
    return (usec_t)(SAMPLE_TIMES_BASE_S * (int64_t)USEC_PER_SEC + at_us);
}

static bool sample_times_send(struct sample_times_plugin *p, const char *id, const struct sample_times_sample *s) {
    usec_t at_ut = sample_times_abs_ut(s->at_us);

    return sample_times_command(p, "BEGIN sample_times.%s", id) &&
           sample_times_command(p, "SET 'events' = %lld", (long long)s->value) &&
           (!s->set_late || sample_times_command(p, "SET 'late' = %lld", (long long)s->late_value)) &&
           sample_times_command(p, "END %llu %llu 1",
                                (unsigned long long)(at_ut / USEC_PER_SEC), (unsigned long long)(at_ut % USEC_PER_SEC));
}

// Feeds the case to one chart, the samples behind the chart's clock only when with_behind is set. After every
// sample, no stored point and no chart clock may be past the newest collection time sent so far (the first sample's
// clock is aligned to its update_every boundary, plus half a second without store_first).
static RRDSET *sample_times_feed(const struct sample_times_case *tc, const char *id, bool with_behind, int *errors) {
    struct sample_times_plugin p;
    sample_times_plugin_start(&p, tc->update_every);

    bool late = false;
    RRDSET *st = sample_times_define(&p, id, tc, late);
    if(!st) {
        (*errors)++;
        sample_times_plugin_stop(&p, NULL);
        return NULL;
    }

    usec_t newest_ut = 0, first_clock_ut = 0;
    for(size_t i = 0; i < tc->samples_count; i++) {
        const struct sample_times_sample *s = &tc->samples[i];

        if(s->restart) {
            sample_times_plugin_stop(&p, st);
            sample_times_plugin_start(&p, tc->update_every);
        }

        if(s->restart || s->add_late) {
            late = late || s->add_late;
            if(sample_times_define(&p, id, tc, late) != st) {
                fprintf(stderr, "SAMPLE TIMES unittest '%s': the chart definition did not reach the same chart\n", id);
                (*errors)++;
                break;
            }
        }

        if(s->behind && !with_behind)
            continue;

        if(!sample_times_send(&p, id, s)) {
            (*errors)++;
            break;
        }

        newest_ut = MAX(newest_ut, sample_times_abs_ut(s->at_us));
        usec_t clock_ut = timeval_usec(&st->last_collected_time);
        if(!first_clock_ut)
            first_clock_ut = clock_ut;

        usec_t last_stored_ut = (usec_t)st->last_updated.tv_sec * USEC_PER_SEC;
        if(clock_ut > MAX(newest_ut, first_clock_ut) || last_stored_ut > newest_ut) {
            fprintf(stderr,
                    "SAMPLE TIMES unittest '%s': after the sample at %+.6f s the chart clock is at %+.6f s and its "
                    "last point at %+lld s, past the newest collection time %+.6f s\n",
                    id, (double)s->at_us / USEC_PER_SEC,
                    (double)((int64_t)clock_ut - SAMPLE_TIMES_BASE_S * (int64_t)USEC_PER_SEC) / USEC_PER_SEC,
                    (long long)(st->last_updated.tv_sec - SAMPLE_TIMES_BASE_S),
                    (double)((int64_t)newest_ut - SAMPLE_TIMES_BASE_S * (int64_t)USEC_PER_SEC) / USEC_PER_SEC);
            (*errors)++;
        }
    }

    sample_times_plugin_stop(&p, NULL);
    return st;
}

// Reads the dimension's stored points over the expected slots: each must be at its slot with its value, and the
// chart's last point must be the last expected one.
static int sample_times_check_points(const char *id, RRDSET *st, const char *dimension,
                                     const struct sample_times_point *points, size_t points_count) {
    RRDDIM *rd = rrddim_find(st, dimension, false);
    if(!rd) {
        fprintf(stderr, "SAMPLE TIMES unittest '%s': no dimension '%s'\n", id, dimension);
        return 1;
    }

    int errors = 0;
    time_t first_s = (time_t)(SAMPLE_TIMES_BASE_S + points[0].at_s);
    time_t last_s = (time_t)(SAMPLE_TIMES_BASE_S + points[points_count - 1].at_s);

    if(st->last_updated.tv_sec != last_s) {
        fprintf(stderr, "SAMPLE TIMES unittest '%s': last point at %+lld s, expected %+lld s\n",
                id, (long long)(st->last_updated.tv_sec - SAMPLE_TIMES_BASE_S),
                (long long)points[points_count - 1].at_s);
        errors++;
    }

    struct storage_engine_query_handle seqh;
    unittest_storage_engine_query_init(rd->tiers[0].seb, rd->tiers[0].smh, &seqh, first_s, last_s,
                                       STORAGE_PRIORITY_SYNCHRONOUS);

    size_t found = 0;
    while(!unittest_storage_engine_query_is_finished(&seqh)) {
        STORAGE_POINT sp = unittest_storage_engine_query_next_metric(&seqh);
        if(found >= points_count) {
            found++;
            continue;
        }

        const struct sample_times_point *want = &points[found++];
        bool ok = sp.end_time_s == (time_t)(SAMPLE_TIMES_BASE_S + want->at_s) &&
                  (isnan(want->value) ? isnan(sp.sum) : (!isnan(sp.sum) && fabsndd(sp.sum - want->value) < 0.0005));
        if(!ok) {
            fprintf(stderr, "SAMPLE TIMES unittest '%s': '%s' point at %+lld s holds " NETDATA_DOUBLE_FORMAT
                    ", expected " NETDATA_DOUBLE_FORMAT " at %+lld s\n",
                    id, dimension, (long long)(sp.end_time_s - SAMPLE_TIMES_BASE_S), sp.sum, want->value,
                    (long long)want->at_s);
            errors++;
        }
    }
    unittest_storage_engine_query_finalize(&seqh);

    if(found != points_count) {
        fprintf(stderr, "SAMPLE TIMES unittest '%s': '%s' read %zu points, expected %zu\n",
                id, dimension, found, points_count);
        errors++;
    }

    return errors;
}

static int sample_times_check_chart(const struct sample_times_case *tc, const char *id, RRDSET *st) {
    int errors = sample_times_check_points(id, st, "events", tc->points, tc->points_count);
    if(tc->late_points_count) {
        errors += sample_times_check_points(id, st, "late", tc->late_points, tc->late_points_count);

        RRDDIM *late = rrddim_find(st, "late", false);
        if(late && (rrddim_is_float(late) ? true : false) != tc->late_float) {
            fprintf(stderr, "SAMPLE TIMES unittest '%s': 'late' is %s, expected %s\n",
                    id, rrddim_is_float(late) ? "float" : "int", tc->late_float ? "float" : "int");
            errors++;
        }
    }
    return errors;
}

static int sample_times_run_case(const struct sample_times_case *tc) {
    fprintf(stderr, "SAMPLE TIMES unittest: %s\n", tc->name);

    char test_id[RRD_ID_LENGTH_MAX + 1], control_id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(test_id, sizeof(test_id), "%s_behind", tc->name);
    snprintfz(control_id, sizeof(control_id), "%s_control", tc->name);

    int errors = 0;
    RRDSET *st = sample_times_feed(tc, test_id, true, &errors);
    RRDSET *control = sample_times_feed(tc, control_id, false, &errors);

    if(st && control) {
        errors += sample_times_check_chart(tc, test_id, st);
        errors += sample_times_check_chart(tc, control_id, control);

        if(timeval_usec(&st->last_collected_time) != timeval_usec(&control->last_collected_time)) {
            fprintf(stderr, "SAMPLE TIMES unittest '%s': chart clock %llu us, control %llu us\n", tc->name,
                    (unsigned long long)timeval_usec(&st->last_collected_time),
                    (unsigned long long)timeval_usec(&control->last_collected_time));
            errors++;
        }
    }

    rrdset_free(st);
    rrdset_free(control);
    return errors;
}

// A time that reaches a chart without its own collection time (BEGIN without microseconds, or SYNC_CLOCK) is the
// Agent's receipt time. When the chart's clock is ahead of it, the chart must wait, not jump ahead by the difference.
static int sample_times_receipt_time_behind_clock(void) {
    fprintf(stderr, "SAMPLE TIMES unittest: receipt time behind the chart clock\n");

    const struct sample_times_case tc = { .name = "receipt", .update_every = 1 };
    struct sample_times_plugin p;
    sample_times_plugin_start(&p, tc.update_every);

    int errors = 0;
    RRDSET *st = sample_times_define(&p, "receipt", &tc, false);
    const struct sample_times_sample first = { .at_us = 10 * USEC_PER_SEC + 200000, .value = 1 };
    const struct sample_times_sample second = { .at_us = 11 * USEC_PER_SEC + 200000, .value = 2 };
    if(!st || !sample_times_send(&p, "receipt", &first) || !sample_times_send(&p, "receipt", &second)) {
        sample_times_plugin_stop(&p, NULL);
        rrdset_free(st);
        return 1;
    }

    usec_t clock_ut = timeval_usec(&st->last_collected_time);
    struct {
        const char *name;
        int64_t receipt_from_clock_us;
        usec_t duration_ut;
        bool sync_clock;
        usec_t expected_ut;
    } cases[] = {
        { "no interval, 5 s behind", -5 * (int64_t)USEC_PER_SEC, 0, false, 0 },
        { "no interval, at the clock", 0, 0, false, 0 },
        { "no interval, 1 s ahead", (int64_t)USEC_PER_SEC, 0, false, USEC_PER_SEC },
        { "SYNC_CLOCK, 5 s behind", -5 * (int64_t)USEC_PER_SEC, USEC_PER_SEC, true, 0 },
        { "SYNC_CLOCK, 2 s ahead", 2 * (int64_t)USEC_PER_SEC, USEC_PER_SEC, true, 2 * USEC_PER_SEC },
    };

    for(size_t i = 0; i < _countof(cases); i++) {
        usec_t receipt_ut = (usec_t)((int64_t)clock_ut + cases[i].receipt_from_clock_us);
        struct timeval receipt = { .tv_sec = (time_t)(receipt_ut / USEC_PER_SEC),
                                   .tv_usec = (suseconds_t)(receipt_ut % USEC_PER_SEC) };
        if(cases[i].sync_clock)
            rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);

        rrdset_timed_next(st, receipt, cases[i].duration_ut);
        if(st->usec_since_last_update != cases[i].expected_ut) {
            fprintf(stderr, "SAMPLE TIMES unittest 'receipt': %s: interval %llu us, expected %llu us\n",
                    cases[i].name, (unsigned long long)st->usec_since_last_update,
                    (unsigned long long)cases[i].expected_ut);
            errors++;
        }
    }

    sample_times_plugin_stop(&p, NULL);
    rrdset_free(st);
    return errors;
}

#define SAMPLE_TIMES_CASE(case_name, ue, first, samples_array, points_array) {  \
    .name = (case_name), .update_every = (ue), .store_first = (first),         \
    .samples = (samples_array), .samples_count = _countof(samples_array),      \
    .points = (points_array), .points_count = _countof(points_array) }

#define S(seconds) ((int64_t)((seconds) * 1000000.0 + ((seconds) < 0 ? -0.5 : 0.5)))

int pluginsd_sample_times_unittest(void) {
    int saved_gap = gap_when_lost_iterations_above;
    // the Agent's effective default: the configured 1 plus the 2 it always adds
    gap_when_lost_iterations_above = 3;

    // The first sample at +0.1 s puts the chart clock at +0.5 s. A backward wall-clock step then makes the plugin's
    // own clock hand out +0.100001 s and +0.100002 s, and two more samples arrive at +0.3 s and at the clock itself.
    // Counters grow 100/s with the collection time.
    static const struct sample_times_sample aligned[] = {
        { S(0.1), 10 },
        { S(0.100001), 10, true }, { S(0.100002), 10, true }, { S(0.3), 30, true }, { S(0.5), 50, true },
        { S(1.1), 110 }, { S(2.1), 210 }, { S(3.1), 310 }, { S(4.1), 410 },
    };
    // the first sample is half a second off its slot: 100 * 0.5/0.6 (and the remainder into the next slot)
    static const struct sample_times_point aligned_points[] = {
        { 1, 250.0 / 3.0 }, { 2, 90.0 + 50.0 / 3.0 }, { 3, 100 }, { 4, 100 },
    };

    // With store_first the first sample's clock is its whole second, and it is stored there (as a counter seen from
    // zero). Samples behind that second, and one at it, follow.
    static const struct sample_times_sample aligned_store_first[] = {
        { S(0.1), 1010 },
        { S(-0.9), 910, true }, { S(-0.5), 950, true }, { S(0.0), 1000, true },
        { S(1.1), 1110 }, { S(2.1), 1210 }, { S(3.1), 1310 }, { S(4.1), 1410 },
    };
    static const struct sample_times_point aligned_store_first_points[] = {
        { 0, 1010 }, { 1, 1000.0 / 11.0 }, { 2, 90.0 + 100.0 / 11.0 }, { 3, 100 }, { 4, 100 },
    };

    // The plugin restarts during a 30 s backward wall-clock step and the Agent keeps the chart: the new process
    // sends times 30 s behind the chart before its own clock catches up. The control chart restarts at the same
    // point (the restart is checked before the sample is skipped).
    static const struct sample_times_sample retained[] = {
        { S(96.2), 9620 }, { S(97.2), 9720 }, { S(98.2), 9820 }, { S(99.2), 9920 }, { S(100.2), 10020 },
        { S(70.2), 7020, true, true }, { S(71.2), 7120, true }, { S(72.2), 7220, true },
        { S(101.2), 10120 }, { S(102.2), 10220 }, { S(103.2), 10320 },
    };
    static const struct sample_times_point retained_points[] = {
        { 97, 500.0 / 7.0 }, { 98, 80.0 + 200.0 / 7.0 }, { 99, 100 }, { 100, 100 },
        { 101, 100 }, { 102, 100 }, { 103, 100 },
    };

    // Two jobs feed one chart: a slow scrape collected at +2.3 s arrives after the sample of +5.2 s, and a second
    // sample of +5.2 s repeats that time.
    static const struct sample_times_sample out_of_order[] = {
        { S(0.2), 20 }, { S(1.2), 120 }, { S(2.2), 220 }, { S(3.2), 320 }, { S(4.2), 420 }, { S(5.2), 520 },
        { S(2.3), 230, true }, { S(5.2), 530, true },
        { S(6.2), 620 }, { S(7.2), 720 }, { S(8.2), 820 }, { S(9.2), 920 },
    };
    static const struct sample_times_point out_of_order_points[] = {
        { 1, 500.0 / 7.0 }, { 2, 80.0 + 200.0 / 7.0 }, { 3, 100 }, { 4, 100 }, { 5, 100 },
        { 6, 100 }, { 7, 100 }, { 8, 100 }, { 9, 100 },
    };

    // A 10 s job: the scrape of +10.2 s is slow, the tick of +20.2 s is dropped, the scrape of +30.2 s omits the
    // chart. The next sample is placed at +40.2 s; the oldest missed slot is a gap (gap when lost iterations above).
    static const struct sample_times_sample slow_scrape[] = {
        { S(0.2), 20 }, { S(10.2), 1020 }, { S(40.2), 4020 }, { S(50.2), 5020 }, { S(60.2), 6020 },
    };
    static const struct sample_times_point slow_scrape_points[] = {
        { 10, 95.0 / 0.97 }, { 20, NAN }, { 30, 100 }, { 40, 100 }, { 50, 100 }, { 60, 100 },
    };

    // The plugin adds dimension 'late', and its first value arrives in a sample behind the chart. That value must not
    // become the dimension's baseline: its first accepted value (+3.2 s) has no rate yet, the next one has.
    static const struct sample_times_sample late_dimension[] = {
        { S(0.2), 20 }, { S(1.2), 120 }, { S(2.2), 220 },
        { .at_us = S(1.5), .value = 150, .behind = true, .add_late = true, .set_late = true, .late_value = 5000 },
        { .at_us = S(3.2), .value = 320, .set_late = true, .late_value = 5100 },
        { .at_us = S(4.2), .value = 420, .set_late = true, .late_value = 5200 },
        { .at_us = S(5.2), .value = 520, .set_late = true, .late_value = 5300 },
    };
    static const struct sample_times_point late_dimension_points[] = {
        { 1, 500.0 / 7.0 }, { 2, 80.0 + 200.0 / 7.0 }, { 3, 100 }, { 4, 100 }, { 5, 100 },
    };
    static const struct sample_times_point late_dimension_late_points[] = {
        { 3, NAN }, { 4, 80 }, { 5, 100 },
    };

    struct sample_times_case late_dimension_case =
        SAMPLE_TIMES_CASE("late_dimension", 1, false, late_dimension, late_dimension_points);
    late_dimension_case.late_points = late_dimension_late_points;
    late_dimension_case.late_points_count = _countof(late_dimension_late_points);

    // the same with a `type=float` dimension, which takes the float SET path
    struct sample_times_case late_float_dimension_case = late_dimension_case;
    late_float_dimension_case.name = "late_float_dimension";
    late_float_dimension_case.late_float = true;

    const struct sample_times_case cases[] = {
        SAMPLE_TIMES_CASE("aligned", 1, false, aligned, aligned_points),
        SAMPLE_TIMES_CASE("aligned_store_first", 1, true, aligned_store_first, aligned_store_first_points),
        SAMPLE_TIMES_CASE("retained", 1, false, retained, retained_points),
        SAMPLE_TIMES_CASE("out_of_order", 1, false, out_of_order, out_of_order_points),
        SAMPLE_TIMES_CASE("slow_scrape", 10, false, slow_scrape, slow_scrape_points),
        late_dimension_case,
        late_float_dimension_case,
    };

    int errors = 0;
    for(size_t i = 0; i < _countof(cases); i++)
        errors += sample_times_run_case(&cases[i]);

    errors += sample_times_receipt_time_behind_clock();

    gap_when_lost_iterations_above = saved_gap;

    fprintf(stderr, "SAMPLE TIMES unittest: %s\n", errors ? "FAILED" : "OK");
    return errors;
}
