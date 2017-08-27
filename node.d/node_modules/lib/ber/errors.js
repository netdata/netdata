
module.exports = {
	InvalidAsn1Error: function(msg) {
		var e = new Error()
		e.name = 'InvalidAsn1Error'
		e.message = msg || ''
		return e
	}
}
