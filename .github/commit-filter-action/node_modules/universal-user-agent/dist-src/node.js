import osName from "os-name";
export function getUserAgent() {
    try {
        return `Node.js/${process.version.substr(1)} (${osName()}; ${process.arch})`;
    }
    catch (error) {
        if (/wmic os get Caption/.test(error.message)) {
            return "Windows <version undetectable>";
        }
        throw error;
    }
}
