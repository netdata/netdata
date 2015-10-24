#include <pthread.h>
#include <sys/time.h>
#include <stdlib.h>

#include "log.h"
#include "common.h"
#include "rrd2json.h"

#define HOSTNAME_MAX 1024
char *hostname = "unknown";

unsigned long rrd_stats_one_json(RRDSET *st, char *options, struct web_buffer *wb)
{
	time_t now = time(NULL);
	web_buffer_increase(wb, 65536);

	pthread_rwlock_rdlock(&st->rwlock);

	web_buffer_printf(wb,
		"\t\t{\n"
		"\t\t\t\"id\": \"%s\",\n"
		"\t\t\t\"name\": \"%s\",\n"
		"\t\t\t\"type\": \"%s\",\n"
		"\t\t\t\"family\": \"%s\",\n"
		"\t\t\t\"title\": \"%s\",\n"
		"\t\t\t\"priority\": %ld,\n"
		"\t\t\t\"enabled\": %d,\n"
		"\t\t\t\"units\": \"%s\",\n"
		"\t\t\t\"url\": \"/data/%s/%s\",\n"
		"\t\t\t\"chart_type\": \"%s\",\n"
		"\t\t\t\"counter\": %ld,\n"
		"\t\t\t\"entries\": %ld,\n"
		"\t\t\t\"first_entry_t\": %lu,\n"
		"\t\t\t\"last_entry\": %ld,\n"
		"\t\t\t\"last_entry_t\": %lu,\n"
		"\t\t\t\"last_entry_secs_ago\": %lu,\n"
		"\t\t\t\"update_every\": %d,\n"
		"\t\t\t\"isdetail\": %d,\n"
		"\t\t\t\"usec_since_last_update\": %llu,\n"
		"\t\t\t\"collected_total\": " TOTAL_NUMBER_FORMAT ",\n"
		"\t\t\t\"last_collected_total\": " TOTAL_NUMBER_FORMAT ",\n"
		"\t\t\t\"dimensions\": [\n"
		, st->id
		, st->name
		, st->type
		, st->family
		, st->title
		, st->priority
		, st->enabled
		, st->units
		, st->name, options?options:""
		, rrdset_type_name(st->chart_type)
		, st->counter
		, st->entries
		, rrdset_first_entry_t(st)
		, rrdset_last_slot(st)
		, rrdset_last_entry_t(st)
		, (now < rrdset_last_entry_t(st)) ? (time_t)0 : now - rrdset_last_entry_t(st)
		, st->update_every
		, st->isdetail
		, st->usec_since_last_update
		, st->collected_total
		, st->last_collected_total
		);

	unsigned long memory = st->memsize;

	RRDDIM *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
		web_buffer_increase(wb, 4096);

		memory += rd->memsize;

		web_buffer_printf(wb,
			"\t\t\t\t{\n"
			"\t\t\t\t\t\"id\": \"%s\",\n"
			"\t\t\t\t\t\"name\": \"%s\",\n"
			"\t\t\t\t\t\"entries\": %ld,\n"
			"\t\t\t\t\t\"isHidden\": %d,\n"
			"\t\t\t\t\t\"algorithm\": \"%s\",\n"
			"\t\t\t\t\t\"multiplier\": %ld,\n"
			"\t\t\t\t\t\"divisor\": %ld,\n"
			"\t\t\t\t\t\"last_entry_t\": %lu,\n"
			"\t\t\t\t\t\"collected_value\": " COLLECTED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"calculated_value\": " CALCULATED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"last_collected_value\": " COLLECTED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"last_calculated_value\": " CALCULATED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"memory\": %lu\n"
			"\t\t\t\t}%s\n"
			, rd->id
			, rd->name
			, rd->entries
			, rd->hidden
			, rrddim_algorithm_name(rd->algorithm)
			, rd->multiplier
			, rd->divisor
			, rd->last_collected_time.tv_sec
			, rd->collected_value
			, rd->calculated_value
			, rd->last_collected_value
			, rd->last_calculated_value
			, rd->memsize
			, rd->next?",":""
			);
	}

	web_buffer_printf(wb,
		"\t\t\t],\n"
		"\t\t\t\"memory\" : %lu\n"
		"\t\t}"
		, memory
		);

	pthread_rwlock_unlock(&st->rwlock);
	return memory;
}

