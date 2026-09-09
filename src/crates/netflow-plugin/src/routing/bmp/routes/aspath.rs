use super::*;

pub(crate) fn flatten_as_path(path: &AsPath) -> Vec<u32> {
    match path {
        AsPath::As2PathSegments(segments) => {
            let mut flattened = Vec::new();
            for segment in segments {
                match segment.segment_type() {
                    AsPathSegmentType::AsSet => {
                        if let Some(first) = segment.as_numbers().first() {
                            flattened.push(u32::from(*first));
                        }
                    }
                    AsPathSegmentType::AsSequence => {
                        flattened.extend(segment.as_numbers().iter().copied().map(u32::from))
                    }
                }
            }
            flattened
        }
        AsPath::As4PathSegments(segments) => {
            let mut flattened = Vec::new();
            for segment in segments {
                match segment.segment_type() {
                    AsPathSegmentType::AsSet => {
                        if let Some(first) = segment.as_numbers().first() {
                            flattened.push(*first);
                        }
                    }
                    AsPathSegmentType::AsSequence => {
                        flattened.extend(segment.as_numbers().iter().copied())
                    }
                }
            }
            flattened
        }
    }
}

pub(crate) fn flatten_as4_path(path: &As4Path) -> Vec<u32> {
    let mut flattened = Vec::new();
    for segment in path.segments() {
        match segment.segment_type() {
            AsPathSegmentType::AsSet => {
                if let Some(first) = segment.as_numbers().first() {
                    flattened.push(*first);
                }
            }
            AsPathSegmentType::AsSequence => flattened.extend(segment.as_numbers().iter().copied()),
        }
    }
    flattened
}
