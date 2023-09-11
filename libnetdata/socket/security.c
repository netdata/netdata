#include "../libnetdata.h"

#ifdef ENABLE_HTTPS

SSL_CTX *netdata_ssl_exporting_ctx =NULL;
SSL_CTX *netdata_ssl_streaming_sender_ctx =NULL;
SSL_CTX *netdata_ssl_web_server_ctx =NULL;
const char *netdata_ssl_security_key =NULL;
const char *netdata_ssl_security_cert =NULL;
const char *tls_version=NULL;
const char *tls_ciphers=NULL;
bool netdata_ssl_validate_certificate =  true;
bool netdata_ssl_validate_certificate_sender =  true;

static SOCKET_PEERS netdata_ssl_peers(NETDATA_SSL *ssl) {
    int sock_fd;

    if(unlikely(!ssl->conn))
        sock_fd = -1;
    else
        sock_fd = SSL_get_rfd(ssl->conn);

    return socket_peers(sock_fd);
}

static void netdata_ssl_log_error_queue(const char *call, NETDATA_SSL *ssl, unsigned long err) {
    error_limit_static_thread_var(erl, 1, 0);

    if(err == SSL_ERROR_NONE)
        err = ERR_get_error();

    if(err == SSL_ERROR_NONE)
        return;

    do {
        char *code;

        switch (err) {
            case SSL_ERROR_SSL:
                code = "SSL_ERROR_SSL";
                ssl->state = NETDATA_SSL_STATE_FAILED;
                break;

            case SSL_ERROR_WANT_READ:
                code = "SSL_ERROR_WANT_READ";
                break;

            case SSL_ERROR_WANT_WRITE:
                code = "SSL_ERROR_WANT_WRITE";
                break;

            case SSL_ERROR_WANT_X509_LOOKUP:
                code = "SSL_ERROR_WANT_X509_LOOKUP";
                break;

            case SSL_ERROR_SYSCALL:
                code = "SSL_ERROR_SYSCALL";
                ssl->state = NETDATA_SSL_STATE_FAILED;
                break;

            case SSL_ERROR_ZERO_RETURN:
                code = "SSL_ERROR_ZERO_RETURN";
                break;

            case SSL_ERROR_WANT_CONNECT:
                code = "SSL_ERROR_WANT_CONNECT";
                break;

            case SSL_ERROR_WANT_ACCEPT:
                code = "SSL_ERROR_WANT_ACCEPT";
                break;

#ifdef SSL_ERROR_WANT_ASYNC
            case SSL_ERROR_WANT_ASYNC:
                code = "SSL_ERROR_WANT_ASYNC";
                break;
#endif

#ifdef SSL_ERROR_WANT_ASYNC_JOB
            case SSL_ERROR_WANT_ASYNC_JOB:
                code = "SSL_ERROR_WANT_ASYNC_JOB";
                break;
#endif

#ifdef SSL_ERROR_WANT_CLIENT_HELLO_CB
            case SSL_ERROR_WANT_CLIENT_HELLO_CB:
                code = "SSL_ERROR_WANT_CLIENT_HELLO_CB";
                break;
#endif

#ifdef SSL_ERROR_WANT_RETRY_VERIFY
            case SSL_ERROR_WANT_RETRY_VERIFY:
                code = "SSL_ERROR_WANT_RETRY_VERIFY";
                break;
#endif

            default:
                code = "SSL_ERROR_UNKNOWN";
                break;
        }

        char str[1024 + 1];
        ERR_error_string_n(err, str, 1024);
        str[1024] = '\0';
        SOCKET_PEERS peers = netdata_ssl_peers(ssl);
        error_limit(&erl, "SSL: %s() on socket local [[%s]:%d] <-> remote [[%s]:%d], returned error %lu (%s): %s",
                    call, peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, err, code, str);

    } while((err = ERR_get_error()));
}

