# Gearman

Module monitors Gearman worker statistics. A chart
is shown for each job as well as one showing a summary
of all workers.

Note: Charts may show as a line graph rather than an area 
graph if you load Netdata with no jobs running. To change 
this go to "Settings" > "Which dimensions to show?" and 
select "All".

Plugin can obtain data from tcp socket **OR** unix socket.

**Requirement:**
Socket MUST be readable by netdata user.

It produces:

 * Workers queued
 * Workers idle
 * Workers running

### configuration

```yaml
localhost:
  name     : 'local'
  host     : 'localhost'
  port     : 4730
  
  # TLS information can be provided as well
  tls      : no
  cert     : /path/to/cert
  key      : /path/to/key
```

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:4730`.

---
