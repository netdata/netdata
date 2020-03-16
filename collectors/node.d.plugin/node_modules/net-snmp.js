
// Copyright 2013 Stephen Vickers <stephen.vickers.sv@gmail.com>

var ber = require ("asn1-ber").Ber;
var dgram = require ("dgram");
var events = require ("events");
var util = require ("util");
var crypto = require ("crypto");
var mibparser = require ("./lib/mib");
var DEBUG = false;

var MAX_INT32 = 2147483647;

function debug (line) {
	if ( DEBUG ) {
		console.debug (line);
	}
}

/*****************************************************************************
 ** Constants
 **/


function _expandConstantObject (object) {
	var keys = [];
	for (var key in object)
		keys.push (key);
	for (var i = 0; i < keys.length; i++)
		object[object[keys[i]]] = parseInt (keys[i]);
}

var ErrorStatus = {
	0: "NoError",
	1: "TooBig",
	2: "NoSuchName",
	3: "BadValue",
	4: "ReadOnly",
	5: "GeneralError",
	6: "NoAccess",
	7: "WrongType",
	8: "WrongLength",
	9: "WrongEncoding",
	10: "WrongValue",
	11: "NoCreation",
	12: "InconsistentValue",
	13: "ResourceUnavailable",
	14: "CommitFailed",
	15: "UndoFailed",
	16: "AuthorizationError",
	17: "NotWritable",
	18: "InconsistentName"
};

_expandConstantObject (ErrorStatus);

var ObjectType = {
	1: "Boolean",
	2: "Integer",
	4: "OctetString",
	5: "Null",
	6: "OID",
	64: "IpAddress",
	65: "Counter",
	66: "Gauge",
	67: "TimeTicks",
	68: "Opaque",
	70: "Counter64",
	128: "NoSuchObject",
	129: "NoSuchInstance",
	130: "EndOfMibView"
};

_expandConstantObject (ObjectType);

// ASN.1
ObjectType.INTEGER = ObjectType.Integer;
ObjectType["OCTET STRING"] = ObjectType.OctetString;
ObjectType["OBJECT IDENTIFIER"] = ObjectType.OID;
// SNMPv2-SMI
ObjectType.Integer32 = ObjectType.Integer;
ObjectType.Counter32 = ObjectType.Counter;
ObjectType.Gauge32 = ObjectType.Gauge;
ObjectType.Unsigned32 = ObjectType.Gauge32;
// SNMPv2-TC
ObjectType.AutonomousType = ObjectType["OBJECT IDENTIFIER"];
ObjectType.DateAndTime = ObjectType["OCTET STRING"];
ObjectType.DisplayString = ObjectType["OCTET STRING"];
ObjectType.InstancePointer = ObjectType["OBJECT IDENTIFIER"];
ObjectType.MacAddress = ObjectType["OCTET STRING"];
ObjectType.PhysAddress = ObjectType["OCTET STRING"];
ObjectType.RowPointer = ObjectType["OBJECT IDENTIFIER"];
ObjectType.RowStatus = ObjectType.INTEGER;
ObjectType.StorageType = ObjectType.INTEGER;
ObjectType.TestAndIncr = ObjectType.INTEGER;
ObjectType.TimeStamp = ObjectType.TimeTicks;
ObjectType.TruthValue = ObjectType.INTEGER;
ObjectType.TAddress = ObjectType["OCTET STRING"];
ObjectType.TDomain = ObjectType["OBJECT IDENTIFIER"];
ObjectType.VariablePointer = ObjectType["OBJECT IDENTIFIER"];

var PduType = {
	160: "GetRequest",
	161: "GetNextRequest",
	162: "GetResponse",
	163: "SetRequest",
	164: "Trap",
	165: "GetBulkRequest",
	166: "InformRequest",
	167: "TrapV2",
	168: "Report"
};

_expandConstantObject (PduType);

var TrapType = {
	0: "ColdStart",
	1: "WarmStart",
	2: "LinkDown",
	3: "LinkUp",
	4: "AuthenticationFailure",
	5: "EgpNeighborLoss",
	6: "EnterpriseSpecific"
};

_expandConstantObject (TrapType);

var SecurityLevel = {
	1: "noAuthNoPriv",
	2: "authNoPriv",
	3: "authPriv"
};

_expandConstantObject (SecurityLevel);

var AuthProtocols = {
	"1": "none",
	"2": "md5",
	"3": "sha"
};

_expandConstantObject (AuthProtocols);

var PrivProtocols = {
	"1": "none",
	"2": "des",
	"4": "aes"
};

_expandConstantObject (PrivProtocols);

var MibProviderType = {
	"1": "Scalar",
	"2": "Table"
};

_expandConstantObject (MibProviderType);

var Version1 = 0;
var Version2c = 1;
var Version3 = 3;

var Version = {
	"1": Version1,
	"2c": Version2c,
	"3": Version3
};

/*****************************************************************************
 ** Exception class definitions
 **/

function ResponseInvalidError (message) {
	this.name = "ResponseInvalidError";
	this.message = message;
	Error.captureStackTrace(this, ResponseInvalidError);
}
util.inherits (ResponseInvalidError, Error);

function RequestInvalidError (message) {
	this.name = "RequestInvalidError";
	this.message = message;
	Error.captureStackTrace(this, RequestInvalidError);
}
util.inherits (RequestInvalidError, Error);

function RequestFailedError (message, status) {
	this.name = "RequestFailedError";
	this.message = message;
	this.status = status;
	Error.captureStackTrace(this, RequestFailedError);
}
util.inherits (RequestFailedError, Error);

function RequestTimedOutError (message) {
	this.name = "RequestTimedOutError";
	this.message = message;
	Error.captureStackTrace(this, RequestTimedOutError);
}
util.inherits (RequestTimedOutError, Error);

/*****************************************************************************
 ** OID and varbind helper functions
 **/

function isVarbindError (varbind) {
	return !!(varbind.type == ObjectType.NoSuchObject
	|| varbind.type == ObjectType.NoSuchInstance
	|| varbind.type == ObjectType.EndOfMibView);
}

function varbindError (varbind) {
	return (ObjectType[varbind.type] || "NotAnError") + ": " + varbind.oid;
}

function oidFollowsOid (oidString, nextString) {
	var oid = {str: oidString, len: oidString.length, idx: 0};
	var next = {str: nextString, len: nextString.length, idx: 0};
	var dotCharCode = ".".charCodeAt (0);

	function getNumber (item) {
		var n = 0;
		if (item.idx >= item.len)
			return null;
		while (item.idx < item.len) {
			var charCode = item.str.charCodeAt (item.idx++);
			if (charCode == dotCharCode)
				return n;
			n = (n ? (n * 10) : n) + (charCode - 48);
		}
		return n;
	}

	while (1) {
		var oidNumber = getNumber (oid);
		var nextNumber = getNumber (next);

		if (oidNumber !== null) {
			if (nextNumber !== null) {
				if (nextNumber > oidNumber) {
					return true;
				} else if (nextNumber < oidNumber) {
					return false;
				}
			} else {
				return true;
			}
		} else {
			return true;
		}
	}
}

function oidInSubtree (oidString, nextString) {
	var oid = oidString.split (".");
	var next = nextString.split (".");

	if (oid.length > next.length)
		return false;

	for (var i = 0; i < oid.length; i++) {
		if (next[i] != oid[i])
			return false;
	}

	return true;
}

/**
 ** Some SNMP agents produce integers on the wire such as 00 ff ff ff ff.
 ** The ASN.1 BER parser we use throws an error when parsing this, which we
 ** believe is correct.  So, we decided not to bother the "asn1" developer(s)
 ** with this, instead opting to work around it here.
 **
 ** If an integer is 5 bytes in length we check if the first byte is 0, and if so
 ** simply drop it and parse it like it was a 4 byte integer, otherwise throw
 ** an error since the integer is too large.
 **/

function readInt (buffer) {
	return readUint (buffer, true);
}

function readIpAddress (buffer) {
	var bytes = buffer.readString (ObjectType.IpAddress, true);
	if (bytes.length != 4)
		throw new ResponseInvalidError ("Length '" + bytes.length
				+ "' of IP address '" + bytes.toString ("hex")
				+ "' is not 4");
	var value = bytes[0] + "." + bytes[1] + "." + bytes[2] + "." + bytes[3];
	return value;
}

function readUint (buffer, isSigned) {
	buffer.readByte ();
	var length = buffer.readByte ();
	var value = 0;
	var signedBitSet = false;

	if (length > 5) {
		 throw new RangeError ("Integer too long '" + length + "'");
	} else if (length == 5) {
		if (buffer.readByte () !== 0)
			throw new RangeError ("Integer too long '" + length + "'");
		length = 4;
	}

	for (var i = 0; i < length; i++) {
		value *= 256;
		value += buffer.readByte ();

		if (isSigned && i <= 0) {
			if ((value & 0x80) == 0x80)
				signedBitSet = true;
		}
	}
	
	if (signedBitSet)
		value -= (1 << (i * 8));

	return value;
}

function readUint64 (buffer) {
	var value = buffer.readString (ObjectType.Counter64, true);

	return value;
}

function readVarbinds (buffer, varbinds) {
	buffer.readSequence ();

	while (1) {
		buffer.readSequence ();
		if ( buffer.peek () != ObjectType.OID )
			break;
		var oid = buffer.readOID ();
		var type = buffer.peek ();

		if (type == null)
			break;

		var value;

		if (type == ObjectType.Boolean) {
			value = buffer.readBoolean ();
		} else if (type == ObjectType.Integer) {
			value = readInt (buffer);
		} else if (type == ObjectType.OctetString) {
			value = buffer.readString (null, true);
		} else if (type == ObjectType.Null) {
			buffer.readByte ();
			buffer.readByte ();
			value = null;
		} else if (type == ObjectType.OID) {
			value = buffer.readOID ();
		} else if (type == ObjectType.IpAddress) {
			var bytes = buffer.readString (ObjectType.IpAddress, true);
			if (bytes.length != 4)
				throw new ResponseInvalidError ("Length '" + bytes.length
						+ "' of IP address '" + bytes.toString ("hex")
						+ "' is not 4");
			value = bytes[0] + "." + bytes[1] + "." + bytes[2] + "." + bytes[3];
		} else if (type == ObjectType.Counter) {
			value = readUint (buffer);
		} else if (type == ObjectType.Gauge) {
			value = readUint (buffer);
		} else if (type == ObjectType.TimeTicks) {
			value = readUint (buffer);
		} else if (type == ObjectType.Opaque) {
			value = buffer.readString (ObjectType.Opaque, true);
		} else if (type == ObjectType.Counter64) {
			value = readUint64 (buffer);
		} else if (type == ObjectType.NoSuchObject) {
			buffer.readByte ();
			buffer.readByte ();
			value = null;
		} else if (type == ObjectType.NoSuchInstance) {
			buffer.readByte ();
			buffer.readByte ();
			value = null;
		} else if (type == ObjectType.EndOfMibView) {
			buffer.readByte ();
			buffer.readByte ();
			value = null;
		} else {
			throw new ResponseInvalidError ("Unknown type '" + type
					+ "' in response");
		}

		varbinds.push ({
			oid: oid,
			type: type,
			value: value
		});
	}
}

function writeUint (buffer, type, value) {
	var b = Buffer.alloc (4);
	b.writeUInt32BE (value, 0);
	buffer.writeBuffer (b, type);
}

function writeUint64 (buffer, value) {
	buffer.writeBuffer (value, ObjectType.Counter64);
}

function writeVarbinds (buffer, varbinds) {
	buffer.startSequence ();
	for (var i = 0; i < varbinds.length; i++) {
		buffer.startSequence ();
		buffer.writeOID (varbinds[i].oid);

		if (varbinds[i].type && varbinds[i].hasOwnProperty("value")) {
			var type = varbinds[i].type;
			var value = varbinds[i].value;

			if (type == ObjectType.Boolean) {
				buffer.writeBoolean (value ? true : false);
			} else if (type == ObjectType.Integer) { // also Integer32
				buffer.writeInt (value);
			} else if (type == ObjectType.OctetString) {
				if (typeof value == "string")
					buffer.writeString (value);
				else
					buffer.writeBuffer (value, ObjectType.OctetString);
			} else if (type == ObjectType.Null) {
				buffer.writeNull ();
			} else if (type == ObjectType.OID) {
				buffer.writeOID (value);
			} else if (type == ObjectType.IpAddress) {
				var bytes = value.split (".");
				if (bytes.length != 4)
					throw new RequestInvalidError ("Invalid IP address '"
							+ value + "'");
				buffer.writeBuffer (Buffer.from (bytes), 64);
			} else if (type == ObjectType.Counter) { // also Counter32
				writeUint (buffer, ObjectType.Counter, value);
			} else if (type == ObjectType.Gauge) { // also Gauge32 & Unsigned32
				writeUint (buffer, ObjectType.Gauge, value);
			} else if (type == ObjectType.TimeTicks) {
				writeUint (buffer, ObjectType.TimeTicks, value);
			} else if (type == ObjectType.Opaque) {
				buffer.writeBuffer (value, ObjectType.Opaque);
			} else if (type == ObjectType.Counter64) {
				writeUint64 (buffer, value);
			} else if (type == ObjectType.EndOfMibView ) {
				buffer.writeByte (130);
				buffer.writeByte (0);
			} else {
				throw new RequestInvalidError ("Unknown type '" + type
						+ "' in request");
			}
		} else {
			buffer.writeNull ();
		}

		buffer.endSequence ();
	}
	buffer.endSequence ();
}

/*****************************************************************************
 ** PDU class definitions
 **/

var SimplePdu = function () {
};

SimplePdu.prototype.toBuffer = function (buffer) {
	buffer.startSequence (this.type);

	buffer.writeInt (this.id);
	buffer.writeInt ((this.type == PduType.GetBulkRequest)
			? (this.options.nonRepeaters || 0)
			: 0);
	buffer.writeInt ((this.type == PduType.GetBulkRequest)
			? (this.options.maxRepetitions || 0)
			: 0);

	writeVarbinds (buffer, this.varbinds);

	buffer.endSequence ();
};

SimplePdu.prototype.initializeFromVariables = function (id, varbinds, options) {
	this.id = id;
	this.varbinds = varbinds;
	this.options = options || {};
	this.contextName = (options && options.context) ? options.context : "";
}

SimplePdu.prototype.initializeFromBuffer = function (reader) {
	this.type = reader.peek ();
	reader.readSequence ();

	this.id = reader.readInt ();
	this.nonRepeaters = reader.readInt ();
	this.maxRepetitions = reader.readInt ();

	this.varbinds = [];
	readVarbinds (reader, this.varbinds);

};

SimplePdu.prototype.getResponsePduForRequest = function () {
	var responsePdu = GetResponsePdu.createFromVariables(this.id, [], {});
	if ( this.contextEngineID ) {
		responsePdu.contextEngineID = this.contextEngineID;
		responsePdu.contextName = this.contextName;
	}
	return responsePdu;
};

SimplePdu.createFromVariables = function (pduClass, id, varbinds, options) {
	var pdu = new pduClass (id, varbinds, options);
	pdu.id = id;
	pdu.varbinds = varbinds;
	pdu.options = options || {};
	pdu.contextName = (options && options.context) ? options.context : "";
	return pdu;
};

var GetBulkRequestPdu = function () {
	this.type = PduType.GetBulkRequest;
	GetBulkRequestPdu.super_.apply (this, arguments);
};

util.inherits (GetBulkRequestPdu, SimplePdu);

