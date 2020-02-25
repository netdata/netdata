module.exports = paginatePlugin;

const { paginateRest } = require("@octokit/plugin-paginate-rest");

function paginatePlugin(octokit) {
  Object.assign(octokit, paginateRest(octokit));
}
