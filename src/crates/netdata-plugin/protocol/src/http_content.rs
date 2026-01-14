#![allow(dead_code)]

use std::fmt;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum HttpContent {
    ApplicationJson,
    TextPlain,
    TextHtml,
    TextCss,
    TextYaml,
    ApplicationYaml,
    TextXml,
    TextXsl,
    ApplicationXml,
    ApplicationJavascript,
    ApplicationOctetStream,
    ImageSvgXml,
    ApplicationXFontTruetype,
    ApplicationXFontOpentype,
    ApplicationFontWoff,
    ApplicationFontWoff2,
    ApplicationVndMsFontobject,
    ImagePng,
    ImageJpeg,
    ImageGif,
    ImageXIcon,
    ImageBmp,
    ImageIcns,
    AudioMpeg,
    AudioOgg,
    VideoMp4,
    ApplicationPdf,
    ApplicationZip,
    Prometheus,
}

#[derive(Debug, Clone)]
pub struct HttpContentInfo {
    pub format: &'static str,
    pub content_type: HttpContent,
    pub needs_charset: bool,
    pub options: Option<&'static str>,
}

impl HttpContent {
    /// Get the primary format string for this content type
    pub fn as_str(&self) -> &'static str {
        self.info().format
    }

    /// Get full information about this content type
    pub fn info(&self) -> HttpContentInfo {
        use HttpContent::*;
        match self {
            ApplicationJson => HttpContentInfo {
                format: "application/json",
                content_type: ApplicationJson,
                needs_charset: true,
                options: None,
            },
            TextPlain => HttpContentInfo {
                format: "text/plain",
                content_type: TextPlain,
                needs_charset: true,
                options: None,
            },
            TextHtml => HttpContentInfo {
                format: "text/html",
                content_type: TextHtml,
                needs_charset: true,
                options: None,
            },
            TextCss => HttpContentInfo {
                format: "text/css",
                content_type: TextCss,
                needs_charset: true,
                options: None,
            },
            TextYaml => HttpContentInfo {
                format: "text/yaml",
                content_type: TextYaml,
                needs_charset: true,
                options: None,
            },
            ApplicationYaml => HttpContentInfo {
                format: "application/yaml",
                content_type: ApplicationYaml,
                needs_charset: true,
                options: None,
            },
            TextXml => HttpContentInfo {
                format: "text/xml",
                content_type: TextXml,
                needs_charset: true,
                options: None,
            },
            TextXsl => HttpContentInfo {
                format: "text/xsl",
                content_type: TextXsl,
                needs_charset: true,
                options: None,
            },
            ApplicationXml => HttpContentInfo {
                format: "application/xml",
                content_type: ApplicationXml,
                needs_charset: true,
                options: None,
            },
            ApplicationJavascript => HttpContentInfo {
                format: "application/javascript",
                content_type: ApplicationJavascript,
                needs_charset: true,
                options: None,
            },
            ApplicationOctetStream => HttpContentInfo {
                format: "application/octet-stream",
                content_type: ApplicationOctetStream,
                needs_charset: false,
                options: None,
            },
            ImageSvgXml => HttpContentInfo {
                format: "image/svg+xml",
                content_type: ImageSvgXml,
                needs_charset: false,
                options: None,
            },
            ApplicationXFontTruetype => HttpContentInfo {
                format: "application/x-font-truetype",
                content_type: ApplicationXFontTruetype,
                needs_charset: false,
                options: None,
            },
            ApplicationXFontOpentype => HttpContentInfo {
                format: "application/x-font-opentype",
                content_type: ApplicationXFontOpentype,
                needs_charset: false,
                options: None,
            },
            ApplicationFontWoff => HttpContentInfo {
                format: "application/font-woff",
                content_type: ApplicationFontWoff,
                needs_charset: false,
                options: None,
            },
            ApplicationFontWoff2 => HttpContentInfo {
                format: "application/font-woff2",
                content_type: ApplicationFontWoff2,
                needs_charset: false,
                options: None,
            },
            ApplicationVndMsFontobject => HttpContentInfo {
                format: "application/vnd.ms-fontobject",
                content_type: ApplicationVndMsFontobject,
                needs_charset: false,
                options: None,
            },
            ImagePng => HttpContentInfo {
                format: "image/png",
                content_type: ImagePng,
                needs_charset: false,
                options: None,
            },
            ImageJpeg => HttpContentInfo {
                format: "image/jpeg",
                content_type: ImageJpeg,
                needs_charset: false,
                options: None,
            },
            ImageGif => HttpContentInfo {
                format: "image/gif",
                content_type: ImageGif,
                needs_charset: false,
                options: None,
            },
            ImageXIcon => HttpContentInfo {
                format: "image/x-icon",
                content_type: ImageXIcon,
                needs_charset: false,
                options: None,
            },
            ImageBmp => HttpContentInfo {
                format: "image/bmp",
                content_type: ImageBmp,
                needs_charset: false,
                options: None,
            },
            ImageIcns => HttpContentInfo {
                format: "image/icns",
                content_type: ImageIcns,
                needs_charset: false,
                options: None,
            },
            AudioMpeg => HttpContentInfo {
                format: "audio/mpeg",
                content_type: AudioMpeg,
                needs_charset: false,
                options: None,
            },
            AudioOgg => HttpContentInfo {
                format: "audio/ogg",
                content_type: AudioOgg,
                needs_charset: false,
                options: None,
            },
            VideoMp4 => HttpContentInfo {
                format: "video/mp4",
                content_type: VideoMp4,
                needs_charset: false,
                options: None,
            },
            ApplicationPdf => HttpContentInfo {
                format: "application/pdf",
                content_type: ApplicationPdf,
                needs_charset: false,
                options: None,
            },
            ApplicationZip => HttpContentInfo {
                format: "application/zip",
                content_type: ApplicationZip,
                needs_charset: false,
                options: None,
            },
            Prometheus => HttpContentInfo {
                format: "text/plain",
                content_type: Prometheus,
                needs_charset: true,
                options: Some("version=0.0.4"),
            },
        }
    }

    /// Parse a content type from a string
    #[allow(clippy::should_implement_trait)]
    pub fn from_str(format: &str) -> Option<Self> {
        use HttpContent::*;

        // Create a static lookup table for all formats
        match format {
            // Primary formats
            "application/json" => Some(ApplicationJson),
            "text/plain" => Some(TextPlain),
            "text/html" => Some(TextHtml),
            "text/css" => Some(TextCss),
            "text/yaml" => Some(TextYaml),
            "application/yaml" => Some(ApplicationYaml),
            "text/xml" => Some(TextXml),
            "text/xsl" => Some(TextXsl),
            "application/xml" => Some(ApplicationXml),
            "application/javascript" => Some(ApplicationJavascript),
            "application/octet-stream" => Some(ApplicationOctetStream),
            "image/svg+xml" => Some(ImageSvgXml),
            "application/x-font-truetype" => Some(ApplicationXFontTruetype),
            "application/x-font-opentype" => Some(ApplicationXFontOpentype),
            "application/font-woff" => Some(ApplicationFontWoff),
            "application/font-woff2" => Some(ApplicationFontWoff2),
            "application/vnd.ms-fontobject" => Some(ApplicationVndMsFontobject),
            "image/png" => Some(ImagePng),
            "image/jpeg" => Some(ImageJpeg),
            "image/gif" => Some(ImageGif),
            "image/x-icon" => Some(ImageXIcon),
            "image/bmp" => Some(ImageBmp),
            "image/icns" => Some(ImageIcns),
            "audio/mpeg" => Some(AudioMpeg),
            "audio/ogg" => Some(AudioOgg),
            "video/mp4" => Some(VideoMp4),
            "application/pdf" => Some(ApplicationPdf),
            "application/zip" => Some(ApplicationZip),

            // Secondary formats (aliases)
            "prometheus" => Some(Prometheus),
            "text" | "txt" => Some(TextPlain),
            "json" => Some(ApplicationJson),
            "html" => Some(TextHtml),
            "xml" => Some(ApplicationXml),

            _ => None,
        }
    }

    /// Parse with a default fallback (matching the C function behavior)
    pub fn from_str_or_default(format: &str) -> Self {
        Self::from_str(format).unwrap_or(HttpContent::TextPlain)
    }

    /// Check if this content type needs a charset parameter
    pub fn needs_charset(&self) -> bool {
        self.info().needs_charset
    }

    /// Get the full content type header value (with charset if needed)
    pub fn to_header_value(self, charset: Option<&str>) -> String {
        let info = self.info();
        let mut result = String::from(info.format);

        if let Some(options) = info.options {
            result.push_str("; ");
            result.push_str(options);
        }

        if info.needs_charset {
            if let Some(charset) = charset {
                result.push_str("; charset=");
                result.push_str(charset);
            }
        }

        result
    }
}

