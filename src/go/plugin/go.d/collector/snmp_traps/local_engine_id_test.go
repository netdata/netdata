// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"net"
	"os"
	"sync/atomic"
	"testing"

	"github.com/gosnmp/gosnmp"
)

const testLocalEngineIDHex = "86123456789abcdef0123456"

func withEngineStateDir(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	prev := engineBootsDirBase
	engineBootsDirBase = tmpDir
	t.Cleanup(func() { engineBootsDirBase = prev })
}

func buildV3InformWithEngineID(t *testing.T, user, engineIDHex, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	engineID, err := hex.DecodeString(engineIDHex)
	if err != nil {
		t.Fatalf("invalid test engine ID: %v", err)
	}
	sp := &gosnmp.UsmSecurityParameters{
		UserName:                 user,
		AuthenticationProtocol:   gosnmp.NoAuth,
		PrivacyProtocol:          gosnmp.NoPriv,
		AuthoritativeEngineID:    string(engineID),
		AuthoritativeEngineBoots: 1,
		AuthoritativeEngineTime:  1,
	}
	g := &gosnmp.GoSNMP{
		Version:            gosnmp.Version3,
		SecurityModel:      gosnmp.UserSecurityModel,
		MsgFlags:           gosnmp.NoAuthNoPriv,
		SecurityParameters: sp,
		Logger:             trapDecodeLogger,
	}
	pdus := []gosnmp.SnmpPDU{
		{Name: sysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: snmpTrapOIDOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	data, err := g.SnmpEncodePacket(gosnmp.InformRequest, pdus, 0, 0)
	if err != nil {
		t.Fatalf("failed to marshal v3 INFORM test packet: %v", err)
	}
	return data
}

func TestLocalEngineIDConfiguredAcceptedAndPersisted(t *testing.T) {
	withEngineStateDir(t)

	lid, err := NewLocalEngineID("test-job", testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID with configured value failed: %v", err)
	}
	if lid.Hex() != testLocalEngineIDHex {
		t.Fatalf("local engine ID hex = %q, want %q", lid.Hex(), testLocalEngineIDHex)
	}

	lid2, err := NewLocalEngineID("test-job", "")
	if err != nil {
		t.Fatalf("NewLocalEngineID reload failed: %v", err)
	}
	if lid2.Hex() != testLocalEngineIDHex {
		t.Fatalf("reloaded local engine ID hex = %q, want %q", lid2.Hex(), testLocalEngineIDHex)
	}
}

func TestLocalEngineIDOmittedGeneratesPersistsAndReloads(t *testing.T) {
	withEngineStateDir(t)

	lid, err := NewLocalEngineID("test-job", "")
	if err != nil {
		t.Fatalf("NewLocalEngineID generation failed: %v", err)
	}
	generated := lid.Hex()
	raw, err := hex.DecodeString(generated)
	if err != nil {
		t.Fatalf("generated hex is invalid: %v", err)
	}
	if len(raw) < 5 || len(raw) > 32 {
		t.Fatalf("generated engine ID byte length = %d, want 5-32", len(raw))
	}
	if raw[0]&0x80 != 0 {
		t.Fatalf("generated engine ID first bit is set: %x", raw[0])
	}

	lid2, err := NewLocalEngineID("test-job", "")
	if err != nil {
		t.Fatalf("NewLocalEngineID reload failed: %v", err)
	}
	if lid2.Hex() != generated {
		t.Fatalf("reloaded local engine ID hex = %q, want %q", lid2.Hex(), generated)
	}
}

func TestLocalEngineIDInvalidFailsValidation(t *testing.T) {
	if err := validateLocalEngineID("nothex"); err == nil {
		t.Fatal("expected error for invalid hex local_engine_id")
	}
	if err := validateLocalEngineID("12"); err == nil {
		t.Fatal("expected error for too-short local_engine_id")
	}
	if err := validateLocalEngineID("0000000000"); err == nil {
		t.Fatal("expected error for all-zero local_engine_id")
	}
	if err := validateLocalEngineID("ffffffffff"); err == nil {
		t.Fatal("expected error for all-0xff local_engine_id")
	}
	if err := validateLocalEngineID(""); err != nil {
		t.Fatalf("empty local_engine_id should pass validation: %v", err)
	}
	if err := validateLocalEngineID(testLocalEngineIDHex); err != nil {
		t.Fatalf("valid local_engine_id should pass validation: %v", err)
	}
}

func TestLocalEngineIDInitFailsWhenStateCannotBeWritten(t *testing.T) {
	withEngineStateDir(t)

	if err := os.MkdirAll(engineBootsDir("test-job"), 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	if err := os.WriteFile(localEngineIDPath("test-job"), []byte("not-hex\n"), 0640); err != nil {
		t.Fatalf("write corrupt local engine id: %v", err)
	}

	if _, err := NewLocalEngineID("test-job", ""); err == nil {
		t.Fatal("expected error for corrupt persisted local engine ID")
	}
}

func TestLocalEngineIDInitFailsWhenDirCannotBeCreated(t *testing.T) {
	withEngineStateDir(t)

	dir := engineBootsDir("test-job")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	if err := os.WriteFile(localEngineIDPath("test-job"), []byte("abc\n"), 0640); err != nil {
		t.Fatalf("create file in place of dir: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if err := os.WriteFile(dir, []byte("blocker"), 0440); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	if _, err := NewLocalEngineID("test-job", ""); err == nil {
		t.Fatal("expected error when engine dir is a file")
	}
}

func TestCleanupCreatedEngineStateKeepsPreExistingDir(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-cleanup-preexisting-dir"
	if err := os.MkdirAll(engineBootsDir(jobName), 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}

	cleanupCreatedEngineState(jobName, true, true, false)

	if st, err := os.Stat(engineBootsDir(jobName)); err != nil || !st.IsDir() {
		t.Fatalf("pre-existing engine dir should remain, stat=%v err=%v", st, err)
	}
}

func TestCleanupCreatedEngineStateRemovesNewDir(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-cleanup-new-dir"
	if err := os.MkdirAll(engineBootsDir(jobName), 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	if err := os.WriteFile(engineBootsPath(jobName), []byte("1\n"), 0640); err != nil {
		t.Fatalf("write engine boots: %v", err)
	}
	if err := os.WriteFile(localEngineIDPath(jobName), []byte(testLocalEngineIDHex+"\n"), 0640); err != nil {
		t.Fatalf("write local engine ID: %v", err)
	}

	cleanupCreatedEngineState(jobName, true, true, true)

	if _, err := os.Stat(engineBootsDir(jobName)); !os.IsNotExist(err) {
		t.Fatalf("new engine dir should be removed, err=%v", err)
	}
}

func TestV3InformAcceptedWithLocalEngineID(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-inform-local"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	lid, err := NewLocalEngineID(jobName, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}

	data := buildV3InformWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}})
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}
	if err := registerUSMUsersWithLocalEngineID(secTable, []USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}}, lid.Bytes()); err != nil {
		t.Fatalf("registerUSMUsersWithLocalEngineID failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:       jobName,
		trapWriter:    writer,
		versions:      map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:     NewAllowlist(nil, nil),
		v3SecTable:    secTable,
		localEngineID: lid,
	}

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 0 {
		t.Fatalf("v3 INFORM with local engine ID should be accepted, got unknown_engine_id = %d", v)
	}
}

