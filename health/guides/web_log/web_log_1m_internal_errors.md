# web_log_1m_internal_errors

**Web Server | Web log**

HTTP response status codes indicate whether a specific HTTP request has been successfully completed
or not.

Response status codes beginning with the digit "5" indicate cases in which the server is aware that
it has erred or is incapable of performing the request. Except when responding to a HEAD request,
the server should include an entity containing an explanation of the error situation, and whether it
is a temporary or permanent condition. User agents should display any included entity to the user.
These response codes are applicable to any request
method.<sup>[1](https://datatracker.ietf.org/doc/html/rfc2616#section-10.5) </sup>

The Netdata Agent calculates the ratio of server error HTTP requests over the last minute.

<details>
  <summary>Server error responses (5XX)</summary>

The error codes below contain the descriptions as provided by
Mozilla. <sup>[2](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#server_error_responses) </sup>

- _500 Internal Server Error_: The server has encountered a situation it does not know how to
  handle.

- _501 Not Implemented_: The request method is not supported by the server and cannot be handled.
  The only methods that servers are required to support (and therefore that must not return this
  code)
  are GET and HEAD.

- _502 Bad Gateway_: This error response means that the server, while working as a gateway to get a
  response needed to handle the request, got an invalid response.

- _503 Service Unavailable_: The server is not ready to handle the request. Common causes are a
  server that is down for maintenance or that is overloaded. Note that together with this response,
  a user-friendly page explaining the problem should be sent. This response should be used for
  temporary conditions and the Retry-After HTTP header should, if possible, contain the estimated
  time before the recovery of the service. The webmaster must also take care about the
  caching-related headers that are sent along with this response, as these temporary condition
  responses should usually not be cached.

- _504 Gateway Timeout_: This error response is given when the server is acting as a gateway and
  cannot get a response in time.

- _505 HTTP Version Not Supported_: The HTTP version used in the request is not supported by the
  server.

- _506 Variant Also Negotiates_: The server has an internal configuration error: the chosen variant
  resource is configured to engage in transparent content negotiation itself, and is therefore not a
  proper end point in the negotiation process.

- _507 Insufficient Storage (WebDAV)_:
  The method could not be performed on the resource because the server is unable to store the
  representation needed to successfully complete the request.

- _508 Loop Detected (WebDAV)_:
  The server detected an infinite loop while processing the request.

- _510 Not Extended_: Further extensions to the request are required for the server to fulfill it.

- _511 Network Authentication_: Required Indicates that the client needs to authenticate to gain
  network access.

Source: [https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#server_error_responses](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#server_error_responses)

</details>

<details>
  <summary>References and sources</summary>

1. [Server errors on Datatracker](https://datatracker.ietf.org/doc/html/rfc2616#section-10.5)

2. [HTTP server errors on Mozilla](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#server_error_responses)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

To identify the HTTP response code your web server sends back: 

1. Open the Netdata dashboard.
2. Inspect the `detailed_response_codes` chart for your web server. This chart keeps
track of exactly what error codes your web server sends out.

You should also check the server error logs. For example, web servers such as Apache or Nginx
produce and error logs, by default under `/var/log/{nginx, apache2}/{access.log, error.log}`

</details>

<details>
<summary>Troubleshoot 500 error code </summary>

One of the things that can cause HTTP 500 response errors is a misconfiguration in the `.htaccess`
file of your web server.

</details>
