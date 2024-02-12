<!--
title: "web"
custom_edit_url: "https://github.com/netdata/go.d.plugin/edit/master/pkg/web/README.md"
sidebar_label: "web"
learn_status: "Published"
learn_rel_path: "Developers/External plugins/go.d.plugin/Helper Packages"
-->

# web

This package contains HTTP related configurations (`Request`, `Client` and `HTTP` structs) and functions to
create `http.Request` and `http.Client` from them.

`HTTP` embeds both `Request` and `Client`.

Every module that collects metrics doing HTTP requests should use `HTTP`. It allows to have same set of user
configurable options across all modules.

## Configuration options

HTTP request options:

- `url`: the URL to access.
- `username`: the username for basic HTTP authentication.
- `password`: the password for basic HTTP authentication.
- `proxy_username`: the username for basic HTTP authentication of a user agent to a proxy server.
- `proxy_password`: the password for basic HTTP authentication of a user agent to a proxy server.
- `body`: the HTTP request body to be sent by the client.
- `method`: the HTTP method (GET, POST, PUT, etc.).
- `headers`: the HTTP request header fields to be sent by the client.

HTTP client options:

- `timeout`: the HTTP request time limit.
- `not_follow_redirects`: the policy for handling redirects.
- `proxy_url`: the URL of the proxy to use.
- `tls_skip_verify`: controls whether a client verifies the server's certificate chain and host name.
- `tls_ca`: certificate authority to use when verifying server certificates.
- `tls_cert`: tls certificate to use.
- `tls_key`: tls key to use.

## Usage

Just make `HTTP` part of your module configuration.

```go
package example

import "github.com/netdata/go.d.plugin/pkg/web"

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Example struct {
	Config `yaml:",inline"`
}

func (e *Example) Init() bool {
	httpReq, err := web.NewHTTPRequest(e.Request)
	if err != nil {
		// ...
		return false
	}

	httpClient, err := web.NewHTTPClient(e.Client)
	if err != nil {
		// ...
		return false
	}

	// ...
	return true
}
```

Having `HTTP` embedded your configuration inherits all [configuration options](#configuration-options):

```yaml
jobs:
  - name: name
    url: url
    username: username
    password: password
    proxy_url: proxy_url
    proxy_username: proxy_username
    proxy_password: proxy_password
    timeout: 1
    method: GET
    body: '{"key": "value"}'
    headers:
      X-API-Key: key
    not_follow_redirects: no
    tls_skip_verify: no
    tls_ca: path/to/ca.pem
    tls_cert: path/to/cert.pem
    tls_key: path/to/key.pem
```
