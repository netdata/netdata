# Netdata Security and Disclosure Information

This page describes netdata security and disclosure information.

## Security Announcements

Every time a security issue is fixed in netdata, we immediately release a new version of it. So, to get notified of all security incidents, please subscribe to our releases on github.

## Report a Vulnerability

We’re extremely grateful for security researchers and users that report vulnerabilities to Netdata Open Source Community. All reports are thoroughly investigated by a set of community volunteers.

To make a report, please email the private [security@netdata.cloud](mailto:security@netdata.cloud) list with the security details and the details expected for [all netdata bug reports](https://github.com/netdata/netdata/.github/ISSUE_TEMPLATE.md).

## When Should I Report a Vulnerability?

- You think you discovered a potential security vulnerability in Netdata
- You are unsure how a vulnerability affects Netdata
- You think you discovered a vulnerability in another project that Netdata depends on (e.g. python, node, etc)

### When Should I NOT Report a Vulnerability?

- You need help tuning Netdata for security
- You need help applying security related updates
- Your issue is not security related

## Security Vulnerability Response

Each report is acknowledged and analyzed by Netdata Team members within 3 working days. This will set off a Security Release Process.

Any vulnerability information shared with Netdata Team stays within Netdata project and will not be disseminated to other projects unless it is necessary to get the issue fixed.

As the security issue moves from triage, to identified fix, to release planning we will keep the reporter updated.

## Public Disclosure Timing

A public disclosure date is negotiated by the Netdata team and the bug submitter. We prefer to fully disclose the bug as soon as possible once a user mitigation is available. It is reasonable to delay disclosure when the bug or the fix is not yet fully understood, the solution is not well-tested, or for vendor coordination. The timeframe for disclosure is from immediate (especially if it's already publicly known) to a few weeks. As a basic default, we expect report date to disclosure date to be on the order of 7 days. The Netdata team holds the final say when setting a disclosure date.
