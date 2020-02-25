module.exports = hasPreviousPage

const deprecate = require('./deprecate')
const getPageLinks = require('./get-page-links')

function hasPreviousPage (link) {
  deprecate(`octokit.hasPreviousPage() – You can use octokit.paginate or async iterators instead: https://github.com/octokit/rest.js#pagination.`)
  return getPageLinks(link).prev
}
