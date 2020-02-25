const test = require('tap').test
const nock = require('nock')

const Octokit = require('@octokit/rest')
  .plugin(require('.'))

test('@octokit/pagination-methods', (t) => {
  nock('https://api.github.com', {
    reqheaders: {
      authorization: 'token secrettoken123'
    }
  })
    .get('/organizations')
    .query({ page: 3, per_page: 1 })
    .reply(200, [{}], {
      'Link': '<https://api.github.com/organizations?page=4&per_page=1>; rel="next", <https://api.github.com/organizations?page=1&per_page=1>; rel="first", <https://api.github.com/organizations?page=2&per_page=1>; rel="prev"',
      'X-GitHub-Media-Type': 'octokit.v3; format=json'
    })
    .get('/organizations')
    .query({ page: 1, per_page: 1 })
    .reply(200, [{}])
    .get('/organizations')
    .query({ page: 2, per_page: 1 })
    .reply(200, [{}])
    .get('/organizations')
    .query({ page: 4, per_page: 1 })
    .reply(404, {})

  const octokit = new Octokit()

  octokit.authenticate({
    type: 'token',
    token: 'secrettoken123'
  })

  return octokit.orgs.getAll({
    page: 3,
    per_page: 1
  })

    .then((response) => {
      t.ok(octokit.hasNextPage(response))
      t.ok(octokit.hasPreviousPage(response))
      t.ok(octokit.hasFirstPage(response))
      t.notOk(octokit.hasLastPage(response))

      const noop = () => {}

      return Promise.all([
        octokit.getFirstPage(response)
          .then(response => {
            t.doesNotThrow(() => {
              octokit.hasPreviousPage(response)
            })
            t.notOk(octokit.hasPreviousPage(response))
          }),
        octokit.getPreviousPage(response, { foo: 'bar', accept: 'application/vnd.octokit.v3+json' }),
        octokit.getNextPage(response).catch(noop),
        octokit.getLastPage(response, { foo: 'bar' })
          .catch(error => {
            t.equals(error.code, 404)
          }),
        // test error with promise
        octokit.getLastPage(response).catch(noop)
      ])
    })

    .catch(t.error)
})

test('carries accept header correctly', () => {
  nock('https://api.github.com', {
    reqheaders: {
      accept: 'application/vnd.github.hellcat-preview+json'
    }
  })
    .get('/user/teams')
    .query({ per_page: 1 })
    .reply(200, [{}], {
      'Link': '<https://api.github.com/user/teams?page=2&per_page=1>; rel="next"',
      'X-GitHub-Media-Type': 'github; param=hellcat-preview; format=json'
    })
    .get('/user/teams')
    .query({ page: 2, per_page: 1 })
    .reply(200, [])

  const octokit = new Octokit()

  return octokit.users.getTeams({ per_page: 1 })
    .then(response => {
      return octokit.getNextPage(response)
    })
})
