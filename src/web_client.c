#include "common.h"

#define INITIAL_WEB_DATA_LENGTH 16384
#define WEB_REQUEST_LENGTH 16384
#define TOO_BIG_REQUEST 16384

int web_client_timeout = DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS;
int web_donotrack_comply = 0;

#ifdef NETDATA_WITH_ZLIB
int web_enable_gzip = 1, web_gzip_level = 3, web_gzip_strategy = Z_DEFAULT_STRATEGY;
#endif /* NETDATA_WITH_ZLIB */

struct web_client *web_clients = NULL;
unsigned long long web_clients_count = 0;

static inline int web_client_crock_socket(struct web_client *w) {
#ifdef TCP_CORK
    if(likely(!w->tcp_cork && w->ofd != -1)) {
        w->tcp_cork = 1;
        if(unlikely(setsockopt(w->ofd, IPPROTO_TCP, TCP_CORK, (char *) &w->tcp_cork, sizeof(int)) != 0)) {
            error("%llu: failed to enable TCP_CORK on socket.", w->id);
            w->tcp_cork = 0;
            return -1;
        }
    }
#endif /* TCP_CORK */

    return 0;
}

static inline int web_client_uncrock_socket(struct web_client *w) {
#ifdef TCP_CORK
    if(likely(w->tcp_cork && w->ofd != -1)) {
        w->tcp_cork = 0;
        if(unlikely(setsockopt(w->ofd, IPPROTO_TCP, TCP_CORK, (char *) &w->tcp_cork, sizeof(int)) != 0)) {
            error("%llu: failed to disable TCP_CORK on socket.", w->id);
            w->tcp_cork = 1;
            return -1;
        }
    }
#endif /* TCP_CORK */

    return 0;
}

struct web_client *web_client_create(int listener)
{
    struct web_client *w;

    w = callocz(1, sizeof(struct web_client));
    w->id = ++web_clients_count;
    w->mode = WEB_CLIENT_MODE_NORMAL;

    {
        struct sockaddr *sadr;
        socklen_t addrlen;

        sadr = (struct sockaddr*) &w->clientaddr;
        addrlen = sizeof(w->clientaddr);

        w->ifd = accept4(listener, sadr, &addrlen, SOCK_NONBLOCK);
        if (w->ifd == -1) {
            error("%llu: Cannot accept new incoming connection.", w->id);
            freez(w);
            return NULL;
        }
        w->ofd = w->ifd;

        if(getnameinfo(sadr, addrlen, w->client_ip, NI_MAXHOST, w->client_port, NI_MAXSERV, NI_NUMERICHOST | NI_NUMERICSERV) != 0) {
            error("Cannot getnameinfo() on received client connection.");
            strncpyz(w->client_ip,   "UNKNOWN", NI_MAXHOST);
            strncpyz(w->client_port, "UNKNOWN", NI_MAXSERV);
        }
        w->client_ip[NI_MAXHOST]   = '\0';
        w->client_port[NI_MAXSERV] = '\0';

        switch(sadr->sa_family) {
        case AF_INET:
            debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv4 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
            break;

        case AF_INET6:
            if(strncmp(w->client_ip, "::ffff:", 7) == 0) {
                memmove(w->client_ip, &w->client_ip[7], strlen(&w->client_ip[7]) + 1);
                debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv4 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
            }
            else
                debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv6 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
            break;

        default:
            debug(D_WEB_CLIENT_ACCESS, "%llu: New UNKNOWN web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
            break;
        }

        int flag = 1;
        if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0)
            error("%llu: failed to enable TCP_NODELAY on socket.", w->id);

        flag = 1;
        if(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0)
            error("%llu: Cannot set SO_KEEPALIVE on socket.", w->id);
    }

    w->response.data = buffer_create(INITIAL_WEB_DATA_LENGTH);
    w->response.header = buffer_create(HTTP_RESPONSE_HEADER_SIZE);
    w->response.header_output = buffer_create(HTTP_RESPONSE_HEADER_SIZE);
    w->origin[0] = '*';
    w->wait_receive = 1;

    if(web_clients) web_clients->prev = w;
    w->next = web_clients;
    web_clients = w;

    web_client_connected();

    return(w);
}

