#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <unistd.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <errno.h>
#include <pthread.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <netinet/tcp.h>
#include <malloc.h>
#include <pwd.h>
#include <ctype.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "url.h"
#include "web_buffer.h"
#include "web_server.h"
#include "global_statistics.h"
#include "rrd.h"
#include "rrd2json.h"

#include "web_client.h"

#define INITIAL_WEB_DATA_LENGTH 16384
#define WEB_REQUEST_LENGTH 16384

int web_client_timeout = DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS;
int web_enable_gzip = 1;

extern int netdata_exit;

struct web_client *web_clients = NULL;
unsigned long long web_clients_count = 0;

struct web_client *web_client_create(int listener)
{
	struct web_client *w;

	w = calloc(1, sizeof(struct web_client));
	if(!w) {
		error("Cannot allocate new web_client memory.");
		return NULL;
	}

	w->id = ++web_clients_count;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	{
		struct sockaddr *sadr;
		socklen_t addrlen;

		sadr = (struct sockaddr*) &w->clientaddr;
		addrlen = sizeof(w->clientaddr);

		w->ifd = accept(listener, sadr, &addrlen);
		if (w->ifd == -1) {
			error("%llu: Cannot accept new incoming connection.", w->id);
			free(w);
			return NULL;
		}
		w->ofd = w->ifd;

		if(getnameinfo(sadr, addrlen, w->client_ip, NI_MAXHOST, w->client_port, NI_MAXSERV, NI_NUMERICHOST | NI_NUMERICSERV) != 0) {
			error("Cannot getnameinfo() on received client connection.");
			strncpy(w->client_ip,   "UNKNOWN", NI_MAXHOST);
			strncpy(w->client_port, "UNKNOWN", NI_MAXSERV);
		}
		w->client_ip[NI_MAXHOST]   = '\0';
		w->client_port[NI_MAXSERV] = '\0';

		switch(sadr->sa_family) {

		case AF_INET:
			debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv4 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			break;

		case AF_INET6:
			if(strncmp(w->client_ip, "::ffff:", 7) == 0) {
				strcpy(w->client_ip, &w->client_ip[7]);
				debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv4 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			}
			debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv6 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			break;

		default:
			debug(D_WEB_CLIENT_ACCESS, "%llu: New UNKNOWN web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			break;
		}

		int flag = 1;
		if(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0) error("%llu: Cannot set SO_KEEPALIVE on socket.", w->id);
	}

	w->response.data = buffer_create(INITIAL_WEB_DATA_LENGTH);
	if(unlikely(!w->response.data)) {
		// no need for error log - web_buffer_create already logged the error
		close(w->ifd);
		free(w);
		return NULL;
	}

	w->response.header = buffer_create(HTTP_RESPONSE_HEADER_SIZE);
	if(unlikely(!w->response.header)) {
		// no need for error log - web_buffer_create already logged the error
		buffer_free(w->response.data);
		close(w->ifd);
		free(w);
		return NULL;
	}

	w->response.header_output = buffer_create(HTTP_RESPONSE_HEADER_SIZE);
	if(unlikely(!w->response.header_output)) {
		// no need for error log - web_buffer_create already logged the error
		buffer_free(w->response.header);
		buffer_free(w->response.data);
		close(w->ifd);
		free(w);
		return NULL;
	}

	w->wait_receive = 1;

	if(web_clients) web_clients->prev = w;
	w->next = web_clients;
	web_clients = w;

	global_statistics.connected_clients++;

	return(w);
}

void web_client_reset(struct web_client *w)
{
	struct timeval tv;
	gettimeofday(&tv, NULL);

	long sent = (w->mode == WEB_CLIENT_MODE_FILECOPY)?w->response.rlen:w->response.data->len;

#ifdef NETDATA_WITH_ZLIB
	if(likely(w->response.zoutput)) sent = (long)w->response.zstream.total_out;
#endif

	long size = (w->mode == WEB_CLIENT_MODE_FILECOPY)?w->response.rlen:w->response.data->len;

	if(likely(w->last_url[0]))
		log_access("%llu: (sent/all = %ld/%ld bytes %0.0f%%, prep/sent/total = %0.2f/%0.2f/%0.2f ms) %s: %d '%s'",
			w->id,
			sent, size, -((size>0)?((float)(size-sent)/(float)size * 100.0):0.0),
			(float)usecdiff(&w->tv_ready, &w->tv_in) / 1000.0,
			(float)usecdiff(&tv, &w->tv_ready) / 1000.0,
			(float)usecdiff(&tv, &w->tv_in) / 1000.0,
			(w->mode == WEB_CLIENT_MODE_FILECOPY)?"filecopy":((w->mode == WEB_CLIENT_MODE_OPTIONS)?"options":"data"),
			w->response.code,
			w->last_url
		);

	debug(D_WEB_CLIENT, "%llu: Reseting client.", w->id);

	if(unlikely(w->mode == WEB_CLIENT_MODE_FILECOPY)) {
		debug(D_WEB_CLIENT, "%llu: Closing filecopy input file.", w->id);
		close(w->ifd);
		w->ifd = w->ofd;
	}

	w->last_url[0] = '\0';

	w->mode = WEB_CLIENT_MODE_NORMAL;

	buffer_reset(w->response.header_output);
	buffer_reset(w->response.header);
	buffer_reset(w->response.data);
	w->response.rlen = 0;
	w->response.sent = 0;
	w->response.code = 0;

	w->wait_receive = 1;
	w->wait_send = 0;

	w->response.zoutput = 0;

	// if we had enabled compression, release it
#ifdef NETDATA_WITH_ZLIB
	if(w->response.zinitialized) {
		debug(D_DEFLATE, "%llu: Reseting compression.", w->id);
		deflateEnd(&w->response.zstream);
		w->response.zsent = 0;
		w->response.zhave = 0;
		w->response.zstream.avail_in = 0;
		w->response.zstream.avail_out = 0;
		w->response.zstream.total_in = 0;
		w->response.zstream.total_out = 0;
		w->response.zinitialized = 0;
	}
#endif // NETDATA_WITH_ZLIB
}

struct web_client *web_client_free(struct web_client *w)
{
	struct web_client *n = w->next;

	debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s port %s.", w->id, w->client_ip, w->client_port);

	if(w->prev)	w->prev->next = w->next;
	if(w->next) w->next->prev = w->prev;

	if(w == web_clients) web_clients = w->next;

	if(w->response.header_output) buffer_free(w->response.header_output);
	if(w->response.header) buffer_free(w->response.header);
	if(w->response.data) buffer_free(w->response.data);
	close(w->ifd);
	if(w->ofd != w->ifd) close(w->ofd);
	free(w);

	global_statistics.connected_clients--;