#define RRD_GRAPH_JSON_HEADER "{\n\t\"charts\": [\n"
#define RRD_GRAPH_JSON_FOOTER "\n\t]\n}\n"

void rrd_stats_graph_json(RRDSET *st, char *options, struct web_buffer *wb)
{
	web_buffer_increase(wb, 2048);

	web_buffer_printf(wb, RRD_GRAPH_JSON_HEADER);
	rrd_stats_one_json(st, options, wb);
	web_buffer_printf(wb, RRD_GRAPH_JSON_FOOTER);
}

void rrd_stats_all_json(struct web_buffer *wb)
{
	web_buffer_increase(wb, 2048);

	unsigned long memory = 0;
	long c;
	RRDSET *st;

	web_buffer_printf(wb, RRD_GRAPH_JSON_HEADER);

	pthread_rwlock_rdlock(&rrdset_root_rwlock);
	for(st = rrdset_root, c = 0; st ; st = st->next) {
		if(st->enabled) {
			if(c) web_buffer_printf(wb, "%s", ",\n");
			memory += rrd_stats_one_json(st, NULL, wb);
			c++;
		}
	}
	pthread_rwlock_unlock(&rrdset_root_rwlock);
	
	web_buffer_printf(wb, "\n\t],\n"
		"\t\"hostname\": \"%s\",\n"
		"\t\"update_every\": %d,\n"
		"\t\"history\": %d,\n"
		"\t\"memory\": %lu\n"
		"}\n"
		, hostname
		, rrd_update_every
		, rrd_default_history_entries
		, memory
		);
}

