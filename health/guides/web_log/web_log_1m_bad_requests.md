### Understand the alert

HTTP response status codes indicate whether a specific HTTP request has been successfully completed or not.

The 4xx class of status code is intended for cases in which the client seems to have erred. Except when responding to a HEAD request, the server should include an entity containing an explanation of
the error situation, and whether it is a temporary or permanent condition. These status codes are applicable to any request method. 

The Netdata Agent calculates the ratio of client error HTTP requests over the last minute. This metric does not include the 401 errors.


### Troubleshoot the alert

To identify the HTTP response code your web server sends back: 

1. Open the Netdata dashboard.
2. Inspect the `detailed_response_codes` chart for your web server. This chart keeps track of exactly what error codes your web server sends out.

You should also check server logs for more details about how the server is handling the requests. For example, web servers such as Apache or Nginx produce two files called access.log and error.log (by default under `/var/log/{nginx, apache2}/{access.log, error.log}`)

3. Troubleshoot 404 codes on the server side

The 404 requests indicate outdated links on your website or in other websites that redirect to your website. To check for dead links on your on website, use a `broken link checker` software periodically.

### Useful resources

1. [https://datatracker.ietf.org/doc/html/rfc2616#section-10.4](https://datatracker.ietf.org/doc/html/rfc2616#section-10.4)
2. [https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses)