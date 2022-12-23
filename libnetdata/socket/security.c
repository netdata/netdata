#include "../libnetdata.h"

#ifdef ENABLE_HTTPS

SSL_CTX *netdata_ssl_exporting_ctx =NULL;
SSL_CTX *netdata_ssl_client_ctx =NULL;
SSL_CTX *netdata_ssl_srv_ctx =NULL;
const char *netdata_ssl_security_key =NULL;
const char *netdata_ssl_security_cert =NULL;
const char *tls_version=NULL;
const char *tls_ciphers=NULL;
int netdata_ssl_validate_server =  NETDATA_SSL_VALID_CERTIFICATE;

/**
 * Info Callback
 *
 * Function used as callback for the OpenSSL Library
 *
 * @param ssl a pointer to the SSL structure of the client
 * @param where the variable with the flags set.
 * @param ret the return of the caller
 */
static void security_info_callback(const SSL *ssl, int where, int ret __maybe_unused) {
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
void security_openssl_library()
{
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
int tls_select_version(const char *lversion) {
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
 * OpenSSL common options
 *
 * Clients and SERVER have common options, this function is responsible to set them in the context.
 *
 * @param ctx the initialized SSL context.
 * @param side 0 means server, and 1 client.
 */
void security_openssl_common_options(SSL_CTX *ctx, int side) {
#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_110
    if (!side) {
        int version =  tls_select_version(tls_version) ;
#endif
#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
        SSL_CTX_set_options (ctx,SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3|SSL_OP_NO_COMPRESSION);
#else
        SSL_CTX_set_min_proto_version(ctx, TLS1_VERSION);
        SSL_CTX_set_max_proto_version(ctx, version);

        if(tls_ciphers  && strcmp(tls_ciphers, "none") != 0) {
            if (!SSL_CTX_set_cipher_list(ctx, tls_ciphers)) {
                error("SSL error. cannot set the cipher list");
            }
        }
    }
#endif

    SSL_CTX_set_mode(ctx, SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER);
}

/**
 * Initialize Openssl Client
 *
 * Starts the client context with TLS 1.2.
 *
 * @return It returns the context on success or NULL otherwise
 */
SSL_CTX * security_initialize_openssl_client() {
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

    return ctx;
}

/**
 * Initialize OpenSSL server
 *
 * Starts the server context with TLS 1.2 and load the certificate.
 *
 * @return It returns the context on success or NULL otherwise
 */
static SSL_CTX * security_initialize_openssl_server() {
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
    security_openssl_common_options(ctx, 0);

    SSL_CTX_use_PrivateKey_file(ctx, netdata_ssl_security_key,SSL_FILETYPE_PEM);

    if (!SSL_CTX_check_private_key(ctx)) {
        ERR_error_string_n(ERR_get_error(),lerror,sizeof(lerror));
		error("SSL cannot check the private key: %s",lerror);
        SSL_CTX_free(ctx);
        return NULL;
    }

	SSL_CTX_set_session_id_context(ctx,(void*)&netdata_id_context,(unsigned int)sizeof(netdata_id_context));
    SSL_CTX_set_info_callback(ctx,security_info_callback);

#if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_095)
	SSL_CTX_set_verify_depth(ctx,1);
#endif
    debug(D_WEB_CLIENT,"SSL GLOBAL CONTEXT STARTED\n");

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
void security_start_ssl(int selector) {
    static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
    netdata_spinlock_lock(&sp);

    switch (selector) {
        case NETDATA_SSL_CONTEXT_SERVER: {
            if(!netdata_ssl_srv_ctx) {
                struct stat statbuf;
                if (stat(netdata_ssl_security_key, &statbuf) || stat(netdata_ssl_security_cert, &statbuf))
                    info("To use encryption it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
                else {
                    netdata_ssl_srv_ctx = security_initialize_openssl_server();
                    SSL_CTX_set_mode(netdata_ssl_srv_ctx, SSL_MODE_ENABLE_PARTIAL_WRITE);
                }
            }
            break;
        }

        case NETDATA_SSL_CONTEXT_STREAMING: {
            if(!netdata_ssl_client_ctx) {
                netdata_ssl_client_ctx = security_initialize_openssl_client();
                //This is necessary for the stream, because it is working sometimes with nonblock socket.
                //It returns the bitmask after to change, there is not any description of errors in the documentation
                SSL_CTX_set_mode(netdata_ssl_client_ctx,
                                 SSL_MODE_ENABLE_PARTIAL_WRITE | SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER |
                                 SSL_MODE_AUTO_RETRY);
            }
            break;
        }

        case NETDATA_SSL_CONTEXT_EXPORTING: {
            if(!netdata_ssl_exporting_ctx)
                netdata_ssl_exporting_ctx = security_initialize_openssl_client();
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
void security_clean_openssl()
{
    if (netdata_ssl_srv_ctx) {
        SSL_CTX_free(netdata_ssl_srv_ctx);
    }

    if (netdata_ssl_client_ctx) {
        SSL_CTX_free(netdata_ssl_client_ctx);
    }

    if (netdata_ssl_exporting_ctx) {
        SSL_CTX_free(netdata_ssl_exporting_ctx);
    }

#if OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
    ERR_free_strings();
#endif
}

/**
 * Process accept
 *
 * Process the SSL handshake with the client case it is necessary.
 *
 * @param ssl is a pointer for the SSL structure
 * @param msg is a copy of the first 8 bytes of the initial message received
 *
 * @return it returns 0 case it performs the handshake, 8 case it is clean connection
 *  and another integer power of 2 otherwise.
 */
int security_process_accept(SSL *ssl,int msg) {
    int sock = SSL_get_fd(ssl);
    int test;
    if (msg > 0x17)
    {
        return NETDATA_SSL_NO_HANDSHAKE;
    }

    ERR_clear_error();
    if ((test = SSL_accept(ssl)) <= 0) {
         int sslerrno = SSL_get_error(ssl, test);
         switch(sslerrno) {
             case SSL_ERROR_WANT_READ:
             {
                 error("SSL handshake did not finish and it wanna read on socket %d!", sock);
                 return NETDATA_SSL_WANT_READ;
             }
             case SSL_ERROR_WANT_WRITE:
             {
                 error("SSL handshake did not finish and it wanna read on socket %d!", sock);
                 return NETDATA_SSL_WANT_WRITE;
             }
             case SSL_ERROR_NONE:
             case SSL_ERROR_SSL:
             case SSL_ERROR_SYSCALL:
             default:
			 {
                 u_long err;
                 char buf[256];
                 int counter = 0;
                 while ((err = ERR_get_error()) != 0) {
                     ERR_error_string_n(err, buf, sizeof(buf));
                     error("%d SSL Handshake error (%s) on socket %d", counter++, ERR_error_string((long)SSL_get_error(ssl, test), NULL), sock);
			     }
                 return NETDATA_SSL_NO_HANDSHAKE;
			 }
         }
    }

    if (SSL_is_init_finished(ssl))
    {
        debug(D_WEB_CLIENT_ACCESS,"SSL Handshake finished %s errno %d on socket fd %d", ERR_error_string((long)SSL_get_error(ssl, test), NULL), errno, sock);
    }

    return NETDATA_SSL_HANDSHAKE_COMPLETE;
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
