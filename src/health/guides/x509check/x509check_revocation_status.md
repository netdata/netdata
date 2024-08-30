### Understand the alert

This alert indicates that the X.509 certificate has been revoked, meaning that it is no longer valid or trusted. The certificate can be revoked for various reasons, such as key compromise, errors within the certificate, change of usage, or the certificate owner no longer being deemed trustworthy.

### Troubleshoot the alert

1. **Identify the affected certificate**: The alert should provide information about the affected X.509 certificate. Take note of the certificate's details, such as the domain name, subject, issuer, and serial number.

2. **Verify the revocation status**: You can use the `openssl` command to verify the revocation status of the affected certificate. Use the following command to check the certificate against the Certificate Revocation List (CRL) provided by the CA:

   ```
   openssl verify -crl_check -CAfile CA_certificate.pem -CRLfile CRL.pem certificate.pem
   ```

   Replace `CA_certificate.pem`, `CRL.pem`, and `certificate.pem` with the appropriate file names of the CA certificate, CRL file, and the target X.509 certificate.

   Alternatively, you can use online tools such as [SSL Shopper's SSL Checker](https://www.sslshopper.com/ssl-checker.html) to verify the revocation status. Be sure to input the domain and port associated with the revoked certificate.

3. **Remove or replace the revoked certificate**: If you have confirmed that the certificate is indeed revoked, you should stop using it immediately. Remove the revoked certificate from your server or application, and replace it with a valid one.

   - If the certificate was issued by a commercial CA, you can request a new certificate from the CA. The CA might provide you with a free replacement or require you to purchase a new one.
   - If the certificate was issued by [Let's Encrypt](https://letsencrypt.org/), you can renew the certificate using [Certbot](https://certbot.eff.org/) or another ACME client.
   - If the certificate was self-signed, you can create a new self-signed certificate using the `openssl` command or another certificate management tool.

4. **Update server or application configuration**: After obtaining a new certificate, update your server or application configuration to use the new certificate. Make sure to restart the server or application for the changes to take effect.

5. **Monitor the new certificate**: Keep an eye on the new certificate's status using the X.509 monitoring tools provided by Netdata. Regularly check for any new alerts or changes in the certificate's status.

### Useful resources

1. [SSL Shopper's SSL Checker](https://www.sslshopper.com/ssl-checker.html)
2. [Renewing certificates with Certbot](https://certbot.eff.org/docs/using.html#renewing-certificates)
3. [Creating a Self-Signed SSL Certificate](https://www.akadia.com/services/ssh_test_certificate.html)