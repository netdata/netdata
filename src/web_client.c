#include "common.h"

#define INITIAL_WEB_DATA_LENGTH 16384
#define WEB_REQUEST_LENGTH 16384
#define TOO_BIG_REQUEST 16384

int web_client_timeout = DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS;
int respect_web_browser_do_not_track_policy = 0;
char *web_x_frame_options = NULL;

SIMPLE_PATTERN *web_client_access_list = NULL;

#ifdef NETDATA_WITH_ZLIB
int web_enable_gzip = 1, web_gzip_level = 3, web_gzip_strategy = Z_DEFAULT_STRATEGY;
#endif /* NETDATA_WITH_ZLIB */

struct web_client *web_clients = NULL;
unsigned long long web_clients_count = 0;

static inline int web_client_crock_socket(struct web_client *w) {
#ifdef TCP_CORK
    if(likely(web_client_is_corkable(w) && !w->tcp_cork && w->ofd != -1)) {
        w->tcp_cork = 1;
        if(unlikely(setsockopt(w->ofd, IPPROTO_TCP, TCP_CORK, (char *) &w->tcp_cork, sizeof(int)) != 0)) {
            error("%llu: failed to enable TCP_CORK on socket.", w->id);

            w->tcp_cork = 0;
            return -1;
        }
    }
#else
    (void)w;
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
#else
    (void)w;
#endif /* TCP_CORK */

    return 0;
}

struct web_client *web_client_create(int listener) {
    struct web_client *w;

    w = callocz(1, sizeof(struct web_client));
    w->id = ++web_clients_count;
    w->mode = WEB_CLIENT_MODE_NORMAL;

