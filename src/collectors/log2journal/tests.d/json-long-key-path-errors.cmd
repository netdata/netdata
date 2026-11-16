${TESTED_LOG2JOURNAL_BIN} -f tests.d/json-long-key-path.yaml < tests.d/json-long-key-path.input 2>&1 >/dev/null | grep 'JSON PARSER'
