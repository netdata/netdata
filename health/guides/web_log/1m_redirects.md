# 1m_redirects

**Web Server | Web log**

HTTP response status codes indicate whether a specific HTTP request has been successfully completed
or not.

The 3XX class of status code indicates that further action needs to be taken by the user agent in
order to fulfill the request. The action required may be carried out by the user agent without
interaction with the user if and only if the method used in the second request is GET or HEAD. A
client SHOULD detect infinite redirection loops, since such loops generate network traffic for each
redirection.
<sup> [1](https://datatracker.ietf.org/doc/html/rfc2616#section-10.3) </sup>

The Netdata Agent calculates the ratio of redirection HTTP requests over the last minute. This
metric does not include the "304 Not modified" message.

<details>
  <summary>Redirection messages (3XX)</summary>

The redirect messages below contain the descriptions as provided by
Mozilla.<sup> [2](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#redirection_messages) </sup>

- _300 Multiple Choice_: The request has more than one possible response. The user agent or user
  should choose one of them. (There is no standardized way of choosing one of the responses, but
  HTML links to the possibilities are recommended so the user can pick.)

- _301 Moved Permanently_: The URL of the requested resource has been changed permanently. The new
  URL is given in the response.

- _302 Found_: This response code means that the URI of requested resource has been changed
  temporarily. Further changes in the URI might be made in the future. Therefore, this same URI
  should be used by the client in future requests.

- _303 See Other_: The server sent this response to direct the client to get the requested resource
  at another URI with a GET request.

- _304 Not Modified_: This is used for caching purposes. It tells the client that the response has
  not been modified, so the client can continue to use the same cached version of the response.

- _305 Use Proxy_: Defined in a previous version of the HTTP specification to indicate that a
  requested response must be accessed by a proxy. It has been deprecated due to security concerns
  regarding in-band configuration of a proxy.

- _306 unused_: This response code is no longer used; it is just reserved. It was used in a previous
  version of the HTTP/1.1 specification.

- _307 Temporary Redirect_: The server sends this response to direct the client to get the requested
  resource at another URI with same method that was used in the prior request. This has the same
  semantics as the 302 Found HTTP response code, with the exception that the user agent must not
  change the HTTP method used: if a POST was used in the first request, a POST must be used in the
  second request.

- _308 Permanent Redirect_: This means that the resource is now permanently located at another URI,
  specified by the Location: HTTP Response header. This has the same semantics as the 301 Moved
  Permanently HTTP response code, with the exception that the user agent must not change the HTTP
  method used: if a POST was used in the first request, a POST must be used in the second request.

</details>


<details>
  <summary>References and sources</summary>

1. [3XX codes in the HTTP protocol](https://datatracker.ietf.org/doc/html/rfc2616#section-10.3)

2. [HTTP redirection messages on Mozilla](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#redirection_messages)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

You can identify exactly what HTTP response code your web server send back to your clients by opening the Netdata
dashboard and inspecting the `detailed_response_codes` chart for your web server. This chart keeps
track of exactly what error codes your web server sends out.

You should also check the server error logs. For example, web servers such as Apache or Nginx
produce and error logs, by default under `/var/log/{nginx, apache2}/{access.log, error.log}`

</details>



