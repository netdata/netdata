
var errors = require('./errors')
var types = require('./types')

var Reader = require('./reader')
var Writer = require('./writer')

for (var t in types)
	if (types.hasOwnProperty(t))
		exports[t] = types[t]

for (var e in errors)
	if (errors.hasOwnProperty(e))
		exports[e] = errors[e]

exports.Reader = Reader
exports.Writer = Writer
