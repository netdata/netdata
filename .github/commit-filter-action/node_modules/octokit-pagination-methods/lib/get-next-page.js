module.exports = getNextPage

const getPage = require('./get-page')

function getNextPage (octokit, link, headers) {
  return getPage(octokit, link, 'next', headers)
}
