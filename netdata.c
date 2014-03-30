// enable strcasestr()
#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <pwd.h>
#include <sys/types.h>
#include <sys/time.h>

#include <sys/socket.h>
#include <sys/select.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <sys/sendfile.h>

#include <arpa/inet.h>
#include <netinet/in.h>
#include <netinet/tcp.h>

#include <errno.h>
#include <stdarg.h>
#include <locale.h>
#include <signal.h>
#include <ctype.h>
#include <time.h>
#include <fcntl.h>
#include <syslog.h>

#include <pthread.h>

#define RRD_DIMENSION_ABSOLUTE		0
#define RRD_DIMENSION_INCREMENTAL	1

#define RRD_TYPE_NET			"net"
#define RRD_TYPE_NET_LEN		strlen(RRD_TYPE_NET)

#define RRD_TYPE_TC				"tc"
#define RRD_TYPE_TC_LEN			strlen(RRD_TYPE_TC)

#define RRD_TYPE_DISK			"disk"
#define RRD_TYPE_DISK_LEN		strlen(RRD_TYPE_DISK)

#define RRD_TYPE_NET_SNMP		"ipv4"
#define RRD_TYPE_NET_SNMP_LEN	strlen(RRD_TYPE_NET_SNMP)

#define WEB_PATH_FILE			"file"
#define WEB_PATH_DATA			"data"
#define WEB_PATH_GRAPH			"graph"

// internal defaults
#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600
#define LISTEN_PORT 19999
#define HISTORY 3600
#define HISTORY_MAX (86400*10)
#define SAVE_PATH "/tmp"

#define D_WEB_BUFFER 		0x00000001
#define D_WEB_CLIENT 		0x00000002
#define D_LISTENER   		0x00000004
#define D_WEB_DATA   		0x00000008
#define D_OPTIONS    		0x00000010
#define D_PROCNETDEV_LOOP   0x00000020
#define D_RRD_STATS 		0x00000040
#define D_WEB_CLIENT_ACCESS	0x00000080
#define D_TC_LOOP           0x00000100

#define CT_APPLICATION_JSON				1
#define CT_TEXT_PLAIN					2
#define CT_TEXT_HTML					3
#define CT_APPLICATION_X_JAVASCRIPT		4
#define CT_TEXT_CSS						5
#define CT_TEXT_XML						6
#define CT_APPLICATION_XML				7
#define CT_TEXT_XSL						8
#define CT_APPLICATION_OCTET_STREAM		9
#define CT_APPLICATION_X_FONT_TRUETYPE	10
#define CT_APPLICATION_X_FONT_OPENTYPE	11
#define CT_APPLICATION_FONT_WOFF		12
#define CT_APPLICATION_VND_MS_FONTOBJ	13
#define CT_IMAGE_SVG_XML				14

// configuration
#define DEBUG (D_WEB_CLIENT_ACCESS|D_LISTENER|D_RRD_STATS)
//#define DEBUG 0xffffffff
//#define DEBUG (0)

#define EXIT_FAILURE 1
#define LISTEN_BACKLOG 100

#define MAX_SOCKET_INPUT_DATA 65536
#define MAX_SOCKET_OUTPUT_DATA 65536

#define MIN_SOCKET_INPUT_DATA 16384
#define DEFAULT_DATA_BUFFER 65536

#define MAX_HTTP_HEADER_SIZE 8192

#define MAX_PROC_NET_DEV_LINE 4096
#define MAX_PROC_NET_DEV_IFACE_NAME 1024

#define MAX_PROC_DISKSTATS_LINE 4096
#define MAX_PROC_DISKSTATS_DISK_NAME 1024

#define MAX_PROC_NET_SNMP_LINE 4096
#define MAX_PROC_NET_SNMP_NAME 1024


int silent = 0;
int save_history = HISTORY;
int update_every = UPDATE_EVERY;
int listen_port = LISTEN_PORT;


// ----------------------------------------------------------------------------
// helpers

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000) + now->tv_usec) - ((last->tv_sec * 1000000) + last->tv_usec)));
}

// ----------------------------------------------------------------------------
// LOG

void log_date()
{
        char outstr[200];
        time_t t;
        struct tm *tmp;

        t = time(NULL);
        tmp = localtime(&t);

        if (tmp == NULL) return;
        if (strftime(outstr, sizeof(outstr), "%y-%m-%d %H:%M:%S", tmp) == 0) return;

        fprintf(stderr, "%s: ", outstr);
}

#define debug(args...)  debug_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void debug_int( const char *file, const char *function, const unsigned long line, unsigned long type, const char *fmt, ... )
{
	if(silent) return;

    va_list args;

    if(DEBUG & type) {
        log_date();
        va_start( args, fmt );
        fprintf(stderr, "DEBUG (%04lu@%-15.15s): ", line, function);
        vfprintf( stderr, fmt, args );
        va_end( args );
        fprintf(stderr, "\n");
    }
}

#define error(args...)  error_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void error_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	if(silent) return;

    va_list args;

    log_date();

    va_start( args, fmt );
    fprintf(stderr, "ERROR (%04lu@%-15.15s): ", line, function);
    vfprintf( stderr, fmt, args );
    va_end( args );

    if(errno) {
            fprintf(stderr, " (errno %d, %s)\n", errno, strerror(errno));
            errno = 0;
    }
    else fprintf(stderr, "\n");

    va_start( args, fmt );
	vsyslog(LOG_ERR,  fmt, args );
    va_end( args );
}

#define fatal(args...)  fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	if(silent) exit(EXIT_FAILURE);

	va_list args;

	log_date();

	va_start( args, fmt );
	fprintf(stderr, "FATAL (%04lu@%-15.15s): ", line, function);
	vfprintf( stderr, fmt, args );
	va_end( args );

	perror(" # ");
	fprintf(stderr, "\n");

    va_start( args, fmt );
	vsyslog(LOG_CRIT,  fmt, args );
    va_end( args );

	exit(EXIT_FAILURE);
}


// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
char from_hex(char ch) {
	return (char)(isdigit(ch) ? ch - '0' : tolower(ch) - 'a' + 10);
}

/* Converts an integer value to its hex character*/
char to_hex(char code) {
	static char hex[] = "0123456789abcdef";
	return hex[code & 15];
}

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(char *str) {
	char *pstr = str,
		*buf = malloc(strlen(str) * 3 + 1),
		*pbuf = buf;

	while (*pstr) {
		if (isalnum(*pstr) || *pstr == '-' || *pstr == '_' || *pstr == '.' || *pstr == '~')
			*pbuf++ = *pstr;

		else if (*pstr == ' ')
			*pbuf++ = '+';

		else
			*pbuf++ = '%', *pbuf++ = to_hex(*pstr >> 4), *pbuf++ = to_hex(*pstr & 15);

		pstr++;
	}

	*pbuf = '\0';

	return buf;
}

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_decode(char *str) {
	char *pstr = str,
		*buf = malloc(strlen(str) + 1),
		*pbuf = buf;

	if(!buf) fatal("Cannot allocate memory.");

	while (*pstr) {
		if (*pstr == '%') {
			if (pstr[1] && pstr[2]) {
				*pbuf++ = from_hex(pstr[1]) << 4 | from_hex(pstr[2]);
				pstr += 2;
			}
		}
		else if (*pstr == '+')
			*pbuf++ = ' ';

		else
			*pbuf++ = *pstr;
		
		pstr++;
	}
	
	*pbuf = '\0';
	
	return buf;
}


// ----------------------------------------------------------------------------
// socket

int create_listen_socket(int port)
{
        int sock=-1;
        int sockopt=1;
        struct sockaddr_in name;

        debug(D_LISTENER, "Creating new listening socket on port %d", port);

        sock = socket(AF_INET, SOCK_STREAM, 0);
        if (sock < 0)
                fatal("socket() failed, errno=%d", errno);

        /* avoid "address already in use" */
        setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&sockopt, sizeof(sockopt));

        memset(&name, 0, sizeof(struct sockaddr_in));
        name.sin_family = AF_INET;
        name.sin_port = htons (port);
        name.sin_addr.s_addr = htonl (INADDR_ANY);
        if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0)
            fatal("bind() failed, errno=%d", errno);

        if (listen(sock, LISTEN_BACKLOG) < 0)
			fatal("listen() failed, errno=%d", errno);

        debug(D_LISTENER, "Listening Port %d created", port);
        return sock;
}


// ----------------------------------------------------------------------------
// RRD STATS

#define RRD_STATS_NAME_MAX 1024

struct rrd_dimension {
	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];
	size_t bytes;
	size_t entries;

	int type;

	long multiplier;
	long divisor;

	void *values;
	
	time_t last_updated;

	struct rrd_dimension *next;
};
typedef struct rrd_dimension RRD_DIMENSION;

struct rrd_stats {
	pthread_mutex_t mutex;

	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];
	char title[RRD_STATS_NAME_MAX + 1];
	char vtitle[RRD_STATS_NAME_MAX + 1];

	char usertitle[RRD_STATS_NAME_MAX + 1];
	char userpriority[RRD_STATS_NAME_MAX + 1];

	char envtitle[RRD_STATS_NAME_MAX + 1];
	char envpriority[RRD_STATS_NAME_MAX + 1];

	char type[RRD_STATS_NAME_MAX + 1];

	char hostname[RRD_STATS_NAME_MAX + 1];

	size_t entries;
	size_t current_entry;
	// size_t last_entry;

	time_t last_updated;

	struct timeval *times;
	RRD_DIMENSION *dimensions;
	struct rrd_stats *next;
};
typedef struct rrd_stats RRD_STATS;

RRD_STATS *root = NULL;
pthread_mutex_t root_mutex = PTHREAD_MUTEX_INITIALIZER;

