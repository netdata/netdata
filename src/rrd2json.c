#include <pthread.h>
#include <sys/time.h>

#include "log.h"
#include "common.h"
#include "rrd2json.h"

#define HOSTNAME_MAX 1024
char *hostname = "unknown";

unsigned long rrd_stats_one_json(RRDSET *st, char *options, struct web_buffer *wb)
{
	time_t now = time(NULL);
	web_buffer_increase(wb, 16384);

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
		, st->id, options?options:""
		, rrdset_type_name(st->chart_type)
		, st->counter
		, st->entries
		, rrdset_first_entry_t(st)
		, st->current_entry
		, st->last_updated.tv_sec
		, now - (st->last_updated.tv_sec > now) ? now : st->last_updated.tv_sec
		, st->update_every
		, st->isdetail
		, st->usec_since_last_update
		, st->collected_total
		, st->last_collected_total
		);

	unsigned long memory = st->memsize;

	RRDDIM *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
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
	web_buffer_increase(wb, 16384);

	web_buffer_printf(wb, RRD_GRAPH_JSON_HEADER);
	rrd_stats_one_json(st, options, wb);
	web_buffer_printf(wb, RRD_GRAPH_JSON_FOOTER);
}

void rrd_stats_all_json(struct web_buffer *wb)
{
	web_buffer_increase(wb, 1024);

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
	
	// make sure current_entry is within limits
	long current_entry = (long)st->current_entry - (long)1;
	if(current_entry < 0) current_entry = 0;
	else if(current_entry >= st->entries) current_entry = st->entries - 1;
	
	// find the oldest entry of the round-robin
	long max_entries_init = (st->counter < (unsigned long)st->entries) ? st->counter : (unsigned long)st->entries;
	
	time_t time_init = st->last_updated.tv_sec;
	
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
			, st->last_updated.tv_sec
			, st->last_updated.tv_sec - rrdset_first_entry_t(st)
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
	calculated_number print_values[dimensions]; // keep the final value to be printed
	int               print_hidden[dimensions]; // keep hidden flags
	int               found_non_zero[dimensions];
	int               found_non_existing[dimensions];

	// initialize them
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
		group_values[c] = print_values[c] = 0;
		print_hidden[c] = rd->hidden;
		found_non_zero[c] = 0;
		found_non_existing[c] = 0;
	}

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
		for(t = current_entry; max_entries ; now -= st->update_every, t--, max_entries--) {
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
