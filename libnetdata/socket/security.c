#include "../libnetdata.h"

SSL_CTX *netdata_ctx=NULL;
const char *security_key=NULL;
const char *security_cert=NULL;

static void security_info_callback(const SSL *ssl, int where, int ret) {
    (void)ssl;
    if ( where & SSL_CB_ALERT ) {
        debug(D_WEB_CLIENT,"SSL INFO CALLBACK %s %s",SSL_alert_type_string( ret ),SSL_alert_desc_string_long(ret));
    }
}

static SSL_CTX * security_initialize_openssl(){
    SSL_CTX *ctx;
    char error[512];
	static int netdata_id_context = 1;

    //TO DO: Confirm the necessity to check return for other OPENSSL function
#if OPENSSL_VERSION_NUMBER < 0x10100000L
# if (SSLEAY_VERSION_NUMBER >= 0x0907000L)
    OPENSSL_config(NULL);
# endif
    SSL_library_init();

# if OPENSSL_API_COMPAT < 0x10100000L
    SSL_load_error_strings();
# endif

	ctx = SSL_CTX_new(SSLv23_server_method());
    if ( !ctx ) {
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_set_options(ctx,SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3);

    SSL_CTX_use_certificate_file(ctx, security_cert, SSL_FILETYPE_PEM);
#else
    OPENSSL_init_ssl(OPENSSL_INIT_LOAD_CONFIG,NULL);
    ctx = SSL_CTX_new(TLS_server_method());
    if ( !ctx ){
		error("Cannot create a new SSL context, netdata won't encrypt communication");
        return NULL;
    }

    SSL_CTX_set_min_proto_version(ctx,TLS1_VERSION);
    //SET OTHER PROTOCOL VERSIONS(TLS1_2_VERSION)

    SSL_CTX_use_certificate_chain_file(ctx, security_cert );
#endif

    SSL_CTX_use_PrivateKey_file(ctx, security_key, SSL_FILETYPE_PEM);
    if ( !SSL_CTX_check_private_key(ctx) ){
        ERR_error_string_n(ERR_get_error(),error,sizeof(error));
		error("Private key check error: %s",error);
        SSL_CTX_free(ctx);
        return NULL;
    }

    SSL_CTX_set_session_cache_mode(ctx,SSL_SESS_CACHE_NO_AUTO_CLEAR|SSL_SESS_CACHE_SERVER);
	SSL_CTX_set_session_id_context(ctx,(void*)&netdata_id_context,(unsigned int)sizeof(netdata_id_context));
    SSL_CTX_set_info_callback(ctx,security_info_callback);

#if (OPENSSL_VERSION_NUMBER < 0x00905100L)
	SSL_CTX_set_verify_depth(ctx,1);
#endif
    debug(D_WEB_CLIENT,"SSL GLOBAL CONTEXT STARTED\n");

    return ctx;
}

void security_start_ssl(){
    struct stat statbuf;
    if ( (stat(security_key,&statbuf)) || (stat(security_cert,&statbuf)) ){
        info("To use encryptation it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
        return;
    }

    netdata_ctx =  security_initialize_openssl();
}

void security_clean_openssl(){
	if ( netdata_ctx )
	{
		SSL_CTX_free(netdata_ctx);
	}

#if OPENSSL_VERSION_NUMBER < 0x10100000L
    ERR_free_strings();
#endif
}

int security_process_accept(SSL *ssl,int sock) {
    sock_delnonblock(sock);
    usec_t end = now_realtime_usec() + 600000;
    usec_t curr;
    int test;
    while (!SSL_is_init_finished(ssl)) {
        ERR_clear_error();
        if ((test = SSL_do_handshake(ssl)) <= 0) {
            int sslerrno = SSL_get_error(ssl, test);
            curr = now_realtime_sec();
            switch(sslerrno) {
                case SSL_ERROR_WANT_READ:
                {
                    if ( curr >=  end)
                    {
                        debug(D_WEB_CLIENT_ACCESS,"SSL Handshake wanna read on socket fd %d : %s ",sock,ERR_error_string((long)SSL_get_error(ssl,test),NULL));
                        return 1;
                    }
                    break;
                }
                case SSL_ERROR_WANT_WRITE:
                {
                    if ( curr >=  end)
                    {
                        debug(D_WEB_CLIENT_ACCESS,"SSL Handshake wanna write on socket fd %d : %s ",sock,ERR_error_string((long)SSL_get_error(ssl,test),NULL));
                        return 2;
                    }
                    break;
                }
				case SSL_ERROR_SSL:
				{
					debug(D_WEB_CLIENT_ACCESS,"There is not a SSL request on socket %d : %s ",sock,ERR_error_string((long)SSL_get_error(ssl,test),NULL));
                	return 3;
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
    }

    sock_setnonblock(sock);

    if ( SSL_is_init_finished(ssl) )
    {
        debug(D_WEB_CLIENT_ACCESS,"SSL Handshake finished %s errno %d on socket fd %d",ERR_error_string((long)SSL_get_error(ssl,test),NULL),errno,sock);
    }

    return 0;
}