	return(n);
}

uid_t web_files_uid(void)
{
	static char *web_owner = NULL;
	static uid_t owner_uid = 0;

	if(unlikely(!web_owner)) {
		web_owner = config_get("global", "web files owner", NETDATA_USER);
		if(!web_owner || !*web_owner)
			owner_uid = geteuid();
		else {
			struct passwd *pw = getpwnam(web_owner);
			if(!pw) {
				error("User %s is not present. Ignoring option.", web_owner);
				owner_uid = geteuid();
			}
			else {
				debug(D_WEB_CLIENT, "Web files owner set to %s.\n", web_owner);
				owner_uid = pw->pw_uid;
			}
		}
	}

	return(owner_uid);
}

int mysendfile(struct web_client *w, char *filename)
{
	static char *web_dir = NULL;

	// initialize our static data
	if(unlikely(!web_dir)) web_dir = config_get("global", "web files directory", WEB_DIR);

	debug(D_WEB_CLIENT, "%llu: Looking for file '%s/%s'", w->id, web_dir, filename);

	// skip leading slashes
	while (*filename == '/') filename++;

	// if the filename contain known paths, skip them
	if(strncmp(filename, WEB_PATH_FILE "/", strlen(WEB_PATH_FILE) + 1) == 0) filename = &filename[strlen(WEB_PATH_FILE) + 1];

	char *s;
	for(s = filename; *s ;s++) {
		if( !isalnum(*s) && *s != '/' && *s != '.' && *s != '-' && *s != '_') {
			debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
			buffer_sprintf(w->response.data, "File '%s' cannot be served. Filename contains invalid character '%c'", filename, *s);
			return 400;
		}
	}

	// if the filename contains a .. refuse to serve it
	if(strstr(filename, "..") != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
		buffer_sprintf(w->response.data, "File '%s' cannot be served. Relative filenames with '..' in them are not supported.", filename);
		return 400;
	}

	// access the file
	char webfilename[FILENAME_MAX + 1];
	snprintf(webfilename, FILENAME_MAX, "%s/%s", web_dir, filename);

	// check if the file exists
	struct stat stat;
	if(lstat(webfilename, &stat) != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not found.", w->id, webfilename);
		buffer_sprintf(w->response.data, "File '%s' does not exist, or is not accessible.", filename);
		return 404;
	}

	// check if the file is owned by expected user
	if(stat.st_uid != web_files_uid()) {
		error("%llu: File '%s' is owned by user %d (expected user %d). Access Denied.", w->id, webfilename, stat.st_uid, web_files_uid());
		buffer_sprintf(w->response.data, "Access to file '%s' is not permitted.", filename);
		return 403;
	}

	if((stat.st_mode & S_IFMT) == S_IFDIR) {
		snprintf(webfilename, FILENAME_MAX+1, "%s/index.html", filename);
		return mysendfile(w, webfilename);
	}

	if((stat.st_mode & S_IFMT) != S_IFREG) {
		error("%llu: File '%s' is not a regular file. Access Denied.", w->id, webfilename);
		buffer_sprintf(w->response.data, "Access to file '%s' is not permitted.", filename);
		return 403;
	}

	// open the file
	w->ifd = open(webfilename, O_NONBLOCK, O_RDONLY);
	if(w->ifd == -1) {
		w->ifd = w->ofd;

		if(errno == EBUSY || errno == EAGAIN) {
			error("%llu: File '%s' is busy, sending 307 Moved Temporarily to force retry.", w->id, webfilename);
			buffer_sprintf(w->response.header, "Location: /" WEB_PATH_FILE "/%s\r\n", filename);
			buffer_sprintf(w->response.data, "The file '%s' is currently busy. Please try again later.", filename);
			return 307;
		}
		else {
			error("%llu: Cannot open file '%s'.", w->id, webfilename);
			buffer_sprintf(w->response.data, "Cannot open file '%s'.", filename);
			return 404;
		}
	}

	// pick a Content-Type for the file
		 if(strstr(filename, ".html") != NULL)	w->response.data->contenttype = CT_TEXT_HTML;
	else if(strstr(filename, ".js")   != NULL)	w->response.data->contenttype = CT_APPLICATION_X_JAVASCRIPT;
	else if(strstr(filename, ".css")  != NULL)	w->response.data->contenttype = CT_TEXT_CSS;
	else if(strstr(filename, ".xml")  != NULL)	w->response.data->contenttype = CT_TEXT_XML;
	else if(strstr(filename, ".xsl")  != NULL)	w->response.data->contenttype = CT_TEXT_XSL;
	else if(strstr(filename, ".txt")  != NULL)  w->response.data->contenttype = CT_TEXT_PLAIN;
	else if(strstr(filename, ".svg")  != NULL)  w->response.data->contenttype = CT_IMAGE_SVG_XML;
	else if(strstr(filename, ".ttf")  != NULL)  w->response.data->contenttype = CT_APPLICATION_X_FONT_TRUETYPE;
	else if(strstr(filename, ".otf")  != NULL)  w->response.data->contenttype = CT_APPLICATION_X_FONT_OPENTYPE;
	else if(strstr(filename, ".woff2")!= NULL)  w->response.data->contenttype = CT_APPLICATION_FONT_WOFF2;
	else if(strstr(filename, ".woff") != NULL)  w->response.data->contenttype = CT_APPLICATION_FONT_WOFF;
	else if(strstr(filename, ".eot")  != NULL)  w->response.data->contenttype = CT_APPLICATION_VND_MS_FONTOBJ;
	else if(strstr(filename, ".png")  != NULL)  w->response.data->contenttype = CT_IMAGE_PNG;
	else if(strstr(filename, ".jpg")  != NULL)  w->response.data->contenttype = CT_IMAGE_JPG;
	else if(strstr(filename, ".jpeg") != NULL)  w->response.data->contenttype = CT_IMAGE_JPG;
	else if(strstr(filename, ".gif")  != NULL)  w->response.data->contenttype = CT_IMAGE_GIF;
	else if(strstr(filename, ".bmp")  != NULL)  w->response.data->contenttype = CT_IMAGE_BMP;
	else if(strstr(filename, ".ico")  != NULL)  w->response.data->contenttype = CT_IMAGE_XICON;
	else if(strstr(filename, ".icns") != NULL)  w->response.data->contenttype = CT_IMAGE_ICNS;
	else w->response.data->contenttype = CT_APPLICATION_OCTET_STREAM;

	debug(D_WEB_CLIENT_ACCESS, "%llu: Sending file '%s' (%ld bytes, ifd %d, ofd %d).", w->id, webfilename, stat.st_size, w->ifd, w->ofd);

	w->mode = WEB_CLIENT_MODE_FILECOPY;
	w->wait_receive = 1;
	w->wait_send = 0;
	buffer_flush(w->response.data);
	w->response.rlen = stat.st_size;
	w->response.data->date = stat.st_mtim.tv_sec;

	return 200;
}


#ifdef NETDATA_WITH_ZLIB
void web_client_enable_deflate(struct web_client *w) {
	if(w->response.zinitialized == 1) {
		error("%llu: Compression has already be initialized for this client.", w->id);
		return;
	}

	if(w->response.sent) {
		error("%llu: Cannot enable compression in the middle of a conversation.", w->id);
		return;
	}

	w->response.zstream.zalloc = Z_NULL;
	w->response.zstream.zfree = Z_NULL;
	w->response.zstream.opaque = Z_NULL;

	w->response.zstream.next_in = (Bytef *)w->response.data->buffer;
	w->response.zstream.avail_in = 0;
	w->response.zstream.total_in = 0;

	w->response.zstream.next_out = w->response.zbuffer;
	w->response.zstream.avail_out = 0;
	w->response.zstream.total_out = 0;

	w->response.zstream.zalloc = Z_NULL;
	w->response.zstream.zfree = Z_NULL;
	w->response.zstream.opaque = Z_NULL;

//	if(deflateInit(&w->response.zstream, Z_DEFAULT_COMPRESSION) != Z_OK) {
//		error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
//		return;
//	}

	// Select GZIP compression: windowbits = 15 + 16 = 31
	if(deflateInit2(&w->response.zstream, Z_DEFAULT_COMPRESSION, Z_DEFLATED, 31, 8, Z_DEFAULT_STRATEGY) != Z_OK) {
		error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
		return;
	}

	w->response.zsent = 0;
	w->response.zoutput = 1;
	w->response.zinitialized = 1;

	debug(D_DEFLATE, "%llu: Initialized compression.", w->id);
}
#endif // NETDATA_WITH_ZLIB

uint32_t web_client_api_request_v1_data_options(char *o)
{
	uint32_t ret = 0x00000000;
	char *tok;

	while(o && *o && (tok = mystrsep(&o, ", |"))) {
		if(!*tok) continue;

		if(!strcmp(tok, "nonzero"))
			ret |= RRDR_OPTION_NONZERO;
		else if(!strcmp(tok, "flip") || !strcmp(tok, "reversed") || !strcmp(tok, "reverse"))
			ret |= RRDR_OPTION_REVERSED;
		else if(!strcmp(tok, "jsonwrap"))
			ret |= RRDR_OPTION_JSON_WRAP;
		else if(!strcmp(tok, "min2max"))
			ret |= RRDR_OPTION_MIN2MAX;
		else if(!strcmp(tok, "ms") || !strcmp(tok, "milliseconds"))
			ret |= RRDR_OPTION_MILLISECONDS;
		else if(!strcmp(tok, "abs") || !strcmp(tok, "absolute") || !strcmp(tok, "absolute_sum") || !strcmp(tok, "absolute-sum"))
			ret |= RRDR_OPTION_ABSOLUTE;
		else if(!strcmp(tok, "seconds"))
			ret |= RRDR_OPTION_SECONDS;
		else if(!strcmp(tok, "null2zero"))
			ret |= RRDR_OPTION_NULL2ZERO;
		else if(!strcmp(tok, "objectrows"))
			ret |= RRDR_OPTION_OBJECTSROWS;
		else if(!strcmp(tok, "google_json"))
			ret |= RRDR_OPTION_GOOGLE_JSON;
		else if(!strcmp(tok, "percentage"))
			ret |= RRDR_OPTION_PERCENTAGE;
	}

	return ret;
}

uint32_t web_client_api_request_v1_data_format(char *name)
{
	if(!strcmp(name, DATASOURCE_FORMAT_DATATABLE_JSON)) // datatable
		return DATASOURCE_DATATABLE_JSON;

	else if(!strcmp(name, DATASOURCE_FORMAT_DATATABLE_JSONP)) // datasource
		return DATASOURCE_DATATABLE_JSONP;

	else if(!strcmp(name, DATASOURCE_FORMAT_JSON)) // json
		return DATASOURCE_JSON;

	else if(!strcmp(name, DATASOURCE_FORMAT_JSONP)) // jsonp
		return DATASOURCE_JSONP;

	else if(!strcmp(name, DATASOURCE_FORMAT_SSV)) // ssv
		return DATASOURCE_SSV;

	else if(!strcmp(name, DATASOURCE_FORMAT_CSV)) // csv
		return DATASOURCE_CSV;

	else if(!strcmp(name, DATASOURCE_FORMAT_TSV) || !strcmp(name, "tsv-excel")) // tsv
		return DATASOURCE_TSV;

	else if(!strcmp(name, DATASOURCE_FORMAT_HTML)) // html
		return DATASOURCE_HTML;

	else if(!strcmp(name, DATASOURCE_FORMAT_JS_ARRAY)) // array
		return DATASOURCE_JS_ARRAY;

	else if(!strcmp(name, DATASOURCE_FORMAT_SSV_COMMA)) // ssvcomma
		return DATASOURCE_SSV_COMMA;

	else if(!strcmp(name, DATASOURCE_FORMAT_CSV_JSON_ARRAY)) // csvjsonarray
		return DATASOURCE_CSV_JSON_ARRAY;

	return DATASOURCE_JSON;
}

uint32_t web_client_api_request_v1_data_google_format(char *name)
{
	if(!strcmp(name, "json"))
		return DATASOURCE_DATATABLE_JSONP;

	else if(!strcmp(name, "html"))
		return DATASOURCE_HTML;

	else if(!strcmp(name, "csv"))
		return DATASOURCE_CSV;

	else if(!strcmp(name, "tsv-excel"))
		return DATASOURCE_TSV;

	return DATASOURCE_JSON;
}

int web_client_api_request_v1_data_group(char *name)
{
	if(!strcmp(name, "max"))
		return GROUP_MAX;

	else if(!strcmp(name, "average"))
		return GROUP_AVERAGE;

	return GROUP_MAX;
}

int web_client_api_request_v1_charts(struct web_client *w, char *url)
{
	if(url) { ; }

	buffer_flush(w->response.data);
	w->response.data->contenttype = CT_APPLICATION_JSON;
	rrd_stats_api_v1_charts(w->response.data);
	return 200;
}

int web_client_api_request_v1_chart(struct web_client *w, char *url)
{
	int ret = 400;
	char *chart = NULL;

	buffer_flush(w->response.data);

	while(url) {
		char *value = mystrsep(&url, "?&[]");
		if(!value || !*value) continue;

		char *name = mystrsep(&value, "=");
		if(!name || !*name) continue;
		if(!value || !*value) continue;

		// name and value are now the parameters
		// they are not null and not empty

		if(!strcmp(name, "chart")) chart = value;
		//else {
		///	buffer_sprintf(w->response.data, "Unknown parameter '%s' in request.", name);
		//	goto cleanup;
		//}
	}

	if(!chart || !*chart) {
		buffer_sprintf(w->response.data, "No chart id is given at the request.");
		goto cleanup;
	}

	RRDSET *st = rrdset_find(chart);
	if(!st) st = rrdset_find_byname(chart);
	if(!st) {
		buffer_sprintf(w->response.data, "Chart '%s' is not found.", chart);
		ret = 404;
		goto cleanup;
	}

	w->response.data->contenttype = CT_APPLICATION_JSON;
	rrd_stats_api_v1_chart(st, w->response.data);
	return 200;

cleanup:
	return ret;
}

// returns the HTTP code
int web_client_api_request_v1_data(struct web_client *w, char *url)
{
	debug(D_WEB_CLIENT, "%llu: API v1 data with URL '%s'", w->id, url);

	int ret = 400;
	BUFFER *dimensions = NULL;

	buffer_flush(w->response.data);

	char 	*google_version = "0.6",
			*google_reqId = "0",
			*google_sig = "0",
			*google_out = "json",
			*responseHandler = NULL,
			*outFileName = NULL;

	time_t last_timestamp_in_data = 0, google_timestamp = 0;

	char *chart = NULL
			, *before_str = NULL
			, *after_str = NULL
			, *points_str = NULL;

	int group = GROUP_MAX;
	uint32_t format = DATASOURCE_JSON;
	uint32_t options = 0x00000000;

	while(url) {
		char *value = mystrsep(&url, "?&[]");
		if(!value || !*value) continue;

		char *name = mystrsep(&value, "=");
		if(!name || !*name) continue;
		if(!value || !*value) continue;

		debug(D_WEB_CLIENT, "%llu: API v1 query param '%s' with value '%s'", w->id, name, value);

		// name and value are now the parameters
		// they are not null and not empty

		if(!strcmp(name, "chart")) chart = value;
		else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
			if(!dimensions) dimensions = buffer_create(strlen(value));
			if(dimensions) {
				buffer_strcat(dimensions, "|");
				buffer_strcat(dimensions, value);
			}
		}
		else if(!strcmp(name, "after")) after_str = value;
		else if(!strcmp(name, "before")) before_str = value;
		else if(!strcmp(name, "points")) points_str = value;
		else if(!strcmp(name, "group")) {
			group = web_client_api_request_v1_data_group(value);
		}
		else if(!strcmp(name, "format")) {
			format = web_client_api_request_v1_data_format(value);
		}
		else if(!strcmp(name, "options")) {
			options |= web_client_api_request_v1_data_options(value);
		}
		else if(!strcmp(name, "callback")) {
			responseHandler = value;
		}
		else if(!strcmp(name, "filename")) {
			outFileName = value;
		}
		else if(!strcmp(name, "tqx")) {
			// parse Google Visualization API options
			// https://developers.google.com/chart/interactive/docs/dev/implementing_data_source
			char *tqx_name, *tqx_value;

			while(value) {
				tqx_value = mystrsep(&value, ";");
				if(!tqx_value || !*tqx_value) continue;

				tqx_name = mystrsep(&tqx_value, ":");
				if(!tqx_name || !*tqx_name) continue;
				if(!tqx_value || !*tqx_value) continue;

				if(!strcmp(tqx_name, "version"))
					google_version = tqx_value;
				else if(!strcmp(tqx_name, "reqId"))
					google_reqId = tqx_value;
				else if(!strcmp(tqx_name, "sig")) {
					google_sig = tqx_value;
					google_timestamp = strtoul(google_sig, NULL, 0);
				}
				else if(!strcmp(tqx_name, "out")) {
					google_out = tqx_value;
					format = web_client_api_request_v1_data_google_format(google_out);
				}
				else if(!strcmp(tqx_name, "responseHandler"))
					responseHandler = tqx_value;
				else if(!strcmp(tqx_name, "outFileName"))
					outFileName = tqx_value;
			}
		}
	}

