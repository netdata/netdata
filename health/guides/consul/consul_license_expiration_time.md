### Understand the alert

This alert checks the Consul Enterprise license expiration time. It triggers a warning if the license expiration time is less than 14 days, and critical if it's less than 7 days.

_consul.license_expiration_time_: Monitors the remaining time in seconds until the Consul Enterprise license expires.

### What is Consul?

Consul is a service mesh solution that enables organizations to discover services and safely process network traffic across dynamic, distributed environments.

### Troubleshoot the alert

1. Check the current license expiration time

  You can check the remaining license expiration time for your Consul Enterprise instance using the Consul API:

   ```
   curl http://localhost:8500/v1/operator/license
   ```

  Look for the `ExpirationTime` field in the returned JSON output.

2. Renew the license

   If your license is about to expire, you will need to acquire a new license. Contact [HashiCorp Support](https://support.hashicorp.com/) to obtain and renew the license key.

3. Apply the new license
   
   You can apply the new license key either by restarting Consul with the new key specified via the `CONSUL_LICENSE` environment variable or the `license_path` configuration option, or by updating the license through the Consul API:
   
   ```
   curl -X PUT -d @new_license.json http://localhost:8500/v1/operator/license
   ```

   Replace `new_license.json` with the path to a file containing the new license key in JSON format.

4. Verify the new license expiration time

   After applying the new license, you can check the new license expiration time using the Consul API again:

   ```
   curl http://localhost:8500/v1/operator/license
   ```

   Ensure that the `ExpirationTime` field shows the new expiration time.

### Useful resources

1. [Consul License Documentation](https://www.consul.io/docs/enterprise/license)
2. [HashiCorp Support](https://support.hashicorp.com/)
