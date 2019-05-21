#include "../libnetdata.h"

SSL_CTX *netdata_cli_ctx=NULL;
SSL_CTX *netdata_srv_ctx=NULL;
const char *security_key=NULL;
const char *security_cert=NULL;
int netdata_use_ssl_on_stream = NETDATA_SSL_OPTIONAL;
int netdata_use_ssl_on_http = NETDATA_SSL_FORCE; //We force SSL due safety reasons

static void security_info_callback(const SSL *ssl, int where, int ret) {
    (void)ssl;
    if ( where & SSL_CB_ALERT ) {
        debug(D_WEB_CLIENT,"SSL INFO CALLBACK %s %s",SSL_alert_type_string( ret ),SSL_alert_desc_string_long(ret));
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
    if ( OPENSSL_init_ssl(OPENSSL_INIT_LOAD_CONFIG,NULL) != 1 ){
        error("SSL library cannot be initialized.");
    }
#endif
}

void security_openssl_common_options(SSL_CTX *ctx){
#if OPENSSL_VERSION_NUMBER < 0x10100000L
    SSL_CTX_set_options (ctx,SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3|SSL_OP_NO_COMPRESSION);
#else
    SSL_CTX_set_min_proto_version(ctx,TLS1_VERSION);
#endif
    SSL_CTX_set_mode(ctx, SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER);
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

static SSL_CTX * security_initialize_openssl_server(){
    SSL_CTX *ctx;
    char lerror[512];
	static int netdata_id_context = 1;

    //TO DO: Confirm the necessity to check return for other OPENSSL function
#if OPENSSL_VERSION_NUMBER < 0x10100000L
	ctx = SSL_CTX_new(SSLv23_server_method());
    if ( !ctx ) {
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_file(ctx, security_cert, SSL_FILETYPE_PEM);
#else
    ctx = SSL_CTX_new(TLS_server_method());
    if ( !ctx ){
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_use_certificate_chain_file(ctx, security_cert );
#endif
    security_openssl_common_options(ctx);

    SSL_CTX_use_PrivateKey_file(ctx, security_key, SSL_FILETYPE_PEM);
    if ( !SSL_CTX_check_private_key(ctx) ){
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

void security_start_ssl(int type){
    if ( !type){
        struct stat statbuf;
        if ( (stat(security_key,&statbuf)) || (stat(security_cert,&statbuf)) ){
            info("To use encryption it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
            return;
        }

        netdata_srv_ctx =  security_initialize_openssl_server();
    }
    else {
        netdata_cli_ctx =  security_initialize_openssl_client();
    }
}

void security_clean_openssl(){
	if ( netdata_srv_ctx )
	{
		SSL_CTX_free(netdata_srv_ctx);
	}

    if ( netdata_cli_ctx )
    {
        SSL_CTX_free(netdata_cli_ctx);
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
                 error("SSL handshake did not finish and it wanna read on socket %d!",sock);
                 return NETDATA_SSL_WANT_READ;
             }
             case SSL_ERROR_WANT_WRITE:
             {
                 error("SSL handshake did not finish and it wanna read on socket %d!",sock);
                 return NETDATA_SSL_WANT_WRITE;
             }
             case SSL_ERROR_SSL:
			 {
				debug(D_WEB_CLIENT_ACCESS,"There is not a SSL request on socket %d : %s ",sock,ERR_error_string((long)SSL_get_error(ssl,test),NULL));
                return NETDATA_SSL_NO_HANDSHAKE;
			 }
             case SSL_ERROR_NONE:
             {
                debug(D_WEB_CLIENT_ACCESS,"SSL Handshake ERROR_NONE on socket fd %d : %s ",sock,ERR_error_string((long)SSL_get_error(ssl,test),NULL));
                 break;
             }
             case SSL_ERROR_SYSCALL:
             default:
             {
                    debug(D_WEB_CLIENT_ACCESS,"SSL Handshake ERROR_SYSCALL on socket fd %d : %s ",sock,ERR_error_string((long)SSL_get_error(ssl,test),NULL));
                    break;
             }
          }
      }

    if ( SSL_is_init_finished(ssl) )
    {
        debug(D_WEB_CLIENT_ACCESS,"SSL Handshake finished %s errno %d on socket fd %d",ERR_error_string((long)SSL_get_error(ssl,test),NULL),errno,sock);
    }

    return 0;
}
