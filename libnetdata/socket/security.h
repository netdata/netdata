#ifndef NETDATA_SECURITY_H
# define NETDATA_SECURITY_H

typedef enum __attribute__((packed)) {
    NETDATA_SSL_STATE_NOT_SSL = 1,  // This connection is not SSL
    NETDATA_SSL_STATE_INIT,         // SSL handshake is initialized
    NETDATA_SSL_STATE_FAILED,       // SSL handshake failed
    NETDATA_SSL_STATE_COMPLETE,     // SSL handshake successful
} NETDATA_SSL_STATE;

#define NETDATA_SSL_WEB_SERVER_CTX 0
#define NETDATA_SSL_STREAMING_SENDER_CTX 1
#define NETDATA_SSL_EXPORTING_CTX 2

# ifdef ENABLE_HTTPS

#define OPENSSL_VERSION_095 0x00905100L
#define OPENSSL_VERSION_097 0x0907000L
#define OPENSSL_VERSION_110 0x10100000L
#define OPENSSL_VERSION_111 0x10101000L
#define OPENSSL_VERSION_300 0x30000000L

#  include <openssl/ssl.h>
#  include <openssl/err.h>
#  include <openssl/evp.h>
#  include <openssl/pem.h>
#  if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097) && (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
#   include <openssl/conf.h>
#  endif

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
#include <openssl/core_names.h>
#include <openssl/decoder.h>
#endif

typedef struct netdata_ssl {
    SSL *conn;               // SSL connection
    NETDATA_SSL_STATE state; // The state for SSL connection
    unsigned long ssl_errno; // The SSL errno of the last SSL call
} NETDATA_SSL;

#define NETDATA_SSL_UNSET_CONNECTION (NETDATA_SSL){ .conn = NULL, .state = NETDATA_SSL_STATE_NOT_SSL }

#define SSL_connection(ssl) ((ssl)->conn && (ssl)->state != NETDATA_SSL_STATE_NOT_SSL)

extern SSL_CTX *netdata_ssl_exporting_ctx;
extern SSL_CTX *netdata_ssl_streaming_sender_ctx;
extern SSL_CTX *netdata_ssl_web_server_ctx;
extern const char *netdata_ssl_security_key;
extern const char *netdata_ssl_security_cert;
extern const char *tls_version;
extern const char *tls_ciphers;
extern bool netdata_ssl_validate_certificate;
extern bool netdata_ssl_validate_certificate_sender;
int ssl_security_location_for_context(SSL_CTX *ctx,char *file,char *path);

void netdata_ssl_initialize_openssl();
void netdata_ssl_cleanup();
void netdata_ssl_initialize_ctx(int selector);
int security_test_certificate(SSL *ssl);
SSL_CTX * netdata_ssl_create_client_ctx(unsigned long mode);

bool netdata_ssl_connect(NETDATA_SSL *ssl);
bool netdata_ssl_accept(NETDATA_SSL *ssl);

bool netdata_ssl_open(NETDATA_SSL *ssl, SSL_CTX *ctx, int fd);
void netdata_ssl_close(NETDATA_SSL *ssl);

ssize_t netdata_ssl_read(NETDATA_SSL *ssl, void *buf, size_t num);
ssize_t netdata_ssl_write(NETDATA_SSL *ssl, const void *buf, size_t num);

# endif //ENABLE_HTTPS
#endif //NETDATA_SECURITY_H
