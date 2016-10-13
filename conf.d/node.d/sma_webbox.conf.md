
Example netdata configuration for node.d/sma_webbox.conf
The module supports any number of name servers, like this:

WHEN PLACED AS node.d/sma_webbox.conf THIS FILE SHOULD BE VALID JSON.
YOU CAN CHECK JSON VALIDITY AT http://jsonlint.com/

YOU HAVE TO DELETE THE COMMENTS - JSON DOES NOT SUPPORT COMMENTS.

{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
        {
            "name": "plant1",
            "hostname": "10.0.1.1",
            "update_every": 10
        },
        {
            "name": "plant2",
            "hostname": "10.0.2.1",
            "update_every": 15
        }
    ]
}
