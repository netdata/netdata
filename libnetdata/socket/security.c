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

bool netdata_ssl_open(struct netdata_ssl *ssl, SSL_CTX *ctx, int fd) {
    if(ssl->conn) {
        if(!ctx || SSL_get_SSL_CTX(ssl->conn) != ctx) {
            SSL_free(ssl->conn);
            ssl->conn = NULL;
        }
        else if (SSL_clear(ssl->conn) == 0) {
            netdata_ssl_log_error_queue("SSL_clear", ssl);
            SSL_free(ssl->conn);
            ssl->conn = NULL;
        }
    }

    if(!ssl->conn) {
        if(!ctx) {
            internal_error(true, "SSL: not CTX given");
            ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
            return false;
        }

        ssl->conn = SSL_new(ctx);
        if (!ssl->conn) {
            netdata_ssl_log_error_queue("SSL_new", ssl);
            ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
            return false;
        }
    }

    if(SSL_set_fd(ssl->conn, fd) != 1) {
        netdata_ssl_log_error_queue("SSL_set_fd", ssl);
        ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
        return false;
    }

    ssl->flags = NETDATA_SSL_OPEN;

    ERR_clear_error();

    return true;
}

void netdata_ssl_close(struct netdata_ssl *ssl) {
    if(ssl->conn) {
        if(SSL_connection(ssl)) {
            int ret = SSL_shutdown(ssl->conn);
            if(ret == 0)
                SSL_shutdown(ssl->conn);
        }

        SSL_free(ssl->conn);
    }

    ERR_clear_error();

    *ssl = NETDATA_SSL_UNSET_CONNECTION;
}

