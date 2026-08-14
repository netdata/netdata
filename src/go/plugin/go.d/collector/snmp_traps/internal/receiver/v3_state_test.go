// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/hex"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

const testLocalEngineIDHex = "86123456789abcdef0123456"

func TestEngineBootsPersistence(t *testing.T) {
	paths := newEngineStatePaths(t.TempDir(), "test-job")

	eb, err := newEngineBoots(paths)
	if err != nil {
		t.Fatalf("newEngineBoots failed: %v", err)
	}
	if got := eb.bootsValue(); got != 1 {
		t.Fatalf("first boot value = %d, want 1", got)
	}

	eb, err = newEngineBoots(paths)
	if err != nil {
		t.Fatalf("second newEngineBoots failed: %v", err)
	}
	if got := eb.bootsValue(); got != 2 {
		t.Fatalf("second boot value = %d, want 2", got)
	}
}

func TestEngineBootsRejectsInvalidState(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, engineStatePaths){
		"corrupt": func(t *testing.T, paths engineStatePaths) {
			if err := os.MkdirAll(paths.dir, 0750); err != nil {
				t.Fatalf("mkdir engine state dir: %v", err)
			}
			if err := os.WriteFile(paths.engineBoots, []byte("not-a-number\n"), 0640); err != nil {
				t.Fatalf("write engine boots: %v", err)
			}
		},
		"maximum": func(t *testing.T, paths engineStatePaths) {
			if err := os.MkdirAll(paths.dir, 0750); err != nil {
				t.Fatalf("mkdir engine state dir: %v", err)
			}
			if err := os.WriteFile(paths.engineBoots, []byte("2147483647\n"), 0640); err != nil {
				t.Fatalf("write engine boots: %v", err)
			}
		},
		"read error": func(t *testing.T, paths engineStatePaths) {
			if err := os.MkdirAll(paths.engineBoots, 0750); err != nil {
				t.Fatalf("mkdir engine boots path: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			paths := newEngineStatePaths(t.TempDir(), "test-job")
			setup(t, paths)
			if _, err := newEngineBoots(paths); err == nil {
				t.Fatal("expected invalid engine-boots state to fail creation")
			}
		})
	}
}

func TestEngineBootsEngineTimeCapsAtUint32(t *testing.T) {
	eb := &engineBoots{
		startedAt: time.Now().Add(-time.Duration(uint64(maxSnmpEngineTime)+1) * time.Second),
	}

	if got := eb.engineTime(); got != maxSnmpEngineTime {
		t.Fatalf("engine time = %d, want %d", got, maxSnmpEngineTime)
	}
}

