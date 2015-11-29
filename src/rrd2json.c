#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <pthread.h>
#include <sys/time.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "common.h"
#include "rrd2json.h"

#define HOSTNAME_MAX 1024
char *hostname = "unknown";

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb)
{
	pthread_rwlock_rdlock(&st->rwlock);

	buffer_sprintf(wb,
		"\t\t{\n"
		"\t\t\t\"id\": \"%s\",\n"
		"\t\t\t\"name\": \"%s\",\n"
		"\t\t\t\"type\": \"%s\",\n"
		"\t\t\t\"family\": \"%s\",\n"
		"\t\t\t\"title\": \"%s\",\n"
		"\t\t\t\"priority\": %ld,\n"
		"\t\t\t\"enabled\": %s,\n"
		"\t\t\t\"units\": \"%s\",\n"
		"\t\t\t\"data_url\": \"/api/v1/data?chart=%s\",\n"
		"\t\t\t\"chart_type\": \"%s\",\n"
		"\t\t\t\"duration\": %ld,\n"
		"\t\t\t\"first_entry\": %lu,\n"
		"\t\t\t\"last_entry\": %lu,\n"
		"\t\t\t\"update_every\": %d,\n"
		"\t\t\t\"dimensions\": {\n"
		, st->id
		, st->name
		, st->type
		, st->family
		, st->title
		, st->priority
		, st->enabled?"true":"false"
		, st->units
		, st->name
		, rrdset_type_name(st->chart_type)
		, st->entries * st->update_every
		, rrdset_first_entry_t(st)
		, rrdset_last_entry_t(st)
		, st->update_every
		);

	unsigned long memory = st->memsize;

	int c = 0;
	RRDDIM *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
		if(rd->flags & RRDDIM_FLAG_HIDDEN) continue;

		memory += rd->memsize;

		buffer_sprintf(wb,
			"%s"
			"\t\t\t\t\"%s\": { \"name\": \"%s\" }"
			, c?",\n":""
			, rd->id
			, rd->name
			);

		c++;
	}

	buffer_sprintf(wb,
		"\n\t\t\t}\n"
		"\t\t}"
		);

	pthread_rwlock_unlock(&st->rwlock);
}

void rrd_stats_api_v1_charts(BUFFER *wb)
{
	long c;
	RRDSET *st;

	buffer_sprintf(wb, "{\n"
		   "\t\"hostname\": \"%s\""
		",\n\t\"update_every\": %d"
		",\n\t\"history\": %d"
		",\n\t\"charts\": {"
		, hostname
		, rrd_update_every
		, rrd_default_history_entries
		);

	pthread_rwlock_rdlock(&rrdset_root_rwlock);
	for(st = rrdset_root, c = 0; st ; st = st->next) {
		if(st->enabled) {
			if(c) buffer_strcat(wb, ",");
			buffer_strcat(wb, "\n\t\t\"");
			buffer_strcat(wb, st->id);
			buffer_strcat(wb, "\": ");
			rrd_stats_api_v1_chart(st, wb);
			c++;
		}
	}
	pthread_rwlock_unlock(&rrdset_root_rwlock);

	buffer_strcat(wb, "\n\t}\n}\n");
}