void web_client_reset(struct web_client *w) {
    web_client_uncrock_socket(w);

    debug(D_WEB_CLIENT, "%llu: Resetting client.", w->id);

    if(likely(w->last_url[0])) {
        struct timeval tv;
        now_realtime_timeval(&tv);

        size_t size = (w->mode == WEB_CLIENT_MODE_FILECOPY)?w->response.rlen:w->response.data->len;
        size_t sent = size;
#ifdef NETDATA_WITH_ZLIB
        if(likely(w->response.zoutput)) sent = (size_t)w->response.zstream.total_out;
#endif

        // --------------------------------------------------------------------
        // global statistics

        finished_web_request_statistics(dt_usec(&tv, &w->tv_in),
                                        w->stats_received_bytes,
                                        w->stats_sent_bytes,
                                        size,
                                        sent);

        w->stats_received_bytes = 0;
        w->stats_sent_bytes = 0;


        // --------------------------------------------------------------------
        // access log

        log_access("%llu: (sent/all = %zu/%zu bytes %0.0f%%, prep/sent/total = %0.2f/%0.2f/%0.2f ms) %s: %d '%s'",
                   w->id,
                   sent, size, -((size > 0) ? ((size - sent) / (double) size * 100.0) : 0.0),
                   dt_usec(&w->tv_ready, &w->tv_in) / 1000.0,
                   dt_usec(&tv, &w->tv_ready) / 1000.0,
                   dt_usec(&tv, &w->tv_in) / 1000.0,
                   (w->mode == WEB_CLIENT_MODE_FILECOPY) ? "filecopy" : ((w->mode == WEB_CLIENT_MODE_OPTIONS)
                                                                         ? "options" : "data"),
                   w->response.code,
                   w->last_url
        );
    }

    if(unlikely(w->mode == WEB_CLIENT_MODE_FILECOPY)) {
        if(w->ifd != w->ofd) {
            debug(D_WEB_CLIENT, "%llu: Closing filecopy input file descriptor %d.", w->id, w->ifd);
            if(w->ifd != -1) close(w->ifd);
            w->ifd = w->ofd;
        }
    }

    w->last_url[0] = '\0';
    w->cookie1[0] = '\0';
    w->cookie2[0] = '\0';
    w->origin[0] = '*';
    w->origin[1] = '\0';

    w->mode = WEB_CLIENT_MODE_NORMAL;

    w->tcp_cork = 0;
    w->donottrack = 0;
    w->tracking_required = 0;
    w->keepalive = 0;
    w->decoded_url[0] = '\0';

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
        debug(D_DEFLATE, "%llu: Freeing compression resources.", w->id);
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

struct web_client *web_client_free(struct web_client *w) {
    web_client_reset(w);

    struct web_client *n = w->next;
    if(w == web_clients) web_clients = n;

    debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s port %s.", w->id, w->client_ip, w->client_port);

    if(w->prev) w->prev->next = w->next;
    if(w->next) w->next->prev = w->prev;
    if(w->response.header_output) buffer_free(w->response.header_output);
    if(w->response.header) buffer_free(w->response.header);
    if(w->response.data) buffer_free(w->response.data);
    if(w->ifd != -1) close(w->ifd);
    if(w->ofd != -1 && w->ofd != w->ifd) close(w->ofd);
    freez(w);

    web_client_disconnected();

    return(n);
}

uid_t web_files_uid(void) {
    static char *web_owner = NULL;
    static uid_t owner_uid = 0;

    if(unlikely(!web_owner)) {
        web_owner = config_get("global", "web files owner", config_get("global", "run as user", ""));
        if(!web_owner || !*web_owner)
            owner_uid = geteuid();
        else {
            // getpwnam() is not thread safe,
            // but we have called this function once
            // while single threaded
            struct passwd *pw = getpwnam(web_owner);
            if(!pw) {
                error("User '%s' is not present. Ignoring option.", web_owner);
                owner_uid = geteuid();
            }
            else {
                debug(D_WEB_CLIENT, "Web files owner set to %s.", web_owner);
                owner_uid = pw->pw_uid;
            }
        }
    }

    return(owner_uid);
}

gid_t web_files_gid(void) {
    static char *web_group = NULL;
    static gid_t owner_gid = 0;

    if(unlikely(!web_group)) {
        web_group = config_get("global", "web files group", config_get("global", "web files owner", ""));
        if(!web_group || !*web_group)
            owner_gid = getegid();
        else {
            // getgrnam() is not thread safe,
            // but we have called this function once
            // while single threaded
            struct group *gr = getgrnam(web_group);
            if(!gr) {
                error("Group '%s' is not present. Ignoring option.", web_group);
                owner_gid = getegid();
            }
            else {
                debug(D_WEB_CLIENT, "Web files group set to %s.", web_group);
                owner_gid = gr->gr_gid;
            }
        }
    }

    return(owner_gid);
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
    if(strncmp(filename, WEB_PATH_FILE "/", strlen(WEB_PATH_FILE) + 1) == 0)
        filename = &filename[strlen(WEB_PATH_FILE) + 1];

    char *s;
    for(s = filename; *s ;s++) {
        if( !isalnum(*s) && *s != '/' && *s != '.' && *s != '-' && *s != '_') {
            debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
            buffer_sprintf(w->response.data, "Filename contains invalid characters: ");
            buffer_strcat_htmlescape(w->response.data, filename);
            return 400;
        }
    }

    // if the filename contains a .. refuse to serve it
    if(strstr(filename, "..") != 0) {
        debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
        buffer_strcat(w->response.data, "Relative filenames are not supported: ");
        buffer_strcat_htmlescape(w->response.data, filename);
        return 400;
    }

    // access the file
    char webfilename[FILENAME_MAX + 1];
    snprintfz(webfilename, FILENAME_MAX, "%s/%s", web_dir, filename);

    // check if the file exists
    struct stat stat;
    if(lstat(webfilename, &stat) != 0) {
        debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not found.", w->id, webfilename);
        buffer_strcat(w->response.data, "File does not exist, or is not accessible: ");
        buffer_strcat_htmlescape(w->response.data, webfilename);
        return 404;
    }

    // check if the file is owned by expected user
    if(stat.st_uid != web_files_uid()) {
        error("%llu: File '%s' is owned by user %u (expected user %u). Access Denied.", w->id, webfilename, stat.st_uid, web_files_uid());
        buffer_strcat(w->response.data, "Access to file is not permitted: ");
        buffer_strcat_htmlescape(w->response.data, webfilename);
        return 403;
    }

    // check if the file is owned by expected group
    if(stat.st_gid != web_files_gid()) {
        error("%llu: File '%s' is owned by group %u (expected group %u). Access Denied.", w->id, webfilename, stat.st_gid, web_files_gid());
        buffer_strcat(w->response.data, "Access to file is not permitted: ");
        buffer_strcat_htmlescape(w->response.data, webfilename);
        return 403;
    }

    if((stat.st_mode & S_IFMT) == S_IFDIR) {
        snprintfz(webfilename, FILENAME_MAX, "%s/index.html", filename);
        return mysendfile(w, webfilename);
    }

    if((stat.st_mode & S_IFMT) != S_IFREG) {
        error("%llu: File '%s' is not a regular file. Access Denied.", w->id, webfilename);
        buffer_strcat(w->response.data, "Access to file is not permitted: ");
        buffer_strcat_htmlescape(w->response.data, webfilename);
        return 403;
    }

    // open the file
    w->ifd = open(webfilename, O_NONBLOCK, O_RDONLY);
    if(w->ifd == -1) {
        w->ifd = w->ofd;

        if(errno == EBUSY || errno == EAGAIN) {
            error("%llu: File '%s' is busy, sending 307 Moved Temporarily to force retry.", w->id, webfilename);
            buffer_sprintf(w->response.header, "Location: /" WEB_PATH_FILE "/%s\r\n", filename);
            buffer_strcat(w->response.data, "File is currently busy, please try again later: ");
            buffer_strcat_htmlescape(w->response.data, webfilename);
            return 307;
        }
        else {
            error("%llu: Cannot open file '%s'.", w->id, webfilename);
            buffer_strcat(w->response.data, "Cannot open file: ");
            buffer_strcat_htmlescape(w->response.data, webfilename);
            return 404;
        }
    }
    if(fcntl(w->ifd, F_SETFL, O_NONBLOCK) < 0)
        error("%llu: Cannot set O_NONBLOCK on file '%s'.", w->id, webfilename);

    // pick a Content-Type for the file
         if(strstr(filename, ".html") != NULL)  w->response.data->contenttype = CT_TEXT_HTML;
    else if(strstr(filename, ".js")   != NULL)  w->response.data->contenttype = CT_APPLICATION_X_JAVASCRIPT;
    else if(strstr(filename, ".css")  != NULL)  w->response.data->contenttype = CT_TEXT_CSS;
    else if(strstr(filename, ".xml")  != NULL)  w->response.data->contenttype = CT_TEXT_XML;
    else if(strstr(filename, ".xsl")  != NULL)  w->response.data->contenttype = CT_TEXT_XSL;
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
    buffer_cacheable(w->response.data);

    return 200;
}


#ifdef NETDATA_WITH_ZLIB
void web_client_enable_deflate(struct web_client *w, int gzip) {
    if(unlikely(w->response.zinitialized)) {
        debug(D_DEFLATE, "%llu: Compression has already be initialized for this client.", w->id);
        return;
    }

    if(unlikely(w->response.sent)) {
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

//  if(deflateInit(&w->response.zstream, Z_DEFAULT_COMPRESSION) != Z_OK) {
//      error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
//      return;
//  }

    // Select GZIP compression: windowbits = 15 + 16 = 31
    if(deflateInit2(&w->response.zstream, web_gzip_level, Z_DEFLATED, 15 + ((gzip)?16:0), 8, web_gzip_strategy) != Z_OK) {
        error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
        return;
    }

    w->response.zsent = 0;
    w->response.zoutput = 1;
    w->response.zinitialized = 1;

    debug(D_DEFLATE, "%llu: Initialized compression.", w->id);
}
#endif // NETDATA_WITH_ZLIB

void buffer_data_options2string(BUFFER *wb, uint32_t options) {
    int count = 0;

    if(options & RRDR_OPTION_NONZERO) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "nonzero");
    }

    if(options & RRDR_OPTION_REVERSED) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "flip");
    }

    if(options & RRDR_OPTION_JSON_WRAP) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "jsonwrap");
    }

    if(options & RRDR_OPTION_MIN2MAX) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "min2max");
    }

    if(options & RRDR_OPTION_MILLISECONDS) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "ms");
    }

    if(options & RRDR_OPTION_ABSOLUTE) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "absolute");
    }

    if(options & RRDR_OPTION_SECONDS) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "seconds");
    }

    if(options & RRDR_OPTION_NULL2ZERO) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "null2zero");
    }

    if(options & RRDR_OPTION_OBJECTSROWS) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "objectrows");
    }

    if(options & RRDR_OPTION_GOOGLE_JSON) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "google_json");
    }

    if(options & RRDR_OPTION_PERCENTAGE) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "percentage");
    }

    if(options & RRDR_OPTION_NOT_ALIGNED) {
        if(count++) buffer_strcat(wb, " ");
        buffer_strcat(wb, "unaligned");
    }
}

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
        else if(!strcmp(tok, "unaligned"))
            ret |= RRDR_OPTION_NOT_ALIGNED;
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

const char *group_method2string(int group) {
    switch(group) {
        case GROUP_UNDEFINED:
            return "";

        case GROUP_AVERAGE:
            return "average";

        case GROUP_MIN:
            return "min";

        case GROUP_MAX:
            return "max";

        case GROUP_SUM:
            return "sum";

        case GROUP_INCREMENTAL_SUM:
            return "incremental-sum";

        default:
            return "unknown-group-method";
    }
}

int web_client_api_request_v1_data_group(char *name, int def)
{
    if(!strcmp(name, "average"))
        return GROUP_AVERAGE;

    else if(!strcmp(name, "min"))
        return GROUP_MIN;

    else if(!strcmp(name, "max"))
        return GROUP_MAX;

    else if(!strcmp(name, "sum"))
        return GROUP_SUM;

    else if(!strcmp(name, "incremental-sum"))
        return GROUP_INCREMENTAL_SUM;

    return def;
}

int web_client_api_request_v1_alarms(struct web_client *w, char *url)
{
    int all = 0;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        if(!strcmp(value, "all")) all = 1;
        else if(!strcmp(value, "active")) all = 0;
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarms2json(&localhost, w->response.data, all);
    return 200;
}

int web_client_api_request_v1_alarm_log(struct web_client *w, char *url)
{
    uint32_t after = 0;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "after")) after = strtoul(value, NULL, 0);
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarm_log2json(&localhost, w->response.data, after);
    return 200;
}