GetBulkRequestPdu.createFromBuffer = function (reader) {
	var pdu = new GetBulkRequestPdu ();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

var GetNextRequestPdu = function () {
	this.type = PduType.GetNextRequest;
	GetNextRequestPdu.super_.apply (this, arguments);
};

util.inherits (GetNextRequestPdu, SimplePdu);

GetNextRequestPdu.createFromBuffer = function (reader) {
	var pdu = new GetNextRequestPdu ();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

var GetRequestPdu = function () {
	this.type = PduType.GetRequest;
	GetRequestPdu.super_.apply (this, arguments);
};

util.inherits (GetRequestPdu, SimplePdu);

GetRequestPdu.createFromBuffer = function (reader) {
	var pdu = new GetRequestPdu();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

GetRequestPdu.createFromVariables = function (id, varbinds, options) {
	var pdu = new GetRequestPdu();
	pdu.initializeFromVariables (id, varbinds, options);
	return pdu;
};

var InformRequestPdu = function () {
	this.type = PduType.InformRequest;
	InformRequestPdu.super_.apply (this, arguments);
};

util.inherits (InformRequestPdu, SimplePdu);

InformRequestPdu.createFromBuffer = function (reader) {
	var pdu = new InformRequestPdu();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

var SetRequestPdu = function () {
	this.type = PduType.SetRequest;
	SetRequestPdu.super_.apply (this, arguments);
};

util.inherits (SetRequestPdu, SimplePdu);

SetRequestPdu.createFromBuffer = function (reader) {
	var pdu = new SetRequestPdu ();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

var TrapPdu = function () {
	this.type = PduType.Trap;
};

TrapPdu.prototype.toBuffer = function (buffer) {
	buffer.startSequence (this.type);

	buffer.writeOID (this.enterprise);
	buffer.writeBuffer (Buffer.from (this.agentAddr.split (".")),
			ObjectType.IpAddress);
	buffer.writeInt (this.generic);
	buffer.writeInt (this.specific);
	writeUint (buffer, ObjectType.TimeTicks,
			this.upTime || Math.floor (process.uptime () * 100));

	writeVarbinds (buffer, this.varbinds);

	buffer.endSequence ();
};

TrapPdu.createFromBuffer = function (reader) {
	var pdu = new TrapPdu();
	reader.readSequence ();

	pdu.enterprise = reader.readOID ();
	pdu.agentAddr = readIpAddress (reader);
	pdu.generic = reader.readInt ();
	pdu.specific = reader.readInt ();
	pdu.upTime = readUint (reader)

	pdu.varbinds = [];
	readVarbinds (reader, pdu.varbinds);

	return pdu;
};

TrapPdu.createFromVariables = function (typeOrOid, varbinds, options) {
	var pdu = new TrapPdu ();
	pdu.agentAddr = options.agentAddr || "127.0.0.1";
	pdu.upTime = options.upTime;

	if (typeof typeOrOid == "string") {
		pdu.generic = TrapType.EnterpriseSpecific;
		pdu.specific = parseInt (typeOrOid.match (/\.(\d+)$/)[1]);
		pdu.enterprise = typeOrOid.replace (/\.(\d+)$/, "");
	} else {
		pdu.generic = typeOrOid;
		pdu.specific = 0;
		pdu.enterprise = "1.3.6.1.4.1";
	}

	pdu.varbinds = varbinds;

	return pdu;
};

var TrapV2Pdu = function () {
	this.type = PduType.TrapV2;
	TrapV2Pdu.super_.apply (this, arguments);
};

util.inherits (TrapV2Pdu, SimplePdu);

TrapV2Pdu.createFromBuffer = function (reader) {
	var pdu = new TrapV2Pdu();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

TrapV2Pdu.createFromVariables = function (id, varbinds, options) {
	var pdu = new TrapV2Pdu();
	pdu.initializeFromVariables (id, varbinds, options);
	return pdu;
};

var SimpleResponsePdu = function() {
};

SimpleResponsePdu.prototype.toBuffer = function (writer) {
	writer.startSequence (this.type);

	writer.writeInt (this.id);
	writer.writeInt (this.errorStatus || 0);
	writer.writeInt (this.errorIndex || 0);
	writeVarbinds (writer, this.varbinds);
	writer.endSequence ();

};

SimpleResponsePdu.prototype.initializeFromBuffer = function (reader) {
	reader.readSequence (this.type);

	this.id = reader.readInt ();
	this.errorStatus = reader.readInt ();
	this.errorIndex = reader.readInt ();

	this.varbinds = [];
	readVarbinds (reader, this.varbinds);
};

SimpleResponsePdu.prototype.initializeFromVariables = function (id, varbinds, options) {
	this.id = id;
	this.varbinds = varbinds;
	this.options = options || {};
};

var GetResponsePdu = function () {
	this.type = PduType.GetResponse;
	GetResponsePdu.super_.apply (this, arguments);
};

util.inherits (GetResponsePdu, SimpleResponsePdu);

GetResponsePdu.createFromBuffer = function (reader) {
	var pdu = new GetResponsePdu ();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

GetResponsePdu.createFromVariables = function (id, varbinds, options) {
	var pdu = new GetResponsePdu();
	pdu.initializeFromVariables (id, varbinds, options);
	return pdu;
};

var ReportPdu = function () {
	this.type = PduType.Report;
	ReportPdu.super_.apply (this, arguments);
};

util.inherits (ReportPdu, SimpleResponsePdu);

ReportPdu.createFromBuffer = function (reader) {
	var pdu = new ReportPdu ();
	pdu.initializeFromBuffer (reader);
	return pdu;
};

ReportPdu.createFromVariables = function (id, varbinds, options) {
	var pdu = new ReportPdu();
	pdu.initializeFromVariables (id, varbinds, options);
	return pdu;
};

var readPdu = function (reader, scoped) {
	var pdu;
	var contextEngineID;
	var contextName;
	if ( scoped ) {
		reader.readSequence ();
		contextEngineID = reader.readString (ber.OctetString, true);
		contextName = reader.readString ();
	}
	var type = reader.peek ();

	if (type == PduType.GetResponse) {
		pdu = GetResponsePdu.createFromBuffer (reader);
	} else if (type == PduType.Report ) {
		pdu = ReportPdu.createFromBuffer (reader);
	} else if (type == PduType.Trap ) {
		pdu = TrapPdu.createFromBuffer (reader);
	} else if (type == PduType.TrapV2 ) {
		pdu = TrapV2Pdu.createFromBuffer (reader);
	} else if (type == PduType.InformRequest ) {
		pdu = InformRequestPdu.createFromBuffer (reader);
	} else if (type == PduType.GetRequest ) {
		pdu = GetRequestPdu.createFromBuffer (reader);
	} else if (type == PduType.SetRequest ) {
		pdu = SetRequestPdu.createFromBuffer (reader);
	} else if (type == PduType.GetNextRequest ) {
		pdu = GetNextRequestPdu.createFromBuffer (reader);
	} else if (type == PduType.GetBulkRequest ) {
		pdu = GetBulkRequestPdu.createFromBuffer (reader);
	} else {
		throw new ResponseInvalidError ("Unknown PDU type '" + type
				+ "' in response");
	}
	if ( scoped ) {
		pdu.contextEngineID = contextEngineID;
		pdu.contextName = contextName;
	}
	pdu.scoped = scoped;
	return pdu;
};

var createDiscoveryPdu = function (context) {
	return GetRequestPdu.createFromVariables(_generateId(), [], {context: context});
};

var Authentication = {};

Authentication.HMAC_BUFFER_SIZE = 1024*1024;
Authentication.HMAC_BLOCK_SIZE = 64;
Authentication.AUTHENTICATION_CODE_LENGTH = 12;
Authentication.AUTH_PARAMETERS_PLACEHOLDER = Buffer.from('8182838485868788898a8b8c', 'hex');

Authentication.algorithms = {};

Authentication.algorithms[AuthProtocols.md5] = {
	// KEY_LENGTH: 16,
	CRYPTO_ALGORITHM: 'md5'
};

Authentication.algorithms[AuthProtocols.sha] = {
	// KEY_LENGTH: 20,
	CRYPTO_ALGORITHM: 'sha1'
};

// Adapted from RFC3414 Appendix A.2.1. Password to Key Sample Code for MD5
Authentication.passwordToKey = function (authProtocol, authPasswordString, engineID) {
	var hashAlgorithm;
	var firstDigest;
	var finalDigest;
	var buf = Buffer.alloc (Authentication.HMAC_BUFFER_SIZE);
	var bufOffset = 0;
	var passwordIndex = 0;
	var count = 0;
	var password = Buffer.from (authPasswordString);
	var cryptoAlgorithm = Authentication.algorithms[authProtocol].CRYPTO_ALGORITHM;
	
	while (count < Authentication.HMAC_BUFFER_SIZE) {
		for (var i = 0; i < Authentication.HMAC_BLOCK_SIZE; i++) {
			buf.writeUInt8(password[passwordIndex++ % password.length], bufOffset++);
		}
		count += Authentication.HMAC_BLOCK_SIZE;
	}
	hashAlgorithm = crypto.createHash(cryptoAlgorithm);
	hashAlgorithm.update(buf);
	firstDigest = hashAlgorithm.digest();
	// debug ("First digest:  " + firstDigest.toString('hex'));

	hashAlgorithm = crypto.createHash(cryptoAlgorithm);
	hashAlgorithm.update(firstDigest);
	hashAlgorithm.update(engineID);
	hashAlgorithm.update(firstDigest);
	finalDigest = hashAlgorithm.digest();
	debug ("Localized key: " + finalDigest.toString('hex'));

	return finalDigest;
};

Authentication.addParametersToMessageBuffer = function (messageBuffer, authProtocol, authPassword, engineID) {
	var authenticationParametersOffset;
	var digestToAdd;

	// clear the authenticationParameters field in message
	authenticationParametersOffset = messageBuffer.indexOf (Authentication.AUTH_PARAMETERS_PLACEHOLDER);
	messageBuffer.fill (0, authenticationParametersOffset, authenticationParametersOffset + Authentication.AUTHENTICATION_CODE_LENGTH);

	digestToAdd = Authentication.calculateDigest (messageBuffer, authProtocol, authPassword, engineID);
	digestToAdd.copy (messageBuffer, authenticationParametersOffset, 0, Authentication.AUTHENTICATION_CODE_LENGTH);
	debug ("Added Auth Parameters: " + digestToAdd.toString('hex'));
};

Authentication.isAuthentic = function (messageBuffer, authProtocol, authPassword, engineID, digestInMessage) {
	var authenticationParametersOffset;
	var calculatedDigest;

	// clear the authenticationParameters field in message
	authenticationParametersOffset = messageBuffer.indexOf (digestInMessage);
	messageBuffer.fill (0, authenticationParametersOffset, authenticationParametersOffset + Authentication.AUTHENTICATION_CODE_LENGTH);

	calculatedDigest = Authentication.calculateDigest (messageBuffer, authProtocol, authPassword, engineID);

	// replace previously cleared authenticationParameters field in message
	digestInMessage.copy (messageBuffer, authenticationParametersOffset, 0, Authentication.AUTHENTICATION_CODE_LENGTH);

	debug ("Digest in message: " + digestInMessage.toString('hex'));
	debug ("Calculated digest: " + calculatedDigest.toString('hex'));
	return calculatedDigest.equals (digestInMessage, Authentication.AUTHENTICATION_CODE_LENGTH);
};

Authentication.calculateDigest = function (messageBuffer, authProtocol, authPassword, engineID) {
	var authKey = Authentication.passwordToKey (authProtocol, authPassword, engineID);

	// Adapted from RFC3147 Section 6.3.1. Processing an Outgoing Message
	var hashAlgorithm;
	var kIpad;
	var kOpad;
	var firstDigest;
	var finalDigest;
	var truncatedDigest;
	var i;
	var cryptoAlgorithm = Authentication.algorithms[authProtocol].CRYPTO_ALGORITHM;

	if (authKey.length > Authentication.HMAC_BLOCK_SIZE) {
		hashAlgorithm = crypto.createHash (cryptoAlgorithm);
		hashAlgorithm.update (authKey);
		authKey = hashAlgorithm.digest ();
	}

	// MD(K XOR opad, MD(K XOR ipad, msg))
	kIpad = Buffer.alloc (Authentication.HMAC_BLOCK_SIZE);
	kOpad = Buffer.alloc (Authentication.HMAC_BLOCK_SIZE);
	for (i = 0; i < authKey.length; i++) {
		kIpad[i] = authKey[i] ^ 0x36;
		kOpad[i] = authKey[i] ^ 0x5c;
	}
	kIpad.fill (0x36, authKey.length);
	kOpad.fill (0x5c, authKey.length);

	// inner MD
	hashAlgorithm = crypto.createHash (cryptoAlgorithm);
	hashAlgorithm.update (kIpad);
	hashAlgorithm.update (messageBuffer);
	firstDigest = hashAlgorithm.digest ();
	// outer MD
	hashAlgorithm = crypto.createHash (cryptoAlgorithm);
	hashAlgorithm.update (kOpad);
	hashAlgorithm.update (firstDigest);
	finalDigest = hashAlgorithm.digest ();

	truncatedDigest = Buffer.alloc (Authentication.AUTHENTICATION_CODE_LENGTH);
	finalDigest.copy (truncatedDigest, 0, 0, Authentication.AUTHENTICATION_CODE_LENGTH);
	return truncatedDigest;
};

var Encryption = {};

Encryption.PRIV_PARAMETERS_PLACEHOLDER = Buffer.from ('9192939495969798', 'hex');

Encryption.encryptPdu = function (privProtocol, scopedPdu, privPassword, authProtocol, engine) {
	var encryptFunction = Encryption.algorithms[privProtocol].encryptPdu;
	return encryptFunction (scopedPdu, privPassword, authProtocol, engine);
};

Encryption.decryptPdu = function (privProtocol, encryptedPdu, privParameters, privPassword, authProtocol, engine, forceAutoPaddingDisable) {
	var decryptFunction = Encryption.algorithms[privProtocol].decryptPdu;
	return decryptFunction (encryptedPdu, privParameters, privPassword, authProtocol, engine, forceAutoPaddingDisable);
};

Encryption.debugEncrypt = function (encryptionKey, iv, plainPdu, encryptedPdu) {
	debug ("Key: " + encryptionKey.toString ('hex'));
	debug ("IV:  " + iv.toString ('hex'));
	debug ("Plain:     " + plainPdu.toString ('hex'));
	debug ("Encrypted: " + encryptedPdu.toString ('hex'));
};

Encryption.debugDecrypt = function (decryptionKey, iv, encryptedPdu, plainPdu) {
	debug ("Key: " + decryptionKey.toString ('hex'));
	debug ("IV:  " + iv.toString ('hex'));
	debug ("Encrypted: " + encryptedPdu.toString ('hex'));
	debug ("Plain:     " + plainPdu.toString ('hex'));
};

Encryption.generateLocalizedKey = function (algorithm, authProtocol, privPassword, engineID) {
	var privLocalizedKey;
	var encryptionKey;

	privLocalizedKey = Authentication.passwordToKey (authProtocol, privPassword, engineID);
	encryptionKey = Buffer.alloc (algorithm.KEY_LENGTH);
	privLocalizedKey.copy (encryptionKey, 0, 0, algorithm.KEY_LENGTH);

	return encryptionKey;
};

Encryption.encryptPduDes = function (scopedPdu, privPassword, authProtocol, engine) {
	var des = Encryption.algorithms[PrivProtocols.des];
	var privLocalizedKey;
	var encryptionKey;
	var preIv;
	var salt;
	var iv;
	var i;
	var paddedScopedPduLength;
	var paddedScopedPdu;
	var encryptedPdu;

	encryptionKey = Encryption.generateLocalizedKey (des, authProtocol, privPassword, engine.engineID);
	privLocalizedKey = Authentication.passwordToKey (authProtocol, privPassword, engine.engineID);
	encryptionKey = Buffer.alloc (des.KEY_LENGTH);
	privLocalizedKey.copy (encryptionKey, 0, 0, des.KEY_LENGTH);
	preIv = Buffer.alloc (des.BLOCK_LENGTH);
	privLocalizedKey.copy (preIv, 0, des.KEY_LENGTH, des.KEY_LENGTH + des.BLOCK_LENGTH);

	salt = Buffer.alloc (des.BLOCK_LENGTH);
	// set local SNMP engine boots part of salt to 1, as we have no persistent engine state
	salt.fill ('00000001', 0, 4, 'hex');
	// set local integer part of salt to random
	salt.fill (crypto.randomBytes (4), 4, 8);
	iv = Buffer.alloc (des.BLOCK_LENGTH);
	for (i = 0; i < iv.length; i++) {
		iv[i] = preIv[i] ^ salt[i];
	}
	
	if (scopedPdu.length % des.BLOCK_LENGTH == 0) {
		paddedScopedPdu = scopedPdu;
	} else {
		paddedScopedPduLength = des.BLOCK_LENGTH * (Math.floor (scopedPdu.length / des.BLOCK_LENGTH) + 1);
		paddedScopedPdu = Buffer.alloc (paddedScopedPduLength);
		scopedPdu.copy (paddedScopedPdu, 0, 0, scopedPdu.length);
	}
	cipher = crypto.createCipheriv (des.CRYPTO_ALGORITHM, encryptionKey, iv);
	encryptedPdu = cipher.update (paddedScopedPdu);
	encryptedPdu = Buffer.concat ([encryptedPdu, cipher.final()]);
	// Encryption.debugEncrypt (encryptionKey, iv, paddedScopedPdu, encryptedPdu);

	return {
		encryptedPdu: encryptedPdu,
		msgPrivacyParameters: salt
	};
};

Encryption.decryptPduDes = function (encryptedPdu, privParameters, privPassword, authProtocol, engine, forceAutoPaddingDisable) {
	var des = Encryption.algorithms[PrivProtocols.des];
	var privLocalizedKey;
	var decryptionKey;
	var preIv;
	var salt;
	var iv;
	var i;
	var decryptedPdu;

	privLocalizedKey = Authentication.passwordToKey (authProtocol, privPassword, engine.engineID);
	decryptionKey = Buffer.alloc (des.KEY_LENGTH);
	privLocalizedKey.copy (decryptionKey, 0, 0, des.KEY_LENGTH);
	preIv = Buffer.alloc (des.BLOCK_LENGTH);
	privLocalizedKey.copy (preIv, 0, des.KEY_LENGTH, des.KEY_LENGTH + des.BLOCK_LENGTH);

	salt = privParameters;
	iv = Buffer.alloc (des.BLOCK_LENGTH);
	for (i = 0; i < iv.length; i++) {
		iv[i] = preIv[i] ^ salt[i];
	}
	
	decipher = crypto.createDecipheriv (des.CRYPTO_ALGORITHM, decryptionKey, iv);
	if ( forceAutoPaddingDisable ) {
		decipher.setAutoPadding(false);
	}
	decryptedPdu = decipher.update (encryptedPdu);
	// This try-catch is a workaround for a seemingly incorrect error condition
	// - where sometimes a decrypt error is thrown with decipher.final()
	// It replaces this line which should have been sufficient:
	// decryptedPdu = Buffer.concat ([decryptedPdu, decipher.final()]);
	try {
		decryptedPdu = Buffer.concat ([decryptedPdu, decipher.final()]);
	} catch (error) {
		// debug("Decrypt error: " + error);
		decipher = crypto.createDecipheriv (des.CRYPTO_ALGORITHM, decryptionKey, iv);
		decipher.setAutoPadding(false);
		decryptedPdu = decipher.update (encryptedPdu);
		decryptedPdu = Buffer.concat ([decryptedPdu, decipher.final()]);
	}
	// Encryption.debugDecrypt (decryptionKey, iv, encryptedPdu, decryptedPdu);

	return decryptedPdu;
};

Encryption.generateIvAes = function (aes, engineBoots, engineTime, salt) {
	var iv;

	// iv = engineBoots(4) | engineTime(4) | salt(8)
	iv = Buffer.alloc (aes.BLOCK_LENGTH);
	engineBootsBuffer = Buffer.alloc (4);
	engineBootsBuffer.writeUInt32BE (engineBoots);
	engineTimeBuffer = Buffer.alloc (4);
	engineTimeBuffer.writeUInt32BE (engineTime);
	engineBootsBuffer.copy (iv, 0, 0, 4);
	engineTimeBuffer.copy (iv, 4, 0, 4);
	salt.copy (iv, 8, 0, 8);

	return iv;
}

Encryption.encryptPduAes = function (scopedPdu, privPassword, authProtocol, engine) {
	var aes = Encryption.algorithms[PrivProtocols.aes];
	var encryptionKey;
	var salt;
	var iv;
	var cipher;
	var encryptedPdu;

	encryptionKey = Encryption.generateLocalizedKey (aes, authProtocol, privPassword, engine.engineID);
	salt = Buffer.alloc (8).fill (crypto.randomBytes (8), 0, 8);
	iv = Encryption.generateIvAes (aes, engine.engineBoots, engine.engineTime, salt);
	cipher = crypto.createCipheriv (aes.CRYPTO_ALGORITHM, encryptionKey, iv);
	encryptedPdu = cipher.update (scopedPdu);
	encryptedPdu = Buffer.concat ([encryptedPdu, cipher.final()]);
	// Encryption.debugEncrypt (encryptionKey, iv, scopedPdu, encryptedPdu);

	return {
		encryptedPdu: encryptedPdu,
		msgPrivacyParameters: salt
	};
};

Encryption.decryptPduAes = function (encryptedPdu, privParameters, privPassword, authProtocol, engine) {
	var aes = Encryption.algorithms[PrivProtocols.aes];
	var decryptionKey;
	var iv;
	var decipher;
	var decryptedPdu;

	decryptionKey = Encryption.generateLocalizedKey (aes, authProtocol, privPassword, engine.engineID);
	iv = Encryption.generateIvAes (aes, engine.engineBoots, engine.engineTime, privParameters);
	decipher = crypto.createDecipheriv (aes.CRYPTO_ALGORITHM, decryptionKey, iv);
	decryptedPdu = decipher.update (encryptedPdu);
	decryptedPdu = Buffer.concat ([decryptedPdu, decipher.final()]);
	// Encryption.debugDecrypt (decryptionKey, iv, encryptedPdu, decryptedPdu);

	return decryptedPdu;
};

Encryption.addParametersToMessageBuffer = function (messageBuffer, msgPrivacyParameters) {
	privacyParametersOffset = messageBuffer.indexOf (Encryption.PRIV_PARAMETERS_PLACEHOLDER);
	msgPrivacyParameters.copy (messageBuffer, privacyParametersOffset, 0, Encryption.DES_IV_LENGTH);
};

Encryption.algorithms = {};

Encryption.algorithms[PrivProtocols.des] = {
	CRYPTO_ALGORITHM: 'des-cbc',
	KEY_LENGTH: 8,
	BLOCK_LENGTH: 8,
	encryptPdu: Encryption.encryptPduDes,
	decryptPdu: Encryption.decryptPduDes
};

Encryption.algorithms[PrivProtocols.aes] = {
	CRYPTO_ALGORITHM: 'aes-128-cfb',
	KEY_LENGTH: 16,
	BLOCK_LENGTH: 16,
	encryptPdu: Encryption.encryptPduAes,
	decryptPdu: Encryption.decryptPduAes
};

/*****************************************************************************
 ** Message class definition
 **/

var Message = function () {
}

Message.prototype.getReqId = function () {
	return this.version == Version3 ? this.msgGlobalData.msgID : this.pdu.id;
};

Message.prototype.toBuffer = function () {
	if ( this.version == Version3 ) {
		return this.toBufferV3();
	} else {
		return this.toBufferCommunity();
	}
}

Message.prototype.toBufferCommunity = function () {
	if (this.buffer)
		return this.buffer;

	var writer = new ber.Writer ();

	writer.startSequence ();

	writer.writeInt (this.version);
	writer.writeString (this.community);

	this.pdu.toBuffer (writer);

	writer.endSequence ();

	this.buffer = writer.buffer;

	return this.buffer;
};

Message.prototype.toBufferV3 = function () {
	var encryptionResult;

	if (this.buffer)
		return this.buffer;

	var writer = new ber.Writer ();

	writer.startSequence ();

	writer.writeInt (this.version);

	// HeaderData
	writer.startSequence ();
	writer.writeInt (this.msgGlobalData.msgID);
	writer.writeInt (this.msgGlobalData.msgMaxSize);
	writer.writeByte (ber.OctetString);
	writer.writeByte (1);
	writer.writeByte (this.msgGlobalData.msgFlags);
	writer.writeInt (this.msgGlobalData.msgSecurityModel);
	writer.endSequence ();

	// msgSecurityParameters
	var msgSecurityParametersWriter = new ber.Writer ();
	msgSecurityParametersWriter.startSequence ();
	//msgSecurityParametersWriter.writeString (this.msgSecurityParameters.msgAuthoritativeEngineID);
	// writing a zero-length buffer fails - should fix asn1-ber for this condition
	if ( this.msgSecurityParameters.msgAuthoritativeEngineID.length == 0 ) {
		msgSecurityParametersWriter.writeString ("");
	} else {
		msgSecurityParametersWriter.writeBuffer (this.msgSecurityParameters.msgAuthoritativeEngineID, ber.OctetString);
	}
	msgSecurityParametersWriter.writeInt (this.msgSecurityParameters.msgAuthoritativeEngineBoots);
	msgSecurityParametersWriter.writeInt (this.msgSecurityParameters.msgAuthoritativeEngineTime);
	msgSecurityParametersWriter.writeString (this.msgSecurityParameters.msgUserName);

	if ( this.hasAuthentication() ) {
		msgSecurityParametersWriter.writeBuffer (Authentication.AUTH_PARAMETERS_PLACEHOLDER, ber.OctetString);
	// should never happen where msgFlags has no authentication but authentication parameters still present
	} else if ( this.msgSecurityParameters.msgAuthenticationParameters.length > 0 ) {
		msgSecurityParametersWriter.writeBuffer (this.msgSecurityParameters.msgAuthenticationParameters, ber.OctetString);
	} else {
		msgSecurityParametersWriter.writeString ("");
	}

	if ( this.hasPrivacy() ) {
		msgSecurityParametersWriter.writeBuffer (Encryption.PRIV_PARAMETERS_PLACEHOLDER, ber.OctetString);
	// should never happen where msgFlags has no privacy but privacy parameters still present
	} else if ( this.msgSecurityParameters.msgPrivacyParameters.length > 0 ) {
		msgSecurityParametersWriter.writeBuffer (this.msgSecurityParameters.msgPrivacyParameters, ber.OctetString);
	} else {
		 msgSecurityParametersWriter.writeString ("");
	}
	msgSecurityParametersWriter.endSequence ();

	writer.writeBuffer (msgSecurityParametersWriter.buffer, ber.OctetString);

	// ScopedPDU
	var scopedPduWriter = new ber.Writer ();
	scopedPduWriter.startSequence ();
	var contextEngineID = this.pdu.contextEngineID ? this.pdu.contextEngineID : this.msgSecurityParameters.msgAuthoritativeEngineID;
	if ( contextEngineID.length == 0 ) {
		scopedPduWriter.writeString ("");
	} else {
		scopedPduWriter.writeBuffer (contextEngineID, ber.OctetString);
	}
	scopedPduWriter.writeString (this.pdu.contextName);
	this.pdu.toBuffer (scopedPduWriter);
	scopedPduWriter.endSequence ();

	if ( this.hasPrivacy() ) {
		var authoritativeEngine = {
			engineID: this.msgSecurityParameters.msgAuthoritativeEngineID,
			engineBoots: this.msgSecurityParameters.msgAuthoritativeEngineBoots,
			engineTime: this.msgSecurityParameters.msgAuthoritativeEngineTime,
		};
		encryptionResult = Encryption.encryptPdu (this.user.privProtocol, scopedPduWriter.buffer,
				this.user.privKey, this.user.authProtocol, authoritativeEngine);
		writer.writeBuffer (encryptionResult.encryptedPdu, ber.OctetString);
	} else {
		writer.writeBuffer (scopedPduWriter.buffer);
	}

	writer.endSequence ();

	this.buffer = writer.buffer;

	if ( this.hasPrivacy() ) {
		Encryption.addParametersToMessageBuffer(this.buffer, encryptionResult.msgPrivacyParameters);
	}

	if ( this.hasAuthentication() ) {
		Authentication.addParametersToMessageBuffer(this.buffer, this.user.authProtocol, this.user.authKey,
			this.msgSecurityParameters.msgAuthoritativeEngineID);
	}

	return this.buffer;
};

Message.prototype.processIncomingSecurity = function (user, responseCb) {
	if ( this.hasPrivacy() ) {
		if ( ! this.decryptPdu(user, responseCb) ) {
			return false;
		}
	}

	if ( this.hasAuthentication() && ! this.isAuthenticationDisabled() ) {
		return this.checkAuthentication(user, responseCb);
	} else {
		return true;
	}
};

Message.prototype.decryptPdu = function (user, responseCb) {
	var decryptedPdu;
	var decryptedPduReader;
	try {
		var authoratitiveEngine = {
			engineID: this.msgSecurityParameters.msgAuthoritativeEngineID,
			engineBoots: this.msgSecurityParameters.msgAuthoritativeEngineBoots,
			engineTime: this.msgSecurityParameters.msgAuthoritativeEngineTime
		};
		decryptedPdu = Encryption.decryptPdu(user.privProtocol, this.encryptedPdu,
				this.msgSecurityParameters.msgPrivacyParameters, user.privKey, user.authProtocol,
				authoratitiveEngine);
		decryptedPduReader = new ber.Reader (decryptedPdu);
		this.pdu = readPdu(decryptedPduReader, true);
		return true;
	// really really occasionally the decrypt truncates a single byte
	// causing an ASN read failure in readPdu()
	// in this case, disabling auto padding decrypts the PDU correctly
	// this try-catch provides the workaround for this condition
	} catch (possibleTruncationError) {
		try {
			decryptedPdu = Encryption.decryptPdu(user.privProtocol, this.encryptedPdu,
					this.msgSecurityParameters.msgPrivacyParameters, user.privKey, user.authProtocol,
					this.msgSecurityParameters.msgAuthoritativeEngineID, true);
			decryptedPduReader = new ber.Reader (decryptedPdu);
			this.pdu = readPdu(decryptedPduReader, true);
			return true;
		} catch (error) {
			responseCb (new ResponseInvalidError ("Failed to decrypt PDU: " + error));
			return false;
		}
	}

};

Message.prototype.checkAuthentication = function (user, responseCb) {
	if ( Authentication.isAuthentic(this.buffer, user.authProtocol, user.authKey,
			this.msgSecurityParameters.msgAuthoritativeEngineID, this.msgSecurityParameters.msgAuthenticationParameters) ) {
		return true;
	} else {
		responseCb (new ResponseInvalidError ("Authentication digest "
				+ this.msgSecurityParameters.msgAuthenticationParameters.toString ('hex')
				+ " received in message does not match digest "
				+ Authentication.calculateDigest (buffer, user.authProtocol, user.authKey,
					this.msgSecurityParameters.msgAuthoritativeEngineID).toString ('hex')
				+ " calculated for message") );
		return false;
	}

};

Message.prototype.setMsgFlags = function (bitPosition, flag) {
	if ( this.msgGlobalData && this.msgGlobalData !== undefined && this.msgGlobalData !== null ) {
		if ( flag ) {
			this.msgGlobalData.msgFlags = this.msgGlobalData.msgFlags | ( 2 ** bitPosition );
		} else {
			this.msgGlobalData.msgFlags = this.msgGlobalData.msgFlags & ( 255 - 2 ** bitPosition );
		}
	}
}

Message.prototype.hasAuthentication = function () {
	return this.msgGlobalData && this.msgGlobalData.msgFlags && this.msgGlobalData.msgFlags & 1;
};

Message.prototype.setAuthentication = function (flag) {
	this.setMsgFlags (0, flag);
};

Message.prototype.hasPrivacy = function () {
	return this.msgGlobalData && this.msgGlobalData.msgFlags && this.msgGlobalData.msgFlags & 2;
};

Message.prototype.setPrivacy = function (flag) {
	this.setMsgFlags (1, flag);
};

Message.prototype.isReportable = function () {
	return this.msgGlobalData && this.msgGlobalData.msgFlags && this.msgGlobalData.msgFlags & 4;
};

Message.prototype.setReportable = function (flag) {
	this.setMsgFlags (2, flag);
};

Message.prototype.isAuthenticationDisabled = function () {
	return this.disableAuthentication;
};

Message.prototype.hasAuthoritativeEngineID = function () {
	return this.msgSecurityParameters && this.msgSecurityParameters.msgAuthoritativeEngineID &&
		this.msgSecurityParameters.msgAuthoritativeEngineID != "";
};

Message.prototype.createReportResponseMessage = function (engine, context) {
	var user = {
		name: "",
		level: SecurityLevel.noAuthNoPriv
	};
	var responseSecurityParameters = {
		msgAuthoritativeEngineID: engine.engineID,
		msgAuthoritativeEngineBoots: engine.engineBoots,
		msgAuthoritativeEngineTime: engine.engineTime,
		msgUserName: user.name,
		msgAuthenticationParameters: "",
		msgPrivacyParameters: ""
	};
	var reportPdu = ReportPdu.createFromVariables (this.pdu.id, [], {});
	reportPdu.contextName = context;
	var responseMessage = Message.createRequestV3 (user, responseSecurityParameters, reportPdu);
	responseMessage.msgGlobalData.msgID = this.msgGlobalData.msgID;
	return responseMessage;
};

Message.prototype.createResponseForRequest = function (responsePdu) {
	if ( this.version == Version3 ) {
		return this.createV3ResponseFromRequest(responsePdu);
	} else {
		return this.createCommunityResponseFromRequest(responsePdu);
	}
};

Message.prototype.createCommunityResponseFromRequest = function (responsePdu) {
	return Message.createCommunity(this.version, this.community, responsePdu);
};

Message.prototype.createV3ResponseFromRequest = function (responsePdu) {
	var responseUser = {
		name: this.user.name,
		level: this.user.name,
		authProtocol: this.user.authProtocol,
		authKey: this.user.authKey,
		privProtocol: this.user.privProtocol,
		privKey: this.user.privKey
	};
	var responseSecurityParameters = {
		msgAuthoritativeEngineID: this.msgSecurityParameters.msgAuthoritativeEngineID,
		msgAuthoritativeEngineBoots: this.msgSecurityParameters.msgAuthoritativeEngineBoots,
		msgAuthoritativeEngineTime: this.msgSecurityParameters.msgAuthoritativeEngineTime,
		msgUserName: this.msgSecurityParameters.msgUserName,
		msgAuthenticationParameters: "",
		msgPrivacyParameters: ""
	};
	var responseGlobalData = {
		msgID: this.msgGlobalData.msgID,
		msgMaxSize: 65507,
		msgFlags: this.msgGlobalData.msgFlags & (255 - 4),
		msgSecurityModel: 3
	};
	return Message.createV3 (responseUser, responseGlobalData, responseSecurityParameters, responsePdu);
};

Message.createCommunity = function (version, community, pdu) {
	var message = new Message ();

	message.version = version;
	message.community = community;
	message.pdu = pdu;

	return message;
};

Message.createRequestV3 = function (user, msgSecurityParameters, pdu) {
	var authFlag = user.level == SecurityLevel.authNoPriv || user.level == SecurityLevel.authPriv ? 1 : 0;
	var privFlag = user.level == SecurityLevel.authPriv ? 1 : 0;
	var reportableFlag = ( pdu.type == PduType.GetResponse || pdu.type == PduType.TrapV2 ) ? 0 : 1;
	var msgGlobalData = {
		msgID: _generateId(), // random ID
		msgMaxSize: 65507,
		msgFlags: reportableFlag * 4 | privFlag * 2 | authFlag * 1,
		msgSecurityModel: 3
	};
	return Message.createV3 (user, msgGlobalData, msgSecurityParameters, pdu);
};

Message.createV3 = function (user, msgGlobalData, msgSecurityParameters, pdu) {
	var message = new Message ();

	message.version = 3;
	message.user = user;
	message.msgGlobalData = msgGlobalData;
	message.msgSecurityParameters = {
		msgAuthoritativeEngineID: msgSecurityParameters.msgAuthoritativeEngineID || Buffer.from(""),
		msgAuthoritativeEngineBoots: msgSecurityParameters.msgAuthoritativeEngineBoots || 0,
		msgAuthoritativeEngineTime: msgSecurityParameters.msgAuthoritativeEngineTime || 0,
		msgUserName: user.name || "",
		msgAuthenticationParameters: "",
		msgPrivacyParameters: ""
	};
	message.pdu = pdu;

	return message;
};

Message.createDiscoveryV3 = function (pdu) {
	var msgSecurityParameters = {
		msgAuthoritativeEngineID: Buffer.from(""),
		msgAuthoritativeEngineBoots: 0,
		msgAuthoritativeEngineTime: 0
	};
	var emptyUser = {
		name: "",
		level: SecurityLevel.noAuthNoPriv
	};
	return Message.createRequestV3 (emptyUser, msgSecurityParameters, pdu);
}

Message.createFromBuffer = function (buffer, user) {
	var reader = new ber.Reader (buffer);
	var message = new Message();

	reader.readSequence ();

	message.version = reader.readInt ();

	if (message.version != 3) {
		message.community = reader.readString ();
		message.pdu = readPdu(reader, false);
	} else {
		// HeaderData
		message.msgGlobalData = {};
		reader.readSequence ();
		message.msgGlobalData.msgID = reader.readInt ();
		message.msgGlobalData.msgMaxSize = reader.readInt ();
		message.msgGlobalData.msgFlags = reader.readString (ber.OctetString, true)[0];
		message.msgGlobalData.msgSecurityModel = reader.readInt ();

		// msgSecurityParameters
		message.msgSecurityParameters = {};
		var msgSecurityParametersReader = new ber.Reader (reader.readString (ber.OctetString, true));
		msgSecurityParametersReader.readSequence ();
		message.msgSecurityParameters.msgAuthoritativeEngineID = msgSecurityParametersReader.readString (ber.OctetString, true);
		message.msgSecurityParameters.msgAuthoritativeEngineBoots = msgSecurityParametersReader.readInt ();
		message.msgSecurityParameters.msgAuthoritativeEngineTime = msgSecurityParametersReader.readInt ();
		message.msgSecurityParameters.msgUserName = msgSecurityParametersReader.readString ();
		message.msgSecurityParameters.msgAuthenticationParameters = Buffer.from(msgSecurityParametersReader.readString (ber.OctetString, true));
		message.msgSecurityParameters.msgPrivacyParameters = Buffer.from(msgSecurityParametersReader.readString (ber.OctetString, true));
		scopedPdu = true;

		if ( message.hasPrivacy() ) {
			message.encryptedPdu = reader.readString (ber.OctetString, true);
			message.pdu = null;
		} else {
			message.pdu = readPdu(reader, true);
		}
	}

	message.buffer = buffer;

	return message;
};


var Req = function (session, message, feedCb, responseCb, options) {

	this.message = message;
	this.responseCb = responseCb;
	this.retries = session.retries;
	this.timeout = session.timeout;
	// Add timeout backoff
	this.backoff = session.backoff;
	this.onResponse = session.onSimpleGetResponse;
	this.feedCb = feedCb;
	this.port = (options && options.port) ? options.port : session.port;
	this.context = session.context;
};

Req.prototype.getId = function() {
	return this.message.getReqId ();
};


/*****************************************************************************
 ** Session class definition
 **/

var Session = function (target, authenticator, options) {
	this.target = target || "127.0.0.1";

	this.version = (options && options.version)
			? options.version
			: Version1;

	if ( this.version == Version3 ) {
		this.user = authenticator;
	} else {
		this.community = authenticator || "public";
	}

	this.transport = (options && options.transport)
			? options.transport
			: "udp4";
	this.port = (options && options.port )
			? options.port
			: 161;
	this.trapPort = (options && options.trapPort )
			? options.trapPort
			: 162;

	this.retries = (options && (options.retries || options.retries == 0))
			? options.retries
			: 1;
	this.timeout = (options && options.timeout)
			? options.timeout
			: 5000;

	this.backoff = (options && options.backoff >= 1.0)
			? options.backoff
			: 1.0;

	this.sourceAddress = (options && options.sourceAddress )
			? options.sourceAddress
			: undefined;
	this.sourcePort = (options && options.sourcePort )
			? parseInt(options.sourcePort)
			: undefined;

	this.idBitsSize = (options && options.idBitsSize)
			? parseInt(options.idBitsSize)
			: 32;

	this.context = (options && options.context) ? options.context : "";

	DEBUG = options.debug;

	this.engine = new Engine ();
	this.reqs = {};
	this.reqCount = 0;

	this.dgram = dgram.createSocket (this.transport);
	this.dgram.unref();
	
	var me = this;
	this.dgram.on ("message", me.onMsg.bind (me));
	this.dgram.on ("close", me.onClose.bind (me));
	this.dgram.on ("error", me.onError.bind (me));

	if (this.sourceAddress || this.sourcePort)
		this.dgram.bind (this.sourcePort, this.sourceAddress);
};

util.inherits (Session, events.EventEmitter);

Session.prototype.close = function () {
	this.dgram.close ();
	return this;
};

Session.prototype.cancelRequests = function (error) {
	var id;
	for (id in this.reqs) {
		var req = this.reqs[id];
		this.unregisterRequest (req.getId ());
		req.responseCb (error);
	}
};

function _generateId (bitSize) {
	if (bitSize === 16) {
		return Math.floor(Math.random() * 10000) % 65535;
	}
	return Math.floor(Math.random() * 100000000) % 4294967295;
}

Session.prototype.get = function (oids, responseCb) {
	function feedCb (req, message) {
		var pdu = message.pdu;
		var varbinds = [];

		if (req.message.pdu.varbinds.length != pdu.varbinds.length) {
			req.responseCb (new ResponseInvalidError ("Requested OIDs do not "
					+ "match response OIDs"));
		} else {
			for (var i = 0; i < req.message.pdu.varbinds.length; i++) {
				if (req.message.pdu.varbinds[i].oid != pdu.varbinds[i].oid) {
					req.responseCb (new ResponseInvalidError ("OID '"
							+ req.message.pdu.varbinds[i].oid
							+ "' in request at positiion '" + i + "' does not "
							+ "match OID '" + pdu.varbinds[i].oid + "' in response "
							+ "at position '" + i + "'"));
					return;
				} else {
					varbinds.push (pdu.varbinds[i]);
				}
			}

			req.responseCb (null, varbinds);
		}
	}

	var pduVarbinds = [];

	for (var i = 0; i < oids.length; i++) {
		var varbind = {
			oid: oids[i]
		};
		pduVarbinds.push (varbind);
	}

	this.simpleGet (GetRequestPdu, feedCb, pduVarbinds, responseCb);

	return this;
};

Session.prototype.getBulk = function () {
	var oids, nonRepeaters, maxRepetitions, responseCb;

	if (arguments.length >= 4) {
		oids = arguments[0];
		nonRepeaters = arguments[1];
		maxRepetitions = arguments[2];
		responseCb = arguments[3];
	} else if (arguments.length >= 3) {
		oids = arguments[0];
		nonRepeaters = arguments[1];
		maxRepetitions = 10;
		responseCb = arguments[2];
	} else {
		oids = arguments[0];
		nonRepeaters = 0;
		maxRepetitions = 10;
		responseCb = arguments[1];
	}

	function feedCb (req, message) {
		var pdu = message.pdu;
		var varbinds = [];
		var i = 0;

		// first walk through and grab non-repeaters
		if (pdu.varbinds.length < nonRepeaters) {
			req.responseCb (new ResponseInvalidError ("Varbind count in "
					+ "response '" + pdu.varbinds.length + "' is less than "
					+ "non-repeaters '" + nonRepeaters + "' in request"));
		} else {
			for ( ; i < nonRepeaters; i++) {
				if (isVarbindError (pdu.varbinds[i])) {
					varbinds.push (pdu.varbinds[i]);
				} else if (! oidFollowsOid (req.message.pdu.varbinds[i].oid,
						pdu.varbinds[i].oid)) {
					req.responseCb (new ResponseInvalidError ("OID '"
							+ req.message.pdu.varbinds[i].oid + "' in request at "
							+ "positiion '" + i + "' does not precede "
							+ "OID '" + pdu.varbinds[i].oid + "' in response "
							+ "at position '" + i + "'"));
					return;
				} else {
					varbinds.push (pdu.varbinds[i]);
				}
			}
		}

		var repeaters = req.message.pdu.varbinds.length - nonRepeaters;

		// secondly walk through and grab repeaters
		if (pdu.varbinds.length % (repeaters)) {
			req.responseCb (new ResponseInvalidError ("Varbind count in "
					+ "response '" + pdu.varbinds.length + "' is not a "
					+ "multiple of repeaters '" + repeaters
					+ "' plus non-repeaters '" + nonRepeaters + "' in request"));
		} else {
			while (i < pdu.varbinds.length) {
				for (var j = 0; j < repeaters; j++, i++) {
					var reqIndex = nonRepeaters + j;
					var respIndex = i;

					if (isVarbindError (pdu.varbinds[respIndex])) {
						if (! varbinds[reqIndex])
							varbinds[reqIndex] = [];
						varbinds[reqIndex].push (pdu.varbinds[respIndex]);
					} else if (! oidFollowsOid (
							req.message.pdu.varbinds[reqIndex].oid,
							pdu.varbinds[respIndex].oid)) {
						req.responseCb (new ResponseInvalidError ("OID '"
								+ req.message.pdu.varbinds[reqIndex].oid
								+ "' in request at positiion '" + (reqIndex)
								+ "' does not precede OID '"
								+ pdu.varbinds[respIndex].oid
								+ "' in response at position '" + (respIndex) + "'"));
						return;
					} else {
						if (! varbinds[reqIndex])
							varbinds[reqIndex] = [];
						varbinds[reqIndex].push (pdu.varbinds[respIndex]);
					}
				}
			}
		}

		req.responseCb (null, varbinds);
	}

	var pduVarbinds = [];

	for (var i = 0; i < oids.length; i++) {
		var varbind = {
			oid: oids[i]
		};
		pduVarbinds.push (varbind);
	}

	var options = {
		nonRepeaters: nonRepeaters,
		maxRepetitions: maxRepetitions
	};

	this.simpleGet (GetBulkRequestPdu, feedCb, pduVarbinds, responseCb,
			options);

	return this;
};

Session.prototype.getNext = function (oids, responseCb) {
	function feedCb (req, message) {
		var pdu = message.pdu;
		var varbinds = [];

		if (req.message.pdu.varbinds.length != pdu.varbinds.length) {
			req.responseCb (new ResponseInvalidError ("Requested OIDs do not "
					+ "match response OIDs"));
		} else {
			for (var i = 0; i < req.message.pdu.varbinds.length; i++) {
				if (isVarbindError (pdu.varbinds[i])) {
					varbinds.push (pdu.varbinds[i]);
				} else if (! oidFollowsOid (req.message.pdu.varbinds[i].oid,
						pdu.varbinds[i].oid)) {
					req.responseCb (new ResponseInvalidError ("OID '"
							+ req.message.pdu.varbinds[i].oid + "' in request at "
							+ "positiion '" + i + "' does not precede "
							+ "OID '" + pdu.varbinds[i].oid + "' in response "
							+ "at position '" + i + "'"));
					return;
				} else {
					varbinds.push (pdu.varbinds[i]);
				}
			}

			req.responseCb (null, varbinds);
		}
	}

	var pduVarbinds = [];

	for (var i = 0; i < oids.length; i++) {
		var varbind = {
			oid: oids[i]
		};
		pduVarbinds.push (varbind);
	}

	this.simpleGet (GetNextRequestPdu, feedCb, pduVarbinds, responseCb);

	return this;
};

Session.prototype.inform = function () {
	var typeOrOid = arguments[0];
	var varbinds, options = {}, responseCb;

	/**
	 ** Support the following signatures:
	 ** 
	 **    typeOrOid, varbinds, options, callback
	 **    typeOrOid, varbinds, callback
	 **    typeOrOid, options, callback
	 **    typeOrOid, callback
	 **/
	if (arguments.length >= 4) {
		varbinds = arguments[1];
		options = arguments[2];
		responseCb = arguments[3];
	} else if (arguments.length >= 3) {
		if (arguments[1].constructor != Array) {
			varbinds = [];
			options = arguments[1];
			responseCb = arguments[2];
		} else {
			varbinds = arguments[1];
			responseCb = arguments[2];
		}
	} else {
		varbinds = [];
		responseCb = arguments[1];
	}

	if ( this.version == Version1 ) {
		responseCb (new RequestInvalidError ("Inform not allowed for SNMPv1"));
		return;
	}

	function feedCb (req, message) {
		var pdu = message.pdu;
		var varbinds = [];

		if (req.message.pdu.varbinds.length != pdu.varbinds.length) {
			req.responseCb (new ResponseInvalidError ("Inform OIDs do not "
					+ "match response OIDs"));
		} else {
			for (var i = 0; i < req.message.pdu.varbinds.length; i++) {
				if (req.message.pdu.varbinds[i].oid != pdu.varbinds[i].oid) {
					req.responseCb (new ResponseInvalidError ("OID '"
							+ req.message.pdu.varbinds[i].oid
							+ "' in inform at positiion '" + i + "' does not "
							+ "match OID '" + pdu.varbinds[i].oid + "' in response "
							+ "at position '" + i + "'"));
					return;
				} else {
					varbinds.push (pdu.varbinds[i]);
				}
			}

			req.responseCb (null, varbinds);
		}
	}

	if (typeof typeOrOid != "string")
		typeOrOid = "1.3.6.1.6.3.1.1.5." + (typeOrOid + 1);

	var pduVarbinds = [
		{
			oid: "1.3.6.1.2.1.1.3.0",
			type: ObjectType.TimeTicks,
			value: options.upTime || Math.floor (process.uptime () * 100)
		},
		{
			oid: "1.3.6.1.6.3.1.1.4.1.0",
			type: ObjectType.OID,
			value: typeOrOid
		}
	];

	for (var i = 0; i < varbinds.length; i++) {
		var varbind = {
			oid: varbinds[i].oid,
			type: varbinds[i].type,
			value: varbinds[i].value
		};
		pduVarbinds.push (varbind);
	}
	
	options.port = this.trapPort;

	this.simpleGet (InformRequestPdu, feedCb, pduVarbinds, responseCb, options);

	return this;
};

Session.prototype.onClose = function () {
	this.cancelRequests (new Error ("Socket forcibly closed"));
	this.emit ("close");
};

Session.prototype.onError = function (error) {
	this.emit (error);
};

Session.prototype.onMsg = function (buffer) {
	try {
		var message = Message.createFromBuffer (buffer);

		var req = this.unregisterRequest (message.getReqId ());
		if ( ! req )
			return;

		if ( ! message.processIncomingSecurity (this.user, req.responseCb) )
			return;

		try {
			if (message.version != req.message.version) {
				req.responseCb (new ResponseInvalidError ("Version in request '"
						+ req.message.version + "' does not match version in "
						+ "response '" + message.version + "'"));
			} else if (message.community != req.message.community) {
				req.responseCb (new ResponseInvalidError ("Community '"
						+ req.message.community + "' in request does not match "
						+ "community '" + message.community + "' in response"));
			} else if (message.pdu.type == PduType.Report) {
				this.msgSecurityParameters = {
					msgAuthoritativeEngineID: message.msgSecurityParameters.msgAuthoritativeEngineID,
					msgAuthoritativeEngineBoots: message.msgSecurityParameters.msgAuthoritativeEngineBoots,
					msgAuthoritativeEngineTime: message.msgSecurityParameters.msgAuthoritativeEngineTime
				};
				if ( this.proxy ) {
					this.msgSecurityParameters.msgUserName = this.proxy.user.name;
					this.msgSecurityParameters.msgAuthenticationParameters = "";
					this.msgSecurityParameters.msgPrivacyParameters = "";
				} else {
					if ( ! req.originalPdu ) {
						req.responseCb (new ResponseInvalidError ("Unexpected Report PDU") );
						return;
					}
					req.originalPdu.contextName = this.context;
					this.sendV3Req (req.originalPdu, req.feedCb, req.responseCb, req.options, req.port);
				}
			} else if ( this.proxy ) {
				this.onProxyResponse (req, message);
			} else if (message.pdu.type == PduType.GetResponse) {
				req.onResponse (req, message);
			} else {
				req.responseCb (new ResponseInvalidError ("Unknown PDU type '"
						+ message.pdu.type + "' in response"));
			}
		} catch (error) {
			req.responseCb (error);
		}
	} catch (error) {
		this.emit("error", error);
	}
};

Session.prototype.onSimpleGetResponse = function (req, message) {
	var pdu = message.pdu;

	if (pdu.errorStatus > 0) {
		var statusString = ErrorStatus[pdu.errorStatus]
				|| ErrorStatus.GeneralError;
		var statusCode = ErrorStatus[statusString]
				|| ErrorStatus[ErrorStatus.GeneralError];

		if (pdu.errorIndex <= 0 || pdu.errorIndex > pdu.varbinds.length) {
			req.responseCb (new RequestFailedError (statusString, statusCode));
		} else {
			var oid = pdu.varbinds[pdu.errorIndex - 1].oid;
			var error = new RequestFailedError (statusString + ": " + oid,
					statusCode);
			req.responseCb (error);
		}
	} else {
		req.feedCb (req, message);
	}
};

Session.prototype.registerRequest = function (req) {
	if (! this.reqs[req.getId ()]) {
		this.reqs[req.getId ()] = req;
		if (this.reqCount <= 0)
			this.dgram.ref();
		this.reqCount++;
	}
	var me = this;
	req.timer = setTimeout (function () {
		if (req.retries-- > 0) {
			me.send (req);
		} else {
			me.unregisterRequest (req.getId ());
			req.responseCb (new RequestTimedOutError (
					"Request timed out"));
		}
	}, req.timeout);
	// Apply timeout backoff
	if (req.backoff && req.backoff >= 1)
		req.timeout *= req.backoff;
};

Session.prototype.send = function (req, noWait) {
	try {
		var me = this;
		
		var buffer = req.message.toBuffer ();

		this.dgram.send (buffer, 0, buffer.length, req.port, this.target,
				function (error, bytes) {
			if (error) {
				req.responseCb (error);
			} else {
				if (noWait) {
					req.responseCb (null);
				} else {
					me.registerRequest (req);
				}
			}
		});
	} catch (error) {
		req.responseCb (error);
	}
	
	return this;
};

Session.prototype.set = function (varbinds, responseCb) {
	function feedCb (req, message) {
		var pdu = message.pdu;
		var varbinds = [];

		if (req.message.pdu.varbinds.length != pdu.varbinds.length) {
			req.responseCb (new ResponseInvalidError ("Requested OIDs do not "
					+ "match response OIDs"));
		} else {
			for (var i = 0; i < req.message.pdu.varbinds.length; i++) {
				if (req.message.pdu.varbinds[i].oid != pdu.varbinds[i].oid) {
					req.responseCb (new ResponseInvalidError ("OID '"
							+ req.message.pdu.varbinds[i].oid
							+ "' in request at positiion '" + i + "' does not "
							+ "match OID '" + pdu.varbinds[i].oid + "' in response "
							+ "at position '" + i + "'"));
					return;
				} else {
					varbinds.push (pdu.varbinds[i]);
				}
			}

			req.responseCb (null, varbinds);
		}
	}

	var pduVarbinds = [];

	for (var i = 0; i < varbinds.length; i++) {
		var varbind = {
			oid: varbinds[i].oid,
			type: varbinds[i].type,
			value: varbinds[i].value
		};
		pduVarbinds.push (varbind);
	}

	this.simpleGet (SetRequestPdu, feedCb, pduVarbinds, responseCb);

	return this;
};

Session.prototype.simpleGet = function (pduClass, feedCb, varbinds,
		responseCb, options) {
	try {
		var id = _generateId (this.idBitsSize);
		var pdu = SimplePdu.createFromVariables (pduClass, id, varbinds, options);
		var message;
		var req;

		if ( this.version == Version3 ) {
			if ( this.msgSecurityParameters ) {
				this.sendV3Req (pdu, feedCb, responseCb, options, this.port);
			} else {
				this.sendV3Discovery (pdu, feedCb, responseCb, options);
			}
		} else {
			message = Message.createCommunity (this.version, this.community, pdu);
			req = new Req (this, message, feedCb, responseCb, options);
			this.send (req);
		}
	} catch (error) {
		if (responseCb)
			responseCb (error);
	}
}

function subtreeCb (req, varbinds) {
	var done = 0;

	for (var i = varbinds.length; i > 0; i--) {
		if (! oidInSubtree (req.baseOid, varbinds[i - 1].oid)) {
			done = 1;
			varbinds.pop ();
		}
	}

	if (varbinds.length > 0)
		req.feedCb (varbinds);

	if (done)
		return true;
}

Session.prototype.subtree  = function () {
	var me = this;
	var oid = arguments[0];
	var maxRepetitions, feedCb, doneCb;

	if (arguments.length < 4) {
		maxRepetitions = 20;
		feedCb = arguments[1];
		doneCb = arguments[2];
	} else {
		maxRepetitions = arguments[1];
		feedCb = arguments[2];
		doneCb = arguments[3];
	}

	var req = {
		feedCb: feedCb,
		doneCb: doneCb,
		maxRepetitions: maxRepetitions,
		baseOid: oid
	};

	this.walk (oid, maxRepetitions, subtreeCb.bind (me, req), doneCb);

	return this;
};

function tableColumnsResponseCb (req, error) {
	if (error) {
		req.responseCb (error);
	} else if (req.error) {
		req.responseCb (req.error);
	} else {
		if (req.columns.length > 0) {
			var column = req.columns.pop ();
			var me = this;
			this.subtree (req.rowOid + column, req.maxRepetitions,
					tableColumnsFeedCb.bind (me, req),
					tableColumnsResponseCb.bind (me, req));
		} else {
			req.responseCb (null, req.table);
		}
	}
}

function tableColumnsFeedCb (req, varbinds) {
	for (var i = 0; i < varbinds.length; i++) {
		if (isVarbindError (varbinds[i])) {
			req.error = new RequestFailedError (varbindError (varbind[i]));
			return true;
		}

		var oid = varbinds[i].oid.replace (req.rowOid, "");
		if (oid && oid != varbinds[i].oid) {
			var match = oid.match (/^(\d+)\.(.+)$/);
			if (match && match[1] > 0) {
				if (! req.table[match[2]])
					req.table[match[2]] = {};
				req.table[match[2]][match[1]] = varbinds[i].value;
			}
		}
	}
}

Session.prototype.tableColumns = function () {
	var me = this;

	var oid = arguments[0];
	var columns = arguments[1];
	var maxRepetitions, responseCb;

	if (arguments.length < 4) {
		responseCb = arguments[2];
		maxRepetitions = 20;
	} else {
		maxRepetitions = arguments[2];
		responseCb = arguments[3];
	}

	var req = {
		responseCb: responseCb,
		maxRepetitions: maxRepetitions,
		baseOid: oid,
		rowOid: oid + ".1.",
		columns: columns.slice(0),
		table: {}
	};

	if (req.columns.length > 0) {
		var column = req.columns.pop ();
		this.subtree (req.rowOid + column, maxRepetitions,
				tableColumnsFeedCb.bind (me, req),
				tableColumnsResponseCb.bind (me, req));
	}

	return this;
};

function tableResponseCb (req, error) {
	if (error)
		req.responseCb (error);
	else if (req.error)
		req.responseCb (req.error);
	else
		req.responseCb (null, req.table);
}

function tableFeedCb (req, varbinds) {
	for (var i = 0; i < varbinds.length; i++) {
		if (isVarbindError (varbinds[i])) {
			req.error = new RequestFailedError (varbindError (varbind[i]));
			return true;
		}

		var oid = varbinds[i].oid.replace (req.rowOid, "");
		if (oid && oid != varbinds[i].oid) {
			var match = oid.match (/^(\d+)\.(.+)$/);
			if (match && match[1] > 0) {
				if (! req.table[match[2]])
					req.table[match[2]] = {};
				req.table[match[2]][match[1]] = varbinds[i].value;
			}
		}
	}
}

Session.prototype.table = function () {
	var me = this;

	var oid = arguments[0];
	var maxRepetitions, responseCb;

	if (arguments.length < 3) {
		responseCb = arguments[1];
		maxRepetitions = 20;
	} else {
		maxRepetitions = arguments[1];
		responseCb = arguments[2];
	}

	var req = {
		responseCb: responseCb,
		maxRepetitions: maxRepetitions,
		baseOid: oid,
		rowOid: oid + ".1.",
		table: {}
	};

	this.subtree (oid, maxRepetitions, tableFeedCb.bind (me, req),
			tableResponseCb.bind (me, req));

	return this;
};

Session.prototype.trap = function () {
	var req = {};

	try {
		var typeOrOid = arguments[0];
		var varbinds, options = {}, responseCb;
		var message;

		/**
		 ** Support the following signatures:
		 ** 
		 **    typeOrOid, varbinds, options, callback
		 **    typeOrOid, varbinds, agentAddr, callback
		 **    typeOrOid, varbinds, callback
		 **    typeOrOid, agentAddr, callback
		 **    typeOrOid, options, callback
		 **    typeOrOid, callback
		 **/
		if (arguments.length >= 4) {
			varbinds = arguments[1];
			if (typeof arguments[2] == "string") {
				options.agentAddr = arguments[2];
			} else if (arguments[2].constructor != Array) {
				options = arguments[2];
			}
			responseCb = arguments[3];
		} else if (arguments.length >= 3) {
			if (typeof arguments[1] == "string") {
				varbinds = [];
				options.agentAddr = arguments[1];
			} else if (arguments[1].constructor != Array) {
				varbinds = [];
				options = arguments[1];
			} else {
				varbinds = arguments[1];
				agentAddr = null;
			}
			responseCb = arguments[2];
		} else {
			varbinds = [];
			responseCb = arguments[1];
		}

		var pdu, pduVarbinds = [];

		for (var i = 0; i < varbinds.length; i++) {
			var varbind = {
				oid: varbinds[i].oid,
				type: varbinds[i].type,
				value: varbinds[i].value
			};
			pduVarbinds.push (varbind);
		}
		
		var id = _generateId (this.idBitsSize);

		if (this.version == Version2c || this.version == Version3 ) {
			if (typeof typeOrOid != "string")
				typeOrOid = "1.3.6.1.6.3.1.1.5." + (typeOrOid + 1);

			pduVarbinds.unshift (
				{
					oid: "1.3.6.1.2.1.1.3.0",
					type: ObjectType.TimeTicks,
					value: options.upTime || Math.floor (process.uptime () * 100)
				},
				{
					oid: "1.3.6.1.6.3.1.1.4.1.0",
					type: ObjectType.OID,
					value: typeOrOid
				}
			);

			pdu = TrapV2Pdu.createFromVariables (id, pduVarbinds, options);
		} else {
			pdu = TrapPdu.createFromVariables (typeOrOid, pduVarbinds, options);
		}

		if ( this.version == Version3 ) {
			var msgSecurityParameters = {
				msgAuthoritativeEngineID: this.user.engineID,
				msgAuthoritativeEngineBoots: 0,
				msgAuthoritativeEngineTime: 0
			};
			message = Message.createRequestV3 (this.user, msgSecurityParameters, pdu);
		} else {
			message = Message.createCommunity (this.version, this.community, pdu);
		}

		req = {
			id: id,
			message: message,
			responseCb: responseCb,
			port: this.trapPort
		};

		this.send (req, true);
	} catch (error) {
		if (req.responseCb)
			req.responseCb (error);
	}

	return this;
};

Session.prototype.unregisterRequest = function (id) {
	var req = this.reqs[id];
	if (req) {
		delete this.reqs[id];
		clearTimeout (req.timer);
		delete req.timer;
		this.reqCount--;
		if (this.reqCount <= 0)
			this.dgram.unref();
		return req;
	} else {
		return null;
	}
};

function walkCb (req, error, varbinds) {
	var done = 0;
	var oid;

	if (error) {
		if (error instanceof RequestFailedError) {
			if (error.status != ErrorStatus.NoSuchName) {
				req.doneCb (error);
				return;
			} else {
				// signal the version 1 walk code below that it should stop
				done = 1;
			}
		} else {
			req.doneCb (error);
			return;
		}
	}

	if (this.version == Version2c || this.version == Version3 ) {
		for (var i = varbinds[0].length; i > 0; i--) {
			if (varbinds[0][i - 1].type == ObjectType.EndOfMibView) {
				varbinds[0].pop ();
				done = 1;
			}
		}
		if (req.feedCb (varbinds[0]))
			done = 1;
		if (! done)
			oid = varbinds[0][varbinds[0].length - 1].oid;
	} else {
		if (! done) {
			if (req.feedCb (varbinds)) {
				done = 1;
			} else {
				oid = varbinds[0].oid;
			}
		}
	}

	if (done)
		req.doneCb (null);
	else
		this.walk (oid, req.maxRepetitions, req.feedCb, req.doneCb,
				req.baseOid);
}

Session.prototype.walk  = function () {
	var me = this;
	var oid = arguments[0];
	var maxRepetitions, feedCb, doneCb, baseOid;

	if (arguments.length < 4) {
		maxRepetitions = 20;
		feedCb = arguments[1];
		doneCb = arguments[2];
	} else {
		maxRepetitions = arguments[1];
		feedCb = arguments[2];
		doneCb = arguments[3];
	}

	var req = {
		maxRepetitions: maxRepetitions,
		feedCb: feedCb,
		doneCb: doneCb
	};

	if (this.version == Version2c || this.version == Version3)
		this.getBulk ([oid], 0, maxRepetitions,
				walkCb.bind (me, req));
	else
		this.getNext ([oid], walkCb.bind (me, req));

	return this;
};

Session.prototype.sendV3Req = function (pdu, feedCb, responseCb, options, port) {
	var message = Message.createRequestV3 (this.user, this.msgSecurityParameters, pdu);
	var reqOptions = options || {};
	var req = new Req (this, message, feedCb, responseCb, reqOptions);
	req.port = port;
	this.send (req);
};

Session.prototype.sendV3Discovery = function (originalPdu, feedCb, responseCb, options) {
	var discoveryPdu = createDiscoveryPdu(this.context);
	var discoveryMessage = Message.createDiscoveryV3 (discoveryPdu);
	var discoveryReq = new Req (this, discoveryMessage, feedCb, responseCb, options);
	discoveryReq.originalPdu = originalPdu;
	this.send (discoveryReq);
}

Session.prototype.onProxyResponse = function (req, message) {
	if ( message.version != Version3 ) {
		this.callback (new RequestFailedError ("Only SNMP version 3 contexts are supported"));
		return;
	}
	message.pdu.contextName = this.proxy.context;
	message.user = req.proxiedUser;
	message.setAuthentication ( ! (req.proxiedUser.level == SecurityLevel.noAuthNoPriv));
	message.setPrivacy (req.proxiedUser.level == SecurityLevel.authPriv);
	message.msgSecurityParameters = {
		msgAuthoritativeEngineID: req.proxiedEngine.engineID,
		msgAuthoritativeEngineBoots: req.proxiedEngine.engineBoots,
		msgAuthoritativeEngineTime: req.proxiedEngine.engineTime,
		msgUserName: req.proxiedUser.name,
		msgAuthenticationParameters: "",
		msgPrivacyParameters: ""
	};
	message.buffer = null;
	message.pdu.contextEngineID = message.msgSecurityParameters.msgAuthoritativeEngineID;
	message.pdu.contextName = this.proxy.context;
	message.pdu.id = req.proxiedPduId;
	this.proxy.listener.send (message, req.proxiedRinfo);
};

Session.create = function (target, community, options) {
	// Ensure that options may be optional
	var version = (options && options.version) ? options.version : Version1;
	if (version != Version1 && version != Version2c) {
		throw new ResponseInvalidError ("SNMP community session requested but version '" + options.version + "' specified in options not valid");
	} else {
		if (!options)
			options = {};
		options.version = version;
		return new Session (target, community, options);
	}
};

Session.createV3 = function (target, user, options) {
	// Ensure that options may be optional
	if ( options && options.version && options.version != Version3 ) {
		throw new ResponseInvalidError ("SNMPv3 session requested but version '" + options.version + "' specified in options");
	} else {
		if (!options)
			options = {};
		options.version = Version3;
	}
	return new Session (target, user, options);
};

var Engine = function (engineID, engineBoots, engineTime) {
	if ( engineID ) {
		this.engineID = Buffer.from (engineID, 'hex');
	} else {
		this.generateEngineID ();
	}
	this.engineBoots = 0;
	this.engineTime = 10;
};

Engine.prototype.generateEngineID = function() {
	// generate a 17-byte engine ID in the following format:
	// 0x80 + 0x00B983 (enterprise OID) | 0x80 (enterprise-specific format) | 12 bytes of random
	this.engineID = Buffer.alloc (17);
	this.engineID.fill ('8000B98380', 'hex', 0, 5);
	this.engineID.fill (crypto.randomBytes (12), 5, 17, 'hex');
}

var Listener = function (options, receiver) {
	this.receiver = receiver;
	this.callback = receiver.onMsg;
	this.family = options.transport || 'udp4';
	this.port = options.port || 161;
	this.disableAuthorization = options.disableAuthorization || false;
};

Listener.prototype.startListening = function (receiver) {
	var me = this;
	this.dgram = dgram.createSocket (this.family);
	this.dgram.bind (this.port);
	this.dgram.on ("message", me.callback.bind (me.receiver));
};

Listener.prototype.send = function (message, rinfo) {
	var me = this;
	
	var buffer = message.toBuffer ();

	this.dgram.send (buffer, 0, buffer.length, rinfo.port, rinfo.address,
			function (error, bytes) {
		if (error) {
			// me.callback (error);
			console.error ("Error sending: " + error.message);
		} else {
			// debug ("Listener sent response message");
		}
	});
};

Listener.formatCallbackData = function (pdu, rinfo) {
	if ( pdu.contextEngineID ) {
		pdu.contextEngineID = pdu.contextEngineID.toString('hex');
	}
	delete pdu.nonRepeaters;
	delete pdu.maxRepetitions;
	return {
		pdu: pdu,
		rinfo: rinfo 
	};
};

Listener.processIncoming = function (buffer, authorizer, callback) {
	var message = Message.createFromBuffer (buffer);
	var community;

	// Authorization
	if ( message.version == Version3 ) {
		message.user = authorizer.users.filter( localUser => localUser.name ==
				message.msgSecurityParameters.msgUserName )[0];
		message.disableAuthentication = authorizer.disableAuthorization;
		if ( ! message.user ) {
			if ( message.msgSecurityParameters.msgUserName != "" && ! authorizer.disableAuthorization ) {
				callback (new RequestFailedError ("Local user not found for message with user " +
						message.msgSecurityParameters.msgUserName));
				return;
			} else if ( message.hasAuthentication () ) {
				callback (new RequestFailedError ("Local user not found and message requires authentication with user " +
						message.msgSecurityParameters.msgUserName));
				return;
			} else {
				message.user = {
					name: "",
					level: SecurityLevel.noAuthNoPriv
				};
			}
		}
		if ( ! message.processIncomingSecurity (message.user, callback) ) {
			return;
		}
	} else {
		community = authorizer.communities.filter( localCommunity => localCommunity == message.community )[0];
		if ( ! community && ! authorizer.disableAuthorization ) {
			callback (new RequestFailedError ("Local community not found for message with community " + message.community));
			return;
		}
	}

	return message;
};

var Authorizer = function (disableAuthorization) {
	this.communities = [];
	this.users = [];
	this.disableAuthorization = disableAuthorization;
}

Authorizer.prototype.addCommunity = function (community) {
	if ( this.getCommunity (community) ) {
		return;
	} else {
		this.communities.push (community);
	}
};

Authorizer.prototype.getCommunity = function (community) {
	return this.communities.filter( localCommunity => localCommunity == community )[0] || null;
};

Authorizer.prototype.getCommunities = function () {
	return this.communities;
};

Authorizer.prototype.deleteCommunity = function (community) {
	var index = this.communities.indexOf(community);
	if ( index > -1 ) {
		this.communities.splice(index, 1);
	}
};

Authorizer.prototype.addUser = function (user) {
	if ( this.getUser (user.name) ) {
		this.deleteUser (user.name);
	}
	this.users.push (user);
};

Authorizer.prototype.getUser = function (userName) {
	return this.users.filter( localUser => localUser.name == userName )[0] || null;
};

Authorizer.prototype.getUsers = function () {
	return this.users;
};

Authorizer.prototype.deleteUser = function (userName) {
	var index = this.users.findIndex(localUser => localUser.name == userName );
	if ( index > -1 ) {
		this.users.splice(index, 1);
	}
};



/*****************************************************************************
 ** Receiver class definition
 **/

var Receiver = function (options, callback) {
	DEBUG = options.debug;
	this.listener = new Listener (options, this);
	this.authorizer = new Authorizer (options.disableAuthorization);
	this.engine = new Engine (options.engineID);

	this.engineBoots = 0;
	this.engineTime = 10;
	this.disableAuthorization = false;

	this.callback = callback;
	this.family = options.transport || 'udp4';
	this.port = options.port || 162;
	options.port = this.port;
	this.disableAuthorization = options.disableAuthorization || false;
	this.context = (options && options.context) ? options.context : "";
	this.listener = new Listener (options, this);
};

Receiver.prototype.getAuthorizer = function () {
	return this.authorizer;
};

Receiver.prototype.onMsg = function (buffer, rinfo) {
	var message = Listener.processIncoming (buffer, this.authorizer, this.callback);
	var reportMessage;

	if ( ! message ) {
		return;
	}

	// The only GetRequest PDUs supported are those used for SNMPv3 discovery
	if ( message.pdu.type == PduType.GetRequest ) {
		if ( message.version != Version3 ) {
			this.callback (new RequestInvalidError ("Only SNMPv3 discovery GetRequests are supported"));
			return;
		} else if ( message.hasAuthentication() ) {
			this.callback (new RequestInvalidError ("Only discovery (noAuthNoPriv) GetRequests are supported but this message has authentication"));
			return;
		} else if ( ! message.isReportable () ) {
			this.callback (new RequestInvalidError ("Only discovery GetRequests are supported and this message does not have the reportable flag set"));
			return;
		}
		var reportMessage = message.createReportResponseMessage (this.engine, this.context);
		this.listener.send (reportMessage, rinfo);
		return;
	};

	// Inform/trap processing
	debug (JSON.stringify (message.pdu, null, 2));
	if ( message.pdu.type == PduType.Trap || message.pdu.type == PduType.TrapV2 ) {
		this.callback (null, this.formatCallbackData (message.pdu, rinfo) );
	} else if ( message.pdu.type == PduType.InformRequest ) {
		message.pdu.type = PduType.GetResponse;
		message.buffer = null;
		message.setReportable (false);
		this.listener.send (message, rinfo);
		message.pdu.type = PduType.InformRequest;
		this.callback (null, this.formatCallbackData (message.pdu, rinfo) );
	} else {
		this.callback (new RequestInvalidError ("Unexpected PDU type " + message.pdu.type + " (" + PduType[message.pdu.type] + ")"));
	}
}

Receiver.prototype.formatCallbackData = function (pdu, rinfo) {
	if ( pdu.contextEngineID ) {
		pdu.contextEngineID = pdu.contextEngineID.toString('hex');
	}
	delete pdu.nonRepeaters;
	delete pdu.maxRepetitions;
	return {
		pdu: pdu,
		rinfo: rinfo 
	};
};

Receiver.prototype.close  = function() {
	this.listener.close ();
};

Receiver.create = function (options, callback) {
	var receiver = new Receiver (options, callback);
	receiver.listener.startListening ();
	return receiver;
};

var ModuleStore = function () {
	this.parser = mibparser ();
};

ModuleStore.prototype.getSyntaxTypes = function () {
	var syntaxTypes = {};
	Object.assign (syntaxTypes, ObjectType);
	var entryArray;

	// var mibModule = this.parser.Modules[moduleName];
	for ( var mibModule of Object.values (this.parser.Modules) ) {
		entryArray = Object.values (mibModule);
		for ( mibEntry of entryArray ) {
			if ( mibEntry.MACRO == "TEXTUAL-CONVENTION" ) {
				if ( mibEntry.SYNTAX && ! syntaxTypes[mibEntry.ObjectName] ) {
					if ( typeof mibEntry.SYNTAX == "object" ) {
						syntaxTypes[mibEntry.ObjectName] = syntaxTypes.Integer;
					} else {
						syntaxTypes[mibEntry.ObjectName] = syntaxTypes[mibEntry.SYNTAX];
					}
				}
			}
		}
	}
	return syntaxTypes;
};

ModuleStore.prototype.loadFromFile = function (fileName) {
	this.parser.Import (fileName);
	this.parser.Serialize ();
};

ModuleStore.prototype.getModule = function (moduleName) {
	return this.parser.Modules[moduleName];
};

ModuleStore.prototype.getModules = function (includeBase) {
	var modules = {};
	for ( var moduleName of Object.keys(this.parser.Modules) ) {
		if ( includeBase || ModuleStore.BASE_MODULES.indexOf (moduleName) == -1 ) {
			modules[moduleName] = this.parser.Modules[moduleName];
		}
	}
	return modules;
};

ModuleStore.prototype.getModuleNames = function (includeBase) {
	var modules = [];
	for ( var moduleName of Object.keys(this.parser.Modules) ) {
		if ( includeBase || ModuleStore.BASE_MODULES.indexOf (moduleName) == -1 ) {
			modules.push (moduleName);
		}
	}
	return modules;
};

ModuleStore.prototype.getProvidersForModule = function (moduleName) {
	var mibModule = this.parser.Modules[moduleName];
	var scalars = [];
	var tables = [];
	var mibEntry;
	var syntaxTypes;

	if ( ! mibModule ) {
		throw new ReferenceError ("MIB module " + moduleName + " not loaded");
	}
	syntaxTypes = this.getSyntaxTypes ();
	entryArray = Object.values (mibModule);
	for ( var i = 0; i < entryArray.length ; i++ ) {
		var mibEntry = entryArray[i];
		var syntax = mibEntry.SYNTAX;

		if ( syntax ) {
			if ( typeof syntax == "object" ) {
				syntax = "INTEGER";
			}
			if ( syntax.startsWith ("SEQUENCE OF") ) {
				// start of table
				currentTableProvider = {
					tableName: mibEntry.ObjectName,
					type: MibProviderType.Table,
					//oid: mibEntry.OID,
					tableColumns: [],
					tableIndex: [1]  // default - assume first column is index
				};
				// read table to completion
				while ( currentTableProvider || i >= entryArray.length ) {
					i++;
					mibEntry = entryArray[i];
					syntax = mibEntry.SYNTAX

					if ( typeof syntax == "object" ) {
						syntax = "INTEGER";
					}

					if ( mibEntry.MACRO == "SEQUENCE" ) {
						// table entry sequence - ignore
					} else  if ( ! mibEntry["OBJECT IDENTIFIER"] ) {
						// unexpected
					} else {
						parentOid = mibEntry["OBJECT IDENTIFIER"].split (" ")[0];
						if ( parentOid == currentTableProvider.tableName ) {
							// table entry
							currentTableProvider.name = mibEntry.ObjectName;
							currentTableProvider.oid = mibEntry.OID;
							if ( mibEntry.INDEX ) {
								currentTableProvider.tableIndex = [];
								for ( var indexEntry of mibEntry.INDEX ) {
									indexEntry = indexEntry.trim ();
									if ( indexEntry.includes(" ") ) {
										if ( indexEntry.split(" ")[0] == "IMPLIED" ) {
											currentTableProvider.tableIndex.push ({
												columnName: indexEntry.split(" ")[1],
												implied: true
											});
										} else {
											// unknown condition - guess that last token is name
											currentTableProvider.tableIndex.push ({
												columnName: indexEntry.split(" ").slice(-1)[0],
											});
										}
									} else {
										currentTableProvider.tableIndex.push ({
											columnName: indexEntry
										});
									}
								}
							}
							if ( mibEntry.AUGMENTS ) {
								currentTableProvider.tableAugments = mibEntry.AUGMENTS[0].trim();
								currentTableProvider.tableIndex = null;
							}
						} else if ( parentOid == currentTableProvider.name ) {
							// table column
							currentTableProvider.tableColumns.push ({
								number: parseInt (mibEntry["OBJECT IDENTIFIER"].split (" ")[1]),
								name: mibEntry.ObjectName,
								type: syntaxTypes[syntax]
							});
						} else {
							// table finished
							tables.push (currentTableProvider);
							// console.log ("Table: " + currentTableProvider.name);
							currentTableProvider = null;
							i--;
						}
					}
				}
			} else if ( mibEntry.MACRO == "OBJECT-TYPE" ) {
				// OBJECT-TYPE entries not in a table are scalars
				scalars.push ({
					name: mibEntry.ObjectName,
					type: MibProviderType.Scalar,
					oid: mibEntry.OID,
					scalarType: syntaxTypes[syntax]
				});
				// console.log ("Scalar: " + mibEntry.ObjectName);
			}
		}
	}
	return scalars.concat (tables);
};

ModuleStore.prototype.loadBaseModules = function () {
	for ( var mibModule of ModuleStore.BASE_MODULES ) {
		this.parser.Import("mibs/" + mibModule + ".mib");
	}
	this.parser.Serialize ();
};

ModuleStore.create = function () {
	store = new ModuleStore ();
	store.loadBaseModules ();
	return store;
};

ModuleStore.BASE_MODULES = [
	"RFC1155-SMI",
	"RFC1158-MIB",
	"RFC-1212",
	"RFC1213-MIB",
	"SNMPv2-SMI",
	"SNMPv2-CONF",
	"SNMPv2-TC",
	"SNMPv2-MIB"
];

var MibNode = function(address, parent) {
	this.address = address;
	this.oid = this.address.join('.');;
	this.parent = parent;
	this.children = {};
};

MibNode.prototype.child = function (index) {
	return this.children[index];
};

MibNode.prototype.listChildren = function (lowest) {
	var sorted = [];

	lowest = lowest || 0;

	this.children.forEach (function (c, i) {
		if (i >= lowest)
			sorted.push (i);
	});

	sorted.sort (function (a, b) {
		return (a - b);
	});

	return sorted;
};

MibNode.prototype.isDescendant = function (address) {
	return MibNode.oidIsDescended(this.address, address);
};

MibNode.prototype.isAncestor = function (address) {
	return MibNode.oidIsDescended (address, this.address);
};

MibNode.prototype.getAncestorProvider = function () {
	if ( this.provider ) {
		return this;
	} else if ( ! this.parent ) {
		return null;
	} else {
		return this.parent.getAncestorProvider ();
	}
};

MibNode.prototype.getInstanceNodeForTableRow = function () {
	var childCount = Object.keys (this.children).length;
	if ( childCount == 0 ) {
		if ( this.value != null ) {
			return this;
		} else {
			return null;
		}
	} else if ( childCount == 1 ) {
		return this.children[0].getInstanceNodeForTableRow();
	} else if ( childCount > 1 ) {
		return null;
	}
};

MibNode.prototype.getInstanceNodeForTableRowIndex = function (index) {
	var childCount = Object.keys (this.children).length;
	if ( childCount == 0 ) {
		if ( this.value != null ) {
			return this;
		} else {
			// not found
			return null;
		}
	} else {
		if ( index.length == 0 ) {
			return this.getInstanceNodeForTableRow();
		} else {
			var nextChildIndexPart = index[0];
			if ( ! nextChildIndexPart ) {
				return null;
			}
			remainingIndex = index.slice(1);
			return this.children[nextChildIndexPart].getInstanceNodeForTableRowIndex(remainingIndex);
		}
	}
};

MibNode.prototype.getInstanceNodesForColumn = function () {
	var columnNode = this;
	var instanceNode = this;
	var instanceNodes = [];

	while (instanceNode && ( instanceNode == columnNode || columnNode.isAncestor (instanceNode.address) ) ) {
		instanceNode = instanceNode.getNextInstanceNode ();
		if ( instanceNode && columnNode.isAncestor (instanceNode.address) ) {
			instanceNodes.push (instanceNode);
		}
	}
	return instanceNodes;
};

MibNode.prototype.getNextInstanceNode = function () {

	node = this;
	if ( this.value != null ) {
		// Need upwards traversal first
		node = this;
		while ( node ) {
			siblingIndex = node.address.slice(-1)[0];
			node = node.parent;
			if ( ! node ) {
				// end of MIB
				return null;
			} else {
				childrenAddresses = Object.keys (node.children).sort ( (a, b) => a - b);
				siblingPosition = childrenAddresses.indexOf(siblingIndex.toString());
				if ( siblingPosition + 1 < childrenAddresses.length ) {
					node = node.children[childrenAddresses[siblingPosition + 1]];
					break;
				}
			}
		}
	}
	// Descent
	while ( node ) {
		if ( node.value != null ) {
			return node;
		}
		childrenAddresses = Object.keys (node.children).sort ( (a, b) => a - b);
		node = node.children[childrenAddresses[0]];
		if ( ! node ) {
			// unexpected 
			return null;
		}
	}
};

MibNode.prototype.delete = function () {
	if ( Object.keys (this.children) > 0 ) {
		throw new Error ("Cannot delete non-leaf MIB node");
	}
	addressLastPart = this.address.slice(-1)[0];
	delete this.parent.children[addressLastPart];
	this.parent = null;
};

MibNode.prototype.pruneUpwards = function () {
	if ( ! this.parent ) {
		return
	}
	if ( Object.keys (this.children).length == 0 ) {
		var lastAddressPart = this.address.splice(-1)[0].toString();
		delete this.parent.children[lastAddressPart];
		this.parent.pruneUpwards();
		this.parent = null;
	}
}

MibNode.prototype.dump = function (options) {
	var valueString;
	if ( ( ! options.leavesOnly || options.showProviders ) && this.provider ) {
		console.log (this.oid + " [" + MibProviderType[this.provider.type] + ": " + this.provider.name + "]");
	} else if ( ( ! options.leavesOnly ) || Object.keys (this.children).length == 0 ) {
		if ( this.value != null ) {
			valueString = " = ";
			valueString += options.showTypes ? ObjectType[this.valueType] + ": " : "";
			valueString += options.showValues ? this.value : "";
		} else {
			valueString = "";
		}
		console.log (this.oid + valueString);
	}
	for ( node of Object.keys (this.children).sort ((a, b) => a - b)) {
		this.children[node].dump (options);
	}
};

MibNode.oidIsDescended = function (oid, ancestor) {
	var ancestorAddress = Mib.convertOidToAddress(ancestor);
	var address = Mib.convertOidToAddress(oid);
	var isAncestor = true;

	if (address.length <= ancestorAddress.length) {
		return false;
	}

	ancestorAddress.forEach (function (o, i) {
		if (address[i] !== ancestorAddress[i]) {
			isAncestor = false;
		}
	});

	return isAncestor;
};

var Mib = function () {
	this.root = new MibNode ([], null);
	this.providers = {};
	this.providerNodes = {};
};

Mib.prototype.addNodesForOid = function (oidString) {
	var address = Mib.convertOidToAddress (oidString);
	return this.addNodesForAddress (address);
};

Mib.prototype.addNodesForAddress = function (address) {
	var address;
	var node;
	var i;

	node = this.root;

	for (i = 0; i < address.length; i++) {
		if ( ! node.children.hasOwnProperty (address[i]) ) {
			node.children[address[i]] = new MibNode (address.slice(0, i + 1), node);
		}
		node = node.children[address[i]];
	}

	return node;
};

Mib.prototype.lookup = function (oid) {
	var address;
	var i;
	var node;

	address = Mib.convertOidToAddress (oid);
	node = this.root;
	for (i = 0; i < address.length; i++) {
		if ( ! node.children.hasOwnProperty (address[i])) {
			return null
		}
		node = node.children[address[i]];
	}

	return node;
};

Mib.prototype.getProviderNodeForInstance = function (instanceNode) {
	if ( instanceNode.provider ) {
		throw new ReferenceError ("Instance node has provider which should never happen");
	}
	return instanceNode.getAncestorProvider ();
};

Mib.prototype.addProviderToNode = function (provider) {
	var node = this.addNodesForOid (provider.oid);

	node.provider = provider;
	if ( provider.type == MibProviderType.Table ) {
		if ( ! provider.tableIndex ) {
			provider.tableIndex = [1];
		}
	}
	this.providerNodes[provider.name] = node;
	return node;
};

Mib.prototype.getColumnFromProvider = function (provider, indexEntry) {
	var column = null;
	if ( indexEntry.columnName ) {
		column = provider.tableColumns.filter (column => column.name == indexEntry.columnName )[0];
	} else if ( indexEntry.columnName ) {
		column = provider.tableColumns.filter (column => column.number == indexEntry.columnNumber )[0];
	}
	return column;
};

Mib.prototype.populateIndexEntryFromColumn = function (localProvider, indexEntry) {
	var column = null;
	var tableProviders;
	if ( ! indexEntry.columnName && ! indexEntry.columnNumber ) {
		throw new Error ("Index entry " + i + ": does not have either a columnName or columnNumber");
	}
	if ( indexEntry.foreign ) {
		// Explicit foreign table is first to search
		column = this.getColumnFromProvider (this.providers[indexEntry.foreign], indexEntry);
	} else {
		// If foreign table isn't given, search the local table next
		column = this.getColumnFromProvider (localProvider, indexEntry);
		if ( ! column ) {
			// as a last resort, try to find the column in a foreign table
			tableProviders = Object.values(this.providers).
					filter ( prov => prov.type == MibProviderType.Table );
			for ( var provider of tableProviders ) {
				column = this.getColumnFromProvider (provider, indexEntry);
				if ( column ) {
					indexEntry.foreign = provider.name;
					break;
				}
			}
		}
	}
	if ( ! column ) {
		throw new Error ("Could not find column for index entry with column " + indexEntry.columnName);
	}
	if ( indexEntry.columnName && indexEntry.columnName != column.name ) {
		throw new Error ("Index entry " + i + ": Calculated column name " + calculatedColumnName +
				"does not match supplied column name " + indexEntry.columnName);
	}
	if ( indexEntry.columnNumber && indexEntry.columnNumber != column.number ) {
		throw new Error ("Index entry " + i + ": Calculated column number " + calculatedColumnNumber +
				" does not match supplied column number " + indexEntry.columnNumber);
	}
	if ( ! indexEntry.columnName ) {
		indexEntry.columnName = column.name;
	}
	if ( ! indexEntry.columnNumber ) {
		indexEntry.columnNumber = column.number;
	}
	indexEntry.type = column.type;

};

Mib.prototype.registerProvider = function (provider) {
	this.providers[provider.name] = provider;
	if ( provider.type == MibProviderType.Table ) {
		if ( provider.tableAugments ) {
			if ( provider.tableAugments == provider.name ) {
				throw new Error ("Table " + provider.name + " cannot augment itself");
			}
			augmentProvider = this.providers[provider.tableAugments];
			if ( ! augmentProvider ) {
				throw new Error ("Cannot find base table " + provider.tableAugments + " to augment");
			}
			provider.tableIndex = JSON.parse(JSON.stringify(augmentProvider.tableIndex));
			provider.tableIndex.map (index => index.foreign = augmentProvider.name);
		} else {
			if ( ! provider.tableIndex ) {
				provider.tableIndex = [1]; // default to first column index
			}
			for ( var i = 0 ; i < provider.tableIndex.length ; i++ ) {
				var indexEntry = provider.tableIndex[i];
				if ( typeof indexEntry == 'number' ) {
					provider.tableIndex[i] = {
						columnNumber: indexEntry
					};
				} else if ( typeof indexEntry == 'string' ) {
					provider.tableIndex[i] = {
						columnName: indexEntry
					};
				}
				indexEntry = provider.tableIndex[i];
				this.populateIndexEntryFromColumn (provider, indexEntry);
			}
		}
	}
};

Mib.prototype.registerProviders = function (providers) {
	for ( var provider of providers ) {
		this.registerProvider (provider);
	}
};

Mib.prototype.unregisterProvider = function (name) {
	var providerNode = this.providerNodes[name];
	if ( providerNode ) {
		providerNodeParent = providerNode.parent;
		providerNode.delete();
		providerNodeParent.pruneUpwards();
		delete this.providerNodes[name];
	}
	delete this.providers[name];
};

Mib.prototype.getProvider = function (name) {
	return this.providers[name];
};

Mib.prototype.getProviders = function () {
	return this.providers;
};

Mib.prototype.dumpProviders = function () {
	var extraInfo;
	for ( var provider of Object.values(this.providers) ) {
		extraInfo = provider.type == MibProviderType.Scalar ? ObjectType[provider.scalarType] : "Columns = " + provider.tableColumns.length;
		console.log(MibProviderType[provider.type] + ": " + provider.name + " (" + provider.oid + "): " + extraInfo);
	}
};

Mib.prototype.getScalarValue = function (scalarName) {
	var providerNode = this.providerNodes[scalarName];
	if ( ! providerNode || ! providerNode.provider || providerNode.provider.type != MibProviderType.Scalar ) {
		throw new ReferenceError ("Failed to get node for registered MIB provider " + scalarName);
	}
	var instanceAddress = providerNode.address.concat ([0]);
	if ( ! this.lookup (instanceAddress) ) {
		throw new Error ("Failed created instance node for registered MIB provider " + scalarName);
	}
	var instanceNode = this.lookup (instanceAddress);
	return instanceNode.value;
};

Mib.prototype.setScalarValue = function (scalarName, newValue) {
	var providerNode;
	var instanceNode;

	if ( ! this.providers[scalarName] ) {
		throw new ReferenceError ("Provider " + scalarName + " not registered with this MIB");
	}

	providerNode = this.providerNodes[scalarName];
	if ( ! providerNode ) {
		providerNode = this.addProviderToNode (this.providers[scalarName]);
	}
	if ( ! providerNode || ! providerNode.provider || providerNode.provider.type != MibProviderType.Scalar ) {
		throw new ReferenceError ("Could not find MIB node for registered provider " + scalarName);
	}
	var instanceAddress = providerNode.address.concat ([0]);
	instanceNode = this.lookup (instanceAddress);
	if ( ! instanceNode ) {
		this.addNodesForAddress (instanceAddress);
		instanceNode = this.lookup (instanceAddress);
		instanceNode.valueType = providerNode.provider.scalarType;
	}
	instanceNode.value = newValue;
};

Mib.prototype.getProviderNodeForTable = function (table) {
	var providerNode;
	var provider;

	providerNode = this.providerNodes[table];
	if ( ! providerNode ) {
		throw new ReferenceError ("No MIB provider registered for " + table);
	}
	provider = providerNode.provider;
	if ( ! providerNode ) {
		throw new ReferenceError ("No MIB provider definition for registered provider " + table);
	}
	if ( provider.type != MibProviderType.Table ) {
		throw new TypeError ("Registered MIB provider " + table +
			" is not of the correct type (is type " + MibProviderType[provider.type] + ")");
	}
	return providerNode;
};

Mib.prototype.getOidAddressFromValue = function (value, indexPart) {
	var oidComponents;
	switch ( indexPart.type ) {
		case ObjectType.OID:
			oidComponents = value.split (".");
			break;
		case ObjectType.OctetString:
			oidComponents = [...value].map (c => c.charCodeAt());
			break;
		case ObjectType.IpAddress:
			return value.split (".");
		default:
			return [value];
	}
	if ( ! indexPart.implied && ! indexPart.length ) {
		oidComponents.unshift (oidComponents.length);
	}
	return oidComponents;
};

Mib.prototype.getValueFromOidAddress = function (oid, indexPart) {

};

Mib.prototype.getTableRowInstanceFromRow = function (provider, row) {
	var rowIndex = [];
	var foreignColumnParts;
	var localColumnParts;
	var localColumnPosition;
	var oidArrayForValue;

	// foreign columns are first in row
	foreignColumnParts = provider.tableIndex.filter ( indexPart => indexPart.foreign );
	for ( var i = 0; i < foreignColumnParts.length ; i++ ) {
		//rowIndex.push (row[i]);
		oidArrayForValue = this.getOidAddressFromValue (row[i], foreignColumnParts[i]);
		rowIndex = rowIndex.concat (oidArrayForValue);
	}
	// then local columns
	localColumnParts = provider.tableIndex.filter ( indexPart => ! indexPart.foreign );
	for ( var localColumnPart of localColumnParts ) {
		localColumnPosition = provider.tableColumns.findIndex (column => column.number == localColumnPart.columnNumber);
		oidArrayForValue = this.getOidAddressFromValue (row[foreignColumnParts.length + localColumnPosition], localColumnPart);
		rowIndex = rowIndex.concat (oidArrayForValue);
	}
	return rowIndex;
};

Mib.prototype.getRowIndexFromOid = function (oid, index) {
	var addressRemaining = oid.split (".");
	var length = 0;
	var values = [];
	for ( indexPart of index ) {
		switch ( indexPart.type ) {
			case ObjectType.OID:
				if ( indexPart.implied ) {
					length = addressRemaining.length;
				} else {
					length = addressRemaining.shift ();
				}
				value = addressRemaining.splice (0, length);
				values.push (value.join ("."));
				break;
			case ObjectType.IpAddress:
				length = 4;
				value = addressRemaining.splice (0, length);
				values.push (value.join ("."));
				break;
			case ObjectType.OctetString:
				if ( indexPart.implied ) {
					length = addressRemaining.length;
				} else {
					length = addressRemaining.shift ();
				}
				value = addressRemaining.splice (0, length);
				value = value.map (c => String.fromCharCode(c)).join ("");
				values.push (value);
				break;
			default:
				values.push (parseInt (addressRemaining.shift ()) );
		}
	}
	return values;
};

Mib.prototype.getTableRowInstanceFromRowIndex = function (provider, rowIndex) {
	rowIndexOid = [];
	for ( var i = 0; i < provider.tableIndex.length ; i++ ) {
		indexPart = provider.tableIndex[i];
		keyPart = rowIndex[i];
		rowIndexOid = rowIndexOid.concat (this.getOidAddressFromValue (keyPart, indexPart));
	}
	return rowIndexOid;
};

Mib.prototype.addTableRow = function (table, row) {
	var providerNode;
	var provider;
	var instance = [];
	var instanceAddress;
	var instanceNode;

	if ( this.providers[table] && ! this.providerNodes[table] ) {
		this.addProviderToNode (this.providers[table]);
	}
	providerNode = this.getProviderNodeForTable (table);
	provider = providerNode.provider;
	rowValueOffset = provider.tableIndex.filter ( indexPart => indexPart.foreign ).length;
	instance = this.getTableRowInstanceFromRow (provider, row);
	for ( var i = 0; i < provider.tableColumns.length ; i++ ) {
		var column = provider.tableColumns[i];
		instanceAddress = providerNode.address.concat (column.number).concat (instance);
		this.addNodesForAddress (instanceAddress);
		instanceNode = this.lookup (instanceAddress);
		instanceNode.valueType = column.type;
		instanceNode.value = row[rowValueOffset + i];
	}
};

Mib.prototype.getTableColumnDefinitions = function (table) {
	var providerNode;
	var provider;

	providerNode = this.getProviderNodeForTable (table);
	provider = providerNode.provider;
	return provider.tableColumns;
};

Mib.prototype.getTableColumnCells = function (table, columnNumber, includeInstances) {
	var provider = this.providers[table];
	var providerIndex = provider.tableIndex;
	var providerNode = this.getProviderNodeForTable (table);
	var columnNode = providerNode.children[columnNumber];
	var instanceNodes = columnNode.getInstanceNodesForColumn ();
	var instanceOid;
	var indexValues = [];
	var columnValues = [];

	for ( var instanceNode of instanceNodes ) {
		instanceOid = Mib.getSubOidFromBaseOid (instanceNode.oid, columnNode.oid);
		indexValues.push (this.getRowIndexFromOid (instanceOid, providerIndex));
		columnValues.push (instanceNode.value);
	}
	if ( includeInstances ) {
		return [ indexValues, columnValues ];
	} else {
		return columnValues;
	}
};

Mib.prototype.getTableRowCells = function (table, rowIndex) {
	var provider;
	var providerNode;
	var columnNode;
	var instanceAddress;
	var instanceNode;
	var row = [];

	provider = this.providers[table];
	providerNode = this.getProviderNodeForTable (table);
	instanceAddress = this.getTableRowInstanceFromRowIndex (provider, rowIndex);
	for ( var columnNumber of Object.keys (providerNode.children) ) {
		columnNode = providerNode.children[columnNumber];
		instanceNode = columnNode.getInstanceNodeForTableRowIndex (instanceAddress);
		row.push (instanceNode.value);
	}
	return row;
};

Mib.prototype.getTableCells = function (table, byRows, includeInstances) {
	var providerNode;
	var column;
	var data = [];

	providerNode = this.getProviderNodeForTable (table);
	for ( var columnNumber of Object.keys (providerNode.children) ) {
		column = this.getTableColumnCells (table, columnNumber, includeInstances);
		if ( includeInstances ) {
			data.push (...column);
			includeInstances = false;
		} else {
			data.push (column);
		}
	}

	if ( byRows ) {
		return Object.keys (data[0]).map (function (c) {
			return data.map (function (r) { return r[c]; });
		});
	} else {
		return data;
	}
	
};

Mib.prototype.getTableSingleCell = function (table, columnNumber, rowIndex) {
	var provider;
	var providerNode;
	var instanceAddress;
	var columnNode;
	var instanceNode;

	provider = this.providers[table];
	providerNode = this.getProviderNodeForTable (table);
	instanceAddress = this.getTableRowInstanceFromRowIndex (provider, rowIndex);
	columnNode = providerNode.children[columnNumber];
	instanceNode = columnNode.getInstanceNodeForTableRowIndex (instanceAddress);
	return instanceNode.value;
};

Mib.prototype.setTableSingleCell = function (table, columnNumber, rowIndex, value) {
	var provider;
	var providerNode;
	var columnNode;
	var instanceNode;

	provider = this.providers[table];
	providerNode = this.getProviderNodeForTable (table);
	instanceAddress = this.getTableRowInstanceFromRowIndex (provider, rowIndex);
	columnNode = providerNode.children[columnNumber];
	instanceNode = columnNode.getInstanceNodeForTableRowIndex (instanceAddress);
	instanceNode.value = value;
};

Mib.prototype.deleteTableRow = function (table, rowIndex) {
	var provider;
	var providerNode;
	var instanceAddress;
	var columnNode;
	var instanceNode;

	provider = this.providers[table];
	providerNode = this.getProviderNodeForTable (table);
	instanceAddress = this.getTableRowInstanceFromRowIndex (provider, rowIndex);
	for ( var columnNumber of Object.keys (providerNode.children) ) {
		columnNode = providerNode.children[columnNumber];
		instanceNode = columnNode.getInstanceNodeForTableRowIndex (instanceAddress);
		if ( instanceNode ) {
			instanceParentNode = instanceNode.parent;
			instanceNode.delete();
			instanceParentNode.pruneUpwards();
		} else {
			throw new ReferenceError ("Cannot find row for index " + rowIndex + " at registered provider " + table);
		}
	}
	return true;
};

Mib.prototype.dump = function (options) {
	if ( ! options ) {
		options = {};
	}
	var completedOptions = {
		leavesOnly: options.leavesOnly || true,
		showProviders: options.leavesOnly || true,
		showValues: options.leavesOnly || true,
		showTypes: options.leavesOnly || true
	};
	this.root.dump (completedOptions);
};

Mib.convertOidToAddress = function (oid) {
	var address;
	var oidArray;
	var i;

	if (typeof (oid) === 'object' && util.isArray(oid)) {
		address = oid;
	} else if (typeof (oid) === 'string') {
		address = oid.split('.');
	} else {
		throw new TypeError('oid (string or array) is required');
	}

	if (address.length < 3)
		throw new RangeError('object identifier is too short');

	oidArray = [];
	for (i = 0; i < address.length; i++) {
		var n;

		if (address[i] === '')
			continue;

		if (address[i] === true || address[i] === false) {
			throw new TypeError('object identifier component ' +
			    address[i] + ' is malformed');
		}

		n = Number(address[i]);

		if (isNaN(n)) {
			throw new TypeError('object identifier component ' +
			    address[i] + ' is malformed');
		}
		if (n % 1 !== 0) {
			throw new TypeError('object identifier component ' +
			    address[i] + ' is not an integer');
		}
		if (i === 0 && n > 2) {
			throw new RangeError('object identifier does not ' +
			    'begin with 0, 1, or 2');
		}
		if (i === 1 && n > 39) {
			throw new RangeError('object identifier second ' +
			    'component ' + n + ' exceeds encoding limit of 39');
		}
		if (n < 0) {
			throw new RangeError('object identifier component ' +
			    address[i] + ' is negative');
		}
		if (n > MAX_INT32) {
			throw new RangeError('object identifier component ' +
			    address[i] + ' is too large');
		}
		oidArray.push(n);
	}

	return oidArray;

};

Mib.getSubOidFromBaseOid = function (oid, base) {
	return oid.substring (base.length + 1);
}

var MibRequest = function (requestDefinition) {
	this.operation = requestDefinition.operation;
	this.address = Mib.convertOidToAddress (requestDefinition.oid);
	this.oid = this.address.join ('.');
	this.providerNode = requestDefinition.providerNode;
	this.instanceNode = requestDefinition.instanceNode;
};

MibRequest.prototype.isScalar = function () {
	return this.providerNode && this.providerNode.provider &&
		this.providerNode.provider.type == MibProviderType.Scalar;
};

MibRequest.prototype.isTabular = function () {
	return this.providerNode && this.providerNode.provider &&
		this.providerNode.provider.type == MibProviderType.Table;
};

var Agent = function (options, callback) {
	DEBUG = options.debug;
	this.listener = new Listener (options, this);
	this.engine = new Engine (options.engineID);
	this.authorizer = new Authorizer (options.disableAuthorization);
	this.mib = new Mib ();
	this.callback = callback || function () {};
	this.context = "";
	this.forwarder = new Forwarder (this.listener, this.callback);
};

Agent.prototype.getMib = function () {
	return this.mib;
};

Agent.prototype.getAuthorizer = function () {
	return this.authorizer;
};

Agent.prototype.registerProvider = function (provider) {
	this.mib.registerProvider (provider);
};

Agent.prototype.registerProviders = function (providers) {
	this.mib.registerProviders (providers);
};

Agent.prototype.unregisterProvider = function (provider) {
	this.mib.unregisterProvider (provider);
};

Agent.prototype.getProvider = function (provider) {
	return this.mib.getProvider (provider);
};

Agent.prototype.getProviders = function () {
	return this.mib.getProviders ();
};

Agent.prototype.onMsg = function (buffer, rinfo) {
	var message = Listener.processIncoming (buffer, this.authorizer, this.callback);
	var reportMessage;
	var responseMessage;

	if ( ! message ) {
		return;
	}

	// SNMPv3 discovery
	if ( message.version == Version3 && message.pdu.type == PduType.GetRequest &&
			! message.hasAuthoritativeEngineID() && message.isReportable () ) {
		reportMessage = message.createReportResponseMessage (this.engine, this.context);
		this.listener.send (reportMessage, rinfo);
		return;
	}

	// Request processing
	debug (JSON.stringify (message.pdu, null, 2));
	if ( message.pdu.contextName && message.pdu.contextName != "" ) {
		this.onProxyRequest (message, rinfo);
	} else if ( message.pdu.type == PduType.GetRequest ) {
		responseMessage = this.request (message, rinfo);
	} else if ( message.pdu.type == PduType.SetRequest ) {
		responseMessage = this.request (message, rinfo);
	} else if ( message.pdu.type == PduType.GetNextRequest ) {
		responseMessage = this.getNextRequest (message, rinfo);
	} else if ( message.pdu.type == PduType.GetBulkRequest ) {
		responseMessage = this.getBulkRequest (message, rinfo);
	} else {
		this.callback (new RequestInvalidError ("Unexpected PDU type " +
			message.pdu.type + " (" + PduType[message.pdu.type] + ")"));
	}

};

Agent.prototype.request = function (requestMessage, rinfo) {
	var me = this;
	var varbindsCompleted = 0;
	var requestPdu = requestMessage.pdu;
	var varbindsLength = requestPdu.varbinds.length;
	var responsePdu = requestPdu.getResponsePduForRequest ();

	for ( var i = 0; i < requestPdu.varbinds.length; i++ ) {
		var requestVarbind = requestPdu.varbinds[i];
		var instanceNode = this.mib.lookup (requestVarbind.oid);
		var providerNode;
		var mibRequest;
		var handler;
		var responseVarbindType;

		if ( ! instanceNode ) {
			mibRequest = new MibRequest ({
				operation: requestPdu.type,
				oid: requestVarbind.oid
			});
			handler = function getNsoHandler (mibRequestForNso) {
				mibRequestForNso.done ({
					errorStatus: ErrorStatus.NoSuchName,
					errorIndex: i
				});
			};
		} else {
			providerNode = this.mib.getProviderNodeForInstance (instanceNode);
			mibRequest = new MibRequest ({
				operation: requestPdu.type,
				providerNode: providerNode,
				instanceNode: instanceNode,
				oid: requestVarbind.oid
			});
			handler = providerNode.provider.handler;
		}

		mibRequest.done = function (error) {
			if ( error ) {
				responsePdu.errorStatus = error.errorStatus;
				responsePdu.errorIndex = error.errorIndex;
				responseVarbind = {
					oid: mibRequest.oid,
					type: ObjectType.Null,
					value: null
				};
			} else {
				if ( requestPdu.type == PduType.SetRequest ) {
					mibRequest.instanceNode.value = requestVarbind.value;
				}
				if ( ( requestPdu.type == PduType.GetNextRequest || requestPdu.type == PduType.GetBulkRequest ) &&
						requestVarbind.type == ObjectType.EndOfMibView ) {
					responseVarbindType = ObjectType.EndOfMibView;
				} else {
					responseVarbindType = mibRequest.instanceNode.valueType;
				}
				responseVarbind = {
					oid: mibRequest.oid,
					type: responseVarbindType,
					value: mibRequest.instanceNode.value
				};
			}
			me.setSingleVarbind (responsePdu, i, responseVarbind);
			if ( ++varbindsCompleted == varbindsLength) {
				me.sendResponse.call (me, rinfo, requestMessage, responsePdu);
			}
		};
		if ( handler ) {
			handler (mibRequest);
		} else {
			mibRequest.done ();
		}
	};
};

Agent.prototype.addGetNextVarbind = function (targetVarbinds, startOid) {
	var startNode = this.mib.lookup (startOid);
	var getNextNode;

	if ( ! startNode ) {
		// Off-tree start specified
		targetVarbinds.push ({
			oid: startOid,
			type: ObjectType.Null,
			value: null
		});
	} else {
		getNextNode = startNode.getNextInstanceNode();
		if ( ! getNextNode ) {
			// End of MIB
			targetVarbinds.push ({
				oid: startOid,
				type: ObjectType.EndOfMibView,
				value: null
			});
		} else {
			// Normal response
			targetVarbinds.push ({
				oid: getNextNode.oid,
				type: getNextNode.valueType,
				value: getNextNode.value
			});
		}
	}
	return getNextNode;
};

Agent.prototype.getNextRequest = function (requestMessage, rinfo) {
	var requestPdu = requestMessage.pdu;
	var varbindsLength = requestPdu.varbinds.length;
	var getNextVarbinds = [];

	for (var i = 0 ; i < varbindsLength ; i++ ) {
		this.addGetNextVarbind (getNextVarbinds, requestPdu.varbinds[i].oid);
	}

	requestMessage.pdu.varbinds = getNextVarbinds;
	this.request (requestMessage, rinfo);
};

Agent.prototype.getBulkRequest = function (requestMessage, rinfo) {
	var requestPdu = requestMessage.pdu;
	var requestVarbinds = requestPdu.varbinds;
	var getBulkVarbinds = [];
	var startOid = [];
	var getNextNode;
	var endOfMib = false;

	for (var n = 0 ; n < requestPdu.nonRepeaters ; n++ ) {
		this.addGetNextVarbind (getBulkVarbinds, requestVarbinds[n].oid);
	}

	for (var v = requestPdu.nonRepeaters ; v < requestVarbinds.length ; v++ ) {
		startOid.push (requestVarbinds[v].oid);
	}

	while ( getBulkVarbinds.length < requestPdu.maxRepetitions && ! endOfMib ) {
		for (var v = requestPdu.nonRepeaters ; v < requestVarbinds.length ; v++ ) {
			if (getBulkVarbinds.length < requestPdu.maxRepetitions ) {
				getNextNode = this.addGetNextVarbind (getBulkVarbinds, startOid[v - requestPdu.nonRepeaters]);
				if ( getNextNode ) {
					startOid[v - requestPdu.nonRepeaters] = getNextNode.oid;
					if ( getNextNode.type == ObjectType.EndOfMibView ) {
						endOfMib = true;
					}
				}
			}
		}
	}

	requestMessage.pdu.varbinds = getBulkVarbinds;
	this.request (requestMessage, rinfo);
};

Agent.prototype.setSingleVarbind = function (responsePdu, index, responseVarbind) {
	responsePdu.varbinds[index] = responseVarbind;
};

Agent.prototype.sendResponse = function (rinfo, requestMessage, responsePdu) {
	var responseMessage = requestMessage.createResponseForRequest (responsePdu);
	this.listener.send (responseMessage, rinfo);
	this.callback (null, Listener.formatCallbackData (responseMessage.pdu, rinfo) );
};

Agent.prototype.onProxyRequest = function (message, rinfo) {
	var contextName = message.pdu.contextName;
	var proxy;
	var proxiedPduId;
	var proxiedUser;

	if ( message.version != Version3 ) {
		this.callback (new RequestFailedError ("Only SNMP version 3 contexts are supported"));
		return;
	}
	proxy = this.forwarder.getProxy (contextName);
	if ( ! proxy ) {
		this.callback (new RequestFailedError ("No proxy found for message received with context " + contextName));
		return;
	}
	if ( ! proxy.session.msgSecurityParameters ) {
		// Discovery required - but chaining not implemented from here yet
		proxy.session.sendV3Discovery (null, null, this.callback, {});
	} else {
		message.msgSecurityParameters = proxy.session.msgSecurityParameters;
		message.setAuthentication ( ! (proxy.user.level == SecurityLevel.noAuthNoPriv));
		message.setPrivacy (proxy.user.level == SecurityLevel.authPriv);
		proxiedUser = message.user;
		message.user = proxy.user;
		message.buffer = null;
		message.pdu.contextEngineID = proxy.session.msgSecurityParameters.msgAuthoritativeEngineID;
		message.pdu.contextName = "";
		proxiedPduId = message.pdu.id;
		message.pdu.id = _generateId ();
		var req = new Req (proxy.session, message, null, this.callback, {}, true);
		req.port = proxy.port;
		req.proxiedRinfo = rinfo;
		req.proxiedPduId = proxiedPduId;
		req.proxiedUser = proxiedUser;
		req.proxiedEngine = this.engine;
		proxy.session.send (req);
	}
};

Agent.prototype.getForwarder = function () {
	return this.forwarder;
};

Agent.create = function (options, callback) {
	var agent = new Agent (options, callback);
	agent.listener.startListening ();
	return agent;
};

var Forwarder = function (listener, callback) {
	this.proxies = {};
	this.listener = listener;
	this.callback = callback;
};

Forwarder.prototype.addProxy = function (proxy) {
	var options = {
		version: Version3,
		port: proxy.port,
		transport: proxy.transport
	};
	proxy.session = Session.createV3 (proxy.target, proxy.user, options);
	proxy.session.proxy = proxy;
	proxy.session.proxy.listener = this.listener;
	this.proxies[proxy.context] = proxy;
	proxy.session.sendV3Discovery (null, null, this.callback);
};

Forwarder.prototype.deleteProxy = function (proxyName) {
	var proxy = this.proxies[proxyName];

	if ( proxy && proxy.session ) {
		proxy.session.close ();
	}
	delete this.proxies[proxyName];
};

Forwarder.prototype.getProxy = function (proxyName) {
	return this.proxies[proxyName];
};

Forwarder.prototype.getProxies = function () {
	return this.proxies;
};

Forwarder.prototype.dumpProxies = function () {
	var dump = {};
	for ( var proxy of Object.values (this.proxies) ) {
		dump[proxy.context] = {
			context: proxy.context,
			target: proxy.target,
			user: proxy.user,
			port: proxy.port
		}
	}
	console.log (JSON.stringify (dump, null, 2));
};



/*****************************************************************************
 ** Exports
 **/

exports.Session = Session;

exports.createSession = Session.create;
exports.createV3Session = Session.createV3;

exports.createReceiver = Receiver.create;
exports.createAgent = Agent.create;
exports.createModuleStore = ModuleStore.create;

exports.isVarbindError = isVarbindError;
exports.varbindError = varbindError;

exports.Version1 = Version1;
exports.Version2c = Version2c;
exports.Version3 = Version3;
exports.Version = Version;

exports.ErrorStatus = ErrorStatus;
exports.TrapType = TrapType;
exports.ObjectType = ObjectType;
exports.PduType = PduType;
exports.MibProviderType = MibProviderType;
exports.SecurityLevel = SecurityLevel;
exports.AuthProtocols = AuthProtocols;
exports.PrivProtocols = PrivProtocols;

exports.ResponseInvalidError = ResponseInvalidError;
exports.RequestInvalidError = RequestInvalidError;
exports.RequestFailedError = RequestFailedError;
exports.RequestTimedOutError = RequestTimedOutError;

/**
 ** We've added this for testing.
 **/
exports.ObjectParser = {
	readInt: readInt,
	readUint: readUint
};
exports.Authentication = Authentication;
exports.Encryption = Encryption;