unsigned long rrd_stats_one_json(RRDSET *st, char *options, BUFFER *wb)
{
	time_t now = time(NULL);

	pthread_rwlock_rdlock(&st->rwlock);

	buffer_sprintf(wb,
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

		memory += rd->memsize;

		buffer_sprintf(wb,
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
			, (rd->flags & RRDDIM_FLAG_HIDDEN)?1:0
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

	buffer_sprintf(wb,
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

void rrd_stats_graph_json(RRDSET *st, char *options, BUFFER *wb)
{
	buffer_strcat(wb, RRD_GRAPH_JSON_HEADER);
	rrd_stats_one_json(st, options, wb);
	buffer_strcat(wb, RRD_GRAPH_JSON_FOOTER);
}

void rrd_stats_all_json(BUFFER *wb)
{
	unsigned long memory = 0;
	long c;
	RRDSET *st;

	buffer_strcat(wb, RRD_GRAPH_JSON_HEADER);

	pthread_rwlock_rdlock(&rrdset_root_rwlock);
	for(st = rrdset_root, c = 0; st ; st = st->next) {
		if(st->enabled) {
			if(c) buffer_strcat(wb, ",\n");
			memory += rrd_stats_one_json(st, NULL, wb);
			c++;
		}
	}
	pthread_rwlock_unlock(&rrdset_root_rwlock);
	
	buffer_sprintf(wb, "\n\t],\n"
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



// ----------------------------------------------------------------------------

// RRDR options
#define RRDR_EMPTY  	0x01 // the dimension contains / the value is empty (null)
#define RRDR_RESET  	0x02 // the dimension contains / the value is reset
#define RRDR_HIDDEN 	0x04 // the dimension contains / the value is hidden
#define RRDR_NONZERO 	0x08 // the dimension contains / the value is non-zero


typedef struct rrdresult {
	RRDSET *st;			// the chart this result refers to

	int d;					// the number of dimensions
	int n;					// the number of values in the arrays

	uint8_t *od;			// the options for the dimensions

	time_t *t;				// array of n timestamps
	calculated_number *v;	// array n x d values
	uint8_t *o;				// array n x d options

	int c;					// current line ( -1 ~ n ), ( -1 = none, use rrdr_rows() to get number of rows )

	int has_st_lock;		// if st is read locked by us
} RRDR;

#define rrdr_rows(r) ((r)->c + 1)

/*
static void rrdr_dump(RRDR *r)
{
	long c, i;
	RRDDIM *d;

	fprintf(stderr, "\nCHART %s (%s)\n", r->st->id, r->st->name);

	for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
		fprintf(stderr, "DIMENSION %s (%s), %s%s%s%s\n"
				, d->id
				, d->name
				, (r->od[c] & RRDR_EMPTY)?"EMPTY ":""
				, (r->od[c] & RRDR_RESET)?"RESET ":""
				, (r->od[c] & RRDR_HIDDEN)?"HIDDEN ":""
				, (r->od[c] & RRDR_NONZERO)?"NONZERO ":""
				);
	}

	if(r->c < 0) {
		fprintf(stderr, "RRDR does not have any values in it.\n");
		return;
	}

	fprintf(stderr, "RRDR includes %d values in it:\n", r->c + 1);

	// for each line in the array
	for(i = 0; i <= r->c ;i++) {
		calculated_number *cn = &r->v[ i * r->d ];
		uint8_t *co = &r->o[ i * r->d ];

		// print the id and the timestamp of the line
		fprintf(stderr, "%ld %ld ", i + 1, r->t[i]);

		// for each dimension
		for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
			if(unlikely(r->od[c] & RRDR_HIDDEN)) continue;
			if(unlikely(!(r->od[c] & RRDR_NONZERO))) continue;

			if(co[c] & RRDR_EMPTY)
				fprintf(stderr, "null ");
			else
				fprintf(stderr, CALCULATED_NUMBER_FORMAT " %s%s%s%s "
					, cn[c]
					, (co[c] & RRDR_EMPTY)?"E":" "
					, (co[c] & RRDR_RESET)?"R":" "
					, (co[c] & RRDR_HIDDEN)?"H":" "
					, (co[c] & RRDR_NONZERO)?"N":" "
					);
		}

		fprintf(stderr, "\n");
	}
}
*/

void rrdr_disable_not_selected_dimensions(RRDR *r, const char *dims)
{
	char b[strlen(dims) + 1];
	char *o = b, *tok;
	strcpy(o, dims);

	long c;
	RRDDIM *d;

	// disable all of them
	for(c = 0, d = r->st->dimensions; d ;c++, d = d->next)
		r->od[c] |= RRDR_HIDDEN;

	while(o && *o && (tok = mystrsep(&o, ", |"))) {
		if(!*tok) continue;

		// find it and enable it
		for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
			if(!strcmp(d->name, tok)) {
				r->od[c] &= ~RRDR_HIDDEN;
			}
		}
	}
}

#define JSON_DATES_JS 1
#define JSON_DATES_TIMESTAMP 2

static void rrdr2json(RRDR *r, BUFFER *wb, uint32_t options, int google)
{
	int annotations = 0, dates = JSON_DATES_JS;
	char kq[2] = "",					// key quote
		sq[2] = "",						// string quote
		pre_label[101] = "",			// before each label
		post_label[101] = "",			// after each label
		pre_date[101] = "",				// the beginning of line, to the date
		post_date[101] = "",			// closing the date
		pre_value[101] = "",			// before each value
		post_value[101] = "",			// after each value
		post_line[101] = "",			// at the end of each row
		normal_annotation[201] = "",	// default row annotation
		overflow_annotation[201] = "",	// overflow row annotation
		data_begin[101] = "",			// between labels and values
		finish[101] = "";				// at the end of everything

	if(google) {
		dates = JSON_DATES_JS;
		kq[0] = '\0';
		sq[0] = '\'';
		annotations = 1;
		snprintf(pre_date,   100, "		{%sc%s:[{%sv%s:%s", kq, kq, kq, kq, sq);
		snprintf(post_date,  100, "%s}", sq);
		snprintf(pre_label,  100, ",\n		{%sid%s:%s%s,%slabel%s:%s", kq, kq, sq, sq, kq, kq, sq);
		snprintf(post_label, 100, "%s,%spattern%s:%s%s,%stype%s:%snumber%s}", sq, kq, kq, sq, sq, kq, kq, sq, sq);
		snprintf(pre_value,  100, ",{%sv%s:", kq, kq);
		snprintf(post_value, 100, "}");
		snprintf(post_line,  100, "]}");
		snprintf(data_begin, 100, "\n	],\n	%srows%s:\n	[\n", kq, kq);
		snprintf(finish,     100, "\n	]\n}\n");

		snprintf(overflow_annotation, 200, ",{%sv%s:%sRESET OR OVERFLOW%s},{%sv%s:%sThe counters have been wrapped.%s}", kq, kq, sq, sq, kq, kq, sq, sq);
		snprintf(normal_annotation,   200, ",{%sv%s:null},{%sv%s:null}", kq, kq, kq, kq);

		buffer_sprintf(wb, "{\n	%scols%s:\n	[\n", kq, kq);
		buffer_sprintf(wb, "		{%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
		buffer_sprintf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
		buffer_sprintf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
	}
	else {
		if((options & RRDR_OPTION_SECONDS) || (options & RRDR_OPTION_MILLISECONDS)) {
			dates = JSON_DATES_TIMESTAMP;
			snprintf(pre_date,   100, "		[");
		}
		else {
			dates = JSON_DATES_JS;
			snprintf(pre_date,   100, "		[new ");
		}
		kq[0] = '"';
		sq[0] = '"';
		snprintf(pre_label,  100, ", \"");
		snprintf(post_label, 100, "\"");
		snprintf(pre_value,  100, ", ");
		snprintf(post_line,  100, "]");
		snprintf(data_begin, 100, "],\n	%sdata%s:\n	[\n", kq, kq);
		snprintf(finish,     100, "\n	]\n}\n");

		buffer_sprintf(wb, "{\n	%slabels%s: [", kq, kq);
		buffer_sprintf(wb, "%stime%s", sq, sq);
	}

	// -------------------------------------------------------------------------
	// print the JSON header

	long c, i;
	RRDDIM *rd;

	// print the csv header
	for(c = 0, i = 0, rd = r->st->dimensions; rd ;c++, rd = rd->next) {
		if(unlikely(r->od[c] & RRDR_HIDDEN)) continue;
		if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_NONZERO))) continue;

		buffer_strcat(wb, pre_label);
		buffer_strcat(wb, rd->name);
		buffer_strcat(wb, post_label);
		i++;
	}
	if(!i) {
		buffer_strcat(wb, pre_label);
		buffer_strcat(wb, "no data");
		buffer_strcat(wb, post_label);
	}

	// print the begin of row data
	buffer_strcat(wb, data_begin);

	// if all dimensions are hidden, print a null
	if(!i) {
		buffer_strcat(wb, pre_value);
		if(options & RRDR_OPTION_NULL2ZERO)
			buffer_strcat(wb, "0");
		else
			buffer_strcat(wb, "null");
		buffer_strcat(wb, post_value);
	}

	long start = 0, end = rrdr_rows(r), step = 1;
	if((options & RRDR_OPTION_REVERSED)) {
		start = rrdr_rows(r) - 1;
		end = -1;
		step = -1;
	}

	// for each line in the array
	for(i = start; i != end ;i += step) {
		calculated_number *cn = &r->v[ i * r->d ];
		uint8_t *co = &r->o[ i * r->d ];

		time_t now = r->t[i];

		if(dates == JSON_DATES_JS) {
			// generate the local date time
			struct tm *tm = localtime(&now);
			if(!tm) { error("localtime() failed."); continue; }

			if(likely(i != start)) buffer_strcat(wb, ",\n");
			buffer_strcat(wb, pre_date);
			buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
			buffer_strcat(wb, post_date);

			if(annotations) {
				if(co[c] & RRDR_RESET)
					buffer_strcat(wb, overflow_annotation);
				else
					buffer_strcat(wb, normal_annotation);
			}
		}
		else {
			// print the timestamp of the line
			if(likely(i != start)) buffer_strcat(wb, ",\n");
			buffer_strcat(wb, pre_date);
			buffer_rrd_value(wb, (calculated_number)r->t[i]);
			// in ms
			if(options & RRDR_OPTION_MILLISECONDS) buffer_strcat(wb, "000");
			buffer_strcat(wb, post_date);
		}

		// for each dimension
		for(c = 0, rd = r->st->dimensions; rd ;c++, rd = rd->next) {
			if(unlikely(r->od[c] & RRDR_HIDDEN)) continue;
			if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_NONZERO))) continue;

			calculated_number n = cn[c];

			if(co[c] & RRDR_EMPTY) {
				buffer_strcat(wb, pre_value);
				if(options & RRDR_OPTION_NULL2ZERO)
					buffer_strcat(wb, "0");
				else
					buffer_strcat(wb, "null");
				buffer_strcat(wb, post_value);
			}
			else if((options & RRDR_OPTION_ABSOLUTE)) {
				buffer_strcat(wb, pre_value);
				buffer_rrd_value(wb, (n<0)?-n:n);
				buffer_strcat(wb, post_value);
			}
			else {
				buffer_strcat(wb, pre_value);
				buffer_rrd_value(wb, n);
				buffer_strcat(wb, post_value);
			}
		}

		buffer_strcat(wb, post_line);
	}

	buffer_strcat(wb, finish);
}

