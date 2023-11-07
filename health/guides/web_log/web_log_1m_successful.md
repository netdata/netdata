# web_log_1m_successful

**Web Server | Web log**

HTTP response status codes indicate whether a specific HTTP request has been successfully completed
or not.

The Netdata Agent calculates the ratio of successful HTTP requests over the last minute. These
requests consist of 1xx, 2xx, 304, 401 response codes. You receive this alert in warning when the
percentage of successful requests is less than 85% and in critical when it is below 75%. This alert
can indicate:

- A malfunction in the services of your web server
- Malicious activity towards your website
- Broken links towards your servers.

In most cases, the Agent will send you another alert indicating high incidences
of "abnormal" HTTP requests code, for example you could also receive the `web_log_1m_bad_requests` alert.


<details>
  <summary>See more about the response codes this alert track </summary>

The response codes below contain the descriptions as provided by
Mozilla. <sup> [1](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status) </sup>

**Information responses (1XX)**

- _100 Continue_:This interim response indicates that the client should continue the request or ignore
  the response if the request is already finished.

- _101 Switching Protocol_: This code is sent in response to an Upgrade request header from the client
  and indicates the protocol the server is switching to.

- _102 Processing (WebDAV)_:
  This code indicates that the server has received and is processing the request, but no response is
  available yet.

- _103 Early Hints_: This status code is primarily intended to be used with the link header, letting
  the user agent start preloading resources while the server prepares a response.

**Successful responses (2XX)**

- _200 OK_: The request succeeded. The result meaning of "success" depends on the HTTP method:

    * GET: The resource has been fetched and transmitted in the message body.
    * HEAD: The representation headers are included in the response without any message body.
    * PUT or POST: The resource describing the result of the action is transmitted in the message
       body.
    * TRACE: The message body contains the request message as received by the server.

- _201 Created_: The request succeeded, and a new resource created as a result. This is typically the
  response sent after POST requests, or some PUT requests.

- _202 Accepted_: The request has been received but not yet acted upon. It is noncommittal, since there
  is no way in HTTP to later send an asynchronous response indicating the outcome of the request. It
  is intended for cases where another process or server handles the request, or for batch
  processing.

- _203 Non-Authoritative Information_: This response code means the returned metadata is not exactly
  the same as is available from the origin server, but is collected from a local or a third-party
  copy. This is mostly used for mirrors or backups of another resource. Except for that specific
  case, the 200 OK response is preferred to this status.

- _204 No Content_: There is no content to send for this request, but the headers may be useful. The
  user agent may update its cached headers for this resource with the new ones.

- _205 Reset Content_: Tells the user agent to reset the document which sent this request.

- _206 Partial Content_: This response code is used when the range header is sent from the client to
  request only part of a resource.

- _207 Multi-Status (WebDAV)_:
  Conveys information about multiple resources, for situations where multiple status codes might be
  appropriate.

- _208 Already Reported (WebDAV)_:
  Used inside a <dav:propstat> response element to avoid repeatedly enumerating the internal members
  of multiple bindings to the same collection.

- _226 IM Used (HTTP Delta encoding)_:
  The server has fulfilled a GET request for the resource, and the response is a representation of
  the result of one or more instance-manipulations applied to the current instance.

**Redirection messages (3XX)**

- _304 Not Modified _: This is used for caching purposes. It tells the client that the response has not
  been modified, so the client can continue to use the same cached version of the response.

**Client error responses (4XX)**

- _401 Unauthorized_: Although the HTTP standard specifies "unauthorized", semantically this response
  means "
  unauthenticated". That is, the client must authenticate itself to get the requested response.

</details>

<details>
  <summary>References and Sources </summary>

1. [HTTP status codes on Mozilla](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status)

</details>

### Troubleshooting section:

There are a number of reasons triggering this alert. All of them could eventually cause bad user
experience with your web services.

<details> 
<summary>General approach</summary>

Identify exactly what HTTP response code your web server sent back to your clients. 
Open the Netdata dashboard and inspect the `detailed_response_codes` chart for your web server. This chart keeps
track of exactly what error codes your web server sends out.

</details>
  
