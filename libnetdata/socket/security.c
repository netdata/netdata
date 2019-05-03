#include "../libnetdata.h"

SSL_CTX *netdata_ctx=NULL;
const char *security_key=NULL;
const char *security_cert=NULL;

static SSL_CTX * security_initialize_openssl(){
    SSL_CTX *ctx;
    char error[512];

    //TO DO: Confirm the necessity to check return for other OPENSSL function
#if OPENSSL_VERSION_NUMBER < 0x10100000L
# if (SSLEAY_VERSION_NUMBER >= 0x0907000L)
    OPENSSL_config(NULL);
# endif
    SSL_library_init();

# if OPENSSL_API_COMPAT < 0x10100000L
    SSL_load_error_strings();
# endif

    if ( !(ctx = SSL_CTX_new(SSLv23_server_method()) ) ){
        return NULL;
    }

    SSL_CTX_set_options(ctx,SSL_OP_NO_SSLv2|SSL_OP_NO_SSLv3);

    SSL_CTX_use_certificate_file(ctx, security_cert, SSL_FILETYPE_PEM);
#else
    OPENSSL_init_ssl(OPENSSL_INIT_LOAD_CONFIG,NULL);
    if ( !(ctx = SSL_CTX_new(TLS_server_method()) ) ){
        return NULL;
    }

    /* What is the minimum version?
    SSL_CTX_set_min_proto_version(ctx,TLS1_1_VERSION);
     */
    SSL_CTX_set_min_proto_version(ctx,SSL3_VERSION);

    SSL_CTX_use_certificate_chain_file(ctx, security_cert );
#endif

    SSL_CTX_use_PrivateKey_file(ctx, security_key, SSL_FILETYPE_PEM);
    if ( !SSL_CTX_check_private_key(ctx) ){
        ERR_error_string_n(ERR_get_error(),error,sizeof(error));
        fprintf(stderr,"Check private key: %s\n",security_key,security_cert,error);
        SSL_CTX_free(ctx);
        return NULL;
    }

    SSL_CTX_set_session_cache_mode(ctx,SSL_SESS_CACHE_NO_AUTO_CLEAR|SSL_SESS_CACHE_SERVER);

    return ctx;
}

void security_start_ssl(){
    struct stat statbuf;
    if ( (stat(security_key,&statbuf)) || (stat(security_cert,&statbuf)) ){
        fprintf(stdout,"To use SSL it is necessary to set \"ssl certificate\" and \"ssl key\" in [web] !\n");
        return;
    }

    netdata_ctx =  security_initialize_openssl();
}

void security_clean_openssl(){
#if OPENSSL_VERSION_NUMBER < 0x10100000L
    ERR_free_strings();
#endif
}
