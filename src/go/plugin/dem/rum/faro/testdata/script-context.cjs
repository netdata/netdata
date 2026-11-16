// SPDX-License-Identifier: GPL-3.0-or-later
// Model the native getter independently of Document's named-property lookup.
module.exports = function installScriptContext(context) {
  const script = context.document.currentScript;
  context.Document = class Document {
    get currentScript() { return script; }
  };
};