bool netdata_ssl_open(NETDATA_SSL *ssl, SSL_CTX *ctx, int fd) {
    errno = 0;
    ssl->ssl_errno = 0;

    if(ssl->conn) {
        if(!ctx || SSL_get_SSL_CTX(ssl->conn) != ctx) {
            SSL_free(ssl->conn);
            ssl->conn = NULL;
        }
        else if (SSL_clear(ssl->conn) == 0) {
            netdata_ssl_log_error_queue("SSL_clear", ssl, SSL_ERROR_NONE);
            SSL_free(ssl->conn);
            ssl->conn = NULL;
        }
    }

    if(!ssl->conn) {
        if(!ctx) {
            internal_error(true, "SSL: not CTX given");
            ssl->state = NETDATA_SSL_STATE_FAILED;
            return false;
        }

        ssl->conn = SSL_new(ctx);
        if (!ssl->conn) {
            netdata_ssl_log_error_queue("SSL_new", ssl, SSL_ERROR_NONE);
            ssl->state = NETDATA_SSL_STATE_FAILED;
            return false;
        }
    }

    if(SSL_set_fd(ssl->conn, fd) != 1) {
        netdata_ssl_log_error_queue("SSL_set_fd", ssl, SSL_ERROR_NONE);
        ssl->state = NETDATA_SSL_STATE_FAILED;
        return false;
    }

    ssl->state = NETDATA_SSL_STATE_INIT;

    ERR_clear_error();

    return true;
}

void netdata_ssl_close(NETDATA_SSL *ssl) {
    errno = 0;
    ssl->ssl_errno = 0;

    if(ssl->conn) {
        if(SSL_connection(ssl)) {
            int ret = SSL_shutdown(ssl->conn);
            if(ret == 0)
                SSL_shutdown(ssl->conn);
        }

        SSL_free(ssl->conn);

        ERR_clear_error();
    }

    *ssl = NETDATA_SSL_UNSET_CONNECTION;
}

static inline bool is_handshake_complete(NETDATA_SSL *ssl, const char *op) {
    error_limit_static_thread_var(erl, 1, 0);

    if(unlikely(!ssl->conn)) {
        internal_error(true, "SSL: trying to %s on a NULL connection", op);
        return false;
    }

    switch(ssl->state) {
        case NETDATA_SSL_STATE_NOT_SSL: {
            SOCKET_PEERS peers = netdata_ssl_peers(ssl);
            error_limit(&erl, "SSL: on socket local [[%s]:%d] <-> remote [[%s]:%d], attempt to %s on non-SSL connection",
                        peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, op);
            return false;
        }

        case NETDATA_SSL_STATE_INIT: {
            SOCKET_PEERS peers = netdata_ssl_peers(ssl);
            error_limit(&erl, "SSL: on socket local [[%s]:%d] <-> remote [[%s]:%d], attempt to %s on an incomplete connection",
                        peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, op);
            return false;
        }

        case NETDATA_SSL_STATE_FAILED: {
            SOCKET_PEERS peers = netdata_ssl_peers(ssl);
            error_limit(&erl, "SSL: on socket local [[%s]:%d] <-> remote [[%s]:%d], attempt to %s on a failed connection",
                        peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, op);
            return false;
        }

        case NETDATA_SSL_STATE_COMPLETE: {
            return true;
        }
    }

    return false;
}

/*
 * netdata_ssl_read() should return the same as read():
 *
 * Positive value: The read() function succeeded and read some bytes. The exact number of bytes read is returned.
 *
 * Zero: For files and sockets, a return value of zero signifies end-of-file (EOF), meaning no more data is available
 *       for reading. For sockets, this usually means the other side has closed the connection.
 *
 * -1: An error occurred. The specific error can be found by examining the errno variable.
 *     EAGAIN or EWOULDBLOCK: The file descriptor is in non-blocking mode, and the read operation would block.
 *     (These are often the same value, but can be different on some systems.)
 */