unsigned long rrd_stats_json(int type, RRDSET *st, struct web_buffer *wb, int entries_to_show, int group, int group_method, time_t after, time_t before, int only_non_zero)
{
	int c;
	pthread_rwlock_rdlock(&st->rwlock);


	// -------------------------------------------------------------------------
	// switch from JSON to google JSON
	
	char kq[2] = "\"";
	char sq[2] = "\"";
	switch(type) {
		case DATASOURCE_GOOGLE_JSON:
		case DATASOURCE_GOOGLE_JSONP:
			kq[0] = '\0';
			sq[0] = '\'';
			break;

		case DATASOURCE_JSON:
		default:
			break;
	}


	// -------------------------------------------------------------------------
	// validate the parameters
	
	if(entries_to_show < 1) entries_to_show = 1;
	if(group < 1) group = 1;
	
	// find the oldest entry of the round-robin
	long max_entries_init = (st->counter < (unsigned long)st->entries) ? st->counter : (unsigned long)st->entries;
	
	time_t time_init = rrdset_last_entry_t(st);
	
	if(before == 0 || before > time_init) before = time_init;
	if(after  == 0) after = rrdset_first_entry_t(st);


	// ---

	// our return value (the last timestamp printed)
	// this is required to detect re-transmit in google JSONP
	time_t last_timestamp = 0;			


	// -------------------------------------------------------------------------
	// find how many dimensions we have
	
	int dimensions = 0;
	RRDDIM *rd;
	for( rd = st->dimensions ; rd ; rd = rd->next) dimensions++;
	if(!dimensions) {
		pthread_rwlock_unlock(&st->rwlock);
		web_buffer_printf(wb, "No dimensions yet.");
		return 0;
	}

	
	// -------------------------------------------------------------------------
	// prepare various strings, to speed up the loop
	
	char overflow_annotation[201]; snprintf(overflow_annotation, 200, ",{%sv%s:%sRESET OR OVERFLOW%s},{%sv%s:%sThe counters have been wrapped.%s}", kq, kq, sq, sq, kq, kq, sq, sq);
	char normal_annotation[201];   snprintf(normal_annotation,   200, ",{%sv%s:null},{%sv%s:null}", kq, kq, kq, kq);
	char pre_date[51];             snprintf(pre_date,             50, "		{%sc%s:[{%sv%s:%s", kq, kq, kq, kq, sq);
	char post_date[21];            snprintf(post_date,            20, "%s}", sq);
	char pre_value[21];            snprintf(pre_value,            20, ",{%sv%s:", kq, kq);
	char post_value[21];           snprintf(post_value,           20, "}");


	// -------------------------------------------------------------------------
	// checks for debuging
	
	if(st->debug) {
		debug(D_RRD_STATS, "%s first_entry_t = %lu, last_entry_t = %lu, duration = %lu, after = %lu, before = %lu, duration = %lu, entries_to_show = %lu, group = %lu, max_entries = %ld"
			, st->id
			, rrdset_first_entry_t(st)
			, rrdset_last_entry_t(st)
			, rrdset_last_entry_t(st) - rrdset_first_entry_t(st)
			, after
			, before
			, before - after
			, entries_to_show
			, group
			, max_entries_init
			);

		if(before < after)
			debug(D_RRD_STATS, "WARNING: %s The newest value in the database (%lu) is earlier than the oldest (%lu)", st->name, before, after);

		if((before - after) > st->entries * st->update_every)
			debug(D_RRD_STATS, "WARNING: %s The time difference between the oldest and the newest entries (%lu) is higher than the capacity of the database (%lu)", st->name, before - after, st->entries * st->update_every);
	}


	// -------------------------------------------------------------------------
	// temp arrays for keeping values per dimension
	
	calculated_number group_values[dimensions]; // keep sums when grouping
	int               print_hidden[dimensions]; // keep hidden flags
	int               found_non_zero[dimensions];
	int               found_non_existing[dimensions];

	// initialize them
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
		group_values[c] = 0;
		print_hidden[c] = rd->hidden;
		found_non_zero[c] = 0;
		found_non_existing[c] = 0;
	}


	// error("OLD: points=%d after=%d before=%d group=%d, duration=%d", entries_to_show, before - (st->update_every * group * entries_to_show), before, group, before - after + 1);
	// rrd2array(st, entries_to_show, before - (st->update_every * group * entries_to_show), before, group_method, only_non_zero);

	// -------------------------------------------------------------------------
	// remove dimensions that contain only zeros

	int max_loop = 1;
	if(only_non_zero) max_loop = 2;

	for(; max_loop ; max_loop--) {

		// -------------------------------------------------------------------------
		// print the JSON header
		
		web_buffer_printf(wb, "{\n	%scols%s:\n	[\n", kq, kq);
		web_buffer_printf(wb, "		{%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
		web_buffer_printf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
		web_buffer_printf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);

		// print the header for each dimension
		// and update the print_hidden array for the dimensions that should be hidden
		int pc = 0;
		for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
			if(!print_hidden[c]) {
				pc++;
				web_buffer_printf(wb, ",\n		{%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, rd->name, sq, kq, kq, sq, sq, kq, kq, sq, sq);
			}
		}
		if(!pc) {
			web_buffer_printf(wb, ",\n		{%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, "no data", sq, kq, kq, sq, sq, kq, kq, sq, sq);
		}

		// print the begin of row data
		web_buffer_printf(wb, "\n	],\n	%srows%s:\n	[\n", kq, kq);


		// -------------------------------------------------------------------------
		// the main loop

		int annotate_reset = 0;
		int annotation_count = 0;
		
		// to allow grouping on the same values, we need a pad
		long pad = before % group;

		// the minimum line length we expect
		int line_size = 4096 + (dimensions * 200);

		time_t now = time_init;
		long max_entries = max_entries_init;

		long t;

		long count = 0, printed = 0, group_count = 0;
		last_timestamp = 0;

		long expected_to_start_at_slot = rrdset_time2slot(st, before);

		for(t = rrdset_last_slot(st); max_entries ; now -= st->update_every, t--, max_entries--) {
			if(t < 0) t = st->entries - 1;

			int print_this = 0;

			if(st->debug) {
				debug(D_RRD_STATS, "%s t = %ld, count = %ld, group_count = %ld, printed = %ld, now = %lu, %s %s"
					, st->id
					, t
					, count + 1
					, group_count + 1
					, printed
					, now
					, (((count + 1 - pad) % group) == 0)?"PRINT":"  -  "
					, (now >= after && now <= before)?"RANGE":"  -  "
					);
			}

			// make sure we return data in the proper time range
			if(now < after || now > before) continue;

			if(expected_to_start_at_slot != -999999) {
				error("%s: Expected to start on slot %ld, started on %ld, %s", st->id, expected_to_start_at_slot, t, (t == expected_to_start_at_slot)?"OK":"ERROR");
				expected_to_start_at_slot = -999999;
			}

			count++;
			group_count++;

			// check if we have to print this now
			if(((count - pad) % group) == 0) {
				if(printed >= entries_to_show) {
					// debug(D_RRD_STATS, "Already printed all rows. Stopping.");
					break;
				}
				
				if(group_count != group) {
					// this is an incomplete group, skip it.
					for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
						group_values[c] = 0;
						found_non_existing[c] = 0;
					}
						
					group_count = 0;
					continue;
				}

				// check if we may exceed the buffer provided
				web_buffer_increase(wb, line_size);

				// generate the local date time
				struct tm *tm = localtime(&now);
				if(!tm) { error("localtime() failed."); continue; }
				if(now > last_timestamp) last_timestamp = now;

				if(printed) web_buffer_strcpy(wb, "]},\n");
				web_buffer_strcpy(wb, pre_date);
				web_buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
				web_buffer_strcpy(wb, post_date);

				print_this = 1;
			}

			// do the calculations
			for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
				storage_number n = rd->values[t];
				calculated_number value = unpack_storage_number(n);
				
				if(!does_storage_number_exist(n)) {
					value = 0.0;
					found_non_existing[c]++;
				}
				if(did_storage_number_reset(n)) annotate_reset = 1;

				switch(group_method) {
					case GROUP_MAX:
						if(abs(value) > abs(group_values[c])) group_values[c] = value;
						break;

					case GROUP_SUM:
						group_values[c] += value;
						break;

					default:
					case GROUP_AVERAGE:
						group_values[c] += value;
						if(print_this) group_values[c] /= ( group_count - found_non_existing[c] );
						break;
				}
			}

			if(print_this) {
				if(annotate_reset) {
					annotation_count++;
					web_buffer_strcpy(wb, overflow_annotation);
					annotate_reset = 0;
				}
				else
					web_buffer_strcpy(wb, normal_annotation);

				pc = 0;
				for(c = 0 ; c < dimensions ; c++) {
					if(found_non_existing[c] == group_count) {
						// all entries are non-existing
						pc++;
						web_buffer_strcpy(wb, pre_value);
						web_buffer_strcpy(wb, "null");
						web_buffer_strcpy(wb, post_value);
					}
					else if(!print_hidden[c]) {
						pc++;
						web_buffer_strcpy(wb, pre_value);
						web_buffer_rrd_value(wb, group_values[c]);
						web_buffer_strcpy(wb, post_value);

						if(group_values[c]) found_non_zero[c]++;
					}

					// reset them for the next loop
					group_values[c] = 0;
					found_non_existing[c] = 0;
				}

				// if all dimensions are hidden, print a null
				if(!pc) {
					web_buffer_strcpy(wb, pre_value);
					web_buffer_strcpy(wb, "null");
					web_buffer_strcpy(wb, post_value);
				}

				printed++;
				group_count = 0;
			}
		}

		if(printed) web_buffer_printf(wb, "]}");
		web_buffer_printf(wb, "\n	]\n}\n");

		if(only_non_zero && max_loop > 1) {
			int changed = 0;
			for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
				group_values[c] = 0;
				found_non_existing[c] = 0;

				if(!print_hidden[c] && !found_non_zero[c]) {
					changed = 1;
					print_hidden[c] = 1;
				}
			}

			if(changed) web_buffer_reset(wb);
			else break;
		}
		else break;
		
	} // max_loop

	debug(D_RRD_STATS, "RRD_STATS_JSON: %s total %ld bytes", st->name, wb->bytes);

	pthread_rwlock_unlock(&st->rwlock);
	return last_timestamp;
}


