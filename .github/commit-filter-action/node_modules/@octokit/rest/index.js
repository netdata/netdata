const { requestLog } = require("@octokit/plugin-request-log");
const {
  restEndpointMethods
} = require("@octokit/plugin-rest-endpoint-methods");

const Core = require("./lib/core");

const CORE_PLUGINS = [
  require("./plugins/authentication"),
  require("./plugins/authentication-deprecated"), // deprecated: remove in v17
  requestLog,
  require("./plugins/pagination"),
  restEndpointMethods,
  require("./plugins/validate"),

  require("octokit-pagination-methods") // deprecated: remove in v17
];

const OctokitRest = Core.plugin(CORE_PLUGINS);

function DeprecatedOctokit(options) {
  const warn =
    options && options.log && options.log.warn
      ? options.log.warn
      : console.warn;
  warn(
    '[@octokit/rest] `const Octokit = require("@octokit/rest")` is deprecated. Use `const { Octokit } = require("@octokit/rest")` instead'
  );
  return new OctokitRest(options);
}

const Octokit = Object.assign(DeprecatedOctokit, {
  Octokit: OctokitRest
});

Object.keys(OctokitRest).forEach(key => {
  /* istanbul ignore else */
  if (OctokitRest.hasOwnProperty(key)) {
    Octokit[key] = OctokitRest[key];
  }
});

module.exports = Octokit;