ssize_t netdata_ssl_read(NETDATA_SSL *ssl, void *buf, size_t num) {
    errno = 0;
    ssl->ssl_errno = 0;

    if(unlikely(!is_handshake_complete(ssl, "read")))
        return -1;

    int bytes = SSL_read(ssl->conn, buf, (int)num);

    if(unlikely(bytes <= 0)) {
        int err = SSL_get_error(ssl->conn, bytes);
        if (err == SSL_ERROR_ZERO_RETURN) {
            ssl->ssl_errno = err;
            return 0;
        }

        if (err == SSL_ERROR_WANT_READ || err == SSL_ERROR_WANT_WRITE) {
            ssl->ssl_errno = err;
            errno = EWOULDBLOCK;
        }
        else
            netdata_ssl_log_error_queue("SSL_read", ssl, err);

        bytes = -1;  // according to read() or recv()
    }

    return bytes;
}

/*
 * netdata_ssl_write() should return the same as write():
 *
 * Positive value: The write() function succeeded and wrote some bytes. The exact number of bytes written is returned.
 *
 * Zero: It's technically possible for write() to return zero, indicating that zero bytes were written. However, for a
 * socket, this generally does not happen unless the size of the data to be written is zero.
 *
 * -1: An error occurred. The specific error can be found by examining the errno variable.
 *     EAGAIN or EWOULDBLOCK: The file descriptor is in non-blocking mode, and the write operation would block.
 *     (These are often the same value, but can be different on some systems.)
 */

ssize_t netdata_ssl_write(NETDATA_SSL *ssl, const void *buf, size_t num) {
    errno = 0;
    ssl->ssl_errno = 0;

    if(unlikely(!is_handshake_complete(ssl, "write")))
        return -1;

    int bytes = SSL_write(ssl->conn, (uint8_t *)buf, (int)num);

    if(unlikely(bytes <= 0)) {
        int err = SSL_get_error(ssl->conn, bytes);
        if (err == SSL_ERROR_WANT_READ || err == SSL_ERROR_WANT_WRITE) {
            ssl->ssl_errno = err;
            errno = EWOULDBLOCK;
        }
        else
            netdata_ssl_log_error_queue("SSL_write", ssl, err);

        bytes = -1; // according to write() or send()
    }

    return bytes;
}

static inline bool is_handshake_initialized(NETDATA_SSL *ssl, const char *op) {
    error_limit_static_thread_var(erl, 1, 0);

    if(unlikely(!ssl->conn)) {
        internal_error(true, "SSL: trying to %s on a NULL connection", op);
        return false;
    }

    switch(ssl->state) {
        case NETDATA_SSL_STATE_NOT_SSL: {
            SOCKET_PEERS peers = netdata_ssl_peers(ssl);
            error_limit(&erl, "SSL: on socket local [[%s]:%d] <-> remote [[%s]:%d], attempt to %s on non-SSL connection",
                        peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, op);
            return false;
        }

        case NETDATA_SSL_STATE_INIT: {
            return true;
        }

        case NETDATA_SSL_STATE_FAILED: {
            SOCKET_PEERS peers = netdata_ssl_peers(ssl);
            error_limit(&erl, "SSL: on socket local [[%s]:%d] <-> remote [[%s]:%d], attempt to %s on a failed connection",
                        peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, op);
            return false;
        }

        case NETDATA_SSL_STATE_COMPLETE: {
            SOCKET_PEERS peers = netdata_ssl_peers(ssl);
            error_limit(&erl, "SSL: on socket local [[%s]:%d] <-> remote [[%s]:%d], attempt to %s on an complete connection",
                        peers.local.ip, peers.local.port, peers.peer.ip, peers.peer.port, op);
            return false;
        }
    }

    return false;
}

#define WANT_READ_WRITE_TIMEOUT_MS 10

static inline bool want_read_write_should_retry(NETDATA_SSL *ssl, int err) {
    int ssl_errno = SSL_get_error(ssl->conn, err);
    if(ssl_errno == SSL_ERROR_WANT_READ || ssl_errno == SSL_ERROR_WANT_WRITE) {
        struct pollfd pfds[1] = { [0] = {
                .fd = SSL_get_rfd(ssl->conn),
                .events = (short)(((ssl_errno == SSL_ERROR_WANT_READ ) ? POLLIN  : 0) |
                                  ((ssl_errno == SSL_ERROR_WANT_WRITE) ? POLLOUT : 0)),
        }};

        if(poll(pfds, 1, WANT_READ_WRITE_TIMEOUT_MS) <= 0)
            return false; // timeout (0) or error (<0)

        return true; // we have activity, so we should retry
    }

    return false; // an unknown error
}

