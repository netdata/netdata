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
#include <zlib.h>
#include <malloc.h>

#define RRD_DIMENSION_ABSOLUTE					0
#define RRD_DIMENSION_INCREMENTAL				1
#define RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL 	2
#define RRD_DIMENSION_PCENT_OVER_ROW_TOTAL 		3

#define RRD_TYPE_NET				"net"
#define RRD_TYPE_NET_LEN			strlen(RRD_TYPE_NET)

#define RRD_TYPE_TC					"tc"
#define RRD_TYPE_TC_LEN				strlen(RRD_TYPE_TC)

#define RRD_TYPE_DISK				"disk"
#define RRD_TYPE_DISK_LEN			strlen(RRD_TYPE_DISK)

#define RRD_TYPE_NET_SNMP			"ipv4"
#define RRD_TYPE_NET_SNMP_LEN		strlen(RRD_TYPE_NET_SNMP)

#define RRD_TYPE_NET_STAT_CONNTRACK 	"conntrack"
#define RRD_TYPE_NET_STAT_CONNTRACK_LEN	strlen(RRD_TYPE_NET_STAT_CONNTRACK)

#define RRD_TYPE_NET_IPVS 			"ipvs"
#define RRD_TYPE_NET_IPVS_LEN		strlen(RRD_TYPE_NET_IPVS)

#define RRD_TYPE_STAT 				"cpu"
#define RRD_TYPE_STAT_LEN			strlen(RRD_TYPE_STAT)

#define WEB_PATH_FILE				"file"
#define WEB_PATH_DATA				"data"
#define WEB_PATH_DATASOURCE			"datasource"
#define WEB_PATH_GRAPH				"graph"

// type of JSON generations
#define DATASOURCE_JSON 0
#define DATASOURCE_GOOGLE_JSON 1
#define DATASOURCE_GOOGLE_JSONP 2

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
#define D_DEFLATE           0x00000200

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


#define DEBUG (D_WEB_CLIENT_ACCESS|D_LISTENER|D_RRD_STATS)
//#define DEBUG 0xffffffff
//#define DEBUG (0)

#define HOSTNAME_MAX 1024
char hostname[HOSTNAME_MAX + 1];

unsigned long long debug_flags = DEBUG;
char *debug_log = NULL;
int debug_fd = -1;

#define EXIT_FAILURE 1
#define LISTEN_BACKLOG 100

#define INITIAL_WEB_DATA_LENGTH 65536
#define WEB_DATA_LENGTH_INCREASE_STEP 65536
#define ZLIB_CHUNK 	16384

#define MAX_HTTP_HEADER_SIZE 16384

#define MAX_PROC_NET_SNMP_LINE 4096
#define MAX_PROC_NET_SNMP_NAME 1024

#define MAX_PROC_NET_STAT_CONNTRACK_LINE 4096
#define MAX_PROC_NET_STAT_CONNTRACK_NAME 1024

#define MAX_PROC_NET_IPVS_LINE 4096
#define MAX_PROC_NET_IPVS_NAME 1024

#define MAX_PROC_STAT_LINE 4096
#define MAX_PROC_STAT_NAME 1024


int silent = 0;
int save_history = HISTORY;
int update_every = UPDATE_EVERY;
int listen_port = LISTEN_PORT;


// ----------------------------------------------------------------------------
// helpers

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000L) + now->tv_usec) - ((last->tv_sec * 1000000L) + last->tv_usec)));
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

	if(debug_flags & type) {
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
	va_list args;

	if(!silent) {
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
	}

	va_start( args, fmt );
	vsyslog(LOG_ERR,  fmt, args );
	va_end( args );
}

#define fatal(args...)  fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	va_list args;

	if(!silent) {
		log_date();

		va_start( args, fmt );
		fprintf(stderr, "FATAL (%04lu@%-15.15s): ", line, function);
		vfprintf( stderr, fmt, args );
		va_end( args );

		perror(" # ");
		fprintf(stderr, "\n");
	}

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
	char id[RRD_STATS_NAME_MAX + 1];			// the id of this dimension (for internal identification)
	char name[RRD_STATS_NAME_MAX + 1];			// the name of this dimension (as presented to user)
	
	int issigned;								// if true, the values are signed

	size_t bytes;								// how many bytes each value has, e.g. sizeof(long)
	size_t entries;								// how many entries this dimension has
												// this should be the same to the entries of the data set

	int hidden;									// if set to non zero, this dimension will not be sent to the client

												// before presenting the value to the user:
	int type;									//  - first calculate a long double value based on this type of calculation
	long multiplier;							//  - then, multiple by this
	long divisor;								//  - then, divide by this

	void *values;								// the array of values, each value is 'bytes' in length

//	char **annotations;	
//	char *(*annotator)(long double previous, long double current);

	time_t last_updated;						// when was this dimension last updated

	struct rrd_dimension *next;					// linking of dimensions within the same data set
};
typedef struct rrd_dimension RRD_DIMENSION;

struct rrd_stats {
	pthread_mutex_t mutex;

	char id[RRD_STATS_NAME_MAX + 1];			// id of the data set
	char name[RRD_STATS_NAME_MAX + 1];			// name of the data set

	char type[RRD_STATS_NAME_MAX + 1];			// the type of graph RRD_TYPE_* (a category, for determining graphing options)
	char group[RRD_STATS_NAME_MAX + 1];			// the group of this data set (for grouping them together)

	char title[RRD_STATS_NAME_MAX + 1];			// title shown to user
	char units[RRD_STATS_NAME_MAX + 1];			// units of measurement

	char usertitle[RRD_STATS_NAME_MAX + 1];		// the title as taken from the environment variable
	char userpriority[RRD_STATS_NAME_MAX + 1];	// the priority as taken from the environment variable

	char envtitle[RRD_STATS_NAME_MAX + 1];		// the variable name for taking the title
	char envpriority[RRD_STATS_NAME_MAX + 1];	// the variable name for taking the priority

	size_t entries;								// total number of entries in the data set
	size_t current_entry;						// the entry that is currently being updated
												// it goes around in a round-robin fashion
	// size_t last_entry;

	time_t last_updated;						// when this data set was last updated

	int isdetail;								// if set, the data set should be considered as a detail of another
												// (the master data set should be the one that has the same group and is not detail)

	struct timeval *times;						// the time in microseconds each data entry was collected
	RRD_DIMENSION *dimensions;					// the actual data for every dimension

	struct rrd_stats *next;						// linking of rrd stats
};
typedef struct rrd_stats RRD_STATS;

RRD_STATS *root = NULL;
pthread_mutex_t root_mutex = PTHREAD_MUTEX_INITIALIZER;

RRD_STATS *rrd_stats_create(const char *type, const char *id, const char *name, const char *group, const char *title, const char *units, unsigned long entries)
{
	RRD_STATS *st = NULL;
	char *p;

	if(!id || !id[0]) {
		fatal("Cannot create rrd stats without an id.");
		return NULL;
	}

	debug(D_RRD_STATS, "Creating RRD_STATS for '%s.%s'.", type, id);

	st = calloc(1, sizeof(RRD_STATS));
	if(!st) {
		fatal("Cannot allocate memory for RRD_STATS %s.%s", type, id);
		return NULL;
	}

	st->times = calloc(entries, sizeof(struct timeval));
	if(!st->times) {
		free(st);
		fatal("Cannot allocate %lu entries of %lu bytes each for RRD_STATS.", st->entries, sizeof(struct timeval));
		return NULL;
	}
	
	// no need to terminate the strings after strncpy(), because of calloc()

	strncpy(st->id, type, RRD_STATS_NAME_MAX-1);
	strcat(st->id, ".");
	int len = strlen(st->id);
	strncpy(&st->id[len], id, RRD_STATS_NAME_MAX - len);

	if(name) {
		strncpy(st->name, type, RRD_STATS_NAME_MAX - 1);
		strcat(st->name, ".");
		len = strlen(st->name);
		strncpy(&st->name[len], name, RRD_STATS_NAME_MAX - len);
	}
	else strcpy(st->name, st->id);

	// replace illegal characters in name
	while((p = strchr(st->name, ' '))) *p = '_';
	while((p = strchr(st->name, '/'))) *p = '_';
	while((p = strchr(st->name, '?'))) *p = '_';
	while((p = strchr(st->name, '&'))) *p = '_';

	if(group) strncpy(st->group, group, RRD_STATS_NAME_MAX);
	else strcpy(st->group, st->id);

	strncpy(st->title, title, RRD_STATS_NAME_MAX);
	strncpy(st->units, units, RRD_STATS_NAME_MAX);
	strncpy(st->type, type, RRD_STATS_NAME_MAX);

	// check if there is a name for it in the environment
	sprintf(st->envtitle, "NETDATA_TITLE_%s", st->id);
	while((p = strchr(st->envtitle, '/'))) *p = '_';
	while((p = strchr(st->envtitle, '.'))) *p = '_';
	while((p = strchr(st->envtitle, '-'))) *p = '_';
	p = getenv(st->envtitle);

	if(p) strncpy(st->usertitle, p, RRD_STATS_NAME_MAX);
	else strncpy(st->usertitle, st->name, RRD_STATS_NAME_MAX);

	sprintf(st->envpriority, "NETDATA_PRIORITY_%s", st->id);
	while((p = strchr(st->envpriority, '/'))) *p = '_';
	while((p = strchr(st->envpriority, '.'))) *p = '_';
	while((p = strchr(st->envpriority, '-'))) *p = '_';
	p = getenv(st->envpriority);

	if(p) strncpy(st->userpriority, p, RRD_STATS_NAME_MAX);
	else strncpy(st->userpriority, st->name, RRD_STATS_NAME_MAX);

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

RRD_DIMENSION *rrd_stats_dimension_add(RRD_STATS *st, const char *id, const char *name, size_t bytes, int issigned, long multiplier, long divisor, int type, void *obsolete)
{
	RRD_DIMENSION *rd = NULL;

	debug(D_RRD_STATS, "Adding dimension '%s/%s'.", st->id, id);

	rd = calloc(1, sizeof(RRD_DIMENSION));
	if(!rd) {
		fatal("Cannot allocate RRD_DIMENSION %s/%s.", st->id, id);
		return NULL;
	}

	rd->bytes = bytes;
	rd->entries = st->entries;
	rd->multiplier = multiplier;
	rd->divisor = divisor;
	rd->type = type;
	rd->issigned = issigned;

	rd->values = calloc(rd->entries, rd->bytes);
	if(!rd->values) {
		free(rd);
		fatal("Cannot allocate %lu entries of %lu bytes each for RRD_DIMENSION values.", rd->entries, rd->bytes);
		return NULL;
	}

/*	rd->annotations = calloc(rd->entries, sizeof(char *));
	if(!rd->annotations) {
		free(rd->values);
		free(rd);
		fatal("Cannot allocate %lu entries of %lu bytes each for RRD_DIMENSION annotations.", rd->entries, sizeof(char *));
		return NULL;
	}
*/
	// no need to terminate the strings after strncpy(), because of calloc()

	strncpy(rd->id, id, RRD_STATS_NAME_MAX);

	if(name) strncpy(rd->name, name, RRD_STATS_NAME_MAX);
	else strncpy(rd->name, id, RRD_STATS_NAME_MAX);

	// append this dimension
	if(!st->dimensions)
		st->dimensions = rd;
	else {
		RRD_DIMENSION *td = st->dimensions;
		for(; td->next; td = td->next) ;
		td->next = rd;
	}

	//rd->annotator = annotator;

	return(rd);
}

void rrd_stats_dimension_free(RRD_DIMENSION *rd)
{
	if(rd->next) rrd_stats_dimension_free(rd->next);
	debug(D_RRD_STATS, "Removing dimension '%s'.", rd->name);
	// free(rd->annotations);
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

RRD_STATS *rrd_stats_find_bytype(const char *type, const char *id)
{
	char buf[RRD_STATS_NAME_MAX + 1];

	strncpy(buf, type, RRD_STATS_NAME_MAX - 1);
	buf[RRD_STATS_NAME_MAX - 1] = '\0';
	strcat(buf, ".");
	int len = strlen(buf);
	strncpy(&buf[len], id, RRD_STATS_NAME_MAX - len);
	buf[RRD_STATS_NAME_MAX] = '\0';

	return(rrd_stats_find(buf));
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

int rrd_stats_dimension_hide(RRD_STATS *st, const char *id)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rd->hidden = 1;
	return 0;
}

void rrd_stats_dimension_set_by_pointer(RRD_STATS *st, RRD_DIMENSION *rd, void *data, void *obsolete)
{
	rd->last_updated = st->last_updated;

	if(rd->issigned) {
		if(rd->bytes == sizeof(long long)) {
			long long *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else if(rd->bytes == sizeof(long)) {
			long *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else if(rd->bytes == sizeof(short int)) {
			short int *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else if(rd->bytes == sizeof(char)) {
			char *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else fatal("I don't know how to handle data of length %d (signed) bytes.", rd->bytes);
	}
	else {
		if(rd->bytes == sizeof(unsigned long long)) {
			unsigned long long *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else if(rd->bytes == sizeof(unsigned long)) {
			unsigned long *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else if(rd->bytes == sizeof(unsigned short int)) {
			unsigned short int *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else if(rd->bytes == sizeof(unsigned char)) {
			unsigned char *dimension = rd->values, *value = data;

			dimension[st->current_entry] = (*value);
		}
		else fatal("I don't know how to handle data of length %d (unsigned) bytes.", rd->bytes);
	}

	// clear any previous annotations
/*	if(rd->annotations[st->current_entry]) {
		free(rd->annotations[st->current_entry]);
		rd->annotations[st->current_entry] = NULL;
	}

	// set the new annotation
	if(annotation)
		rd->annotations[st->current_entry] = strdup(annotation);
*/
}

int rrd_stats_dimension_set(RRD_STATS *st, char *id, void *data, void *obsolete)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rrd_stats_dimension_set_by_pointer(st, rd, data, obsolete);
	return 0;
}

unsigned long long rrd_stats_next(RRD_STATS *st)
{
	struct timeval *now, *old;

	pthread_mutex_lock(&st->mutex);

	old = &st->times[st->current_entry];

	// st->current_entry should never be outside the array
	// or, the parallel threads may end up crashing
	// st->last_entry = st->current_entry;
	st->current_entry = ((st->current_entry + 1) >= st->entries) ? 0 : st->current_entry + 1;

	now = &st->times[st->current_entry];
	gettimeofday(now, NULL);

	st->last_updated = now->tv_sec;

	// leave mutex locked

	return usecdiff(now, old);
}

void rrd_stats_done(RRD_STATS *st)
{
	RRD_DIMENSION *rd, *last;

	// find if there are any obsolete dimensions (not updated recently)
	for( rd = st->dimensions, last = NULL ; rd ; ) {
		if((rd->last_updated + (10 * update_every)) < st->last_updated) { // remove it only it is not updated in 10 seconds
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
		else if((rd->last_updated + update_every) < st->last_updated) {
			debug(D_RRD_STATS, "Clearing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

			unsigned char zero[rd->bytes];
			int i;
			for(i = 0; i < rd->bytes ; i++) zero[i] = 0;

			rrd_stats_dimension_set_by_pointer(st, rd, &zero, NULL);
		}

		last = rd;
		rd = rd->next;
	}

	pthread_mutex_unlock(&st->mutex);
}



// ----------------------------------------------------------------------------
// web buffer

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
	if(increase < WEB_DATA_LENGTH_INCREASE_STEP) increase = WEB_DATA_LENGTH_INCREASE_STEP;

	debug(D_WEB_BUFFER, "Increasing data buffer from size %d to %d.", b->size, b->size + increase);

	b->buffer = realloc(b->buffer, b->size + increase);
	if(!b->buffer) fatal("Failed to increase data buffer from size %d to %d.", b->size, b->size + increase);
	
	b->size += increase;
}

#define WEB_CLIENT_MODE_NORMAL		0
#define WEB_CLIENT_MODE_FILECOPY	1

#define URL_MAX 8192

struct web_client {
	unsigned long long id;
	char client_ip[101];
	char last_url[URL_MAX+1];

	struct timeval tv_in, tv_ready;

	int mode;
	int keepalive;

	struct sockaddr_in clientaddr;

	pthread_t thread;				// the thread servicing this client
	int obsolete;					// if set to 1, the listener will remove this client

	int ifd;
	int ofd;

	struct web_buffer *data;

	int zoutput;					// if set to 1, web_client_send() will send compressed data
	z_stream zstream;				// zlib stream for sending compressed output to client
	Bytef zbuffer[ZLIB_CHUNK];		// temporary buffer for storing compressed output
	size_t zsent;					// the compressed bytes we have sent to the client
	size_t zhave;					// the compressed bytes that we have to send
	int zinitialized;

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

	// syslog(LOG_NOTICE, "%llu: New client from %s.", w->id, w->client_ip);
	debug(D_WEB_CLIENT_ACCESS, "%llu: New web client from %s on socket %d.", w->id, w->client_ip, w->ifd);

	{
		int flag = 1; 
		if(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0) error("%llu: Cannot set SO_KEEPALIVE on socket.", w->id);
	}

	w->data = web_buffer_create(INITIAL_WEB_DATA_LENGTH);
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

#define GROUP_AVERAGE	0
#define GROUP_MAX 		1

// find the oldest entry in the data, skipping all empty slots
size_t rrd_stats_first_entry(RRD_STATS *st)
{
	size_t first_entry = st->current_entry + 1;
	if(first_entry >= st->entries) first_entry = 0;

	while(st->times[first_entry].tv_sec == 0 && first_entry != st->current_entry) {
		first_entry++;
		if(first_entry >= st->entries) first_entry = 0;
	}

	return first_entry;
}

unsigned long rrd_stats_one_json(RRD_STATS *st, char *options, struct web_buffer *wb)
{
	web_buffer_increase(wb, 16384);

	pthread_mutex_lock(&st->mutex);

	size_t first_entry = rrd_stats_first_entry(st);
	time_t first_entry_t = st->times[first_entry].tv_sec;

	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t{\n");
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"id\" : \"%s\",\n", st->id);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"name\" : \"%s\",\n", st->name);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"type\" : \"%s\",\n", st->type);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"group_tag\" : \"%s\",\n", st->group);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"title\" : \"%s\",\n", st->title);

	if(strcmp(st->id, st->usertitle) == 0 || strcmp(st->name, st->usertitle) == 0)
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"usertitle\" : \"%s (%s)\",\n", st->title, st->name);
	else 
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"usertitle\" : \"%s %s (%s)\",\n", st->title, st->usertitle, st->name);

	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"userpriority\" : \"%s\",\n", st->userpriority);
	//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"envtitle\" : \"%s\",\n", st->envtitle);
	//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"envpriority\" : \"%s\",\n", st->envpriority);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"units\" : \"%s\",\n", st->units);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"url\" : \"/data/%s/%s\",\n", st->name, options?options:"");
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"entries\" : %ld,\n", st->entries);
	//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"first_entry\" : %ld,\n", first_entry);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"first_entry_t\" : %ld,\n", first_entry_t);
	//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"last_entry\" : %ld,\n", st->current_entry);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"last_entry_t\" : %lu,\n", st->last_updated);
	//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"last_entry_secs_ago\" : %lu,\n", time(NULL) - st->last_updated);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"update_every\" : %d,\n", update_every);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"isdetail\" : %d,\n", st->isdetail);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"dimensions\" : [\n");

	unsigned long memory = sizeof(RRD_STATS) + (sizeof(struct timeval) * st->entries);

	RRD_DIMENSION *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
		unsigned long rdmem = sizeof(RRD_DIMENSION) + (rd->bytes * rd->entries);
		memory += rdmem;

		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t{\n");
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"id\" : \"%s\",\n", rd->id);
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"name\" : \"%s\",\n", rd->name);
		//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"bytes\" : %ld,\n", rd->bytes);
		//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"entries\" : %ld,\n", rd->entries);
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"isSigned\" : %d,\n", rd->issigned);
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"isHidden\" : %d,\n", rd->hidden);

		switch(rd->type) {
			case RRD_DIMENSION_INCREMENTAL:
				wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"type\" : \"%s\",\n", "incremental");
				break;

			case RRD_DIMENSION_ABSOLUTE:
				wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"type\" : \"%s\",\n", "absolute");
				break;

			case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
				wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"type\" : \"%s\",\n", "percent on incremental total");
				break;

			case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
				wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"type\" : \"%s\",\n", "percent on absolute total");
				break;

			default:
				wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"type\" : %d,\n", rd->type);
				break;
		}

		//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"multiplier\" : %ld,\n", rd->multiplier);
		//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"divisor\" : %ld,\n", rd->divisor);
		//wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"last_entry_t\" : %lu,\n", rd->last_updated);
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t\t\"memory\" : %lu\n", rdmem);
		wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\t}%s\n", rd->next?",":"");
	}

	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t],\n");
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t\t\"memory\" : %lu\n", memory);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\t}");

	pthread_mutex_unlock(&st->mutex);
	return memory;
}

