// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "web_client_cache.h"

// ----------------------------------------------------------------------------
// allocate and free web_clients

#ifdef ENABLE_HTTPS

static void web_client_reuse_ssl(struct web_client *w) {
    if (netdata_srv_ctx) {
        if (w->ssl.conn) {
            SSL_clear(w->ssl.conn);
        }
    }
}
#endif


static void web_client_zero(struct web_client *w) {
    // zero everything about it - but keep the buffers

    // remember the pointers to the buffers
    BUFFER *b1 = w->response.data;
    BUFFER *b2 = w->response.header;
    BUFFER *b3 = w->response.header_output;

    // empty the buffers
    buffer_flush(b1);
    buffer_flush(b2);
    buffer_flush(b3);

    freez(w->user_agent);

    // zero everything
    memset(w, 0, sizeof(struct web_client));

    // restore the pointers of the buffers
    w->response.data = b1;
    w->response.header = b2;
    w->response.header_output = b3;
}

static void web_client_free(struct web_client *w) {
    buffer_free(w->response.header_output);
    buffer_free(w->response.header);
    buffer_free(w->response.data);
    freez(w->user_agent);
#ifdef ENABLE_HTTPS
    if ((!web_client_check_unix(w)) && ( netdata_srv_ctx )) {
        if (w->ssl.conn) {
            SSL_free(w->ssl.conn);
            w->ssl.conn = NULL;
        }
    }
#endif
    freez(w);
}

static struct web_client *web_client_alloc(void) {
    struct web_client *w = callocz(1, sizeof(struct web_client));
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    w->response.header = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    w->response.header_output = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    return w;
}

// ----------------------------------------------------------------------------
// web clients caching

// When clients connect and disconnect, avoid allocating and releasing memory.
// Instead, when new clients get connected, reuse any memory previously allocated
// for serving web clients that are now disconnected.

// The size of the cache is adaptive. It caches the structures of 2x
// the number of currently connected clients.

// Comments per server:
// SINGLE-THREADED : 1 cache is maintained
// MULTI-THREADED  : 1 cache is maintained
// STATIC-THREADED : 1 cache for each thred of the web server

__thread struct clients_cache web_clients_cache = {
        .pid = 0,
        .used = NULL,
        .used_count = 0,
        .avail = NULL,
        .avail_count = 0,
        .allocated = 0,
        .reused = 0
};

inline void web_client_cache_verify(int force) {
#ifdef NETDATA_INTERNAL_CHECKS
    static __thread size_t count = 0;
    count++;

    if(unlikely(force || count > 1000)) {
        count = 0;

        struct web_client *w;
        size_t used = 0, avail = 0;
        for(w = web_clients_cache.used; w ; w = w->next) used++;
        for(w = web_clients_cache.avail; w ; w = w->next) avail++;

        info("web_client_cache has %zu (%zu) used and %zu (%zu) available clients, allocated %zu, reused %zu (hit %zu%%)."
             , used, web_clients_cache.used_count
             , avail, web_clients_cache.avail_count
             , web_clients_cache.allocated
             , web_clients_cache.reused
             , (web_clients_cache.allocated + web_clients_cache.reused)?(web_clients_cache.reused * 100 / (web_clients_cache.allocated + web_clients_cache.reused)):0
        );
    }
#else
    if(unlikely(force)) {
        info("web_client_cache has %zu used and %zu available clients, allocated %zu, reused %zu (hit %zu%%)."
             , web_clients_cache.used_count
             , web_clients_cache.avail_count
             , web_clients_cache.allocated
             , web_clients_cache.reused
             , (web_clients_cache.allocated + web_clients_cache.reused)?(web_clients_cache.reused * 100 / (web_clients_cache.allocated + web_clients_cache.reused)):0
            );
    }
#endif
}

// destroy the cache and free all the memory it uses
void web_client_cache_destroy(void) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(web_clients_cache.pid != 0 && web_clients_cache.pid != gettid()))
        error("Oops! wrong thread accessing the cache. Expected %d, found %d", (int)web_clients_cache.pid, (int)gettid());

    web_client_cache_verify(1);