static void rrdr2csv(RRDR *r, BUFFER *wb, uint32_t options, const char *startline, const char *separator, const char *endline)
{
	long c, i;
	RRDDIM *d;

	// print the csv header
	for(c = 0, i = 0, d = r->st->dimensions; d ;c++, d = d->next) {
		if(unlikely(r->od[c] & RRDR_HIDDEN)) continue;
		if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_NONZERO))) continue;

		if(!i) {
			buffer_strcat(wb, startline);
			buffer_strcat(wb, "time");
		}
		buffer_strcat(wb, separator);
		buffer_strcat(wb, d->name);
		i++;
	}
	buffer_strcat(wb, endline);

	if(!i) {
		// no dimensions present
		return;
	}

	long start = 0, end = rrdr_rows(r), step = 1;
	if((options & RRDR_OPTION_REVERSED)) {
		start = rrdr_rows(r) - 1;
		end = -1;
		step = -1;
	}

	// for each line in the array
	for(i = start; i != end ;i += step) {
		calculated_number *cn = &r->v[ i * r->d ];
		uint8_t *co = &r->o[ i * r->d ];

		buffer_strcat(wb, startline);

		time_t now = r->t[i];

		if((options & RRDR_OPTION_SECONDS) || (options & RRDR_OPTION_MILLISECONDS)) {
			// print the timestamp of the line
			buffer_rrd_value(wb, (calculated_number)now);
			// in ms
			if(options & RRDR_OPTION_MILLISECONDS) buffer_strcat(wb, "000");
		}
		else {
			// generate the local date time
			struct tm *tm = localtime(&now);
			if(!tm) { error("localtime() failed."); continue; }
			buffer_date(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
		}

		// for each dimension
		for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
			if(unlikely(r->od[c] & RRDR_HIDDEN)) continue;
			if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_NONZERO))) continue;

			buffer_strcat(wb, separator);

			calculated_number n = cn[c];

			if(co[c] & RRDR_EMPTY) {
				if(options & RRDR_OPTION_NULL2ZERO)
					buffer_strcat(wb, "0");
				else
					buffer_strcat(wb, "null");
			}
			else if((options & RRDR_OPTION_ABSOLUTE))
				buffer_rrd_value(wb, (n<0)?-n:n);
			else
				buffer_rrd_value(wb, n);
		}

		buffer_strcat(wb, endline);
	}
}

