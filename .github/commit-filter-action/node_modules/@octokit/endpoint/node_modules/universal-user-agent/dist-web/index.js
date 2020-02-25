function getUserAgent() {
    try {
        return navigator.userAgent;
    }
    catch (e) {
        return "<environment undetectable>";
    }
}

export { getUserAgent };
//# sourceMappingURL=index.js.map
