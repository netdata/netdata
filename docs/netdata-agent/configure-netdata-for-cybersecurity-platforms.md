# Configure Netdata for cybersecurity platforms

This guide is intended to help you place Netdata behind enterprise security controls such as reverse proxies, firewalls, VPN gateways, or secure-access platforms. The UI differs by product, but the Netdata-side configuration and network design remain the same.

## Choose how users should access Netdata

Before you configure your security platform, choose the Netdata access model that fits your environment:

| Access model                                        | Best for                                                                           | Netdata configuration                                                      |
|-----------------------------------------------------|------------------------------------------------------------------------------------|----------------------------------------------------------------------------|
| Cloud-only access                                   | Environments that do not allow inbound access to the Agent                         | Disable the local dashboard with `mode = none`                             |
| Access through a security platform or reverse proxy | Environments that publish Netdata through HTTPS, SSO, VPN, or application gateways | Keep Netdata on a private interface and allow only the gateway path        |
| Access on a private management network              | Environments with a dedicated admin network                                        | Bind Netdata only to the management interface and restrict allowed clients |

If you are not sure which model to use, start with the smallest exposed surface:

1. Use **Cloud-only access** to give users dashboard access through Netdata Cloud without exposing the Agent directly.
2. Use **access through a security platform** if you must publish the local dashboard to users.
3. Use **private management network access** only when you already have a trusted admin network.

## Configure Cloud-only access

Use this pattern when your security platform mainly controls outbound traffic and users access dashboards via Netdata Cloud instead of connecting directly to port `19999`.

### Prerequisites

- Your Agent is [claimed to Netdata Cloud](/src/claim/README.md)
- Your environment allows the Agent to maintain its secure outbound Cloud connection

1. Edit `netdata.conf` with the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) script.

2. Disable the local dashboard:

   ```ini
   [web]
       mode = none
   ```