void netdata_ssl_log_error_queue(const char *call, struct netdata_ssl *ssl) {
    error_limit_static_thread_var(erl, 1, 0);
    unsigned long err;
    while((err = ERR_get_error())) {
        char *code;

        switch (err) {
            case SSL_ERROR_NONE:
                code = "SSL_ERROR_NONE";
                break;

            case SSL_ERROR_SSL:
                code = "SSL_ERROR_SSL";
                ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
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
                ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
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
        error_limit(&erl, "SSL: %s() returned error %lu (%s): %s", call, err, code, str);
    }
}

ssize_t netdata_ssl_read(struct netdata_ssl *ssl, void *buf, size_t num) {
    if(unlikely(!ssl->conn || ssl->flags != NETDATA_SSL_HANDSHAKE_COMPLETE)) {
        internal_error(true, "SSL: trying to read from an invalid SSL connection");
        return -1;
    }

    errno = 0;

    int bytes, err;

    bytes = SSL_read(ssl->conn, buf, (int)num);

    if(unlikely(bytes <= 0)) {
        err = SSL_get_error(ssl->conn, bytes);
        netdata_ssl_log_error_queue("SSL_read", ssl);
        if (err == SSL_ERROR_WANT_READ || err == SSL_ERROR_WANT_WRITE)
            bytes = 0;
    }

    return bytes;
}

ssize_t netdata_ssl_write(struct netdata_ssl *ssl, const void *buf, size_t num) {
    if(unlikely(!ssl->conn || ssl->flags != NETDATA_SSL_HANDSHAKE_COMPLETE)) {
        internal_error(true, "SSL: trying to write to an invalid SSL connection");
        return -1;
    }

    errno = 0;

    int bytes, err;

    bytes = SSL_write(ssl->conn, (uint8_t *)buf, (int)num);

    if(unlikely(bytes <= 0)) {
        err = SSL_get_error(ssl->conn, bytes);
        netdata_ssl_log_error_queue("SSL_write", ssl);
        if (err == SSL_ERROR_WANT_READ || err == SSL_ERROR_WANT_WRITE)
            bytes = 0;
    }

    return bytes;
}

bool netdata_ssl_connect(struct netdata_ssl *ssl) {
    if(unlikely(!ssl->conn || ssl->flags != NETDATA_SSL_OPEN)) {
        internal_error(true, "SSL: trying to connect using an invalid SSL structure");
        return false;
    }

    SSL_set_connect_state(ssl->conn);

    int err = SSL_connect(ssl->conn);
    if (err != 1) {
        int ssl_errno = SSL_get_error(ssl->conn, err);
        netdata_ssl_log_error_queue("SSL_connect", ssl);
        switch(ssl_errno) {
            case SSL_ERROR_WANT_READ:
            case SSL_ERROR_WANT_WRITE:
                ssl->flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
                return true;

            default:
                ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
                return false;
        }
    }

    ssl->flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
    return true;
}

bool netdata_ssl_accept(struct netdata_ssl *ssl) {
    if(unlikely(!ssl->conn || ssl->flags != NETDATA_SSL_OPEN)) {
        internal_error(true, "SSL: trying to accept a connection using an invalid SSL structure");
        return false;
    }

    SSL_set_accept_state(ssl->conn);

    int err;
    if ((err = SSL_accept(ssl->conn)) <= 0) {
        int ssl_errno = SSL_get_error(ssl->conn, err);
        netdata_ssl_log_error_queue("SSL_accept", ssl);
        switch(ssl_errno) {
            case SSL_ERROR_WANT_READ:
            case SSL_ERROR_WANT_WRITE:
                ssl->flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
                return true;

            default:
                ssl->flags = NETDATA_SSL_HANDSHAKE_FAILED;
                return false;
        }
    }

    ssl->flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
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
        debug(D_WEB_CLIENT,"SSL INFO CALLBACK %s %s", SSL_alert_type_string(ret), SSL_alert_desc_string_long(ret));
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
        error("SSL library cannot be initialized.");
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
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_file(ctx, netdata_ssl_security_cert, SSL_FILETYPE_PEM);
#else
    ctx = SSL_CTX_new(TLS_server_method());
    if (!ctx) {
		error("Cannot create a new SSL context, netdata won't encrypt communication");
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
            error("SSL error. cannot set the cipher list");
        }
    }
#endif

    SSL_CTX_use_PrivateKey_file(ctx, netdata_ssl_security_key,SSL_FILETYPE_PEM);

    if (!SSL_CTX_check_private_key(ctx)) {
        ERR_error_string_n(ERR_get_error(),lerror,sizeof(lerror));
		error("SSL cannot check the private key: %s",lerror);
        SSL_CTX_free(ctx);
        return NULL;
    }

	SSL_CTX_set_session_id_context(ctx,(void*)&netdata_id_context,(unsigned int)sizeof(netdata_id_context));
    SSL_CTX_set_info_callback(ctx, netdata_ssl_info_callback);

#if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_095)
	SSL_CTX_set_verify_depth(ctx,1);
#endif
    debug(D_WEB_CLIENT,"SSL GLOBAL CONTEXT STARTED\n");

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
    netdata_spinlock_lock(&sp);

    switch (selector) {
        case NETDATA_SSL_WEB_SERVER_CTX: {
            if(!netdata_ssl_web_server_ctx) {
                struct stat statbuf;
                if (stat(netdata_ssl_security_key, &statbuf) || stat(netdata_ssl_security_cert, &statbuf))
                    info("To use encryption it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
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
                        SSL_MODE_AUTO_RETRY |
                        0
                );

                if(netdata_ssl_streaming_sender_ctx && !netdata_ssl_validate_certificate)
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

    netdata_spinlock_unlock(&sp);
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
        error("SSL RFC4158 check:  We have a invalid certificate, the tests result with %ld and message %s", status, error);
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
            info("Netdata can not verify custom CAfile or CApath for parent's SSL certificate, so it will use the default OpenSSL configuration to validate certificates!");
            load_custom = 0;
        }
    }

    if(!SSL_CTX_set_default_verify_paths(ctx)) {
        info("Can not verify default OpenSSL configuration to validate certificates!");
        load_default = 0;
    }

    if (load_custom  == 0 && load_default == 0)
        return -1;

    return 0;
}
#endif