	if(!chart || !*chart) {
		buffer_sprintf(w->response.data, "No chart id is given at the request.");
		goto cleanup;
	}

	RRDSET *st = rrdset_find(chart);
	if(!st) st = rrdset_find_byname(chart);
	if(!st) {
		buffer_sprintf(w->response.data, "Chart '%s' is not found.", chart);
		ret = 404;
		goto cleanup;
	}

	long long before = (before_str && *before_str)?atol(before_str):0;
	long long after  = (after_str  && *after_str) ?atol(after_str):0;
	int       points = (points_str && *points_str)?atoi(points_str):0;

	debug(D_WEB_CLIENT, "%llu: API command 'data' for chart '%s', dimensions '%s', after '%lld', before '%lld', points '%d', group '%u', format '%u', options '0x%08x'"
			, w->id
			, chart
			, (dimensions)?buffer_tostring(dimensions):""
			, after
			, before
			, points
			, group
			, format
			, options
			);

	if(outFileName && *outFileName) {
		buffer_sprintf(w->response.header, "Content-Disposition: attachment; filename=\"%s\"\r\n", outFileName);
		error("generating outfilename header: '%s'", outFileName);
	}

	if(format == DATASOURCE_DATATABLE_JSONP) {
		if(responseHandler == NULL)
			responseHandler = "google.visualization.Query.setResponse";

		debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSON/JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
				w->id, google_version, google_reqId, google_sig, google_out, responseHandler, outFileName
			);