// ----------------------------------------------------------------------------

// RRDR options
#define RRDR_EMPTY  	0x01
#define RRDR_RESET  	0x02
#define RRDR_HIDDEN 	0x04
#define RRDR_NONZERO 	0x08


typedef struct rrdresult {
	RRDSET *st;			// the chart this result refers to

	int d;					// the number of dimensions
	int n;					// the number of values in the arrays

	time_t *t;				// array of timestamps
	calculated_number *v;	// array n x d values
	uint8_t *o;				// array n x d options

	int c;					// current line (n)

	int has_st_lock;		// if st is read locked by us
} RRDR;

inline static calculated_number *rrdr_line_values(RRDR *r)
{
	return &r->v[ r->c * r->d ];
}

inline static uint8_t *rrdr_line_options(RRDR *r)
{
	return &r->o[ r->c * r->d ];
}

inline static int rrdr_line_next(RRDR *r, time_t t)
{
	// save the time
	r->t[r->c] = t;

	r->c++;
	if(unlikely(r->c >= r->n)) {
		r->c--;
		return 0;
	}

	return 1;
}

inline static void rrdr_lock_rrdset(RRDR *r) {
	if(unlikely(!r)) {
		error("NULL value given!");
		return;
	}

	pthread_rwlock_rdlock(&r->st->rwlock);
	r->has_st_lock = 1;
}

