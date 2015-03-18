#include <stdio.h>
#include <stdlib.h>

#include "storage_number.h"
#include "rrd.h"
#include "log.h"
#include "web_buffer.h"

int unit_test_storage()
{
	char buffer[100], *msg;
	storage_number s;
	calculated_number c, a = 0, d, ddiff, dcdiff, f, p, pdiff, pcdiff, maxddiff = 0, maxpdiff = 0;
	int i, j, g, r = 0, l;

	for(g = -1; g <= 1 ; g++) {
		a = 0;

		if(!g) continue;

		for(j = 0; j < 9 ;j++) {
			a += 0.0000001;
			c = a * g;
			for(i = 0; i < 21 ;i++, c *= 10) {
				if(c > 0 && c < 0.00001) continue;
				if(c < 0 && c > -0.00001) continue;

				s = pack_storage_number(c);
				d = unpack_storage_number(s);

				ddiff = d - c;
				dcdiff = ddiff * 100.0 / c;
				if(dcdiff < 0) dcdiff = -dcdiff;
				if(dcdiff > maxddiff) maxddiff = dcdiff;

				f = d / c;

				l = print_calculated_number(buffer, d);
				p = strtold(buffer, NULL);
				pdiff = c - p;
				pcdiff = pdiff * 100.0 / c;
				if(pcdiff < 0) pcdiff = -pcdiff;
				if(pcdiff > maxpdiff) maxpdiff = pcdiff;

				if(f < 0.99999 || f > 1.00001) {
					msg = "ERROR";
					r++;
				}
				else msg = "OK";

				fprintf(stderr, "%s\n"
					CALCULATED_NUMBER_FORMAT " original\n"
					CALCULATED_NUMBER_FORMAT " unpacked, (stored as 0x%08X, diff " CALCULATED_NUMBER_FORMAT ", " CALCULATED_NUMBER_FORMAT "%%)\n"
					"%s printed (%d bytes)\n"
					CALCULATED_NUMBER_FORMAT " re-parsed with diff " CALCULATED_NUMBER_FORMAT ", " CALCULATED_NUMBER_FORMAT "%%\n\n",
					msg,
					c,
					d, s, ddiff, dcdiff,
					buffer,
					l, p, pdiff, pcdiff
				);
			}
		}
	}

	fprintf(stderr, "Worst accuracy loss on unpacked numbers: " CALCULATED_NUMBER_FORMAT "%%\n", maxddiff);
	fprintf(stderr, "Worst accuracy loss on printed numbers: " CALCULATED_NUMBER_FORMAT "%%\n", maxpdiff);

	return r;
}

int unit_test(long delay, long shift)
{
	static int repeat = 0;
	repeat++;

	char name[101];
	snprintf(name, 100, "unittest-%d-%ld-%ld", repeat, delay, shift);

	debug_flags = 0xffffffff;
	memory_mode = NETDATA_MEMORY_MODE_RAM;
	update_every = 1;

	int do_abs = 1;
	int do_inc = 1;
	int do_abst = 1;
	int do_absi = 1;

	RRD_STATS *st = rrd_stats_create("netdata", name, name, "netdata", "Unit Testing", "a value", 1, 1, CHART_TYPE_LINE);
	st->debug = 1;

	RRD_DIMENSION *rdabs = NULL;
	RRD_DIMENSION *rdinc = NULL;
	RRD_DIMENSION *rdabst = NULL;
	RRD_DIMENSION *rdabsi = NULL;

	if(do_abs) rdabs = rrd_stats_dimension_add(st, "absolute", "absolute", 1, 1, RRD_DIMENSION_ABSOLUTE);
	if(do_inc) rdinc = rrd_stats_dimension_add(st, "incremental", "incremental", 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
	if(do_abst) rdabst = rrd_stats_dimension_add(st, "percentage-of-absolute-row", "percentage-of-absolute-row", 1, 1, RRD_DIMENSION_PCENT_OVER_ROW_TOTAL);
	if(do_absi) rdabsi = rrd_stats_dimension_add(st, "percentage-of-incremental-row", "percentage-of-incremental-row", 1, 1, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);

	long increment = 1000;
	collected_number i = 0;

	unsigned long c, dimensions = 0;
	RRD_DIMENSION *rd;
	for(rd = st->dimensions ; rd ; rd = rd->next) dimensions++;

	for(c = 0; c < 20 ;c++) {
		i += increment;

		fprintf(stderr, "\n\nLOOP = %lu, DELAY = %ld, VALUE = " COLLECTED_NUMBER_FORMAT "\n", c, delay, i);
		if(c) {
			rrd_stats_next_usec(st, delay);
		}
		if(do_abs) rrd_stats_dimension_set(st, "absolute", i);
		if(do_inc) rrd_stats_dimension_set(st, "incremental", i);
		if(do_abst) rrd_stats_dimension_set(st, "percentage-of-absolute-row", i);
		if(do_absi) rrd_stats_dimension_set(st, "percentage-of-incremental-row", i);

		if(!c) {
			gettimeofday(&st->last_collected_time, NULL);
			st->last_collected_time.tv_usec = shift;
		}

		// prevent it from deleting the dimensions
		for(rd = st->dimensions ; rd ; rd = rd->next) rd->last_collected_time.tv_sec = st->last_collected_time.tv_sec;

		rrd_stats_done(st);
	}

	unsigned long oincrement = increment;
	increment = increment * st->update_every * 1000000 / delay;
	fprintf(stderr, "\n\nORIGINAL INCREMENT: %lu, INCREMENT %lu, DELAY %lu, SHIFT %lu\n", oincrement * 10, increment * 10, delay, shift);

	int ret = 0;
	storage_number v;
	for(c = 0 ; c < st->counter ; c++) {
		fprintf(stderr, "\nPOSITION: c = %lu, VALUE %lu\n", c, (oincrement + c * increment + increment * (1000000 - shift) / 1000000 )* 10);

		for(rd = st->dimensions ; rd ; rd = rd->next) {
			fprintf(stderr, "\t %s " STORAGE_NUMBER_FORMAT "   ->   ", rd->id, rd->values[c]);

			if(rd == rdabs) v = 
				(	  oincrement 
					+ (increment * (1000000 - shift) / 1000000)
					+ c * increment
				) * 10;

			else if(rd == rdinc) v = (c?(increment):(increment * (1000000 - shift) / 1000000)) * 10;
			else if(rd == rdabst) v = oincrement / dimensions;
			else if(rd == rdabsi) v = oincrement / dimensions;
			else v = 0;

			if(v == rd->values[c]) fprintf(stderr, "passed.\n");
			else {
				fprintf(stderr, "ERROR! (expected " STORAGE_NUMBER_FORMAT ")\n", v);
				ret = 1;
			}
		}
	}

	if(ret)
		fprintf(stderr, "\n\nUNIT TEST(%ld, %ld) FAILED\n\n", delay, shift);

	return ret;
}

