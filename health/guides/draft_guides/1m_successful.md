# 1m_successful

**Web Server | Web log**

HTTP response status codes indicate whether a specific HTTP request has been successfully completed or not.

The Netdata Agent calculates the ratio of successful HTTP requests over the last minute. These requests consist of 1xx,
2xx, 304, 401 response codes.

<details>
  <summary>See more about the response codes this alert track </summary>

The response codes below contain the descriptions as provided by
Mozilla. <sup> [1](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status) </sup>

**Information responses (1XX)**

- 100 Continue 
  This interim response indicates that the client should continue the request or ignore the response if the
  request is already finished.

- 101 Switching Protocol 
  This code is sent in response to an Upgrade request header from the client and indicates the
  protocol the server is switching to.

- 102 Processing (WebDAV)
  This code indicates that the server has received and is processing the request, but no response is available yet.

- 103 Early Hints 
  This status code is primarily intended to be used with the Link header, letting the user agent start
  preloading resources while the server prepares a response.

**Successful responses (2XX)**

- 200 OK 
  The request succeeded. The result meaning of "success" depends on the HTTP method:

    1. GET: The resource has been fetched and transmitted in the message body.
    1. HEAD: The representation headers are included in the response without any message body.
    1. PUT or POST: The resource describing the result of the action is transmitted in the message body.
    1. TRACE: The message body contains the request message as received by the server.

- 201 Created 
  The request succeeded, and a new resource created as a result. This is typically the response sent after
  POST requests, or some PUT requests.

- 202 Accepted 
  The request has been received but not yet acted upon. It is noncommittal, since there is no way in HTTP
  to later send an asynchronous response indicating the outcome of the request. It is intended for cases where another
  process or server handles the request, or for batch processing.

- 203 Non-Authoritative 
  Information This response code means the returned metadata is not exactly the same as is
  available from the origin server, but is collected from a local or a third-party copy. This is mostly used for mirrors
  or backups of another resource. Except for that specific case, the 200 OK response is preferred to this status.

- 204 No Content 
  There is no content to send for this request, but the headers may be useful. The user agent may update
  its cached headers for this resource with the new ones.

- 205 Reset Content 
  Tells the user agent to reset the document which sent this request.

- 206 Partial Content 
  This response code is used when the Range header is sent from the client to request only part of a
  resource.

- 207 Multi-Status (WebDAV)
  Conveys information about multiple resources, for situations where multiple status codes might be appropriate.

- 208 Already Reported (WebDAV)
  Used inside a <dav:propstat> response element to avoid repeatedly enumerating the internal members of multiple
  bindings to the same collection.

- 226 IM Used (HTTP Delta encoding)
  The server has fulfilled a GET request for the resource, and the response is a representation of the result of one or
  more instance-manipulations applied to the current instance.

**Redirection messages (3XX)**

- 304 Not Modified 
  This is used for caching purposes. It tells the client that the response has not been modified, so
  the client can continue to use the same cached version of the response.

**Client error responses (4XX)**

- 401 Unauthorized 
  Although the HTTP standard specifies "unauthorized", semantically this response means "
  unauthenticated". That is, the client must authenticate itself to get the requested response.

</details>

<details>
  <summary>References and Sources </summary>

[[1] https://developer.mozilla.org/en-US/docs/Web/HTTP/Status](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status)

</details>

![](upload://p6TlBRJaDaG8NFPXHn0p5xbSXHA.png)