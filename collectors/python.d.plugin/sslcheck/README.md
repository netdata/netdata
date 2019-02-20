# SSL certificate expiry check

Checks the time until a remote SSL certificate expires in days.

## Requirements

None

## Example configuration

```yaml
job_name:
  name: myname                 # [optional] the JOB's name as it will appear at the
                               # dashboard (by default is the job_name)
  host: 'my-netdata.io'        # [required] the remote host address in either IPv4, IPv6 or as DNS name
  port: 443                    # [required] the port number to check. Specify an integer, not service name
  daysuntilexpiration: 5       # [required] days before the status is critical?
  timeout: 10                  # [optional] the socket timeout when connecting
  update_every: 120            # [optional] the JOB's data collection frequency. As the chart is in days
                               # remaining, a high value should be chosen
  priority: 60000              # [optional] the JOB's order on the dashboard
  penalty: yes                 # [optional] the JOB's penalty
```