inline static void rrdr_unlock_rrdset(RRDR *r) {
	if(unlikely(!r)) {
		error("NULL value given!");
		return;
	}

	if(likely(r->has_st_lock)) {
		pthread_rwlock_unlock(&r->st->rwlock);
		r->has_st_lock = 0;
	}
}

inline static void rrdr_free(RRDR *r)
{
	if(unlikely(!r)) {
		error("NULL value given!");
		return;
	}

	rrdr_unlock_rrdset(r);
	if(likely(r->t)) free(r->t);
	if(likely(r->v)) free(r->v);
	if(likely(r->o)) free(r->o);
	free(r);
}

static RRDR *rrdr_create(RRDSET *st, int n)
{
	if(unlikely(!st)) {
		error("NULL value given!");
		return NULL;
	}

	RRDR *r = calloc(1, sizeof(RRDR));
	if(unlikely(!r)) goto cleanup;

	r->st = st;

	rrdr_lock_rrdset(r);

	RRDDIM *rd;
	for(rd = st->dimensions ; rd ; rd = rd->next) r->d++;

	r->n = n;
	r->t = malloc(sizeof(time_t) * n);
	if(unlikely(!r->t)) goto cleanup;

	r->t = malloc(sizeof(time_t) * n);
	if(unlikely(!r->t)) goto cleanup;

	r->v = malloc(sizeof(calculated_number) * n * r->d);
	if(unlikely(!r->v)) goto cleanup;

	r->o = malloc(sizeof(calculated_number) * n * r->d);
	if(unlikely(!r->o)) goto cleanup;

	return r;

cleanup:
	error("Cannot allocate memory");
	if(likely(r)) rrdr_free(r);
	return NULL;
}

