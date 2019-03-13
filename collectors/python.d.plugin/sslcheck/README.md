# SSL certificate expiry check

Checks the time until a remote SSL certificate expires.

## Requirements

None

### configuration

```yaml
update_every : 60

example_org:
  host: 'example.org'

my_site_org:
  host: 'my-site.org'
  days_until_expiration_warning: 10
  days_until_expiration_critical: 1
```
