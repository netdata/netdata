// enable strcasestr()
#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
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

#include <pthread.h>



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

#define CT_APPLICATION_JSON				1
#define CT_TEXT_PLAIN					2
#define CT_TEXT_HTML					3
#define CT_APPLICATION_X_JAVASCRIPT		4
#define CT_TEXT_CSS						5
#define CT_TEXT_XML						6
#define CT_APPLICATION_XML				7
#define CT_TEXT_XSL						8

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

#define MAX_HTTP_HEADER_SIZE 1024

#define MAX_PROC_NET_DEV_LINE 4096
#define MAX_PROC_NET_DEV_IFACE_NAME 1024

#define MAX_PROC_DISKSTATS_LINE 4096
#define MAX_PROC_DISKSTATS_DISK_NAME 1024


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
	char name[RRD_STATS_NAME_MAX + 1];
	size_t bytes;
	size_t entries;

	long multiplier;
	long divisor;

	void *values;
	
	struct rrd_dimension *next;
};
typedef struct rrd_dimension RRD_DIMENSION;

struct rrd_stats {
	pthread_mutex_t mutex;

	char name[RRD_STATS_NAME_MAX + 1];

	size_t entries;
	size_t last_entry;

	struct timeval *times;
	RRD_DIMENSION *dimensions;
	struct rrd_stats *next;
};
typedef struct rrd_stats RRD_STATS;

RRD_STATS *root = NULL;
pthread_mutex_t root_mutex = PTHREAD_MUTEX_INITIALIZER;

RRD_STATS *rrd_stats_create(const char *name, unsigned long entries)
{
	RRD_STATS *st = NULL;

	debug(D_RRD_STATS, "Creating RRD_STATS for '%s'.", name);

	st = calloc(sizeof(RRD_STATS), 1);
	if(!st) return NULL;

	st->times = calloc(entries, sizeof(struct timeval));
	if(!st->times) {
		error("Cannot allocate %lu entries of %lu bytes each for RRD_STATS.", st->entries, sizeof(struct timeval));
		free(st);
		return NULL;
	}
	
	strncpy(st->name, name, RRD_STATS_NAME_MAX);
	st->name[RRD_STATS_NAME_MAX] = '\0';

	st->entries = entries;
	st->last_entry = 0;
	st->dimensions = NULL;

	pthread_mutex_init(&st->mutex, NULL);
	pthread_mutex_lock(&st->mutex);
	pthread_mutex_lock(&root_mutex);

	st->next = root;
	root = st;

	pthread_mutex_unlock(&root_mutex);
	// leave st->mutex locked

	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_add(RRD_STATS *st, const char *name, size_t bytes, size_t multiplier, size_t divisor)
{
	RRD_DIMENSION *rd = NULL;

	debug(D_RRD_STATS, "Adding dimension '%s' to RRD_STATS '%s'.", name, st->name);

	rd = calloc(sizeof(RRD_DIMENSION), 1);
	if(!rd) return NULL;

	rd->bytes = bytes;
	rd->entries = st->entries;
	rd->multiplier = multiplier;
	rd->divisor = divisor;
	rd->values = calloc(rd->entries, rd->bytes);
	if(!rd->values) {
		error("Cannot allocate %lu entries of %lu bytes each for RRD_DIMENSION.", rd->entries, rd->bytes);
		free(rd);
		return NULL;
	}

	strncpy(rd->name, name, RRD_STATS_NAME_MAX);
	rd->name[RRD_STATS_NAME_MAX] = '\0';

	rd->next = st->dimensions;
	st->dimensions = rd;

	return(rd);
}

RRD_STATS *rrd_stats_find(const char *name)
{
	pthread_mutex_lock(&root_mutex);

	RRD_STATS *st = root;

	for ( ; st ; st = st->next ) {
		if(strcmp(st->name, name) == 0) break;
	}

	pthread_mutex_unlock(&root_mutex);
	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_find(RRD_STATS *st, const char *dimension)
{
	RRD_DIMENSION *rd = st->dimensions;

	for ( ; rd ; rd = rd->next ) {
		if(strcmp(rd->name, dimension) == 0) break;
	}

	return(rd);
}

void rrd_stats_next(RRD_STATS *st)
{
	struct timeval *now;

	pthread_mutex_lock(&st->mutex);

	// st->last_entry should never be outside the array
	// or, the parallel threads may end up crashing
	st->last_entry = ((st->last_entry + 1) >= st->entries) ? 0 : st->last_entry + 1;

	now = &st->times[st->last_entry];
	gettimeofday(now, NULL);

	// leave mutex locked
}

void rrd_stats_done(RRD_STATS *st)
{
	pthread_mutex_unlock(&st->mutex);
}

void rrd_stats_dimension_set(RRD_STATS *st, const char *dimension, void *data)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, dimension);
	if(!rd) {
		error("Cannot find dimension '%s' on stats '%s'.", dimension, st->name);
		return;
	}

	if(rd->bytes == sizeof(unsigned long long)) {
		long long *dimension = rd->values, *value = data;

		dimension[st->last_entry] = (*value);
	}
	else if(rd->bytes == sizeof(unsigned long)) {
		long *dimension = rd->values, *value = data;

		dimension[st->last_entry] = (*value);
	}
	else if(rd->bytes == sizeof(unsigned int)) {
		int *dimension = rd->values, *value = data;

		dimension[st->last_entry] = (*value);
	}
	else if(rd->bytes == sizeof(unsigned char)) {
		char *dimension = rd->values, *value = data;

		dimension[st->last_entry] = (*value);
	}
	else fatal("I don't know how to handle data of length %d bytes.", rd->bytes);
}

