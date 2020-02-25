module.exports = hasNextPage

const deprecate = require('./deprecate')
const getPageLinks = require('./get-page-links')

function hasNextPage (link) {
  deprecate(`octokit.hasNextPage() – You can use octokit.paginate or async iterators instead: https://github.com/octokit/rest.js#pagination.`)
  return getPageLinks(link).next
}
