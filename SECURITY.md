<!--
---
title: "Security policy"
custom_edit_url: https://github.com/netdata/netdata/edit/master/SECURITY.md
---
-->

# Security policy

## Supported versions

| Version | Supported |
|-------  | --------- |
| Latest  | Yes       |

## Reporting a vulnerability

We're incredibly grateful for security researchers and users that report vulnerabilities to Netdata Open Source Community. A set of community volunteers thoroughly investigates all reports.

To make a report, [create a post](https://groups.google.com/a/netdata.cloud/forum/#!newtopic/security) with details about the vulnerability and other details expected for [all Netdata bug reports](https://github.com/netdata/netdata/blob/c1f4c6cf503995cd4d896c5821b00d55afcbde87/.github/ISSUE_TEMPLATE/bug_report.md).

### When should I report a vulnerability?

-   You think you discovered a potential security vulnerability in Netdata
-   You are unsure how a vulnerability affects Netdata
-   You think you discovered a vulnerability in another project that Netdata depends on (e.g. python, node, etc)

### When should I NOT report a vulnerability?

-   You need help tuning Netdata for security
-   You need help applying security related updates
-   Your issue is not security related

### Security vulnerability response

Each report is acknowledged and analyzed by Netdata Team members within 3 working days. This will set off a Security Release Process.

Any vulnerability information shared with Netdata Team stays within Netdata project and will not be disseminated to other projects unless it is necessary to get the issue fixed.

As the security issue moves from triage, to identified fix, to release planning we will keep the reporter updated.

### Public disclosure timing

A public disclosure date is negotiated by the Netdata team and the bug submitter. We prefer to fully disclose the bug as soon as possible once a user mitigation is available. It is reasonable to delay disclosure when the bug or the fix is not yet fully understood, the solution is not well-tested, or for vendor coordination. The timeframe for disclosure is from immediate (especially if it's already publicly known) to a few weeks. As a basic default, we expect report date to disclosure date to be on the order of 7 days. The Netdata team holds the final say when setting a disclosure date.

### Security announcements

Every time a security issue is fixed in Netdata, we immediately release a new version of it. So, to get notified of all security incidents, please subscribe to our releases on github.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FSECURITY&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
