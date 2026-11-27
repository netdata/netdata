# json

`json` contains the JSON helpers of libnetdata, built on the [JSON-C](https://github.com/json-c/json-c) library:
`json_parse()` calls a callback for the string, integer and boolean members of the root object, and for each object
found in its arrays (then for that object's string and boolean members); `json_strict_validate()` checks a document
against RFC 8259.
