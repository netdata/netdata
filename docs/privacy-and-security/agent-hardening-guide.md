# Netdata Agent Hardening Guide

This guide provides security best practices for deploying and operating the Netdata Agent in a secure and robust manner. It applies to Netdata Agent installations on Linux, macOS, Windows, FreeBSD, and containers (Docker, Kubernetes).

All recommendations are based on official Netdata documentation, public GitHub repositories, and known configuration options. No commercial components or features from Netdata Cloud are included here.

## Audience

This guide is intended for platform engineers, DevOps teams, SREs, and security-conscious sysadmins responsible for deploying or maintaining Netdata in production, compliance-sensitive, or regulated environments.

## Scope

This guide covers:

- Netdata Agent installed directly on bare-metal, virtual machines, or cloud VMs
- Netdata Agent running in Docker containers
- Netdata Agent deployed to Kubernetes clusters

## Security Hardening Areas

| Section | Purpose |
|--------|---------|
| **1. Secure Installation** | Verifying checksums/signatures of binaries, installing from trusted sources, using secure package managers. |
| **2. Least Privilege** | Running the monitoring agent as a non-root user (if possible), configuring systemd units with limited privileges. |
| **3. Network Access Control** | Limiting access to monitoring ports (`19999`) via firewalls or IP allowlists. Using reverse proxies to enforce access control. |
| **4. Encryption in Transit** | Enabling HTTPS via TLS (e.g., behind nginx or Caddy), securing streaming between agents and parents with mutual TLS. |
| **5. Authentication and Authorization** | Enforcing authentication for dashboards. Avoid unauthenticated public exposure. |
| **6. Logging and Audit Trails** | Ensuring secure log storage, controlling log verbosity, integrating logs with SIEM or centralized logging systems. |
| **7. Disable Unused Features** | Turning off collectors, plugins, and telemetry not needed in your environment to reduce the attack surface. |
| **8. File and Directory Permissions** | Restricting read/write access to configuration files, secrets, and Netdata’s database folders. |
| **9. System Hardening** | Using AppArmor/SELinux profiles, enabling Linux kernel hardening (ASLR, etc.), keeping OS patched. |
| **10. Configuration Management** | Using GitOps or configuration management tools (Ansible, Puppet) for version-controlled, repeatable deployments. |
| **11. Updates and Patch Management** | Staying current with Netdata releases and CVEs. Use automatic updates cautiously with proper testing. |
| **12. Compliance Alignment** | Mapping practices to compliance standards (SOC 2, ISO 27001, NIS2, GDPR, etc.). |

## TL;DR: Hardening Checklist