		buffer_sprintf(w->response.data,
			"%s({version:'%s',reqId:'%s',status:'ok',sig:'%lu',table:",
			responseHandler, google_version, google_reqId, st->last_updated.tv_sec);
	}
	else if(format == DATASOURCE_JSONP) {
		if(responseHandler == NULL)
			responseHandler = "callback";

		buffer_strcat(w->response.data, responseHandler);
		buffer_strcat(w->response.data, "(");
	}

	ret = rrd2format(st, w->response.data, dimensions, format, points, after, before, group, options, &last_timestamp_in_data);

	if(format == DATASOURCE_DATATABLE_JSONP) {
		if(google_timestamp < last_timestamp_in_data)
			buffer_strcat(w->response.data, "});");

		else {
			// the client already has the latest data
			buffer_flush(w->response.data);
			buffer_sprintf(w->response.data,
				"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
				responseHandler, google_version, google_reqId);
		}
	}
	else if(format == DATASOURCE_JSONP)
		buffer_strcat(w->response.data, ");");

cleanup:
	if(dimensions) buffer_free(dimensions);
	return ret;
}

int web_client_api_request_v1(struct web_client *w, char *url)
{
	// get the command
	char *tok = mystrsep(&url, "/?&");
	debug(D_WEB_CLIENT, "%llu: Searching for API v1 command '%s'.", w->id, tok);

	if(strcmp(tok, "data") == 0)
		return web_client_api_request_v1_data(w, url);
	else if(strcmp(tok, "chart") == 0)
		return web_client_api_request_v1_chart(w, url);
	else if(strcmp(tok, "charts") == 0)
		return web_client_api_request_v1_charts(w, url);

	buffer_flush(w->response.data);
	buffer_sprintf(w->response.data, "Unsupported v1 API command: %s", tok);
	return 404;
}