#define RRD_GRAPH_JSON_HEADER "{\n\t\"charts\": [\n"
#define RRD_GRAPH_JSON_FOOTER "\n\t]\n}\n"

void rrd_stats_graph_json(RRD_STATS *st, char *options, struct web_buffer *wb)
{
	web_buffer_increase(wb, 16384);

	wb->bytes += sprintf(&wb->buffer[wb->bytes], RRD_GRAPH_JSON_HEADER);
	rrd_stats_one_json(st, options, wb);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], RRD_GRAPH_JSON_FOOTER);
}

void rrd_stats_all_json(struct web_buffer *wb)
{
	web_buffer_increase(wb, 150000);

	unsigned long memory = 0;
	size_t c;
	RRD_STATS *st;

	wb->bytes += sprintf(&wb->buffer[wb->bytes], RRD_GRAPH_JSON_HEADER);

	for(st = root, c = 0; st ; st = st->next, c++) {
		if(c) wb->bytes += sprintf(&wb->buffer[wb->bytes], "%s", ",\n");
		memory += rrd_stats_one_json(st, NULL, wb);
	}
	
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\n\t],\n");
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\"hostname\": \"%s\",\n", hostname);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\"update_every\": %d,\n", update_every);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\"history\": %d,\n", save_history);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\t\"memory\": %lu\n", memory);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "}\n");
}

long double rrd_stats_dimension_get(RRD_DIMENSION *rd, size_t position)
{
	if(rd->issigned) {
		if(rd->bytes == sizeof(long long)) {
			long long *dimension = rd->values;
			return(dimension[position]);
		}
		else if(rd->bytes == sizeof(long)) {
			long *dimension = rd->values;
			return(dimension[position]);
		}
		else if(rd->bytes == sizeof(short int)) {
			short int *dimension = rd->values;
			return(dimension[position]);
		}
		else if(rd->bytes == sizeof(char)) {
			char *dimension = rd->values;
			return(dimension[position]);
		}
		else fatal("Cannot produce JSON for size %d bytes (signed) dimension.", rd->bytes);
	}
	else {
		if(rd->bytes == sizeof(unsigned long long)) {
			unsigned long long *dimension = rd->values;
			return(dimension[position]);
		}
		else if(rd->bytes == sizeof(unsigned long)) {
			unsigned long *dimension = rd->values;
			return(dimension[position]);
		}
		else if(rd->bytes == sizeof(unsigned short int)) {
			unsigned short int *dimension = rd->values;
			return(dimension[position]);
		}
		else if(rd->bytes == sizeof(unsigned char)) {
			unsigned char *dimension = rd->values;
			return(dimension[position]);
		}
		else fatal("Cannot produce JSON for size %d bytes (unsigned) dimension.", rd->bytes);
	}

	return(0);
}

unsigned long rrd_stats_json(int type, RRD_STATS *st, struct web_buffer *wb, size_t entries_to_show, size_t group_count, int group_method, time_t after, time_t before)
{
	pthread_mutex_lock(&st->mutex);

	unsigned long last_timestamp = 0;

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

	// check the options
	if(entries_to_show < 1) entries_to_show = 1;
	if(group_count < 1) group_count = 1;
	// if(group_count > st->entries / 20) group_count = st->entries / 20;

	long printed = 0;			// the lines of JSON data we have generated so far

	long stop_entry, current_entry = st->current_entry;

	int c = 0;					// counter for dimension loops
	int dimensions = 0;			// the total number of dimensions present

	unsigned long long usec = 0;// usec between the entries
	char dtm[201];				// temp variable for storing dates

	int we_need_totals = 0;		// if set, we should calculate totals for all dimensions

	// find how many dimensions we have
	RRD_DIMENSION *rd;
	for( rd = st->dimensions ; rd ; rd = rd->next) {
		dimensions++;
		if(rd->type == RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL || rd->type == RRD_DIMENSION_PCENT_OVER_ROW_TOTAL) we_need_totals++;
	}

	if(!dimensions) {
		pthread_mutex_unlock(&st->mutex);
		wb->bytes = sprintf(wb->buffer, "No dimensions yet.");
		return 0;
	}

	int annotation_count = 0;

	// temp for the printable values
	long double print_values[dimensions];
	int print_hidden[dimensions];

	// temporary storage to keep track of group values and counts
	long double group_values[dimensions];
	int group_counts[dimensions];
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++)
		group_values[c] = group_counts[c] = 0;

	// print the labels
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "{\n	%scols%s:\n	[\n", kq, kq);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "		{%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);

	// print the header for each dimension
	// and update the print_hidden array for the dimensions that should be hidden
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
		if(rd->hidden)
			print_hidden[c] = 1;
		else {
			print_hidden[c] = 0;
			wb->bytes += sprintf(&wb->buffer[wb->bytes], ",\n		{%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, rd->name, sq, kq, kq, sq, sq, kq, kq, sq, sq);
		}
	}

	// print the begin of row data
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\n	],\n	%srows%s:\n	[\n", kq, kq);

	// make sure current_entry is within limits
	if(current_entry < 0 || current_entry >= st->entries) current_entry = 0;
	if(before == 0) before = st->times[current_entry].tv_sec;

	// find the oldest entry of the round-robin
	stop_entry = rrd_stats_first_entry(st);

	// skip the oldest, to have incremental data
	if(after == 0) after = st->times[stop_entry].tv_sec;

	// the minimum line length we expect
	int line_size = 4096 + (dimensions * 200);

	char overflow_annotation[201];
	int overflow_annotation_len = snprintf(overflow_annotation, 200, ",{%sv%s:%sRESET OR OVERFLOW%s},{%sv%s:%sThe counters have been wrapped.%s}", kq, kq, sq, sq, kq, kq, sq, sq);
	overflow_annotation[200] = '\0';

	char normal_annotation[201];
	int normal_annotation_len = snprintf(normal_annotation, 200, ",{%sv%s:null},{%sv%s:null}", kq, kq, kq, kq);
	normal_annotation[200] = '\0';

	// to allow grouping on the same values, we need a pad
	long pad = before % group_count;

	// checks for debuging
	if(before < after)
		debug(D_RRD_STATS, "WARNING: %s The newest value in the database (%lu) is earlier than the oldest (%lu)", st->name, before, after);

	if((before - after) > st->entries * update_every)
		debug(D_RRD_STATS, "WARNING: %s The time difference between the oldest and the newest entries (%lu) is higher than the capacity of the database (%lu)", st->name, before - after, st->entries * update_every);

	// loop in dimension data
	int annotate_reset = 0;
	long t = current_entry, lt = current_entry - 1, count; // t = the current entry, lt = the last entry of data
	if(lt < 0) lt = st->entries - 1;
	for (count = printed = 0; t != stop_entry ; t = lt--) {
		int print_this = 0;

		if(lt < 0) lt = st->entries - 1;

		// make sure we return data in the proper time range
		if(st->times[t].tv_sec < after || st->times[t].tv_sec > before) continue;
		count++;

		// ok. we will use this entry!
		// find how much usec since the previous entry
		usec = usecdiff(&st->times[t], &st->times[lt]);

		if(((count - pad) % group_count) == 0) {
			if(printed >= entries_to_show) {
				// debug(D_RRD_STATS, "Already printed all rows. Stopping.");
				break;
			}

			// check if we may exceed the buffer provided
			web_buffer_increase(wb, line_size);

			// generate the local date time
			struct tm *tm = localtime(&st->times[t].tv_sec);
			if(!tm) { error("localtime() failed."); continue; }

			if(st->times[t].tv_sec > last_timestamp) last_timestamp = st->times[t].tv_sec;

			sprintf(dtm, "Date(%d, %d, %d, %d, %d, %d, %d)", tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec, (int)(st->times[t].tv_usec / 1000)); // datetime
			wb->bytes += sprintf(&wb->buffer[wb->bytes], "%s		{%sc%s:[{%sv%s:%s%s%s}", printed?"]},\n":"", kq, kq, kq, kq, sq, dtm, sq);

			print_this = 1;
		}

		// if we need a PCENT_OVER_TOTAL, calculate the totals for the current and the last
		long double total = 0, oldtotal = 0;
		if(we_need_totals) {
			for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
				total    += rrd_stats_dimension_get(rd, t);
				oldtotal += rrd_stats_dimension_get(rd, lt);
			}
		}

		for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
			long double oldvalue, value;			// temp variable for storing data values

			value    = rrd_stats_dimension_get(rd, t);
			oldvalue = rrd_stats_dimension_get(rd, lt);
			
			switch(rd->type) {
				case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
					value = 100.0 * (value - oldvalue) / (total - oldtotal);
					break;

				case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
					value = 100.0 * value / total;
					break;

				case RRD_DIMENSION_INCREMENTAL:
					if(oldvalue > value) {
						annotate_reset = 1;
						value = 0;
					}
					else value -= oldvalue;

					value = value * 1000000.0 / (long double)usec;
					break;

				default:
					break;
			}

			value = value * (long double)rd->multiplier / (long double)rd->divisor;

//			if(rd->options & RRD_OPTION_ANNOTATE_NOT_ZERO) {
//				annotation_add(my_annotations, rd->name, "%s = %0.1lf", value);
//			}

			group_counts[c]++;
			switch(group_method) {
				case GROUP_MAX:
					if(abs(value) > abs(group_values[c])) group_values[c] = value;
					break;

				default:
				case GROUP_AVERAGE:
					group_values[c] += value;
					if(print_this) group_values[c] /= group_counts[c];
					break;
			}

			if(print_this) {
				print_values[c] = group_values[c];
				group_values[c] = 0;
				group_counts[c] = 0;
			}
		}

		if(print_this) {
			if(annotate_reset) {
				annotation_count++;
				strcpy(&wb->buffer[wb->bytes], overflow_annotation);
				wb->bytes += overflow_annotation_len;
				annotate_reset = 0;
			}
			else {
				strcpy(&wb->buffer[wb->bytes], normal_annotation);
				wb->bytes += normal_annotation_len;
			}

			for(c = 0 ; c < dimensions ; c++) {
				if(!print_hidden[c])
					wb->bytes += sprintf(&wb->buffer[wb->bytes], ",{%sv%s:%0.1Lf}", kq, kq, print_values[c]);
			}

			printed++;
		}
	}

	if(printed) wb->bytes += sprintf(&wb->buffer[wb->bytes], "]}");
	wb->bytes += sprintf(&wb->buffer[wb->bytes], "\n	]\n}\n");

	debug(D_RRD_STATS, "RRD_STATS_JSON: %s Generated %ld rows, total %ld bytes", st->name, printed, wb->bytes);

	pthread_mutex_unlock(&st->mutex);
	return last_timestamp;
}