func TestV3InformRejectedWithNonLocalEngineID(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-inform-nonlocal"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	lid, err := NewLocalEngineID(jobName, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}

	data := buildV3InformWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}})
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}
	if err := registerUSMUsersWithLocalEngineID(secTable, []USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}}, lid.Bytes()); err != nil {
		t.Fatalf("registerUSMUsersWithLocalEngineID failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:       jobName,
		trapWriter:    writer,
		versions:      map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:     NewAllowlist(nil, nil),
		v3SecTable:    secTable,
		localEngineID: lid,
	}

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("v3 INFORM with non-local engine ID should be rejected, got unknown_engine_id = %d", v)
	}
	if len(writer.entries) != 0 {
		t.Fatalf("v3 INFORM with non-local engine ID should not be journaled, got %d entries", len(writer.entries))
	}
}

func TestV3TrapStillRequiresSenderEngineWhitelist(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-trap-sender-whitelist"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	lid, err := NewLocalEngineID(jobName, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}

	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}})
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}

	engineIDs, err := buildEngineIDWhitelist([]string{"80001f888077dfe44faa700259"})
	if err != nil {
		t.Fatalf("buildEngineIDWhitelist failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:       jobName,
		trapWriter:    writer,
		versions:      map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:     NewAllowlist(nil, nil),
		v3SecTable:    secTable,
		engineIDs:     engineIDs,
		localEngineID: lid,
	}

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("v3 Trap should be rejected when sender engine ID not in whitelist, got unknown_engine_id = %d", v)
	}
}