RRD_STATS *rrd_stats_create(const char *id, const char *name, unsigned long entries, const char *title, const char *vtitle, const char *type)
{
	RRD_STATS *st = NULL;
	char *p;

	debug(D_RRD_STATS, "Creating RRD_STATS for '%s'.", name);

	st = calloc(1, sizeof(RRD_STATS));
	if(!st) return NULL;

	st->times = calloc(entries, sizeof(struct timeval));
	if(!st->times) {
		error("Cannot allocate %lu entries of %lu bytes each for RRD_STATS.", st->entries, sizeof(struct timeval));
		free(st);
		return NULL;
	}
	
	strncpy(st->id, id, RRD_STATS_NAME_MAX);
	st->id[RRD_STATS_NAME_MAX] = '\0';

	strncpy(st->name, name, RRD_STATS_NAME_MAX);
	st->name[RRD_STATS_NAME_MAX] = '\0';
	while((p = strchr(st->name, '/'))) *p = '_';
	while((p = strchr(st->name, '?'))) *p = '_';
	while((p = strchr(st->name, '&'))) *p = '_';

	strncpy(st->title, title, RRD_STATS_NAME_MAX);
	st->title[RRD_STATS_NAME_MAX] = '\0';

	strncpy(st->vtitle, vtitle, RRD_STATS_NAME_MAX);
	st->vtitle[RRD_STATS_NAME_MAX] = '\0';

	strncpy(st->type, type, RRD_STATS_NAME_MAX);
	st->type[RRD_STATS_NAME_MAX] = '\0';

	// check if there is a name for it in the environment
	sprintf(st->envtitle, "NETDATA_TITLE_%s", st->id);
	while((p = strchr(st->envtitle, '/'))) *p = '_';
	while((p = strchr(st->envtitle, '.'))) *p = '_';
	while((p = strchr(st->envtitle, '-'))) *p = '_';
	p = getenv(st->envtitle);

	if(p) strncpy(st->usertitle, p, RRD_STATS_NAME_MAX);
	else strncpy(st->usertitle, st->name, RRD_STATS_NAME_MAX);
	st->usertitle[RRD_STATS_NAME_MAX] = '\0';

	sprintf(st->envpriority, "NETDATA_PRIORITY_%s", st->id);
	while((p = strchr(st->envpriority, '/'))) *p = '_';
	while((p = strchr(st->envpriority, '.'))) *p = '_';
	while((p = strchr(st->envpriority, '-'))) *p = '_';
	p = getenv(st->envpriority);

	if(p) strncpy(st->userpriority, p, RRD_STATS_NAME_MAX);
	else strncpy(st->userpriority, st->name, RRD_STATS_NAME_MAX);
	st->userpriority[RRD_STATS_NAME_MAX] = '\0';

	if(gethostname(st->hostname, RRD_STATS_NAME_MAX) == -1)
		error("Cannot get hostname.");

	st->entries = entries;
	st->current_entry = 0;
	// st->last_entry = 0;
	st->dimensions = NULL;
	st->last_updated = time(NULL);

	pthread_mutex_init(&st->mutex, NULL);
	pthread_mutex_lock(&st->mutex);
	pthread_mutex_lock(&root_mutex);

	st->next = root;
	root = st;

	pthread_mutex_unlock(&root_mutex);
	// leave st->mutex locked

	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_add(RRD_STATS *st, const char *id, const char *name, size_t bytes, long multiplier, long divisor, int type)
{
	RRD_DIMENSION *rd = NULL;

	debug(D_RRD_STATS, "Adding dimension '%s' (%s) to RRD_STATS '%s' (%s).", name, id, st->name, st->id);

	rd = calloc(1, sizeof(RRD_DIMENSION));
	if(!rd) return NULL;

	rd->bytes = bytes;
	rd->entries = st->entries;
	rd->multiplier = multiplier;
	rd->divisor = divisor;
	rd->type = type;
	rd->values = calloc(rd->entries, rd->bytes);
	if(!rd->values) {
		error("Cannot allocate %lu entries of %lu bytes each for RRD_DIMENSION.", rd->entries, rd->bytes);
		free(rd);
		return NULL;
	}

	strncpy(rd->id, id, RRD_STATS_NAME_MAX);
	rd->id[RRD_STATS_NAME_MAX] = '\0';

	strncpy(rd->name, name, RRD_STATS_NAME_MAX);
	rd->name[RRD_STATS_NAME_MAX] = '\0';

	rd->next = st->dimensions;
	st->dimensions = rd;

	return(rd);
}

void rrd_stats_dimension_free(RRD_DIMENSION *rd)
{
	if(rd->next) rrd_stats_dimension_free(rd->next);
	debug(D_RRD_STATS, "Removing dimension '%s'.", rd->name);
	free(rd->values);
	free(rd);
}

RRD_STATS *rrd_stats_find(const char *id)
{
	pthread_mutex_lock(&root_mutex);

	RRD_STATS *st = root;

	for ( ; st ; st = st->next ) {
		if(strcmp(st->id, id) == 0) break;
	}

	pthread_mutex_unlock(&root_mutex);
	return(st);
}

RRD_STATS *rrd_stats_find_byname(const char *name)
{
	pthread_mutex_lock(&root_mutex);

	RRD_STATS *st = root;

	for ( ; st ; st = st->next ) {
		if(strcmp(st->name, name) == 0) break;
	}

	pthread_mutex_unlock(&root_mutex);
	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_find(RRD_STATS *st, const char *id)
{
	RRD_DIMENSION *rd = st->dimensions;

	for ( ; rd ; rd = rd->next ) {
		if(strcmp(rd->id, id) == 0) break;
	}

	return(rd);
}

void rrd_stats_next(RRD_STATS *st)
{
	struct timeval *now;

	pthread_mutex_lock(&st->mutex);

	// st->current_entry should never be outside the array
	// or, the parallel threads may end up crashing
	// st->last_entry = st->current_entry;
	st->current_entry = ((st->current_entry + 1) >= st->entries) ? 0 : st->current_entry + 1;

	now = &st->times[st->current_entry];
	gettimeofday(now, NULL);

	st->last_updated = now->tv_sec;

	// leave mutex locked
}

void rrd_stats_done(RRD_STATS *st)
{
	RRD_DIMENSION *rd, *last;

	// find if there are any obsolete dimensions (not updated recently)
	for( rd = st->dimensions, last = NULL ; rd ; ) {
		if(rd->last_updated < st->last_updated) {
			debug(D_RRD_STATS, "Removing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

			if(!last) {
				st->dimensions = rd->next;
				rd->next = NULL;
				rrd_stats_dimension_free(rd);
				rd = st->dimensions;
				continue;
			}
			else {
				last->next = rd->next;
				rd->next = NULL;
				rrd_stats_dimension_free(rd);
				rd = last->next;
				continue;
			}
		}

		last = rd;
		rd = rd->next;
	}

	pthread_mutex_unlock(&st->mutex);
}

void rrd_stats_dimension_set(RRD_STATS *st, const char *id, void *data)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return;
	}

	rd->last_updated = st->last_updated;

	if(rd->bytes == sizeof(long long)) {
		long long *dimension = rd->values, *value = data;

		dimension[st->current_entry] = (*value);
	}
	else if(rd->bytes == sizeof(long)) {
		long *dimension = rd->values, *value = data;

		dimension[st->current_entry] = (*value);
	}
	else if(rd->bytes == sizeof(int)) {
		int *dimension = rd->values, *value = data;

		dimension[st->current_entry] = (*value);
	}
	else if(rd->bytes == sizeof(char)) {
		char *dimension = rd->values, *value = data;

		dimension[st->current_entry] = (*value);
	}
	else fatal("I don't know how to handle data of length %d bytes.", rd->bytes);
}

#define GROUP_AVERAGE	0
#define GROUP_MAX 		1

size_t rrd_stats_one_json(RRD_STATS *st, char *options, char *buffer, size_t len)
{
	size_t i = 0;
	if(i + 200 > len) return(0);

	i += sprintf(&buffer[i], "\t\t{\n");
	i += sprintf(&buffer[i], "\t\t\t\"id\" : \"%s\",\n", st->id);
	i += sprintf(&buffer[i], "\t\t\t\"name\" : \"%s\",\n", st->name);
	i += sprintf(&buffer[i], "\t\t\t\"type\" : \"%s\",\n", st->type);
	i += sprintf(&buffer[i], "\t\t\t\"title\" : \"%s %s\",\n", st->title, st->name);
	i += sprintf(&buffer[i], "\t\t\t\"usertitle\" : \"%s %s (%s)\",\n", st->title, st->usertitle, st->name);
	i += sprintf(&buffer[i], "\t\t\t\"userpriority\" : \"%s\",\n", st->userpriority);
	i += sprintf(&buffer[i], "\t\t\t\"envtitle\" : \"%s\",\n", st->envtitle);
	i += sprintf(&buffer[i], "\t\t\t\"envpriority\" : \"%s\",\n", st->envpriority);
	i += sprintf(&buffer[i], "\t\t\t\"hostname\" : \"%s\",\n", st->hostname);
	i += sprintf(&buffer[i], "\t\t\t\"vtitle\" : \"%s\",\n", st->vtitle);
	i += sprintf(&buffer[i], "\t\t\t\"url\" : \"/data/%s/%s\",\n", st->name, options?options:"");
	i += sprintf(&buffer[i], "\t\t\t\"entries\" : %ld,\n", st->entries);
	i += sprintf(&buffer[i], "\t\t\t\"current\" : %ld,\n", st->current_entry);
	i += sprintf(&buffer[i], "\t\t\t\"update_every\" : %d,\n", update_every);
	i += sprintf(&buffer[i], "\t\t\t\"last_updated\" : %lu,\n", st->last_updated);
	i += sprintf(&buffer[i], "\t\t\t\"last_updated_secs_ago\" : %lu\n", time(NULL) - st->last_updated);
	i += sprintf(&buffer[i], "\t\t}");

	return(i);
}

#define RRD_GRAPH_JSON_HEADER "{\n\t\"charts\": [\n"
#define RRD_GRAPH_JSON_FOOTER "\n\t]\n}\n"

size_t rrd_stats_graph_json(RRD_STATS *st, char * options, char *buffer, size_t len)
{
	size_t i = 0;
	i += sprintf(&buffer[i], RRD_GRAPH_JSON_HEADER);
	i += rrd_stats_one_json(st, options, &buffer[i], len - i);
	i += sprintf(&buffer[i], RRD_GRAPH_JSON_FOOTER);
	return(i);
}

size_t rrd_stats_all_json(char *buffer, size_t len)
{
	size_t i = 0, c;
	RRD_STATS *st;

	i += sprintf(&buffer[i], RRD_GRAPH_JSON_HEADER);

	for(st = root, c = 0; st ; st = st->next, c++) {
		if(c) i += sprintf(&buffer[i], "%s", ",\n");
		i += rrd_stats_one_json(st, NULL, &buffer[i], len - i);
	}
	
	i += sprintf(&buffer[i], RRD_GRAPH_JSON_FOOTER);
	return(i);
}

size_t rrd_stats_json(RRD_STATS *st, char *b, size_t length, size_t entries_to_show, size_t group_count, int group_method)
{
	pthread_mutex_lock(&st->mutex);

	// check the options
	if(entries_to_show <= 0) entries_to_show = 1;
	if(group_count <= 0) group_count = 1;
	if(group_count > st->entries / 20) group_count = st->entries / 20;

	size_t i = 0;				// the bytes of JSON output we have generated so far
	size_t printed = 0;			// the lines of JSON data we have generated so far

	size_t current_entry = st->current_entry;
	size_t t, lt;				// t = the current entry, lt = the lest entry of data
	long count = st->entries;	// count down of the entries examined so far
	int pad = 0;				// align the entries when grouping values together

	RRD_DIMENSION *rd;
	size_t c = 0;				// counter for dimension loops
	size_t dimensions = 0;		// the total number of dimensions present

	unsigned long long usec = 0;// usec between the entries
	char dtm[201];				// temp variable for storing dates

	// find how many dimensions we have
	for( rd = st->dimensions ; rd ; rd = rd->next)
		dimensions++;

	if(!dimensions) {
		pthread_mutex_unlock(&st->mutex);
		return sprintf(b, "No dimensions yet.");
	}

	// temporary storage to keep track of group values and counts
	long long group_values[dimensions];
	long long group_counts[dimensions];
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++)
		group_values[c] = group_counts[c] = 0;

	i += sprintf(&b[i], "{\n	\"cols\":\n	[\n");
	i += sprintf(&b[i], "		{\"id\":\"\",\"label\":\"time\",\"pattern\":\"\",\"type\":\"datetime\"},\n");

	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++)
		i += sprintf(&b[i], "		{\"id\":\"\",\"label\":\"%s\",\"pattern\":\"\",\"type\":\"number\"}%s\n", rd->name, rd->next?",":"");

	i += sprintf(&b[i], "	],\n	\"rows\":\n	[\n");

	// to allow grouping on the same values, we need a pad
	pad = current_entry % group_count;

	// make sure current_entry is within limits
	if(current_entry < 0 || current_entry >= st->entries) current_entry = 0;

	// find the old entry of the round-robin
	t = current_entry + 1;
	if(t >= st->entries) t = 0;
	lt = t;

	// find the current entry
	t++;
	if(t >= st->entries) t = 0;

	// the loop in dimension data
	count -= 2;
	for ( ; t != current_entry && count >= 0 ; lt = t++, count--) {
		if(t >= st->entries) t = 0;

		// if the last is empty, loop again
		if(!st->times[lt].tv_sec) continue;

		// check if we may exceed the buffer provided
		if((length - i) < 1024) break;

		// prefer the most recent last entries
		if(((count-pad) / group_count) > entries_to_show) continue;


		// ok. we will use this entry!
		// find how much usec since the previous entry

		usec = usecdiff(&st->times[t], &st->times[lt]);

		if(((count-pad) % group_count) == 0) {
			if(printed >= entries_to_show) break;

			// generate the local date time
			struct tm *tm = localtime(&st->times[t].tv_sec);
			if(!tm) { error("localtime() failed."); continue; }

			sprintf(dtm, "\"Date(%d, %d, %d, %d, %d, %d, %d)\"", tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec, (int)(st->times[t].tv_usec / 1000)); // datetime
			// strftime(dtm, 200, "\"Date(%Y, %m, %d, %H, %M, %S)\"", tm); // datetime
			// strftime(dtm, 200, "[%H, %M, %S, 0]", tm); // timeofday
 			i += sprintf(&b[i], "%s		{\"c\":[{\"v\":%s},", printed?"]},\n":"", dtm);

 			printed++;
 		}

		for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
			long long oldvalue = 0, value = 0;			// temp variable for storing data values

			if(rd->bytes == sizeof(long long)) {
				long long *dimension = rd->values;
				value    = dimension[t];
				oldvalue = dimension[lt];
			}
			else if(rd->bytes == sizeof(long)) {
				long *dimension = rd->values;
				value    = dimension[t];
				oldvalue = dimension[lt];
			}
			else if(rd->bytes == sizeof(int)) {
				int *dimension = rd->values;
				value    = dimension[t];
				oldvalue = dimension[lt];
			}
			else if(rd->bytes == sizeof(char)) {
				char *dimension = rd->values;
				value    = dimension[t];
				oldvalue = dimension[lt];
			}
			else fatal("Cannot produce JSON for size %d bytes dimension.", rd->bytes);

			switch(rd->type) {
				case RRD_DIMENSION_INCREMENTAL:
					if(oldvalue > value) value = 0;	// detect overflows and resets
					else value -= oldvalue;
					value = value * 1000000L / usec;
					break;

				default:
					break;
			}

			value = value * rd->multiplier / rd->divisor;

			group_counts[c]++;
			switch(group_method) {
				case GROUP_MAX:
					if(abs(value) > abs(group_values[c])) group_values[c] = value;
					break;

				default:
				case GROUP_AVERAGE:
					group_values[c] += value;
					if(((count-pad) % group_count) == 0) group_values[c] /= group_counts[c];
					break;
			}

			if(((count-pad) % group_count) == 0) {
				i += sprintf(&b[i], "{\"v\":%lld}%s", group_values[c], rd->next?",":"");
				group_values[c] = group_counts[c] = 0;
			}
		}
	}
	if(printed) i += sprintf(&b[i], "]}");
 	i += sprintf(&b[i], "\n	]\n}\n");

	pthread_mutex_unlock(&st->mutex);
 	return(i);
}

