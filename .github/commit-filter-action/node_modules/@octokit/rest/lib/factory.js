module.exports = factory;

const Octokit = require("./constructor");
const registerPlugin = require("./register-plugin");

function factory(plugins) {
  const Api = Octokit.bind(null, plugins || []);
  Api.plugin = registerPlugin.bind(null, plugins || []);
  return Api;
}
