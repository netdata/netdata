module.exports = hasLastPage

const deprecate = require('./deprecate')
const getPageLinks = require('./get-page-links')

function hasLastPage (link) {
  deprecate(`octokit.hasLastPage() – You can use octokit.paginate or async iterators instead: https://github.com/octokit/rest.js#pagination.`)
  return getPageLinks(link).last
}