// ----------------------------------------------------------------------------
// listener (web server)

struct web_buffer {
	size_t size;	// allocation size of buffer
	size_t bytes;	// current data length in buffer
	size_t sent;	// current data length sent to output
	char *buffer;	// the buffer
	int contenttype;
	size_t rbytes; 	// if non-zero, the excepted size of ifd
	time_t date;	// the date this content has been generated
};

struct web_buffer *web_buffer_create(size_t size)
{
	struct web_buffer *b;

	debug(D_WEB_BUFFER, "Creating new web buffer of size %d.", size);

	b = calloc(1, sizeof(struct web_buffer));
	if(!b) {
		error("Cannot allocate a web_buffer.");
		return NULL;
	}

	b->buffer = calloc(1, size);
	if(!b->buffer) {
		error("Cannot allocate a buffer of size %u.", size);
		free(b);
		return NULL;
	}
	b->size = size;
	b->contenttype = CT_TEXT_PLAIN;
	return(b);
}

void web_buffer_free(struct web_buffer *b)
{
	debug(D_WEB_BUFFER, "Freeing web buffer of size %d.", b->size);

	if(b->buffer) free(b->buffer);
	free(b);
}

void web_buffer_increase(struct web_buffer *b, size_t free_size_required)
{
	size_t left = b->size - b->bytes;

	if(left >= free_size_required) return;
	size_t increase = free_size_required - left;

	debug(D_WEB_BUFFER, "Increasing data buffer from size %d to %d.", b->size, b->size + increase);

	b->buffer = realloc(b->buffer, b->size + free_size_required - left);
	if(!b->buffer) fatal("Failed to increase data buffer from size %d to %d.", b->size, b->size + increase);
	
	b->size += increase;
}

#define WEB_CLIENT_MODE_NORMAL		0
#define WEB_CLIENT_MODE_FILECOPY	1

struct web_client {
	unsigned long long id;
	char client_ip[101];

	int mode;
	int keepalive;

	struct sockaddr_in clientaddr;

	pthread_t thread;				// the thread servicing this client
	int obsolete;					// if set to 1, the listener will remove this client

	int ifd;
	int ofd;

	struct web_buffer *data;

	int wait_receive;
	int wait_send;

	char response_header[MAX_HTTP_HEADER_SIZE+1];

	struct web_client *prev;
	struct web_client *next;
} *web_clients = NULL;

unsigned long long web_clients_count = 0;

struct web_client *web_client_create(int listener)
{
	struct web_client *w;
	socklen_t addrlen;
	
	w = calloc(1, sizeof(struct web_client));
	if(!w) {
		error("Cannot allocate new web_client memory.");
		return NULL;
	}

	w->id = ++web_clients_count;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	addrlen = sizeof(w->clientaddr);
	w->ifd = accept(listener, (struct sockaddr *)&w->clientaddr, &addrlen);
	if (w->ifd == -1) {
		error("%llu: Cannot accept new incoming connection.", w->id);
		free(w);
		return NULL;
	}
	w->ofd = w->ifd;

	strncpy(w->client_ip, inet_ntoa(w->clientaddr.sin_addr), 100);
	w->client_ip[100] = '\0';

	syslog(LOG_NOTICE, "%llu: New client from %s.", w->id, w->client_ip);
	debug(D_WEB_CLIENT_ACCESS, "%llu: New web client from %s on socket %d.", w->id, w->client_ip, w->ifd);

	{
		int flag = 1; 
		if(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0) error("%llu: Cannot set SO_KEEPALIVE on socket.", w->id);
	}

	w->data = web_buffer_create(DEFAULT_DATA_BUFFER);
	if(!w->data) {
		close(w->ifd);
		free(w);
		return NULL;
	}

	w->wait_receive = 1;

	if(web_clients) web_clients->prev = w;
	w->next = web_clients;
	web_clients = w;

	return(w);
}

struct web_client *web_client_free(struct web_client *w)
{
	struct web_client *n = w->next;

	debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s.", w->id, inet_ntoa(w->clientaddr.sin_addr));

	if(w->prev)	w->prev->next = w->next;
	if(w->next) w->next->prev = w->prev;

	if(w == web_clients) web_clients = w->next;

	if(w->data) web_buffer_free(w->data);
	close(w->ifd);
	if(w->ofd != w->ifd) close(w->ofd);
	free(w);

	return(n);
}

