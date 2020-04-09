
// ----------------------------------------------------------------------------------------------------------------
// "Status Image" chart - Displays binary status images in the DOM

NETDATA.statusImageCreate = function(state, data) {
    var trueImage = NETDATA.dataAttribute(state.element, 'statusimage-true-image', '');
    var falseImage = NETDATA.dataAttribute(state.element, 'statusimage-false-image', '');

    var imageUrl = trueImage;
    if (data.result[0] == 0) imageUrl = falseImage;

    state.element.innerHTML = "<img src=" + imageUrl + "></img>";
    return true;
}

NETDATA.statusImageUpdate = NETDATA.statusImageCreate;