func TestV3InformResponseContainsLocalEngineID(t *testing.T) {
	withEngineStateDir(t)

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	lid, err := NewLocalEngineID("test-inform-resp", testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}

	engineID, err := hex.DecodeString(testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("invalid test engine ID: %v", err)
	}
	sp := &gosnmp.UsmSecurityParameters{
		UserName:                 "testuser",
		AuthenticationProtocol:   gosnmp.NoAuth,
		PrivacyProtocol:          gosnmp.NoPriv,
		AuthoritativeEngineID:    string(engineID),
		AuthoritativeEngineBoots: 1,
		AuthoritativeEngineTime:  1,
	}
	pkt := &gosnmp.SnmpPacket{
		Version:            gosnmp.Version3,
		MsgFlags:           gosnmp.NoAuthNoPriv,
		SecurityModel:      gosnmp.UserSecurityModel,
		SecurityParameters: sp,
		PDUType:            gosnmp.InformRequest,
		RequestID:          42,
		Variables: []gosnmp.SnmpPDU{
			{Name: sysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
			{Name: snmpTrapOIDOID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.6.3.1.1.5.1"},
		},
	}

	eb, err := NewEngineBoots("test-inform-resp")
	if err != nil {
		t.Fatalf("NewEngineBoots failed: %v", err)
	}

	if err := sendInformResponse(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), pkt, eb, lid.Bytes()); err != nil {
		t.Fatalf("sendInformResponse failed: %v", err)
	}

	respPkt := readInformResponse(t, peerConn)
	if respPkt.PDUType != gosnmp.GetResponse {
		t.Fatalf("response PDU type = %s, want GetResponse", respPkt.PDUType)
	}

	usp, ok := respPkt.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		t.Fatal("response security parameters are not USM")
	}
	respEngineID := hex.EncodeToString([]byte(usp.AuthoritativeEngineID))
	if respEngineID != testLocalEngineIDHex {
		t.Fatalf("response authoritative engine ID = %q, want %q", respEngineID, testLocalEngineIDHex)
	}
	if usp.AuthoritativeEngineBoots == 0 {
		t.Fatal("response engine boots is zero")
	}
}

func TestSendInformResponseV3AuthPriv(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-inform-response-authpriv"
	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	lid, err := NewLocalEngineID(jobName, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}
	user := USMUserConfig{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{user})
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}
	if err := registerUSMUsersWithLocalEngineID(secTable, []USMUserConfig{user}, lid.Bytes()); err != nil {
		t.Fatalf("registerUSMUsersWithLocalEngineID failed: %v", err)
	}

	reqData := buildV3SecuredInform(t, "testuser", testLocalEngineIDHex, "sha256", "aes", "authpassword", "privpassword", "1.3.6.1.6.3.1.1.5.1")
	reqCtx, err := DecodeTrap(reqData, net.ParseIP("10.1.2.3"), secTable)
	if err != nil {
		t.Fatalf("DecodeTrap failed: %v", err)
	}

	eb, err := NewEngineBoots(jobName)
	if err != nil {
		t.Fatalf("NewEngineBoots failed: %v", err)
	}
	if err := sendInformResponse(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), reqCtx.Packet, eb, lid.Bytes()); err != nil {
		t.Fatalf("sendInformResponse failed: %v", err)
	}

	localEngineID := lid.Bytes()
	decoderSP := &gosnmp.UsmSecurityParameters{
		UserName:                 "testuser",
		AuthenticationProtocol:   gosnmp.SHA256,
		AuthenticationPassphrase: "authpassword",
		PrivacyProtocol:          gosnmp.AES,
		PrivacyPassphrase:        "privpassword",
		AuthoritativeEngineID:    string(localEngineID),
	}
	if err := decoderSP.InitSecurityKeys(); err != nil {
		t.Fatalf("initialize response decoder keys: %v", err)
	}
	decoder := &gosnmp.GoSNMP{
		Version:            gosnmp.Version3,
		SecurityModel:      gosnmp.UserSecurityModel,
		MsgFlags:           gosnmp.AuthPriv,
		SecurityParameters: decoderSP,
		Logger:             trapDecodeLogger,
	}
	respPkt := readInformResponseWithDecoder(t, peerConn, decoder)
	if respPkt.PDUType != gosnmp.GetResponse {
		t.Fatalf("response PDU type = %s, want GetResponse", respPkt.PDUType)
	}
	if respPkt.RequestID != reqCtx.Packet.RequestID {
		t.Fatalf("response request ID = %d, want %d", respPkt.RequestID, reqCtx.Packet.RequestID)
	}

	usp, ok := respPkt.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		t.Fatal("response security parameters are not USM")
	}
	if got := hex.EncodeToString([]byte(usp.AuthoritativeEngineID)); got != testLocalEngineIDHex {
		t.Fatalf("response authoritative engine ID = %q, want %q", got, testLocalEngineIDHex)
	}
}

func TestCollectorCleanupWithLocalEngineID(t *testing.T) {
	withEngineStateDir(t)

	const jobName = "test-cleanup-lid"
	lid, err := NewLocalEngineID(jobName, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}

	c := &Collector{
		jobName:       jobName,
		localEngineID: lid,
	}
	c.Cleanup(nil)
	_ = c
}

func TestCollectorCleanupNilLocalEngineID(t *testing.T) {
	c := &Collector{
		jobName: "test-cleanup-nil-lid",
	}

	c.Cleanup(nil)
}