int mysendfile(struct web_client *w, char *filename)
{
	debug(D_WEB_CLIENT, "%llu: Looking for file '%s'...", w->id, filename);

	// skip leading slashes
	while (filename[0] == '/') filename = &filename[1];

	// if the filename contain known paths, skip them
	     if(strncmp(filename, WEB_PATH_DATA "/",  strlen(WEB_PATH_DATA)  + 1) == 0) filename = &filename[strlen(WEB_PATH_DATA)  + 1];
	else if(strncmp(filename, WEB_PATH_GRAPH "/", strlen(WEB_PATH_GRAPH) + 1) == 0) filename = &filename[strlen(WEB_PATH_GRAPH) + 1];
	else if(strncmp(filename, WEB_PATH_FILE "/",  strlen(WEB_PATH_FILE)  + 1) == 0) filename = &filename[strlen(WEB_PATH_FILE)  + 1];

	// if the filename contains a / or a .., refuse to serve it
	if(strstr(filename, "/") != 0 || strstr(filename, "..") != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
		w->data->bytes = sprintf(w->data->buffer, "File '%s' cannot be served. Filenames cannot contain / or ..", filename);
		return 400;
	}

	// access the file in web/*
	char webfilename[FILENAME_MAX + 1];
	strcpy(webfilename, "web/");
	strncpy(&webfilename[4], filename, FILENAME_MAX - 4);
	webfilename[FILENAME_MAX] = '\0';

	// check if the file exists
	struct stat stat;
	if(lstat(webfilename, &stat) != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not found.", w->id, filename);
		w->data->bytes = sprintf(w->data->buffer, "File '%s' does not exist, or is not accessible.", filename);
		return 404;
	}

	// check if the file is owned by us
	if(stat.st_uid != getuid() && stat.st_uid != geteuid()) {
		error("%llu: File '%s' is owned by user %d (I run as user %d). Access Denied.", w->id, filename, stat.st_uid, getuid());
		w->data->bytes = sprintf(w->data->buffer, "Access to file '%s' is not permitted.", filename);
		return 403;
	}

	// open the file
	w->ifd = open(webfilename, O_NONBLOCK, O_RDONLY);
	if(w->ifd == -1) {
		w->ifd = w->ofd;

		if(errno == EBUSY || errno == EAGAIN) {
			error("%llu: File '%s' is busy, sending 307 Moved Temporarily to force retry.", w->id, filename);
			sprintf(w->response_header, "Location: /" WEB_PATH_FILE "/%s\r\n", filename);
			w->data->bytes = sprintf(w->data->buffer, "The file '%s' is currently busy. Please try again later.", filename);
			return 307;
		}
		else {
			error("%llu: Cannot open file '%s'.", w->id, filename);
			w->data->bytes = sprintf(w->data->buffer, "Cannot open file '%s'.", filename);
			return 404;
		}
	}
	
	syslog(LOG_NOTICE, "%llu: Sending file '%s' to client %s.", w->id, filename, w->client_ip);

	// pick a Content-Type for the file
	     if(strstr(filename, ".html") != NULL)	w->data->contenttype = CT_TEXT_HTML;
	else if(strstr(filename, ".js")   != NULL)	w->data->contenttype = CT_APPLICATION_X_JAVASCRIPT;
	else if(strstr(filename, ".css")  != NULL)	w->data->contenttype = CT_TEXT_CSS;
	else if(strstr(filename, ".xml")  != NULL)	w->data->contenttype = CT_TEXT_XML;
	else if(strstr(filename, ".xsl")  != NULL)	w->data->contenttype = CT_TEXT_XSL;
	else if(strstr(filename, ".txt")  != NULL)  w->data->contenttype = CT_TEXT_PLAIN;
	else if(strstr(filename, ".svg")  != NULL)  w->data->contenttype = CT_IMAGE_SVG_XML;
	else if(strstr(filename, ".ttf")  != NULL)  w->data->contenttype = CT_APPLICATION_X_FONT_TRUETYPE;
	else if(strstr(filename, ".otf")  != NULL)  w->data->contenttype = CT_APPLICATION_X_FONT_OPENTYPE;
	else if(strstr(filename, ".woff") != NULL)  w->data->contenttype = CT_APPLICATION_FONT_WOFF;
	else if(strstr(filename, ".eot")  != NULL)  w->data->contenttype = CT_APPLICATION_VND_MS_FONTOBJ;
	else w->data->contenttype = CT_APPLICATION_OCTET_STREAM;

	debug(D_WEB_CLIENT_ACCESS, "%llu: Sending file '%s' (%ld bytes, ifd %d, ofd %d).", w->id, filename, stat.st_size, w->ifd, w->ofd);

	w->mode = WEB_CLIENT_MODE_FILECOPY;
	w->wait_receive = 1;
	w->wait_send = 0;
	w->data->bytes = 0;
	w->data->buffer[0] = '\0';
	w->data->rbytes = stat.st_size;
	w->data->date = stat.st_mtim.tv_sec;

	return 200;
}

char *mystrsep(char **ptr, char *s)
{
	char *p = "";
	while ( !p[0] && *ptr ) p = strsep(ptr, s);
	return(p);
}

void web_client_reset(struct web_client *w)
{
	if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
		close(w->ifd);
		w->ifd = w->ofd;
	}

	w->data->contenttype = CT_TEXT_PLAIN;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	w->data->rbytes = 0;
	w->data->bytes = 0;
	w->data->sent = 0;

	w->data->buffer[0] = '\0';

	w->wait_receive = 1;
	w->wait_send = 0;
}