RRDR *rrd2rrdr(RRDSET *st, long points, time_t after, time_t before, int group_method)
{
	time_t first_entry_t = rrdset_first_entry_t(st);
	time_t last_entry_t = rrdset_last_entry_t(st);

	// allow relative for before and after
	if(before <= st->update_every * st->entries) before = last_entry_t + before;
	if(after <= st->update_every * st->entries) after = last_entry_t + after;

	// make sure they are within our timeframe
	if(before > last_entry_t) before = last_entry_t;
	if(before < first_entry_t) before = first_entry_t;

	if(after > last_entry_t) after = last_entry_t;
	if(after < first_entry_t) after = first_entry_t;

	// check if they are upside down
	if(after > before) {
		time_t t = before;
		before = after;
		after = t;
	}

	// the duration of the chart
	time_t duration = before - after;
	if(duration <= 0) return NULL;

	// check the required points
	if(points <= 0) points = duration;

	// calculate proper grouping of source data
	int group = duration / points;
	if(group <= 0) group = 1;
	if(duration / group > points) group++;

	// align timestamps to group
	before -= before % group;
	after -= after % group;
	duration = before - after;

	// error("NEW: points=%d after=%d before=%d group=%d, duration=%d", points, after, before, group, duration);

	// Now we have:
	// before = the end time of the calculation
	// after = the start time of the calculation
	// duration = the duration of the calculation
	// group = the number of source points to aggregate / group together
	// method = the method of grouping source points
	// points = the number of points to generate


	// -------------------------------------------------------------------------
	// initialize our result set

	RRDR *r = rrdr_create(st, points);
	if(!r) return NULL;
	if(!r->d) {
		rrdr_free(r);
		return NULL;
	}

	// find how many dimensions we have
	long dimensions = r->d;


	// -------------------------------------------------------------------------

	// how many entries can we use from the source data?
	long max_entries = (st->counter < (unsigned long)st->entries) ? st->counter : (unsigned long)st->entries;


	// -------------------------------------------------------------------------
	// checks for debugging

	if(st->debug) {
		debug(D_RRD_STATS, "%s first_entry_t = %lu, last_entry_t = %lu, duration = %lu, after = %lu, before = %lu, duration = %lu, entries_to_show = %lu, group = %lu, max_entries = %ld"
			, st->id
			, first_entry_t
			, last_entry_t
			, last_entry_t - first_entry_t
			, after
			, before
			, duration
			, points
			, group
			, max_entries
			);
	}


	// -------------------------------------------------------------------------
	// temp arrays for keeping values per dimension

	calculated_number 	group_values[dimensions]; // keep sums when grouping
	long 				group_counts[dimensions]; // keep the number of values added to group_values
	uint8_t 			group_options[dimensions];
	uint8_t				found_non_zero[dimensions];


	// initialize them
	RRDDIM *rd;
	long c;
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
		group_values[c] = 0;
		group_counts[c] = 0;
		group_options[c] = 0;
		found_non_zero[c] = 0;
	}


	// -------------------------------------------------------------------------
	// the main loop

	int debug = st->debug;

	time_t 	now = last_entry_t,
			dt = st->update_every;

	long 	t = rrdset_time2slot(st, before), // rrdset_last_slot(st),
			count = 0,
			added = 0,
			group_count = group,
			add_this = 0;

	for( ; max_entries ; now -= dt, t--, max_entries--) {
		if(unlikely(t < 0)) c = st->entries - 1;

		if(unlikely(debug)) debug(D_RRD_STATS, "%s c = %ld, count = %ld, group_count = %ld, added = %ld, now = %lu, %s %s"
				, st->id
				, t
				, count + 1
				, group_count + 1
				, added
				, now
				, (group_count == 0)?"PRINT":"  -  "
				, (now >= after && now <= before)?"RANGE":"  -  "
				);

		// make sure we return data in the proper time range
		if(unlikely(now < after || now > before)) continue;

		count++;
		group_count++;

		if(unlikely(group_count == group)) {
			if(unlikely(added >= points)) break;
			add_this = 1;
		}

		// do the calculations
		for(rd = st->dimensions, c = 0 ; likely(rd && c < dimensions) ; rd = rd->next, c++) {
			storage_number n = rd->values[t];
			if(unlikely(!does_storage_number_exist(n))) continue;

			group_counts[c]++;

			calculated_number value = unpack_storage_number(n);
			if(value != 0.0) {
				group_options[c] |= RRDR_NONZERO;
				found_non_zero[c] = 1;
			}

			if(unlikely(did_storage_number_reset(n)))
				group_options[c] |= RRDR_RESET;

			switch(group_method) {
				case GROUP_MAX:
					if(unlikely(abs(value) > abs(group_values[c])))
						group_values[c] = value;
					break;

				default:
				case GROUP_SUM:
				case GROUP_AVERAGE:
					group_values[c] += value;
					break;

					group_values[c] += value;
					break;
			}
		}

		// added it
		if(unlikely(add_this)) {
			calculated_number *cn = rrdr_line_values(r);
			uint8_t *co = rrdr_line_options(r);

			for(rd = st->dimensions, c = 0 ; likely(rd && c < dimensions) ; rd = rd->next, c++) {
				if(rd->hidden) group_options[c] |= RRDR_HIDDEN;

				co[c] = group_options[c];

				if(group_counts[c] == 0) {
					cn[c] = 0.0;
					co[c] |= RRDR_EMPTY;
				}
				else if(unlikely(group_method == GROUP_AVERAGE)) {
					cn[c] = group_values[c] / group_counts[c];
				}
				else {
					cn[c] = group_values[c];
				}

				// reset them for the next loop
				group_values[c] = 0;
				group_counts[c] = 0;
				group_options[c] = 0;
			}

			added++;
			group_count = 0;
			add_this = 0;
		}

		rrdr_line_next(r, now);
	}

	return r;
}


