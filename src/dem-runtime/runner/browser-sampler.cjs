// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const { encoded } = require("./failure-evidence.cjs");
const { safeText } = require("./browser-summary.cjs");

// Keep the only observer API call in this file: the shared diagnosis view uses
// its canonical path and pw:api category, never a user-controlled step title.
module.exports = function sample(args) {
  const [first, ...rest] = args.slice(0, 4);
  return first
    .evaluate((value, remaining) => {
      // Application code can replace page globals and prototype methods.
      const output = [null, null, null, null];
      output.length = remaining.length + 1;
      for (let index = 0; index < output.length; index++) {
        const item = index === 0 ? value : remaining[index - 1],
          type = typeof item;
        if (type === "string")
          output[index] =
            item.length <= 512 ? { value: item } : { omitted: "too_long" };
        else if (item === null || type === "boolean")
          output[index] = { value: item };
        else if (type === "number")
          output[index] =
            item === item && item !== 1 / 0 && item !== -1 / 0
              ? { value: item }
              : { omitted: "non_finite" };
        // Playwright 1.63 can deliver BigInt handles as undefined. Do not claim
        // the original browser type when the provider cannot represent it.
        else
          output[index] = {
            omitted:
              type === "undefined" || type === "bigint"
                ? "unrepresentable"
                : type,
          };
      }
      return output;
    }, rest)
    .then((values) =>
      values.map((item) => {
        if (!Object.hasOwn(item, "value")) return item;
        const value =
          typeof item.value === "string" ? safeText(item.value) : item.value;
        return encoded(value) <= 512 ? { value } : { omitted: "too_long" };
      }),
    );
};
