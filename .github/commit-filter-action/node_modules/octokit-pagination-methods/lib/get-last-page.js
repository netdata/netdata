module.exports = getLastPage

const getPage = require('./get-page')

function getLastPage (octokit, link, headers) {
  return getPage(octokit, link, 'last', headers)
}
