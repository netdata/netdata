# Security and Privacy Design

This document serves as the relevant Annex to the [Terms of Service](https://www.netdata.cloud/service-terms/),
the [Privacy Policy](https://www.netdata.cloud/privacy/) and
the Data Processing Addendum, when applicable. It provides more information regarding Netdata’s technical and
organizational security and privacy measures.

We have given special attention to all aspects of Netdata, ensuring that everything throughout its operation is as
secure as possible. Netdata has been designed with security in mind.

## Netdata's Security Principles

### Security by Design

Netdata, an open-source software widely installed across the globe, prioritizes security by design, showcasing our
commitment to safeguarding user data. The entire structure and internal architecture of the software is built to ensure
maximum security. We aim to provide a secure environment from the ground up, rather than as an afterthought.

### Compliance with Open Source Security Foundation Best Practices

Netdata is committed to adhering to the best practices laid out by the Open Source Security Foundation (OSSF).
Currently, the Netdata Agent follows the OSSF best practices at the passing level. Feel free to audit our approach to
the [OSSF guidelines](https://bestpractices.coreinfrastructure.org/en/projects/2231)

Netdata Cloud boasts of comprehensive end-to-end automated testing, encompassing the UI, back-end, and agents, where
involved. In addition, the Netdata Agent uses an array of third-party services for static code analysis, static code
security analysis, and CI/CD integrations to ensure code quality on a per pull request basis. Tools like Github's
CodeQL, Github's Dependabot, our own unit tests, various types of linters,
and [Coverity](https://scan.coverity.com/projects/netdata-netdata?tab=overview) are utilized to this end.

Moreover, each PR requires two code reviews from our senior engineers before being merged. We also maintain two
high-performance environments (a production-like kubernetes cluster and a highly demanding stress lab) for
stress-testing our entire solution. This robust pipeline ensures the delivery of high-quality software consistently.

### Regular Third-Party Testing and Isolation

While Netdata doesn't have a dedicated internal security team, the open-source Netdata Agent undergoes regular testing
by third parties. Any security reports received are addressed immediately. In contrast, Netdata Cloud operates in a
fully automated and isolated environment with Infrastructure as Code (IaC), ensuring no direct access to production
applications. Monitoring and reporting is also fully automated.

### Security Vulnerability Response

Netdata has a transparent and structured process for handling security vulnerabilities. We appreciate and value the
contributions of security researchers and users who report vulnerabilities to us. All reports are thoroughly
investigated, and any identified vulnerabilities trigger a Security Release Process.

We aim to fully disclose any bugs as soon as a user mitigation is available, typically within a week of the report. In
case of security fixes, we promptly release a new version of the software. Users can subscribe to our releases on GitHub
to stay updated about all security incidents. More details about our vulnerability response process can be
found [here](https://github.com/netdata/netdata/security/policy).

### Adherence to Open Source Security Foundation Best Practices

In line with our commitment to security, we uphold the best practices as outlined by the Open Source Security
Foundation. This commitment reflects in every aspect of our operations, from the design phase to the release process,
ensuring the delivery of a secure and reliable product to our users. For more information
check [here](https://bestpractices.coreinfrastructure.org/en/projects/2231).

## Compliance with Regulations

Netdata is committed to ensuring the security, privacy, and integrity of user data. It complies with both the General
Data Protection Regulation (GDPR), a regulation in EU law on data protection and privacy, and the California Consumer
Privacy Act (CCPA), a state statute intended to enhance privacy rights and consumer protection for residents of
California.

### Compliance with GDPR and CCPA

Compliance with GDPR and CCPA are self-assessment processes, and Netdata has undertaken thorough internal audits and
controls to ensure it meets all requirements.

As per request basis, any customer may enter with Netdata into a data processing addendum (DPA) governing customer’s
ability to load and permit Netdata to process any personal data or information regulated under applicable data
protection laws, including the GDPR and CCPA.

### Data Transfers

While Netdata Agent itself does not engage in any cross-border data transfers, certain infrastructure metadata
is transferred to Netdata Cloud for the purpose of providing its services. All observability data collected and
processed by Netdata Agents, however, stays strictly within the user's infrastructure, eliminating any concerns about
cross-border data transfer issues.

When users utilize Netdata Cloud, the parts of the observability data which are viewed via Netdata Cloud, are streamed
from the Netdata Agents to the users’ web browsers via Netdata Cloud, without being stored on Netdata Cloud's servers.
This is optional and users have the ability to use observability dashboards bypassing Netdata Cloud.

Netdata Cloud only stores Netdata Cloud users identification data (such as observability users' email addresses) and
infrastructure metadata (such as infrastructure hostnames) necessary for Netdata Cloud's operation. All these metadata
are stored in data centers in the United States, using compliant infrastructure providers such as Google Cloud and
Amazon Web Services. These transfers and storage are carried out in full compliance with applicable data protection
laws, including GDPR and CCPA.

### Privacy Rights

Netdata ensures user privacy rights as mandated by the GDPR and CCPA. This includes the right to access, correct, and
delete personal data. These functions are all available online via the Netdata Cloud User Interface (UI). In case a user
wants to remove all personal information (email and activities), they can delete their cloud account by logging
into <https://app.netdata.cloud> and accessing their profile, at the bottom left of the screen.

### Regular Review and Updates

Netdata is dedicated to keeping its practices up-to-date with the latest developments in data protection regulations.
Therefore, as soon as updates or changes are made to these regulations, Netdata reviews and updates its policies and
practices accordingly to ensure continual compliance.

While Netdata is confident in its compliance with GDPR and CCPA, users are encouraged to review Netdata's privacy policy
and reach out with any questions or concerns they may have about data protection and privacy.

## Anonymous Statistics

The anonymous statistics collected by the Netdata Agent are related to the installations and not to individual users.
This data includes community size, types of plugins used, possible crashes, operating systems installed, and the use of
the registry feature. No IP addresses are collected, but each Netdata installation has a unique ID.

Netdata also collects anonymous telemetry events, which provide information on the usage of various features, errors,
and performance metrics. This data is used to understand how the software is being used and to identify areas for
improvement.

The purpose of collecting these statistics and telemetry data is to guide the development of the open-source agent,
focusing on areas that are most beneficial to users.

Users have the option to opt out of this data collection during the installation of the agent, or at any time by
removing a specific file from their system.

Netdata retains this data indefinitely in order to track changes and trends within the community over time.

Netdata does not share these anonymous statistics or telemetry data with any third parties.

By collecting this data, Netdata is able to continuously improve their service and identify any issues or areas for
improvement, while respecting user privacy and maintaining transparency.

## Internal Security Measures

Internal Security Measures at Netdata are designed with an emphasis on data privacy and protection. The measures
include:

1. **Infrastructure as Code (IaC)** :
   Netdata Cloud follows the IaC model, which means it is a microservices environment that is completely isolated. All
   changes are managed through Terraform, an open-source IaC software tool that provides a consistent CLI workflow for
   managing cloud services.
2. **TLS Termination and IAM Service** :
   At the edge of Netdata Cloud, there is a TLS termination, which provides the decryption point for incoming TLS
   connections. Additionally, an Identity Access Management (IAM) service validates JWT tokens included in request
   cookies or denies access to them.
3. **Session Identification** :
   Once inside the microservices environment, all requests are associated with session IDs that identify the user making
   the request. This approach provides additional layers of security and traceability.
4. **Data Storage** :
   Data is stored in various NoSQL and SQL databases and message brokers. The entire environment is fully isolated,
   providing a secure space for data management.
5. **Authentication** :
   Netdata Cloud does not store credentials. It offers three types of authentication: GitHub Single Sign-On (SSO),
   Google SSO, and email validation.
6. **DDoS Protection** :
   Netdata Cloud has multiple protection mechanisms against Distributed Denial of Service (DDoS) attacks, including
   rate-limiting and automated blacklisting.
7. **Security-Focused Development Process** :
   To ensure a secure environment, Netdata employs a security-focused development process. This includes the use of
   static code analyzers to identify potential security vulnerabilities in the codebase.
8. **High Security Standards** :
   Netdata Cloud maintains high security standards and can provide additional customization on a per contract basis.
9. **Employee Security Practices** :
   Netdata ensures its employees follow security best practices, including role-based access, periodic access review,
   and multi-factor authentication. This helps to minimize the risk of unauthorized access to sensitive data.
10. **Experienced Developers** :
    Netdata hires senior developers with vast experience in security-related matters. It enforces two code reviews for
    every Pull Request (PR), ensuring that any potential issues are identified and addressed promptly.
11. **DevOps Methodologies** :
    Netdata's DevOps methodologies use the highest standards in access control in all places, utilizing the best
    practices available.
12. **Risk-Based Security Program** :
    Netdata has a risk-based security program that continually assesses and mitigates risks associated with data
    security. This program helps maintain a secure environment for user data.

These security measures ensure that Netdata Cloud is a secure environment for users to monitor and troubleshoot their
systems. The company remains committed to continuously improving its security practices to safeguard user data
effectively.

## PCI DSS

PCI DSS (Payment Card Industry Data Security Standard) is a set of security standards designed to ensure that all
companies that accept, process, store or transmit credit card information maintain a secure environment.

Netdata is committed to providing secure and privacy-respecting services, and it aligns its practices with many of the
key principles of the PCI DSS. However, it's important to clarify that Netdata is not officially certified as PCI
DSS-compliant. While Netdata follows practices that align with PCI DSS's key principles, the company itself has not
undergone the formal certification process for PCI DSS compliance.

PCI DSS compliance is not just about the technical controls but also involves a range of administrative and procedural
safeguards that go beyond the scope of Netdata's services. These include, among other things, maintaining a secure
network, implementing strong access control measures, regularly monitoring and testing networks, and maintaining an
information security policy.

Therefore, while Netdata can support entities with their data security needs in relation to PCI DSS, it is ultimately
the responsibility of the entity to ensure full PCI DSS compliance across all of their operations. Entities should
always consult with a legal expert or a PCI DSS compliance consultant to ensure that their use of any product, including
Netdata, aligns with PCI DSS regulations.

## HIPAA

HIPAA stands for the Health Insurance Portability and Accountability Act, which is a United States federal law enacted
in 1996. HIPAA is primarily focused on protecting the privacy and security of individuals' health information.

Netdata is committed to providing secure and privacy-respecting services, and it aligns its practices with many key
principles of HIPAA. However, it's important to clarify that Netdata is not officially certified as HIPAA-compliant.
While Netdata follows practices that align with HIPAA's key principles, the company itself has not undergone the formal
certification process for HIPAA compliance.

HIPAA compliance is not just about technical controls but also involves a range of administrative and procedural
safeguards that go beyond the scope of Netdata's services. These include, among other things, employee training,
physical security, and contingency planning.

Therefore, while Netdata can support HIPAA-regulated entities with their data security needs and is prepared to sign a
Business Associate Agreement (BAA), it is ultimately the responsibility of the healthcare entity to ensure full HIPAA
compliance across all of their operations. Entities should always consult with a legal expert or a HIPAA compliance
consultant to ensure that their use of any product, including Netdata, aligns with HIPAA regulations.

## SOC 2 Compliance

Service Organization Control 2 (SOC 2) is a framework for managing data to ensure the security, availability, processing integrity, confidentiality, and privacy of customer data. Developed by the American Institute of CPAs (AICPA), SOC 2 is specifically designed for service providers storing customer data in the cloud. It requires companies to establish and follow strict information security policies and procedures.

While Netdata is not currently SOC 2 certified, our commitment to security and privacy aligns closely with the principles of SOC 2. Here’s how Netdata's practices resonate with the key components of SOC 2 compliance:

### Security

Netdata has implemented robust security measures, including infrastructure as code, TLS termination, DDoS protection, and a security-focused development process. These measures echo the SOC 2 principle of ensuring the security of customer data against unauthorized access and potential threats.

### Availability

Netdata's commitment to system monitoring and troubleshooting ensures the availability of our service, consistent with the availability principle of SOC 2. Our infrastructure is designed to be resilient and reliable, providing users with continuous access to our services.

### Processing Integrity

Although Netdata primarily focuses on system monitoring and does not typically process customer data in a way that alters it, our commitment to accurate, timely, and valid delivery of services aligns with the processing integrity principle of SOC 2.

### Confidentiality

Netdata's measures to protect data—such as data encryption, strict access controls, and data isolation—demonstrate our commitment to confidentiality, ensuring that customer data is accessed only by authorized personnel and for authorized reasons.

### Privacy

Aligning with the privacy principle of SOC 2, Netdata adheres to GDPR and CCPA regulations, ensuring the protection and proper handling of personal data. Our privacy policies and practices are transparent, giving users control over their data.

### Continuous Improvement and Future Considerations

Netdata is committed to continuous improvement in security and privacy. While we are not currently SOC 2 certified, we understand the importance of this framework and are continuously evaluating our processes and controls against industry best practices. As Netdata grows and evolves, we remain open to pursuing SOC 2 certification or other similar standards to further demonstrate our dedication to data security and privacy.

## Conclusion

In conclusion, Netdata Cloud's commitment to data security and user privacy is paramount. From the careful design of the
infrastructure and stringent internal security measures to compliance with international regulations and standards like
GDPR and CCPA, Netdata Cloud ensures a secure environment for users to monitor and troubleshoot their systems.

The use of advanced encryption techniques, role-based access control, and robust authentication methods further
strengthen the security of user data. Netdata Cloud also maintains transparency in its data handling practices, giving
users control over their data and the ability to easily access, retrieve, correct, and delete their personal data.

Netdata's approach to anonymous statistics collection respects user privacy while enabling the company to improve its
product based on real-world usage data. Even in such cases, users have the choice to opt-out, underlining Netdata's
respect for user autonomy.

In summary, Netdata Cloud offers a highly secure, user-centric environment for system monitoring and troubleshooting.
The company's emphasis on continuous security improvement and commitment to user privacy make it a trusted choice in the
data monitoring landscape.
