# x509check_days_until_expiration

**Certificates | x509 certificates**

_An X.509 certificate is a digital certificate based on the widely accepted International
Telecommunications Union (ITU) X.509 standard, which defines the format of public key
infrastructure (PKI) certificates. They are used to manage identity and security in internet
communications and computer networking. They are unobtrusive and ubiquitous, and we encounter them
every day when using websites, mobile apps, online documents, and connected
devices._ <sup>[1](https://sectigo.com/resource-library/what-is-x509-certificate#:~:text=Share%20this-,An%20X.,internet%20communications%20and%20computer%20networking.) </sup>

The Netdata Agent monitors the time until an X.509 certificate expires. This alert indicates that,
the X.509 certificate will expire soon. Check more about
the [x509 certificate monitoring with Netdata](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/x509check).

By default, this alert is triggered in warning state when your certificate has less than 14 days to expire and
in critical state when it has less than 7 days to expire, but these levels are configurable.

A certification authority (CA) is an entity that issues digital certificates. A digital certificate
certifies the ownership of a public key by the named subject of the certificate. This allows
others (relying parties) to rely upon signatures or on assertions made about the private key that
corresponds to the certified public key. A CA acts as a trusted third party—trusted both by the
subject (owner) of the certificate and by the party relying upon the certificate. The format of
these certificates is specified by the X.509 or EMV standard.

<details>
<summary>Where and why we need X.509 certificates</summary>

The following provides a comprehensive explanation from the sectigo's website: <sup> [1](https://sectigo.com/resource-library/what-is-x509-certificate#:~:text=Share%20this-,An%20X.,internet%20communications%20and%20computer%20networking.) </sup>


Common Applications of X.509 Public Key Infrastructure Many internet protocols rely on X.509, and
there are many applications of the PKI technology that are used every day, including Web server
security, digital signatures and document signing, and digital identities.

- **Web Server Security with TLS/SSL Certificates:**
  PKI is the basis for the secure sockets layer (SSL)
  and transport layer security (TLS) protocols that are the foundation of HTTPS secure browser
  connections. Without SSL certificates or TLS to establish secure connections, cybercriminals could
  exploit the Internet or other IP networks using a variety of attack vectors, such as
  man-in-the-middle attacks, to intercept messages and access their contents.

- **Digital Signatures and Document Signing:**
  In addition to being used to secure messages, PKI-based certificates can be used for digital
  signatures and document signing. Digital signatures are a specific type of electronic signature
  that leverages PKI to authenticate the identity of the signer and the integrity of the signature
  and the document. Digital signatures cannot be altered or duplicated in any way, as the signature
  is created by generating a hash, which is encrypted using a sender's private key. This
  cryptographic verification mathematically binds the signature to the original message to ensure
  that the sender is authenticated and the message itself has not been altered.

- **Code Signing:**
  Code Signing enables application developers to add a layer of assurance by digitally signing
  applications, drivers, and software programs so that end users can verify that a third party has
  not altered or compromised the code they receive. To verify the code is safe and trusted, these
  digital certificates include the software developer's signature, the company name, and
  timestamping.

- **Email Certificates:**
  S/MIME certificates validate email senders and encrypt email contents to protect against
  increasingly sophisticated social engineering and spear phishing attacks. By encrypting/decrypting
  email messages and attachments and by validating identity, S/MIME email certificates assure users
  that emails are authentic and unmodified.

- **SSH Keys:**
  SSH keys are a form of X.509 certificate that provides a secure access credential used in the
  Secure Shell (SSH) protocol. As the SSH protocol is widely used for communication in cloud
  services, network environments, file transfer tools, and configuration management tools, most
  organizations use SSH keys to authenticate identity and protect those services from unintended use
  or malicious attacks. SSH keys not only improve security, but also enable the automation of
  connected processes, single sign-on (SSO), and identity and access management at the scale that
  today's businesses require.

- **Digital Identities:**
  X.509 digital certificates also provide effective digital identity authentication. As data and
  applications expand beyond traditional networks to mobile devices, public clouds, private clouds,
  and Internet of Things devices, securing identities becomes more important than ever. And digital
  identities don't have to be restricted to devices; they can also be used to authenticate people,
  data, or applications. Digital identity certificates based on this standard enable organizations
  to improve security by replacing passwords, which attackers have become increasingly adept at
  stealing.

</details>

<details>
<summary>Popular CAs </summary>

1. https://letsencrypt.org/
2. https://securitycloud.symantec.com/cc/landing
3. https://www.geotrust.com/
4. https://sectigo.com/
5. https://www.digicert.com/

</details>

<details>
<summary>References and source </summary>

1. [X.509 explained](https://sectigo.com/resource-library/what-is-x509-certificate#:~:text=Share%20this-,An%20X.,internet%20communications%20and%20computer%20networking.)

</details>


### Troubleshooting section

Anyone can issue an X.509 certificate, and a X.509 certificate may or may not have an expiration date.
In most cases the certificates which are issued by a CA have a validity period. In order to
persist your certificate's validity, you must either renew (or re-key) it. If your certificate is
issued by a CA, you must manage it from your CA.


