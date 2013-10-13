#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/time.h>

// internal defaults
#define UPDATE_EVERY 1
#define HISTORY 3600
#define SAVE_PATH "/tmp"

#define DEBUG 0

// configuration
#define MAX_LINE 4096
#define MAX_IFACE_NAME 1024
char save_path[FILENAME_MAX+1] = SAVE_PATH;
int update_every = UPDATE_EVERY;
int save_history = 60;

struct iface_history {
	time_t time;
	unsigned long long usec;
	
	unsigned long long rbytes;
	unsigned long long tbytes;
};

struct iface_stats {
	char name[MAX_IFACE_NAME + 1];

	int last_history_id;
	struct iface_history history[HISTORY];

	struct iface_stats *next;
} *interfaces = NULL;

void update_iface_history(unsigned long long usec, char *name, unsigned long long rbytes, unsigned long long tbytes) {
	struct iface_stats *iface = NULL;

	for(iface = interfaces; iface != NULL; iface = iface->next)
		if(strcmp(iface->name, name) == 0) break;

	if(!iface) {
		int i;

		if(DEBUG) printf("Creating new interface for %s\n", name);

		iface = malloc(sizeof(struct iface_stats));
		if(!iface) return;

		iface->last_history_id = HISTORY + 1;

		// link it to beginning
		iface->next = interfaces;
		interfaces = iface;

		for(i = 0; i < HISTORY ;i++) {
			iface->history[i].time = time(NULL) - (HISTORY * update_every) + (i * update_every);
			iface->history[i].rbytes = rbytes;
			iface->history[i].tbytes = tbytes;
			iface->history[i].usec = usec;
		}
	}

	iface->last_history_id++;
	if(iface->last_history_id >= HISTORY) iface->last_history_id = 0;

	if(DEBUG) printf("Updating values for interface %s at position %d, rbytes = %llu, tbytes = %llu\n", iface->name, iface->last_history_id, rbytes, tbytes);

	strcpy(iface->name, name);
	iface->history[iface->last_history_id].time = time(NULL);
	iface->history[iface->last_history_id].rbytes = rbytes;
	iface->history[iface->last_history_id].tbytes = tbytes;
	iface->history[iface->last_history_id].usec = usec;
}

void save_proc_net_dev() {
	struct iface_stats *iface = NULL;
	int r;
	
	for(iface = interfaces; iface != NULL; iface = iface->next) {
		char tmp[FILENAME_MAX+1];
		char filename[FILENAME_MAX+1];
		FILE *fp;
		int i, ld;

		sprintf(tmp, "%s/%s.json.tmp.%d", save_path, iface->name, getpid());
		sprintf(filename, "%s/%s.json", save_path, iface->name);
		fp = fopen(tmp, "w");
		if(!fp) {
			perror(filename);
			return;
		}


		fprintf(fp, "{\n	\"cols\":\n	[\n");
		fprintf(fp, "		{\"id\":\"\",\"label\":\"time\",\"pattern\":\"\",\"type\":\"timeofday\"},\n");
		fprintf(fp, "		{\"id\":\"\",\"label\":\"received\",\"pattern\":\"\",\"type\":\"number\"},\n");
		fprintf(fp, "		{\"id\":\"\",\"label\":\"sent\",\"pattern\":\"\",\"type\":\"number\"}\n");
		fprintf(fp, "	],\n	\"rows\":\n	[\n");

		ld = iface->last_history_id;
		for(i = 1; i < save_history ; i++) {
			int dt, s, d;
			long long rb, tb;
			struct tm *tm;
			char dtm[1025];

			d = iface->last_history_id - i;
			if(d < 0) d += HISTORY;

			dt = iface->history[ld].time - iface->history[d].time;
			if(dt == 0) dt = 1;
			//rb = (iface->history[ld].rbytes - iface->history[d].rbytes) * 8 / dt / 1024;
			//tb = (iface->history[ld].tbytes - iface->history[d].tbytes) * 8 / dt / 1024;
			rb = (iface->history[ld].rbytes - iface->history[d].rbytes) * 1000000 * 8 / iface->history[ld].usec / 1024;
			tb = (iface->history[ld].tbytes - iface->history[d].tbytes) * 1000000 * 8 / iface->history[ld].usec / 1024;

			s = time(NULL) - iface->history[ld].time;

			tm = localtime(&iface->history[d].time);
			if(!tm) { perror("localtime"); continue; }
			// strftime(dtm, 1024, "%T", tm);
			// strftime(dtm, 1024, "%H:%M %B %d, %Y %z", tm);
			// strftime(dtm, 1024, "new Date(\"%B %d, %Y %T\")", tm);
			strftime(dtm, 1024, "[%H, %M, %S, 0]", tm);

 			// fprintf(fp, "		{\"c\":[{\"v\":\"%ds\",\"f\":null},{\"v\":%llu,\"f\":\"%llu\"},{\"v\":%llu,\"f\":\"%llu\"}]}", -s, rb, rb, tb, tb);
 			// fprintf(fp, "		{\"c\":[{\"v\":%s,\"f\":null},{\"v\":%lld,\"f\":null},{\"v\":%lld,\"f\":null}]}", dtm, rb, tb);
 			fprintf(fp, "		{\"c\":[{\"v\":%s},{\"v\":%lld},{\"v\":%lld}]}", dtm, rb, tb);
			if(i == save_history - 1) fprintf(fp, "\n");
			else fprintf(fp, ",\n");

			ld = d;
		}
		fprintf(fp, "	]\n}\n");
		fclose(fp);
		
		r = rename(tmp, filename);
		if(r == -1) perror("Cannot rename JSON file.");
		//unlink(filename);
		//r = link(tmp, filename);
		//unlink(tmp);
	}
}

