#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>

#include "storage_number.h"
#include "rrd.h"
#include "log.h"
#include "web_buffer.h"

#define ACCURACY_LOSS 0.0000001

int check_storage_number(calculated_number n, int debug) {
	char buffer[100];

	storage_number s = pack_storage_number(n);
	calculated_number d = unpack_storage_number(s);

	calculated_number ddiff = d - n;
	calculated_number dcdiff = ddiff * 100.0 / n;

	if(dcdiff < 0) dcdiff = -dcdiff;

	size_t len = print_calculated_number(buffer, d);
	calculated_number p = strtold(buffer, NULL);
	calculated_number pdiff = n - p;
	calculated_number pcdiff = pdiff * 100.0 / n;
	if(pcdiff < 0) pcdiff = -pcdiff;

	if(debug) {
		fprintf(stderr,
			CALCULATED_NUMBER_FORMAT " original\n"
			CALCULATED_NUMBER_FORMAT " packed and unpacked, (stored as 0x%08X, diff " CALCULATED_NUMBER_FORMAT ", " CALCULATED_NUMBER_FORMAT "%%)\n"
			"%s printed after unpacked (%zu bytes)\n"
			CALCULATED_NUMBER_FORMAT " re-parsed from printed (diff " CALCULATED_NUMBER_FORMAT ", " CALCULATED_NUMBER_FORMAT "%%)\n\n",
			n,
			d, s, ddiff, dcdiff,
			buffer,
			len, p, pdiff, pcdiff
		);
		if(len != strlen(buffer)) fprintf(stderr, "ERROR: printed number %s is reported to have length %zu but it has %zu\n", buffer, len, strlen(buffer));
		if(dcdiff > ACCURACY_LOSS) fprintf(stderr, "WARNING: packing number " CALCULATED_NUMBER_FORMAT " has accuracy loss %0.7Lf %%\n", n, dcdiff);
		if(pcdiff > ACCURACY_LOSS) fprintf(stderr, "WARNING: re-parsing the packed, unpacked and printed number " CALCULATED_NUMBER_FORMAT " has accuracy loss %0.7Lf %%\n", n, pcdiff);
	}

	if(len != strlen(buffer)) return 1;
	if(dcdiff > ACCURACY_LOSS) return 3;
	if(pcdiff > ACCURACY_LOSS) return 4;
	return 0;
}

