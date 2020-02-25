module.exports = paginationMethodsPlugin

function paginationMethodsPlugin (octokit) {
  octokit.getFirstPage = require('./lib/get-first-page').bind(null, octokit)
  octokit.getLastPage = require('./lib/get-last-page').bind(null, octokit)
  octokit.getNextPage = require('./lib/get-next-page').bind(null, octokit)
  octokit.getPreviousPage = require('./lib/get-previous-page').bind(null, octokit)
  octokit.hasFirstPage = require('./lib/has-first-page')
  octokit.hasLastPage = require('./lib/has-last-page')
  octokit.hasNextPage = require('./lib/has-next-page')
  octokit.hasPreviousPage = require('./lib/has-previous-page')
}
