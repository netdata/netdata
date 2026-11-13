// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
// Original step objects and trees remain untouched. Only the canonical sampler is hidden.
module.exports = function createStepView(observerFile) {
  const visible = (step) =>
    !(
      observerFile &&
      step.category === "pw:api" &&
      step.location?.file === observerFile
    );
  const parent = (step) => {
    let value = step.parent;
    while (value && !visible(value)) value = value.parent;
    return value;
  };
  function children(steps) {
    const cursors = [{ steps, index: 0 }];
    const iterator = {
      incomplete: false,
      [Symbol.iterator]() {
        return this;
      },
      next() {
        while (cursors.length) {
          const cursor = cursors.at(-1);
          if (cursor.index === cursor.steps.length) {
            cursors.pop();
            continue;
          }
          const step = cursor.steps[cursor.index++];
          if (visible(step)) return { done: false, value: step };
          if (step.steps?.length) {
            if (cursors.length === 64) iterator.incomplete = true;
            else cursors.push({ steps: step.steps, index: 0 });
          }
        }
        return { done: true };
      },
    };
    return iterator;
  }
  return { visible, children, parent };
};
