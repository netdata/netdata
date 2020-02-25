function getUserAgent() {
    try {
        return navigator.userAgent;
    }
    catch (e) {
        return "<environment unknown>";
    }
}

export { getUserAgent };
//# sourceMappingURL=index.js.map