bool netdata_ssl_connect(NETDATA_SSL *ssl) {
    errno = 0;
    ssl->ssl_errno = 0;

    if(unlikely(!is_handshake_initialized(ssl, "connect")))
        return false;

    SSL_set_connect_state(ssl->conn);

    int err;
    while ((err = SSL_connect(ssl->conn)) != 1) {
        if(!want_read_write_should_retry(ssl, err))
            break;
    }

    if (err != 1) {
        err = SSL_get_error(ssl->conn, err);
        netdata_ssl_log_error_queue("SSL_connect", ssl, err);
        ssl->state = NETDATA_SSL_STATE_FAILED;
        return false;
    }

    ssl->state = NETDATA_SSL_STATE_COMPLETE;
    return true;
}

bool netdata_ssl_accept(NETDATA_SSL *ssl) {
    errno = 0;
    ssl->ssl_errno = 0;

    if(unlikely(!is_handshake_initialized(ssl, "accept")))
        return false;

    SSL_set_accept_state(ssl->conn);

    int err;
    while ((err = SSL_accept(ssl->conn)) != 1) {
        if(!want_read_write_should_retry(ssl, err))
            break;
    }

    if (err != 1) {
        err = SSL_get_error(ssl->conn, err);
        netdata_ssl_log_error_queue("SSL_accept", ssl, err);
        ssl->state = NETDATA_SSL_STATE_FAILED;
        return false;
    }

    ssl->state = NETDATA_SSL_STATE_COMPLETE;
    return true;
}

/**
 * Info Callback
 *
 * Function used as callback for the OpenSSL Library
 *
 * @param ssl a pointer to the SSL structure of the client
 * @param where the variable with the flags set.
 * @param ret the return of the caller
 */
static void netdata_ssl_info_callback(const SSL *ssl, int where, int ret __maybe_unused) {
    (void)ssl;
    if (where & SSL_CB_ALERT) {
        netdata_log_debug(D_WEB_CLIENT,"SSL INFO CALLBACK %s %s", SSL_alert_type_string(ret), SSL_alert_desc_string_long(ret));
    }
}

/**
 * OpenSSL Library
 *
 * Starts the openssl library for the Netdata.
 */
void netdata_ssl_initialize_openssl() {

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
# if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097)
    OPENSSL_config(NULL);
# endif

    SSL_load_error_strings();

    SSL_library_init();

#else

    if (OPENSSL_init_ssl(OPENSSL_INIT_LOAD_CONFIG, NULL) != 1) {
        netdata_log_error("SSL library cannot be initialized.");
    }

#endif
}

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_110
/**
 * TLS version
 *
 * Returns the TLS version depending of the user input.
 *
 * @param lversion is the user input.
 *
 * @return it returns the version number.
 */
static int netdata_ssl_select_tls_version(const char *lversion) {
    if (!strcmp(lversion, "1") || !strcmp(lversion, "1.0"))
        return TLS1_VERSION;
    else if (!strcmp(lversion, "1.1"))
        return TLS1_1_VERSION;
    else if (!strcmp(lversion, "1.2"))
        return TLS1_2_VERSION;
#if defined(TLS1_3_VERSION)
    else if (!strcmp(lversion, "1.3"))
        return TLS1_3_VERSION;
#endif

#if defined(TLS_MAX_VERSION)
    return TLS_MAX_VERSION;
#else
    return TLS1_2_VERSION;
#endif
}
#endif

/**
 * Initialize Openssl Client
 *
 * Starts the client context with TLS 1.2.
 *
 * @return It returns the context on success or NULL otherwise
 */
