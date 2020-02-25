module.exports = authenticationPlugin;

const { Deprecation } = require("deprecation");
const once = require("once");

const deprecateAuthenticate = once((log, deprecation) => log.warn(deprecation));

const authenticate = require("./authenticate");
const beforeRequest = require("./before-request");
const requestError = require("./request-error");

function authenticationPlugin(octokit, options) {
  if (options.auth) {
    octokit.authenticate = () => {
      deprecateAuthenticate(
        octokit.log,
        new Deprecation(
          '[@octokit/rest] octokit.authenticate() is deprecated and has no effect when "auth" option is set on Octokit constructor'
        )
      );
    };
    return;
  }
  const state = {
    octokit,
    auth: false
  };
  octokit.authenticate = authenticate.bind(null, state);
  octokit.hook.before("request", beforeRequest.bind(null, state));
  octokit.hook.error("request", requestError.bind(null, state));
}