func TestEngineStatePathExistsCheckedReturnsStatError(t *testing.T) {
	dir := t.TempDir()
	notDir := dir + "/not-a-dir"
	if err := os.WriteFile(notDir, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	exists, err := engineStatePathExistsChecked(notDir + "/engine-boots")
	if err == nil {
		t.Fatal("expected stat error")
	}
	if exists {
		t.Fatal("invalid path must not exist")
	}
}

func TestPrepareV3RollbackKeepsPreExistingEngineBootsPath(t *testing.T) {
	const jobName = "cleanup-v3-state"
	root := t.TempDir()
	paths := newEngineStatePaths(root, jobName)
	if err := os.MkdirAll(paths.engineBoots, 0750); err != nil {
		t.Fatalf("mkdir engine boots path: %v", err)
	}

	recv := New(NewPolicy(PolicyConfig{
		Versions:          []string{"v3"},
		USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
		EngineIDWhitelist: []string{testEngineIDHex},
	}), nil)
	if err := recv.PrepareV3(root, jobName); err == nil {
		t.Fatal("expected engine-boots preparation failure")
	}
	if _, err := os.Stat(paths.localEngineID); !os.IsNotExist(err) {
		t.Fatalf("local engine ID should be rolled back, err=%v", err)
	}
	if info, err := os.Stat(paths.engineBoots); err != nil || !info.IsDir() {
		t.Fatalf("pre-existing engine boots path should remain, info=%v err=%v", info, err)
	}
}

func withEngineStateDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
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
		{Name: model.SysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: model.SNMPTrapOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	data, err := g.SnmpEncodePacket(gosnmp.InformRequest, pdus, 0, 0)
	if err != nil {
		t.Fatalf("failed to marshal v3 INFORM test packet: %v", err)
	}
	return data
}

func TestLocalEngineIDConfiguredAcceptedAndPersisted(t *testing.T) {
	paths := newEngineStatePaths(withEngineStateDir(t), "test-job")

	lid, err := newLocalEngineID(paths, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID with configured value failed: %v", err)
	}
	if lid.hexString() != testLocalEngineIDHex {
		t.Fatalf("local engine ID hex = %q, want %q", lid.hexString(), testLocalEngineIDHex)
	}

	lid2, err := newLocalEngineID(paths, "")
	if err != nil {
		t.Fatalf("NewLocalEngineID reload failed: %v", err)
	}
	if lid2.hexString() != testLocalEngineIDHex {
		t.Fatalf("reloaded local engine ID hex = %q, want %q", lid2.hexString(), testLocalEngineIDHex)
	}
}

func TestLocalEngineIDOmittedGeneratesPersistsAndReloads(t *testing.T) {
	paths := newEngineStatePaths(withEngineStateDir(t), "test-job")

	lid, err := newLocalEngineID(paths, "")
	if err != nil {
		t.Fatalf("NewLocalEngineID generation failed: %v", err)
	}
	generated := lid.hexString()
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

	lid2, err := newLocalEngineID(paths, "")
	if err != nil {
		t.Fatalf("NewLocalEngineID reload failed: %v", err)
	}
	if lid2.hexString() != generated {
		t.Fatalf("reloaded local engine ID hex = %q, want %q", lid2.hexString(), generated)
	}
}

func TestLocalEngineIDInvalidFailsValidation(t *testing.T) {
	if err := ValidateLocalEngineID("nothex"); err == nil {
		t.Fatal("expected error for invalid hex local_engine_id")
	}
	if err := ValidateLocalEngineID("12"); err == nil {
		t.Fatal("expected error for too-short local_engine_id")
	}
	if err := ValidateLocalEngineID("0000000000"); err == nil {
		t.Fatal("expected error for all-zero local_engine_id")
	}
	if err := ValidateLocalEngineID("ffffffffff"); err == nil {
		t.Fatal("expected error for all-0xff local_engine_id")
	}
	if err := ValidateLocalEngineID(""); err != nil {
		t.Fatalf("empty local_engine_id should pass validation: %v", err)
	}
	if err := ValidateLocalEngineID(testLocalEngineIDHex); err != nil {
		t.Fatalf("valid local_engine_id should pass validation: %v", err)
	}
}

func TestLocalEngineIDInitFailsWhenStateCannotBeWritten(t *testing.T) {
	paths := newEngineStatePaths(withEngineStateDir(t), "test-job")

	if err := os.MkdirAll(paths.dir, 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	if err := os.WriteFile(paths.localEngineID, []byte("not-hex\n"), 0640); err != nil {
		t.Fatalf("write corrupt local engine id: %v", err)
	}

	if _, err := newLocalEngineID(paths, ""); err == nil {
		t.Fatal("expected error for corrupt persisted local engine ID")
	}
}

func TestLocalEngineIDInitFailsWhenDirCannotBeCreated(t *testing.T) {
	paths := newEngineStatePaths(withEngineStateDir(t), "test-job")

	dir := paths.dir
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	if err := os.WriteFile(paths.localEngineID, []byte("abc\n"), 0640); err != nil {
		t.Fatalf("create file in place of dir: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if err := os.WriteFile(dir, []byte("blocker"), 0440); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	if _, err := newLocalEngineID(paths, ""); err == nil {
		t.Fatal("expected error when engine dir is a file")
	}
}

func TestCleanupCreatedEngineStateKeepsPreExistingDir(t *testing.T) {
	const jobName = "test-cleanup-preexisting-dir"
	paths := newEngineStatePaths(withEngineStateDir(t), jobName)
	if err := os.MkdirAll(paths.dir, 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}

	cleanupCreatedEngineState(paths, true, true, false)

	if st, err := os.Stat(paths.dir); err != nil || !st.IsDir() {
		t.Fatalf("pre-existing engine dir should remain, stat=%v err=%v", st, err)
	}
}

func TestCleanupCreatedEngineStateRemovesNewDir(t *testing.T) {
	const jobName = "test-cleanup-new-dir"
	paths := newEngineStatePaths(withEngineStateDir(t), jobName)
	if err := os.MkdirAll(paths.dir, 0750); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	if err := os.WriteFile(paths.engineBoots, []byte("1\n"), 0640); err != nil {
		t.Fatalf("write engine boots: %v", err)
	}
	if err := os.WriteFile(paths.localEngineID, []byte(testLocalEngineIDHex+"\n"), 0640); err != nil {
		t.Fatalf("write local engine ID: %v", err)
	}

	cleanupCreatedEngineState(paths, true, true, true)

	if _, err := os.Stat(paths.dir); !os.IsNotExist(err) {
		t.Fatalf("new engine dir should be removed, err=%v", err)
	}
}

func TestV3InformAcceptedWithLocalEngineID(t *testing.T) {
	data := buildV3InformWithEngineID(t, "testuser", testLocalEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU == nil {
		t.Fatalf("result = %+v, want accepted INFORM", result)
	}
	if got := events.count(EventError, "unknown_engine_id"); got != 0 {
		t.Fatalf("unknown_engine_id events = %d, want 0", got)
	}
}

func TestV3InformRejectedWithNonLocalEngineID(t *testing.T) {
	data := buildV3InformWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events := newStaticV3TestReceiver(t, []string{testEngineIDHex})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("result = %+v, want policy drop", result)
	}
	if got := events.count(EventError, "unknown_engine_id"); got != 1 {
		t.Fatalf("unknown_engine_id events = %d, want 1", got)
	}
}

func TestV3TrapStillRequiresSenderEngineWhitelist(t *testing.T) {
	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	recv, events := newStaticV3TestReceiver(t, []string{"80001f888077dfe44faa700259"})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	if result := recv.Process(Datagram{Data: data, PeerIP: peer.IP, Peer: peer}); result.PDU != nil || result.DecodeFailure != nil {
		t.Fatalf("result = %+v, want policy drop", result)
	}
	if got := events.count(EventError, "unknown_engine_id"); got != 1 {
		t.Fatalf("unknown_engine_id events = %d, want 1", got)
	}
}

func newStaticV3TestReceiver(t *testing.T, whitelist []string) (*Receiver, *receiverEventRecorder) {
	t.Helper()
	recorder := &receiverEventRecorder{}
	recv := New(NewPolicy(PolicyConfig{
		Versions:          []string{"v3"},
		USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
		EngineIDWhitelist: whitelist,
		LocalEngineID:     testLocalEngineIDHex,
	}), recorder.report)
	if err := recv.PrepareV3(t.TempDir(), "static-v3-test"); err != nil {
		t.Fatalf("prepare v3 receiver: %v", err)
	}
	t.Cleanup(recv.RollbackPreparedState)
	return recv, recorder
}

func TestV3InformResponseContainsLocalEngineID(t *testing.T) {
	paths := newEngineStatePaths(withEngineStateDir(t), "test-inform-resp")

	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	lid, err := newLocalEngineID(paths, testLocalEngineIDHex)
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
			{Name: model.SysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
			{Name: model.SNMPTrapOID, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.6.3.1.1.5.1"},
		},
	}

	eb, err := newEngineBoots(paths)
	if err != nil {
		t.Fatalf("NewEngineBoots failed: %v", err)
	}

	if err := sendInformResponse(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), pkt, eb, lid.bytes()); err != nil {
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
	const jobName = "test-inform-response-authpriv"
	paths := newEngineStatePaths(withEngineStateDir(t), jobName)
	listenerConn, peerConn := informUDPConnPair(t)
	defer listenerConn.Close()
	defer peerConn.Close()

	lid, err := newLocalEngineID(paths, testLocalEngineIDHex)
	if err != nil {
		t.Fatalf("NewLocalEngineID failed: %v", err)
	}
	user := USMUser{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}
	secTable := newTestV3SecurityTable(t, user)
	if err := registerUSMUsersWithLocalEngineID(secTable, []USMUser{user}, lid.bytes()); err != nil {
		t.Fatalf("register local engine ID: %v", err)
	}

	reqData := buildV3SecuredInform(t, v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: testLocalEngineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "authpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
	})
	reqCtx, err := decodeTrap(reqData, net.ParseIP("10.1.2.3"), secTable)
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}

	eb, err := newEngineBoots(paths)
	if err != nil {
		t.Fatalf("NewEngineBoots failed: %v", err)
	}
	if err := sendInformResponse(listenerConn, peerConn.LocalAddr().(*net.UDPAddr), reqCtx.Packet, eb, lid.bytes()); err != nil {
		t.Fatalf("sendInformResponse failed: %v", err)
	}

	localEngineID := lid.bytes()
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