void web_client_process(struct web_client *w)
{
	int code = 500;
	int bytes;

	// check if we have an empty line (end of HTTP header)
	if(!strstr(w->data->buffer, "\r\n\r\n")) return;

	w->response_header[0] = '\0';
	w->data->date = time(NULL);
	w->wait_receive = 0;
	w->data->sent = 0;

	debug(D_WEB_DATA, "%llu: Processing data buffer of %d bytes: '%s'.", w->id, w->data->bytes, w->data->buffer);

	// check if the client requested keep-alive HTTP
	if(strcasestr(w->data->buffer, "Connection: keep-alive") == 0) w->keepalive = 0;
	else w->keepalive = 1;

	char *buf = w->data->buffer;
	char *tok = strsep(&buf, " \r\n");
	char *url = NULL;

	if(buf && strcmp(tok, "GET") == 0) {
		tok = strsep(&buf, " \r\n");
		url = url_decode(tok);
		debug(D_WEB_CLIENT, "%llu: Processing HTTP GET on url '%s'.", w->id, url);
	}
	else if (buf && strcmp(tok, "POST") == 0) {
		w->keepalive = 0;
		tok = strsep(&buf, " \r\n");
		url = url_decode(tok);

		debug(D_WEB_CLIENT, "%llu: I don't know how to handle POST with form data. Assuming it is a GET on url '%s'.", w->id, url);
	}

	if(url) {
		tok = mystrsep(&url, "/?&");

		debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

		if(strcmp(tok, WEB_PATH_DATA) == 0) { // "data"
			// the client is requesting rrd data

			// get the name of the data to show
			tok = mystrsep(&url, "/?&");
			debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

			// do we have such a data set?
			RRD_STATS *st = rrd_stats_find_byname(tok);
			if(!st) {
				// we don't have it
				// try to send a file with that name
				code = mysendfile(w, tok);
			}
			else {
				// we have it
				debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

				// how many entries does the client want?
				size_t lines = save_history;
				size_t group_count = 1;
				int group_method = GROUP_AVERAGE;

				if(url) {
					// parse the lines required
					tok = mystrsep(&url, "/?&");
					if(tok) lines = atoi(tok);
					if(lines < 5) lines = save_history;
				}
				if(url) {
					// parse the group count required
					tok = mystrsep(&url, "/?&");
					if(tok) group_count = atoi(tok);
					if(group_count < 1) group_count = 1;
					if(group_count > save_history / 20) group_count = save_history / 20;
				}
				if(url) {
					// parse the grouping method required
					tok = mystrsep(&url, "/?&");
					if(strcmp(tok, "max") == 0) group_method = GROUP_MAX;
					else if(strcmp(tok, "average") == 0) group_method = GROUP_AVERAGE;
					else debug(D_WEB_CLIENT, "%llu: Unknown group method '%s'", w->id, tok);
				}

				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (id %s, %d lines, %d group_count, %d group_method).", w->id, st->name, st->id, lines, group_count, group_method);

				code = 200;
				w->data->contenttype = CT_APPLICATION_JSON;
				w->data->bytes = rrd_stats_json(st, w->data->buffer, w->data->size, lines, group_count, group_method);
			}
		}
		else if(strcmp(tok, WEB_PATH_GRAPH) == 0) { // "graph"
			// the client is requesting an rrd graph

			// get the name of the data to show
			tok = mystrsep(&url, "/?&");
			debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

			// do we have such a data set?
			RRD_STATS *st = rrd_stats_find_byname(tok);
			if(!st) {
				// we don't have it
				// try to send a file with that name
				code = mysendfile(w, tok);
			}
			else {
				code = 200;
				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending %s.json of RRD_STATS...", w->id, st->name);
				w->data->contenttype = CT_APPLICATION_JSON;
				w->data->bytes = rrd_stats_graph_json(st, url, w->data->buffer, w->data->size);
			}
		}
		else if(strcmp(tok, "mirror") == 0) {
			code = 200;

			debug(D_WEB_CLIENT_ACCESS, "%llu: Mirroring...", w->id);

			// just leave the buffer as is
			// it will be copied back to the client
		}
		else if(strcmp(tok, "list") == 0) {
			code = 200;

			debug(D_WEB_CLIENT_ACCESS, "%llu: Sending list of RRD_STATS...", w->id);

			w->data->bytes = 0;
			RRD_STATS *st = root;

			for ( ; st ; st = st->next )
				w->data->bytes += sprintf(&w->data->buffer[w->data->bytes], "%s\n", st->name);
		}
		else if(strcmp(tok, "envlist") == 0) {
			code = 200;

			debug(D_WEB_CLIENT_ACCESS, "%llu: Sending envlist of RRD_STATS...", w->id);

			w->data->bytes = 0;
			RRD_STATS *st = root;

			for ( ; st ; st = st->next ) {
				w->data->bytes += sprintf(&w->data->buffer[w->data->bytes], "%s=%s\n", st->envtitle, st->usertitle);
				w->data->bytes += sprintf(&w->data->buffer[w->data->bytes], "%s=%s\n", st->envpriority, st->userpriority);
			}
		}
		else if(strcmp(tok, "all.json") == 0) {
			code = 200;
			debug(D_WEB_CLIENT_ACCESS, "%llu: Sending all.json of RRD_STATS...", w->id);
			w->data->contenttype = CT_APPLICATION_JSON;
			w->data->bytes = rrd_stats_all_json(w->data->buffer, w->data->size);
		}
		else if(strcmp(tok, WEB_PATH_FILE) == 0) { // "file"
			tok = mystrsep(&url, "/?&");
			if(tok && *tok) code = mysendfile(w, tok);
			else {
				code = 400;
				strcpy(w->data->buffer, "You have to give a filename to get.\r\n");
				w->data->bytes = strlen(w->data->buffer);
			}
		}
		else if(!tok[0]) {
			code = mysendfile(w, "index.html");
		}
		else {
			code = mysendfile(w, tok);
		}
	}
	else {
		if(buf) debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot understand '%s'.", w->id, buf);

		code = 500;
		strcpy(w->data->buffer, "I don't understand you...\r\n");
		w->data->bytes = strlen(w->data->buffer);
	}

	// prepare the HTTP response header
	debug(D_WEB_CLIENT, "%llu: Generating HTTP header with response %d.", w->id, code);

	char *content_type_string = "";
	switch(w->data->contenttype) {
		case CT_TEXT_HTML:
			content_type_string = "text/html";
			break;

		case CT_APPLICATION_XML:
			content_type_string = "application/xml";
			break;

		case CT_APPLICATION_JSON:
			content_type_string = "application/json";
			break;

		case CT_APPLICATION_X_JAVASCRIPT:
			content_type_string = "application/x-javascript";
			break;

		case CT_TEXT_CSS:
			content_type_string = "text/css";
			break;

		case CT_TEXT_XML:
			content_type_string = "text/xml";
			break;

		case CT_TEXT_XSL:
			content_type_string = "text/xsl";
			break;

		case CT_APPLICATION_OCTET_STREAM:
			content_type_string = "application/octet-stream";
			break;

		case CT_IMAGE_SVG_XML:
			content_type_string = "image/svg+xml";
			break;

		case CT_APPLICATION_X_FONT_TRUETYPE:
			content_type_string = "application/x-font-truetype";
			break;

		case CT_APPLICATION_X_FONT_OPENTYPE:
			content_type_string = "application/x-font-opentype";
			break;

		case CT_APPLICATION_FONT_WOFF:
			content_type_string = "application/font-woff";
			break;

		case CT_APPLICATION_VND_MS_FONTOBJ:
			content_type_string = "application/vnd.ms-fontobject";
			break;

		default:
		case CT_TEXT_PLAIN:
			content_type_string = "text/plain";
			break;
	}

	char *code_msg = "";
	switch(code) {
		case 200:
			code_msg = "OK";
			break;

		case 307:
			code_msg = "Temporary Redirect";
			break;

		case 400:
			code_msg = "Bad Request";
			break;

		case 403:
			code_msg = "Forbidden";
			break;

		case 404:
			code_msg = "Not Found";
			break;

		default:
			code_msg = "Internal Server Error";
			break;
	}

	char date[100];
	struct tm tm = *gmtime(&w->data->date);
	strftime(date, sizeof(date), "%a, %d %b %Y %H:%M:%S %Z", &tm);

	char custom_header[MAX_HTTP_HEADER_SIZE + 1] = "";
	if(w->response_header[0]) 
		strcpy(custom_header, w->response_header);

	size_t headerlen = 0;
	headerlen += sprintf(&w->response_header[headerlen],
		"HTTP/1.1 %d %s\r\n"
		"Connection: %s\r\n"
		"Server: Data Collector HTTP Server\r\n"
		"Content-Type: %s\r\n"
		"Access-Control-Allow-Origin: *\r\n"
		"Date: %s\r\n"
		, code, code_msg
		, w->keepalive?"keep-alive":"close"
		, content_type_string
		, date
		);

	if(custom_header[0])
		headerlen += sprintf(&w->response_header[headerlen], "%s", custom_header);

	if(w->mode == WEB_CLIENT_MODE_NORMAL) {
		headerlen += sprintf(&w->response_header[headerlen],
			"Expires: %s\r\n"
			"Cache-Control: private\r\n"
			, date
			);
	}

	// if we know the content length, put it
	if(w->data->bytes || w->data->rbytes)
		headerlen += sprintf(&w->response_header[headerlen],
			"Content-Length: %ld\r\n"
			, w->data->bytes?w->data->bytes:w->data->rbytes
			);
	else w->keepalive = 0;	// content-length is required for keep-alive

	headerlen += sprintf(&w->response_header[headerlen], "\r\n");

	// disable TCP_NODELAY, to buffer the header
	int flag = 0;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to disable TCP_NODELAY on socket.", w->id);

	// sent the HTTP header
	debug(D_WEB_CLIENT, "%llu: Sending response HTTP header of size %d.", w->id, headerlen);

	bytes = send(w->ofd, w->response_header, headerlen, 0);
	if(bytes != headerlen)
		error("%llu: HTTP Header failed to be sent (I sent %d bytes but the system sent %d bytes).", w->id, headerlen, bytes);

	// enable TCP_NODELAY, to send all data immediately at the next send()
	flag = 1;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to enable TCP_NODELAY on socket.", w->id);

	// enable sending immediately if we have data
	if(w->data->bytes) w->wait_send = 1;
	else w->wait_send = 0;

	// pretty logging
	switch(w->mode) {
		case WEB_CLIENT_MODE_NORMAL:
			debug(D_WEB_CLIENT, "%llu: Done preparing the response. Sending data (%d bytes) to client.", w->id, w->data->bytes);
			break;

		case WEB_CLIENT_MODE_FILECOPY:
			if(w->data->rbytes) {
				debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending data file of %d bytes to client.", w->id, w->data->rbytes);

				// utilize the kernel sendfile() for copying the file to the socket.
				// this block of code can be commented, without anything missing.
				// when it is commented, the program will copy the data using async I/O.
				{
					ssize_t len = sendfile(w->ofd, w->ifd, NULL, w->data->rbytes);
					if(len != w->data->rbytes) error("%llu: sendfile() should copy %ld bytes, but copied %ld. Falling back to manual copy.", w->id, w->data->rbytes, len);
					else web_client_reset(w);
				}
			}
			else
				debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending an unknown amount of bytes to client.", w->id);
			break;

		default:
			fatal("%llu: Unknown client mode %d.", w->id, w->mode);
			break;
	}
}

ssize_t web_client_send(struct web_client *w)
{
	ssize_t bytes;

	if(w->data->bytes - w->data->sent == 0) {
		// there is nothing to send

		debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

		// there can be two cases for this
		// A. we have done everything
		// B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

		if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->ifd != w->ofd && w->data->rbytes && w->data->rbytes > w->data->bytes) {
			// we have to wait, more data will come
			debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
			w->wait_send = 0;
			return(0);
		}

		if(w->keepalive == 0) {
			debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %ld bytes sent.", w->id, w->data->sent);
			errno = 0;
			return(-1);
		}

		web_client_reset(w);
		debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
		return(0);
	}

	bytes = send(w->ofd, &w->data->buffer[w->data->sent], w->data->bytes - w->data->sent, MSG_DONTWAIT);
	if(bytes > 0) {
		w->data->sent += bytes;
		debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, bytes);
	}
	else if(bytes == 0) debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, bytes);

	return(bytes);
}

ssize_t web_client_receive(struct web_client *w)
{
	// do we have any space for more data?
	web_buffer_increase(w->data, MIN_SOCKET_INPUT_DATA);

	ssize_t left = w->data->size - w->data->bytes;
	ssize_t bytes;

	if(w->mode == WEB_CLIENT_MODE_FILECOPY)
		bytes = read(w->ifd, &w->data->buffer[w->data->bytes], (left-1));
	else
		bytes = recv(w->ifd, &w->data->buffer[w->data->bytes], left-1, MSG_DONTWAIT);

	if(bytes > 0) {
		int old = w->data->bytes;
		w->data->bytes += bytes;
		w->data->buffer[w->data->bytes] = '\0';

		debug(D_WEB_CLIENT, "%llu: Received %d bytes.", w->id, bytes);
		debug(D_WEB_DATA, "%llu: Received data: '%s'.", w->id, &w->data->buffer[old]);

		if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
			w->wait_send = 1;
			if(w->data->rbytes && w->data->bytes >= w->data->rbytes) w->wait_receive = 0;
		}
	}
	else if(bytes == 0) {
		debug(D_WEB_CLIENT, "%llu: Out of input data.", w->id);

		// if we cannot read, it means we have an error on input.
		// if however, we are copying a file from ifd to ofd, we should not return an error.
		// in this case, the error should be generated when the file has been sent to the client.

		if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
			// we are copying data fron ifd to ofd
			// let it finish copying...
			w->wait_receive = 0;
			debug(D_WEB_CLIENT, "%llu: Disabling input.", w->id);
		}
		else {
			bytes = -1;
			errno = 0;
		}
	}

	return(bytes);
}


// --------------------------------------------------------------------------------------
// the thread of a single client

// 1. waits for input and output, using async I/O
// 2. it processes HTTP requests
// 3. it generates HTTP responses
// 4. it copies data from input to output if mode is FILECOPY