3. Restart the Agent. For platform-specific steps, see [Edit Configuration Files](/docs/netdata-agent/configuration/README.md#edit-configuration-files).

4. Verify that dashboards remain available from Netdata Cloud.

For step-by-step operational details, see:

- [Edit Configuration Files](/docs/netdata-agent/configuration/README.md#edit-configuration-files)
- [Secure Your Netdata Agent with Bearer Token Protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md)

The Agent web server stops accepting inbound requests. This reduces exposure, but it also disables inbound streaming. Do not use this setting on Parent Agents that receive data from Child Agents.

## Configure access through a cybersecurity platform

Use this pattern when a platform such as Sophos, an authenticating reverse proxy, or a secure-access gateway publishes Netdata to users over HTTPS.

### Prerequisites

- A private IP address or loopback interface that the Netdata Agent can bind to
- A gateway, reverse proxy, firewall portal, or secure-access platform that can reach the Agent on that private address
- A decision on authentication:
  - Use the external platform for authentication, or
  - Use Netdata bearer token protection with Netdata Cloud identities

1. Bind Netdata to a private interface instead of exposing it on every address:

   ```ini
   [web]
       bind to = 10.0.0.15:19999 localhost:19999
   ```

   Replace `10.0.0.15` with the private interface your security platform can reach.

2. If you want direct Netdata authorization through Netdata Cloud, enable bearer token protection:

   ```ini
   [web]
       bind to = 10.0.0.15:19999 localhost:19999
       bearer token protection = yes
   ```

   This keeps the dashboard available only on the private interface while requiring Cloud authentication for data access.

3. Restart the Agent. For platform-specific steps, see [Edit Configuration Files](/docs/netdata-agent/configuration/README.md#edit-configuration-files).

4. Publish the Agent through your security platform using the private Netdata address as the upstream target.

   Use an internal upstream such as:

   ```text
   http://10.0.0.15:19999
   ```

   The platform should terminate HTTPS, apply your organization's authentication and policy controls, and forward requests only to this private Netdata endpoint.

5. Restrict network access so only the security platform path can reach Netdata on port `19999`.

   In practice, this usually means:

   - No direct public inbound access to `19999`
   - Only the reverse proxy, VPN users, or management network can reach the private Netdata address
   - Firewall rules block all other sources

For implementation details and hardening examples, see:

- [Running Netdata Behind a Reverse Proxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/README.md)
- [Secure Your Netdata Agent with Bearer Token Protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md)

### Authentication choices for published access

If the external platform already authenticates users, you can rely on it and keep Netdata on a private address. If you want Netdata Cloud roles and permissions to apply when users open the Agent directly through the published URL, enable [bearer token protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md).

Use bearer token protection when:

- You want Netdata Cloud SSO and role-based permissions
- You do not want to manage per-proxy password files
- Your Agent is claimed and connected to Netdata Cloud

Use external-platform authentication only when:

- The platform must remain the primary identity provider for the published application
- You need an access path that does not depend on Netdata Cloud authentication

## Configure private management network access

Use this pattern when administrators already use a private LAN, VPN, or bastion network and you do not want to publish Netdata through a broader application gateway.

1. Bind Netdata to the management interface:

   ```ini
   [web]
       bind to = 10.1.1.1:19999 localhost:19999
   ```

2. Restrict who can connect:

   ```ini
   [web]
       bind to = 10.1.1.1:19999 localhost:19999
       allow connections from = localhost 10.*
   ```

3. Restart the Agent. For platform-specific steps, see [Edit Configuration Files](/docs/netdata-agent/configuration/README.md#edit-configuration-files).

4. Verify that the dashboard is reachable only from the management network.

For detailed operational steps, see:

- [Edit Configuration Files](/docs/netdata-agent/configuration/README.md#edit-configuration-files)
- [Securing Netdata Agents](/docs/netdata-agent/securing-netdata-agents.md)

## Network rules your cybersecurity platform should enforce

No matter which product you use, your platform design should follow these rules:

- Do not expose `19999` directly to the public internet
- Prefer HTTPS on the published entry point
- Allow inbound access only through your approved gateway, VPN, proxy, or management network
- Allow outbound Agent connectivity to Netdata Cloud if you use claiming, Cloud dashboards, or bearer token protection
- Keep Netdata on private IPs whenever possible

If you deploy Netdata Parents, apply the same principle to the Parent dashboard. Child Agents should stream to Parents over trusted paths instead of being individually exposed.

### Example architecture for platforms such as Sophos

Platforms such as Sophos are typically used to enforce one or more of these controls:

- Reverse-proxy publishing
- Web access policies
- VPN-based access
- Firewall-based source restrictions

The exact product workflow differs, but the architecture usually looks like this:

1. Users authenticate to the organization's approved security platform.
2. The platform forwards approved requests to a private Netdata address such as `http://10.0.0.15:19999`.
3. Netdata accepts traffic only from that controlled path.
4. Netdata Cloud remains the preferred dashboard path when direct local access is not required.

This approach keeps Netdata aligned with enterprise security controls without requiring public exposure of the Agent.

## Sophos Central example for Windows exclusions

:::note

This is a **vendor-specific example**. Use it as a pattern and adapt it to your platform's equivalent exclusion workflow and policy controls.

:::

If you run the Netdata Agent on Windows and Sophos Central blocks or interferes with the Netdata executable, add a global exclusion for the Netdata binary.

In our testing, Sophos did not flag the Netdata binary as malicious. Sophos may still block Netdata because the Agent monitors network traffic and CPU activity on the host, which can trigger behavior-based protections.

:::note

Sophos Central labels and layout may change over time. The steps below reflect the current workflow.

:::

1. Open **Global Settings** in Sophos Central:

   ```text
   https://central.sophos.com/manage/overview/settings-list
   ```

2. Open **Global Exclusions**.

3. Select **Add exclusions**.

4. In the new window, add an exclusion for:

   - **Exploit Mitigation Activity Monitoring (Windows)**

5. When Sophos asks for the path, enter:

   ```text
   C:\Program Files\Netdata\usr\bin\netdata.exe
   ```

6. Clear **Protect Application**.

7. Select **Save**.

This exclusion applies to the Windows Netdata executable path shown above.

Map this exclusion into your broader access model and network controls in [Network rules your cybersecurity platform should enforce](#network-rules-your-cybersecurity-platform-should-enforce).

## What's next?

- Review [Securing Netdata Agents](/docs/netdata-agent/securing-netdata-agents.md) for the broader list of supported hardening options
- Use [Secure Your Netdata Agent with Bearer Token Protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md) if you want Netdata Cloud SSO on published Agent URLs
- Use [Running the Agent behind a reverse proxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/README.md) for web-server-specific proxy examples
