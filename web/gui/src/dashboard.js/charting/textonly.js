
// ----------------------------------------------------------------------------------------------------------------
// "Text-only" chart - Just renders the raw value to the DOM

NETDATA.textOnlyCreate = function(state, data) {
    // Round to one decimal place
    state.element.innerHTML = Math.round(data.result[0] * 10) / 10;
    return true;
}

NETDATA.textOnlyUpdate = NETDATA.textOnlyCreate;