SSL_CTX * netdata_ssl_create_client_ctx(unsigned long mode) {
    SSL_CTX *ctx;
#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
    ctx = SSL_CTX_new(SSLv23_client_method());
#else
    ctx = SSL_CTX_new(TLS_client_method());
#endif
    if(ctx) {
#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
        SSL_CTX_set_options (ctx,SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3|SSL_OP_NO_COMPRESSION);
#else
        SSL_CTX_set_min_proto_version(ctx, TLS1_VERSION);
# if defined(TLS_MAX_VERSION)
        SSL_CTX_set_max_proto_version(ctx, TLS_MAX_VERSION);
# elif defined(TLS1_3_VERSION)
        SSL_CTX_set_max_proto_version(ctx, TLS1_3_VERSION);
# elif defined(TLS1_2_VERSION)
        SSL_CTX_set_max_proto_version(ctx, TLS1_2_VERSION);
# endif
#endif
    }

    if(mode)
        SSL_CTX_set_mode(ctx, mode);

    return ctx;
}

/**
 * Initialize OpenSSL server
 *
 * Starts the server context with TLS 1.2 and load the certificate.
 *
 * @return It returns the context on success or NULL otherwise
 */
static SSL_CTX * netdata_ssl_create_server_ctx(unsigned long mode) {
    SSL_CTX *ctx;
    char lerror[512];
	static int netdata_id_context = 1;

    //TO DO: Confirm the necessity to check return for other OPENSSL function
#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
	ctx = SSL_CTX_new(SSLv23_server_method());
    if (!ctx) {
		netdata_log_error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_file(ctx, netdata_ssl_security_cert, SSL_FILETYPE_PEM);
#else
    ctx = SSL_CTX_new(TLS_server_method());
    if (!ctx) {
        netdata_log_error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_chain_file(ctx, netdata_ssl_security_cert);
#endif

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
    SSL_CTX_set_options(ctx, SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3|SSL_OP_NO_COMPRESSION);
#else
    SSL_CTX_set_min_proto_version(ctx, TLS1_VERSION);
    SSL_CTX_set_max_proto_version(ctx, netdata_ssl_select_tls_version(tls_version));

    if(tls_ciphers  && strcmp(tls_ciphers, "none") != 0) {
        if (!SSL_CTX_set_cipher_list(ctx, tls_ciphers)) {
            netdata_log_error("SSL error. cannot set the cipher list");
        }
    }
#endif

    SSL_CTX_use_PrivateKey_file(ctx, netdata_ssl_security_key,SSL_FILETYPE_PEM);

    if (!SSL_CTX_check_private_key(ctx)) {
        ERR_error_string_n(ERR_get_error(),lerror,sizeof(lerror));
        netdata_log_error("SSL cannot check the private key: %s",lerror);
        SSL_CTX_free(ctx);
        return NULL;
    }

	SSL_CTX_set_session_id_context(ctx,(void*)&netdata_id_context,(unsigned int)sizeof(netdata_id_context));
    SSL_CTX_set_info_callback(ctx, netdata_ssl_info_callback);

#if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_095)
	SSL_CTX_set_verify_depth(ctx,1);
#endif
    netdata_log_debug(D_WEB_CLIENT,"SSL GLOBAL CONTEXT STARTED\n");

    SSL_CTX_set_mode(ctx, mode);

    return ctx;
}

/**
 * Start SSL
 *
 * Call the correct function to start the SSL context.
 *
 * @param selector informs the context that must be initialized, the following list has the valid values:
 *      NETDATA_SSL_CONTEXT_SERVER - the server context
 *      NETDATA_SSL_CONTEXT_STREAMING - Starts the streaming context.
 *      NETDATA_SSL_CONTEXT_EXPORTING - Starts the OpenTSDB context
 */
void netdata_ssl_initialize_ctx(int selector) {
    static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&sp);

    switch (selector) {
        case NETDATA_SSL_WEB_SERVER_CTX: {
            if(!netdata_ssl_web_server_ctx) {
                struct stat statbuf;
                if (stat(netdata_ssl_security_key, &statbuf) || stat(netdata_ssl_security_cert, &statbuf))
                    netdata_log_info("To use encryption it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
                else {
                    netdata_ssl_web_server_ctx = netdata_ssl_create_server_ctx(
                            SSL_MODE_ENABLE_PARTIAL_WRITE |
                            SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER |
                            // SSL_MODE_AUTO_RETRY |
                            0);

                    if(netdata_ssl_web_server_ctx && !netdata_ssl_validate_certificate)
                        SSL_CTX_set_verify(netdata_ssl_web_server_ctx, SSL_VERIFY_NONE, NULL);
                }
            }
            break;
        }

        case NETDATA_SSL_STREAMING_SENDER_CTX: {
            if(!netdata_ssl_streaming_sender_ctx) {
                //This is necessary for the stream, because it is working sometimes with nonblock socket.
                //It returns the bitmask after to change, there is not any description of errors in the documentation
                netdata_ssl_streaming_sender_ctx = netdata_ssl_create_client_ctx(
                        SSL_MODE_ENABLE_PARTIAL_WRITE |
                        SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER |
                        // SSL_MODE_AUTO_RETRY |
                        0
                );

                if(netdata_ssl_streaming_sender_ctx && !netdata_ssl_validate_certificate_sender)
                    SSL_CTX_set_verify(netdata_ssl_streaming_sender_ctx, SSL_VERIFY_NONE, NULL);
            }
            break;
        }

        case NETDATA_SSL_EXPORTING_CTX: {
            if(!netdata_ssl_exporting_ctx) {
                netdata_ssl_exporting_ctx = netdata_ssl_create_client_ctx(0);

                if(netdata_ssl_exporting_ctx && !netdata_ssl_validate_certificate)
                    SSL_CTX_set_verify(netdata_ssl_exporting_ctx, SSL_VERIFY_NONE, NULL);
            }
            break;
        }
    }

    spinlock_unlock(&sp);
}

/**
 * Clean Open SSL
 *
 * Clean all the allocated contexts from netdata.
 */
void netdata_ssl_cleanup()
{
    if (netdata_ssl_web_server_ctx) {
        SSL_CTX_free(netdata_ssl_web_server_ctx);
        netdata_ssl_web_server_ctx = NULL;
    }

    if (netdata_ssl_streaming_sender_ctx) {
        SSL_CTX_free(netdata_ssl_streaming_sender_ctx);
        netdata_ssl_streaming_sender_ctx = NULL;
    }

    if (netdata_ssl_exporting_ctx) {
        SSL_CTX_free(netdata_ssl_exporting_ctx);
        netdata_ssl_exporting_ctx = NULL;
    }

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
    ERR_free_strings();
#endif
}

/**
 * Test Certificate
 *
 * Check the certificate of Netdata parent
 *
 * @param ssl is the connection structure
 *
 * @return It returns 0 on success and -1 otherwise
 */
int security_test_certificate(SSL *ssl) {
    X509* cert = SSL_get_peer_certificate(ssl);
    int ret;
    long status;
    if (!cert) {
        return -1;
    }

    status = SSL_get_verify_result(ssl);
    if((X509_V_OK != status))
    {
        char error[512];
        ERR_error_string_n(ERR_get_error(), error, sizeof(error));
        netdata_log_error("SSL RFC4158 check:  We have a invalid certificate, the tests result with %ld and message %s", status, error);
        ret = -1;
    } else {
        ret = 0;
    }

    return ret;
}

/**
 * Location for context
 *
 * Case the user give us a directory with the certificates available and
 * the Netdata parent certificate, we use this function to validate the certificate.
 *
 * @param ctx the context where the path will be set.
 * @param file the file with Netdata parent certificate.
 * @param path the directory where the certificates are stored.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
int ssl_security_location_for_context(SSL_CTX *ctx, char *file, char *path) {
    int load_custom = 1, load_default = 1;
    if (file || path) {
        if(!SSL_CTX_load_verify_locations(ctx, file, path)) {
            netdata_log_info("Netdata can not verify custom CAfile or CApath for parent's SSL certificate, so it will use the default OpenSSL configuration to validate certificates!");
            load_custom = 0;
        }
    }

    if(!SSL_CTX_set_default_verify_paths(ctx)) {
        netdata_log_info("Can not verify default OpenSSL configuration to validate certificates!");
        load_default = 0;
    }

    if (load_custom  == 0 && load_default == 0)
        return -1;

    return 0;
}
#endif