int web_client_api_request(struct web_client *w, char *url)
{
	// get the api version
	char *tok = mystrsep(&url, "/?&");
	debug(D_WEB_CLIENT, "%llu: Searching for API version '%s'.", w->id, tok);

	if(strcmp(tok, "v1") == 0)
		return web_client_api_request_v1(w, url);

	buffer_flush(w->response.data);
	buffer_sprintf(w->response.data, "Unsupported API version: %s", tok);
	return 404;
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
	RRDSET *st = rrdset_find_byname(tok);
	if(!st) st = rrdset_find(tok);
	if(!st) {
		// we don't have it
		// try to send a file with that name
		buffer_flush(w->response.data);
		return(mysendfile(w, tok));
	}

	// we have it
	debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

	// how many entries does the client want?
	long lines = rrd_default_history_entries;
	long group_count = 1;
	time_t after = 0, before = 0;
	int group_method = GROUP_AVERAGE;
	int nonzero = 0;

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
		else if(strcmp(tok, "sum") == 0) group_method = GROUP_SUM;
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
	if(url) {
		// parse nonzero
		tok = mystrsep(&url, "/");
		if(tok && strcmp(tok, "nonzero") == 0) nonzero = 1;
	}

	w->response.data->contenttype = CT_APPLICATION_JSON;
	buffer_flush(w->response.data);

	char *google_version = "0.6";
	char *google_reqId = "0";
	char *google_sig = "0";
	char *google_out = "json";
	char *google_responseHandler = "google.visualization.Query.setResponse";
	char *google_outFileName = NULL;
	time_t last_timestamp_in_data = 0;
	if(datasource_type == DATASOURCE_DATATABLE_JSON || datasource_type == DATASOURCE_DATATABLE_JSONP) {

		w->response.data->contenttype = CT_APPLICATION_X_JAVASCRIPT;

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

		if(datasource_type == DATASOURCE_DATATABLE_JSONP) {
			last_timestamp_in_data = strtoul(google_sig, NULL, 0);

			// check the client wants json
			if(strcmp(google_out, "json") != 0) {
				buffer_sprintf(w->response.data,
					"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'invalid_query',message:'output format is not supported',detailed_message:'the format %s requested is not supported by netdata.'}]});",
					google_responseHandler, google_version, google_reqId, google_out);
					return 200;
			}
		}
	}

	if(datasource_type == DATASOURCE_DATATABLE_JSONP) {
		buffer_sprintf(w->response.data,
			"%s({version:'%s',reqId:'%s',status:'ok',sig:'%lu',table:",
			google_responseHandler, google_version, google_reqId, st->last_updated.tv_sec);
	}

	debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (id %s, %d lines, %d group, %d group_method, %lu after, %lu before).", w->id, st->name, st->id, lines, group_count, group_method, after, before);
	time_t timestamp_in_data = rrd_stats_json(datasource_type, st, w->response.data, lines, group_count, group_method, after, before, nonzero);

	if(datasource_type == DATASOURCE_DATATABLE_JSONP) {
		if(timestamp_in_data > last_timestamp_in_data)
			buffer_strcat(w->response.data, "});");

		else {
			// the client already has the latest data
			buffer_flush(w->response.data);
			buffer_sprintf(w->response.data,
				"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
				google_responseHandler, google_version, google_reqId);
		}
	}

	return 200;
}

/*
int web_client_parse_request(struct web_client *w) {
	// protocol
	// hostname
	// path
	// query string name-value
	// http version
	// method
	// http request headers name-value

	web_client_clean_request(w);

	debug(D_WEB_DATA, "%llu: Processing data buffer of %d bytes: '%s'.", w->id, w->response.data->bytes, w->response.data->buffer);

	char *buf = w->response.data->buffer;
	char *line, *tok;

	// ------------------------------------------------------------------------
	// the first line

	if(buf && (line = strsep(&buf, "\r\n"))) {
		// method
		if(line && (tok = strsep(&line, " "))) {
			w->request.protocol = strdup(tok);
		}
		else goto cleanup;

		// url
	}
	else goto cleanup;

	// ------------------------------------------------------------------------
	// the rest of the lines

	while(buf && (line = strsep(&buf, "\r\n"))) {
		while(line && (tok = strsep(&line, ": "))) {
		}
	}

	char *url = NULL;


cleanup:
	web_client_clean_request(w);
	return 0;
}
*/

