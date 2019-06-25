#include "../libnetdata.h"

#ifdef ENABLE_HTTPS

SSL_CTX *netdata_client_ctx=NULL;
SSL_CTX *netdata_srv_ctx=NULL;
const char *security_key=NULL;
const char *security_cert=NULL;
int netdata_use_ssl_on_stream = NETDATA_SSL_OPTIONAL;
int netdata_use_ssl_on_http = NETDATA_SSL_FORCE; //We force SSL due safety reasons
int netdata_validate_server =  NETDATA_SSL_VALID_CERTIFICATE;

static void security_info_callback(const SSL *ssl, int where, int ret) {
    (void)ssl;
    if (where & SSL_CB_ALERT) {
        debug(D_WEB_CLIENT,"SSL INFO CALLBACK %s %s", SSL_alert_type_string(ret), SSL_alert_desc_string_long(ret));
    }
}

void security_openssl_library()
{
#if OPENSSL_VERSION_NUMBER < 0x10100000L
# if (SSLEAY_VERSION_NUMBER >= 0x0907000L)
    OPENSSL_config(NULL);
# endif

# if OPENSSL_API_COMPAT < 0x10100000L
    SSL_load_error_strings();
# endif

    SSL_library_init();
#else
    if (OPENSSL_init_ssl(OPENSSL_INIT_LOAD_CONFIG, NULL) != 1) {
        error("SSL library cannot be initialized.");
    }
#endif
}

void security_openssl_common_options(SSL_CTX *ctx) {
#if OPENSSL_VERSION_NUMBER >= 0x10100000L
    static char *ciphers = {"ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-SHA:!aNULL:!eNULL:!EXPORT:!DES:!RC4:!MD5:!PSK:!aECDH:!EDH-DSS-DES-CBC3-SHA:!EDH-RSA-DES-CBC3-SHA:!KRB5-DES-CBC3-SHA"};
#endif
#if OPENSSL_VERSION_NUMBER < 0x10100000L
    SSL_CTX_set_options (ctx,SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3|SSL_OP_NO_COMPRESSION);
#else
    SSL_CTX_set_min_proto_version(ctx, TLS1_2_VERSION);
    //We are avoiding the TLS v1.3 for while, because Google Chrome
    //is giving the message net::ERR_SSL_VERSION_INTERFERENCE with it.
    SSL_CTX_set_max_proto_version(ctx, TLS1_2_VERSION);
#endif
    SSL_CTX_set_mode(ctx, SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER);

#if OPENSSL_VERSION_NUMBER >= 0x10100000L
    if (!SSL_CTX_set_cipher_list(ctx, ciphers)) {
        error("SSL error. cannot set the cipher list");
    }
#endif


}

static SSL_CTX * security_initialize_openssl_client() {
    SSL_CTX *ctx;
#if OPENSSL_VERSION_NUMBER < 0x10100000L
    ctx = SSL_CTX_new(SSLv23_client_method());
#else
    ctx = SSL_CTX_new(TLS_client_method());
#endif
    security_openssl_common_options(ctx);

    return ctx;
}

static SSL_CTX * security_initialize_openssl_server() {
    SSL_CTX *ctx;
    char lerror[512];
	static int netdata_id_context = 1;

    //TO DO: Confirm the necessity to check return for other OPENSSL function
#if OPENSSL_VERSION_NUMBER < 0x10100000L
	ctx = SSL_CTX_new(SSLv23_server_method());
    if (!ctx) {
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_file(ctx, security_cert, SSL_FILETYPE_PEM);
#else
    ctx = SSL_CTX_new(TLS_server_method());
    if (!ctx) {
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_chain_file(ctx, security_cert);
#endif
    security_openssl_common_options(ctx);

    SSL_CTX_use_PrivateKey_file(ctx,security_key,SSL_FILETYPE_PEM);

    if (!SSL_CTX_check_private_key(ctx)) {
        ERR_error_string_n(ERR_get_error(),lerror,sizeof(lerror));
		error("SSL cannot check the private key: %s",lerror);
        SSL_CTX_free(ctx);
        return NULL;
    }

	SSL_CTX_set_session_id_context(ctx,(void*)&netdata_id_context,(unsigned int)sizeof(netdata_id_context));
    SSL_CTX_set_info_callback(ctx,security_info_callback);

#if (OPENSSL_VERSION_NUMBER < 0x00905100L)
	SSL_CTX_set_verify_depth(ctx,1);
#endif
    debug(D_WEB_CLIENT,"SSL GLOBAL CONTEXT STARTED\n");

    return ctx;
}

void security_start_ssl(int type) {
    if (!type) {
        struct stat statbuf;
        if (stat(security_key,&statbuf) || stat(security_cert,&statbuf)) {
            info("To use encryption it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
            return;
        }

        netdata_srv_ctx =  security_initialize_openssl_server();
    }
    else {
        netdata_client_ctx =  security_initialize_openssl_client();
    }
}

void security_clean_openssl() {
	if (netdata_srv_ctx)
	{
		SSL_CTX_free(netdata_srv_ctx);
	}

    if (netdata_client_ctx)
    {
        SSL_CTX_free(netdata_client_ctx);
    }

#if OPENSSL_VERSION_NUMBER < 0x10100000L
    ERR_free_strings();
#endif
}

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
                     info("%d SSL Handshake error (%s) on socket %d ", counter++, ERR_error_string((long)SSL_get_error(ssl, test), NULL), sock);
			     }
                 return NETDATA_SSL_NO_HANDSHAKE;
			 }
         }
    }

    if (SSL_is_init_finished(ssl))
    {
        debug(D_WEB_CLIENT_ACCESS,"SSL Handshake finished %s errno %d on socket fd %d", ERR_error_string((long)SSL_get_error(ssl, test), NULL), errno, sock);
    }

    return 0;
}

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

#endif