void *new_client(void *ptr)
{
	struct timeval tv;
	struct web_client *w = ptr;
	int retval;
	fd_set ifds, ofds, efds;
	int fdmax = 0;

	for(;;) {
		FD_ZERO (&ifds);
		FD_ZERO (&ofds);
		FD_ZERO (&efds);

		FD_SET(w->ifd, &efds);
		if(w->ifd != w->ofd)	FD_SET(w->ofd, &efds);
		if (w->wait_receive) {
			debug(D_WEB_CLIENT, "%llu: Waiting for input (fd %d).", w->id, w->ifd);
			FD_SET(w->ifd, &ifds);
			if(w->ifd > fdmax) fdmax = w->ifd;
		}
		if (w->wait_send) {
			debug(D_WEB_CLIENT, "%llu: Waiting for output (fd %d).", w->id, w->ofd);
			FD_SET(w->ofd, &ofds);
			if(w->ofd > fdmax) fdmax = w->ofd;
		}

		tv.tv_sec = 10;
		tv.tv_usec = 0;

		debug(D_WEB_CLIENT, "%llu: Waiting...", w->id);
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("%llu: LISTENER: select() failed.", w->id);
			continue;
		}
		else if(!retval) {
			// timeout
			w->obsolete = 1;
			return NULL;
		}

		if(FD_ISSET(w->ifd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on input socket (%s).", w->id, strerror(errno));
			w->obsolete = 1;
			return NULL;
		}

		if(FD_ISSET(w->ofd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on output socket (%s).", w->id, strerror(errno));
			w->obsolete = 1;
			return NULL;
		}

		if(w->wait_send && FD_ISSET(w->ofd, &ofds)) {
			if(web_client_send(w) < 0) {
				debug(D_WEB_CLIENT, "%llu: Closing client (input: %s).", w->id, strerror(errno));
				errno = 0;
				w->obsolete = 1;
				return NULL;
			}
		}

		if(w->wait_receive && FD_ISSET(w->ifd, &ifds)) {
			if(web_client_receive(w) < 0) {
				debug(D_WEB_CLIENT, "%llu: Closing client (output: %s).", w->id, strerror(errno));
				errno = 0;
				w->obsolete = 1;
				return NULL;
			}

			if(w->mode == WEB_CLIENT_MODE_NORMAL) web_client_process(w);
		}
	}
	debug(D_WEB_CLIENT, "%llu: done...", w->id);

	return NULL;
}

// --------------------------------------------------------------------------------------
// the main socket listener

// 1. it accepts new incoming requests on our port
// 2. creates a new web_client for each connection received
// 3. spawns a new pthread to serve the client (this is optimal for keep-alive clients)
// 4. cleans up old web_clients that their pthreads have been exited

void *socket_listen_main(void *ptr)
{
	struct web_client *w;
	struct timeval tv;
	int retval;

	int listener = create_listen_socket(listen_port);
	if(listener == -1) fatal("LISTENER: Cannot create listening socket on port 19999.");

	fd_set ifds, ofds, efds;
	int fdmax = listener;

	FD_ZERO (&ifds);
	FD_ZERO (&ofds);
	FD_ZERO (&efds);

	for(;;) {
		tv.tv_sec = 1;
		tv.tv_usec = 0;

		FD_SET(listener, &ifds);
		FD_SET(listener, &efds);

		// debug(D_WEB_CLIENT, "LISTENER: Waiting...");
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("LISTENER: select() failed.");
			continue;
		}
		else if(retval) {
			// check for new incoming connections
			if(FD_ISSET(listener, &ifds)) {
				w = web_client_create(listener);	

				if(pthread_create(&w->thread, NULL, new_client, w) != 0)
					error("%llu: failed to create new thread.");

				if(pthread_detach(w->thread) != 0)
					error("%llu: Cannot request detach of newly created thread.", w->id);
			}
			else debug(D_WEB_CLIENT, "LISTENER: select() didn't do anything.");
		}
		else debug(D_WEB_CLIENT, "LISTENER: select() timeout.");

		// cleanup unused clients
		for(w = web_clients; w ; w = w?w->next:NULL) {
			if(w->obsolete) {
				debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
				w = web_client_free(w);
			}
		}
	}

	error("LISTENER: exit!");

	close(listener);
	exit(2);

	return NULL;
}

// ----------------------------------------------------------------------------
// /proc/net/dev processor

