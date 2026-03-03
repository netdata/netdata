# Forward Secure Sealing (FSS) in Systemd-Journal

Forward Secure Sealing (FSS) is a feature in the systemd journal designed to detect log file tampering.
Given that attackers often try to hide their actions by modifying or deleting log file entries,
FSS provides administrators with a mechanism to identify any such unauthorized alterations.

## Importance

Logs are a crucial component of system monitoring and auditing. Ensuring their integrity means administrators can trust
the data, detect potential breaches, and trace actions back to their origins. Traditional methods to maintain this
integrity involve writing logs to external systems or printing them out. While these methods are effective, they are
not foolproof. FSS offers a more streamlined approach, allowing for log verification directly on the local system.

## How FSS Works

FSS operates by "sealing" binary logs at regular intervals. This seal is a cryptographic operation, ensuring that any
tampering with the logs prior to the sealing can be detected. If an attacker modifies logs before they are sealed,
these changes become a permanent part of the sealed record, highlighting any malicious activity.

The technology behind FSS is based on "Forward Secure Pseudo Random Generators" (FSPRG), a concept stemming from
academic research.

Two keys are central to FSS:

- **Sealing Key**: Kept on the system, used to seal the logs.
- **Verification Key**: Stored securely off-system, used to verify the sealed logs.

Every so often, the sealing key is regenerated in a non-reversible process, ensuring that old keys are obsolete and the
latest logs are sealed with a fresh key. The off-site verification key can regenerate any past sealing key, allowing
administrators to verify older seals. If logs are tampered with, verification will fail, alerting administrators to the
breach.

## Enabling FSS

To enable FSS, use the following command:

```bash
journalctl --setup-keys
```

By default, systemd will seal the logs every 15 minutes. However, this interval can be adjusted using a flag during key
generation. For example, to seal logs every 10 seconds:

```bash
journalctl --setup-keys --interval=10s
```

## Verifying Journals

After enabling FSS, you can verify the integrity of your logs using the verification key:

```bash
journalctl --verify
```

If any discrepancies are found, you'll be alerted, indicating potential tampering.

## Disabling FSS

Should you wish to disable FSS:

**Delete the Sealing Key**: This stops new log entries from being sealed.

```bash
journalctl --rotate
```

**Rotate and Prune the Journals**: This will start a new unsealed journal and can remove old sealed journals.

```bash
journalctl --vacuum-time=1s
```

**Adjust Systemd Configuration (Optional)**: If you've made changes to facilitate FSS in `/etc/systemd/journald.conf`,
consider reverting or adjusting those. Restart the systemd-journald service afterward:

```bash
systemctl restart systemd-journald
```

## Conclusion

FSS is a significant advancement in maintaining log integrity. While not a replacement for all traditional integrity
methods, it offers a valuable tool in the battle against unauthorized log tampering. By integrating FSS into your log
management strategy, you ensure a more transparent, reliable, and tamper-evident logging system.
