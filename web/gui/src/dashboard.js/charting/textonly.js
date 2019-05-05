
// ----------------------------------------------------------------------------------------------------------------
// "Text-only" chart - Just renders the raw value to the DOM

NETDATA.textOnlyCreate = function(state, data) {
    var decimalPlaces = NETDATA.dataAttribute(state.element, 'textonly-decimal-places', 1);
    var prefix = NETDATA.dataAttribute(state.element, 'textonly-prefix', '');
    var suffix = NETDATA.dataAttribute(state.element, 'textonly-suffix', '');

    // Round based on number of decimal places to show
    var precision = Math.pow(10, decimalPlaces);
    var value = Math.round(data.result[0] * precision) / precision;

    state.element.textContent = prefix + value + suffix;
    return true;
}

NETDATA.textOnlyUpdate = NETDATA.textOnlyCreate;