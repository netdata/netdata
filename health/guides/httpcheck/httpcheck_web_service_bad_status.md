# httpcheck_web_service_bad_status

**Web Server | HTTP endpoint**

The Netdata agent monitors your HTTP endpoints. You can specify endpoints the agent will monitor in
Agent's Go module under `go.d/httpcheck.conf`. You can also specify the expected response statuses
this HTTP endpoint must reply with, in the `status_accepted` option.
<sup>[1](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/httpcheck) </sup>
If the endpoint responds with a response status that is not in the specified `status_accepted` codes, the Agent
marks the response as "bad_status".

The Netdata Agent calculates the average ratio of these unexpected (bad) HTTP status responses over
the last 5 minutes.

This alert is triggered in warning state when the ratio is greater than 10% and in critical state
when it is greater than 40%.

<details>
<summary>References and sources</summary>

1. [HTTP endpoint monitoring with Netdata](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/httpcheck)

</details>

### Troubleshooting section:

<details>
<summary>Check the actual response status and the expected response statuses</summary>

1. Try to implement a request with a verbose result:

```
root@netdata # curl -v <your_http_endpoint>:<port>/<path>
```

2. Compare it with the expected response

Check your configuration under `go.d/httpcheck.conf` which are the `status_accepted` codes for this
particular endpoint.

```
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d/httpcheck.conf
```

</details>
