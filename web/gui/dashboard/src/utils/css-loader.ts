let loadCssPromise: Promise<void>

type LoadCss = (href: string) => Promise<void>
export const loadCss: LoadCss = (href) => {
  if (loadCssPromise) {
    return loadCssPromise
  }
  return new Promise((resolve, reject) => {
    const fileRef = document.createElement("link")
    fileRef.setAttribute("rel", "stylesheet")
    fileRef.setAttribute("type", "text/css")
    fileRef.setAttribute("href", href)

    fileRef.onload = () => {
      resolve()
    }

    fileRef.onerror = () => {
      reject(Error(`Error loading css: ${href}`))
    }

    document.getElementsByTagName("head")[0].appendChild(fileRef)
  })
}