    {
        w->ifd = accept_socket(listener, SOCK_NONBLOCK, w->client_ip, sizeof(w->client_ip), w->client_port, sizeof(w->client_port), web_client_access_list);
        if (w->ifd == -1) {

            if(errno != EPERM)
                error("%llu: Failed to accept new incoming connection.", w->id);

            freez(w);
            return NULL;
        }
        w->ofd = w->ifd;

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
    web_client_enable_wait_receive(w);

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
    web_client_disable_donottrack(w);
    web_client_disable_tracking_required(w);
    web_client_disable_keepalive(w);
    w->decoded_url[0] = '\0';

    buffer_reset(w->response.header_output);
    buffer_reset(w->response.header);
    buffer_reset(w->response.data);
    w->response.rlen = 0;
    w->response.sent = 0;
    w->response.code = 0;

    web_client_enable_wait_receive(w);
    web_client_disable_wait_send(w);

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
    buffer_free(w->response.header_output);
    buffer_free(w->response.header);
    buffer_free(w->response.data);
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
        // getpwuid() is not thread safe,
        // but we have called this function once
        // while single threaded
        struct passwd *pw = getpwuid(geteuid());
        web_owner = config_get(CONFIG_SECTION_WEB, "web files owner", (pw)?(pw->pw_name?pw->pw_name:""):"");
        if(!web_owner || !*web_owner)
            owner_uid = geteuid();
        else {
            // getpwnam() is not thread safe,
            // but we have called this function once
            // while single threaded
            pw = getpwnam(web_owner);
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
        // getgrgid() is not thread safe,
        // but we have called this function once
        // while single threaded
        struct group *gr = getgrgid(getegid());
        web_group = config_get(CONFIG_SECTION_WEB, "web files group", (gr)?(gr->gr_name?gr->gr_name:""):"");
        if(!web_group || !*web_group)
            owner_gid = getegid();
        else {
            // getgrnam() is not thread safe,
            // but we have called this function once
            // while single threaded
            gr = getgrnam(web_group);
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

int mysendfile(struct web_client *w, char *filename) {
    debug(D_WEB_CLIENT, "%llu: Looking for file '%s/%s'", w->id, netdata_configured_web_dir, filename);

    // skip leading slashes
    while (*filename == '/') filename++;

    // if the filename contain known paths, skip them
    if(strncmp(filename, WEB_PATH_FILE "/", strlen(WEB_PATH_FILE) + 1) == 0)
        filename = &filename[strlen(WEB_PATH_FILE) + 1];

    char *s;
    for(s = filename; *s ;s++) {
        if( !isalnum(*s) && *s != '/' && *s != '.' && *s != '-' && *s != '_') {
            debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
            w->response.data->contenttype = CT_TEXT_HTML;
            buffer_sprintf(w->response.data, "Filename contains invalid characters: ");
            buffer_strcat_htmlescape(w->response.data, filename);
            return 400;
        }
    }

    // if the filename contains a .. refuse to serve it
    if(strstr(filename, "..") != 0) {
        debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
        w->response.data->contenttype = CT_TEXT_HTML;
        buffer_strcat(w->response.data, "Relative filenames are not supported: ");
        buffer_strcat_htmlescape(w->response.data, filename);
        return 400;
    }

    // access the file
    char webfilename[FILENAME_MAX + 1];
    snprintfz(webfilename, FILENAME_MAX, "%s/%s", netdata_configured_web_dir, filename);

    // check if the file exists
    struct stat stat;
    if(lstat(webfilename, &stat) != 0) {
        debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not found.", w->id, webfilename);
        w->response.data->contenttype = CT_TEXT_HTML;
        buffer_strcat(w->response.data, "File does not exist, or is not accessible: ");
        buffer_strcat_htmlescape(w->response.data, webfilename);
        return 404;
    }

    // check if the file is owned by expected user
    if(stat.st_uid != web_files_uid()) {
        error("%llu: File '%s' is owned by user %u (expected user %u). Access Denied.", w->id, webfilename, stat.st_uid, web_files_uid());
        w->response.data->contenttype = CT_TEXT_HTML;
        buffer_strcat(w->response.data, "Access to file is not permitted: ");
        buffer_strcat_htmlescape(w->response.data, webfilename);
        return 403;
    }

    // check if the file is owned by expected group
    if(stat.st_gid != web_files_gid()) {
        error("%llu: File '%s' is owned by group %u (expected group %u). Access Denied.", w->id, webfilename, stat.st_gid, web_files_gid());
        w->response.data->contenttype = CT_TEXT_HTML;
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
        w->response.data->contenttype = CT_TEXT_HTML;
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
            w->response.data->contenttype = CT_TEXT_HTML;
            buffer_sprintf(w->response.header, "Location: /" WEB_PATH_FILE "/%s\r\n", filename);
            buffer_strcat(w->response.data, "File is currently busy, please try again later: ");
            buffer_strcat_htmlescape(w->response.data, webfilename);
            return 307;
        }
        else {
            error("%llu: Cannot open file '%s'.", w->id, webfilename);
            w->response.data->contenttype = CT_TEXT_HTML;
            buffer_strcat(w->response.data, "Cannot open file: ");
            buffer_strcat_htmlescape(w->response.data, webfilename);
            return 404;
        }
    }

    sock_setnonblock(w->ifd);

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
    web_client_enable_wait_receive(w);
    web_client_disable_wait_send(w);
    buffer_flush(w->response.data);
    w->response.rlen = stat.st_size;
#ifdef __APPLE__
    w->response.data->date = stat.st_mtimespec.tv_sec;
#else
    w->response.data->date = stat.st_mtim.tv_sec;
#endif /* __APPLE__ */
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

static inline int check_host_and_call(RRDHOST *host, struct web_client *w, char *url, int (*func)(RRDHOST *, struct web_client *, char *)) {
    if(unlikely(host->rrd_memory_mode == RRD_MEMORY_MODE_NONE)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "This host does not maintain a database");
        return 400;
    }

    return func(host, w, url);
}

int web_client_api_request(RRDHOST *host, struct web_client *w, char *url)
{
    // get the api version
    char *tok = mystrsep(&url, "/?&");
    if(tok && *tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for API version '%s'.", w->id, tok);
        if(strcmp(tok, "v1") == 0)
            return web_client_api_request_v1(host, w, url);
        else {
            buffer_flush(w->response.data);
            w->response.data->contenttype = CT_TEXT_HTML;
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

        case CT_PROMETHEUS:
            return "text/plain; version=0.0.4";

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
            web_client_enable_keepalive(w);
    }
    else if(respect_web_browser_do_not_track_policy && hash == hash_donottrack && !strcasecmp(s, "DNT")) {
        if(*v == '0') web_client_disable_donottrack(w);
        else if(*v == '1') web_client_enable_donottrack(w);
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

typedef enum http_validation {
    HTTP_VALIDATION_OK,
    HTTP_VALIDATION_NOT_SUPPORTED,
    HTTP_VALIDATION_INCOMPLETE
} HTTP_VALIDATION;

static inline HTTP_VALIDATION http_request_validate(struct web_client *w) {
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
    else if(!strncmp(s, "STREAM ", 7)) {
        encoded_url = s = &s[7];
        w->mode = WEB_CLIENT_MODE_STREAM;
    }
    else {
        web_client_disable_wait_receive(w);
        return HTTP_VALIDATION_NOT_SUPPORTED;
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
        web_client_enable_wait_receive(w);
        return HTTP_VALIDATION_INCOMPLETE;
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

                web_client_disable_wait_receive(w);
                return HTTP_VALIDATION_OK;
            }

            // another header line
            s = http_header_parse(w, s);
        }
    }

    // incomplete request
    web_client_enable_wait_receive(w);
    return HTTP_VALIDATION_INCOMPLETE;
}

static inline void web_client_send_http_header(struct web_client *w) {
    if(unlikely(w->response.code != 200))
        buffer_no_cacheable(w->response.data);

    // set a proper expiration date, if not already set
    if(unlikely(!w->response.data->expires)) {
        if(w->response.data->options & WB_CONTENT_NO_CACHEABLE)
            w->response.data->expires = w->tv_ready.tv_sec + localhost->rrd_update_every;
        else
            w->response.data->expires = w->tv_ready.tv_sec + 86400;
    }

    // prepare the HTTP response header
    debug(D_WEB_CLIENT, "%llu: Generating HTTP header with response %d.", w->id, w->response.code);

    const char *content_type_string = web_content_type_to_string(w->response.data->contenttype);
    const char *code_msg = web_response_code_to_string(w->response.code);

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
                   , w->response.code, code_msg
                   , web_client_has_keepalive(w)?"keep-alive":"close"
                   , w->origin
                   , content_type_string
                   , date
    );

    if(unlikely(web_x_frame_options))
        buffer_sprintf(w->response.header_output, "X-Frame-Options: %s\r\n", web_x_frame_options);

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

        if(respect_web_browser_do_not_track_policy)
            buffer_sprintf(w->response.header_output,
                    "Tk: T;cookies\r\n");
    }
    else {
        if(respect_web_browser_do_not_track_policy) {
            if(web_client_has_tracking_required(w))
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
            web_client_disable_keepalive(w);
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

    size_t count = 0;
    ssize_t bytes;
    while((bytes = send(w->ofd, buffer_tostring(w->response.header_output), buffer_strlen(w->response.header_output), 0)) == -1) {
        count++;

        if(count > 100 || (errno != EAGAIN && errno != EWOULDBLOCK)) {
            error("Cannot send HTTP headers to web client.");
            break;
        }
    }

    if(bytes != (ssize_t) buffer_strlen(w->response.header_output)) {
        if(bytes > 0)
            w->stats_sent_bytes += bytes;

        error("HTTP headers failed to be sent (I sent %zu bytes but the system sent %zd bytes). Closing web client."
              , buffer_strlen(w->response.header_output)
              , bytes);

        WEB_CLIENT_IS_DEAD(w);
        return;
    }
    else
        w->stats_sent_bytes += bytes;
}

static inline int web_client_process_url(RRDHOST *host, struct web_client *w, char *url);

static inline int web_client_switch_host(RRDHOST *host, struct web_client *w, char *url) {
    static uint32_t hash_localhost = 0;

    if(unlikely(!hash_localhost)) {
        hash_localhost = simple_hash("localhost");
    }

    if(host != localhost) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Nesting of hosts is not allowed.");
        return 400;
    }

    char *tok = mystrsep(&url, "/?&");
    if(tok && *tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for host with name '%s'.", w->id, tok);

        // copy the URL, we need it to serve files
        w->last_url[0] = '/';
        if(url && *url) strncpyz(&w->last_url[1], url, URL_MAX - 1);
        else w->last_url[1] = '\0';

        uint32_t hash = simple_hash(tok);

        host = rrdhost_find_by_hostname(tok, hash);
        if(!host) host = rrdhost_find_by_guid(tok, hash);

        if(host) return web_client_process_url(host, w, url);
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_TEXT_HTML;
    buffer_strcat(w->response.data, "This netdata does not maintain a database for host: ");
    buffer_strcat_htmlescape(w->response.data, tok?tok:"");
    return 404;
}

static inline int web_client_process_url(RRDHOST *host, struct web_client *w, char *url) {
    static uint32_t
            hash_api = 0,
            hash_netdata_conf = 0,
            hash_data = 0,
            hash_datasource = 0,
            hash_graph = 0,
            hash_list = 0,
            hash_all_json = 0,
            hash_host = 0;

#ifdef NETDATA_INTERNAL_CHECKS
    static uint32_t hash_exit = 0, hash_debug = 0, hash_mirror = 0;
#endif

    if(unlikely(!hash_api)) {
        hash_api = simple_hash("api");
        hash_netdata_conf = simple_hash("netdata.conf");
        hash_data = simple_hash(WEB_PATH_DATA);
        hash_datasource = simple_hash(WEB_PATH_DATASOURCE);
        hash_graph = simple_hash(WEB_PATH_GRAPH);
        hash_list = simple_hash("list");
        hash_all_json = simple_hash("all.json");
        hash_host = simple_hash("host");
#ifdef NETDATA_INTERNAL_CHECKS
        hash_exit = simple_hash("exit");
        hash_debug = simple_hash("debug");
        hash_mirror = simple_hash("mirror");
#endif
    }

    char *tok = mystrsep(&url, "/?");
    if(likely(tok && *tok)) {
        uint32_t hash = simple_hash(tok);
        debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

        if(unlikely(hash == hash_api && strcmp(tok, "api") == 0)) {                           // current API
            debug(D_WEB_CLIENT_ACCESS, "%llu: API request ...", w->id);
            return check_host_and_call(host, w, url, web_client_api_request);
        }
        else if(unlikely(hash == hash_host && strcmp(tok, "host") == 0)) {                    // host switching
            debug(D_WEB_CLIENT_ACCESS, "%llu: host switch request ...", w->id);
            return web_client_switch_host(host, w, url);
        }
        else if(unlikely(hash == hash_data && strcmp(tok, WEB_PATH_DATA) == 0)) {             // old API "data"
            debug(D_WEB_CLIENT_ACCESS, "%llu: old API data request...", w->id);
            return check_host_and_call(host, w, url, web_client_api_old_data_request_json);
        }
        else if(unlikely(hash == hash_datasource && strcmp(tok, WEB_PATH_DATASOURCE) == 0)) { // old API "datasource"
            debug(D_WEB_CLIENT_ACCESS, "%llu: old API datasource request...", w->id);
            return check_host_and_call(host, w, url, web_client_api_old_data_request_jsonp);
        }
        else if(unlikely(hash == hash_graph && strcmp(tok, WEB_PATH_GRAPH) == 0)) {           // old API "graph"
            debug(D_WEB_CLIENT_ACCESS, "%llu: old API graph request...", w->id);
            return check_host_and_call(host, w, url, web_client_api_old_graph_request);
        }
        else if(unlikely(hash == hash_list && strcmp(tok, "list") == 0)) {                    // old API "list"
            debug(D_WEB_CLIENT_ACCESS, "%llu: old API list request...", w->id);
            return check_host_and_call(host, w, url, web_client_api_old_list_request);
        }
        else if(unlikely(hash == hash_all_json && strcmp(tok, "all.json") == 0)) {            // old API "all.json"
            debug(D_WEB_CLIENT_ACCESS, "%llu: old API all.json request...", w->id);
            return check_host_and_call(host, w, url, web_client_api_old_all_json);
        }
        else if(unlikely(hash == hash_netdata_conf && strcmp(tok, "netdata.conf") == 0)) {    // netdata.conf
            debug(D_WEB_CLIENT_ACCESS, "%llu: generating netdata.conf ...", w->id);
            w->response.data->contenttype = CT_TEXT_PLAIN;
            buffer_flush(w->response.data);
            config_generate(w->response.data, 0);
            return 200;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        else if(unlikely(hash == hash_exit && strcmp(tok, "exit") == 0)) {
            w->response.data->contenttype = CT_TEXT_PLAIN;
            buffer_flush(w->response.data);

            if(!netdata_exit)
                buffer_strcat(w->response.data, "ok, will do...");
            else
                buffer_strcat(w->response.data, "I am doing it already");

            error("web request to exit received.");
            netdata_cleanup_and_exit(0);
            return 200;
        }
        else if(unlikely(hash == hash_debug && strcmp(tok, "debug") == 0)) {
            buffer_flush(w->response.data);

            // get the name of the data to show
            tok = mystrsep(&url, "/?&");
            if(tok && *tok) {
                debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

                // do we have such a data set?
                RRDSET *st = rrdset_find_byname(host, tok);
                if(!st) st = rrdset_find(host, tok);
                if(!st) {
                    w->response.data->contenttype = CT_TEXT_HTML;
                    buffer_strcat(w->response.data, "Chart is not found: ");
                    buffer_strcat_htmlescape(w->response.data, tok);
                    debug(D_WEB_CLIENT_ACCESS, "%llu: %s is not found.", w->id, tok);
                    return 404;
                }

                debug_flags |= D_RRD_STATS;

                if(rrdset_flag_check(st, RRDSET_FLAG_DEBUG))
                    rrdset_flag_clear(st, RRDSET_FLAG_DEBUG);
                else
                    rrdset_flag_set(st, RRDSET_FLAG_DEBUG);

                w->response.data->contenttype = CT_TEXT_HTML;
                buffer_sprintf(w->response.data, "Chart has now debug %s: ", rrdset_flag_check(st, RRDSET_FLAG_DEBUG)?"enabled":"disabled");
                buffer_strcat_htmlescape(w->response.data, tok);
                debug(D_WEB_CLIENT_ACCESS, "%llu: debug for %s is %s.", w->id, tok, rrdset_flag_check(st, RRDSET_FLAG_DEBUG)?"enabled":"disabled");
                return 200;
            }

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "debug which chart?\r\n");
            return 400;
        }
        else if(unlikely(hash == hash_mirror && strcmp(tok, "mirror") == 0)) {
            debug(D_WEB_CLIENT_ACCESS, "%llu: Mirroring...", w->id);

            // replace the zero bytes with spaces
            buffer_char_replace(w->response.data, '\0', ' ');

            // just leave the buffer as is
            // it will be copied back to the client

            return 200;
        }
#endif  /* NETDATA_INTERNAL_CHECKS */
    }

    char filename[FILENAME_MAX+1];
    url = filename;
    strncpyz(filename, w->last_url, FILENAME_MAX);
    tok = mystrsep(&url, "?");
    buffer_flush(w->response.data);
    return mysendfile(w, (tok && *tok)?tok:"/");
}

void web_client_process_request(struct web_client *w) {

    // start timing us
    now_realtime_timeval(&w->tv_in);

    switch(http_request_validate(w)) {
        case HTTP_VALIDATION_OK:
            switch(w->mode) {
                case WEB_CLIENT_MODE_STREAM:
                    w->response.code = rrdpush_receiver_thread_spawn(localhost, w, w->decoded_url);
                    return;

                case WEB_CLIENT_MODE_OPTIONS:
                    w->response.data->contenttype = CT_TEXT_PLAIN;
                    buffer_flush(w->response.data);
                    buffer_strcat(w->response.data, "OK");
                    w->response.code = 200;
                    break;

                case WEB_CLIENT_MODE_FILECOPY:
                case WEB_CLIENT_MODE_NORMAL:
                    w->response.code = web_client_process_url(localhost, w, w->decoded_url);
                    break;
            }
            break;

        case HTTP_VALIDATION_INCOMPLETE:
            if(w->response.data->len > TOO_BIG_REQUEST) {
                strcpy(w->last_url, "too big request");

                debug(D_WEB_CLIENT_ACCESS, "%llu: Received request is too big (%zu bytes).", w->id, w->response.data->len);

                buffer_flush(w->response.data);
                buffer_sprintf(w->response.data, "Received request is too big  (%zu bytes).\r\n", w->response.data->len);
                w->response.code = 400;
            }
            else {
                // wait for more data
                return;
            }
            break;

        case HTTP_VALIDATION_NOT_SUPPORTED:
            debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot understand '%s'.", w->id, w->response.data->buffer);

            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "I don't understand you...\r\n");
            w->response.code = 400;
            break;
    }

    // keep track of the time we done processing
    now_realtime_timeval(&w->tv_ready);

    w->response.sent = 0;

    // set a proper last modified date
    if(unlikely(!w->response.data->date))
        w->response.data->date = w->tv_ready.tv_sec;

    web_client_send_http_header(w);

    // enable sending immediately if we have data
    if(w->response.data->len) web_client_enable_wait_send(w);
    else web_client_disable_wait_send(w);

    switch(w->mode) {
        case WEB_CLIENT_MODE_STREAM:
            debug(D_WEB_CLIENT, "%llu: STREAM done.", w->id);
            break;

        case WEB_CLIENT_MODE_OPTIONS:
            debug(D_WEB_CLIENT, "%llu: Done preparing the OPTIONS response. Sending data (%zu bytes) to client.", w->id, w->response.data->len);
            break;

        case WEB_CLIENT_MODE_NORMAL:
            debug(D_WEB_CLIENT, "%llu: Done preparing the response. Sending data (%zu bytes) to client.", w->id, w->response.data->len);
            break;

        case WEB_CLIENT_MODE_FILECOPY:
            if(w->response.rlen) {
                debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending data file of %zu bytes to client.", w->id, w->response.rlen);
                web_client_enable_wait_receive(w);

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
            fatal("%llu: Unknown client mode %u.", w->id, w->mode);
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

        if(w->mode == WEB_CLIENT_MODE_FILECOPY && web_client_has_wait_receive(w) && w->response.rlen && w->response.rlen > w->response.data->len) {
            // we have to wait, more data will come
            debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
            web_client_disable_wait_send(w);
            return t;
        }

        if(unlikely(!web_client_has_keepalive(w))) {
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
            || (w->mode == WEB_CLIENT_MODE_FILECOPY && !web_client_has_wait_receive(w) && w->response.data->len == w->response.rlen)) {
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

        if(w->mode == WEB_CLIENT_MODE_FILECOPY && web_client_has_wait_receive(w) && w->response.rlen && w->response.rlen > w->response.data->len) {
            // we have to wait, more data will come
            debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
            web_client_disable_wait_send(w);
            return 0;
        }

        if(unlikely(!web_client_has_keepalive(w))) {
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
            web_client_enable_wait_send(w);

            if(w->response.rlen && w->response.data->len >= w->response.rlen)
                web_client_disable_wait_receive(w);
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
            web_client_disable_wait_receive(w);

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
    int retval, timeout;
    nfds_t fdmax = 0;

    log_access("%llu: %s port %s connected on thread task id %d", w->id, w->client_ip, w->client_port, gettid());

    for(;;) {
        if(unlikely(netdata_exit)) break;

        if(unlikely(web_client_check_dead(w))) {
            debug(D_WEB_CLIENT, "%llu: client is dead.", w->id);
            break;
        }
        else if(unlikely(!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))) {
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

            if(web_client_has_wait_receive(w)) fds[0].events |= POLLIN;
            if(web_client_has_wait_send(w))    fds[0].events |= POLLOUT;

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
            if(web_client_has_wait_receive(w)) fds[0].events |= POLLIN;
            ifd = &fds[0];

            fds[1].fd = w->ofd;
            fds[1].events = 0;
            fds[1].revents = 0;
            if(web_client_has_wait_send(w))    fds[1].events |= POLLOUT;
            ofd = &fds[1];

            fdmax = 2;
        }

        debug(D_WEB_CLIENT, "%llu: Waiting socket async I/O for %s %s", w->id, web_client_has_wait_receive(w)?"INPUT":"", web_client_has_wait_send(w)?"OUTPUT":"");
        errno = 0;
        timeout = web_client_timeout * 1000;
        retval = poll(fds, fdmax, timeout);

        if(unlikely(netdata_exit)) break;

        if(unlikely(retval == -1)) {
            if(errno == EAGAIN || errno == EINTR) {
                debug(D_WEB_CLIENT, "%llu: EAGAIN received.", w->id);
                continue;
            }

            debug(D_WEB_CLIENT, "%llu: LISTENER: poll() failed (input fd = %d, output fd = %d). Closing client.", w->id, w->ifd, w->ofd);
            break;
        }
        else if(unlikely(!retval)) {
            debug(D_WEB_CLIENT, "%llu: Timeout while waiting socket async I/O for %s %s", w->id, web_client_has_wait_receive(w)?"INPUT":"", web_client_has_wait_send(w)?"OUTPUT":"");
            break;
        }

        if(unlikely(netdata_exit)) break;

        int used = 0;
        if(web_client_has_wait_send(w) && ofd->revents & POLLOUT) {
            used++;
            if(web_client_send(w) < 0) {
                debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
                break;
            }
        }

        if(unlikely(netdata_exit)) break;

        if(web_client_has_wait_receive(w) && (ifd->revents & POLLIN || ifd->revents & POLLPRI)) {
            used++;
            if(web_client_receive(w) < 0) {
                debug(D_WEB_CLIENT, "%llu: Cannot receive data from client. Closing client.", w->id);
                break;
            }

            if(w->mode == WEB_CLIENT_MODE_NORMAL) {
                debug(D_WEB_CLIENT, "%llu: Attempting to process received data.", w->id);
                web_client_process_request(w);

                // if the sockets are closed, may have transferred this client
                // to plugins.d
                if(unlikely(w->mode == WEB_CLIENT_MODE_STREAM))
                    break;
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

    WEB_CLIENT_IS_OBSOLETE(w);

    pthread_exit(NULL);
    return NULL;
}
