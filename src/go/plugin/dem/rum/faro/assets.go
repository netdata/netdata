// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import _ "embed"

const (
	sdkSHA256     = "d7be021a7344131c89c02cf5b39aa9825c21cf17482bc1cd47e23cd7023ce118"
	tracingSHA256 = "363695cfc004222dd210cb040b95ef7943ea02f189e6fda4485da44154031aca"
	noticesSHA256 = "5d51662be3b74928d69b9fe6f1b331882a62f87e15d7c74c673b71cb1e1b1e41"

	// SDKPath and TracingPath are immutable asset paths relative to /rum/.
	SDKPath     = "assets/faro-web-sdk-" + Version + "-" + sdkSHA256 + ".js"
	TracingPath = "assets/faro-web-tracing-" + Version + "-" + tracingSHA256 + ".js"
	// NoticesPath redistributes the bundled software's license texts and attributions.
	NoticesPath = "assets/notices-" + noticesSHA256 + ".txt"
)

//go:embed assets/faro-web-sdk.iife.js
var sdkContent string

//go:embed assets/faro-web-tracing.iife.js
var tracingContent string

//go:embed assets/NOTICES.txt
var noticesContent string

// AssetData holds immutable embedded content. Strings avoid mutable shared byte slices.
type AssetData struct {
	Content     string
	ETag        string
	ContentType string
}

// LookupAsset accepts only an exact published filename, never a filesystem path.
func LookupAsset(name string) (AssetData, bool) {
	const jsType = "application/javascript; charset=utf-8"
	switch "assets/" + name {
	case SDKPath:
		return AssetData{
			Content:     sdkContent,
			ETag:        `"` + sdkSHA256 + `"`,
			ContentType: jsType,
		}, true
	case TracingPath:
		return AssetData{
			Content:     tracingContent,
			ETag:        `"` + tracingSHA256 + `"`,
			ContentType: jsType,
		}, true
	case NoticesPath:
		return AssetData{
			Content:     noticesContent,
			ETag:        `"` + noticesSHA256 + `"`,
			ContentType: "text/plain; charset=utf-8",
		}, true
	default:
		return AssetData{}, false
	}
}
