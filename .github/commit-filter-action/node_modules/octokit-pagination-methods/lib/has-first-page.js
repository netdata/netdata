module.exports = hasFirstPage

const deprecate = require('./deprecate')
const getPageLinks = require('./get-page-links')

function hasFirstPage (link) {
  deprecate(`octokit.hasFirstPage() – You can use octokit.paginate or async iterators instead: https://github.com/octokit/rest.js#pagination.`)
  return getPageLinks(link).first
}