int do_proc_net_dev() {
	char buffer[MAX_PROC_NET_DEV_LINE+1] = "";
	char name[MAX_PROC_NET_DEV_IFACE_NAME + 1] = RRD_TYPE_NET ".";
	unsigned long long rbytes, rpackets, rerrors, rdrops, rfifo, rframe, rcompressed, rmulticast;
	unsigned long long tbytes, tpackets, terrors, tdrops, tfifo, tcollisions, tcarrier, tcompressed;
	
	int r;
	char *p;
	
	FILE *fp = fopen("/proc/net/dev", "r");
	if(!fp) {
		error("Cannot read /proc/net/dev.");
		return 1;
	}
	
	// skip the first two lines
	p = fgets(buffer, MAX_PROC_NET_DEV_LINE, fp);
	p = fgets(buffer, MAX_PROC_NET_DEV_LINE, fp);
	
	// read the rest of the lines
	for(;1;) {
		char *c;
		p = fgets(buffer, MAX_PROC_NET_DEV_LINE, fp);
		if(!p) break;
		
		c = strchr(buffer, ':');
		if(c) *c = '\t';
		
		// if(DEBUG) printf("%s\n", buffer);
		r = sscanf(buffer, "%s\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\t%llu\n",
			&name[RRD_TYPE_NET_LEN + 1],
			&rbytes, &rpackets, &rerrors, &rdrops, &rfifo, &rframe, &rcompressed, &rmulticast,
			&tbytes, &tpackets, &terrors, &tdrops, &tfifo, &tcollisions, &tcarrier, &tcompressed);
		if(r == EOF) break;
		if(r != 17) error("Cannot read /proc/net/dev line. Expected 17 params, read %d.", r);
		else {
			RRD_STATS *st = rrd_stats_find(name);

			if(!st) {
				st = rrd_stats_create(name, name, save_history, "Network usage for ", "Bandwidth in kilobits/s", RRD_TYPE_NET);
				if(!st) {
					error("Cannot create RRD_STATS for interface %s.", name);
					continue;
				}

				if(!rrd_stats_dimension_add(st, "sent", "sent", sizeof(unsigned long long), -8, 1024, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "sent");

				if(!rrd_stats_dimension_add(st, "received", "received", sizeof(unsigned long long), 8, 1024, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "received");

			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", &rbytes);
			rrd_stats_dimension_set(st, "sent", &tbytes);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/diskstats processor

int do_proc_diskstats() {
	char buffer[MAX_PROC_DISKSTATS_LINE+1] = "";
	char name[MAX_PROC_DISKSTATS_DISK_NAME + 1] = RRD_TYPE_DISK ".";
	//                               1      2             3            4       5       6              7             8        9           10     11
	unsigned long long major, minor, reads, reads_merged, readsectors, readms, writes, writes_merged, writesectors, writems, currentios, iosms, wiosms;
	
	int r;
	char *p;
	
	FILE *fp = fopen("/proc/diskstats", "r");
	if(!fp) {
		error("Cannot read /proc/diskstats.");
		return 1;
	}
	
	for(;1;) {
		p = fgets(buffer, MAX_PROC_DISKSTATS_LINE, fp);
		if(!p) break;
		
		// if(DEBUG) printf("%s\n", buffer);
		r = sscanf(buffer, "%llu %llu %s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
			&major, &minor, &name[RRD_TYPE_DISK_LEN + 1],
			&reads, &reads_merged, &readsectors, &readms, &writes, &writes_merged, &writesectors, &writems, &currentios, &iosms, &wiosms
		);
		if(r == EOF) break;
		if(r != 14) error("Cannot read /proc/diskstats line. Expected 14 params, read %d.", r);
		else {
			switch(major) {
				case 3: // ide
				case 13: // 8bit ide
				case 22: // ide
				case 33: // ide
				case 34: // ide
				case 56: // ide
				case 57: // ide
				case 88: // ide
				case 89: // ide
				case 90: // ide
				case 91: // ide
					if(minor % 64) continue; // partitions
					break;

				case 8: // scsi disks
				case 65: // scsi disks
				case 66: // scsi disks
				case 67: // scsi disks
				case 68: // scsi disks
				case 69: // scsi disks
				case 70: // scsi disks
				case 71: // scsi disks
				case 72: // scsi disks
				case 73: // scsi disks
				case 74: // scsi disks
				case 75: // scsi disks
				case 76: // scsi disks
				case 77: // scsi disks
				case 78: // scsi disks
				case 79: // scsi disks
				case 80: // i2o
				case 81: // i2o
				case 82: // i2o
				case 83: // i2o
				case 84: // i2o
				case 85: // i2o
				case 86: // i2o
				case 87: // i2o
				case 101: // hyperdisk
				case 102: // compressed
				case 104: // scsi
				case 105: // scsi
				case 106: // scsi
				case 107: // scsi
				case 108: // scsi
				case 109: // scsi
				case 110: // scsi
				case 111: // scsi
				case 114: // bios raid
				case 116: // ram board
				case 128: // scsi
				case 129: // scsi
				case 130: // scsi
				case 131: // scsi
				case 132: // scsi
				case 133: // scsi
				case 134: // scsi
				case 135: // scsi
				case 153: // raid
					if(minor % 16) continue; // partitions
					break;

				case 9: // MDs
				case 43: // network block
				case 144: // nfs
				case 145: // nfs
				case 146: // nfs
					break;

				case 48: // RAID
				case 49: // RAID
				case 50: // RAID
				case 51: // RAID
				case 52: // RAID
				case 53: // RAID
				case 54: // RAID
				case 55: // RAID
				case 112: // RAID
				case 136: // RAID
				case 137: // RAID
				case 138: // RAID
				case 139: // RAID
				case 140: // RAID
				case 141: // RAID
				case 142: // RAID
				case 143: // RAID
					if(minor % 8) continue; // partitions
					break;

				default:
					continue;
			}

			RRD_STATS *st = rrd_stats_find(name);

			if(!st) {
				char ssfilename[FILENAME_MAX + 1];
				int sector_size = 512;

				sprintf(ssfilename, "/sys/block/%s/queue/hw_sector_size", &name[RRD_TYPE_DISK_LEN + 1]);
				FILE *fpss = fopen(ssfilename, "r");
				if(fpss) {
					char ssbuffer[1025];
					char *tmp = fgets(ssbuffer, 1024, fpss);

					if(tmp) {
						sector_size = atoi(tmp);
						if(sector_size <= 0) {
							error("Invalid sector size %d for device %s in %s. Assuming 512.", sector_size, name, ssfilename);
							sector_size = 512;
						}
					}
					else error("Cannot read data for sector size for device %s from %s. Assuming 512.", name, ssfilename);

					fclose(fpss);
				}
				else error("Cannot read sector size for device %s from %s. Assuming 512.", name, ssfilename);

				st = rrd_stats_create(name, name, save_history, "Disk usage for ", "I/O in kilobytes/s", RRD_TYPE_DISK);
				if(!st) {
					error("Cannot create RRD_STATS for disk %s.", name);
					continue;
				}

				if(!rrd_stats_dimension_add(st, "writes", "writes", sizeof(unsigned long long), sector_size * -1, 1024, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "writes");

				if(!rrd_stats_dimension_add(st, "reads", "reads", sizeof(unsigned long long), sector_size, 1024, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "reads");

			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "reads", &readsectors);
			rrd_stats_dimension_set(st, "writes", &writesectors);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/snmp processor

int do_proc_net_snmp() {
	char buffer[MAX_PROC_NET_SNMP_LINE+1] = "";

	FILE *fp = fopen("/proc/net/snmp", "r");
	if(!fp) {
		error("Cannot read /proc/net/snmp.");
		return 1;
	}

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
		if(!p) break;

		if(strncmp(p, "Ip: ", 4) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "Ip: ", 4) != 0) {
				error("Cannot read IP line from /proc/net/snmp.");
				break;
			}

			// see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
			unsigned long long Forwarding, DefaultTTL, InReceives, InHdrErrors, InAddrErrors, ForwDatagrams, InUnknownProtos, InDiscards, InDelivers,
				OutRequests, OutDiscards, OutNoRoutes, ReasmTimeout, ReasmReqds, ReasmOKs, ReasmFails, FragOKs, FragFails, FragCreates;

			int r = sscanf(&buffer[4], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&Forwarding, &DefaultTTL, &InReceives, &InHdrErrors, &InAddrErrors, &ForwDatagrams, &InUnknownProtos, &InDiscards, &InDelivers,
				&OutRequests, &OutDiscards, &OutNoRoutes, &ReasmTimeout, &ReasmReqds, &ReasmOKs, &ReasmFails, &FragOKs, &FragFails, &FragCreates);

			if(r == EOF) break;
			if(r != 19) error("Cannot read /proc/net/snmp IP line. Expected 19 params, read %d.", r);

			RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_SNMP ".ip");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP ".ip", RRD_TYPE_NET_SNMP ".ip", save_history, "IPv4 Packets", "Packets/s", RRD_TYPE_NET_SNMP);
				if(!st) {
					error("Cannot create RRD_STATS for %s.", RRD_TYPE_NET_SNMP ".ip");
					continue;
				}

				if(!rrd_stats_dimension_add(st, "forwarded", "forwarded", sizeof(unsigned long long), 1, 1, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "forwarded");

				if(!rrd_stats_dimension_add(st, "sent", "sent", sizeof(unsigned long long), -1, 1, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "sent");

				if(!rrd_stats_dimension_add(st, "received", "received", sizeof(unsigned long long), 1, 1, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "received");

			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutRequests);
			rrd_stats_dimension_set(st, "received", &InReceives);
			rrd_stats_dimension_set(st, "forwarded", &ForwDatagrams);
			rrd_stats_done(st);
		}
		else if(strncmp(p, "Tcp: ", 5) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "Tcp: ", 5) != 0) {
				error("Cannot read TCP line from /proc/net/snmp.");
				break;
			}

			unsigned long long RtoAlgorithm, RtoMin, RtoMax, MaxConn, ActiveOpens, PassiveOpens, AttemptFails, EstabResets, CurrEstab, InSegs, OutSegs, RetransSegs, InErrs, OutRsts;

			int r = sscanf(&buffer[5], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&RtoAlgorithm, &RtoMin, &RtoMax, &MaxConn, &ActiveOpens, &PassiveOpens, &AttemptFails, &EstabResets, &CurrEstab, &InSegs, &OutSegs, &RetransSegs, &InErrs, &OutRsts);

			if(r == EOF) break;
			if(r != 14) error("Cannot read /proc/net/snmp TCP line. Expected 14 params, read %d.", r);

			// see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
			RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcp");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP ".tcp", RRD_TYPE_NET_SNMP ".tcp", save_history, "IPv4 TCP Established Sockets", "Established Sockets", RRD_TYPE_NET_SNMP);
				if(!st) {
					error("Cannot create RRD_STATS for %s.", RRD_TYPE_NET_SNMP ".tcp");
					continue;
				}

				if(!rrd_stats_dimension_add(st, "established", "established", sizeof(unsigned long long), 1, 1, RRD_DIMENSION_ABSOLUTE))
					error("Cannot add RRD_STATS dimension %s.", "established");
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "established", &CurrEstab);
			rrd_stats_done(st);
		}
		else if(strncmp(p, "Udp: ", 5) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "Udp: ", 5) != 0) {
				error("Cannot read UDP line from /proc/net/snmp.");
				break;
			}

			unsigned long long InDatagrams, NoPorts, InErrors, OutDatagrams, RcvbufErrors, SndbufErrors;

			int r = sscanf(&buffer[5], "%llu %llu %llu %llu %llu %llu\n",
				&InDatagrams, &NoPorts, &InErrors, &OutDatagrams, &RcvbufErrors, &SndbufErrors);

			if(r == EOF) break;
			if(r != 6) error("Cannot read /proc/net/snmp UDP line. Expected 6 params, read %d.", r);

			// see http://net-snmp.sourceforge.net/docs/mibs/udp.html
			RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_SNMP ".udp");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP ".udp", RRD_TYPE_NET_SNMP ".udp", save_history, "IPv4 UDP Packets", "Packets/s", RRD_TYPE_NET_SNMP);
				if(!st) {
					error("Cannot create RRD_STATS for %s.", RRD_TYPE_NET_SNMP ".udp");
					continue;
				}

				if(!rrd_stats_dimension_add(st, "sent", "sent", sizeof(unsigned long long), -1, 1, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "sent");

				if(!rrd_stats_dimension_add(st, "received", "received", sizeof(unsigned long long), 1, 1, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", "received");
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", &InDatagrams);
			rrd_stats_dimension_set(st, "sent", &OutDatagrams);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc processor

void *proc_main(void *ptr)
{
	struct timeval last, now, tmp;

	gettimeofday(&last, NULL);
	last.tv_sec -= update_every;
	
	for(;1;) {
		unsigned long long usec, susec;
		gettimeofday(&now, NULL);
		
		// calculate the time it took for a full loop
		usec = usecdiff(&now, &last);
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: Last full loop took %llu usec.", usec);
		
		// BEGIN -- the job to be done
		do_proc_net_dev(usec);
		do_proc_diskstats(usec);
		do_proc_net_snmp(usec);
		// END -- the job is done
		
		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&tmp, NULL);
		usec = usecdiff(&tmp, &now);
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: This loop's work took %llu usec.", usec);
		
		if(usec < (update_every * 1000000)) susec = (update_every * 1000000) - usec;
		else susec = 0;
		
		// make sure we will wait at least 100ms
		if(susec < 100000) susec = 100000;
		
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: Sleeping for %llu usec.", susec);
		usleep(susec);
		
		// copy now to last
		last.tv_sec = now.tv_sec;
		last.tv_usec = now.tv_usec;
	}

	return NULL;
}

// ----------------------------------------------------------------------------
// /sbin/tc processor
// this requires the script tc-all.sh

#define PIPE_READ 0
#define PIPE_WRITE 1
#define TC_LINE_MAX 1024

struct tc_class {
	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];

	char parentid[RRD_STATS_NAME_MAX + 1];

	int hasparent;
	int isleaf;
	unsigned long long bytes;

	struct tc_class *next;
};

struct tc_device {
	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];

	struct tc_class *classes;
};

void tc_device_commit(struct tc_device *d)
{
	// we only need to add leaf classes
	struct tc_class *c, *x;
	for ( c = d->classes ; c ; c = c->next) {
		c->isleaf = 1;

		for ( x = d->classes ; x ; x = x->next) {
			if(strcmp(c->id, x->parentid) == 0) {
				c->isleaf = 0;
				x->hasparent = 1;
			}
		}
	}
	
	// debugging:
	// for ( c = d->classes ; c ; c = c->next) {
	//	if(c->isleaf && c->hasparent) debug(D_TC_LOOP, "Device %s, class %s, OK", d->name, c->id);
	//	else debug(D_TC_LOOP, "Device %s, class %s, IGNORE (isleaf: %d, hasparent: %d, parent: %s)", d->name, c->id, c->isleaf, c->hasparent, c->parentid);
	// }

	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent) break;
	}
	if(!c) {
		debug(D_TC_LOOP, "Ignoring TC device '%s'. No leaf classes.", d->name);
		return;
	}

	debug(D_TC_LOOP, "Committing TC device '%s'", d->name);

	RRD_STATS *st = rrd_stats_find(d->id);
	if(!st) {
		st = rrd_stats_create(d->id, d->name, save_history, "Class usage for ", "Bandwidth in kilobits/s", RRD_TYPE_TC);
		if(!st) {
			error("Cannot create RRD_STATS for interface %s.", d->name);
			return;
		}

		for ( c = d->classes ; c ; c = c->next) {
			if(c->isleaf && c->hasparent) {
				if(!rrd_stats_dimension_add(st, c->id, c->name, sizeof(unsigned long long), 8, 1024, RRD_DIMENSION_INCREMENTAL))
					error("Cannot add RRD_STATS dimension %s.", c->name);
			}
		}
	}
	else rrd_stats_next(st);

	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent)
			rrd_stats_dimension_set(st, c->id, &c->bytes);
	}
	rrd_stats_done(st);
}

void tc_device_set_class_name(struct tc_device *d, char *id, char *name)
{
	struct tc_class *c;
	for ( c = d->classes ; c ; c = c->next) {
		if(strcmp(c->id, id) == 0) {
			strncpy(c->name, name, RRD_STATS_NAME_MAX);
			c->name[RRD_STATS_NAME_MAX] = '\0';
			break;
		}
	}
}

void tc_device_set_device_name(struct tc_device *d, char *name)
{
	strcpy(d->name, RRD_TYPE_TC ".");
	strncpy(&d->name[RRD_TYPE_TC_LEN + 1], name, RRD_STATS_NAME_MAX - 3);
	d->name[RRD_STATS_NAME_MAX] = '\0';
}

struct tc_device *tc_device_create(char *name)
{
	struct tc_device *d;

	d = calloc(1, sizeof(struct tc_device));
	if(!d) return NULL;

	strcpy(d->name, RRD_TYPE_TC ".");
	strncpy(&d->name[RRD_TYPE_TC_LEN + 1], name, RRD_STATS_NAME_MAX - 3);
	d->name[RRD_STATS_NAME_MAX] = '\0';