static void rrdr2ssv(RRDR *r, BUFFER *out, uint32_t options, const char *prefix, const char *separator, const char *suffix)
{
	long c, i;
	RRDDIM *d;

	buffer_strcat(out, prefix);
	long start = 0, end = rrdr_rows(r), step = 1;
	if((options & RRDR_OPTION_REVERSED)) {
		start = rrdr_rows(r) - 1;
		end = -1;
		step = -1;
	}

	// for each line in the array
	for(i = start; i != end ;i += step) {

		calculated_number *cn = &r->v[ i * r->d ];
		uint8_t *co = &r->o[ i * r->d ];

		calculated_number sum = 0, min = 0, max = 0;
		int all_null = 1, init = 1;

		// for each dimension
		for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
			if(unlikely(r->od[c] & RRDR_HIDDEN)) continue;
			if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_NONZERO))) continue;

			calculated_number n = cn[c];

			if(unlikely(init)) {
				if(n > 0) {
					min = 0;
					max = n;
				}
				else {
					min = n;
					max = 0;
				}
				init = 0;
			}

			if(likely(!(co[c] & RRDR_EMPTY))) {
				all_null = 0;
				if((options & RRDR_OPTION_ABSOLUTE) && n < 0) n = -n;
				sum += n;
			}

			if(n < min) min = n;
			if(n > max) max = n;
		}

		if(likely(i != start))
			buffer_strcat(out, separator);

		if(all_null) {
			if(options & RRDR_OPTION_NULL2ZERO)
				buffer_strcat(out, "0");
			else
				buffer_strcat(out, "null");
		}
		else if(options & RRDR_OPTION_MIN2MAX)
			buffer_rrd_value(out, max - min);
		else
			buffer_rrd_value(out, sum);
	}
	buffer_strcat(out, suffix);
}

