#ifndef NETDATA_SECURITY_H
#define NETDATA_SECURITY_H

#define NETDATA_SSL_HANDSHAKE_COMPLETE 0    //All the steps were successful
#define NETDATA_SSL_START 1                 //Starting handshake, conn variable is NULL
#define NETDATA_SSL_WANT_READ 2             //The connection wanna read from socket
#define NETDATA_SSL_WANT_WRITE 4            //The connection wanna write on socket
#define NETDATA_SSL_NO_HANDSHAKE 8          //Continue without encrypt connection.
#define NETDATA_SSL_OPTIONAL 16             //Flag to define the HTTP request
#define NETDATA_SSL_FORCE 32                //We only accepts HTTPS request
#define NETDATA_SSL_INVALID_CERTIFICATE 64  //Accepts invalid certificate
#define NETDATA_SSL_VALID_CERTIFICATE 128  //Accepts invalid certificate
#define NETDATA_SSL_PROXY_HTTPS 256        //Proxy is using HTTPS

#define NETDATA_SSL_CONTEXT_SERVER 0
#define NETDATA_SSL_CONTEXT_STREAMING 1
#define NETDATA_SSL_CONTEXT_EXPORTING 2

#ifdef ENABLE_HTTPS

#ifdef NETDATA_HTTPS_WITH_OPENSSL
#define OPENSSL_VERSION_095 0x00905100L
#define OPENSSL_VERSION_097 0x0907000L
#define OPENSSL_VERSION_110 0x10100000L
#define OPENSSL_VERSION_111 0x10101000L

#include <openssl/ssl.h>
#include <openssl/err.h>
#if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097) && (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
#include <openssl/conf.h>
#endif
#endif

#ifdef  NETDATA_HTTPS_WITH_WOLFSSL
#include <wolfssl/options.h>
#include <wolfssl/ssl.h>
#include <wolfssl/openssl/ssl.h>
#endif

struct netdata_ssl{
    SSL *conn; //SSL connection
    uint32_t flags; //The flags for SSL connection
};

extern SSL_CTX *netdata_exporting_ctx;
extern SSL_CTX *netdata_client_ctx;
extern SSL_CTX *netdata_srv_ctx;
extern const char *security_key;
extern const char *security_cert;
extern const char *tls_version;
extern const char *tls_ciphers;
extern int netdata_validate_server;
extern int security_location_for_context(SSL_CTX *ctx, char *file, char *path);

#ifdef NETDATA_HTTPS_WITH_OPENSSL
void security_openssl_library();
SSL_CTX * security_initialize_openssl_client();
#endif

#ifdef  NETDATA_HTTPS_WITH_WOLFSSL
void security_wolfssl_library();
#endif

void security_start_ssl(int selector);
void security_clean_ssl();
int security_process_accept(SSL *ssl, int msg);
int security_test_certificate(SSL *ssl);

#endif //ENABLE_HTTPS
#endif //NETDATA_SECURITY_H
