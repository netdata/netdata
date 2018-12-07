# puppet

Monitor status of Puppet Server and Puppet DB.

Following charts are drawn:

1. **JVM Heap**
 * committed (allocated from OS)
 * used (actual use)
2. **JVM Non-Heap**
 * committed (allocated from OS)
 * used (actual use)
3. **CPU Usage**
 * execution
 * GC (taken by garbage collection)
4. **File Descriptors**
 * max
 * used


### configuration

```yaml
puppetdb:
    url: 'https://fqdn.example.com:8081'
    tls_cert_file: /path/to/client.crt
    tls_key_file: /path/to/client.key
    autodetection_retry: 1

puppetserver:
    url: 'https://fqdn.example.com:8140'
    autodetection_retry: 1
```

When no configuration is given, module uses `https://fqdn.example.com:8140`.

### notes

* Exact Fully Qualified Domain Name of the node should be used.
* Usually Puppet Server/DB startup time is VERY long. So, there should
  be quite reasonable retry count.
* Secure PuppetDB config may require client certificate. Not applies
  to default PuppetDB configuration though.

---