void web_client_process(struct web_client *w) {
	int code = 500;
	ssize_t bytes;
	int enable_gzip = 0;

	w->wait_receive = 0;

	// check if we have an empty line (end of HTTP header)
	if(strstr(w->response.data->buffer, "\r\n\r\n")) {
		global_statistics_lock();
		global_statistics.web_requests++;
		global_statistics_unlock();

		gettimeofday(&w->tv_in, NULL);
		debug(D_WEB_DATA, "%llu: Processing data buffer of %d bytes: '%s'.", w->id, w->response.data->len, w->response.data->buffer);

		// check if the client requested keep-alive HTTP
		if(strcasestr(w->response.data->buffer, "Connection: keep-alive")) w->keepalive = 1;
		else w->keepalive = 0;

#ifdef NETDATA_WITH_ZLIB
		// check if the client accepts deflate
		if(web_enable_gzip && strstr(w->response.data->buffer, "gzip"))
			enable_gzip = 1;
#endif // NETDATA_WITH_ZLIB

		int datasource_type = DATASOURCE_DATATABLE_JSONP;
		//if(strstr(w->response.data->buffer, "X-DataSource-Auth"))
		//	datasource_type = DATASOURCE_GOOGLE_JSON;

		char *buf = (char *)buffer_tostring(w->response.data);
		char *tok = strsep(&buf, " \r\n");
		char *url = NULL;
		char *pointer_to_free = NULL; // keep url_decode() allocated buffer

		w->mode = WEB_CLIENT_MODE_NORMAL;

		if(buf && strcmp(tok, "GET") == 0) {
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);
			debug(D_WEB_CLIENT, "%llu: Processing HTTP GET on url '%s'.", w->id, url);
		}
		else if(buf && strcmp(tok, "OPTIONS") == 0) {
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);
			debug(D_WEB_CLIENT, "%llu: Processing HTTP OPTIONS on url '%s'.", w->id, url);
			w->mode = WEB_CLIENT_MODE_OPTIONS;
		}
		else if (buf && strcmp(tok, "POST") == 0) {
			w->keepalive = 0;
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);
			debug(D_WEB_CLIENT, "%llu: I don't know how to handle POST with form data. Assuming it is a GET on url '%s'.", w->id, url);
		}

		w->last_url[0] = '\0';

		if(w->mode == WEB_CLIENT_MODE_OPTIONS) {
			strncpy(w->last_url, url, URL_MAX);
			w->last_url[URL_MAX] = '\0';

			code = 200;
			w->response.data->contenttype = CT_TEXT_PLAIN;
			buffer_flush(w->response.data);
			buffer_strcat(w->response.data, "OK");
		}
		else if(url) {
#ifdef NETDATA_WITH_ZLIB
			if(enable_gzip)
				web_client_enable_deflate(w);
#endif

			strncpy(w->last_url, url, URL_MAX);
			w->last_url[URL_MAX] = '\0';

			tok = mystrsep(&url, "/?");

			debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

			if(strcmp(tok, "api") == 0) {
				// the client is requesting api access
				datasource_type = DATASOURCE_JSON;
				code = web_client_api_request(w, url);
			}
#ifdef NETDATA_INTERNAL_CHECKS
			else if(strcmp(tok, "exit") == 0) {
				netdata_exit = 1;
				code = 200;
				w->response.data->contenttype = CT_TEXT_PLAIN;
				buffer_flush(w->response.data);
				buffer_strcat(w->response.data, "will do");
			}
#endif
			else if(strcmp(tok, WEB_PATH_DATA) == 0) { // "data"
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
				RRDSET *st = rrdset_find_byname(tok);
				if(!st) st = rrdset_find(tok);
				if(!st) {
					// we don't have it
					// try to send a file with that name
					buffer_flush(w->response.data);
					code = mysendfile(w, tok);
				}
				else {
					code = 200;
					debug(D_WEB_CLIENT_ACCESS, "%llu: Sending %s.json of RRD_STATS...", w->id, st->name);
					w->response.data->contenttype = CT_APPLICATION_JSON;
					buffer_flush(w->response.data);
					rrd_stats_graph_json(st, url, w->response.data);
				}
			}
#ifdef NETDATA_INTERNAL_CHECKS
			else if(strcmp(tok, "debug") == 0) {
				buffer_flush(w->response.data);

				// get the name of the data to show
				tok = mystrsep(&url, "/?&");
				debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

				// do we have such a data set?
				RRDSET *st = rrdset_find_byname(tok);
				if(!st) st = rrdset_find(tok);
				if(!st) {
					code = 404;
					buffer_sprintf(w->response.data, "Chart %s is not found.\r\n", tok);
					debug(D_WEB_CLIENT_ACCESS, "%llu: %s is not found.", w->id, tok);
				}
				else {
					code = 200;
					debug_flags |= D_RRD_STATS;
					st->debug = st->debug?0:1;
					buffer_sprintf(w->response.data, "Chart %s has now debug %s.\r\n", tok, st->debug?"enabled":"disabled");
					debug(D_WEB_CLIENT_ACCESS, "%llu: debug for %s is %s.", w->id, tok, st->debug?"enabled":"disabled");
				}
			}
			else if(strcmp(tok, "mirror") == 0) {
				code = 200;

				debug(D_WEB_CLIENT_ACCESS, "%llu: Mirroring...", w->id);

				// replace the zero bytes with spaces
				buffer_char_replace(w->response.data, '\0', ' ');

				// just leave the buffer as is
				// it will be copied back to the client
			}
#endif
			else if(strcmp(tok, "list") == 0) {
				code = 200;

				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending list of RRD_STATS...", w->id);

				buffer_flush(w->response.data);
				RRDSET *st = rrdset_root;

				for ( ; st ; st = st->next )
					buffer_sprintf(w->response.data, "%s\n", st->name);
			}
			else if(strcmp(tok, "all.json") == 0) {
				code = 200;
				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending JSON list of all monitors of RRD_STATS...", w->id);

				w->response.data->contenttype = CT_APPLICATION_JSON;
				buffer_flush(w->response.data);
				rrd_stats_all_json(w->response.data);
			}
			else if(strcmp(tok, "netdata.conf") == 0) {
				code = 200;
				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending netdata.conf ...", w->id);

				w->response.data->contenttype = CT_TEXT_PLAIN;
				buffer_flush(w->response.data);
				generate_config(w->response.data, 0);
			}
			else {
				char filename[FILENAME_MAX+1];
				url = filename;
				strncpy(filename, w->last_url, FILENAME_MAX);
				filename[FILENAME_MAX] = '\0';
				tok = mystrsep(&url, "?");
				buffer_flush(w->response.data);
				code = mysendfile(w, (tok && *tok)?tok:"/");
			}
		}
		else {
			strcpy(w->last_url, "not a valid response");

			if(buf) debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot understand '%s'.", w->id, buf);

			code = 500;
			buffer_flush(w->response.data);
			buffer_strcat(w->response.data, "I don't understand you...\r\n");
		}

		// free url_decode() buffer
		if(pointer_to_free) free(pointer_to_free);
	}
	else if(w->response.data->len > 8192) {
		strcpy(w->last_url, "too big request");

		debug(D_WEB_CLIENT_ACCESS, "%llu: Received request is too big.", w->id);

		code = 400;
		buffer_flush(w->response.data);
		buffer_strcat(w->response.data, "Received request is too big.\r\n");
	}
	else {
		// wait for more data
		w->wait_receive = 1;
		return;
	}

	gettimeofday(&w->tv_ready, NULL);
	w->response.data->date = time(NULL);
	w->response.sent = 0;
	w->response.code = code;

	// prepare the HTTP response header
	debug(D_WEB_CLIENT, "%llu: Generating HTTP header with response %d.", w->id, code);

	char *content_type_string;
	switch(w->response.data->contenttype) {
		case CT_TEXT_HTML:
			content_type_string = "text/html; charset=utf-8";
			break;

		case CT_APPLICATION_XML:
			content_type_string = "application/xml; charset=utf-8";
			break;

		case CT_APPLICATION_JSON:
			content_type_string = "application/json; charset=utf-8";
			break;

		case CT_APPLICATION_X_JAVASCRIPT:
			content_type_string = "application/x-javascript; charset=utf-8";
			break;

		case CT_TEXT_CSS:
			content_type_string = "text/css; charset=utf-8";
			break;

		case CT_TEXT_XML:
			content_type_string = "text/xml; charset=utf-8";
			break;

		case CT_TEXT_XSL:
			content_type_string = "text/xsl; charset=utf-8";
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

		case CT_APPLICATION_FONT_WOFF2:
			content_type_string = "application/font-woff2";
			break;

		case CT_APPLICATION_VND_MS_FONTOBJ:
			content_type_string = "application/vnd.ms-fontobject";
			break;

		case CT_IMAGE_PNG:
			content_type_string = "image/png";
			break;

		case CT_IMAGE_JPG:
			content_type_string = "image/jpeg";
			break;

		case CT_IMAGE_GIF:
			content_type_string = "image/gif";
			break;

		case CT_IMAGE_XICON:
			content_type_string = "image/x-icon";
			break;

		case CT_IMAGE_BMP:
			content_type_string = "image/bmp";
			break;

		case CT_IMAGE_ICNS:
			content_type_string = "image/icns";
			break;

		default:
		case CT_TEXT_PLAIN:
			content_type_string = "text/plain; charset=utf-8";
			break;
	}

	char *code_msg;
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
	struct tm tmbuf, *tm = gmtime_r(&w->response.data->date, &tmbuf);
	strftime(date, sizeof(date), "%a, %d %b %Y %H:%M:%S %Z", tm);

	buffer_sprintf(w->response.header_output,
		"HTTP/1.1 %d %s\r\n"
		"Connection: %s\r\n"
		"Server: NetData Embedded HTTP Server\r\n"
		"Content-Type: %s\r\n"
		"Access-Control-Allow-Origin: *\r\n"
		"Access-Control-Allow-Methods: GET, OPTIONS\r\n"
		"Access-Control-Allow-Headers: accept, x-requested-with\r\n"
		"Access-Control-Max-Age: 86400\r\n"
		"Date: %s\r\n"
		, code, code_msg
		, w->keepalive?"keep-alive":"close"
		, content_type_string
		, date
		);

	if(buffer_strlen(w->response.header))
		buffer_strcat(w->response.header_output, buffer_tostring(w->response.header));

	if(w->mode == WEB_CLIENT_MODE_NORMAL && (w->response.data->options & WB_CONTENT_NO_CACHEABLE)) {
		buffer_sprintf(w->response.header_output,
			"Expires: %s\r\n"
			"Cache-Control: no-cache\r\n"
			, date);
	}
	else {
		char edate[100];
		time_t et = w->response.data->date + (86400 * 14);
		struct tm etmbuf, *etm = gmtime_r(&et, &etmbuf);
		strftime(edate, sizeof(edate), "%a, %d %b %Y %H:%M:%S %Z", etm);

		buffer_sprintf(w->response.header_output,
			"Expires: %s\r\n"
			"Cache-Control: public\r\n"
			, edate);
	}

	// if we know the content length, put it
	if(!w->response.zoutput && (w->response.data->len || w->response.rlen))
		buffer_sprintf(w->response.header_output,
			"Content-Length: %ld\r\n"
			, w->response.data->len? w->response.data->len: w->response.rlen
			);
	else if(!w->response.zoutput)
		w->keepalive = 0;	// content-length is required for keep-alive

	if(w->response.zoutput) {
		buffer_strcat(w->response.header_output,
			"Content-Encoding: gzip\r\n"
			"Transfer-Encoding: chunked\r\n"
			);
	}

	buffer_strcat(w->response.header_output, "\r\n");

	// disable TCP_NODELAY, to buffer the header
	int flag = 0;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0)
		error("%llu: failed to disable TCP_NODELAY on socket.", w->id);

	// sent the HTTP header
	debug(D_WEB_DATA, "%llu: Sending response HTTP header of size %d: '%s'"
			, w->id
			, buffer_strlen(w->response.header_output)
			, buffer_tostring(w->response.header_output)
			);

	bytes = send(w->ofd, buffer_tostring(w->response.header_output), buffer_strlen(w->response.header_output), 0);
	if(bytes != (ssize_t) buffer_strlen(w->response.header_output))
		error("%llu: HTTP Header failed to be sent (I sent %d bytes but the system sent %d bytes)."
				, w->id
				, buffer_strlen(w->response.header_output)
				, bytes);
	else {
		global_statistics_lock();
		global_statistics.bytes_sent += bytes;
		global_statistics_unlock();
	}

	// enable TCP_NODELAY, to send all data immediately at the next send()
	flag = 1;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to enable TCP_NODELAY on socket.", w->id);

	// enable sending immediately if we have data
	if(w->response.data->len) w->wait_send = 1;
	else w->wait_send = 0;

	// pretty logging
	switch(w->mode) {
		case WEB_CLIENT_MODE_OPTIONS:
			debug(D_WEB_CLIENT, "%llu: Done preparing the OPTIONS response. Sending data (%d bytes) to client.", w->id, w->response.data->len);
			break;

		case WEB_CLIENT_MODE_NORMAL:
			debug(D_WEB_CLIENT, "%llu: Done preparing the response. Sending data (%d bytes) to client.", w->id, w->response.data->len);
			break;

		case WEB_CLIENT_MODE_FILECOPY:
			if(w->response.rlen) {
				debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending data file of %d bytes to client.", w->id, w->response.rlen);
				w->wait_receive = 1;

				/*
				// utilize the kernel sendfile() for copying the file to the socket.
				// this block of code can be commented, without anything missing.
				// when it is commented, the program will copy the data using async I/O.
				{
					long len = sendfile(w->ofd, w->ifd, NULL, w->response.data->rbytes);
					if(len != w->response.data->rbytes) error("%llu: sendfile() should copy %ld bytes, but copied %ld. Falling back to manual copy.", w->id, w->response.data->rbytes, len);
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

long web_client_send_chunk_header(struct web_client *w, long len)
{
	debug(D_DEFLATE, "%llu: OPEN CHUNK of %d bytes (hex: %x).", w->id, len, len);
	char buf[1024];
	sprintf(buf, "%lX\r\n", len);
	ssize_t bytes = send(w->ofd, buf, strlen(buf), MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk header %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk header to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk header to client.", w->id);

	return bytes;
}

long web_client_send_chunk_close(struct web_client *w)
{
	//debug(D_DEFLATE, "%llu: CLOSE CHUNK.", w->id);

	ssize_t bytes = send(w->ofd, "\r\n", 2, MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk suffix %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk suffix to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk suffix to client.", w->id);

	return bytes;
}

long web_client_send_chunk_finalize(struct web_client *w)
{
	//debug(D_DEFLATE, "%llu: FINALIZE CHUNK.", w->id);

	ssize_t bytes = send(w->ofd, "\r\n0\r\n\r\n", 7, MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk suffix %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk suffix to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk suffix to client.", w->id);

	return bytes;
}

#ifdef NETDATA_WITH_ZLIB
long web_client_send_deflate(struct web_client *w)
{
	long len = 0, t = 0;

	// when using compression,
	// w->response.sent is the amount of bytes passed through compression

	debug(D_DEFLATE, "%llu: web_client_send_deflate(): w->response.data->len = %d, w->response.sent = %d, w->response.zhave = %d, w->response.zsent = %d, w->response.zstream.avail_in = %d, w->response.zstream.avail_out = %d, w->response.zstream.total_in = %d, w->response.zstream.total_out = %d.", w->id, w->response.data->len, w->response.sent, w->response.zhave, w->response.zsent, w->response.zstream.avail_in, w->response.zstream.avail_out, w->response.zstream.total_in, w->response.zstream.total_out);

	if(w->response.data->len - w->response.sent == 0 && w->response.zstream.avail_in == 0 && w->response.zhave == w->response.zsent && w->response.zstream.avail_out != 0) {
		// there is nothing to send

		debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

		// finalize the chunk
		if(w->response.sent != 0)
			t += web_client_send_chunk_finalize(w);

		// there can be two cases for this
		// A. we have done everything
		// B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

		if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->ifd != w->ofd && w->response.rlen && w->response.rlen > w->response.data->len) {
			// we have to wait, more data will come
			debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
			w->wait_send = 0;
			return(0);
		}

		if(w->keepalive == 0) {
			debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %ld bytes sent.", w->id, w->response.sent);
			errno = 0;
			return(-1);
		}

		// reset the client
		web_client_reset(w);
		debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
		return(0);
	}

	if(w->response.zhave == w->response.zsent) {
		// compress more input data

		// close the previous open chunk
		if(w->response.sent != 0) t += web_client_send_chunk_close(w);

		debug(D_DEFLATE, "%llu: Compressing %d new bytes starting from %d (and %d left behind).", w->id, (w->response.data->len - w->response.sent), w->response.sent, w->response.zstream.avail_in);

		// give the compressor all the data not passed through the compressor yet
		if(w->response.data->len > w->response.sent) {
#ifdef NETDATA_INTERNAL_CHECKS
			if((long)w->response.sent - (long)w->response.zstream.avail_in < 0)
				error("internal error: avail_in is corrupted.");
#endif
			w->response.zstream.next_in = (Bytef *)&w->response.data->buffer[w->response.sent - w->response.zstream.avail_in];
			w->response.zstream.avail_in += (uInt) (w->response.data->len - w->response.sent);
		}

		// reset the compressor output buffer
		w->response.zstream.next_out = w->response.zbuffer;
		w->response.zstream.avail_out = ZLIB_CHUNK;

		// ask for FINISH if we have all the input
		int flush = Z_SYNC_FLUSH;
		if(w->mode == WEB_CLIENT_MODE_NORMAL
			|| (w->mode == WEB_CLIENT_MODE_FILECOPY && !w->wait_receive && w->response.data->len == w->response.rlen)) {
			flush = Z_FINISH;
			debug(D_DEFLATE, "%llu: Requesting Z_FINISH, if possible.", w->id);
		}
		else {
			debug(D_DEFLATE, "%llu: Requesting Z_SYNC_FLUSH.", w->id);
		}

		// compress
		if(deflate(&w->response.zstream, flush) == Z_STREAM_ERROR) {
			error("%llu: Compression failed. Closing down client.", w->id);
			web_client_reset(w);
			return(-1);
		}

		w->response.zhave = ZLIB_CHUNK - w->response.zstream.avail_out;
		w->response.zsent = 0;

		// keep track of the bytes passed through the compressor
		w->response.sent = w->response.data->len;

		debug(D_DEFLATE, "%llu: Compression produced %d bytes.", w->id, w->response.zhave);

		// open a new chunk
		t += web_client_send_chunk_header(w, w->response.zhave);
	}
	
	debug(D_WEB_CLIENT, "%llu: Sending %d bytes of data (+%d of chunk header).", w->id, w->response.zhave - w->response.zsent, t);

	len = send(w->ofd, &w->response.zbuffer[w->response.zsent], (size_t) (w->response.zhave - w->response.zsent), MSG_DONTWAIT);
	if(len > 0) {
		w->response.zsent += len;
		if(t > 0) len += t;
		debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, len);
	}
	else if(len == 0) debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client (zhave = %ld, zsent = %ld, need to send = %ld).", w->id, w->response.zhave, w->response.zsent, w->response.zhave - w->response.zsent);
	else debug(D_WEB_CLIENT, "%llu: Failed to send data to client. Reason: %s", w->id, strerror(errno));

	return(len);
}
#endif // NETDATA_WITH_ZLIB

long web_client_send(struct web_client *w)
{
#ifdef NETDATA_WITH_ZLIB
	if(likely(w->response.zoutput)) return web_client_send_deflate(w);
#endif // NETDATA_WITH_ZLIB

	long bytes;

	if(unlikely(w->response.data->len - w->response.sent == 0)) {
		// there is nothing to send

		debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

		// there can be two cases for this
		// A. we have done everything
		// B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

		if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->ifd != w->ofd && w->response.rlen && w->response.rlen > w->response.data->len) {
			// we have to wait, more data will come
			debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
			w->wait_send = 0;
			return(0);
		}

		if(unlikely(w->keepalive == 0)) {
			debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %ld bytes sent.", w->id, w->response.sent);
			errno = 0;
			return(-1);
		}

		web_client_reset(w);
		debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
		return(0);
	}

	bytes = send(w->ofd, &w->response.data->buffer[w->response.sent], w->response.data->len - w->response.sent, MSG_DONTWAIT);
	if(likely(bytes > 0)) {
		w->response.sent += bytes;
		debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, bytes);
	}
	else if(likely(bytes == 0)) debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
	else debug(D_WEB_CLIENT, "%llu: Failed to send data to client.", w->id);

	return(bytes);
}

long web_client_receive(struct web_client *w)
{
	// do we have any space for more data?
	buffer_need_bytes(w->response.data, WEB_REQUEST_LENGTH);

	long left = w->response.data->size - w->response.data->len;
	long bytes;

	if(unlikely(w->mode == WEB_CLIENT_MODE_FILECOPY))
		bytes = read(w->ifd, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1));
	else
		bytes = recv(w->ifd, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1), MSG_DONTWAIT);

	if(likely(bytes > 0)) {
		size_t old = w->response.data->len;
		w->response.data->len += bytes;
		w->response.data->buffer[w->response.data->len] = '\0';

		debug(D_WEB_CLIENT, "%llu: Received %d bytes.", w->id, bytes);
		debug(D_WEB_DATA, "%llu: Received data: '%s'.", w->id, &w->response.data->buffer[old]);

		if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
			w->wait_send = 1;
			if(w->response.rlen && w->response.data->len >= w->response.rlen) w->wait_receive = 0;
		}
	}
	else if(likely(bytes == 0)) {
		debug(D_WEB_CLIENT, "%llu: Out of input data.", w->id);

		// if we cannot read, it means we have an error on input.
		// if however, we are copying a file from ifd to ofd, we should not return an error.
		// in this case, the error should be generated when the file has been sent to the client.

		if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
			// we are copying data from ifd to ofd
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

void *web_client_main(void *ptr)
{
	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	struct timeval tv;
	struct web_client *w = ptr;
	int retval;
	fd_set ifds, ofds, efds;
	int fdmax = 0;

	log_access("%llu: %s port %s connected on thread task id %d", w->id, w->client_ip, w->client_port, gettid());

	for(;;) {
		FD_ZERO (&ifds);
		FD_ZERO (&ofds);
		FD_ZERO (&efds);

		FD_SET(w->ifd, &efds);

		if(w->ifd != w->ofd)
			FD_SET(w->ofd, &efds);

		if (w->wait_receive) {
			FD_SET(w->ifd, &ifds);
			if(w->ifd > fdmax) fdmax = w->ifd;
		}

		if (w->wait_send) {
			FD_SET(w->ofd, &ofds);
			if(w->ofd > fdmax) fdmax = w->ofd;
		}

		tv.tv_sec = web_client_timeout;
		tv.tv_usec = 0;

		debug(D_WEB_CLIENT, "%llu: Waiting socket async I/O for %s %s", w->id, w->wait_receive?"INPUT":"", w->wait_send?"OUTPUT":"");
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: LISTENER: select() failed.", w->id);
			continue;
		}
		else if(!retval) {
			// timeout
			debug(D_WEB_CLIENT_ACCESS, "%llu: LISTENER: timeout.", w->id);
			break;
		}

		if(FD_ISSET(w->ifd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on input socket.", w->id);
			break;
		}

		if(FD_ISSET(w->ofd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on output socket.", w->id);
			break;
		}

		if(w->wait_send && FD_ISSET(w->ofd, &ofds)) {
			long bytes;
			if((bytes = web_client_send(w)) < 0) {
				debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
				errno = 0;
				break;
			}

			global_statistics_lock();
			global_statistics.bytes_sent += bytes;
			global_statistics_unlock();
		}

		if(w->wait_receive && FD_ISSET(w->ifd, &ifds)) {
			long bytes;
			if((bytes = web_client_receive(w)) < 0) {
				debug(D_WEB_CLIENT, "%llu: Cannot receive data from client. Closing client.", w->id);
				errno = 0;
				break;
			}

			if(w->mode == WEB_CLIENT_MODE_NORMAL) {
				debug(D_WEB_CLIENT, "%llu: Attempting to process received data (%ld bytes).", w->id, bytes);
				// info("%llu: Attempting to process received data (%ld bytes).", w->id, bytes);
				web_client_process(w);
			}

			global_statistics_lock();
			global_statistics.bytes_received += bytes;
			global_statistics_unlock();
		}
	}

	log_access("%llu: %s port %s disconnected from thread task id %d", w->id, w->client_ip, w->client_port, gettid());
	debug(D_WEB_CLIENT, "%llu: done...", w->id);

	web_client_reset(w);
	w->obsolete = 1;

	return NULL;
}