/*
unsigned long rrdset2json(int type, RRDSET *st, struct web_buffer *wb, int entries_to_show, int group, int group_method, time_t after, time_t before, int only_non_zero) {

	if(!st->dimensions) {
		web_buffer_printf(wb, "No dimensions yet.");
		return 0;
	}

	int c;
	pthread_rwlock_rdlock(&st->rwlock);

	// -------------------------------------------------------------------------
	// validate the parameters

	if(entries_to_show < 1) entries_to_show = 1;
	if(group < 1) group = 1;

	// make sure current_entry is within limits
	long current_entry = (long)st->current_entry - (long)1;
	if(current_entry < 0) current_entry = 0;
	else if(current_entry >= st->entries) current_entry = st->entries - 1;

	// find the oldest entry of the round-robin
	long max_entries = (st->counter < (unsigned long)st->entries) ? st->counter : (unsigned long)st->entries;
	if(entries_to_show > max_entries) entries_to_show = max_entries;

	if(before == 0 || before > st->last_updated.tv_sec) before = st->last_updated.tv_sec;
	if(after  == 0 || after  < rrdset_first_entry_t(st)) after = rrdset_first_entry_t(st);

	if(entries_to_show > (before - after) / st->update_every / group) entries_to_show = (before - after) / st->update_every / group;

	// -------------------------------------------------------------------------
	// checks for debuging

	if(st->debug) {
		debug(D_RRD_STATS, "%s first_entry_t = %lu, last_entry_t = %lu, duration = %lu, after = %lu, before = %lu, duration = %lu, entries_to_show = %lu, group = %lu, max_entries = %ld"
			, st->id
			, rrdset_first_entry_t(st)
			, st->last_updated.tv_sec
			, st->last_updated.tv_sec - rrdset_first_entry_t(st)
			, after
			, before
			, before - after
			, entries_to_show
			, group
			, max_entries
			);

		if(before < after)
			debug(D_RRD_STATS, "WARNING: %s The newest value in the database (%lu) is earlier than the oldest (%lu)", st->name, before, after);

		if((before - after) > st->entries * st->update_every)
			debug(D_RRD_STATS, "WARNING: %s The time difference between the oldest and the newest entries (%lu) is higher than the capacity of the database (%lu)", st->name, before - after, st->entries * st->update_every);
	}

	// -------------------------------------------------------------------------

	int dimensions = 0, i, j;
	RRDDIM *rd;
	for( rd = st->dimensions ; rd ; rd = rd->next) dimensions++;

	uint32_t *annotations[dimensions];
	calculated_number *values[dimensions];
	int enabled[dimensions];

	for( rd = st->dimensions, i = 0 ; rd && i < dimensions ; rd = rd->next, i++) {
		annotations[i] = malloc(sizeof(uint32_t) * entries_to_show);
		if(!annotations[i]) fatal("Cannot allocate %z bytes of memory.", sizeof(uint32_t) * entries_to_show);

		values[i] = malloc(sizeof(calculated_number) * entries_to_show);
		if(!values[i]) fatal("Cannot allocate %z bytes of memory.", sizeof(calculated_number) * entries_to_show);

		calculated_number sum = rrddim2values(st, rd, values, annotations, entries_to_show, group, group_method, after, before);
		if(only_non_zero && sum == 0) enabled[i] = 0;
		else enabled[i] = 1;
	}

	switch(type) {
		case DATASOURCE_GOOGLE_JSON:
		case DATASOURCE_GOOGLE_JSONP:
			//rrdvalues2googlejson(st, rd, values, annotations, enabled, entries_to_show, );
			break;

		case DATASOURCE_JSON:
		default:
			//rrdvalues2json();
			break;
	}

	//return ???;
}
*/