int web_client_api_request_single_chart(struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf))
{
    int ret = 400;
    char *chart = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = mystrsep(&url, "?&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        //else {
        /// buffer_sprintf(w->response.data, "Unknown parameter '%s' in request.", name);
        //  goto cleanup;
        //}
    }

    if(!chart || !*chart) {
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    RRDSET *st = rrdset_find(chart);
    if(!st) st = rrdset_find_byname(chart);
    if(!st) {
        buffer_strcat(w->response.data, "Chart is not found: ");
        buffer_strcat_htmlescape(w->response.data, chart);
        ret = 404;
        goto cleanup;
    }

    w->response.data->contenttype = CT_APPLICATION_JSON;
    callback(st, w->response.data);
    return 200;

    cleanup:
    return ret;
}

int web_client_api_request_v1_alarm_variables(struct web_client *w, char *url)
{
    return web_client_api_request_single_chart(w, url, health_api_v1_chart_variables2json);
}

int web_client_api_request_v1_charts(struct web_client *w, char *url)
{
    (void)url;

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    rrd_stats_api_v1_charts(w->response.data);
    return 200;
}

int web_client_api_request_v1_chart(struct web_client *w, char *url)
{
    return web_client_api_request_single_chart(w, url, rrd_stats_api_v1_chart);
}

int web_client_api_request_v1_badge(struct web_client *w, char *url) {
    int ret = 400;
    buffer_flush(w->response.data);

    BUFFER *dimensions = NULL;
    
    const char *chart = NULL
            , *before_str = NULL
            , *after_str = NULL
            , *points_str = NULL
            , *multiply_str = NULL
            , *divide_str = NULL
            , *label = NULL
            , *units = NULL
            , *label_color = NULL
            , *value_color = NULL
            , *refresh_str = NULL
            , *precision_str = NULL
            , *alarm = NULL;

    int group = GROUP_AVERAGE;
    uint32_t options = 0x00000000;

    while(url) {
        char *value = mystrsep(&url, "/?&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 badge.svg query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions)
                dimensions = buffer_create(100);

            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "group")) {
            group = web_client_api_request_v1_data_group(value, GROUP_AVERAGE);
        }
        else if(!strcmp(name, "options")) {
            options |= web_client_api_request_v1_data_options(value);
        }
        else if(!strcmp(name, "label")) label = value;
        else if(!strcmp(name, "units")) units = value;
        else if(!strcmp(name, "label_color")) label_color = value;
        else if(!strcmp(name, "value_color")) value_color = value;
        else if(!strcmp(name, "multiply")) multiply_str = value;
        else if(!strcmp(name, "divide")) divide_str = value;
        else if(!strcmp(name, "refresh")) refresh_str = value;
        else if(!strcmp(name, "precision")) precision_str = value;
        else if(!strcmp(name, "alarm")) alarm = value;
    }

    if(!chart || !*chart) {
        buffer_no_cacheable(w->response.data);
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    RRDSET *st = rrdset_find(chart);
    if(!st) st = rrdset_find_byname(chart);
    if(!st) {
        buffer_no_cacheable(w->response.data);
        buffer_svg(w->response.data, "chart not found", 0, "", NULL, NULL, 1, -1);
        ret = 200;
        goto cleanup;
    }

    RRDCALC *rc = NULL;
    if(alarm) {
        rc = rrdcalc_find(st, alarm);
        if (!rc) {
            buffer_no_cacheable(w->response.data);
            buffer_svg(w->response.data, "alarm not found", 0, "", NULL, NULL, 1, -1);
            ret = 200;
            goto cleanup;
        }
    }

    long long multiply  = (multiply_str  && *multiply_str )?atol(multiply_str):1;
    long long divide    = (divide_str    && *divide_str   )?atol(divide_str):1;
    long long before    = (before_str    && *before_str   )?atol(before_str):0;
    long long after     = (after_str     && *after_str    )?atol(after_str):-st->update_every;
    int       points    = (points_str    && *points_str   )?atoi(points_str):1;
    int       precision = (precision_str && *precision_str)?atoi(precision_str):-1;

    if(!multiply) multiply = 1;
    if(!divide) divide = 1;

    int refresh = 0;
    if(refresh_str && *refresh_str) {
        if(!strcmp(refresh_str, "auto")) {
            if(rc) refresh = rc->update_every;
            else if(options & RRDR_OPTION_NOT_ALIGNED)
                refresh = st->update_every;
            else {
                refresh = (int)(before - after);
                if(refresh < 0) refresh = -refresh;
            }
        }
        else {
            refresh = atoi(refresh_str);
            if(refresh < 0) refresh = -refresh;
        }
    }

    if(!label) {
        if(alarm) {
            char *s = (char *)alarm;
            while(*s) {
                if(*s == '_') *s = ' ';
                s++;
            }
            label = alarm;
        }
        else if(dimensions) {
            const char *dim = buffer_tostring(dimensions);
            if(*dim == '|') dim++;
            label = dim;
        }
        else
            label = st->name;
    }
    if(!units) {
        if(alarm) {
            if(rc->units)
                units = rc->units;
            else
                units = "";
        }
        else if(options & RRDR_OPTION_PERCENTAGE)
            units = "%";
        else
            units = st->units;
    }

    debug(D_WEB_CLIENT, "%llu: API command 'badge.svg' for chart '%s', alarm '%s', dimensions '%s', after '%lld', before '%lld', points '%d', group '%d', options '0x%08x'"
            , w->id
            , chart
            , alarm?alarm:""
            , (dimensions)?buffer_tostring(dimensions):""
            , after
            , before
            , points
            , group
            , options
            );

    if(rc) {
        calculated_number n = rc->value;
        if(isnan(n) || isinf(n)) n = 0;

        if (refresh > 0) {
            buffer_sprintf(w->response.header, "Refresh: %d\r\n", refresh);
            w->response.data->expires = now_realtime_sec() + refresh;
        }
        else buffer_no_cacheable(w->response.data);

        if(!value_color) {
            switch(rc->status) {
                case RRDCALC_STATUS_CRITICAL:
                    value_color = "red";
                    break;

                case RRDCALC_STATUS_WARNING:
                    value_color = "orange";
                    break;

                case RRDCALC_STATUS_CLEAR:
                    value_color = "brightgreen";
                    break;

                case RRDCALC_STATUS_UNDEFINED:
                    value_color = "lightgrey";
                    break;

                case RRDCALC_STATUS_UNINITIALIZED:
                    value_color = "#000";
                    break;

                default:
                    value_color = "grey";
                    break;
            }
        }

        buffer_svg(w->response.data,
                   label,
                   rc->value * multiply / divide,
                   units,
                   label_color,
                   value_color,
                   0,
                   precision);
        ret = 200;
    }
    else {
        time_t latest_timestamp = 0;
        int value_is_null = 1;
        calculated_number n = 0;
        ret = 500;

        // if the collected value is too old, don't calculate its value
        if (rrdset_last_entry_t(st) >= (now_realtime_sec() - (st->update_every * st->gap_when_lost_iterations_above)))
            ret = rrd2value(st,
                            w->response.data,
                            &n,
                            (dimensions) ? buffer_tostring(dimensions) : NULL,
                            points,
                            after,
                            before,
                            group,
                            options,
                            NULL,
                            &latest_timestamp,
                            &value_is_null);

        // if the value cannot be calculated, show empty badge
        if (ret != 200) {
            buffer_no_cacheable(w->response.data);
            value_is_null = 1;
            n = 0;
            ret = 200;
        }
        else if (refresh > 0) {
            buffer_sprintf(w->response.header, "Refresh: %d\r\n", refresh);
            w->response.data->expires = now_realtime_sec() + refresh;
        }
        else buffer_no_cacheable(w->response.data);

        // render the badge
        buffer_svg(w->response.data,
                   label,
                   n * multiply / divide,
                   units,
                   label_color,
                   value_color,
                   value_is_null,
                   precision);
    }

cleanup:
    if(dimensions)
        buffer_free(dimensions);
    return ret;
}

// returns the HTTP code
int web_client_api_request_v1_data(struct web_client *w, char *url)
{
    debug(D_WEB_CLIENT, "%llu: API v1 data with URL '%s'", w->id, url);

    int ret = 400;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    char    *google_version = "0.6",
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

    int group = GROUP_AVERAGE;
    uint32_t format = DATASOURCE_JSON;
    uint32_t options = 0x00000000;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 data query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "group")) {
            group = web_client_api_request_v1_data_group(value, GROUP_AVERAGE);
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
        buffer_strcat(w->response.data, "Chart is not found: ");
        buffer_strcat_htmlescape(w->response.data, chart);
        ret = 404;
        goto cleanup;
    }

    long long before = (before_str && *before_str)?atol(before_str):0;
    long long after  = (after_str  && *after_str) ?atol(after_str):0;
    int       points = (points_str && *points_str)?atoi(points_str):0;

    debug(D_WEB_CLIENT, "%llu: API command 'data' for chart '%s', dimensions '%s', after '%lld', before '%lld', points '%d', group '%d', format '%u', options '0x%08x'"
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
        debug(D_WEB_CLIENT, "%llu: generating outfilename header: '%s'", w->id, outFileName);
    }

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "google.visualization.Query.setResponse";

        debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSON/JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
                w->id, google_version, google_reqId, google_sig, google_out, responseHandler, outFileName
            );

        buffer_sprintf(w->response.data,
            "%s({version:'%s',reqId:'%s',status:'ok',sig:'%ld',table:",
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


#define REGISTRY_VERIFY_COOKIES_GUID "give-me-back-this-cookie-now--please"

int web_client_api_request_v1_registry(struct web_client *w, char *url)
{
    static uint32_t hash_action = 0, hash_access = 0, hash_hello = 0, hash_delete = 0, hash_search = 0,
            hash_switch = 0, hash_machine = 0, hash_url = 0, hash_name = 0, hash_delete_url = 0, hash_for = 0,
            hash_to = 0 /*, hash_redirects = 0 */;

    if(unlikely(!hash_action)) {
        hash_action = simple_hash("action");
        hash_access = simple_hash("access");
        hash_hello = simple_hash("hello");
        hash_delete = simple_hash("delete");
        hash_search = simple_hash("search");
        hash_switch = simple_hash("switch");
        hash_machine = simple_hash("machine");
        hash_url = simple_hash("url");
        hash_name = simple_hash("name");
        hash_delete_url = simple_hash("delete_url");
        hash_for = simple_hash("for");
        hash_to = simple_hash("to");
/*
        hash_redirects = simple_hash("redirects");
*/
    }

    char person_guid[36 + 1] = "";

    debug(D_WEB_CLIENT, "%llu: API v1 registry with URL '%s'", w->id, url);

    // FIXME
    // The browser may send multiple cookies with our id
    
    char *cookie = strstr(w->response.data->buffer, NETDATA_REGISTRY_COOKIE_NAME "=");
    if(cookie)
        strncpyz(person_guid, &cookie[sizeof(NETDATA_REGISTRY_COOKIE_NAME)], 36);

    char action = '\0';
    char *machine_guid = NULL,
            *machine_url = NULL,
            *url_name = NULL,
            *search_machine_guid = NULL,
            *delete_url = NULL,
            *to_person_guid = NULL;
/*
    int redirects = 0;
*/

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 registry query param '%s' with value '%s'", w->id, name, value);

        uint32_t hash = simple_hash(name);

        if(hash == hash_action && !strcmp(name, "action")) {
            uint32_t vhash = simple_hash(value);

            if(vhash == hash_access && !strcmp(value, "access")) action = 'A';
            else if(vhash == hash_hello && !strcmp(value, "hello")) action = 'H';
            else if(vhash == hash_delete && !strcmp(value, "delete")) action = 'D';
            else if(vhash == hash_search && !strcmp(value, "search")) action = 'S';
            else if(vhash == hash_switch && !strcmp(value, "switch")) action = 'W';
#ifdef NETDATA_INTERNAL_CHECKS
            else error("unknown registry action '%s'", value);
#endif /* NETDATA_INTERNAL_CHECKS */
        }
/*
        else if(hash == hash_redirects && !strcmp(name, "redirects"))
            redirects = atoi(value);
*/
        else if(hash == hash_machine && !strcmp(name, "machine"))
            machine_guid = value;

        else if(hash == hash_url && !strcmp(name, "url"))
            machine_url = value;

        else if(action == 'A') {
            if(hash == hash_name && !strcmp(name, "name"))
                url_name = value;
        }
        else if(action == 'D') {
            if(hash == hash_delete_url && !strcmp(name, "delete_url"))
                delete_url = value;
        }
        else if(action == 'S') {
            if(hash == hash_for && !strcmp(name, "for"))
                search_machine_guid = value;
        }
        else if(action == 'W') {
            if(hash == hash_to && !strcmp(name, "to"))
                to_person_guid = value;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        else error("unused registry URL parameter '%s' with value '%s'", name, value);
#endif /* NETDATA_INTERNAL_CHECKS */
    }

    if(web_donotrack_comply && w->donottrack) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Your web browser is sending 'DNT: 1' (Do Not Track). The registry requires persistent cookies on your browser to work.");
        return 400;
    }

    if(action == 'A' && (!machine_guid || !machine_url || !url_name)) {
        error("Invalid registry request - access requires these parameters: machine ('%s'), url ('%s'), name ('%s')",
                machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", url_name?url_name:"UNSET");
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid registry Access request.");
        return 400;
    }
    else if(action == 'D' && (!machine_guid || !machine_url || !delete_url)) {
        error("Invalid registry request - delete requires these parameters: machine ('%s'), url ('%s'), delete_url ('%s')",
                machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", delete_url?delete_url:"UNSET");
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid registry Delete request.");
        return 400;
    }
    else if(action == 'S' && (!machine_guid || !machine_url || !search_machine_guid)) {
        error("Invalid registry request - search requires these parameters: machine ('%s'), url ('%s'), for ('%s')",
                machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", search_machine_guid?search_machine_guid:"UNSET");
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid registry Search request.");
        return 400;
    }
    else if(action == 'W' && (!machine_guid || !machine_url || !to_person_guid)) {
        error("Invalid registry request - switching identity requires these parameters: machine ('%s'), url ('%s'), to ('%s')",
                machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", to_person_guid?to_person_guid:"UNSET");
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid registry Switch request.");
        return 400;
    }

    switch(action) {
        case 'A':
            w->tracking_required = 1;
            if(registry_verify_cookies_redirects() > 0 && (!cookie || !person_guid[0])) {
                buffer_flush(w->response.data);
                registry_set_cookie(w, REGISTRY_VERIFY_COOKIES_GUID);
                w->response.data->contenttype = CT_APPLICATION_JSON;
                buffer_sprintf(w->response.data, "{ \"status\": \"redirect\", \"registry\": \"%s\" }", registry_to_announce());
                return 200;

/*
 * it seems that web browsers are ignoring 307 (Moved Temporarily)
 * under certain conditions, when using CORS
 * so this is commented and we use application level redirects instead
 *
                redirects++;

                if(redirects > registry_verify_cookies_redirects()) {
                    buffer_flush(w->response.data);
                    buffer_sprintf(w->response.data, "Your browser does not support cookies");
                    return 400;
                }

                char *encoded_url = url_encode(machine_url);
                if(!encoded_url) {
                    error("%llu: Cannot URL encode string '%s'", w->id, machine_url);
                    return 500;
                }

                char *encoded_name = url_encode(url_name);
                if(!encoded_name) {
                    free(encoded_url);
                    error("%llu: Cannot URL encode string '%s'", w->id, url_name);
                    return 500;
                }

                char *encoded_guid = url_encode(machine_guid);
                if(!encoded_guid) {
                    free(encoded_url);
                    free(encoded_name);
                    error("%llu: Cannot URL encode string '%s'", w->id, machine_guid);
                    return 500;
                }

                buffer_sprintf(w->response.header, "Location: %s/api/v1/registry?action=access&machine=%s&name=%s&url=%s&redirects=%d\r\n",
                               registry_to_announce(), encoded_guid, encoded_name, encoded_url, redirects);

                free(encoded_guid);
                free(encoded_name);
                free(encoded_url);
                return 307
*/
            }

            if(unlikely(cookie && person_guid[0] && !strcmp(person_guid, REGISTRY_VERIFY_COOKIES_GUID)))
                person_guid[0] = '\0';

            return registry_request_access_json(w, person_guid, machine_guid, machine_url, url_name, now_realtime_sec());

        case 'D':
            w->tracking_required = 1;
            return registry_request_delete_json(w, person_guid, machine_guid, machine_url, delete_url, now_realtime_sec());

        case 'S':
            w->tracking_required = 1;
            return registry_request_search_json(w, person_guid, machine_guid, machine_url, search_machine_guid, now_realtime_sec());

        case 'W':
            w->tracking_required = 1;
            return registry_request_switch_json(w, person_guid, machine_guid, machine_url, to_person_guid, now_realtime_sec());

        case 'H':
            return registry_request_hello_json(w);

        default:
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Invalid registry request - you need to set an action: hello, access, delete, search");
            return 400;
    }
}

int web_client_api_request_v1(struct web_client *w, char *url) {
    static uint32_t hash_data = 0, hash_chart = 0, hash_charts = 0, hash_registry = 0, hash_badge = 0, hash_alarms = 0, hash_alarm_log = 0, hash_alarm_variables = 0;

    if(unlikely(hash_data == 0)) {
        hash_data = simple_hash("data");
        hash_chart = simple_hash("chart");
        hash_charts = simple_hash("charts");
        hash_registry = simple_hash("registry");
        hash_badge = simple_hash("badge.svg");
        hash_alarms = simple_hash("alarms");
        hash_alarm_log = simple_hash("alarm_log");
        hash_alarm_variables = simple_hash("alarm_variables");
    }

    // get the command
    char *tok = mystrsep(&url, "/?&");
    if(tok && *tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for API v1 command '%s'.", w->id, tok);
        uint32_t hash = simple_hash(tok);

        if(hash == hash_data && !strcmp(tok, "data"))
            return web_client_api_request_v1_data(w, url);

        else if(hash == hash_chart && !strcmp(tok, "chart"))
            return web_client_api_request_v1_chart(w, url);

        else if(hash == hash_charts && !strcmp(tok, "charts"))
            return web_client_api_request_v1_charts(w, url);

        else if(hash == hash_registry && !strcmp(tok, "registry"))
            return web_client_api_request_v1_registry(w, url);

        else if(hash == hash_badge && !strcmp(tok, "badge.svg"))
            return web_client_api_request_v1_badge(w, url);

        else if(hash == hash_alarms && !strcmp(tok, "alarms"))
            return web_client_api_request_v1_alarms(w, url);

        else if(hash == hash_alarm_log && !strcmp(tok, "alarm_log"))
            return web_client_api_request_v1_alarm_log(w, url);

        else if(hash == hash_alarm_variables && !strcmp(tok, "alarm_variables"))
            return web_client_api_request_v1_alarm_variables(w, url);

        else {
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Unsupported v1 API command: ");
            buffer_strcat_htmlescape(w->response.data, tok);
            return 404;
        }
    }
    else {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API v1 command?");
        return 400;
    }
}

int web_client_api_request(struct web_client *w, char *url)
{
    // get the api version
    char *tok = mystrsep(&url, "/?&");
    if(tok && *tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for API version '%s'.", w->id, tok);
        if(strcmp(tok, "v1") == 0)
            return web_client_api_request_v1(w, url);
        else {
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Unsupported API version: ");
            buffer_strcat_htmlescape(w->response.data, tok);
            return 404;
        }
    }
    else {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API version?");
        return 400;
    }
}

int web_client_api_old_data_request(struct web_client *w, char *url, int datasource_type)
{
    if(!url || !*url) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Incomplete request.");
        return 400;
    }

    RRDSET *st = NULL;

    char *args = strchr(url, '?');
    if(args) {
        *args='\0';
        args = &args[1];
    }

    // get the name of the data to show
    char *tok = mystrsep(&url, "/");
    if(!tok) tok = "";

    // do we have such a data set?
    if(*tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);
        st = rrdset_find_byname(tok);
        if(!st) st = rrdset_find(tok);
    }

    if(!st) {
        // we don't have it
        // try to send a file with that name
        buffer_flush(w->response.data);
        return(mysendfile(w, tok));
    }

    // we have it
    debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

    // how many entries does the client want?
    int lines = rrd_default_history_entries;
    int group_count = 1;
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
        if(tok && *tok) group_count = atoi(tok);
        if(group_count < 1) group_count = 1;
        //if(group_count > save_history / 20) group_count = save_history / 20;
    }
    if(url) {
        // parse the grouping method required
        tok = mystrsep(&url, "/");
        if(tok && *tok) {
            if(strcmp(tok, "max") == 0) group_method = GROUP_MAX;
            else if(strcmp(tok, "average") == 0) group_method = GROUP_AVERAGE;
            else if(strcmp(tok, "sum") == 0) group_method = GROUP_SUM;
            else debug(D_WEB_CLIENT, "%llu: Unknown group method '%s'", w->id, tok);
        }
    }
    if(url) {
        // parse after time
        tok = mystrsep(&url, "/");
        if(tok && *tok) after = strtoul(tok, NULL, 10);
        if(after < 0) after = 0;
    }
    if(url) {
        // parse before time
        tok = mystrsep(&url, "/");
        if(tok && *tok) before = strtoul(tok, NULL, 10);
        if(before < 0) before = 0;
    }
    if(url) {
        // parse nonzero
        tok = mystrsep(&url, "/");
        if(tok && *tok && strcmp(tok, "nonzero") == 0) nonzero = 1;
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
            if(tok && *tok) {
                char *name = mystrsep(&tok, "=");
                if(name && *name && strcmp(name, "tqx") == 0) {
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
            "%s({version:'%s',reqId:'%s',status:'ok',sig:'%ld',table:",
            google_responseHandler, google_version, google_reqId, st->last_updated.tv_sec);
    }

    debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (id %s, %d lines, %d group, %d group_method, %ld after, %ld before).",
        w->id, st->name, st->id, lines, group_count, group_method, after, before);

    time_t timestamp_in_data = rrd_stats_json(datasource_type, st, w->response.data, lines, group_count, group_method, (unsigned long)after, (unsigned long)before, nonzero);

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

const char *web_content_type_to_string(uint8_t contenttype) {
    switch(contenttype) {
        case CT_TEXT_HTML:
            return "text/html; charset=utf-8";

        case CT_APPLICATION_XML:
            return "application/xml; charset=utf-8";

        case CT_APPLICATION_JSON:
            return "application/json; charset=utf-8";

        case CT_APPLICATION_X_JAVASCRIPT:
            return "application/x-javascript; charset=utf-8";

        case CT_TEXT_CSS:
            return "text/css; charset=utf-8";

        case CT_TEXT_XML:
            return "text/xml; charset=utf-8";

        case CT_TEXT_XSL:
            return "text/xsl; charset=utf-8";

        case CT_APPLICATION_OCTET_STREAM:
            return "application/octet-stream";

        case CT_IMAGE_SVG_XML:
            return "image/svg+xml";

        case CT_APPLICATION_X_FONT_TRUETYPE:
            return "application/x-font-truetype";

        case CT_APPLICATION_X_FONT_OPENTYPE:
            return "application/x-font-opentype";

        case CT_APPLICATION_FONT_WOFF:
            return "application/font-woff";

        case CT_APPLICATION_FONT_WOFF2:
            return "application/font-woff2";

        case CT_APPLICATION_VND_MS_FONTOBJ:
            return "application/vnd.ms-fontobject";

        case CT_IMAGE_PNG:
            return "image/png";

        case CT_IMAGE_JPG:
            return "image/jpeg";

        case CT_IMAGE_GIF:
            return "image/gif";

        case CT_IMAGE_XICON:
            return "image/x-icon";

        case CT_IMAGE_BMP:
            return "image/bmp";

        case CT_IMAGE_ICNS:
            return "image/icns";

        default:
        case CT_TEXT_PLAIN:
            return "text/plain; charset=utf-8";
    }
}


const char *web_response_code_to_string(int code) {
    switch(code) {
        case 200:
            return "OK";

        case 307:
            return "Temporary Redirect";

        case 400:
            return "Bad Request";

        case 403:
            return "Forbidden";

        case 404:
            return "Not Found";

        case 412:
            return "Preconditions Failed";

        default:
            if(code >= 100 && code < 200)
                return "Informational";

            if(code >= 200 && code < 300)
                return "Successful";

            if(code >= 300 && code < 400)
                return "Redirection";

            if(code >= 400 && code < 500)
                return "Bad Request";

            if(code >= 500 && code < 600)
                return "Server Error";

            return "Undefined Error";
    }
}

static inline char *http_header_parse(struct web_client *w, char *s) {
    static uint32_t hash_origin = 0, hash_connection = 0, hash_accept_encoding = 0, hash_donottrack = 0;

    if(unlikely(!hash_origin)) {
        hash_origin = simple_uhash("Origin");
        hash_connection = simple_uhash("Connection");
        hash_accept_encoding = simple_uhash("Accept-Encoding");
        hash_donottrack = simple_uhash("DNT");
    }

    char *e = s;

    // find the :
    while(*e && *e != ':') e++;
    if(!*e) return e;

    // get the name
    *e = '\0';

    // find the value
    char *v = e + 1, *ve;

    // skip leading spaces from value
    while(*v == ' ') v++;
    ve = v;

    // find the \r
    while(*ve && *ve != '\r') ve++;
    if(!*ve || ve[1] != '\n') {
        *e = ':';
        return ve;
    }

    // terminate the value
    *ve = '\0';

    // fprintf(stderr, "HEADER: '%s' = '%s'\n", s, v);
    uint32_t hash = simple_uhash(s);

    if(hash == hash_origin && !strcasecmp(s, "Origin"))
        strncpyz(w->origin, v, ORIGIN_MAX);

    else if(hash == hash_connection && !strcasecmp(s, "Connection")) {
        if(strcasestr(v, "keep-alive"))
            w->keepalive = 1;
    }
    else if(web_donotrack_comply && hash == hash_donottrack && !strcasecmp(s, "DNT")) {
        if(*v == '0') w->donottrack = 0;
        else if(*v == '1') w->donottrack = 1;
    }
#ifdef NETDATA_WITH_ZLIB
    else if(hash == hash_accept_encoding && !strcasecmp(s, "Accept-Encoding")) {
        if(web_enable_gzip) {
            if(strcasestr(v, "gzip"))
                web_client_enable_deflate(w, 1);
            //
            // does not seem to work
            // else if(strcasestr(v, "deflate"))
            //  web_client_enable_deflate(w, 0);
        }
    }
#endif /* NETDATA_WITH_ZLIB */

    *e = ':';
    *ve = '\r';
    return ve;
}

// http_request_validate()
// returns:
// = 0 : all good, process the request
// > 0 : request is not supported
// < 0 : request is incomplete - wait for more data

static inline int http_request_validate(struct web_client *w) {
    char *s = w->response.data->buffer, *encoded_url = NULL;

    // is is a valid request?
    if(!strncmp(s, "GET ", 4)) {
        encoded_url = s = &s[4];
        w->mode = WEB_CLIENT_MODE_NORMAL;
    }
    else if(!strncmp(s, "OPTIONS ", 8)) {
        encoded_url = s = &s[8];
        w->mode = WEB_CLIENT_MODE_OPTIONS;
    }
    else {
        w->wait_receive = 0;
        return 1;
    }

    // find the SPACE + "HTTP/"
    while(*s) {
        // find the next space
        while (*s && *s != ' ') s++;

        // is it SPACE + "HTTP/" ?
        if(*s && !strncmp(s, " HTTP/", 6)) break;
        else s++;
    }

    // incomplete requests
    if(unlikely(!*s)) {
        w->wait_receive = 1;
        return -2;
    }

    // we have the end of encoded_url - remember it
    char *ue = s;

    // make sure we have complete request
    // complete requests contain: \r\n\r\n
    while(*s) {
        // find a line feed
        while(*s && *s++ != '\r');

        // did we reach the end?
        if(unlikely(!*s)) break;

        // is it \r\n ?
        if(likely(*s++ == '\n')) {

            // is it again \r\n ? (header end)
            if(unlikely(*s == '\r' && s[1] == '\n')) {
                // a valid complete HTTP request found

                *ue = '\0';
                url_decode_r(w->decoded_url, encoded_url, URL_MAX + 1);
                *ue = ' ';
                
                // copy the URL - we are going to overwrite parts of it
                // FIXME -- we should avoid it
                strncpyz(w->last_url, w->decoded_url, URL_MAX);

                w->wait_receive = 0;
                return 0;
            }

            // another header line
            s = http_header_parse(w, s);
        }
    }

    // incomplete request
    w->wait_receive = 1;
    return -3;
}

void web_client_process(struct web_client *w) {
    static uint32_t
            hash_api = 0,
            hash_netdata_conf = 0,
            hash_data = 0,
            hash_datasource = 0,
            hash_graph = 0,
            hash_list = 0,
            hash_all_json = 0;

#ifdef NETDATA_INTERNAL_CHECKS
    static uint32_t hash_exit = 0, hash_debug = 0, hash_mirror = 0;
#endif

    // start timing us
    now_realtime_timeval(&w->tv_in);

    if(unlikely(!hash_api)) {
        hash_api = simple_hash("api");
        hash_netdata_conf = simple_hash("netdata.conf");
        hash_data = simple_hash(WEB_PATH_DATA);
        hash_datasource = simple_hash(WEB_PATH_DATASOURCE);
        hash_graph = simple_hash(WEB_PATH_GRAPH);
        hash_list = simple_hash("list");
        hash_all_json = simple_hash("all.json");
#ifdef NETDATA_INTERNAL_CHECKS
        hash_exit = simple_hash("exit");
        hash_debug = simple_hash("debug");
        hash_mirror = simple_hash("mirror");
#endif
    }

    int code = 500;
    ssize_t bytes;

    int what_to_do = http_request_validate(w);

    // wait for more data
    if(what_to_do < 0) {
        if(w->response.data->len > TOO_BIG_REQUEST) {
            strcpy(w->last_url, "too big request");

            debug(D_WEB_CLIENT_ACCESS, "%llu: Received request is too big (%zu bytes).", w->id, w->response.data->len);

            code = 400;
            buffer_flush(w->response.data);
            buffer_sprintf(w->response.data, "Received request is too big  (%zu bytes).\r\n", w->response.data->len);
        }
        else {
            // wait for more data
            return;
        }
    }
    else if(what_to_do > 0) {
        // strcpy(w->last_url, "not a valid request");

        debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot understand '%s'.", w->id, w->response.data->buffer);

        code = 500;
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "I don't understand you...\r\n");
    }
    else { // what_to_do == 0
        if(w->mode == WEB_CLIENT_MODE_OPTIONS) {
            code = 200;
            w->response.data->contenttype = CT_TEXT_PLAIN;
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "OK");
        }
        else {
            char *url = w->decoded_url;
            char *tok = mystrsep(&url, "/?");
            if(tok && *tok) {
                uint32_t hash = simple_hash(tok);
                debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

                if(hash == hash_api && strcmp(tok, "api") == 0) {
                    // the client is requesting api access
                    code = web_client_api_request(w, url);
                }
                else if(hash == hash_netdata_conf && strcmp(tok, "netdata.conf") == 0) {
                    code = 200;
                    debug(D_WEB_CLIENT_ACCESS, "%llu: Sending netdata.conf ...", w->id);

                    w->response.data->contenttype = CT_TEXT_PLAIN;
                    buffer_flush(w->response.data);
                    generate_config(w->response.data, 0);
                }
                else if(hash == hash_data && strcmp(tok, WEB_PATH_DATA) == 0) { // "data"
                    // the client is requesting rrd data -- OLD API
                    code = web_client_api_old_data_request(w, url, DATASOURCE_JSON);
                }
                else if(hash == hash_datasource && strcmp(tok, WEB_PATH_DATASOURCE) == 0) { // "datasource"
                    // the client is requesting google datasource -- OLD API
                    code = web_client_api_old_data_request(w, url, DATASOURCE_DATATABLE_JSONP);
                }
                else if(hash == hash_graph && strcmp(tok, WEB_PATH_GRAPH) == 0) { // "graph"
                    // the client is requesting an rrd graph -- OLD API

                    // get the name of the data to show
                    tok = mystrsep(&url, "/?&");
                    if(tok && *tok) {
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
                    else {
                        code = 400;
                        buffer_flush(w->response.data);
                        buffer_strcat(w->response.data, "Graph name?\r\n");
                    }
                }
                else if(hash == hash_list && strcmp(tok, "list") == 0) {
                    // OLD API
                    code = 200;

                    debug(D_WEB_CLIENT_ACCESS, "%llu: Sending list of RRD_STATS...", w->id);

                    buffer_flush(w->response.data);
                    RRDSET *st = localhost.rrdset_root;

                    for ( ; st ; st = st->next )
                        buffer_sprintf(w->response.data, "%s\n", st->name);
                }
                else if(hash == hash_all_json && strcmp(tok, "all.json") == 0) {
                    // OLD API
                    code = 200;
                    debug(D_WEB_CLIENT_ACCESS, "%llu: Sending JSON list of all monitors of RRD_STATS...", w->id);

                    w->response.data->contenttype = CT_APPLICATION_JSON;
                    buffer_flush(w->response.data);
                    rrd_stats_all_json(w->response.data);
                }
#ifdef NETDATA_INTERNAL_CHECKS
                else if(hash == hash_exit && strcmp(tok, "exit") == 0) {
                    code = 200;
                    w->response.data->contenttype = CT_TEXT_PLAIN;
                    buffer_flush(w->response.data);

                    if(!netdata_exit)
                        buffer_strcat(w->response.data, "ok, will do...");
                    else
                        buffer_strcat(w->response.data, "I am doing it already");

                    error("web request to exit received.");
                    netdata_cleanup_and_exit(0);
                    netdata_exit = 1;
                }
                else if(hash == hash_debug && strcmp(tok, "debug") == 0) {
                    buffer_flush(w->response.data);

                    // get the name of the data to show
                    tok = mystrsep(&url, "/?&");
                    if(tok && *tok) {
                        debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

                        // do we have such a data set?
                        RRDSET *st = rrdset_find_byname(tok);
                        if(!st) st = rrdset_find(tok);
                        if(!st) {
                            code = 404;
                            buffer_strcat(w->response.data, "Chart is not found: ");
                            buffer_strcat_htmlescape(w->response.data, tok);
                            debug(D_WEB_CLIENT_ACCESS, "%llu: %s is not found.", w->id, tok);
                        }
                        else {
                            code = 200;
                            debug_flags |= D_RRD_STATS;
                            st->debug = !st->debug;
                            buffer_sprintf(w->response.data, "Chart has now debug %s: ", st->debug?"enabled":"disabled");
                            buffer_strcat_htmlescape(w->response.data, tok);
                            debug(D_WEB_CLIENT_ACCESS, "%llu: debug for %s is %s.", w->id, tok, st->debug?"enabled":"disabled");
                        }
                    }
                    else {
                        code = 500;
                        buffer_flush(w->response.data);
                        buffer_strcat(w->response.data, "debug which chart?\r\n");
                    }
                }
                else if(hash == hash_mirror && strcmp(tok, "mirror") == 0) {
                    code = 200;

                    debug(D_WEB_CLIENT_ACCESS, "%llu: Mirroring...", w->id);

                    // replace the zero bytes with spaces
                    buffer_char_replace(w->response.data, '\0', ' ');

                    // just leave the buffer as is
                    // it will be copied back to the client
                }
#endif  /* NETDATA_INTERNAL_CHECKS */
                else {
                    char filename[FILENAME_MAX+1];
                    url = filename;
                    strncpyz(filename, w->last_url, FILENAME_MAX);
                    tok = mystrsep(&url, "?");
                    buffer_flush(w->response.data);
                    code = mysendfile(w, (tok && *tok)?tok:"/");
                }
            }
            else {
                char filename[FILENAME_MAX+1];
                url = filename;
                strncpyz(filename, w->last_url, FILENAME_MAX);
                tok = mystrsep(&url, "?");
                buffer_flush(w->response.data);
                code = mysendfile(w, (tok && *tok)?tok:"/");
            }
        }
    }

    now_realtime_timeval(&w->tv_ready);
    w->response.sent = 0;
    w->response.code = code;

    // set a proper last modified date
    if(unlikely(!w->response.data->date))
        w->response.data->date = w->tv_ready.tv_sec;

    if(unlikely(code != 200))
        buffer_no_cacheable(w->response.data);

    // set a proper expiration date, if not already set
    if(unlikely(!w->response.data->expires)) {
        if(w->response.data->options & WB_CONTENT_NO_CACHEABLE)
            w->response.data->expires = w->tv_ready.tv_sec + rrd_update_every;
        else
            w->response.data->expires = w->tv_ready.tv_sec + 86400;
    }

    // prepare the HTTP response header
    debug(D_WEB_CLIENT, "%llu: Generating HTTP header with response %d.", w->id, code);

    const char *content_type_string = web_content_type_to_string(w->response.data->contenttype);
    const char *code_msg = web_response_code_to_string(code);

    // prepare the last modified and expiration dates
    char date[32], edate[32];
    {
        struct tm tmbuf, *tm;

        tm = gmtime_r(&w->response.data->date, &tmbuf);
        strftime(date, sizeof(date), "%a, %d %b %Y %H:%M:%S %Z", tm);

        tm = gmtime_r(&w->response.data->expires, &tmbuf);
        strftime(edate, sizeof(edate), "%a, %d %b %Y %H:%M:%S %Z", tm);
    }

    buffer_sprintf(w->response.header_output,
        "HTTP/1.1 %d %s\r\n"
        "Connection: %s\r\n"
        "Server: NetData Embedded HTTP Server\r\n"
        "Access-Control-Allow-Origin: %s\r\n"
        "Access-Control-Allow-Credentials: true\r\n"
        "Content-Type: %s\r\n"
        "Date: %s\r\n"
        , code, code_msg
        , w->keepalive?"keep-alive":"close"
        , w->origin
        , content_type_string
        , date
        );

    if(w->cookie1[0] || w->cookie2[0]) {
        if(w->cookie1[0]) {
            buffer_sprintf(w->response.header_output,
               "Set-Cookie: %s\r\n",
               w->cookie1);
        }

        if(w->cookie2[0]) {
            buffer_sprintf(w->response.header_output,
               "Set-Cookie: %s\r\n",
               w->cookie2);
        }

        if(web_donotrack_comply)
            buffer_sprintf(w->response.header_output,
               "Tk: T;cookies\r\n");
    }
    else {
        if(web_donotrack_comply) {
            if(w->tracking_required)
                buffer_sprintf(w->response.header_output,
                   "Tk: T;cookies\r\n");
            else
                buffer_sprintf(w->response.header_output,
                   "Tk: N\r\n");
        }
    }

    if(w->mode == WEB_CLIENT_MODE_OPTIONS) {
        buffer_strcat(w->response.header_output,
            "Access-Control-Allow-Methods: GET, OPTIONS\r\n"
            "Access-Control-Allow-Headers: accept, x-requested-with, origin, content-type, cookie, pragma, cache-control\r\n"
            "Access-Control-Max-Age: 1209600\r\n" // 86400 * 14
            );
    }
    else {
        buffer_sprintf(w->response.header_output,
            "Cache-Control: %s\r\n"
            "Expires: %s\r\n",
            (w->response.data->options & WB_CONTENT_NO_CACHEABLE)?"no-cache":"public",
            edate);
    }

    // copy a possibly available custom header
    if(unlikely(buffer_strlen(w->response.header)))
        buffer_strcat(w->response.header_output, buffer_tostring(w->response.header));

    // headers related to the transfer method
    if(likely(w->response.zoutput)) {
        buffer_strcat(w->response.header_output,
            "Content-Encoding: gzip\r\n"
            "Transfer-Encoding: chunked\r\n"
            );
    }
    else {
        if(likely((w->response.data->len || w->response.rlen))) {
            // we know the content length, put it
            buffer_sprintf(w->response.header_output, "Content-Length: %zu\r\n", w->response.data->len? w->response.data->len: w->response.rlen);
        }
        else {
            // we don't know the content length, disable keep-alive
            w->keepalive = 0;
        }
    }

    // end of HTTP header
    buffer_strcat(w->response.header_output, "\r\n");

    // sent the HTTP header
    debug(D_WEB_DATA, "%llu: Sending response HTTP header of size %zu: '%s'"
            , w->id
            , buffer_strlen(w->response.header_output)
            , buffer_tostring(w->response.header_output)
            );

    web_client_crock_socket(w);

    bytes = send(w->ofd, buffer_tostring(w->response.header_output), buffer_strlen(w->response.header_output), 0);
    if(bytes != (ssize_t) buffer_strlen(w->response.header_output)) {
        if(bytes > 0)
            w->stats_sent_bytes += bytes;

        debug(D_WEB_CLIENT, "%llu: HTTP Header failed to be sent (I sent %zu bytes but the system sent %zd bytes). Closing web client."
            , w->id
            , buffer_strlen(w->response.header_output)
            , bytes);

        WEB_CLIENT_IS_DEAD(w);
        return;
    }
    else 
        w->stats_sent_bytes += bytes;

    // enable sending immediately if we have data
    if(w->response.data->len) w->wait_send = 1;
    else w->wait_send = 0;

    // pretty logging
    switch(w->mode) {
        case WEB_CLIENT_MODE_OPTIONS:
            debug(D_WEB_CLIENT, "%llu: Done preparing the OPTIONS response. Sending data (%zu bytes) to client.", w->id, w->response.data->len);
            break;

        case WEB_CLIENT_MODE_NORMAL:
            debug(D_WEB_CLIENT, "%llu: Done preparing the response. Sending data (%zu bytes) to client.", w->id, w->response.data->len);
            break;

        case WEB_CLIENT_MODE_FILECOPY:
            if(w->response.rlen) {
                debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending data file of %zu bytes to client.", w->id, w->response.rlen);
                w->wait_receive = 1;

                /*
                // utilize the kernel sendfile() for copying the file to the socket.
                // this block of code can be commented, without anything missing.
                // when it is commented, the program will copy the data using async I/O.
                {
                    long len = sendfile(w->ofd, w->ifd, NULL, w->response.data->rbytes);
                    if(len != w->response.data->rbytes)
                        error("%llu: sendfile() should copy %ld bytes, but copied %ld. Falling back to manual copy.", w->id, w->response.data->rbytes, len);
                    else
                        web_client_reset(w);
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

ssize_t web_client_send_chunk_header(struct web_client *w, size_t len)
{
    debug(D_DEFLATE, "%llu: OPEN CHUNK of %zu bytes (hex: %zx).", w->id, len, len);
    char buf[24];
    sprintf(buf, "%zX\r\n", len);
    
    ssize_t bytes = send(w->ofd, buf, strlen(buf), 0);
    if(bytes > 0) {
        debug(D_DEFLATE, "%llu: Sent chunk header %zd bytes.", w->id, bytes);
        w->stats_sent_bytes += bytes;
    }

    else if(bytes == 0) {
        debug(D_WEB_CLIENT, "%llu: Did not send chunk header to the client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }
    else {
        debug(D_WEB_CLIENT, "%llu: Failed to send chunk header to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return bytes;
}

ssize_t web_client_send_chunk_close(struct web_client *w)
{
    //debug(D_DEFLATE, "%llu: CLOSE CHUNK.", w->id);

    ssize_t bytes = send(w->ofd, "\r\n", 2, 0);
    if(bytes > 0) {
        debug(D_DEFLATE, "%llu: Sent chunk suffix %zd bytes.", w->id, bytes);
        w->stats_sent_bytes += bytes;
    }

    else if(bytes == 0) {
        debug(D_WEB_CLIENT, "%llu: Did not send chunk suffix to the client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }
    else {
        debug(D_WEB_CLIENT, "%llu: Failed to send chunk suffix to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return bytes;
}

ssize_t web_client_send_chunk_finalize(struct web_client *w)
{
    //debug(D_DEFLATE, "%llu: FINALIZE CHUNK.", w->id);

    ssize_t bytes = send(w->ofd, "\r\n0\r\n\r\n", 7, 0);
    if(bytes > 0) {
        debug(D_DEFLATE, "%llu: Sent chunk suffix %zd bytes.", w->id, bytes);
        w->stats_sent_bytes += bytes;
    }

    else if(bytes == 0) {
        debug(D_WEB_CLIENT, "%llu: Did not send chunk finalize suffix to the client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }
    else {
        debug(D_WEB_CLIENT, "%llu: Failed to send chunk finalize suffix to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return bytes;
}

#ifdef NETDATA_WITH_ZLIB
ssize_t web_client_send_deflate(struct web_client *w)
{
    ssize_t len = 0, t = 0;

    // when using compression,
    // w->response.sent is the amount of bytes passed through compression

    debug(D_DEFLATE, "%llu: web_client_send_deflate(): w->response.data->len = %zu, w->response.sent = %zu, w->response.zhave = %zu, w->response.zsent = %zu, w->response.zstream.avail_in = %u, w->response.zstream.avail_out = %u, w->response.zstream.total_in = %lu, w->response.zstream.total_out = %lu.",
        w->id, w->response.data->len, w->response.sent, w->response.zhave, w->response.zsent, w->response.zstream.avail_in, w->response.zstream.avail_out, w->response.zstream.total_in, w->response.zstream.total_out);

    if(w->response.data->len - w->response.sent == 0 && w->response.zstream.avail_in == 0 && w->response.zhave == w->response.zsent && w->response.zstream.avail_out != 0) {
        // there is nothing to send

        debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

        // finalize the chunk
        if(w->response.sent != 0) {
            t = web_client_send_chunk_finalize(w);
            if(t < 0) return t;
        }

        if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->response.rlen && w->response.rlen > w->response.data->len) {
            // we have to wait, more data will come
            debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
            w->wait_send = 0;
            return t;
        }

        if(unlikely(!w->keepalive)) {
            debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %zu bytes sent.", w->id, w->response.sent);
            WEB_CLIENT_IS_DEAD(w);
            return t;
        }

        // reset the client
        web_client_reset(w);
        debug(D_WEB_CLIENT, "%llu: Done sending all data on socket.", w->id);
        return t;
    }

    if(w->response.zhave == w->response.zsent) {
        // compress more input data

        // close the previous open chunk
        if(w->response.sent != 0) {
            t = web_client_send_chunk_close(w);
            if(t < 0) return t;
        }

        debug(D_DEFLATE, "%llu: Compressing %zu new bytes starting from %zu (and %u left behind).", w->id, (w->response.data->len - w->response.sent), w->response.sent, w->response.zstream.avail_in);

        // give the compressor all the data not passed through the compressor yet
        if(w->response.data->len > w->response.sent) {
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

        debug(D_DEFLATE, "%llu: Compression produced %zu bytes.", w->id, w->response.zhave);

        // open a new chunk
        ssize_t t2 = web_client_send_chunk_header(w, w->response.zhave);
        if(t2 < 0) return t2;
        t += t2;
    }
    
    debug(D_WEB_CLIENT, "%llu: Sending %zu bytes of data (+%zd of chunk header).", w->id, w->response.zhave - w->response.zsent, t);

    len = send(w->ofd, &w->response.zbuffer[w->response.zsent], (size_t) (w->response.zhave - w->response.zsent), MSG_DONTWAIT);
    if(len > 0) {
        w->stats_sent_bytes += len;
        w->response.zsent += len;
        len += t;
        debug(D_WEB_CLIENT, "%llu: Sent %zd bytes.", w->id, len);
    }
    else if(len == 0) {
        debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client (zhave = %zu, zsent = %zu, need to send = %zu).",
            w->id, w->response.zhave, w->response.zsent, w->response.zhave - w->response.zsent);

        WEB_CLIENT_IS_DEAD(w);
    }
    else {
        debug(D_WEB_CLIENT, "%llu: Failed to send data to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return(len);
}
#endif // NETDATA_WITH_ZLIB

ssize_t web_client_send(struct web_client *w) {
#ifdef NETDATA_WITH_ZLIB
    if(likely(w->response.zoutput)) return web_client_send_deflate(w);
#endif // NETDATA_WITH_ZLIB

    ssize_t bytes;

    if(unlikely(w->response.data->len - w->response.sent == 0)) {
        // there is nothing to send

        debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

        // there can be two cases for this
        // A. we have done everything
        // B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

        if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->response.rlen && w->response.rlen > w->response.data->len) {
            // we have to wait, more data will come
            debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
            w->wait_send = 0;
            return 0;
        }

        if(unlikely(!w->keepalive)) {
            debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %zu bytes sent.", w->id, w->response.sent);
            WEB_CLIENT_IS_DEAD(w);
            return 0;
        }

        web_client_reset(w);
        debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
        return 0;
    }

    bytes = send(w->ofd, &w->response.data->buffer[w->response.sent], w->response.data->len - w->response.sent, MSG_DONTWAIT);
    if(likely(bytes > 0)) {
        w->stats_sent_bytes += bytes;
        w->response.sent += bytes;
        debug(D_WEB_CLIENT, "%llu: Sent %zd bytes.", w->id, bytes);
    }
    else if(likely(bytes == 0)) {
        debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }
    else {
        debug(D_WEB_CLIENT, "%llu: Failed to send data to client.", w->id);
        WEB_CLIENT_IS_DEAD(w);
    }

    return(bytes);
}

ssize_t web_client_receive(struct web_client *w)
{
    // do we have any space for more data?
    buffer_need_bytes(w->response.data, WEB_REQUEST_LENGTH);

    ssize_t left = w->response.data->size - w->response.data->len;
    ssize_t bytes;

    if(unlikely(w->mode == WEB_CLIENT_MODE_FILECOPY))
        bytes = read(w->ifd, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1));
    else
        bytes = recv(w->ifd, &w->response.data->buffer[w->response.data->len], (size_t) (left - 1), MSG_DONTWAIT);

    if(likely(bytes > 0)) {
        if(w->mode != WEB_CLIENT_MODE_FILECOPY)
            w->stats_received_bytes += bytes;

        size_t old = w->response.data->len;
        w->response.data->len += bytes;
        w->response.data->buffer[w->response.data->len] = '\0';

        debug(D_WEB_CLIENT, "%llu: Received %zd bytes.", w->id, bytes);
        debug(D_WEB_DATA, "%llu: Received data: '%s'.", w->id, &w->response.data->buffer[old]);

        if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
            w->wait_send = 1;

            if(w->response.rlen && w->response.data->len >= w->response.rlen)
                w->wait_receive = 0;
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

            debug(D_WEB_CLIENT, "%llu: Read the whole file.", w->id);
            if(w->ifd != w->ofd) close(w->ifd);
            w->ifd = w->ofd;
        }
        else {
            debug(D_WEB_CLIENT, "%llu: failed to receive data.", w->id);
            WEB_CLIENT_IS_DEAD(w);
        }
    }
    else {
        debug(D_WEB_CLIENT, "%llu: receive data failed.", w->id);
        WEB_CLIENT_IS_DEAD(w);
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

    struct web_client *w = ptr;
    struct pollfd fds[2], *ifd, *ofd;
    int retval, fdmax = 0, timeout;

    log_access("%llu: %s port %s connected on thread task id %d", w->id, w->client_ip, w->client_port, gettid());

    for(;;) {
        if(unlikely(w->dead)) {
            debug(D_WEB_CLIENT, "%llu: client is dead.", w->id);
            break;
        }
        else if(unlikely(!w->wait_receive && !w->wait_send)) {
            debug(D_WEB_CLIENT, "%llu: client is not set for neither receiving nor sending data.", w->id);
            break;
        }

        if(unlikely(w->ifd < 0 || w->ofd < 0)) {
            error("%llu: invalid file descriptor, ifd = %d, ofd = %d (required 0 <= fd", w->id, w->ifd, w->ofd);
            break;
        }

        if(w->ifd == w->ofd) {
            fds[0].fd = w->ifd;
            fds[0].events = 0;
            fds[0].revents = 0;

            if(w->wait_receive) fds[0].events |= POLLIN;
            if(w->wait_send)    fds[0].events |= POLLOUT;

            fds[1].fd = -1;
            fds[1].events = 0;
            fds[1].revents = 0;

            ifd = ofd = &fds[0];

            fdmax = 1;
        }
        else {
            fds[0].fd = w->ifd;
            fds[0].events = 0;
            fds[0].revents = 0;
            if(w->wait_receive) fds[0].events |= POLLIN;
            ifd = &fds[0];

            fds[1].fd = w->ofd;
            fds[1].events = 0;
            fds[1].revents = 0;
            if(w->wait_send)    fds[1].events |= POLLOUT;
            ofd = &fds[1];

            fdmax = 2;
        }

        debug(D_WEB_CLIENT, "%llu: Waiting socket async I/O for %s %s", w->id, w->wait_receive?"INPUT":"", w->wait_send?"OUTPUT":"");
        errno = 0;
        timeout = web_client_timeout * 1000;
        retval = poll(fds, fdmax, timeout);

        if(unlikely(retval == -1)) {
            if(errno == EAGAIN || errno == EINTR) {
                debug(D_WEB_CLIENT, "%llu: EAGAIN received.", w->id);
                continue;
            }

            debug(D_WEB_CLIENT, "%llu: LISTENER: poll() failed (input fd = %d, output fd = %d). Closing client.", w->id, w->ifd, w->ofd);
            break;
        }
        else if(unlikely(!retval)) {
            debug(D_WEB_CLIENT, "%llu: Timeout while waiting socket async I/O for %s %s", w->id, w->wait_receive?"INPUT":"", w->wait_send?"OUTPUT":"");
            break;
        }

        int used = 0;
        if(w->wait_send && ofd->revents & POLLOUT) {
            used++;
            if(web_client_send(w) < 0) {
                debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
                break;
            }
        }

        if(w->wait_receive && (ifd->revents & POLLIN || ifd->revents & POLLPRI)) {
            used++;
            if(web_client_receive(w) < 0) {
                debug(D_WEB_CLIENT, "%llu: Cannot receive data from client. Closing client.", w->id);
                break;
            }

            if(w->mode == WEB_CLIENT_MODE_NORMAL) {
                debug(D_WEB_CLIENT, "%llu: Attempting to process received data.", w->id);
                web_client_process(w);
            }
        }

        if(unlikely(!used)) {
            debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on socket.", w->id);
            break;
        }
    }

    web_client_reset(w);

    log_access("%llu: %s port %s disconnected from thread task id %d", w->id, w->client_ip, w->client_port, gettid());
    debug(D_WEB_CLIENT, "%llu: done...", w->id);

    // close the sockets/files now
    // to free file descriptors
    if(w->ifd == w->ofd) {
        if(w->ifd != -1) close(w->ifd);
    }
    else {
        if(w->ifd != -1) close(w->ifd);
        if(w->ofd != -1) close(w->ofd);
    }
    w->ifd = -1;
    w->ofd = -1;

    w->obsolete = 1;

    pthread_exit(NULL);
    return NULL;
}