int mysendfile(struct web_client *w, char *filename)
{
	debug(D_WEB_CLIENT, "%llu: Looking for file '%s'...", w->id, filename);

	// skip leading slashes
	while (filename[0] == '/') filename = &filename[1];

	// if the filename contain known paths, skip them
		 if(strncmp(filename, WEB_PATH_DATA       "/", strlen(WEB_PATH_DATA)       + 1) == 0) filename = &filename[strlen(WEB_PATH_DATA)       + 1];
	else if(strncmp(filename, WEB_PATH_DATASOURCE "/", strlen(WEB_PATH_DATASOURCE) + 1) == 0) filename = &filename[strlen(WEB_PATH_DATASOURCE) + 1];
	else if(strncmp(filename, WEB_PATH_GRAPH      "/", strlen(WEB_PATH_GRAPH)      + 1) == 0) filename = &filename[strlen(WEB_PATH_GRAPH)      + 1];
	else if(strncmp(filename, WEB_PATH_FILE       "/", strlen(WEB_PATH_FILE)       + 1) == 0) filename = &filename[strlen(WEB_PATH_FILE)       + 1];

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
	
	//syslog(LOG_NOTICE, "%llu: Sending file '%s' to client %s.", w->id, filename, w->client_ip);

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
	struct timeval tv;
	gettimeofday(&tv, NULL);

	long sent = w->zoutput?w->zstream.total_out:(w->mode == WEB_CLIENT_MODE_FILECOPY)?w->data->rbytes:w->data->bytes;
	long size = (w->mode == WEB_CLIENT_MODE_FILECOPY)?w->data->rbytes:w->data->bytes;
	
	if(w->last_url[0]) syslog(LOG_NOTICE, "%llu: (sent/all = %ld/%ld bytes %0.0f%%, prep/sent/total = %0.2f/%0.2f/%0.2f ms) %s: '%s'",
		w->id,
		sent, size, -((size>0)?((float)(size-sent)/(float)size * 100.0):0.0),
		(float)usecdiff(&w->tv_ready, &w->tv_in) / 1000.0,
		(float)usecdiff(&tv, &w->tv_ready) / 1000.0,
		(float)usecdiff(&tv, &w->tv_in) / 1000.0,
		(w->mode == WEB_CLIENT_MODE_FILECOPY)?"filecopy":"data",
		w->last_url
		);

	debug(D_WEB_CLIENT, "%llu: Reseting client.", w->id);

	if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
		debug(D_WEB_CLIENT, "%llu: Closing filecopy input file.", w->id);
		close(w->ifd);
		w->ifd = w->ofd;
	}

	w->last_url[0] = '\0';

	w->data->contenttype = CT_TEXT_PLAIN;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	w->data->rbytes = 0;
	w->data->bytes = 0;
	w->data->sent = 0;

	w->response_header[0] = '\0';
	w->data->buffer[0] = '\0';

	w->wait_receive = 1;
	w->wait_send = 0;

	// if we had enabled compression, release it
	if(w->zinitialized) {
		debug(D_DEFLATE, "%llu: Reseting compression.", w->id);
		deflateEnd(&w->zstream);
		w->zoutput = 0;
		w->zsent = 0;
		w->zhave = 0;
		w->zstream.avail_in = 0;
		w->zstream.avail_out = 0;
		w->zstream.total_in = 0;
		w->zstream.total_out = 0;
		w->zinitialized = 0;
	}
}

void web_client_enable_deflate(struct web_client *w) {
	if(w->zinitialized == 1) {
		error("%llu: Compression has already be initialized for this client.", w->id);
		return;
	}

	if(w->data->sent) {
		error("%llu: Cannot enable compression in the middle of a conversation.", w->id);
		return;
	}

	w->zstream.zalloc = Z_NULL;
	w->zstream.zfree = Z_NULL;
	w->zstream.opaque = Z_NULL;

	w->zstream.next_in = (Bytef *)w->data->buffer;
	w->zstream.avail_in = 0;
	w->zstream.total_in = 0;

	w->zstream.next_out = w->zbuffer;
	w->zstream.avail_out = 0;
	w->zstream.total_out = 0;

	w->zstream.zalloc = Z_NULL;
	w->zstream.zfree = Z_NULL;
	w->zstream.opaque = Z_NULL;

//	if(deflateInit(&w->zstream, Z_DEFAULT_COMPRESSION) != Z_OK) {
//		error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
//		return;
//	}

	// Select GZIP compression: windowbits = 15 + 16 = 31
	if(deflateInit2(&w->zstream, Z_DEFAULT_COMPRESSION, Z_DEFLATED, 31, 8, Z_DEFAULT_STRATEGY) != Z_OK) {
		error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
		return;
	}

	w->zsent = 0;
	w->zoutput = 1;
	w->zinitialized = 1;

	debug(D_DEFLATE, "%llu: Initialized compression.", w->id);
}

int web_client_data_request(struct web_client *w, char *url, int datasource_type)
{
	char *args = strchr(url, '?');
	if(args) {
		*args='\0';
		args = &args[1];
	}

	// get the name of the data to show
	char *tok = mystrsep(&url, "/");
	debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

	// do we have such a data set?
	RRD_STATS *st = rrd_stats_find_byname(tok);
	if(!st) st = rrd_stats_find(tok);
	if(!st) {
		// we don't have it
		// try to send a file with that name
		w->data->bytes = 0;
		return(mysendfile(w, tok));
	}

	// we have it
	debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

	// how many entries does the client want?
	size_t lines = save_history;
	size_t group_count = 1;
	time_t after = 0, before = 0;
	int group_method = GROUP_AVERAGE;

	if(url) {
		// parse the lines required
		tok = mystrsep(&url, "/");
		if(tok) lines = atoi(tok);
		if(lines < 1) lines = 1;
	}
	if(url) {
		// parse the group count required
		tok = mystrsep(&url, "/");
		if(tok) group_count = atoi(tok);
		if(group_count < 1) group_count = 1;
		//if(group_count > save_history / 20) group_count = save_history / 20;
	}
	if(url) {
		// parse the grouping method required
		tok = mystrsep(&url, "/");
		if(strcmp(tok, "max") == 0) group_method = GROUP_MAX;
		else if(strcmp(tok, "average") == 0) group_method = GROUP_AVERAGE;
		else debug(D_WEB_CLIENT, "%llu: Unknown group method '%s'", w->id, tok);
	}
	if(url) {
		// parse after time
		tok = mystrsep(&url, "/");
		if(tok) after = strtoul(tok, NULL, 10);
		if(after < 0) after = 0;
	}
	if(url) {
		// parse before time
		tok = mystrsep(&url, "/");
		if(tok) before = strtoul(tok, NULL, 10);
		if(before < 0) before = 0;
	}

	w->data->contenttype = CT_APPLICATION_JSON;
	w->data->bytes = 0;

	char *google_version = "0.6";
	char *google_reqId = "0";
	char *google_sig = "0";
	char *google_out = "json";
	char *google_responseHandler = "google.visualization.Query.setResponse";
	char *google_outFileName = NULL;
	unsigned long last_timestamp_in_data = 0;
	if(datasource_type == DATASOURCE_GOOGLE_JSON || datasource_type == DATASOURCE_GOOGLE_JSONP) {

		w->data->contenttype = CT_APPLICATION_X_JAVASCRIPT;

		while(args) {
			tok = mystrsep(&args, "&");
			if(tok) {
				char *name = mystrsep(&tok, "=");
				if(name && strcmp(name, "tqx") == 0) {
					char *key = mystrsep(&tok, ":");
					char *value = mystrsep(&tok, ";");
					if(key && value && *key && *value) {
						if(strcmp(key, "version") == 0)
							google_version = value;

						else if(strcmp(key, "reqId") == 0)
							google_reqId = value;

						else if(strcmp(key, "sig") == 0)
							google_sig = value;
						
						else if(strcmp(key, "out") == 0)
							google_out = value;
						
						else if(strcmp(key, "responseHandler") == 0)
							google_responseHandler = value;
						
						else if(strcmp(key, "outFileName") == 0)
							google_outFileName = value;
					}
				}
			}
		}

		debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
			w->id, google_version, google_reqId, google_sig, google_out, google_responseHandler, google_outFileName
			);

		if(datasource_type == DATASOURCE_GOOGLE_JSONP) {
			last_timestamp_in_data = strtoul(google_sig, NULL, 0);

			// check the client wants json
			if(strcmp(google_out, "json") != 0) {
				w->data->bytes = snprintf(w->data->buffer, w->data->size, 
					"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'invalid_query',message:'output format is not supported',detailed_message:'the format %s requested is not supported by netdata.'}]});",
					google_responseHandler, google_version, google_reqId, google_out);
					return 200;
			}
		}
	}

	if(datasource_type == DATASOURCE_GOOGLE_JSONP) {
		w->data->bytes = snprintf(w->data->buffer, w->data->size, 
			"%s({version:'%s',reqId:'%s',status:'ok',sig:'%lu',table:",
			google_responseHandler, google_version, google_reqId, st->last_updated);
	}
	
	debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (id %s, %d lines, %d group, %d group_method, %lu after, %lu before).", w->id, st->name, st->id, lines, group_count, group_method, after, before);
	unsigned long timestamp_in_data = rrd_stats_json(datasource_type, st, w->data, lines, group_count, group_method, after, before);

	if(datasource_type == DATASOURCE_GOOGLE_JSONP) {
		if(timestamp_in_data > last_timestamp_in_data)
			w->data->bytes += snprintf(&w->data->buffer[w->data->bytes], w->data->size - w->data->bytes, "});");

		else {
			// the client already has the latest data
			w->data->bytes = snprintf(w->data->buffer, w->data->size, 
				"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
				google_responseHandler, google_version, google_reqId);
		}
	}

	return 200;
}

void web_client_process(struct web_client *w)
{
	int code = 500;
	int bytes;

	w->wait_receive = 0;

	// check if we have an empty line (end of HTTP header)
	if(strstr(w->data->buffer, "\r\n\r\n")) {
		gettimeofday(&w->tv_in, NULL);
		debug(D_WEB_DATA, "%llu: Processing data buffer of %d bytes: '%s'.", w->id, w->data->bytes, w->data->buffer);

		// check if the client requested keep-alive HTTP
		if(strcasestr(w->data->buffer, "Connection: keep-alive")) w->keepalive = 1;
		else w->keepalive = 0;

		// check if the client accepts deflate
		if(strstr(w->data->buffer, "gzip"))
			web_client_enable_deflate(w);

		int datasource_type = DATASOURCE_GOOGLE_JSONP;
		//if(strstr(w->data->buffer, "X-DataSource-Auth"))
		//	datasource_type = DATASOURCE_GOOGLE_JSON;

		char *buf = w->data->buffer;
		char *tok = strsep(&buf, " \r\n");
		char *url = NULL;
		char *pointer_to_free = NULL; // keep url_decode() allocated buffer

		if(buf && strcmp(tok, "GET") == 0) {
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);
			debug(D_WEB_CLIENT, "%llu: Processing HTTP GET on url '%s'.", w->id, url);
		}
		else if (buf && strcmp(tok, "POST") == 0) {
			w->keepalive = 0;
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);

			debug(D_WEB_CLIENT, "%llu: I don't know how to handle POST with form data. Assuming it is a GET on url '%s'.", w->id, url);
		}

		w->last_url[0] = '\0';
		if(url) {
			strncpy(w->last_url, url, URL_MAX);
			w->last_url[URL_MAX] = '\0';

			tok = mystrsep(&url, "/?&");

			debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

			if(strcmp(tok, WEB_PATH_DATA) == 0) { // "data"
				// the client is requesting rrd data
				datasource_type = DATASOURCE_JSON;
				code = web_client_data_request(w, url, datasource_type);
			}
			else if(strcmp(tok, WEB_PATH_DATASOURCE) == 0) { // "datasource"
				// the client is requesting google datasource
				code = web_client_data_request(w, url, datasource_type);
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
					w->data->bytes = 0;
					code = mysendfile(w, tok);
				}
				else {
					code = 200;
					debug(D_WEB_CLIENT_ACCESS, "%llu: Sending %s.json of RRD_STATS...", w->id, st->name);
					w->data->contenttype = CT_APPLICATION_JSON;
					w->data->bytes = 0;
					rrd_stats_graph_json(st, url, w->data);
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
				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending JSON list of all monitors of RRD_STATS...", w->id);

				w->data->contenttype = CT_APPLICATION_JSON;
				w->data->bytes = 0;
				rrd_stats_all_json(w->data);
			}
			else if(strcmp(tok, WEB_PATH_FILE) == 0) { // "file"
				tok = mystrsep(&url, "/?&");
				if(tok && *tok) code = mysendfile(w, tok);
				else {
					code = 400;
					w->data->bytes = 0;
					strcpy(w->data->buffer, "You have to give a filename to get.\r\n");
					w->data->bytes = strlen(w->data->buffer);
				}
			}
			else if(!tok[0]) {
				w->data->bytes = 0;
				code = mysendfile(w, "index.html");
			}
			else {
				w->data->bytes = 0;
				code = mysendfile(w, tok);
			}

		}
		else {
			strcpy(w->last_url, "not a valid response");

			if(buf) debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot understand '%s'.", w->id, buf);

			code = 500;
			w->data->bytes = 0;
			strcpy(w->data->buffer, "I don't understand you...\r\n");
			w->data->bytes = strlen(w->data->buffer);
		}
		
		// free url_decode() buffer
		if(pointer_to_free) free(pointer_to_free);
	}
	else if(w->data->bytes > 8192) {
		strcpy(w->last_url, "too big request");

		debug(D_WEB_CLIENT_ACCESS, "%llu: Received request is too big.", w->id);

		code = 400;
		w->data->bytes = 0;
		strcpy(w->data->buffer, "Received request is too big.\r\n");
		w->data->bytes = strlen(w->data->buffer);
	}
	else {
		// wait for more data
		w->wait_receive = 1;
		return;
	}

	if(w->data->bytes > w->data->size) {
		error("%llu: memory overflow encountered (size is %ld, written %ld).", w->data->size, w->data->bytes);
	}

	gettimeofday(&w->tv_ready, NULL);
	w->data->date = time(NULL);
	w->data->sent = 0;

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
			"Cache-Control: no-cache\r\n"
			, date
			);
	}
	else {
		headerlen += sprintf(&w->response_header[headerlen],
			"Cache-Control: public\r\n"
			);
	}

	// if we know the content length, put it
	if(!w->zoutput && (w->data->bytes || w->data->rbytes))
		headerlen += sprintf(&w->response_header[headerlen],
			"Content-Length: %ld\r\n"
			, w->data->bytes?w->data->bytes:w->data->rbytes
			);
	else if(!w->zoutput)
		w->keepalive = 0;	// content-length is required for keep-alive

	if(w->zoutput) {
		headerlen += sprintf(&w->response_header[headerlen],
			"Content-Encoding: gzip\r\n"
			"Transfer-Encoding: chunked\r\n"
			);
	}

	headerlen += sprintf(&w->response_header[headerlen], "\r\n");

	// disable TCP_NODELAY, to buffer the header
	int flag = 0;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to disable TCP_NODELAY on socket.", w->id);

	// sent the HTTP header
	debug(D_WEB_DATA, "%llu: Sending response HTTP header of size %d: '%s'", w->id, headerlen, w->response_header);

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
				w->wait_receive = 1;

				/*
				// utilize the kernel sendfile() for copying the file to the socket.
				// this block of code can be commented, without anything missing.
				// when it is commented, the program will copy the data using async I/O.
				{
					ssize_t len = sendfile(w->ofd, w->ifd, NULL, w->data->rbytes);
					if(len != w->data->rbytes) error("%llu: sendfile() should copy %ld bytes, but copied %ld. Falling back to manual copy.", w->id, w->data->rbytes, len);
					else web_client_reset(w);
				}
				*/
			}
			else
				debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending an unknown amount of bytes to client.", w->id);
			break;

		default:
			fatal("%llu: Unknown client mode %d.", w->id, w->mode);
			break;
	}
}

