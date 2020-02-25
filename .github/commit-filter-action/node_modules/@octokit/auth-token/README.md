# auth-token.js

> GitHub API token authentication for browsers and Node.js

[![@latest](https://img.shields.io/npm/v/@octokit/auth-token.svg)](https://www.npmjs.com/package/@octokit/auth-token)
[![Build Status](https://travis-ci.com/octokit/auth-token.js.svg?branch=master)](https://travis-ci.com/octokit/auth-token.js)
[![Greenkeeper](https://badges.greenkeeper.io/octokit/auth-token.js.svg)](https://greenkeeper.io/)

`@octokit/auth-token` is the simplest of [GitHub’s authentication strategies](https://github.com/octokit/auth.js).

It is useful if you want to support multiple authentication strategies, as it’s API is compatible with its sibling packages for [basic](https://github.com/octokit/auth-basic.js), [GitHub App](https://github.com/octokit/auth-app.js) and [OAuth app](https://github.com/octokit/auth.js) authentication.

<!-- toc -->

## Usage

<table>
<tbody valign=top align=left>
<tr><th>
Browsers
</th><td width=100%>

Load `@octokit/auth-token` directly from [cdn.pika.dev](https://cdn.pika.dev)

```html
<script type="module">
  import { createTokenAuth } from "https://cdn.pika.dev/@octokit/auth-token";
</script>
```

</td></tr>
<tr><th>
Node
</th><td>

Install with <code>npm install @octokit/auth-token</code>

```js
const { createTokenAuth } = require("@octokit/auth-token");
// or: import { createTokenAuth } from "@octokit/auth-token";
```

</td></tr>
</tbody>
</table>

```js
const auth = createTokenAuth("1234567890abcdef1234567890abcdef12345678");
const authentication = await auth();
// {
//   type: 'token',
//   token: '1234567890abcdef1234567890abcdef12345678',
//   tokenType: 'oauth'
```

## `createTokenAuth(token) options`

The `createTokenAuth` method accepts a single argument of type string, which is the token. The passed token can be one of the following:

- [Personal access token](https://help.github.com/en/articles/creating-a-personal-access-token-for-the-command-line)
- [OAuth access token](https://developer.github.com/apps/building-oauth-apps/authorizing-oauth-apps/)
- Installation access token ([GitHub App Installation](https://developer.github.com/apps/building-github-apps/authenticating-with-github-apps/#authenticating-as-an-installation))
- [GITHUB_TOKEN provided to GitHub Actions](https://developer.github.com/actions/creating-github-actions/accessing-the-runtime-environment/#environment-variables)

Examples

```js
// Personal access token or OAuth access token
createTokenAuth("1234567890abcdef1234567890abcdef12345678");

// Installation access token or GitHub Action token
createTokenAuth("v1.d3d433526f780fbcc3129004e2731b3904ad0b86");
```

## `auth()`

The `auth()` method has no options. It returns a promise which resolves with the the authentication object.

## Authentication object

<table width="100%">
  <thead align=left>
    <tr>
      <th width=150>
        name
      </th>
      <th width=70>
        type
      </th>
      <th>
        description
      </th>
    </tr>
  </thead>
  <tbody align=left valign=top>
    <tr>
      <th>
        <code>type</code>
      </th>
      <th>
        <code>string</code>
      </th>
      <td>
        <code>"token"</code>
      </td>
    </tr>
    <tr>
      <th>
        <code>token</code>
      </th>
      <th>
        <code>string</code>
      </th>
      <td>
        The provided token.
      </td>
    </tr>
    <tr>
      <th>
        <code>tokenType</code>
      </th>
      <th>
        <code>string</code>
      </th>
      <td>
        Can be either <code>"oauth"</code> for personal access tokens and OAuth tokens, or <code>"installation"</code> for installation access tokens (includes <code>GITHUB_TOKEN</code> provided to GitHub Actions)
      </td>
    </tr>
  </tbody>
</table>

## `auth.hook(request, route, options)` or `auth.hook(request, options)`

`auth.hook()` hooks directly into the request life cycle. It authenticates the request using the provided token.

The `request` option is an instance of [`@octokit/request`](https://github.com/octokit/request.js#readme). The `route`/`options` parameters are the same as for the [`request()` method](https://github.com/octokit/request.js#request).

`auth.hook()` can be called directly to send an authenticated request

```js
const { data: authorizations } = await auth.hook(
  request,
  "GET /authorizations"
);
```

Or it can be passed as option to [`request()`](https://github.com/octokit/request.js#request).

```js
const requestWithAuth = request.defaults({
  request: {
    hook: auth.hook
  }
});

const { data: authorizations } = await requestWithAuth("GET /authorizations");
```

## Find more information

`auth()` does not send any requests, it only transforms the provided token string into an authentication object.

Here is a list of things you can do to retrieve further information

### Find out what scopes are enabled for oauth tokens

Note that this does not work for installations. There is no way to retrieve permissions based on an installation access tokens.

```js
const TOKEN = "1234567890abcdef1234567890abcdef12345678";

const auth = createTokenAuth(TOKEN);
const authentication = await auth();

const response = await request("HEAD /", {
  headers: authentication.headers
});
const scopes = response.headers["x-oauth-scopes"].split(/,\s+/);

if (scopes.length) {
  console.log(
    `"${TOKEN}" has ${scopes.length} scopes enabled: ${scopes.join(", ")}`
  );
} else {
  console.log(`"${TOKEN}" has no scopes enabled`);
}
```

### Find out if token is a personal access token or if it belongs to an OAuth app

```js
const TOKEN = "1234567890abcdef1234567890abcdef12345678";

const auth = createTokenAuth(TOKEN);
const authentication = await auth();

const response = await request("HEAD /", {
  headers: authentication.headers
});
const clientId = response.headers["x-oauth-client-id"];

if (clientId) {
  console.log(
    `"${token}" is an OAuth token, its app’s client_id is ${clientId}.`
  );
} else {
  console.log(`"${token}" is a personal access token`);
}
```

### Find out what permissions are enabled for a repository

Note that the `permissions` key is not set when authenticated using an installation access token.

```js
const TOKEN = "1234567890abcdef1234567890abcdef12345678";

const auth = createTokenAuth(TOKEN);
const authentication = await auth();

const response = await request("GET /repos/:owner/:repo", {
  owner: 'octocat',
  repo: 'hello-world'
  headers: authentication.headers
});

console.log(response.data.permissions)
// {
//   admin: true,
//   push: true,
//   pull: true
// }
```

### Use token for git operations

Both OAuth and installation access tokens can be used for git operations. However, when using with an installation, [the token must be prefixed with `x-access-token`](https://developer.github.com/apps/building-github-apps/authenticating-with-github-apps/#http-based-git-access-by-an-installation).

This example is using the [`execa`](https://github.com/sindresorhus/execa) package to run a `git push` command.

```js
const TOKEN = "1234567890abcdef1234567890abcdef12345678";

const auth = createTokenAuth(TOKEN);
const { token, tokenType } = await auth();
const tokenWithPrefix =
  tokenType === "installation" ? `x-access-token:${token}` : token;

const repositoryUrl = `https://${tokenWithPrefix}@github.com/octocat/hello-world.git`;

const { stdout } = await execa("git", ["push", repositoryUrl]);
console.log(stdout);
```

## License

[MIT](LICENSE)