#define GROUP_AVERAGE	0
#define GROUP_MAX 		1

size_t rrd_stats_json(RRD_STATS *st, char *b, size_t length, size_t entries_to_show, size_t group_count, int group_method)
{
	pthread_mutex_lock(&st->mutex);

	// check the options
	if(entries_to_show <= 0) entries_to_show = 1;
	if(group_count <= 0) group_count = 1;
	if(group_count > st->entries / 20) group_count = st->entries / 20;

	size_t i = 0;				// the bytes of JSON output we have generated so far
	size_t printed = 0;			// the lines of JSON data we have generated so far

	size_t last_entry = st->last_entry;
	size_t t, lt;				// t = the current entry, lt = the lest entry of data
	long count = st->entries;	// count down of the entries examined so far
	int pad = 0;				// align the entries when grouping values together

	RRD_DIMENSION *rd;
	size_t c = 0;				// counter for dimension loops
	size_t dimensions = 0;		// the total number of dimensions present

	unsigned long long usec = 0;// usec between the entries
	long long value;			// temp variable for storing data values
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
	i += sprintf(&b[i], "		{\"id\":\"\",\"label\":\"time\",\"pattern\":\"\",\"type\":\"timeofday\"},\n");

	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++)
		i += sprintf(&b[i], "		{\"id\":\"\",\"label\":\"%s\",\"pattern\":\"\",\"type\":\"number\"}%s\n", rd->name, rd->next?",":"");

	i += sprintf(&b[i], "	],\n	\"rows\":\n	[\n");

	// to allow grouping on the same values, we need a pad
	pad = last_entry % group_count;

	// make sure last_entry is within limits
	if(last_entry < 0 || last_entry >= st->entries) last_entry = 0;

	// find the old entry of the round-robin
	t = last_entry + 1;
	if(t >= st->entries) t = 0;
	lt = t;

	// find the current entry
	t++;
	if(t >= st->entries) t = 0;

	// the loop in dimension data
	count -= 2;
	for ( ; t != last_entry && count >= 0 ; lt = t++, count--) {
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

			// strftime(dtm, 200, "[%Y, %m, %d, %H, %M, %S, 0]", tm); // datetime
			strftime(dtm, 200, "[%H, %M, %S, 0]", tm); // timeofday
 			i += sprintf(&b[i], "%s		{\"c\":[{\"v\":%s},", printed?"]},\n":"", dtm);

 			printed++;
 		}

		for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
			if(rd->bytes == sizeof(unsigned long long)) {
				long long *dimension = rd->values;
				value = (dimension[t] - dimension[lt]) * 1000000 * rd->multiplier / usec / rd->divisor;
			}
			else if(rd->bytes == sizeof(unsigned long)) {
				long *dimension = rd->values;
				value = (dimension[t] - dimension[lt]) * 1000000 * rd->multiplier / usec / rd->divisor;
			}
			else if(rd->bytes == sizeof(unsigned int)) {
				int *dimension = rd->values;
				value = (dimension[t] - dimension[lt]) * 1000000 * rd->multiplier / usec / rd->divisor;
			}
			else if(rd->bytes == sizeof(unsigned char)) {
				char *dimension = rd->values;
				value = (dimension[t] - dimension[lt]) * 1000000 * rd->multiplier / usec / rd->divisor;
			}
			else fatal("Cannot produce JSON for size %d bytes dimension.", rd->bytes);

			switch(group_method) {
				case GROUP_MAX:
					if(value > group_values[c]) group_values[c] = value;
					break;

				default:
				case GROUP_AVERAGE:
					group_values[c] += value;
					break;
			}
			group_counts[c]++;

			if(((count-pad) % group_count) == 0) {
				if(group_method == GROUP_AVERAGE) group_values[c] /= group_counts[c];

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

	debug(D_WEB_CLIENT_ACCESS, "%llu: New web client from %s on socket %d.", w->id, inet_ntoa(w->clientaddr.sin_addr), w->ifd);

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

	if(strstr(filename, "/") != 0 || strstr(filename, "..") != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
		w->data->bytes = sprintf(w->data->buffer, "File '%s' is not acceptable. Filenames cannot contain / or ..", filename);
		return 404;
	}

	// check if the file exists
	struct stat stat;
	if(lstat(filename, &stat) != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not found.", w->id, filename);
		w->data->bytes = sprintf(w->data->buffer, "File '%s' is not found.", filename);
		return 404;
	}

	if(stat.st_uid != getuid() && stat.st_uid != geteuid()) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not mine.", w->id, filename);
		w->data->bytes = sprintf(w->data->buffer, "File '%s' is not mine.", filename);
		return 404;
	}

	w->ifd = open(filename, O_NONBLOCK, O_RDONLY);
	if(w->ifd < 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot open file '%s'.", w->id, filename);
		w->data->bytes = sprintf(w->data->buffer, "Cannot open file '%s'.", filename);
		w->ifd = w->ofd;
		return 404;
	}
	
	// pick a Content-Type for the file
	     if(strstr(filename, ".html") != NULL)	w->data->contenttype = CT_TEXT_HTML;
	else if(strstr(filename, ".js") != NULL)	w->data->contenttype = CT_APPLICATION_X_JAVASCRIPT;
	else if(strstr(filename, ".css") != NULL)	w->data->contenttype = CT_TEXT_CSS;
	else if(strstr(filename, ".xml") != NULL)	w->data->contenttype = CT_TEXT_XML;
	else if(strstr(filename, ".xsl") != NULL)	w->data->contenttype = CT_TEXT_XSL;

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
		tok = mystrsep(&url, "/?");

		debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

		if(strcmp(tok, "data") == 0) {
			// the client is requesting rrd data

			// get the name of the data to show
			tok = mystrsep(&url, "/?");
			debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

			// do we have such a data set?
			RRD_STATS *st = rrd_stats_find(tok);
			if(!st) {
				// we don't have it
				code = 404;
				w->data->bytes = sprintf(w->data->buffer, "There are not statistics for '%s'\r\n", tok);
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
					tok = mystrsep(&url, "/?");
					if(tok) lines = atoi(tok);
					if(lines < 5) lines = save_history;
				}
				if(url) {
					// parse the group count required
					tok = mystrsep(&url, "/?");
					if(tok) group_count = atoi(tok);
					if(group_count < 1) group_count = 1;
					if(group_count > save_history / 20) group_count = save_history / 20;
				}
				if(url) {
					// parse the grouping method required
					tok = mystrsep(&url, "/?");
					if(strcmp(tok, "max") == 0) group_method = GROUP_MAX;
					else if(strcmp(tok, "average") == 0) group_method = GROUP_AVERAGE;
					else debug(D_WEB_CLIENT, "%llu: Unknown group method '%s'", w->id, tok);
				}

				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (%d lines, %d group_count, %d group_method).", w->id, st->name, lines, group_count, group_method);

				code = 200;
				w->data->contenttype = CT_APPLICATION_JSON;
				w->data->bytes = rrd_stats_json(st, w->data->buffer, w->data->size, lines, group_count, group_method);
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
		else if(strcmp(tok, "file") == 0) {
			code = mysendfile(w, url);
		}
		else if(strcmp(tok, "favicon.ico") == 0) {
			code = mysendfile(w, "favicon.ico");
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

	char header[MAX_HTTP_HEADER_SIZE+1] = "";
	size_t headerlen = 0;
	headerlen += sprintf(&header[headerlen],
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

	if(w->mode == WEB_CLIENT_MODE_NORMAL) {
		headerlen += sprintf(&header[headerlen],
			"Expires: %s\r\n"
			"Cache-Control: private\r\n"
			, date
			);
	}

	// if we know the content length, put it
	if(w->data->bytes || w->data->rbytes)
		headerlen += sprintf(&header[headerlen],
			"Content-Length: %ld\r\n"
			, w->data->bytes?w->data->bytes:w->data->rbytes
			);
	else w->keepalive = 0;	// content-length is required for keep-alive

	headerlen += sprintf(&header[headerlen], "\r\n");

	// disable TCP_NODELAY, to buffer the header
	int flag = 0;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to disable TCP_NODELAY on socket.", w->id);

	// sent the HTTP header
	debug(D_WEB_CLIENT, "%llu: Sending response HTTP header of size %d.", w->id, headerlen);

	bytes = send(w->ofd, header, headerlen, 0);
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

		if(retval == -1)
			fatal("LISTENER: select() failed.");
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

		if(retval == -1)
			fatal("LISTENER: select() failed.");
		else if(retval) {
			// check for new incoming connections
			if(FD_ISSET(listener, &ifds)) {
				w = web_client_create(listener);	

				w->thread = pthread_create(&w->thread, NULL, new_client, w);
			}
		}
		// else timeout

		// cleanup unused clients
		for(w = web_clients; w ; w = w?w->next:NULL) {
			if(w->obsolete) {
				debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
				w = web_client_free(w);
			}
		}
	}

	close(listener);
	exit(2);

	return NULL;
}

// ----------------------------------------------------------------------------
// /proc/net/dev processor

int do_proc_net_dev() {
	char buffer[MAX_PROC_NET_DEV_LINE+1] = "";
	char name[MAX_PROC_NET_DEV_IFACE_NAME + 1] = "net.";
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
			&name[4],
			&rbytes, &rpackets, &rerrors, &rdrops, &rfifo, &rframe, &rcompressed, &rmulticast,
			&tbytes, &tpackets, &terrors, &tdrops, &tfifo, &tcollisions, &tcarrier, &tcompressed);
		if(r == EOF) break;
		if(r != 17) error("Cannot read /proc/net/dev line. Expected 17 params, read %d.", r);
		else {
			RRD_STATS *st = rrd_stats_find(name);

			if(!st) {
				st = rrd_stats_create(name, save_history);
				if(!st) {
					error("Cannot create RRD_STATS for interface %s.", name);
					continue;
				}

				if(!rrd_stats_dimension_add(st, "sent", sizeof(unsigned long long), 8, 1024))
					error("Cannot add RRD_STATS dimension %s.", "sent");

				if(!rrd_stats_dimension_add(st, "received", sizeof(unsigned long long), 8, 1024))
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

int do_proc_diskstats() {
	char buffer[MAX_PROC_DISKSTATS_LINE+1] = "";
	char name[MAX_PROC_DISKSTATS_DISK_NAME + 1] = "disk.";
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
			&major, &minor, &name[5],
			&reads, &reads_merged, &readsectors, &readms, &writes, &writes_merged, &writesectors, &writems, &currentios, &iosms, &wiosms
		);
		if(r == EOF) break;
		if(r != 14) error("Cannot read /proc/diskstats line. Expected 14 params, read %d.", r);
		else {
			switch(major) {
				case 1: // ram drives
					continue;

				case 7: // loops
					continue;

				case 8: // disks
					if(minor % 16) continue; // partitions
					break;

				case 9: // MDs
					break;

				case 11: // CDs
					continue;

				default:
					continue;
			}

			RRD_STATS *st = rrd_stats_find(name);

			if(!st) {
				char ssfilename[FILENAME_MAX + 1];
				int sector_size = 512;

				sprintf(ssfilename, "/sys/block/%s/queue/hw_sector_size", &name[5]);
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

				st = rrd_stats_create(name, save_history);
				if(!st) {
					error("Cannot create RRD_STATS for disk %s.", name);
					continue;
				}

				if(!rrd_stats_dimension_add(st, "writes", sizeof(unsigned long long), sector_size, 1024))
					error("Cannot add RRD_STATS dimension %s.", "writes");

				if(!rrd_stats_dimension_add(st, "reads", sizeof(unsigned long long), sector_size, 1024))
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
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: Last loop took %llu usec.", usec);
		
		do_proc_net_dev(usec);
		do_proc_diskstats(usec);
		
		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&tmp, NULL);
		usec = usecdiff(&tmp, &now);
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: This loop took %llu usec.", usec);
		
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

int main(int argc, char **argv)
{
	int i, daemon = 0;

	// parse  the arguments
	for(i = 1; i < argc ; i++) {
		if(strcmp(argv[i], "-l") == 0 && (i+1) < argc) {
			save_history = atoi(argv[i+1]);
			if(save_history < 5 || save_history > HISTORY_MAX) {
				error("Invalid save lines %d given. Defaulting to %d.", save_history, HISTORY);
				save_history = HISTORY;
			}
			else {
				debug(D_OPTIONS, "save lines set to %d.", save_history);
			}
			i++;
		}
		else if(strcmp(argv[i], "-u") == 0 && (i+1) < argc) {
			update_every = atoi(argv[i+1]);
			if(update_every < 1 || update_every > 600) {
				error("Invalid update timer %d given. Defaulting to %d.", update_every, UPDATE_EVERY_MAX);
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
				error("Invalid listen port %d given. Defaulting to %d.", listen_port, LISTEN_PORT);
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
			fprintf(stderr, "  -u UPDATE_TIMER can be from 1 to %d seconds. Default: %d.\n", UPDATE_EVERY_MAX, UPDATE_EVERY);
			fprintf(stderr, "  -p LISTEN_PORT can be from 1 to %d. Default: %d.\n", 65535, LISTEN_PORT);
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
		silent = 1;
	}

	pthread_t p_proc;
	int r_proc;

	// spawn a child to collect data
	r_proc  = pthread_create(&p_proc, NULL, proc_main, NULL);

	// the main process - the web server listener
	//sleep(1);
	socket_listen_main(NULL);

	// wait for the childs to finish
	pthread_join(p_proc,  NULL);

	printf("PROC NET DEV  thread returns: %d\n", r_proc);

	exit(0);
}