void web_client_send_chunk_header(struct web_client *w, int len)
{
	debug(D_DEFLATE, "%llu: OPEN CHUNK of %d bytes (hex: %x).", w->id, len, len);
	char buf[1024]; 
	sprintf(buf, "%X\r\n", len);
	int bytes = send(w->ofd, buf, strlen(buf), MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk header %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk header to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk header to client. Reason: %s", w->id, strerror(errno));
}

void web_client_send_chunk_close(struct web_client *w)
{
	//debug(D_DEFLATE, "%llu: CLOSE CHUNK.", w->id);

	int bytes = send(w->ofd, "\r\n", 2, MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk suffix %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk suffix to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk suffix to client. Reason: %s", w->id, strerror(errno));
}

void web_client_send_chunk_finalize(struct web_client *w)
{
	//debug(D_DEFLATE, "%llu: FINALIZE CHUNK.", w->id);

	int bytes = send(w->ofd, "\r\n0\r\n\r\n", 7, MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk suffix %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk suffix to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk suffix to client. Reason: %s", w->id, strerror(errno));
}

ssize_t web_client_send_deflate(struct web_client *w)
{
	ssize_t bytes;

	// when using compression,
	// w->data->sent is the amount of bytes passed through compression

	// debug(D_DEFLATE, "%llu: TEST w->data->bytes = %d, w->data->sent = %d, w->zhave = %d, w->zsent = %d, w->zstream.avail_in = %d, w->zstream.avail_out = %d, w->zstream.total_in = %d, w->zstream.total_out = %d.", w->id, w->data->bytes, w->data->sent, w->zhave, w->zsent, w->zstream.avail_in, w->zstream.avail_out, w->zstream.total_in, w->zstream.total_out);

	if(w->data->bytes - w->data->sent == 0 && w->zstream.avail_in == 0 && w->zhave == w->zsent && w->zstream.avail_out != 0) {
		// there is nothing to send

		debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

		// finalize the chunk
		if(w->data->sent != 0)
			web_client_send_chunk_finalize(w);

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

		// reset the client
		web_client_reset(w);
		debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
		return(0);
	}

	if(w->zstream.avail_out == 0 && w->zhave == w->zsent) {
		// compress more input data

		// close the previous open chunk
		if(w->data->sent != 0) web_client_send_chunk_close(w);

		debug(D_DEFLATE, "%llu: Compressing %d bytes starting from %d.", w->id, (w->data->bytes - w->data->sent), w->data->sent);

		// give the compressor all the data not passed through the compressor yet
		if(w->data->bytes > w->data->sent) {
			w->zstream.next_in = (Bytef *)&w->data->buffer[w->data->sent];
			w->zstream.avail_in = (w->data->bytes - w->data->sent);
		}

		// reset the compressor output buffer
		w->zstream.next_out = w->zbuffer;
		w->zstream.avail_out = ZLIB_CHUNK;

		// ask for FINISH if we have all the input
		int flush = Z_SYNC_FLUSH;
		if(w->mode == WEB_CLIENT_MODE_NORMAL
			|| (w->mode == WEB_CLIENT_MODE_FILECOPY && w->data->bytes == w->data->rbytes)) {
			flush = Z_FINISH;
			debug(D_DEFLATE, "%llu: Requesting Z_FINISH.", w->id);
		}
		else {
			debug(D_DEFLATE, "%llu: Requesting Z_SYNC_FLUSH.", w->id);
		}

		// compress
		if(deflate(&w->zstream, flush) == Z_STREAM_ERROR) {
			error("%llu: Compression failed. Closing down client.", w->id);
			web_client_reset(w);
			return(-1);
		}

		w->zhave = ZLIB_CHUNK - w->zstream.avail_out;
		w->zsent = 0;

		// keep track of the bytes passed through the compressor
		w->data->sent = w->data->bytes;

		debug(D_DEFLATE, "%llu: Compression produced %d bytes.", w->id, w->zhave);

		// open a new chunk
		web_client_send_chunk_header(w, w->zhave);
	}

	bytes = send(w->ofd, &w->zbuffer[w->zsent], w->zhave - w->zsent, MSG_DONTWAIT);
	if(bytes > 0) {
		w->zsent += bytes;
		debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, bytes);
	}
	else if(bytes == 0) debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
	else debug(D_WEB_CLIENT, "%llu: Failed to send data to client. Reason: %s", w->id, strerror(errno));

	return(bytes);
}

ssize_t web_client_send(struct web_client *w)
{
	if(w->zoutput) return web_client_send_deflate(w);

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
	else if(bytes == 0) debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
	else debug(D_WEB_CLIENT, "%llu: Failed to send data to client. Reason: %s", w->id, strerror(errno));


	return(bytes);
}

ssize_t web_client_receive(struct web_client *w)
{
	// do we have any space for more data?
	web_buffer_increase(w->data, WEB_DATA_LENGTH_INCREASE_STEP);

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
			FD_SET(w->ifd, &ifds);
			if(w->ifd > fdmax) fdmax = w->ifd;
		}
		if (w->wait_send) {
			FD_SET(w->ofd, &ofds);
			if(w->ofd > fdmax) fdmax = w->ofd;
		}

		tv.tv_sec = 30;
		tv.tv_usec = 0;

		debug(D_WEB_CLIENT, "%llu: Waiting socket async I/O for %s %s", w->id, w->wait_receive?"INPUT":"", w->wait_send?"OUTPUT":"");
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("%llu: LISTENER: select() failed.", w->id);
			continue;
		}
		else if(!retval) {
			// timeout
			web_client_reset(w);
			w->obsolete = 1;
			return NULL;
		}

		if(FD_ISSET(w->ifd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on input socket (%s).", w->id, strerror(errno));
			web_client_reset(w);
			w->obsolete = 1;
			return NULL;
		}

		if(FD_ISSET(w->ofd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on output socket (%s).", w->id, strerror(errno));
			web_client_reset(w);
			w->obsolete = 1;
			return NULL;
		}

		if(w->wait_send && FD_ISSET(w->ofd, &ofds)) {
			if(web_client_send(w) < 0) {
				debug(D_WEB_CLIENT, "%llu: Closing client (input: %s).", w->id, strerror(errno));
				web_client_reset(w);
				w->obsolete = 1;
				errno = 0;
				return NULL;
			}
		}

		if(w->wait_receive && FD_ISSET(w->ifd, &ifds)) {
			if(web_client_receive(w) < 0) {
				debug(D_WEB_CLIENT, "%llu: Closing client (output: %s).", w->id, strerror(errno));
				web_client_reset(w);
				w->obsolete = 1;
				errno = 0;
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

void syslog_allocations(void)
{
	static int mem = 0;

	struct mallinfo mi;

	mi = mallinfo();
	if(mi.uordblks > mem) {
		int clients = 0;
		struct web_client *w;
		for(w = web_clients; w ; w = w->next) clients++;

		syslog(LOG_NOTICE, "Allocated memory increased from %d to %d (increased by %d bytes). There are %d web clients connected.", mem, mi.uordblks, mi.uordblks - mem, clients);
		mem = mi.uordblks;
	}
	/*
	syslog(LOG_NOTICE, "Total non-mmapped bytes (arena):       %d\n", mi.arena);
	syslog(LOG_NOTICE, "# of free chunks (ordblks):            %d\n", mi.ordblks);
	syslog(LOG_NOTICE, "# of free fastbin blocks (smblks):     %d\n", mi.smblks);
	syslog(LOG_NOTICE, "# of mapped regions (hblks):           %d\n", mi.hblks);
	syslog(LOG_NOTICE, "Bytes in mapped regions (hblkhd):      %d\n", mi.hblkhd);
	syslog(LOG_NOTICE, "Max. total allocated space (usmblks):  %d\n", mi.usmblks);
	syslog(LOG_NOTICE, "Free bytes held in fastbins (fsmblks): %d\n", mi.fsmblks);
	syslog(LOG_NOTICE, "Total allocated space (uordblks):      %d\n", mi.uordblks);
	syslog(LOG_NOTICE, "Total free space (fordblks):           %d\n", mi.fordblks);
	syslog(LOG_NOTICE, "Topmost releasable block (keepcost):   %d\n", mi.keepcost);
	*/
}

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
		tv.tv_sec = 0;
		tv.tv_usec = 200000;

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
				
				syslog(LOG_NOTICE, "%llu: %s connected", w->id, w->client_ip);
			}
			else debug(D_WEB_CLIENT, "LISTENER: select() didn't do anything.");

		}
		//else debug(D_WEB_CLIENT, "LISTENER: select() timeout.");

		// cleanup unused clients
		for(w = web_clients; w ; w = w?w->next:NULL) {
			if(w->obsolete) {
				syslog(LOG_NOTICE, "%llu: %s disconnected", w->id, w->client_ip);
				debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
				// pthread_join(w->thread,  NULL);
				w = web_client_free(w);
				syslog_allocations();
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

#define MAX_PROC_NET_DEV_LINE 4096
#define MAX_PROC_NET_DEV_IFACE_NAME 1024

int do_proc_net_dev() {
	char buffer[MAX_PROC_NET_DEV_LINE+1] = "";
	char iface[MAX_PROC_NET_DEV_IFACE_NAME + 1] = "";
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
		
		r = sscanf(buffer, "%s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
			iface,
			&rbytes, &rpackets, &rerrors, &rdrops, &rfifo, &rframe, &rcompressed, &rmulticast,
			&tbytes, &tpackets, &terrors, &tdrops, &tfifo, &tcollisions, &tcarrier, &tcompressed);
		if(r == EOF) break;
		if(r != 17) {
			error("Cannot read /proc/net/dev line. Expected 17 params, read %d.", r);
			continue;
		}

		// --------------------------------------------------------------------

		RRD_STATS *st = rrd_stats_find_bytype(RRD_TYPE_NET, iface);
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET, iface, NULL, iface, "Bandwidth", "kilobits/s", save_history);

			rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "received", &rbytes, NULL);
		rrd_stats_dimension_set(st, "sent", &tbytes, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("net_packets", iface);
		if(!st) {
			st = rrd_stats_create("net_packets", iface, NULL, iface, "Packets", "packets/s", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "received", &rpackets, NULL);
		rrd_stats_dimension_set(st, "sent", &tpackets, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("net_errors", iface);
		if(!st) {
			st = rrd_stats_create("net_errors", iface, NULL, iface, "Interface Errors", "errors/s", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "receive", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "transmit", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "receive", &rerrors, NULL);
		rrd_stats_dimension_set(st, "transmit", &terrors, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("net_fifo", iface);
		if(!st) {
			st = rrd_stats_create("net_fifo", iface, NULL, iface, "Interface Queue", "packets", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "receive", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
			rrd_stats_dimension_add(st, "transmit", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "receive", &rfifo, NULL);
		rrd_stats_dimension_set(st, "transmit", &tfifo, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("net_compressed", iface);
		if(!st) {
			st = rrd_stats_create("net_compressed", iface, NULL, iface, "Compressed Packets", "packets/s", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "received", &rcompressed, NULL);
		rrd_stats_dimension_set(st, "sent", &tcompressed, NULL);
		rrd_stats_done(st);
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/diskstats processor

#define MAX_PROC_DISKSTATS_LINE 4096
#define MAX_PROC_DISKSTATS_DISK_NAME 1024

int do_proc_diskstats() {
	char buffer[MAX_PROC_DISKSTATS_LINE+1] = "";
	char disk[MAX_PROC_DISKSTATS_DISK_NAME + 1] = "";
	
	int r;
	char *p;
	
	FILE *fp = fopen("/proc/diskstats", "r");
	if(!fp) {
		error("Cannot read /proc/diskstats.");
		return 1;
	}
	
	for(;1;) {
		unsigned long long 	major = 0, minor = 0,
							reads = 0,  reads_merged = 0,  readsectors = 0,  readms = 0,
							writes = 0, writes_merged = 0, writesectors = 0, writems = 0,
							currentios = 0, iosms = 0, wiosms = 0;

		p = fgets(buffer, MAX_PROC_DISKSTATS_LINE, fp);
		if(!p) break;
		
		r = sscanf(buffer, "%llu %llu %s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
			&major, &minor, disk,
			&reads, &reads_merged, &readsectors, &readms, &writes, &writes_merged, &writesectors, &writems, &currentios, &iosms, &wiosms
		);
		if(r == EOF) break;
		if(r != 14) {
			error("Cannot read /proc/diskstats line. Expected 14 params, read %d.", r);
			continue;
		}

		switch(major) {
			case 9: // MDs
			case 43: // network block
			case 144: // nfs
			case 145: // nfs
			case 146: // nfs
			case 199: // veritas
			case 201: // veritas
			case 251: // dm
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
			case 179: // MMC
			case 180: // USB
				if(minor % 8) continue; // partitions
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
			case 202: // xen
			case 256: // flash
			case 257: // flash
				if(minor % 16) continue; // partitions
				break;

			case 160: // raid
			case 161: // raid
				if(minor % 32) continue; // partitions
				break;

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

			default:
				continue;
		}

		// --------------------------------------------------------------------

		RRD_STATS *st = rrd_stats_find_bytype(RRD_TYPE_DISK, disk);
		if(!st) {
			char ssfilename[FILENAME_MAX + 1];
			int sector_size = 512;

			sprintf(ssfilename, "/sys/block/%s/queue/hw_sector_size", disk);
			FILE *fpss = fopen(ssfilename, "r");
			if(fpss) {
				char ssbuffer[1025];
				char *tmp = fgets(ssbuffer, 1024, fpss);

				if(tmp) {
					sector_size = atoi(tmp);
					if(sector_size <= 0) {
						error("Invalid sector size %d for device %s in %s. Assuming 512.", sector_size, disk, ssfilename);
						sector_size = 512;
					}
				}
				else error("Cannot read data for sector size for device %s from %s. Assuming 512.", disk, ssfilename);

				fclose(fpss);
			}
			else error("Cannot read sector size for device %s from %s. Assuming 512.", disk, ssfilename);

			st = rrd_stats_create(RRD_TYPE_DISK, disk, NULL, disk, "Disk I/O", "kilobytes/s", save_history);

			rrd_stats_dimension_add(st, "reads", NULL, sizeof(unsigned long long), 0, sector_size, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "writes", NULL, sizeof(unsigned long long), 0, sector_size * -1, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "reads", &readsectors, NULL);
		rrd_stats_dimension_set(st, "writes", &writesectors, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("disk_ops", disk);
		if(!st) {
			st = rrd_stats_create("disk_ops", disk, NULL, disk, "Disk Operations", "operations/s", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "reads", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "writes", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "reads", &reads, NULL);
		rrd_stats_dimension_set(st, "writes", &writes, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("disk_merged_ops", disk);
		if(!st) {
			st = rrd_stats_create("disk_merged_ops", disk, NULL, disk, "Merged Disk Operations", "operations/s", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "reads", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "writes", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "reads", &reads_merged, NULL);
		rrd_stats_dimension_set(st, "writes", &writes_merged, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("disk_iotime", disk);
		if(!st) {
			st = rrd_stats_create("disk_iotime", disk, NULL, disk, "Disk I/O Time", "milliseconds/s", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "reads", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "writes", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "latency", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			rrd_stats_dimension_add(st, "weighted", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "reads", &readms, NULL);
		rrd_stats_dimension_set(st, "writes", &writems, NULL);
		rrd_stats_dimension_set(st, "latency", &iosms, NULL);
		rrd_stats_dimension_set(st, "weighted", &wiosms, NULL);
		rrd_stats_done(st);

		// --------------------------------------------------------------------

		st = rrd_stats_find_bytype("disk_cur_ops", disk);
		if(!st) {
			st = rrd_stats_create("disk_cur_ops", disk, NULL, disk, "Current Disk I/O operations", "operations", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "operations", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "operations", &currentios, NULL);
		rrd_stats_done(st);
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

			// --------------------------------------------------------------------

			RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_SNMP ".packets");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "packets", NULL, RRD_TYPE_NET_SNMP, "IPv4 Packets", "packets/s", save_history);

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "forwarded", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutRequests, NULL);
			rrd_stats_dimension_set(st, "received", &InReceives, NULL);
			rrd_stats_dimension_set(st, "forwarded", &ForwDatagrams, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".fragsout");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "fragsout", NULL, RRD_TYPE_NET_SNMP, "IPv4 Fragments Sent", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "ok", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "failed", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "all", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "ok", &FragOKs, NULL);
			rrd_stats_dimension_set(st, "failed", &FragFails, NULL);
			rrd_stats_dimension_set(st, "all", &FragCreates, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".fragsin");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "fragsin", NULL, RRD_TYPE_NET_SNMP, "IPv4 Fragments Reassembly", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "ok", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "failed", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "all", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "ok", &ReasmOKs, NULL);
			rrd_stats_dimension_set(st, "failed", &ReasmFails, NULL);
			rrd_stats_dimension_set(st, "all", &ReasmReqds, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".errors");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "errors", NULL, RRD_TYPE_NET_SNMP, "IPv4 Errors", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "InDiscards", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "OutDiscards", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);

				rrd_stats_dimension_add(st, "InHdrErrors", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "InAddrErrors", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "InUnknownProtos", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);

				rrd_stats_dimension_add(st, "OutNoRoutes", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "InDiscards", &InDiscards, NULL);
			rrd_stats_dimension_set(st, "OutDiscards", &OutDiscards, NULL);
			rrd_stats_dimension_set(st, "InHdrErrors", &InHdrErrors, NULL);
			rrd_stats_dimension_set(st, "InAddrErrors", &InAddrErrors, NULL);
			rrd_stats_dimension_set(st, "InUnknownProtos", &InUnknownProtos, NULL);
			rrd_stats_dimension_set(st, "OutNoRoutes", &OutNoRoutes, NULL);
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

			unsigned long long RtoAlgorithm, RtoMin, RtoMax, MaxConn, ActiveOpens, PassiveOpens, AttemptFails, EstabResets,
				CurrEstab, InSegs, OutSegs, RetransSegs, InErrs, OutRsts;

			int r = sscanf(&buffer[5], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&RtoAlgorithm, &RtoMin, &RtoMax, &MaxConn, &ActiveOpens, &PassiveOpens, &AttemptFails, &EstabResets, &CurrEstab, &InSegs, &OutSegs, &RetransSegs, &InErrs, &OutRsts);

			if(r == EOF) break;
			if(r != 14) error("Cannot read /proc/net/snmp TCP line. Expected 14 params, read %d.", r);

			// --------------------------------------------------------------------
			
			// see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
			RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcpsock");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcpsock", NULL, "tcp", "IPv4 TCP Connections", "active connections", save_history);

				rrd_stats_dimension_add(st, "connections", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "connections", &CurrEstab, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------
			
			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcppackets");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcppackets", NULL, "tcp", "IPv4 TCP Packets", "packets/s", save_history);

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", &InSegs, NULL);
			rrd_stats_dimension_set(st, "sent", &OutSegs, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------
			
			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcperrors");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcperrors", NULL, "tcp", "IPv4 TCP Errors", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "InErrs", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "RetransSegs", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "InErrs", &InErrs, NULL);
			rrd_stats_dimension_set(st, "RetransSegs", &RetransSegs, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------
			
			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcphandshake");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcphandshake", NULL, "tcp", "IPv4 TCP Handshake Issues", "events/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "EstabResets", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "OutRsts", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "ActiveOpens", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "PassiveOpens", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "AttemptFails", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "EstabResets", &EstabResets, NULL);
			rrd_stats_dimension_set(st, "OutRsts", &OutRsts, NULL);
			rrd_stats_dimension_set(st, "ActiveOpens", &ActiveOpens, NULL);
			rrd_stats_dimension_set(st, "PassiveOpens", &PassiveOpens, NULL);
			rrd_stats_dimension_set(st, "AttemptFails", &AttemptFails, NULL);
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

			// --------------------------------------------------------------------
			
			// see http://net-snmp.sourceforge.net/docs/mibs/udp.html
			RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_SNMP ".udppackets");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "udppackets", NULL, "udp", "IPv4 UDP Packets", "packets/s", save_history);

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", &InDatagrams, NULL);
			rrd_stats_dimension_set(st, "sent", &OutDatagrams, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------
			
			st = rrd_stats_find(RRD_TYPE_NET_SNMP ".udperrors");
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET_SNMP, "udperrors", NULL, "udp", "IPv4 UDP Errors", "events/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "RcvbufErrors", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "SndbufErrors", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "InErrors", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "NoPorts", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "InErrors", &InErrors, NULL);
			rrd_stats_dimension_set(st, "NoPorts", &NoPorts, NULL);
			rrd_stats_dimension_set(st, "RcvbufErrors", &RcvbufErrors, NULL);
			rrd_stats_dimension_set(st, "SndbufErrors", &SndbufErrors, NULL);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/netstat processor

int do_proc_net_netstat() {
	char buffer[MAX_PROC_NET_SNMP_LINE+1] = "";

	FILE *fp = fopen("/proc/net/netstat", "r");
	if(!fp) {
		error("Cannot read /proc/net/netstat.");
		return 1;
	}

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
		if(!p) break;

		if(strncmp(p, "IpExt: ", 7) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "IpExt: ", 7) != 0) {
				error("Cannot read IpExt line from /proc/net/netstat.");
				break;
			}

			unsigned long long
				InNoRoutes = 0, InTruncatedPkts = 0,
				InOctets = 0,  InMcastPkts = 0,  InBcastPkts = 0,  InMcastOctets = 0,  InBcastOctets = 0,
				OutOctets = 0, OutMcastPkts = 0, OutBcastPkts = 0, OutMcastOctets = 0, OutBcastOctets = 0;
	
			int r = sscanf(&buffer[7], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&InNoRoutes, &InTruncatedPkts, &InMcastPkts, &OutMcastPkts, &InBcastPkts, &OutBcastPkts,
				&InOctets, &OutOctets, &InMcastOctets, &OutMcastOctets, &InBcastOctets, &OutBcastOctets);

			if(r == EOF) break;
			if(r != 12) {
				error("Cannot read /proc/net/netstat IpExt line. Expected 12 params, read %d.", r);
				continue;
			}

			// --------------------------------------------------------------------

			RRD_STATS *st = rrd_stats_find("system.ipv4");
			if(!st) {
				st = rrd_stats_create("system", "ipv4", NULL, "ipv4", "IPv4 Bandwidth", "kilobits/s", save_history);

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutOctets, NULL);
			rrd_stats_dimension_set(st, "received", &InOctets, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find("ipv4.inerrors");
			if(!st) {
				st = rrd_stats_create("ipv4", "inerrors", NULL, "ipv4", "IPv4 Input Errors", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "noroutes", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "trunkated", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "noroutes", &InNoRoutes, NULL);
			rrd_stats_dimension_set(st, "trunkated", &InTruncatedPkts, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find("ipv4.mcast");
			if(!st) {
				st = rrd_stats_create("ipv4", "mcast", NULL, "ipv4", "IPv4 Multicast Bandwidth", "kilobits/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutMcastOctets, NULL);
			rrd_stats_dimension_set(st, "received", &InMcastOctets, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find("ipv4.bcast");
			if(!st) {
				st = rrd_stats_create("ipv4", "bcast", NULL, "ipv4", "IPv4 Broadcast Bandwidth", "kilobits/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutBcastOctets, NULL);
			rrd_stats_dimension_set(st, "received", &InBcastOctets, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find("ipv4.mcastpkts");
			if(!st) {
				st = rrd_stats_create("ipv4", "mcastpkts", NULL, "ipv4", "IPv4 Multicast Packets", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutMcastPkts, NULL);
			rrd_stats_dimension_set(st, "received", &InMcastPkts, NULL);
			rrd_stats_done(st);

			// --------------------------------------------------------------------

			st = rrd_stats_find("ipv4.bcastpkts");
			if(!st) {
				st = rrd_stats_create("ipv4", "bcastpkts", NULL, "ipv4", "IPv4 Broadcast Packets", "packets/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "sent", &OutBcastPkts, NULL);
			rrd_stats_dimension_set(st, "received", &InBcastPkts, NULL);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/stat/nf_conntrack processor

int do_proc_net_stat_conntrack() {
	char buffer[MAX_PROC_NET_STAT_CONNTRACK_LINE+1] = "";

	FILE *fp = fopen("/proc/net/stat/nf_conntrack", "r");
	if(!fp) {
		error("Cannot read /proc/net/stat/nf_conntrack.");
		return 1;
	}

	// read and discard the header
	char *p = fgets(buffer, MAX_PROC_NET_STAT_CONNTRACK_LINE, fp);

	unsigned long long aentries = 0, asearched = 0, afound = 0, anew = 0, ainvalid = 0, aignore = 0, adelete = 0, adelete_list = 0,
		ainsert = 0, ainsert_failed = 0, adrop = 0, aearly_drop = 0, aicmp_error = 0, aexpect_new = 0, aexpect_create = 0, aexpect_delete = 0, asearch_restart = 0;

	for(;1;) {
		p = fgets(buffer, MAX_PROC_NET_STAT_CONNTRACK_LINE, fp);
		if(!p) break;

		unsigned long long tentries = 0, tsearched = 0, tfound = 0, tnew = 0, tinvalid = 0, tignore = 0, tdelete = 0, tdelete_list = 0, tinsert = 0, tinsert_failed = 0, tdrop = 0, tearly_drop = 0, ticmp_error = 0, texpect_new = 0, texpect_create = 0, texpect_delete = 0, tsearch_restart = 0;

		int r = sscanf(buffer, "%llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx\n",
			&tentries, &tsearched, &tfound, &tnew, &tinvalid, &tignore, &tdelete, &tdelete_list, &tinsert, &tinsert_failed, &tdrop, &tearly_drop, &ticmp_error, &texpect_new, &texpect_create, &texpect_delete, &tsearch_restart);

		if(r == EOF) break;
		if(r < 16) error("Cannot read /proc/net/stat/nf_conntrack. Expected 17 params, read %d.", r);

		if(!aentries) aentries =  tentries;

		// sum all the cpus together
		asearched 			+= tsearched;			// conntrack.search
		afound 				+= tfound;				// conntrack.search
		anew 				+= tnew;				// conntrack.new
		ainvalid 			+= tinvalid;			// conntrack.new
		aignore 			+= tignore;				// conntrack.new
		adelete 			+= tdelete;				// conntrack.changes
		adelete_list 		+= tdelete_list;		// conntrack.changes
		ainsert 			+= tinsert;				// conntrack.changes
		ainsert_failed 		+= tinsert_failed;		// conntrack.errors
		adrop 				+= tdrop;				// conntrack.errors
		aearly_drop 		+= tearly_drop;			// conntrack.errors
		aicmp_error 		+= ticmp_error;			// conntrack.errors
		aexpect_new 		+= texpect_new;			// conntrack.expect
		aexpect_create 		+= texpect_create;		// conntrack.expect
		aexpect_delete 		+= texpect_delete;		// conntrack.expect
		asearch_restart 	+= tsearch_restart;		// conntrack.search
	}
	fclose(fp);

	// --------------------------------------------------------------------
	
	RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".sockets");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "sockets", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connections", "active connections", save_history);

		rrd_stats_dimension_add(st, "connections", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "connections", &aentries, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------

	st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".new");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "new", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter New Connections", "connections/s", save_history);

		rrd_stats_dimension_add(st, "new", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "ignore", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "invalid", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "new", &anew, NULL);
	rrd_stats_dimension_set(st, "ignore", &aignore, NULL);
	rrd_stats_dimension_set(st, "invalid", &ainvalid, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------

	st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".changes");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "changes", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connection Changes", "changes/s", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "inserted", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "deleted", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "delete_list", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "inserted", &ainsert, NULL);
	rrd_stats_dimension_set(st, "deleted", &adelete, NULL);
	rrd_stats_dimension_set(st, "delete_list", &adelete_list, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------

	st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".expect");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "expect", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connection Expectations", "expectations/s", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "created", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "deleted", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "new", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "created", &aexpect_create, NULL);
	rrd_stats_dimension_set(st, "deleted", &aexpect_delete, NULL);
	rrd_stats_dimension_set(st, "new", &aexpect_new, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------

	st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".search");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "search", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connection Searches", "searches/s", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "searched", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "restarted", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "found", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "searched", &asearched, NULL);
	rrd_stats_dimension_set(st, "restarted", &asearch_restart, NULL);
	rrd_stats_dimension_set(st, "found", &afound, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------

	st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".errors");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "errors", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Errors", "events/s", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "icmp_error", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "insert_failed", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "drop", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "early_drop", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "icmp_error", &aicmp_error, NULL);
	rrd_stats_dimension_set(st, "insert_failed", &ainsert_failed, NULL);
	rrd_stats_dimension_set(st, "drop", &adrop, NULL);
	rrd_stats_dimension_set(st, "early_drop", &aearly_drop, NULL);
	rrd_stats_done(st);

	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/ip_vs_stats processor

int do_proc_net_ip_vs_stats() {
	char buffer[MAX_PROC_NET_IPVS_LINE+1] = "";

	FILE *fp = fopen("/proc/net/ip_vs_stats", "r");
	if(!fp) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	// read the discard the 2 header lines
	char *p = fgets(buffer, MAX_PROC_NET_IPVS_LINE, fp);
	if(!p) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	p = fgets(buffer, MAX_PROC_NET_IPVS_LINE, fp);
	if(!p) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	p = fgets(buffer, MAX_PROC_NET_IPVS_LINE, fp);
	if(!p) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	unsigned long long entries, InPackets, OutPackets, InBytes, OutBytes;

	int r = sscanf(buffer, "%llx %llx %llx %llx %llx\n", &entries, &InPackets, &OutPackets, &InBytes, &OutBytes);

	if(r == EOF) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}
	if(r != 5) error("Cannot read /proc/net/ip_vs_stats. Expected 5 params, read %d.", r);

	fclose(fp);

	// --------------------------------------------------------------------

	RRD_STATS *st = rrd_stats_find(RRD_TYPE_NET_IPVS ".sockets");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_IPVS, "sockets", NULL, RRD_TYPE_NET_IPVS, "IPVS New Connections", "connections/s", save_history);

		rrd_stats_dimension_add(st, "connections", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "connections", &entries, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find(RRD_TYPE_NET_IPVS ".packets");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_IPVS, "packets", NULL, RRD_TYPE_NET_IPVS, "IPVS Packets", "packets/s", save_history);

		rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "received", &InPackets, NULL);
	rrd_stats_dimension_set(st, "sent", &OutPackets, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find(RRD_TYPE_NET_IPVS ".net");
	if(!st) {
		st = rrd_stats_create(RRD_TYPE_NET_IPVS, "net", NULL, RRD_TYPE_NET_IPVS, "IPVS Bandwidth", "kilobits/s", save_history);

		rrd_stats_dimension_add(st, "received", NULL, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "sent", NULL, sizeof(unsigned long long), 0, -8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "received", &InBytes, NULL);
	rrd_stats_dimension_set(st, "sent", &OutBytes, NULL);
	rrd_stats_done(st);

	return 0;
}

int do_proc_stat() {
	char buffer[MAX_PROC_STAT_LINE+1] = "";

	FILE *fp = fopen("/proc/stat", "r");
	if(!fp) {
		error("Cannot read /proc/stat.");
		return 1;
	}

	unsigned long long processes = 0, running = 0 , blocked = 0;

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_STAT_LINE, fp);
		if(!p) break;

		if(strncmp(p, "cpu", 3) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";
			unsigned long long user = 0, nice = 0, system = 0, idle = 0, iowait = 0, irq = 0, softirq = 0, steal = 0, guest = 0, guest_nice = 0;

			int r = sscanf(buffer, "%s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				id, &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal, &guest, &guest_nice);

			if(r == EOF) break;
			if(r < 10) error("Cannot read /proc/stat cpu line. Expected 11 params, read %d.", r);

			char *title = "Core utilization";
			char *type = RRD_TYPE_STAT;
			if(strcmp(id, "cpu") == 0) {
				title = "Total CPU utilization";
				type = "system";
			}

			RRD_STATS *st = rrd_stats_find_bytype(type, id);
			if(!st) {
				st = rrd_stats_create(type, id, NULL, "cpu", title, "percentage", save_history);

				long multiplier = 1;
				long divisor = 1; // sysconf(_SC_CLK_TCK);

				rrd_stats_dimension_add(st, "guest_nice", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "guest", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "steal", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "softirq", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "irq", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "user", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "system", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "nice", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_add(st, "iowait", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);

				rrd_stats_dimension_add(st, "idle", NULL, sizeof(unsigned long long), 0, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL, NULL);
				rrd_stats_dimension_hide(st, "idle");
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "user", &user, NULL);
			rrd_stats_dimension_set(st, "nice", &nice, NULL);
			rrd_stats_dimension_set(st, "system", &system, NULL);
			rrd_stats_dimension_set(st, "idle", &idle, NULL);
			rrd_stats_dimension_set(st, "iowait", &iowait, NULL);
			rrd_stats_dimension_set(st, "irq", &irq, NULL);
			rrd_stats_dimension_set(st, "softirq", &softirq, NULL);
			rrd_stats_dimension_set(st, "steal", &steal, NULL);
			rrd_stats_dimension_set(st, "guest", &guest, NULL);
			rrd_stats_dimension_set(st, "guest_nice", &guest_nice, NULL);
			rrd_stats_done(st);
		}
		else if(strncmp(p, "intr ", 5) == 0) {
			char id[MAX_PROC_STAT_NAME + 1];

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat intr line. Expected 2 params, read %d.", r);

			// --------------------------------------------------------------------
	
			RRD_STATS *st = rrd_stats_find_bytype("system", id);
			if(!st) {
				st = rrd_stats_create("system", id, NULL, "cpu", "CPU Interrupts", "interrupts/s", save_history);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "interrupts", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "interrupts", &value, NULL);
			rrd_stats_done(st);
		}
		else if(strncmp(p, "ctxt ", 5) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat ctxt line. Expected 2 params, read %d.", r);

			// --------------------------------------------------------------------
	
			RRD_STATS *st = rrd_stats_find_bytype("system", id);
			if(!st) {
				st = rrd_stats_create("system", id, NULL, "cpu", "CPU Context Switches", "context switches/s", save_history);

				rrd_stats_dimension_add(st, "switches", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "switches", &value, NULL);
			rrd_stats_done(st);
		}
		else if(strncmp(p, "processes ", 10) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat processes line. Expected 2 params, read %d.", r);

			processes = value;
		}
		else if(strncmp(p, "procs_running ", 14) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat procs_running line. Expected 2 params, read %d.", r);

			running = value;
		}
		else if(strncmp(p, "procs_blocked ", 14) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "procs_running";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat procs_blocked line. Expected 2 params, read %d.", r);

			blocked = value;
		}
	}
	fclose(fp);

	// --------------------------------------------------------------------

	RRD_STATS *st = rrd_stats_find_bytype("system", "forks");
	if(!st) {
		st = rrd_stats_create("system", "forks", NULL, "cpu", "New Processes", "processes/s", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "started", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "started", &processes, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------

	st = rrd_stats_find_bytype("system", "processes");
	if(!st) {
		st = rrd_stats_create("system", "processes", NULL, "cpu", "Processes", "processes", save_history);

		rrd_stats_dimension_add(st, "running", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "blocked", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "running", &running, NULL);
	rrd_stats_dimension_set(st, "blocked", &blocked, NULL);
	rrd_stats_done(st);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/meminfo processor

#define MAX_PROC_MEMINFO_LINE 4096
#define MAX_PROC_MEMINFO_NAME 1024

int do_proc_meminfo() {
	char buffer[MAX_PROC_MEMINFO_LINE+1] = "";

	FILE *fp = fopen("/proc/meminfo", "r");
	if(!fp) {
		error("Cannot read /proc/meminfo.");
		return 1;
	}

	int hwcorrupted = 0;

	unsigned long long MemTotal = 0, MemFree = 0, Buffers = 0, Cached = 0, SwapCached = 0,
		Active = 0, Inactive = 0, ActiveAnon = 0, InactiveAnon = 0, ActiveFile = 0, InactiveFile = 0,
		Unevictable = 0, Mlocked = 0, SwapTotal = 0, SwapFree = 0, Dirty = 0, Writeback = 0, AnonPages = 0,
		Mapped = 0, Shmem = 0, Slab = 0, SReclaimable = 0, SUnreclaim = 0, KernelStack = 0, PageTables = 0,
		NFS_Unstable = 0, Bounce = 0, WritebackTmp = 0, CommitLimit = 0, Committed_AS = 0,
		VmallocTotal = 0, VmallocUsed = 0, VmallocChunk = 0,
		AnonHugePages = 0, HugePages_Total = 0, HugePages_Free = 0, HugePages_Rsvd = 0, HugePages_Surp = 0, Hugepagesize = 0,
		DirectMap4k = 0, DirectMap2M = 0, HardwareCorrupted = 0;

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_MEMINFO_LINE, fp);
		if(!p) break;

		// remove ':'
		while((p = strchr(buffer, ':'))) *p = ' ';

		char name[MAX_PROC_MEMINFO_NAME + 1] = "";
		unsigned long long value = 0;

		int r = sscanf(buffer, "%s %llu kB\n", name, &value);

		if(r == EOF) break;
		if(r != 2) error("Cannot read /proc/meminfo line. Expected 2 params, read %d.", r);

		     if(!MemTotal && strcmp(name, "MemTotal") == 0) MemTotal = value;
		else if(!MemFree && strcmp(name, "MemFree") == 0) MemFree = value;
		else if(!Buffers && strcmp(name, "Buffers") == 0) Buffers = value;
		else if(!Cached && strcmp(name, "Cached") == 0) Cached = value;
		else if(!SwapCached && strcmp(name, "SwapCached") == 0) SwapCached = value;
		else if(!Active && strcmp(name, "Active") == 0) Active = value;
		else if(!Inactive && strcmp(name, "Inactive") == 0) Inactive = value;
		else if(!ActiveAnon && strcmp(name, "ActiveAnon") == 0) ActiveAnon = value;
		else if(!InactiveAnon && strcmp(name, "InactiveAnon") == 0) InactiveAnon = value;
		else if(!ActiveFile && strcmp(name, "ActiveFile") == 0) ActiveFile = value;
		else if(!InactiveFile && strcmp(name, "InactiveFile") == 0) InactiveFile = value;
		else if(!Unevictable && strcmp(name, "Unevictable") == 0) Unevictable = value;
		else if(!Mlocked && strcmp(name, "Mlocked") == 0) Mlocked = value;
		else if(!SwapTotal && strcmp(name, "SwapTotal") == 0) SwapTotal = value;
		else if(!SwapFree && strcmp(name, "SwapFree") == 0) SwapFree = value;
		else if(!Dirty && strcmp(name, "Dirty") == 0) Dirty = value;
		else if(!Writeback && strcmp(name, "Writeback") == 0) Writeback = value;
		else if(!AnonPages && strcmp(name, "AnonPages") == 0) AnonPages = value;
		else if(!Mapped && strcmp(name, "Mapped") == 0) Mapped = value;
		else if(!Shmem && strcmp(name, "Shmem") == 0) Shmem = value;
		else if(!Slab && strcmp(name, "Slab") == 0) Slab = value;
		else if(!SReclaimable && strcmp(name, "SReclaimable") == 0) SReclaimable = value;
		else if(!SUnreclaim && strcmp(name, "SUnreclaim") == 0) SUnreclaim = value;
		else if(!KernelStack && strcmp(name, "KernelStack") == 0) KernelStack = value;
		else if(!PageTables && strcmp(name, "PageTables") == 0) PageTables = value;
		else if(!NFS_Unstable && strcmp(name, "NFS_Unstable") == 0) NFS_Unstable = value;
		else if(!Bounce && strcmp(name, "Bounce") == 0) Bounce = value;
		else if(!WritebackTmp && strcmp(name, "WritebackTmp") == 0) WritebackTmp = value;
		else if(!CommitLimit && strcmp(name, "CommitLimit") == 0) CommitLimit = value;
		else if(!Committed_AS && strcmp(name, "Committed_AS") == 0) Committed_AS = value;
		else if(!VmallocTotal && strcmp(name, "VmallocTotal") == 0) VmallocTotal = value;
		else if(!VmallocUsed && strcmp(name, "VmallocUsed") == 0) VmallocUsed = value;
		else if(!VmallocChunk && strcmp(name, "VmallocChunk") == 0) VmallocChunk = value;
		else if(!HardwareCorrupted && strcmp(name, "HardwareCorrupted") == 0) { HardwareCorrupted = value; hwcorrupted = 1; }
		else if(!AnonHugePages && strcmp(name, "AnonHugePages") == 0) AnonHugePages = value;
		else if(!HugePages_Total && strcmp(name, "HugePages_Total") == 0) HugePages_Total = value;
		else if(!HugePages_Free && strcmp(name, "HugePages_Free") == 0) HugePages_Free = value;
		else if(!HugePages_Rsvd && strcmp(name, "HugePages_Rsvd") == 0) HugePages_Rsvd = value;
		else if(!HugePages_Surp && strcmp(name, "HugePages_Surp") == 0) HugePages_Surp = value;
		else if(!Hugepagesize && strcmp(name, "Hugepagesize") == 0) Hugepagesize = value;
		else if(!DirectMap4k && strcmp(name, "DirectMap4k") == 0) DirectMap4k = value;
		else if(!DirectMap2M && strcmp(name, "DirectMap2M") == 0) DirectMap2M = value;
	}
	fclose(fp);

	// --------------------------------------------------------------------
	
	// http://stackoverflow.com/questions/3019748/how-to-reliably-measure-available-memory-in-linux
	unsigned long long MemUsed = MemTotal - MemFree - Cached - Buffers;

	RRD_STATS *st = rrd_stats_find("system.ram");
	if(!st) {
		st = rrd_stats_create("system", "ram", NULL, "mem", "System RAM", "MB", save_history);

		rrd_stats_dimension_add(st, "buffers", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "used",    NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "cached",  NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "free",    NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "used", &MemUsed, NULL);
	rrd_stats_dimension_set(st, "free", &MemFree, NULL);
	rrd_stats_dimension_set(st, "cached", &Cached, NULL);
	rrd_stats_dimension_set(st, "buffers", &Buffers, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	unsigned long long SwapUsed = SwapTotal - SwapFree;

	st = rrd_stats_find("system.swap");
	if(!st) {
		st = rrd_stats_create("system", "swap", NULL, "mem", "System Swap", "MB", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "free",    NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "used",    NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "used", &SwapUsed, NULL);
	rrd_stats_dimension_set(st, "free", &SwapFree, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	if(hwcorrupted) {
		st = rrd_stats_find("mem.hwcorrupt");
		if(!st) {
			st = rrd_stats_create("mem", "hwcorrupt", NULL, "mem", "Hardware Corrupted ECC", "MB", save_history);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "HardwareCorrupted", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "HardwareCorrupted", &HardwareCorrupted, NULL);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	st = rrd_stats_find("mem.committed");
	if(!st) {
		st = rrd_stats_create("mem", "committed", NULL, "mem", "Committed (Allocated) Memory", "MB", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "Committed_AS", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "Committed_AS", &Committed_AS, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find("mem.writeback");
	if(!st) {
		st = rrd_stats_create("mem", "writeback", NULL, "mem", "Writeback Memory", "MB", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "Dirty", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "Writeback", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "FuseWriteback", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "NfsWriteback", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "Bounce", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "Dirty", &Dirty, NULL);
	rrd_stats_dimension_set(st, "Writeback", &Writeback, NULL);
	rrd_stats_dimension_set(st, "FuseWriteback", &WritebackTmp, NULL);
	rrd_stats_dimension_set(st, "NfsWriteback", &NFS_Unstable, NULL);
	rrd_stats_dimension_set(st, "Bounce", &Bounce, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find("mem.kernel");
	if(!st) {
		st = rrd_stats_create("mem", "kernel", NULL, "mem", "Memory Used by Kernel", "MB", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "Slab", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "KernelStack", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "PageTables", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "VmallocUsed", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "KernelStack", &KernelStack, NULL);
	rrd_stats_dimension_set(st, "Slab", &Slab, NULL);
	rrd_stats_dimension_set(st, "PageTables", &PageTables, NULL);
	rrd_stats_dimension_set(st, "VmallocUsed", &VmallocUsed, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find("mem.slab");
	if(!st) {
		st = rrd_stats_create("mem", "slab", NULL, "mem", "Reclaimable Kernel Memory", "MB", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "reclaimable", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
		rrd_stats_dimension_add(st, "unreclaimable", NULL, sizeof(unsigned long long), 0, 1, 1024, RRD_DIMENSION_ABSOLUTE, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "reclaimable", &SReclaimable, NULL);
	rrd_stats_dimension_set(st, "unreclaimable", &SUnreclaim, NULL);
	rrd_stats_done(st);

	return 0;
}

// ----------------------------------------------------------------------------
// /proc/vmstat processor

#define MAX_PROC_VMSTAT_LINE 4096
#define MAX_PROC_VMSTAT_NAME 1024

int do_proc_vmstat() {
	char buffer[MAX_PROC_VMSTAT_LINE+1] = "";

	FILE *fp = fopen("/proc/vmstat", "r");
	if(!fp) {
		error("Cannot read /proc/vmstat.");
		return 1;
	}

	unsigned long long nr_free_pages = 0, nr_inactive_anon = 0, nr_active_anon = 0, nr_inactive_file = 0, nr_active_file = 0, nr_unevictable = 0, nr_mlock = 0,
		nr_anon_pages = 0, nr_mapped = 0, nr_file_pages = 0, nr_dirty = 0, nr_writeback = 0, nr_slab_reclaimable = 0, nr_slab_unreclaimable = 0, nr_page_table_pages = 0,
		nr_kernel_stack = 0, nr_unstable = 0, nr_bounce = 0, nr_vmscan_write = 0, nr_vmscan_immediate_reclaim = 0, nr_writeback_temp = 0, nr_isolated_anon = 0, nr_isolated_file = 0,
		nr_shmem = 0, nr_dirtied = 0, nr_written = 0, nr_anon_transparent_hugepages = 0, nr_dirty_threshold = 0, nr_dirty_background_threshold = 0,
		pgpgin = 0, pgpgout = 0, pswpin = 0, pswpout = 0, pgalloc_dma = 0, pgalloc_dma32 = 0, pgalloc_normal = 0, pgalloc_movable = 0, pgfree = 0, pgactivate = 0, pgdeactivate = 0,
		pgfault = 0, pgmajfault = 0, pgrefill_dma = 0, pgrefill_dma32 = 0, pgrefill_normal = 0, pgrefill_movable = 0, pgsteal_kswapd_dma = 0, pgsteal_kswapd_dma32 = 0,
		pgsteal_kswapd_normal = 0, pgsteal_kswapd_movable = 0, pgsteal_direct_dma = 0, pgsteal_direct_dma32 = 0, pgsteal_direct_normal = 0, pgsteal_direct_movable = 0, 
		pgscan_kswapd_dma = 0, pgscan_kswapd_dma32 = 0, pgscan_kswapd_normal = 0, pgscan_kswapd_movable = 0, pgscan_direct_dma = 0, pgscan_direct_dma32 = 0, pgscan_direct_normal = 0,
		pgscan_direct_movable = 0, pginodesteal = 0, slabs_scanned = 0, kswapd_inodesteal = 0, kswapd_low_wmark_hit_quickly = 0, kswapd_high_wmark_hit_quickly = 0,
		kswapd_skip_congestion_wait = 0, pageoutrun = 0, allocstall = 0, pgrotated = 0, compact_blocks_moved = 0, compact_pages_moved = 0, compact_pagemigrate_failed = 0,
		compact_stall = 0, compact_fail = 0, compact_success = 0, htlb_buddy_alloc_success = 0, htlb_buddy_alloc_fail = 0, unevictable_pgs_culled = 0, unevictable_pgs_scanned = 0,
		unevictable_pgs_rescued = 0, unevictable_pgs_mlocked = 0, unevictable_pgs_munlocked = 0, unevictable_pgs_cleared = 0, unevictable_pgs_stranded = 0, unevictable_pgs_mlockfreed = 0,
		thp_fault_alloc = 0, thp_fault_fallback = 0, thp_collapse_alloc = 0, thp_collapse_alloc_failed = 0, thp_split = 0;

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_VMSTAT_NAME, fp);
		if(!p) break;

		// remove ':'
		while((p = strchr(buffer, ':'))) *p = ' ';

		char name[MAX_PROC_MEMINFO_NAME + 1] = "";
		unsigned long long value = 0;

		int r = sscanf(buffer, "%s %llu\n", name, &value);

		if(r == EOF) break;
		if(r != 2) error("Cannot read /proc/meminfo line. Expected 2 params, read %d.", r);

		     if(!nr_free_pages && strcmp(name, "nr_free_pages") == 0) nr_free_pages = value;
		else if(!nr_inactive_anon && strcmp(name, "nr_inactive_anon") == 0) nr_inactive_anon = value;
		else if(!nr_active_anon && strcmp(name, "nr_active_anon") == 0) nr_active_anon = value;
		else if(!nr_inactive_file && strcmp(name, "nr_inactive_file") == 0) nr_inactive_file = value;
		else if(!nr_active_file && strcmp(name, "nr_active_file") == 0) nr_active_file = value;
		else if(!nr_unevictable && strcmp(name, "nr_unevictable") == 0) nr_unevictable = value;
		else if(!nr_mlock && strcmp(name, "nr_mlock") == 0) nr_mlock = value;
		else if(!nr_anon_pages && strcmp(name, "nr_anon_pages") == 0) nr_anon_pages = value;
		else if(!nr_mapped && strcmp(name, "nr_mapped") == 0) nr_mapped = value;
		else if(!nr_file_pages && strcmp(name, "nr_file_pages") == 0) nr_file_pages = value;
		else if(!nr_dirty && strcmp(name, "nr_dirty") == 0) nr_dirty = value;
		else if(!nr_writeback && strcmp(name, "nr_writeback") == 0) nr_writeback = value;
		else if(!nr_slab_reclaimable && strcmp(name, "nr_slab_reclaimable") == 0) nr_slab_reclaimable = value;
		else if(!nr_slab_unreclaimable && strcmp(name, "nr_slab_unreclaimable") == 0) nr_slab_unreclaimable = value;
		else if(!nr_page_table_pages && strcmp(name, "nr_page_table_pages") == 0) nr_page_table_pages = value;
		else if(!nr_kernel_stack && strcmp(name, "nr_kernel_stack") == 0) nr_kernel_stack = value;
		else if(!nr_unstable && strcmp(name, "nr_unstable") == 0) nr_unstable = value;
		else if(!nr_bounce && strcmp(name, "nr_bounce") == 0) nr_bounce = value;
		else if(!nr_vmscan_write && strcmp(name, "nr_vmscan_write") == 0) nr_vmscan_write = value;
		else if(!nr_vmscan_immediate_reclaim && strcmp(name, "nr_vmscan_immediate_reclaim") == 0) nr_vmscan_immediate_reclaim = value;
		else if(!nr_writeback_temp && strcmp(name, "nr_writeback_temp") == 0) nr_writeback_temp = value;
		else if(!nr_isolated_anon && strcmp(name, "nr_isolated_anon") == 0) nr_isolated_anon = value;
		else if(!nr_isolated_file && strcmp(name, "nr_isolated_file") == 0) nr_isolated_file = value;
		else if(!nr_shmem && strcmp(name, "nr_shmem") == 0) nr_shmem = value;
		else if(!nr_dirtied && strcmp(name, "nr_dirtied") == 0) nr_dirtied = value;
		else if(!nr_written && strcmp(name, "nr_written") == 0) nr_written = value;
		else if(!nr_anon_transparent_hugepages && strcmp(name, "nr_anon_transparent_hugepages") == 0) nr_anon_transparent_hugepages = value;
		else if(!nr_dirty_threshold && strcmp(name, "nr_dirty_threshold") == 0) nr_dirty_threshold = value;
		else if(!nr_dirty_background_threshold && strcmp(name, "nr_dirty_background_threshold") == 0) nr_dirty_background_threshold = value;
		else if(!pgpgin && strcmp(name, "pgpgin") == 0) pgpgin = value;
		else if(!pgpgout && strcmp(name, "pgpgout") == 0) pgpgout = value;
		else if(!pswpin && strcmp(name, "pswpin") == 0) pswpin = value;
		else if(!pswpout && strcmp(name, "pswpout") == 0) pswpout = value;
		else if(!pgalloc_dma && strcmp(name, "pgalloc_dma") == 0) pgalloc_dma = value;
		else if(!pgalloc_dma32 && strcmp(name, "pgalloc_dma32") == 0) pgalloc_dma32 = value;
		else if(!pgalloc_normal && strcmp(name, "pgalloc_normal") == 0) pgalloc_normal = value;
		else if(!pgalloc_movable && strcmp(name, "pgalloc_movable") == 0) pgalloc_movable = value;
		else if(!pgfree && strcmp(name, "pgfree") == 0) pgfree = value;
		else if(!pgactivate && strcmp(name, "pgactivate") == 0) pgactivate = value;
		else if(!pgdeactivate && strcmp(name, "pgdeactivate") == 0) pgdeactivate = value;
		else if(!pgfault && strcmp(name, "pgfault") == 0) pgfault = value;
		else if(!pgmajfault && strcmp(name, "pgmajfault") == 0) pgmajfault = value;
		else if(!pgrefill_dma && strcmp(name, "pgrefill_dma") == 0) pgrefill_dma = value;
		else if(!pgrefill_dma32 && strcmp(name, "pgrefill_dma32") == 0) pgrefill_dma32 = value;
		else if(!pgrefill_normal && strcmp(name, "pgrefill_normal") == 0) pgrefill_normal = value;
		else if(!pgrefill_movable && strcmp(name, "pgrefill_movable") == 0) pgrefill_movable = value;
		else if(!pgsteal_kswapd_dma && strcmp(name, "pgsteal_kswapd_dma") == 0) pgsteal_kswapd_dma = value;
		else if(!pgsteal_kswapd_dma32 && strcmp(name, "pgsteal_kswapd_dma32") == 0) pgsteal_kswapd_dma32 = value;
		else if(!pgsteal_kswapd_normal && strcmp(name, "pgsteal_kswapd_normal") == 0) pgsteal_kswapd_normal = value;
		else if(!pgsteal_kswapd_movable && strcmp(name, "pgsteal_kswapd_movable") == 0) pgsteal_kswapd_movable = value;
		else if(!pgsteal_direct_dma && strcmp(name, "pgsteal_direct_dma") == 0) pgsteal_direct_dma = value;
		else if(!pgsteal_direct_dma32 && strcmp(name, "pgsteal_direct_dma32") == 0) pgsteal_direct_dma32 = value;
		else if(!pgsteal_direct_normal && strcmp(name, "pgsteal_direct_normal") == 0) pgsteal_direct_normal = value;
		else if(!pgsteal_direct_movable && strcmp(name, "pgsteal_direct_movable") == 0) pgsteal_direct_movable = value;
		else if(!pgscan_kswapd_dma && strcmp(name, "pgscan_kswapd_dma") == 0) pgscan_kswapd_dma = value;
		else if(!pgscan_kswapd_dma32 && strcmp(name, "pgscan_kswapd_dma32") == 0) pgscan_kswapd_dma32 = value;
		else if(!pgscan_kswapd_normal && strcmp(name, "pgscan_kswapd_normal") == 0) pgscan_kswapd_normal = value;
		else if(!pgscan_kswapd_movable && strcmp(name, "pgscan_kswapd_movable") == 0) pgscan_kswapd_movable = value;
		else if(!pgscan_direct_dma && strcmp(name, "pgscan_direct_dma") == 0) pgscan_direct_dma = value;
		else if(!pgscan_direct_dma32 && strcmp(name, "pgscan_direct_dma32") == 0) pgscan_direct_dma32 = value;
		else if(!pgscan_direct_normal && strcmp(name, "pgscan_direct_normal") == 0) pgscan_direct_normal = value;
		else if(!pgscan_direct_movable && strcmp(name, "pgscan_direct_movable") == 0) pgscan_direct_movable = value;
		else if(!pginodesteal && strcmp(name, "pginodesteal") == 0) pginodesteal = value;
		else if(!slabs_scanned && strcmp(name, "slabs_scanned") == 0) slabs_scanned = value;
		else if(!kswapd_inodesteal && strcmp(name, "kswapd_inodesteal") == 0) kswapd_inodesteal = value;
		else if(!kswapd_low_wmark_hit_quickly && strcmp(name, "kswapd_low_wmark_hit_quickly") == 0) kswapd_low_wmark_hit_quickly = value;
		else if(!kswapd_high_wmark_hit_quickly && strcmp(name, "kswapd_high_wmark_hit_quickly") == 0) kswapd_high_wmark_hit_quickly = value;
		else if(!kswapd_skip_congestion_wait && strcmp(name, "kswapd_skip_congestion_wait") == 0) kswapd_skip_congestion_wait = value;
		else if(!pageoutrun && strcmp(name, "pageoutrun") == 0) pageoutrun = value;
		else if(!allocstall && strcmp(name, "allocstall") == 0) allocstall = value;
		else if(!pgrotated && strcmp(name, "pgrotated") == 0) pgrotated = value;
		else if(!compact_blocks_moved && strcmp(name, "compact_blocks_moved") == 0) compact_blocks_moved = value;
		else if(!compact_pages_moved && strcmp(name, "compact_pages_moved") == 0) compact_pages_moved = value;
		else if(!compact_pagemigrate_failed && strcmp(name, "compact_pagemigrate_failed") == 0) compact_pagemigrate_failed = value;
		else if(!compact_stall && strcmp(name, "compact_stall") == 0) compact_stall = value;
		else if(!compact_fail && strcmp(name, "compact_fail") == 0) compact_fail = value;
		else if(!compact_success && strcmp(name, "compact_success") == 0) compact_success = value;
		else if(!htlb_buddy_alloc_success && strcmp(name, "htlb_buddy_alloc_success") == 0) htlb_buddy_alloc_success = value;
		else if(!htlb_buddy_alloc_fail && strcmp(name, "htlb_buddy_alloc_fail") == 0) htlb_buddy_alloc_fail = value;
		else if(!unevictable_pgs_culled && strcmp(name, "unevictable_pgs_culled") == 0) unevictable_pgs_culled = value;
		else if(!unevictable_pgs_scanned && strcmp(name, "unevictable_pgs_scanned") == 0) unevictable_pgs_scanned = value;
		else if(!unevictable_pgs_rescued && strcmp(name, "unevictable_pgs_rescued") == 0) unevictable_pgs_rescued = value;
		else if(!unevictable_pgs_mlocked && strcmp(name, "unevictable_pgs_mlocked") == 0) unevictable_pgs_mlocked = value;
		else if(!unevictable_pgs_munlocked && strcmp(name, "unevictable_pgs_munlocked") == 0) unevictable_pgs_munlocked = value;
		else if(!unevictable_pgs_cleared && strcmp(name, "unevictable_pgs_cleared") == 0) unevictable_pgs_cleared = value;
		else if(!unevictable_pgs_stranded && strcmp(name, "unevictable_pgs_stranded") == 0) unevictable_pgs_stranded = value;
		else if(!unevictable_pgs_mlockfreed && strcmp(name, "unevictable_pgs_mlockfreed") == 0) unevictable_pgs_mlockfreed = value;
		else if(!thp_fault_alloc && strcmp(name, "thp_fault_alloc") == 0) thp_fault_alloc = value;
		else if(!thp_fault_fallback && strcmp(name, "thp_fault_fallback") == 0) thp_fault_fallback = value;
		else if(!thp_collapse_alloc && strcmp(name, "thp_collapse_alloc") == 0) thp_collapse_alloc = value;
		else if(!thp_collapse_alloc_failed && strcmp(name, "thp_collapse_alloc_failed") == 0) thp_collapse_alloc_failed = value;
		else if(!thp_split && strcmp(name, "thp_split") == 0) thp_split = value;
	}
	fclose(fp);

	// --------------------------------------------------------------------
	
	RRD_STATS *st = rrd_stats_find("system.swapio");
	if(!st) {
		st = rrd_stats_create("system", "swapio", NULL, "mem", "Swap I/O", "kilobytes/s", save_history);

		rrd_stats_dimension_add(st, "in",  NULL, sizeof(unsigned long long), 0, sysconf(_SC_PAGESIZE), 1024, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "out", NULL, sizeof(unsigned long long), 0, -sysconf(_SC_PAGESIZE), 1024, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "in", &pswpin, NULL);
	rrd_stats_dimension_set(st, "out", &pswpout, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find("system.io");
	if(!st) {
		st = rrd_stats_create("system", "io", NULL, "disk", "Disk I/O", "kilobytes/s", save_history);

		rrd_stats_dimension_add(st, "in",  NULL, sizeof(unsigned long long), 0,  1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "out", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "in", &pgpgin, NULL);
	rrd_stats_dimension_set(st, "out", &pgpgout, NULL);
	rrd_stats_done(st);

	// --------------------------------------------------------------------
	
	st = rrd_stats_find("system.pgfaults");
	if(!st) {
		st = rrd_stats_create("system", "pgfaults", NULL, "mem", "Memory Page Faults", "page faults/s", save_history);
		st->isdetail = 1;

		rrd_stats_dimension_add(st, "minor",  NULL, sizeof(unsigned long long), 0,  1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
		rrd_stats_dimension_add(st, "major", NULL, sizeof(unsigned long long), 0, -1, 1, RRD_DIMENSION_INCREMENTAL, NULL);
	}
	else rrd_stats_next(st);

	rrd_stats_dimension_set(st, "minor", &pgfault, NULL);
	rrd_stats_dimension_set(st, "major", &pgmajfault, NULL);
	rrd_stats_done(st);

	return 0;
}

// ----------------------------------------------------------------------------
// /proc processor

void *proc_main(void *ptr)
{
	struct rusage me, me_last;
	struct timeval last, now;

	gettimeofday(&last, NULL);
	last.tv_sec -= update_every;
	
	// when ZERO, attempt to do it
	int	vdo_proc_net_dev = 0;
	int vdo_proc_diskstats = 0;
	int vdo_proc_net_snmp = 0;
	int vdo_proc_net_netstat = 0;
	int vdo_proc_net_stat_conntrack = 0;
	int vdo_proc_net_ip_vs_stats = 0;
	int vdo_proc_stat = 0;
	int vdo_proc_meminfo = 0;
	int vdo_proc_vmstat = 0;

	RRD_STATS *stcpu = NULL;

	gettimeofday(&last, NULL);
	getrusage(RUSAGE_SELF, &me_last);

	unsigned long long usec = 0, susec = 0;
	for(;1;) {
		
		// BEGIN -- the job to be done
		if(!vdo_proc_net_dev)				vdo_proc_net_dev			= do_proc_net_dev(usec);
		if(!vdo_proc_diskstats)				vdo_proc_diskstats			= do_proc_diskstats(usec);
		if(!vdo_proc_net_snmp)				vdo_proc_net_snmp			= do_proc_net_snmp(usec);
		if(!vdo_proc_net_netstat)			vdo_proc_net_netstat		= do_proc_net_netstat(usec);
		if(!vdo_proc_net_stat_conntrack)	vdo_proc_net_stat_conntrack	= do_proc_net_stat_conntrack(usec);
		if(!vdo_proc_net_ip_vs_stats)		vdo_proc_net_ip_vs_stats	= do_proc_net_ip_vs_stats(usec);
		if(!vdo_proc_stat)					vdo_proc_stat 				= do_proc_stat(usec);
		if(!vdo_proc_meminfo)				vdo_proc_meminfo			= do_proc_meminfo(usec);
		if(!vdo_proc_vmstat)				vdo_proc_vmstat				= do_proc_vmstat(usec);
		// END -- the job is done
		
		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&now, NULL);
		usec = usecdiff(&now, &last) - susec;
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: last loop took %llu usec (worked for %llu, sleeped for %llu).", usec + susec, usec, susec);
		
		if(usec < (update_every * 1000000)) susec = (update_every * 1000000) - usec;
		else susec = 0;
		
		// make sure we will wait at least 100ms
		if(susec < 100000) susec = 100000;
		
		// --------------------------------------------------------------------

		if(getrusage(RUSAGE_SELF, &me) == 0) {
		
			unsigned long long cpuuser = me.ru_utime.tv_sec * 1000000L + me.ru_utime.tv_usec;
			unsigned long long cpusyst = me.ru_stime.tv_sec * 1000000L + me.ru_stime.tv_usec;

			if(!stcpu) stcpu = rrd_stats_find("cpu.netdata");
			if(!stcpu) {
				stcpu = rrd_stats_create("cpu", "netdata", NULL, "cpu", "NetData CPU usage", "milliseconds/s", save_history);

				rrd_stats_dimension_add(stcpu, "user",  NULL, sizeof(unsigned long long), 0,  1, 1000, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_add(stcpu, "system", NULL, sizeof(unsigned long long), 0, 1, 1000, RRD_DIMENSION_INCREMENTAL, NULL);
			}
			else rrd_stats_next(stcpu);

			rrd_stats_dimension_set(stcpu, "user", &cpuuser, NULL);
			rrd_stats_dimension_set(stcpu, "system", &cpusyst, NULL);
			rrd_stats_done(stcpu);
		}

		usleep(susec);
		
		// copy current to last
		bcopy(&me, &me_last, sizeof(struct rusage));
		bcopy(&now, &last, sizeof(struct timeval));
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

	char leafid[RRD_STATS_NAME_MAX + 1];
	char parentid[RRD_STATS_NAME_MAX + 1];

	int hasparent;
	int isleaf;
	unsigned long long bytes;

	struct tc_class *next;
};

struct tc_device {
	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];
	char group[RRD_STATS_NAME_MAX + 1];

	struct tc_class *classes;
};

void tc_device_commit(struct tc_device *d)
{
	// we only need to add leaf classes
	struct tc_class *c, *x;

	for ( c = d->classes ; c ; c = c->next)
		c->isleaf = 1;

	for ( c = d->classes ; c ; c = c->next) {
		for ( x = d->classes ; x ; x = x->next) {
			if(x->parentid[0] && (strcmp(c->id, x->parentid) == 0 || strcmp(c->leafid, x->parentid) == 0)) {
				// debug(D_TC_LOOP, "TC: In device '%s', class '%s' (leafid: '%s') has leaf the class '%s' (parentid: '%s').", d->name, c->name, c->leafid, x->name, x->parentid);
				c->isleaf = 0;
				x->hasparent = 1;
			}
		}
	}
	
	// debugging:
	/*
	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent) debug(D_TC_LOOP, "TC: Device %s, class %s, OK", d->name, c->id);
		else debug(D_TC_LOOP, "TC: Device %s, class %s, IGNORE (isleaf: %d, hasparent: %d, parent: %s)", d->name, c->id, c->isleaf, c->hasparent, c->parentid);
	}
	*/

	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent) break;
	}
	if(!c) {
		debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. No leaf classes.", d->name);
		return;
	}

	RRD_STATS *st = rrd_stats_find_bytype(RRD_TYPE_TC, d->id);
	if(!st) {
		debug(D_TC_LOOP, "TC: Committing new TC device '%s'", d->name);

		st = rrd_stats_create(RRD_TYPE_TC, d->id, d->name, d->group, "Class usage for ", "kilobits/s", save_history);

		for ( c = d->classes ; c ; c = c->next) {
			if(c->isleaf && c->hasparent)
				rrd_stats_dimension_add(st, c->id, c->name, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
		}
	}
	else {
		unsigned long long usec = rrd_stats_next(st);
		debug(D_TC_LOOP, "TC: Committing TC device '%s' after %llu usec", d->name, usec);
	}

	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent) {
			if(rrd_stats_dimension_set(st, c->id, &c->bytes, NULL) != 0) {
				
				// new class, we have to add it
				rrd_stats_dimension_add(st, c->id, c->name, sizeof(unsigned long long), 0, 8, 1024, RRD_DIMENSION_INCREMENTAL, NULL);
				rrd_stats_dimension_set(st, c->id, &c->bytes, NULL);
			}

			// if it has a name, different to the id
			if(strcmp(c->id, c->name) != 0) {
				// update the rrd dimension with the new name
				RRD_DIMENSION *rd;
				for(rd = st->dimensions ; rd ; rd = rd->next) {
					if(strcmp(rd->id, c->id) == 0) { strcpy(rd->name, c->name); break; }
				}
			}
		}
	}
	rrd_stats_done(st);
}

void tc_device_set_class_name(struct tc_device *d, char *id, char *name)
{
	struct tc_class *c;
	for ( c = d->classes ; c ; c = c->next) {
		if(strcmp(c->id, id) == 0) {
			strncpy(c->name, name, RRD_STATS_NAME_MAX);
			// no need for null termination - it is already null
			break;
		}
	}
}

void tc_device_set_device_name(struct tc_device *d, char *name)
{
	strncpy(d->name, name, RRD_STATS_NAME_MAX);
	// no need for null termination - it is already null
}

void tc_device_set_device_group(struct tc_device *d, char *name)
{
	strncpy(d->group, name, RRD_STATS_NAME_MAX);
	// no need for null termination - it is already null
}

struct tc_device *tc_device_create(char *name)
{
	struct tc_device *d;

	d = calloc(1, sizeof(struct tc_device));
	if(!d) {
		fatal("Cannot allocate memory for tc_device %s", name);
		return NULL;
	}

	strncpy(d->id, name, RRD_STATS_NAME_MAX);
	strcpy(d->name, d->id);
	strcpy(d->group, d->id);

	// no need for null termination on the strings, because of calloc()

	return(d);
}

struct tc_class *tc_class_add(struct tc_device *n, char *id, char *parentid, char *leafid)
{
	struct tc_class *c;

	c = calloc(1, sizeof(struct tc_class));
	if(!c) {
		fatal("Cannot allocate memory for tc class");
		return NULL;
	}

	c->next = n->classes;
	n->classes = c;

	strncpy(c->id, id, RRD_STATS_NAME_MAX);
	strcpy(c->name, c->id);
	if(parentid) strncpy(c->parentid, parentid, RRD_STATS_NAME_MAX);
	if(leafid) strncpy(c->leafid, leafid, RRD_STATS_NAME_MAX);

	// no need for null termination on the strings, because of calloc()

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

		sprintf(buffer, "exec ./tc-all.sh %d", update_every);
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
				if(device) {
					tc_device_free(device);
					device = NULL;
					class = NULL;
				}

				p = strsep(&b, " \n");
				if(p && *p) {
					device = tc_device_create(p);
					class = NULL;
				}
			}
			else if(device && (strcmp(p, "class") == 0)) {
				p = strsep(&b, " \n"); // the class: htb, fq_codel, etc
				char *id       = strsep(&b, " \n"); // the class major:minor
				char *parent   = strsep(&b, " \n"); // 'parent' or 'root'
				char *parentid = strsep(&b, " \n"); // the parent's id
				char *leaf     = strsep(&b, " \n"); // 'leaf'
				char *leafid   = strsep(&b, " \n"); // leafid

				if(id && *id
					&& parent && *parent
					&& parentid && *parentid
					&& (
						(strcmp(parent, "parent") == 0 && parentid && *parentid)
						|| strcmp(parent, "root") == 0
					)) {

					if(strcmp(parent, "root") == 0) {
						parentid = NULL;
						leafid = NULL;
					}
					else if(!leaf || strcmp(leaf, "leaf") != 0)
						leafid = NULL;

					char leafbuf[20 + 1] = "";
					if(leafid && leafid[strlen(leafid) - 1] == ':') {
						strncpy(leafbuf, leafid, 20 - 1);
						strcat(leafbuf, "1");
						leafid = leafbuf;
					}

					class = tc_class_add(device, id, parentid, leafid);
				}
			}
			else if(device && class && (strcmp(p, "Sent") == 0)) {
				p = strsep(&b, " \n");
				if(p && *p) class->bytes = atoll(p);
			}
			else if(device && (strcmp(p, "SETDEVICENAME") == 0)) {
				char *name = strsep(&b, " \n");
				if(name && *name) tc_device_set_device_name(device, name);
			}
			else if(device && (strcmp(p, "SETDEVICEGROUP") == 0)) {
				char *name = strsep(&b, " \n");
				if(name && *name) tc_device_set_device_group(device, name);
			}
			else if(device && (strcmp(p, "SETCLASSNAME") == 0)) {
				char *id    = strsep(&b, " \n");
				char *path  = strsep(&b, " \n");
				if(id && *id && path && *path) tc_device_set_class_name(device, id, path);
			}
			else if((strcmp(p, "MYPID") == 0)) {
				char *id = strsep(&b, " \n");
				tc_child_pid = atol(id);
				debug(D_TC_LOOP, "TC: Child PID is %d.", tc_child_pid);
			}
		}
		pclose(fp);

		if(device) {
			tc_device_free(device);
			device = NULL;
			class = NULL;
		}

		sleep(update_every);
	}

	return NULL;
}

// ----------------------------------------------------------------------------
// cpu jitter calculation

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

void *cpuidlejitter_main(void *ptr)
{

	struct timeval before, after;

	while(1) {
		unsigned long long usec, susec = 0;

		while(susec < (update_every * 1000000L)) {

			gettimeofday(&before, NULL);
			usleep(CPU_IDLEJITTER_SLEEP_TIME_MS * 1000);
			gettimeofday(&after, NULL);

			// calculate the time it took for a full loop
			usec = usecdiff(&after, &before);
			susec += usec;
		}
		usec -= (CPU_IDLEJITTER_SLEEP_TIME_MS * 1000);

		RRD_STATS *st = rrd_stats_find("system.idlejitter");
		if(!st) {
			st = rrd_stats_create("system", "idlejitter", NULL, "cpu", "CPU Idle Jitter", "microseconds lost/s", save_history);

			rrd_stats_dimension_add(st, "jitter", NULL, sizeof(unsigned long long), 0, 1, 1, RRD_DIMENSION_ABSOLUTE, NULL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "jitter", &usec, NULL);
		rrd_stats_done(st);
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
		case SIGSEGV:
			error("Signaled exit (signal %d).", signo);
			if(tc_child_pid) kill(tc_child_pid, SIGTERM);
			tc_child_pid = 0;
			exit(1);
			break;

		case SIGCHLD:
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
		if(i != debug_fd) close(i);

	if(debug_fd >= 0) {
		silent = 0;

		if(dup2(debug_fd, STDOUT_FILENO) < 0)
			silent = 1;

		if(dup2(debug_fd, STDERR_FILENO) < 0)
			silent = 1;
	}
	else silent = 1;
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
		else if(strcmp(argv[i], "-dl") == 0 && (i+1) < argc) {
			debug_log = argv[i+1];
			debug_fd = open(debug_log, O_WRONLY | O_APPEND | O_CREAT, 0666);
			if(debug_fd < 0) {
				fprintf(stderr, "Cannot open file '%s'. Reason: %s\n", debug_log, strerror(errno));
				exit(1);
			}
			debug(D_OPTIONS, "Debug LOG set to '%s'.", debug_log);
			i++;
		}
		else if(strcmp(argv[i], "-df") == 0 && (i+1) < argc) {
			debug_flags = strtoull(argv[i+1], NULL, 0);
			debug(D_OPTIONS, "Debug flags set to '0x%8llx'.", debug_flags);
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
			fprintf(stderr, "\nUSAGE: %s [-d] [-l LINES_TO_SAVE] [-u UPDATE_TIMER] [-p LISTEN_PORT] [-dl debug log file] [-df debug flags].\n\n", argv[0]);
			fprintf(stderr, "  -d enable daemon mode (run in background).\n");
			fprintf(stderr, "  -l LINES_TO_SAVE can be from 5 to %d lines in JSON data. Default: %d.\n", HISTORY_MAX, HISTORY);
			fprintf(stderr, "  -t UPDATE_TIMER can be from 1 to %d seconds. Default: %d.\n", UPDATE_EVERY_MAX, UPDATE_EVERY);
			fprintf(stderr, "  -p LISTEN_PORT can be from 1 to %d. Default: %d.\n", 65535, LISTEN_PORT);
			fprintf(stderr, "  -u USERNAME can be any system username to run as. Default: none.\n");
			fprintf(stderr, "  -dl FILENAME write debug log to FILENAME. Default: none.\n");
			fprintf(stderr, "  -df FLAGS debug options. Default: 0x%8llx.\n", debug_flags);
			exit(1);
		}
	}

	if(gethostname(hostname, HOSTNAME_MAX) == -1)
		error("WARNING: Cannot get machine hostname.");

	// never become a problem
	if(nice(20) == -1) {
		fprintf(stderr, "Cannot lower my CPU priority. Error: %s.\n", strerror(errno));
	}

	if(sizeof(long long) >= sizeof(long double)) {
		fprintf(stderr, "\n\nWARNING:\nThis system does not support [long double] variables properly.\nArithmetic overflows and rounding errors may occur.\n\n");
	}

	if(daemon) become_daemon();

	// open syslog
	openlog("netdata", LOG_PID, LOG_DAEMON);
	syslog(LOG_NOTICE, "netdata started.");

	// make sure we cleanup correctly
	atexit(bye);

	// catch all signals
	for (i = 1 ; i < 65 ;i++) if(i != SIGSEGV) signal(i,  sig_handler);
	

	pthread_t p_proc, p_tc, p_jitter;
	int r_proc, r_tc, r_jitter;

	// spawn a child to collect data
	r_proc   = pthread_create(&p_proc,   NULL, proc_main,          NULL);
	r_tc     = pthread_create(&p_tc,     NULL, tc_main,            NULL);
	r_jitter = pthread_create(&p_jitter, NULL, cpuidlejitter_main, NULL);

	// the main process - the web server listener
	//sleep(1);
	socket_listen_main(NULL);

	// wait for the childs to finish
	pthread_join(p_tc,  NULL);
	pthread_join(p_proc,  NULL);

	printf("TC            thread returns: %d\n", r_tc);
	printf("PROC NET DEV  thread returns: %d\n", r_proc);
	printf("CPU JITTER    thread returns: %d\n", r_jitter);

	exit(0);
}