	strcpy(d->id, d->name);

	return(d);
}

struct tc_class *tc_class_add(struct tc_device *n, char *id, char *parentid)
{
	struct tc_class *c;

	c = calloc(1, sizeof(struct tc_class));
	if(!c) return NULL;

	c->next = n->classes;
	n->classes = c;

	strncpy(c->id, id, RRD_STATS_NAME_MAX);
	strcpy(c->name, c->id);
	if(parentid) strncpy(c->parentid, parentid, RRD_STATS_NAME_MAX);

	return(c);
}

void tc_class_free(struct tc_class *c)
{
	if(c->next) tc_class_free(c->next);
	free(c);
}

void tc_device_free(struct tc_device *n)
{
	if(n->classes) tc_class_free(n->classes);
	free(n);
}

pid_t tc_child_pid = 0;
void *tc_main(void *ptr)
{
	char buffer[TC_LINE_MAX+1] = "";

	for(;1;) {
		FILE *fp;
		struct tc_device *device = NULL;
		struct tc_class *class = NULL;

		sprintf(buffer, "./tc-all.sh %d", update_every);
		fp = popen(buffer, "r");

		while(fgets(buffer, TC_LINE_MAX, fp) != NULL) {
			buffer[TC_LINE_MAX] = '\0';
			char *b = buffer, *p;
			// debug(D_TC_LOOP, "TC: read '%s'", buffer);

			p = strsep(&b, " \n");
			while (p && (*p == ' ' || *p == '\0')) p = strsep(&b, " \n");
			if(!p) continue;

			if(strcmp(p, "END") == 0) {
				if(device) {
					tc_device_commit(device);
					tc_device_free(device);
					device = NULL;
					class = NULL;
				}
			}
			else if(strcmp(p, "BEGIN") == 0) {
				if(device) tc_device_free(device);

				p = strsep(&b, " \n");
				if(p && *p) device = tc_device_create(p);
			}
			else if(device && (strcmp(p, "class") == 0)) {
				p = strsep(&b, " \n"); // the class: htb, fq_codel, etc
				char *id       = strsep(&b, " \n"); // the class major:minor
				char *parent   = strsep(&b, " \n"); // 'parent' or 'root'
				char *parentid = strsep(&b, " \n"); // the parent's id

				if(id && *id
					&& parent && *parent
					&& parentid && *parentid
					&& (
						(strcmp(parent, "parent") == 0 && parentid && *parentid)
						|| strcmp(parent, "root") == 0
					)) {
					if(strcmp(parent, "root") == 0) parentid = NULL;

					class = tc_class_add(device, id, parentid);
				}
			}
			else if(device && class && (strcmp(p, "Sent") == 0)) {
				p = strsep(&b, " \n");
				if(p && *p) class->bytes = atoll(p);
			}
			else if(device && (strcmp(p, "SETDEVICENAME") == 0)) {
				char *name = strsep(&b, " |\n");
				if(name && *name) tc_device_set_device_name(device, name);
			}
			else if(device && (strcmp(p, "SETCLASSNAME") == 0)) {
				char *name  = strsep(&b, " |\n");
				char *path  = strsep(&b, " |\n");
				char *id    = strsep(&b, " |\n");
				char *qdisc = strsep(&b, " |\n");
				if(id && *id && path && *path) tc_device_set_class_name(device, id, path);

				// prevent unused variables warning
				if(qdisc) qdisc = NULL;
				if(name) name = NULL;
			}
			else if((strcmp(p, "MYPID") == 0)) {
				char *id = strsep(&b, " \n");
				tc_child_pid = atol(id);
				debug(D_TC_LOOP, "Child PID is %d.", tc_child_pid);
			}
		}
		pclose(fp);

		sleep(1);
	}

	return NULL;
}

void bye(void)
{
	error("bye...");
	if(tc_child_pid) kill(tc_child_pid, SIGTERM);
	tc_child_pid = 0;
}

void sig_handler(int signo)
{
	switch(signo) {
		case SIGTERM:
		case SIGQUIT:
		case SIGINT:
		case SIGHUP:
			error("Signaled cleanup (signal %d).", signo);
			if(tc_child_pid) kill(tc_child_pid, SIGTERM);
			tc_child_pid = 0;
			exit(1);
			break;

		default:
			error("Signal %d received. Ignoring it.", signo);
			break;
	}
}

int become_user(const char *username)
{
	struct passwd *pw = getpwnam(username);
	if(!pw) {
		fprintf(stderr, "User %s is not present. Error: %s\n", username, strerror(errno));
		return -1;
	}
	if(setgid(pw->pw_gid) != 0) {
		fprintf(stderr, "Cannot switch to user's %s group (gid: %d). Error: %s\n", username, pw->pw_gid, strerror(errno));
		return -1;
	}
	if(setegid(pw->pw_gid) != 0) {
		fprintf(stderr, "Cannot effectively switch to user's %s group (gid: %d). Error: %s\n", username, pw->pw_gid, strerror(errno));
		return -1;
	}
	if(setuid(pw->pw_uid) != 0) {
		fprintf(stderr, "Cannot switch to user %s (uid: %d). Error: %s\n", username, pw->pw_uid, strerror(errno));
		return -1;
	}
	if(seteuid(pw->pw_uid) != 0) {
		fprintf(stderr, "Cannot effectively switch to user %s (uid: %d). Error: %s\n", username, pw->pw_uid, strerror(errno));
		return -1;
	}

	return(0);
}

void become_daemon()
{
	int i = fork();
	if(i == -1) {
		perror("cannot fork");
		exit(1);
	}
	if(i != 0) {
		exit(0); // the parent
	}

	// become session leader
	if (setsid() < 0)
		exit(2);

	signal(SIGCHLD, SIG_IGN);
	signal(SIGHUP, SIG_IGN);
	signal(SIGWINCH, SIG_IGN);

	// fork() again
	i = fork();
	if(i == -1) {
		perror("cannot fork");
		exit(1);
	}
	if(i != 0) {
		exit(0); // the parent
	}

	// Set new file permissions
	umask(0);

    // close all files
 	for(i = sysconf(_SC_OPEN_MAX); i > 0; i--)
 		close(i);

	silent = 1;
}

int main(int argc, char **argv)
{
	int i, daemon = 0;

	// parse  the arguments
	for(i = 1; i < argc ; i++) {
		if(strcmp(argv[i], "-l") == 0 && (i+1) < argc) {
			save_history = atoi(argv[i+1]);
			if(save_history < 5 || save_history > HISTORY_MAX) {
				fprintf(stderr, "Invalid save lines %d given. Defaulting to %d.\n", save_history, HISTORY);
				save_history = HISTORY;
			}
			else {
				debug(D_OPTIONS, "save lines set to %d.", save_history);
			}
			i++;
		}
		else if(strcmp(argv[i], "-u") == 0 && (i+1) < argc) {
			if(become_user(argv[i+1]) != 0) {
				fprintf(stderr, "Cannot become user %s.\n", argv[i+1]);
				exit(1);
			}
			else {
				debug(D_OPTIONS, "Successfully became user %s.", argv[i+1]);
			}
			i++;
		}
		else if(strcmp(argv[i], "-t") == 0 && (i+1) < argc) {
			update_every = atoi(argv[i+1]);
			if(update_every < 1 || update_every > 600) {
				fprintf(stderr, "Invalid update timer %d given. Defaulting to %d.\n", update_every, UPDATE_EVERY_MAX);
				update_every = UPDATE_EVERY;
			}
			else {
				debug(D_OPTIONS, "update timer set to %d.", update_every);
			}
			i++;
		}
		else if(strcmp(argv[i], "-p") == 0 && (i+1) < argc) {
			listen_port = atoi(argv[i+1]);
			if(listen_port < 1 || listen_port > 65535) {
				fprintf(stderr, "Invalid listen port %d given. Defaulting to %d.\n", listen_port, LISTEN_PORT);
				listen_port = LISTEN_PORT;
			}
			else {
				debug(D_OPTIONS, "update timer set to %d.", update_every);
			}
			i++;
		}
		else if(strcmp(argv[i], "-d") == 0) {
			daemon = 1;
			debug(D_OPTIONS, "Enabled daemon mode.");
		}
		else {
			fprintf(stderr, "Cannot understand option '%s'.\n", argv[i]);
			fprintf(stderr, "\nUSAGE: %s [-d] [-l LINES_TO_SAVE] [-u UPDATE_TIMER] [-p LISTEN_PORT].\n\n", argv[0]);
			fprintf(stderr, "  -d enable daemon mode (run in background).\n");
			fprintf(stderr, "  -l LINES_TO_SAVE can be from 5 to %d lines in JSON data. Default: %d.\n", HISTORY_MAX, HISTORY);
			fprintf(stderr, "  -t UPDATE_TIMER can be from 1 to %d seconds. Default: %d.\n", UPDATE_EVERY_MAX, UPDATE_EVERY);
			fprintf(stderr, "  -p LISTEN_PORT can be from 1 to %d. Default: %d.\n", 65535, LISTEN_PORT);
			fprintf(stderr, "  -u USERNAME can be any system username to run as. Default: none.\n");
			exit(1);
		}
	}

	// never become a problem
	if(nice(20) == -1) {
		fprintf(stderr, "Cannot lower my CPU priority. Error %s.\n", strerror(errno));
	}

	if(daemon) become_daemon();

	// open syslog
	openlog("netdata", LOG_PID, LOG_DAEMON);
	syslog(LOG_NOTICE, "netdata started.");

	// make sure we cleanup correctly
	atexit(bye);

	// catch all signals
	for (i = 1 ; i < 65 ;i++) signal(i,  sig_handler);
	

	pthread_t p_proc, p_tc;
	int r_proc, r_tc;

	// spawn a child to collect data
	r_proc  = pthread_create(&p_proc, NULL, proc_main, NULL);
	r_tc    = pthread_create(&p_tc,   NULL, tc_main,   NULL);

	// the main process - the web server listener
	//sleep(1);
	socket_listen_main(NULL);

	// wait for the childs to finish
	pthread_join(p_tc,  NULL);
	pthread_join(p_proc,  NULL);

	printf("TC            thread returns: %d\n", r_tc);
	printf("PROC NET DEV  thread returns: %d\n", r_proc);

	exit(0);
}
