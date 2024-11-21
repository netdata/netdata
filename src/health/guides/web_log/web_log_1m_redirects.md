### Understand the alert

HTTP response status codes indicate whether a specific HTTP request has been successfully completed or not.

The 3XX class of status code indicates that further action needs to be taken by the user Agent in order to fulfill the request. The action required may be carried out by the user Agent without interaction with the user if and only if the method used in the second request is GET or HEAD. A client SHOULD detect infinite redirection loops, since such loops generate network traffic for each redirection.

The Netdata Agent calculates the ratio of redirection HTTP requests over the last minute. This metric does not include the "304 Not modified" message.

### Troubleshoot the alert

You can identify exactly what HTTP response code your web server send back to your clients, by opening the Netdata dashboard and inspecting the `detailed_response_codes` chart for your web server. This chart keeps
track of exactly what error codes your web server sends out. 

You should also check the server error logs. For example, web servers such as Apache or Nginx produce and error logs, by default under `/var/log/{nginx, apache2}/{access.log, error.log}`

### Useful resources

1. [3XX codes in the HTTP protocol](https://datatracker.ietf.org/doc/html/rfc2616#section-10.3)

2. [HTTP redirection messages on Mozilla](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#redirection_messages)


