module.exports = getFirstPage

const getPage = require('./get-page')

function getFirstPage (octokit, link, headers) {
  return getPage(octokit, link, 'first', headers)
}