#endif

    netdata_thread_disable_cancelability();

    struct web_client *w, *t;

    w = web_clients_cache.used;
    while(w) {
        t = w;
        w = w->next;
        web_client_free(t);
    }
    web_clients_cache.used = NULL;
    web_clients_cache.used_count = 0;

    w = web_clients_cache.avail;
    while(w) {
        t = w;
        w = w->next;
        web_client_free(t);
    }
    web_clients_cache.avail = NULL;
    web_clients_cache.avail_count = 0;

    netdata_thread_enable_cancelability();
}

struct web_client *web_client_get_from_cache_or_allocate() {

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(web_clients_cache.pid == 0))
        web_clients_cache.pid = gettid();

    if(unlikely(web_clients_cache.pid != 0 && web_clients_cache.pid != gettid()))
        error("Oops! wrong thread accessing the cache. Expected %d, found %d", (int)web_clients_cache.pid, (int)gettid());
#endif

    netdata_thread_disable_cancelability();

    struct web_client *w = web_clients_cache.avail;

    if(w) {
        // get it from avail
        if (w == web_clients_cache.avail) web_clients_cache.avail = w->next;
        if(w->prev) w->prev->next = w->next;
        if(w->next) w->next->prev = w->prev;
        web_clients_cache.avail_count--;
#ifdef ENABLE_HTTPS
        web_client_reuse_ssl(w);
        SSL *ssl = w->ssl.conn;
#endif
        web_client_zero(w);
        web_clients_cache.reused++;
#ifdef ENABLE_HTTPS
        w->ssl.conn = ssl;
        w->ssl.flags = NETDATA_SSL_START;
        debug(D_WEB_CLIENT_ACCESS,"Reusing SSL structure with (w->ssl = NULL, w->accepted = %d)",w->ssl.flags);
#endif
    }
    else {
        // allocate it
        w = web_client_alloc();
#ifdef ENABLE_HTTPS
        w->ssl.flags = NETDATA_SSL_START;
        debug(D_WEB_CLIENT_ACCESS,"Starting SSL structure with (w->ssl = NULL, w->accepted = %d)",w->ssl.flags);
#endif
        web_clients_cache.allocated++;
    }

    // link it to used web clients
    if (web_clients_cache.used) web_clients_cache.used->prev = w;
    w->next = web_clients_cache.used;
    w->prev = NULL;
    web_clients_cache.used = w;
    web_clients_cache.used_count++;

    // initialize it
    w->id = web_client_connected();
    w->mode = WEB_CLIENT_MODE_NORMAL;

    netdata_thread_enable_cancelability();

    return w;
}

void web_client_release(struct web_client *w) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(web_clients_cache.pid != 0 && web_clients_cache.pid != gettid()))
        error("Oops! wrong thread accessing the cache. Expected %d, found %d", (int)web_clients_cache.pid, (int)gettid());

    if(unlikely(w->running))
        error("%llu: releasing web client from %s port %s, but it still running.", w->id, w->client_ip, w->client_port);
#endif

    debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s port %s.", w->id, w->client_ip, w->client_port);

    web_server_log_connection(w, "DISCONNECTED");
    web_client_request_done(w);
    web_client_disconnected();

    netdata_thread_disable_cancelability();

    if(web_server_mode != WEB_SERVER_MODE_STATIC_THREADED) {
        if (w->ifd != -1) close(w->ifd);
        if (w->ofd != -1 && w->ofd != w->ifd) close(w->ofd);
        w->ifd = w->ofd = -1;
#ifdef ENABLE_HTTPS
        web_client_reuse_ssl(w);
        w->ssl.flags = NETDATA_SSL_START;
#endif

    }

    // unlink it from the used
    if (w == web_clients_cache.used) web_clients_cache.used = w->next;
    if(w->prev) w->prev->next = w->next;
    if(w->next) w->next->prev = w->prev;
    web_clients_cache.used_count--;

    if(web_clients_cache.avail_count >= 2 * web_clients_cache.used_count) {
        // we have too many of them - free it
        web_client_free(w);
    }
    else {
        // link it to the avail
        if (web_clients_cache.avail) web_clients_cache.avail->prev = w;
        w->next = web_clients_cache.avail;
        w->prev = NULL;
        web_clients_cache.avail = w;
        web_clients_cache.avail_count++;
    }

    netdata_thread_enable_cancelability();
}