impl fmt::Display for HttpContent {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.as_str())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_from_str() {
        assert_eq!(
            HttpContent::from_str("application/json"),
            Some(HttpContent::ApplicationJson)
        );
        assert_eq!(
            HttpContent::from_str("json"),
            Some(HttpContent::ApplicationJson)
        );
        assert_eq!(
            HttpContent::from_str("text/plain"),
            Some(HttpContent::TextPlain)
        );
        assert_eq!(HttpContent::from_str("unknown"), None);
    }

    #[test]
    fn test_from_str_with_default() {
        assert_eq!(
            HttpContent::from_str_or_default("application/json"),
            HttpContent::ApplicationJson
        );
        assert_eq!(
            HttpContent::from_str_or_default("unknown"),
            HttpContent::TextPlain
        );
    }

    #[test]
    fn test_to_header_value() {
        assert_eq!(
            HttpContent::ApplicationJson.to_header_value(Some("utf-8")),
            "application/json; charset=utf-8"
        );
        assert_eq!(
            HttpContent::ImagePng.to_header_value(Some("utf-8")),
            "image/png"
        );
        assert_eq!(
            HttpContent::Prometheus.to_header_value(Some("utf-8")),
            "text/plain; version=0.0.4; charset=utf-8"
        );
    }
}