void benchmark_storage_number(int loop, int multiplier) {
	int i, j;
	calculated_number n, d;
	storage_number s;
	unsigned long long user, system, total, mine, their;

	char buffer[100];

	struct rusage now, last;

	fprintf(stderr, "\n\nBenchmarking %d numbers, please wait...\n\n", loop);

	// ------------------------------------------------------------------------

	fprintf(stderr, "SYSTEM  LONG DOUBLE    SIZE: %zu bytes\n", sizeof(calculated_number));
	fprintf(stderr, "NETDATA FLOATING POINT SIZE: %zu bytes\n", sizeof(storage_number));

	mine = (calculated_number)sizeof(storage_number) * (calculated_number)loop;
	their = (calculated_number)sizeof(calculated_number) * (calculated_number)loop;
	
	if(mine > their) {
		fprintf(stderr, "\nNETDATA NEEDS %0.2Lf TIMES MORE MEMORY. Sorry!\n", (long double)(mine / their));
	}
	else {
		fprintf(stderr, "\nNETDATA INTERNAL FLOATING POINT ARITHMETICS NEEDS %0.2Lf TIMES LESS MEMORY.\n", (long double)(their / mine));
	}

	fprintf(stderr, "\nNETDATA FLOATING POINT\n");
	fprintf(stderr, "MIN POSITIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", (calculated_number)STORAGE_NUMBER_POSITIVE_MIN);
	fprintf(stderr, "MAX POSITIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", (calculated_number)STORAGE_NUMBER_POSITIVE_MAX);
	fprintf(stderr, "MIN NEGATIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", (calculated_number)STORAGE_NUMBER_NEGATIVE_MIN);
	fprintf(stderr, "MAX NEGATIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", (calculated_number)STORAGE_NUMBER_NEGATIVE_MAX);
	fprintf(stderr, "Maximum accuracy loss: " CALCULATED_NUMBER_FORMAT "%%\n\n\n", (calculated_number)ACCURACY_LOSS);

	// ------------------------------------------------------------------------

	fprintf(stderr, "INTERNAL LONG DOUBLE PRINTING: ");
	getrusage(RUSAGE_SELF, &last);

	// do the job
	for(j = 1; j < 11 ;j++) {
		n = STORAGE_NUMBER_POSITIVE_MIN * j;

		for(i = 0; i < loop ;i++) {
			n *= multiplier;
			if(n > STORAGE_NUMBER_POSITIVE_MAX) n = STORAGE_NUMBER_POSITIVE_MIN;

			print_calculated_number(buffer, n);
		}
	}

	getrusage(RUSAGE_SELF, &now);
	user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
	system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
	total  = user + system;
	mine = total;

	fprintf(stderr, "user %0.5Lf, system %0.5Lf, total %0.5Lf\n", (long double)(user / 1000000.0), (long double)(system / 1000000.0), (long double)(total / 1000000.0));
	
	// ------------------------------------------------------------------------

	fprintf(stderr, "SYSTEM   LONG DOUBLE PRINTING: ");
	getrusage(RUSAGE_SELF, &last);

	// do the job
	for(j = 1; j < 11 ;j++) {
		n = STORAGE_NUMBER_POSITIVE_MIN * j;

		for(i = 0; i < loop ;i++) {
			n *= multiplier;
			if(n > STORAGE_NUMBER_POSITIVE_MAX) n = STORAGE_NUMBER_POSITIVE_MIN;
			snprintf(buffer, 100, CALCULATED_NUMBER_FORMAT, n);
		}
	}

	getrusage(RUSAGE_SELF, &now);
	user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
	system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
	total  = user + system;
	their = total;

	fprintf(stderr, "user %0.5Lf, system %0.5Lf, total %0.5Lf\n", (long double)(user / 1000000.0), (long double)(system / 1000000.0), (long double)(total / 1000000.0));

	if(mine > total) {
		fprintf(stderr, "NETDATA CODE IS SLOWER %0.2Lf %%\n", (long double)(mine * 100.0 / their - 100.0));
	}
	else {
		fprintf(stderr, "NETDATA CODE IS  F A S T E R  %0.2Lf %%\n", (long double)(their * 100.0 / mine - 100.0));
	}

	// ------------------------------------------------------------------------

	fprintf(stderr, "\nINTERNAL LONG DOUBLE PRINTING WITH PACK / UNPACK: ");
	getrusage(RUSAGE_SELF, &last);

	// do the job
	for(j = 1; j < 11 ;j++) {
		n = STORAGE_NUMBER_POSITIVE_MIN * j;

		for(i = 0; i < loop ;i++) {
			n *= multiplier;
			if(n > STORAGE_NUMBER_POSITIVE_MAX) n = STORAGE_NUMBER_POSITIVE_MIN;

			s = pack_storage_number(n);
			d = unpack_storage_number(s);
			print_calculated_number(buffer, d);
		}
	}

	getrusage(RUSAGE_SELF, &now);
	user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
	system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
	total  = user + system;
	mine = total;

	fprintf(stderr, "user %0.5Lf, system %0.5Lf, total %0.5Lf\n", (long double)(user / 1000000.0), (long double)(system / 1000000.0), (long double)(total / 1000000.0));

	if(mine > their) {
		fprintf(stderr, "WITH PACKING UNPACKING NETDATA CODE IS SLOWER %0.2Lf %%\n", (long double)(mine * 100.0 / their - 100.0));
	}
	else {
		fprintf(stderr, "EVEN WITH PACKING AND UNPACKING, NETDATA CODE IS  F A S T E R  %0.2Lf %%\n", (long double)(their * 100.0 / mine - 100.0));
	}

	// ------------------------------------------------------------------------

}

int unit_test_storage()
{
	calculated_number c, a = 0;
	int i, j, g, r = 0;

	for(g = -1; g <= 1 ; g++) {
		a = 0;

		if(!g) continue;

		for(j = 0; j < 9 ;j++) {
			a += 0.0000001;
			c = a * g;
			for(i = 0; i < 21 ;i++, c *= 10) {
				if(c > 0 && c < STORAGE_NUMBER_POSITIVE_MIN) continue;
				if(c < 0 && c > STORAGE_NUMBER_NEGATIVE_MAX) continue;

				if(check_storage_number(c, 1)) return 1;
			}
		}
	}

	benchmark_storage_number(1000000, 2);
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

