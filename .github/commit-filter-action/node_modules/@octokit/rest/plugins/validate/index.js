module.exports = octokitValidate;

const validate = require("./validate");

function octokitValidate(octokit) {
  octokit.hook.before("request", validate.bind(null, octokit));
}
