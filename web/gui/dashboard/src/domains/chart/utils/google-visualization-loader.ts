let fetchPromise: Promise<string>

const GOOGLE_JS_API_SRC = "https://www.google.com/jsapi"

export const loadGoogleVisualizationApi = () => {
  if (fetchPromise) {
    return fetchPromise
  }
  fetchPromise = new Promise((resolve, reject) => {
    setTimeout(() => {
      const script = document.createElement("script")
      script.type = "text/javascript"
      script.async = true
      script.src = GOOGLE_JS_API_SRC

      script.onerror = () => {
        reject(Error("error loading google.js api"))
      }
      script.onload = () => {
        resolve("ok")
      }

      const firstScript = document.getElementsByTagName("script")[0] as HTMLScriptElement
      (firstScript.parentNode as Node).insertBefore(script, firstScript)
    }, 1000)
  }).then(() => new Promise((resolve) => {
    window.google.load("visualization", "1.1", {
      packages: ["corechart", "controls"],
      callback: resolve,
    })
  }))
  return fetchPromise
}