| Area                       | Recommendation                                                                                           | Notes |
|----------------------------|--------------------------------------------------------------------------------------------------------|-------|
| Package Source             | Use the [official one-line installer](https://learn.netdata.cloud/docs/installing/one-line-installer) or official Docker image. | Avoid unofficial builds |
| File Permissions           | Ensure configuration files (`/etc/netdata/`) are owned by `netdata` user and not writable by others.  | Applies to host installations |
| Data Directory             | Protect `/var/lib/netdata` and other runtime dirs from unauthorized access.                          | Default location for runtime and database |
| Network Access             | Restrict port `19999` using firewalls or expose it only behind a reverse proxy.                      | Applies to all deployments |
| Reverse Proxy              | Use HTTPS with authentication if exposing dashboard publicly.                                         | Use nginx, Traefik, etc. |
| Agent User                 | Do not run Netdata as `root` unless required. Use system user `netdata`.                             | Applies to most packages |
| Update Strategy            | Enable auto-updates or track releases to apply security fixes promptly.                              | Configure `netdata-updater.sh` or container automation |
| Telemetry                  | Disable anonymous telemetry if required by policy.                                                    | See opt-out instructions |
| Registry and Access        | Avoid sharing URLs without secured access (e.g., dashboard tokens, Netdata Registry entries).         | Prevent unauthorized access |
| CVE Monitoring             | Subscribe to [Netdata Security Advisories](https://github.com/netdata/netdata/security/advisories) and track GitHub releases. | Critical for production |

## Container Hardening

| Area                    | Recommendation                                                                                                       | Source |
|-------------------------|------------------------------------------------------------------------------------------------------------------------|--------|
| Base Image             | Use the official `netdata/netdata` image from DockerHub.                                                               | [DockerHub](https://hub.docker.com/r/netdata/netdata) |
| Capabilities           | Drop all Linux capabilities and re-add only those required.                                                           | [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker) |
| Required Capabilities  | `SYS_PTRACE`, `SYS_ADMIN` (for eBPF or advanced logging features).                                                   | [Netdata GitHub](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md) |
| Read-Only Filesystem   | Enable a read-only root filesystem and mount only necessary volumes.                                                  | [Docker Docs](https://docs.docker.com/engine/security/seccomp/) |
| AppArmor / Seccomp     | Apply AppArmor profile and use the `--security-opt seccomp=...` option.                                               | [Docker Hardening Guide](https://docs.docker.com/engine/security/) |
| User Context           | Avoid running as `root` when possible. Use `--user netdata` or a custom UID/GID.                                      | [Netdata Learn](https://learn.netdata.cloud/docs/netdata-agent/packaging/docker) |
| Host Mounts            | Mount `/proc`, `/sys`, and `/etc` read-only when needed. Use `/host` or `/host/proc` for metrics access.              | [Netdata Learn](https://learn.netdata.cloud/docs/netdata-agent/packaging/docker) |
| Network Mode           | Use `host` networking for full visibility or limit exposed ports to 19999 securely.                                   | [Netdata Docker Guide](https://learn.netdata.cloud/docs/netdata-agent/packaging/docker) |
| Dashboard Access       | Restrict access to port `19999` using firewall rules, reverse proxy auth, or VPN.                                    | [Netdata GitHub Discussions](https://github.com/netdata/netdata/discussions) |
| Image Updates          | Always pull the latest verified image before deployment.                                                              | [DockerHub](https://hub.docker.com/r/netdata/netdata) |

## Kubernetes Considerations

The Netdata Agent can be deployed to Kubernetes using Helm charts or container manifests.

| Area                    | Recommendation                                                                                         | Source |
|-------------------------|------------------------------------------------------------------------------------------------------|--------|
| Deployment Method       | Use the [official Helm chart](https://learn.netdata.cloud/docs/installation/install-on-specific-environments/kubernetes/) from Netdata. | Kubernetes Guide |
| Namespace Isolation     | Deploy Netdata in a dedicated namespace and apply RBAC as needed.                                   | Standard Kubernetes best practice |
| Pod Security Context    | Use `securityContext` to drop capabilities and run as non-root.                                     | Configure in `values.yaml` |
| Service Exposure        | Expose Netdata dashboard via Ingress with TLS and authentication.                                 | Avoid NodePort for public access |
| Volume Mounts           | Mount `/host/proc`, `/host/sys`, and `/etc` to allow metrics collection.                           | HostPath mounts required for visibility |
| Resource Limits         | Apply CPU/memory resource requests and limits to prevent abuse or starvation.                      | Helm chart defaults can be tuned |

## Host System Hardening

When running Netdata Agent directly on host systems:

- Limit access to Netdata's dashboard port (19999) using firewalls or local-only binding.
- Configure alerts to use secure channels (e.g., SMTP with TLS, webhook endpoints over HTTPS).
- Avoid editing `/etc/netdata/netdata.conf` directly on production systems. Use version-controlled configurations or config management tools.
- Disable unused plugins and collectors by editing their `conf.d` files or using `disable = yes` options.
- For enhanced security, compile Netdata from source with only required plugins enabled.

## Logging and Telemetry

- Integrate Netdata with journald or syslog for log collection and monitoring.
- Anonymous telemetry can be disabled by placing a file at `/etc/netdata/.opt-out-from-anonymous-statistics`.

## Final Notes

- Regularly audit Netdata configurations and volumes.
- Do not expose Netdata metrics or dashboard to the internet without authentication.
- Always validate configuration changes in staging before applying to production.
- Use compliance mapping frameworks to assess Netdata’s hardening posture relative to your organization’s requirements.

For more, see:
- [Netdata Learn](https://learn.netdata.cloud)
- [Netdata GitHub](https://github.com/netdata/netdata)
- [Netdata Docker Documentation](https://learn.netdata.cloud/docs/netdata-agent/packaging/docker)
- [Netdata Kubernetes Deployment Guide](https://learn.netdata.cloud/docs/installation/install-on-specific-environments/kubernetes/
 [Netdata Security Advisories](https://github.com/netdata/netdata/security/advisories)
x
x
