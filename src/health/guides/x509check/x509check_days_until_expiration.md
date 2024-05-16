### Understand the alert

This alert indicates that your X.509 certificate will expire soon. By default, it is triggered in a warning state when your certificate has less than 14 days to expire and in a critical state when it has less than 7 days to expire. However, these levels are configurable.

An X.509 certificate is a digital certificate used to manage identity and security in internet communications and computer networking. If your certificate expires, your system may encounter security and authentication issues which can disrupt your services.

### Troubleshoot the alert

**Step 1: Check the certificate's expiration details**

To check the details of your X.509 certificate, including its expiration date, run the following command:

```
openssl x509 -in path/to/your/certificate.crt -text -noout
```

Replace `path/to/your/certificate.crt` with the path to your X.509 certificate file.

**Step 2: Renew or re-key the certificate**

If your X.509 certificate is issued by a Certification Authority (CA), you need to renew or re-key the certificate before it expires. The process for renewing or re-keying your certificate depends on your CA. Refer to your CA's documentation or help resources for guidance.

Examples of popular CAs include:

1. [Let's Encrypt](https://letsencrypt.org/)
2. [Symantec](https://securitycloud.symantec.com/cc/landing)
3. [GeoTrust](https://www.geotrust.com/)
4. [Sectigo](https://sectigo.com/)
5. [DigiCert](https://www.digicert.com/)

**Step 3: Update your system with the new certificate**

After renewing or re-keying your certificate, you need to update your system with the new certificate file. The process for updating your system depends on the services and platforms you are using. Refer to their documentation for guidance on how to update your certificate.

**Step 4: Verify the new certificate**

Ensure that your system is running with the updated certificate by checking its details again, as described in Step 1.

If there are still issues or the alert persists, double-check your certificate management process and consult your CA's documentation for any additional help or support.

### Useful resources

1. [Sectigo: What is an X.509 certificate?](https://sectigo.com/resource-library/what-is-x509-certificate)
2. [OpenSSL: X.509 Certificate Commands](https://www.openssl.org/docs/man1.1.1/man1/x509.html)