inline static calculated_number *rrdr_line_values(RRDR *r)
{
	return &r->v[ r->c * r->d ];
}

inline static uint8_t *rrdr_line_options(RRDR *r)
{
	return &r->o[ r->c * r->d ];
}

inline static int rrdr_line_init(RRDR *r, time_t t)
{
	r->c++;
	if(unlikely(r->c >= r->n)) {
		r->c = r->n - 1;
		return 0;
	}

	// save the time
	r->t[r->c] = t;

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
	if(likely(r->od)) free(r->od);
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
	r->t = malloc(n * sizeof(time_t));
	if(unlikely(!r->t)) goto cleanup;

	r->t = malloc(n * sizeof(time_t));
	if(unlikely(!r->t)) goto cleanup;

	r->v = malloc(n * r->d * sizeof(calculated_number));
	if(unlikely(!r->v)) goto cleanup;

	r->o = malloc(n * r->d * sizeof(uint8_t));
	if(unlikely(!r->o)) goto cleanup;

	r->od = calloc(r->d, sizeof(uint8_t));
	if(unlikely(!r->od)) goto cleanup;

	r->c = -1;

	return r;

cleanup:
	error("Cannot allocate memory");
	if(likely(r)) rrdr_free(r);
	return NULL;
}

RRDR *rrd2rrdr(RRDSET *st, long points, long long after, long long before, int group_method)
{
	int debug = st->debug;

	time_t first_entry_t = rrdset_first_entry_t(st);
	time_t last_entry_t = rrdset_last_entry_t(st);

	if(before == 0 && after == 0) {
		before = last_entry_t;
		after = first_entry_t;
	}

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
		time_t tmp = before;
		before = after;
		after = tmp;
	}

	// the duration of the chart
	time_t duration = before - after;
	if(duration <= 0) return NULL;

	// check the required points
	if(points > duration / st->update_every) points = 0;
	if(points <= 0) points = duration / st->update_every;

	// calculate proper grouping of source data
	long group = duration / points;
	if(group <= 0) group = 1;
	if(duration / group > points) group++;

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
	// checks for debugging

	if(debug) debug(D_RRD_STATS, "INFO %s first_t: %lu, last_t: %lu, all_duration: %lu, after: %lu, before: %lu, duration: %lu, points: %ld, group: %ld"
			, st->id
			, first_entry_t
			, last_entry_t
			, last_entry_t - first_entry_t
			, after
			, before
			, duration
			, points
			, group
			);


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

	long 	start_at_slot = rrdset_time2slot(st, before), // rrdset_last_slot(st),
			stop_at_slot = rrdset_time2slot(st, after);

	time_t 	now = rrdset_slot2time(st, start_at_slot),
			dt = st->update_every,
			group_start_t = 0;

	if(unlikely(debug)) debug(D_RRD_STATS, "INFO  %s after_t: %lu (stop_at_t: %ld), before_t: %lu (start_at_t: %ld), start_t(now): %lu, current_entry: %ld, entries: %ld"
			, st->id
			, after
			, stop_at_slot
			, before
			, start_at_slot
			, now
			, st->current_entry
			, st->entries
			);

	// align to group for proper panning of data
	start_at_slot -= start_at_slot % group;
	stop_at_slot -= stop_at_slot % group;
	now = rrdset_slot2time(st, start_at_slot);

	if(unlikely(debug)) debug(D_RRD_STATS, "BEGIN %s after_t: %lu (stop_at_t: %ld), before_t: %lu (start_at_t: %ld), start_t(now): %lu, current_entry: %ld, entries: %ld"
			, st->id
			, after
			, stop_at_slot
			, before
			, start_at_slot
			, now
			, st->current_entry
			, st->entries
			);

	long slot = start_at_slot, counter = 0, stop_now = 0, added = 0, group_count = 0, add_this = 0;
	for(; !stop_now ; now -= dt, slot--, counter++) {
		if(unlikely(slot < 0)) slot = st->entries - 1;
		if(unlikely(slot == stop_at_slot)) stop_now = counter;

		if(unlikely(debug)) debug(D_RRD_STATS, "ROW %s slot: %ld, entries_counter: %ld, group_count: %ld, added: %ld, now: %lu, %s %s"
				, st->id
				, slot
				, counter
				, group_count + 1
				, added
				, now
				, (group_count + 1 == group)?"PRINT":"  -  "
				, (now >= after && now <= before)?"RANGE":"  -  "
				);

		// make sure we return data in the proper time range
		if(unlikely(now > before)) continue;
		if(unlikely(now < after)) break;

		if(unlikely(group_count == 0)) group_start_t = now;
		group_count++;

		if(unlikely(group_count == group)) {
			if(unlikely(added >= points)) break;
			add_this = 1;
		}

		// do the calculations
		for(rd = st->dimensions, c = 0 ; likely(rd && c < dimensions) ; rd = rd->next, c++) {
			storage_number n = rd->values[slot];
			if(unlikely(!does_storage_number_exist(n))) continue;

			group_counts[c]++;

			calculated_number value = unpack_storage_number(n);
			if(likely(value != 0.0)) {
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
			}
		}

		// added it
		if(unlikely(add_this)) {
			if(unlikely(!rrdr_line_init(r, group_start_t))) break;

			calculated_number *cn = rrdr_line_values(r);
			uint8_t *co = rrdr_line_options(r);

			for(rd = st->dimensions, c = 0 ; likely(rd && c < dimensions) ; rd = rd->next, c++) {

				// update the dimension options
				if(likely(found_non_zero[c])) r->od[c] |= RRDR_NONZERO;
				if(unlikely(rd->flags & RRDDIM_FLAG_HIDDEN)) r->od[c] |= RRDR_HIDDEN;

				// store the specific point options
				co[c] = group_options[c];

				// store the value
				if(unlikely(group_counts[c] == 0)) {
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
	}

	return r;
}

int rrd2format(RRDSET *st, BUFFER *out, BUFFER *dimensions, uint32_t format, long points, long long after, long long before, int group_method, uint32_t options, time_t *latest_timestamp)
{
	RRDR *rrdr = rrd2rrdr(st, points, after, before, group_method);
	if(!rrdr) {
		buffer_strcat(out, "Cannot generate output with these parameters on this chart.");
		return 500;
	}

	if(dimensions)
		rrdr_disable_not_selected_dimensions(rrdr, buffer_tostring(dimensions));

	if(latest_timestamp && rrdr_rows(rrdr) > 0)
		*latest_timestamp = rrdr->t[rrdr_rows(rrdr) - 1];

	switch(format) {
	case DATASOURCE_SSV:
		out->contenttype = CT_TEXT_PLAIN;
		rrdr2ssv(rrdr, out, options, "", " ", "");
		break;

	case DATASOURCE_SSV_COMMA:
		out->contenttype = CT_TEXT_PLAIN;
		rrdr2ssv(rrdr, out, options, "", ",", "");
		break;

	case DATASOURCE_JS_ARRAY:
		out->contenttype = CT_APPLICATION_JSON;
		rrdr2ssv(rrdr, out, options, "[", ",", "]");
		break;

	case DATASOURCE_CSV:
		out->contenttype = CT_TEXT_PLAIN;
		rrdr2csv(rrdr, out, options, "", ",", "\r\n");
		break;

	case DATASOURCE_TSV:
		out->contenttype = CT_TEXT_PLAIN;
		rrdr2csv(rrdr, out, options, "", "\t", "\r\n");
		break;

	case DATASOURCE_HTML:
		out->contenttype = CT_TEXT_HTML;
		buffer_strcat(out, "<html>\n<center><table border=\"0\" cellpadding=\"5\" cellspacing=\"5\">");
		rrdr2csv(rrdr, out, options, "<tr><td>", "</td><td>", "</td></tr>\n");
		buffer_strcat(out, "</table>\n</center>\n</html>\n");
		break;

	case DATASOURCE_GOOGLE_JSONP:
		out->contenttype = CT_APPLICATION_X_JAVASCRIPT;
		rrdr2json(rrdr, out, options, 1);
		break;

	case DATASOURCE_GOOGLE_JSON:
		out->contenttype = CT_APPLICATION_JSON;
		rrdr2json(rrdr, out, options, 1);
		break;

	case DATASOURCE_JSONP:
		out->contenttype = CT_APPLICATION_X_JAVASCRIPT;
		rrdr2json(rrdr, out, options, 0);
		break;

	case DATASOURCE_JSON:
	default:
		out->contenttype = CT_APPLICATION_JSON;
		rrdr2json(rrdr, out, options, 0);
		break;
	}

	rrdr_free(rrdr);
	return 200;
}

unsigned long rrd_stats_json(int type, RRDSET *st, BUFFER *wb, int points, int group, int group_method, time_t after, time_t before, int only_non_zero)
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

	if(points < 1) points = 1;
	if(group < 1) group = 1;

	if(before == 0 || before > rrdset_last_entry_t(st)) before = rrdset_last_entry_t(st);
	if(after  == 0 || after < rrdset_first_entry_t(st)) after = rrdset_first_entry_t(st);

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
		buffer_strcat(wb, "No dimensions yet.");
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
	// checks for debugging

	if(st->debug) {
		debug(D_RRD_STATS, "%s first_entry_t = %lu, last_entry_t = %lu, duration = %lu, after = %lu, before = %lu, duration = %lu, entries_to_show = %lu, group = %lu"
			, st->id
			, rrdset_first_entry_t(st)
			, rrdset_last_entry_t(st)
			, rrdset_last_entry_t(st) - rrdset_first_entry_t(st)
			, after
			, before
			, before - after
			, points
			, group
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
		print_hidden[c] = (rd->flags & RRDDIM_FLAG_HIDDEN)?1:0;
		found_non_zero[c] = 0;
		found_non_existing[c] = 0;
	}


	// error("OLD: points=%d after=%d before=%d group=%d, duration=%d", entries_to_show, before - (st->update_every * group * entries_to_show), before, group, before - after + 1);
	// rrd2array(st, entries_to_show, before - (st->update_every * group * entries_to_show), before, group_method, only_non_zero);
	// rrd2rrdr(st, entries_to_show, before - (st->update_every * group * entries_to_show), before, group_method);

	// -------------------------------------------------------------------------
	// remove dimensions that contain only zeros

	int max_loop = 1;
	if(only_non_zero) max_loop = 2;

	for(; max_loop ; max_loop--) {

		// -------------------------------------------------------------------------
		// print the JSON header

		buffer_sprintf(wb, "{\n	%scols%s:\n	[\n", kq, kq);
		buffer_sprintf(wb, "		{%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
		buffer_sprintf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
		buffer_sprintf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);

		// print the header for each dimension
		// and update the print_hidden array for the dimensions that should be hidden
		int pc = 0;
		for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
			if(!print_hidden[c]) {
				pc++;
				buffer_sprintf(wb, ",\n		{%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, rd->name, sq, kq, kq, sq, sq, kq, kq, sq, sq);
			}
		}
		if(!pc) {
			buffer_sprintf(wb, ",\n		{%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, "no data", sq, kq, kq, sq, sq, kq, kq, sq, sq);
		}

		// print the begin of row data
		buffer_sprintf(wb, "\n	],\n	%srows%s:\n	[\n", kq, kq);


		// -------------------------------------------------------------------------
		// the main loop

		int annotate_reset = 0;
		int annotation_count = 0;

		long 	t = rrdset_time2slot(st, before),
				stop_at_t = rrdset_time2slot(st, after),
				stop_now = 0;

		t -= t % group;

		time_t 	now = rrdset_slot2time(st, t),
				dt = st->update_every;

		long count = 0, printed = 0, group_count = 0;
		last_timestamp = 0;

		if(st->debug) debug(D_RRD_STATS, "%s: REQUEST after:%lu before:%lu, points:%d, group:%d, CHART cur:%ld first: %lu last:%lu, CALC start_t:%ld, stop_t:%ld"
					, st->id
					, after
					, before
					, points
					, group
					, st->current_entry
					, rrdset_first_entry_t(st)
					, rrdset_last_entry_t(st)
					, t
					, stop_at_t
					);

		for(; !stop_now ; now -= dt, t--) {
			if(t < 0) t = st->entries - 1;
			if(t == stop_at_t) stop_now = 1;

			int print_this = 0;

			if(st->debug) debug(D_RRD_STATS, "%s t = %ld, count = %ld, group_count = %ld, printed = %ld, now = %lu, %s %s"
					, st->id
					, t
					, count + 1
					, group_count + 1
					, printed
					, now
					, (group_count + 1 == group)?"PRINT":"  -  "
					, (now >= after && now <= before)?"RANGE":"  -  "
					);


			// make sure we return data in the proper time range
			if(now > before) continue;
			if(now < after) break;

			//if(rrdset_slot2time(st, t) != now)
			//	error("%s: slot=%ld, now=%ld, slot2time=%ld, diff=%ld, last_entry_t=%ld, rrdset_last_slot=%ld", st->id, t, now, rrdset_slot2time(st,t), now - rrdset_slot2time(st,t), rrdset_last_entry_t(st), rrdset_last_slot(st));

			count++;
			group_count++;

			// check if we have to print this now
			if(group_count == group) {
				if(printed >= points) {
					// debug(D_RRD_STATS, "Already printed all rows. Stopping.");
					break;
				}

				// generate the local date time
				struct tm *tm = localtime(&now);
				if(!tm) { error("localtime() failed."); continue; }
				if(now > last_timestamp) last_timestamp = now;

				if(printed) buffer_strcat(wb, "]},\n");
				buffer_strcat(wb, pre_date);
				buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
				buffer_strcat(wb, post_date);

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
					buffer_strcat(wb, overflow_annotation);
					annotate_reset = 0;
				}
				else
					buffer_strcat(wb, normal_annotation);

				pc = 0;
				for(c = 0 ; c < dimensions ; c++) {
					if(found_non_existing[c] == group_count) {
						// all entries are non-existing
						pc++;
						buffer_strcat(wb, pre_value);
						buffer_strcat(wb, "null");
						buffer_strcat(wb, post_value);
					}
					else if(!print_hidden[c]) {
						pc++;
						buffer_strcat(wb, pre_value);
						buffer_rrd_value(wb, group_values[c]);
						buffer_strcat(wb, post_value);

						if(group_values[c]) found_non_zero[c]++;
					}

					// reset them for the next loop
					group_values[c] = 0;
					found_non_existing[c] = 0;
				}

				// if all dimensions are hidden, print a null
				if(!pc) {
					buffer_strcat(wb, pre_value);
					buffer_strcat(wb, "null");
					buffer_strcat(wb, post_value);
				}

				printed++;
				group_count = 0;
			}
		}

		if(printed) buffer_strcat(wb, "]}");
		buffer_strcat(wb, "\n	]\n}\n");

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

			if(changed) buffer_flush(wb);
			else break;
		}
		else break;

	} // max_loop

	debug(D_RRD_STATS, "RRD_STATS_JSON: %s total %ld bytes", st->name, wb->len);

	pthread_rwlock_unlock(&st->rwlock);
	return last_timestamp;
}
