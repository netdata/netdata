export function omit(object, keysToOmit) {
    return Object.keys(object)
        .filter(option => !keysToOmit.includes(option))
        .reduce((obj, key) => {
        obj[key] = object[key];
        return obj;
    }, {});
}
