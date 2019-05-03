#ifndef NETDATA_SECURITY_H
# define NETDATA_SECURITY_H


# include <openssl/ssl.h>
# include <openssl/err.h>
# if (SSLEAY_VERSION_NUMBER >= 0x0907000L) && (OPENSSL_VERSION_NUMBER < 0x10100000L)
#  include <openssl/conf.h>
# endif

extern SSL_CTX *netdata_ctx;
extern const char *security_key;
extern const char *security_cert;

void security_clean_openssl();
void security_start_ssl();

#endif //NETDATA_SECURITY_H
