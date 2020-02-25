# http-error.js

> Error class for Octokit request errors

[![@latest](https://img.shields.io/npm/v/@octokit/request-error.svg)](https://www.npmjs.com/package/@octokit/request-error)
[![Build Status](https://travis-ci.com/octokit/request-error.js.svg?branch=master)](https://travis-ci.com/octokit/request-error.js)
[![Greenkeeper](https://badges.greenkeeper.io/octokit/request-error.js.svg)](https://greenkeeper.io/)

## Usage

<table>
<tbody valign=top align=left>
<tr><th>
Browsers
</th><td width=100%>
Load <code>@octokit/request-error</code> directly from <a href="https://cdn.pika.dev">cdn.pika.dev</a>
        
```html
<script type="module">
import { RequestError } from "https://cdn.pika.dev/@octokit/request-error";
</script>
```

</td></tr>
<tr><th>
Node
</th><td>

Install with <code>npm install @octokit/request-error</code>

```js
const { RequestError } = require("@octokit/request-error");
// or: import { RequestError } from "@octokit/request-error";
```

</td></tr>
</tbody>
</table>

```js
const error = new RequestError("Oops", 500, {
  headers: {
    "x-github-request-id": "1:2:3:4"
  }, // response headers
  request: {
    method: "POST",
    url: "https://api.github.com/foo",
    body: {
      bar: "baz"
    },
    headers: {
      authorization: "token secret123"
    }
  }
});

error.message; // Oops
error.status; // 500
error.headers; // { 'x-github-request-id': '1:2:3:4' }
error.request.method; // POST
error.request.url; // https://api.github.com/foo
error.request.body; // { bar: 'baz' }
error.request.headers; // { authorization: 'token [REDACTED]' }
```

## LICENSE

[MIT](LICENSE)