int do_proc_net_dev(unsigned long long usec) {
	char buffer[MAX_LINE+1] = "";
	char iface[MAX_IFACE_NAME + 1] = "";
	unsigned long long rbytes, rpackets, rerrors, rdrops, rfifo, rframe, rcompressed, rmulticast;
	unsigned long long tbytes, tpackets, terrors, tdrops, tfifo, tcollisions, tcarrier, tcompressed;
	
	int r;
	char *p;
	
	FILE *fp = fopen("/proc/net/dev", "r");
	if(!fp) {
		perror("/proc/net/dev");
		return 1;
	}
	
	// skip the first two lines
	p = fgets(buffer, MAX_LINE, fp);
	p = fgets(buffer, MAX_LINE, fp);
	
	// read the rest of the lines
	for(;1;) {
		char *c;
		p = fgets(buffer, MAX_LINE, fp);
		if(!p) break;
		
		c = strchr(buffer, ':');
		if(c) *c = '\t';
		
		// if(DEBUG) printf("%s\n", buffer);
		r = sscanf(buffer, "%s\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\n",
			iface,
			&rbytes, &rpackets, &rerrors, &rdrops, &rfifo, &rframe, &rcompressed, &rmulticast,
			&tbytes, &tpackets, &terrors, &tdrops, &tfifo, &tcollisions, &tcarrier, &tcompressed);
		if(r == EOF) break;
		if(r != 17) fprintf(stderr, "Cannot read line. Expected 17 params, read %d\n", r);
		else {
			// update our data
			update_iface_history(usec, iface, rbytes, tbytes);
		}
	}
	
	// done reading, close it
	fclose(fp);
	
	// save statistics
	save_proc_net_dev();
	
	return 0;
}

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000) + now->tv_usec) - ((last->tv_sec * 1000000) + last->tv_usec)));
}

int main(int argc, char **argv) {
	struct timeval last, now, tmp;
	int i;
	int daemon = 0;
	
	// parse  the arguments
	for(i = 1; i < argc ; i++) {
		if(strcmp(argv[i], "-l") == 0 && (i+1) < argc) {
			save_history = atoi(argv[i+1]);
			if(save_history <= 0 || save_history > HISTORY) {
				fprintf(stderr, "Invalid save lines %d given. Defaulting to %d.\n", save_history, HISTORY);
				save_history = HISTORY;
			}
			else {
				fprintf(stderr, "save lines set to %d.\n", save_history);
			}
			i++;
		}
		else if(strcmp(argv[i], "-u") == 0 && (i+1) < argc) {
			update_every = atoi(argv[i+1]);
			if(update_every <= 0 || update_every > 600) {
				fprintf(stderr, "Invalid update timer %d given. Defaulting to %d.\n", update_every, UPDATE_EVERY);
				update_every = UPDATE_EVERY;
			}
			else {
				fprintf(stderr, "update timer set to %d.\n", update_every);
			}
			i++;
		}
		else if(strcmp(argv[i], "-o") == 0 && (i+1) < argc) {
			strncpy(save_path, argv[i+1], FILENAME_MAX);
			save_path[FILENAME_MAX]='\0';
			fprintf(stderr, "Saving files to '%s'.\n", save_path);
			i++;
		}
		else if(strcmp(argv[i], "-d") == 0) {
			daemon = 1;
			fprintf(stderr, "Enabled daemon mode.\n");
		}
		else {
			fprintf(stderr, "Cannot understand option '%s'.\n", argv[i]);
			fprintf(stderr, "\nUSAGE: %s [-d] [-l LINES_TO_SAVE] [-u UPDATE_TIMER] [-o PATH_TO_SAVE_FILES].\n\n", argv[0]);
			fprintf(stderr, "  -d enabled daemon mode.\n");
			fprintf(stderr, "  -l LINES_TO_SAVE can be from 0 to %d lines in JSON data. Default: %d.\n", HISTORY, save_history);
			fprintf(stderr, "  -u UPDATE_TIMER can be from 1 to %d seconds. Default: %d.\n", 600, update_every);
			fprintf(stderr, "  -o PATH_TO_SAVE_FILES is a directory to place the JSON files. Default: '%s'.\n", save_path);
			exit(1);
		}
	}
	
	if(daemon) {
		i = fork();
		if(i == -1) {
			perror("cannot fork");
			exit(1);
		}
		if(i != 0) exit(0); // the parent
		close(0);
		close(1);
		close(2);
	}
	
	// main loop
	gettimeofday(&last, NULL);
	last.tv_sec -= update_every;
	
	for(;1;) {
		unsigned long long usec, susec;
		gettimeofday(&now, NULL);
		
		// calculate the time it took for a full loop
		usec = usecdiff(&now, &last);
		if(DEBUG) printf("Last loop took %llu usec\n", usec);
		
		do_proc_net_dev(usec);
		
		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&tmp, NULL);
		usec = usecdiff(&tmp, &now);
		if(DEBUG) printf("This loop took %llu usec\n", usec);
		
		if(usec < (update_every * 1000000)) susec = (update_every * 1000000) - usec;
		else susec = 0;
		
		// make sure we will wait at least 100ms
		if(susec < 100000) susec = 100000;
		
		if(DEBUG) printf("Sleeping for %llu usec\n", susec);
		usleep(susec);
		
		// copy now to last
		last.tv_sec = now.tv_sec;
		last.tv_usec = now.tv_usec;
		
		//exit(1);
	}
}
