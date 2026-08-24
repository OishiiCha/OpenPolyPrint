// Package pppp implements the PPPP (peer-to-peer) protocol used by AnkerMake M5 printers.
//
// This file defines the packet types, enums, and message parsing/packing logic.
// It is the Go equivalent of libflagship/pppp.py from the Python implementation.
package pppp

import (
	"encoding/binary"
	"fmt"

	"github.com/lucas/openpolyprint/internal/anker/proto/types"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

// Type is the PPPP message type enum.
type Type uint8

const (
	TypeHello              Type = 0x00
	TypeHelloAck           Type = 0x01
	TypeHelloTo            Type = 0x02
	TypeHelloToAck         Type = 0x03
	TypeQueryDid           Type = 0x08
	TypeQueryDidAck        Type = 0x09
	TypeDevLgn             Type = 0x10
	TypeDevLgnAck          Type = 0x11
	TypeDevLgnCrc          Type = 0x12
	TypeDevLgnAckCrc       Type = 0x13
	TypeDevLgnKey          Type = 0x14
	TypeDevLgnAckKey       Type = 0x15
	TypeDevLgnDsk          Type = 0x16
	TypeDevOnlineReq       Type = 0x18
	TypeDevOnlineReqAck    Type = 0x19
	TypeP2PReq             Type = 0x20
	TypeP2PReqAck          Type = 0x21
	TypeP2PReqDsk          Type = 0x26
	TypeLanSearch          Type = 0x30
	TypeLanNotify          Type = 0x31
	TypeLanNotifyAck       Type = 0x32
	TypePunchTo            Type = 0x40
	TypePunchPkt           Type = 0x41
	TypeP2PRdy             Type = 0x42
	TypeP2PRdyAck          Type = 0x43
	TypeRsLgn              Type = 0x60
	TypeRsLgnAck           Type = 0x61
	TypeRsLgn1             Type = 0x62
	TypeRsLgn1Ack          Type = 0x63
	TypeListReq1           Type = 0x67
	TypeListReq            Type = 0x68
	TypeListReqAck         Type = 0x69
	TypeListReqDsk         Type = 0x6a
	TypeRlyHello           Type = 0x70
	TypeRlyHelloAck        Type = 0x71
	TypeRlyPort            Type = 0x72
	TypeRlyPortAck         Type = 0x73
	TypeRlyPortKey         Type = 0x74
	TypeRlyPortAckKey      Type = 0x75
	TypeRlyByteCount       Type = 0x78
	TypeRlyReq             Type = 0x80
	TypeRlyReqAck          Type = 0x81
	TypeRlyTo              Type = 0x82
	TypeRlyPkt             Type = 0x83
	TypeRlyRdy             Type = 0x84
	TypeRlyToAck           Type = 0x85
	TypeRlyServerReq       Type = 0x87
	TypeSdevRun            Type = 0x90
	TypeSdevLgn            Type = 0x91
	TypeSdevLgnCrc         Type = 0x92
	TypeSdevReport         Type = 0x94
	TypeConnectReport      Type = 0xa0
	TypeReportReq          Type = 0xa1
	TypeReport             Type = 0xa2
	TypeDrw                Type = 0xd0
	TypeDrwAck             Type = 0xd1
	TypePsr                Type = 0xd8
	TypeAlive              Type = 0xe0
	TypeAliveAck           Type = 0xe1
	TypeClose              Type = 0xf0
	TypeMgmDumpLoginDid    Type = 0xf4
	TypeMgmDumpLoginDidDet Type = 0xf5
	TypeMgmDumpLoginDid1   Type = 0xf6
	TypeMgmLogControl      Type = 0xf7
	TypeMgmRemoteMgmt      Type = 0xf8
	TypeReportSessionReady Type = 0xf9
	TypeInvalid            Type = 0xff
)

func (t Type) String() string {
	switch t {
	case TypeHello:
		return "HELLO"
	case TypeHelloAck:
		return "HELLO_ACK"
	case TypeHelloTo:
		return "HELLO_TO"
	case TypeHelloToAck:
		return "HELLO_TO_ACK"
	case TypeQueryDid:
		return "QUERY_DID"
	case TypeQueryDidAck:
		return "QUERY_DID_ACK"
	case TypeDevLgn:
		return "DEV_LGN"
	case TypeDevLgnAck:
		return "DEV_LGN_ACK"
	case TypeDevLgnCrc:
		return "DEV_LGN_CRC"
	case TypeDevLgnAckCrc:
		return "DEV_LGN_ACK_CRC"
	case TypeDevLgnKey:
		return "DEV_LGN_KEY"
	case TypeDevLgnAckKey:
		return "DEV_LGN_ACK_KEY"
	case TypeDevLgnDsk:
		return "DEV_LGN_DSK"
	case TypeDevOnlineReq:
		return "DEV_ONLINE_REQ"
	case TypeDevOnlineReqAck:
		return "DEV_ONLINE_REQ_ACK"
	case TypeP2PReq:
		return "P2P_REQ"
	case TypeP2PReqAck:
		return "P2P_REQ_ACK"
	case TypeP2PReqDsk:
		return "P2P_REQ_DSK"
	case TypeLanSearch:
		return "LAN_SEARCH"
	case TypeLanNotify:
		return "LAN_NOTIFY"
	case TypeLanNotifyAck:
		return "LAN_NOTIFY_ACK"
	case TypePunchTo:
		return "PUNCH_TO"
	case TypePunchPkt:
		return "PUNCH_PKT"
	case TypeP2PRdy:
		return "P2P_RDY"
	case TypeP2PRdyAck:
		return "P2P_RDY_ACK"
	case TypeRsLgn:
		return "RS_LGN"
	case TypeRsLgnAck:
		return "RS_LGN_ACK"
	case TypeRsLgn1:
		return "RS_LGN1"
	case TypeRsLgn1Ack:
		return "RS_LGN1_ACK"
	case TypeListReq1:
		return "LIST_REQ1"
	case TypeListReq:
		return "LIST_REQ"
	case TypeListReqAck:
		return "LIST_REQ_ACK"
	case TypeListReqDsk:
		return "LIST_REQ_DSK"
	case TypeRlyHello:
		return "RLY_HELLO"
	case TypeRlyHelloAck:
		return "RLY_HELLO_ACK"
	case TypeRlyPort:
		return "RLY_PORT"
	case TypeRlyPortAck:
		return "RLY_PORT_ACK"
	case TypeRlyPortKey:
		return "RLY_PORT_KEY"
	case TypeRlyPortAckKey:
		return "RLY_PORT_ACK_KEY"
	case TypeRlyByteCount:
		return "RLY_BYTE_COUNT"
	case TypeRlyReq:
		return "RLY_REQ"
	case TypeRlyReqAck:
		return "RLY_REQ_ACK"
	case TypeRlyTo:
		return "RLY_TO"
	case TypeRlyPkt:
		return "RLY_PKT"
	case TypeRlyRdy:
		return "RLY_RDY"
	case TypeRlyToAck:
		return "RLY_TO_ACK"
	case TypeRlyServerReq:
		return "RLY_SERVER_REQ"
	case TypeSdevRun:
		return "SDEV_RUN"
	case TypeSdevLgn:
		return "SDEV_LGN"
	case TypeSdevLgnCrc:
		return "SDEV_LGN_CRC"
	case TypeSdevReport:
		return "SDEV_REPORT"
	case TypeConnectReport:
		return "CONNECT_REPORT"
	case TypeReportReq:
		return "REPORT_REQ"
	case TypeReport:
		return "REPORT"
	case TypeDrw:
		return "DRW"
	case TypeDrwAck:
		return "DRW_ACK"
	case TypePsr:
		return "PSR"
	case TypeAlive:
		return "ALIVE"
	case TypeAliveAck:
		return "ALIVE_ACK"
	case TypeClose:
		return "CLOSE"
	case TypeMgmDumpLoginDid:
		return "MGM_DUMP_LOGIN_DID"
	case TypeMgmDumpLoginDidDet:
		return "MGM_DUMP_LOGIN_DID_DETAIL"
	case TypeMgmDumpLoginDid1:
		return "MGM_DUMP_LOGIN_DID_1"
	case TypeMgmLogControl:
		return "MGM_LOG_CONTROL"
	case TypeMgmRemoteMgmt:
		return "MGM_REMOTE_MANAGEMENT"
	case TypeReportSessionReady:
		return "REPORT_SESSION_READY"
	case TypeInvalid:
		return "INVALID"
	default:
		return fmt.Sprintf("UNKNOWN(0x%02x)", uint8(t))
	}
}

// P2PCmdType is the P2P command type enum.
type P2PCmdType uint16

const (
	P2PCmdStartRecBroadcase     P2PCmdType = 0x0384
	P2PCmdStopRecBroadcase      P2PCmdType = 0x0385
	P2PCmdBindBroadcast         P2PCmdType = 0x03e8
	P2PCmdBindSyncAccountInfo   P2PCmdType = 0x03e9
	P2PCmdUnbindAccount         P2PCmdType = 0x03ea
	P2PCmdStartRealtimeMedia    P2PCmdType = 0x03eb
	P2PCmdStopRealtimeMedia     P2PCmdType = 0x03ec
	P2PCmdStartTalkback         P2PCmdType = 0x03ed
	P2PCmdStopTalkback          P2PCmdType = 0x03ee
	P2PCmdStartVoicecall        P2PCmdType = 0x03ef
	P2PCmdStopVoicecall         P2PCmdType = 0x03f0
	P2PCmdStartRecord           P2PCmdType = 0x03f1
	P2PCmdStopRecord            P2PCmdType = 0x03f2
	P2PCmdPirSwitch             P2PCmdType = 0x03f3
	P2PCmdClosePir              P2PCmdType = 0x03f4
	P2PCmdIrcutSwitch           P2PCmdType = 0x03f5
	P2PCmdCloseIrcut            P2PCmdType = 0x03f6
	P2PCmdEasSwitch             P2PCmdType = 0x03f7
	P2PCmdCloseEas              P2PCmdType = 0x03f8
	P2PCmdAuddecSwitch          P2PCmdType = 0x03f9
	P2PCmdCloseAuddec           P2PCmdType = 0x03fa
	P2PCmdDevsLockSwitch        P2PCmdType = 0x03fb
	P2PCmdDevsUnlock            P2PCmdType = 0x03fc
	P2PCmdRecordImg             P2PCmdType = 0x03fd
	P2PCmdRecordImgStop         P2PCmdType = 0x03fe
	P2PCmdStopShare             P2PCmdType = 0x03ff
	P2PCmdDownloadVideo         P2PCmdType = 0x0400
	P2PCmdRecordView            P2PCmdType = 0x0401
	P2PCmdRecordPlayCtrl        P2PCmdType = 0x0402
	P2PCmdDeleteRecord          P2PCmdType = 0x0403
	P2PCmdSnapshot              P2PCmdType = 0x0404
	P2PCmdFormatSd              P2PCmdType = 0x0405
	P2PCmdChangePwd             P2PCmdType = 0x0406
	P2PCmdChangeWifiPwd         P2PCmdType = 0x0407
	P2PCmdWifiConfig            P2PCmdType = 0x0408
	P2PCmdTimeSync              P2PCmdType = 0x0409
	P2PCmdHubReboot             P2PCmdType = 0x040a
	P2PCmdDevsSwitch            P2PCmdType = 0x040b
	P2PCmdHubToFactory          P2PCmdType = 0x040c
	P2PCmdDevsToFactory         P2PCmdType = 0x040d
	P2PCmdDevsBindBroadcase     P2PCmdType = 0x040e
	P2PCmdDevsBindNotify        P2PCmdType = 0x040f
	P2PCmdDevsUnbind            P2PCmdType = 0x0410
	P2PCmdRecorddateSearch      P2PCmdType = 0x0411
	P2PCmdRecordlistSearch      P2PCmdType = 0x0412
	P2PCmdGetUpgradeResult      P2PCmdType = 0x0413
	P2PCmdP2PDisconnect         P2PCmdType = 0x0414
	P2PCmdDevLedSwitch          P2PCmdType = 0x0415
	P2PCmdCloseDevLed           P2PCmdType = 0x0416
	P2PCmdCollectRecord         P2PCmdType = 0x0417
	P2PCmdDecollectRecord       P2PCmdType = 0x0418
	P2PCmdBatchRecord           P2PCmdType = 0x0419
	P2PCmdStressTestOper        P2PCmdType = 0x041a
	P2PCmdDownloadCancel        P2PCmdType = 0x041b
	P2PCmdBindSyncAccountInfoEx P2PCmdType = 0x041e
	P2PCmdLiveviewLedSwitch     P2PCmdType = 0x0420
	P2PCmdRepairSd              P2PCmdType = 0x0421
	P2PCmdGetAsekey             P2PCmdType = 0x044c
	P2PCmdGetBattery            P2PCmdType = 0x044d
	P2PCmdSdinfo                P2PCmdType = 0x044e
	P2PCmdCameraInfo            P2PCmdType = 0x044f
	P2PCmdGetRecordTime         P2PCmdType = 0x0450
	P2PCmdGetMdetectParam       P2PCmdType = 0x0451
	P2PCmdMdetectinfo           P2PCmdType = 0x0452
	P2PCmdGetArmingInfo         P2PCmdType = 0x0453
	P2PCmdGetArmingStatus       P2PCmdType = 0x0454
	P2PCmdGetAuddecInfo         P2PCmdType = 0x0455
	P2PCmdGetAuddecSensitivity  P2PCmdType = 0x0456
	P2PCmdGetAuddecStatus       P2PCmdType = 0x0457
	P2PCmdGetMirrormode         P2PCmdType = 0x0458
	P2PCmdGetIrmode             P2PCmdType = 0x0459
	P2PCmdGetIrcutsensitivity   P2PCmdType = 0x045a
	P2PCmdGetPirinfo            P2PCmdType = 0x045b
	P2PCmdGetPirctrl            P2PCmdType = 0x045c
	P2PCmdGetPirsensitivity     P2PCmdType = 0x045d
	P2PCmdGetEasStatus          P2PCmdType = 0x045e
	P2PCmdGetCameraLock         P2PCmdType = 0x045f
	P2PCmdGetGatewayLock        P2PCmdType = 0x0460
	P2PCmdGetUpdateStatus       P2PCmdType = 0x0461
	P2PCmdGetAdminPwd           P2PCmdType = 0x0462
	P2PCmdGetWifiPwd            P2PCmdType = 0x0463
	P2PCmdGetExceptionLog       P2PCmdType = 0x0464
	P2PCmdGetNewvesion          P2PCmdType = 0x0465
	P2PCmdGetHubToneInfo        P2PCmdType = 0x0466
	P2PCmdGetDevToneInfo        P2PCmdType = 0x0467
	P2PCmdGetHubName            P2PCmdType = 0x0468
	P2PCmdGetDevsName           P2PCmdType = 0x0469
	P2PCmdGetP2PConnStatus      P2PCmdType = 0x046a
	P2PCmdSetDevStorageType     P2PCmdType = 0x04cc
	P2PCmdVideoFrame            P2PCmdType = 0x0514
	P2PCmdAudioFrame            P2PCmdType = 0x0515
	P2PCmdStreamMsg             P2PCmdType = 0x0516
	P2PCmdConvertMp4Ok          P2PCmdType = 0x0517
	P2PCmdDownloadFinish        P2PCmdType = 0x0518
	P2PCmdSetPayload            P2PCmdType = 0x0546
	P2PCmdNotifyPayload         P2PCmdType = 0x0547
	P2PCmdMakerSetPayload       P2PCmdType = 0x06a4
	P2PCmdMakerNotifyPayload    P2PCmdType = 0x06a5
	P2PCmdFileRecv              P2PCmdType = 0x3a98
	P2PCmdP2PJsonCmd            P2PCmdType = 0x06a4
	P2PCmdP2PSendFile           P2PCmdType = 0x3a98
)

// P2PSubCmdType is the P2P sub-command type enum.
type P2PSubCmdType uint16

const (
	SubCmdStartLive         P2PSubCmdType = 0x03e8
	SubCmdCloseLive         P2PSubCmdType = 0x03e9
	SubCmdVideoRecordSwitch P2PSubCmdType = 0x03ea
	SubCmdLightStateSwitch  P2PSubCmdType = 0x03eb
	SubCmdLightStateGet     P2PSubCmdType = 0x03ec
	SubCmdLiveModeSet       P2PSubCmdType = 0x03ed
	SubCmdLiveModeGet       P2PSubCmdType = 0x03ee
)

// FileTransfer is the file transfer control enum.
type FileTransfer uint8

const (
	FTBegin FileTransfer = 0x00
	FTData  FileTransfer = 0x01
	FTEnd   FileTransfer = 0x02
	FTAbort FileTransfer = 0x03
	FTReply FileTransfer = 0x80
)

// FileTransferReply is the file transfer reply enum.
type FileTransferReply uint8

const (
	FTReplyOK           FileTransferReply = 0x00
	FTReplyErrTimeout   FileTransferReply = 0xfc
	FTReplyErrFrameType FileTransferReply = 0xfd
	FTReplyErrWrongMd5  FileTransferReply = 0xfe
	FTReplyErrBusy      FileTransferReply = 0xff
)

// ─── Sub-structures ──────────────────────────────────────────────────────────

// Host represents a network endpoint in the PPPP protocol.
type Host struct {
	Afam uint8  // Address family (AF_INET = 2)
	Port uint16 // Port number
	Addr string // IP address (dotted-decimal)
}

func ParseHost(p []byte) (Host, []byte, error) {
	var h Host
	var rest []byte
	var err error

	_, rest, err = types.ParseZeroes(p, 1)
	if err != nil {
		return h, nil, err
	}
	var u8 types.U8
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return h, nil, err
	}
	h.Afam = uint8(u8)
	port, rest, err := types.ParseU16LE(rest)
	if err != nil {
		return h, nil, err
	}
	h.Port = uint16(port)
	h.Addr, rest, err = types.ParseIPv4(rest)
	if err != nil {
		return h, nil, err
	}
	_, rest, err = types.ParseZeroes(rest, 8)
	if err != nil {
		return h, nil, err
	}
	return h, rest, nil
}

func (h Host) Pack() []byte {
	var p []byte
	p = append(p, types.PackZeroes(1)...)
	p = append(p, types.U8(h.Afam).Pack()...)
	p = append(p, types.U16LE(h.Port).Pack()...)
	addrBytes, err := types.PackIPv4(h.Addr)
	if err != nil {
		// This should not happen if the struct was populated correctly
		addrBytes = make([]byte, 4)
	}
	p = append(p, addrBytes...)
	p = append(p, types.PackZeroes(8)...)
	return p
}

// Duid represents a device unique identifier.
type Duid struct {
	Prefix string // 7 chars + null terminator (8 bytes)
	Serial uint32 // device serial number
	Check  string // checkcode (5 chars + null terminator, 6 bytes)
}

func ParseDuid(p []byte) (Duid, []byte, error) {
	var d Duid
	var rest []byte
	var err error

	d.Prefix, rest, err = types.ParseFixedString(p, 8)
	if err != nil {
		return d, nil, err
	}
	serial, rest, err := types.ParseU32BE(rest)
	if err != nil {
		return d, nil, err
	}
	d.Serial = uint32(serial)
	d.Check, rest, err = types.ParseFixedString(rest, 6)
	if err != nil {
		return d, nil, err
	}
	_, rest, err = types.ParseZeroes(rest, 2)
	if err != nil {
		return d, nil, err
	}
	return d, rest, nil
}

func (d Duid) Pack() []byte {
	var p []byte
	p = append(p, types.PackFixedString(d.Prefix, 8)...)
	p = append(p, types.U32BE(d.Serial).Pack()...)
	p = append(p, types.PackFixedString(d.Check, 6)...)
	p = append(p, types.PackZeroes(2)...)
	return p
}

func (d Duid) String() string {
	return fmt.Sprintf("%s-%06d-%s", d.Prefix, d.Serial, d.Check)
}

// DuidFromString parses a DUID string in the format "PREFIX-NNNNNN-CHECK".
func DuidFromString(s string) (Duid, error) {
	var d Duid
	parts := splitDuid(s)
	if len(parts) != 3 {
		return d, fmt.Errorf("invalid DUID format: %s", s)
	}
	d.Prefix = parts[0]
	if n, err := fmt.Sscanf(parts[1], "%d", &d.Serial); n != 1 || err != nil {
		return d, fmt.Errorf("invalid DUID serial: %s", parts[1])
	}
	d.Check = parts[2]
	return d, nil
}

func splitDuid(s string) []string {
	result := []string{}
	current := ""
	for _, c := range s {
		if c == '-' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" || len(result) == 2 {
		result = append(result, current)
	}
	return result
}

// Xzyh represents an XZYH command packet (used for P2P commands).
type Xzyh struct {
	Cmd      P2PCmdType
	Len      uint32
	Unk0     uint8
	Unk1     uint8
	Chan     uint8
	SignCode uint8
	Unk3     uint8
	DevType  uint8
	Data     []byte
}

func ParseXzyh(p []byte) (Xzyh, []byte, error) {
	var x Xzyh
	var rest []byte
	var err error

	_, rest, err = types.ParseMagic(p, []byte("XZYH"))
	if err != nil {
		return x, nil, err
	}
	cmd, rest, err := types.ParseU16LE(rest)
	if err != nil {
		return x, nil, err
	}
	x.Cmd = P2PCmdType(cmd)
	ln, rest, err := types.ParseU32LE(rest)
	if err != nil {
		return x, nil, err
	}
	x.Len = uint32(ln)
	var u8 types.U8
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return x, nil, err
	}
	x.Unk0 = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return x, nil, err
	}
	x.Unk1 = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return x, nil, err
	}
	x.Chan = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return x, nil, err
	}
	x.SignCode = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return x, nil, err
	}
	x.Unk3 = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return x, nil, err
	}
	x.DevType = uint8(u8)
	x.Data, rest, err = types.ParseFixedBytes(rest, int(x.Len))
	if err != nil {
		return x, nil, err
	}
	return x, rest, nil
}

func (x Xzyh) Pack() []byte {
	var p []byte
	p = append(p, types.PackMagic([]byte("XZYH"))...)
	p = append(p, types.U16LE(x.Cmd).Pack()...)
	p = append(p, types.U32LE(x.Len).Pack()...)
	p = append(p, types.U8(x.Unk0).Pack()...)
	p = append(p, types.U8(x.Unk1).Pack()...)
	p = append(p, types.U8(x.Chan).Pack()...)
	p = append(p, types.U8(x.SignCode).Pack()...)
	p = append(p, types.U8(x.Unk3).Pack()...)
	p = append(p, types.U8(x.DevType).Pack()...)
	p = append(p, x.Data...)
	return p
}

// Aabb represents an AABB file transfer header.
type Aabb struct {
	Signature []byte // Must be 0xAABB
	FrameType FileTransfer
	SN        uint8
	Pos       uint32
	Len       uint32
}

func ParseAabb(p []byte) (Aabb, []byte, error) {
	var a Aabb
	var rest []byte
	var err error

	a.Signature, rest, err = types.ParseMagic(p, []byte{0xaa, 0xbb})
	if err != nil {
		return a, nil, err
	}
	ft, rest, err := types.ParseU8(rest)
	if err != nil {
		return a, nil, err
	}
	a.FrameType = FileTransfer(ft)
	var sn types.U8
	sn, rest, err = types.ParseU8(rest)
	if err != nil {
		return a, nil, err
	}
	a.SN = uint8(sn)
	pos, rest, err := types.ParseU32LE(rest)
	if err != nil {
		return a, nil, err
	}
	ln, rest, err := types.ParseU32LE(rest)
	if err != nil {
		return a, nil, err
	}
	a.Pos = uint32(pos)
	a.Len = uint32(ln)
	return a, rest, nil
}

func (a Aabb) Pack() []byte {
	var p []byte
	p = append(p, types.PackMagic([]byte{0xaa, 0xbb})...)
	p = append(p, types.U8(a.FrameType).Pack()...)
	p = append(p, types.U8(a.SN).Pack()...)
	p = append(p, types.U32LE(a.Pos).Pack()...)
	p = append(p, types.U32LE(a.Len).Pack()...)
	return p
}

// ParseAabbWithCRC parses an AABB header with CRC-16 verification.
func ParseAabbWithCRC(m []byte) (Aabb, []byte, []byte, error) {
	if len(m) < 14 {
		return Aabb{}, nil, nil, fmt.Errorf("not enough data for AABB with CRC: %d bytes", len(m))
	}
	head := m[:12]
	header, _, err := ParseAabb(head)
	if err != nil {
		return Aabb{}, nil, nil, err
	}
	dataLen := int(header.Len)
	if len(m) < 12+dataLen+2 {
		return Aabb{}, nil, nil, fmt.Errorf("not enough data for AABB payload + CRC: need %d, have %d", 12+dataLen+2, len(m))
	}
	data := m[12 : 12+dataLen]
	crc1 := m[12+dataLen : 12+dataLen+2]
	crc2 := types.PpcsCRC16(append(append([]byte{}, head[2:]...), data...))
	if string(crc1) != string(crc2) {
		return Aabb{}, nil, nil, fmt.Errorf("CRC mismatch: expected %x, found %x", crc2, crc1)
	}
	rest := m[12+dataLen+2:]
	return header, data, rest, nil
}

// PackAabbWithCRC packs an AABB header with data and CRC-16.
func PackAabbWithCRC(a Aabb, data []byte) []byte {
	header := a.Pack()
	crc := types.PpcsCRC16(append(append([]byte{}, header[2:]...), data...))
	return append(append(header, data...), crc...)
}

// Dsk represents a device secret key.
type Dsk struct {
	Key []byte // 20 bytes
}

func ParseDsk(p []byte) (Dsk, []byte, error) {
	var d Dsk
	var rest []byte
	var err error

	d.Key, rest, err = types.ParseFixedBytes(p, 20)
	if err != nil {
		return d, nil, err
	}
	_, rest, err = types.ParseZeroes(rest, 4)
	if err != nil {
		return d, nil, err
	}
	return d, rest, nil
}

func (d Dsk) Pack() []byte {
	var p []byte
	p = append(p, d.Key...)
	p = append(p, types.PackZeroes(4)...)
	return p
}

// Version represents a 3-component version number.
type Version struct {
	Major uint8
	Minor uint8
	Patch uint8
}

func ParseVersion(p []byte) (Version, []byte, error) {
	var v Version
	var rest []byte
	var err error

	var u8 types.U8
	u8, rest, err = types.ParseU8(p)
	if err != nil {
		return v, nil, err
	}
	v.Major = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return v, nil, err
	}
	v.Minor = uint8(u8)
	u8, rest, err = types.ParseU8(rest)
	if err != nil {
		return v, nil, err
	}
	v.Patch = uint8(u8)
	return v, rest, nil
}

func (v Version) Pack() []byte {
	return []byte{v.Major, v.Minor, v.Patch}
}

// ─── Message Header ──────────────────────────────────────────────────────────

const MessageMagic byte = 0xF1

// MessageHeader is the 4-byte header on every PPPP message.
type MessageHeader struct {
	Magic byte
	Type  Type
	Size  uint16
}

// ParseMessageHeader parses the 4-byte PPPP message header.
func ParseMessageHeader(m []byte) (MessageHeader, error) {
	if len(m) < 4 {
		return MessageHeader{}, fmt.Errorf("not enough data for message header: %d bytes", len(m))
	}
	magic := m[0]
	if magic != MessageMagic {
		return MessageHeader{}, fmt.Errorf("invalid message magic: expected 0x%02x, found 0x%02x", MessageMagic, magic)
	}
	msgType := Type(m[1])
	size := binary.BigEndian.Uint16(m[2:4])
	return MessageHeader{Magic: magic, Type: msgType, Size: size}, nil
}

// PackMessageHeader packs a 4-byte PPPP message header.
func PackMessageHeader(msgType Type, payloadLen uint16) []byte {
	return []byte{
		MessageMagic,
		byte(msgType),
		byte(payloadLen >> 8),
		byte(payloadLen),
	}
}

// ─── Packet Types ────────────────────────────────────────────────────────────

// PktHello is an empty HELLO message.
type PktHello struct{}

func (p PktHello) MsgType() Type       { return TypeHello }
func (p PktHello) PackPayload() []byte { return nil }

// PktHelloAck is a HELLO_ACK message containing a host.
type PktHelloAck struct {
	Host Host
}

func (p PktHelloAck) MsgType() Type       { return TypeHelloAck }
func (p PktHelloAck) PackPayload() []byte { return p.Host.Pack() }

// PktLanSearch is an empty LAN_SEARCH message.
type PktLanSearch struct{}

func (p PktLanSearch) MsgType() Type       { return TypeLanSearch }
func (p PktLanSearch) PackPayload() []byte { return nil }

// PktPunchTo is a PUNCH_TO message containing a host.
type PktPunchTo struct {
	Host Host
}

func (p PktPunchTo) MsgType() Type       { return TypePunchTo }
func (p PktPunchTo) PackPayload() []byte { return p.Host.Pack() }

// PktPunchPkt is a PUNCH_PKT message containing a DUID.
type PktPunchPkt struct {
	Duid Duid
}

func (p PktPunchPkt) MsgType() Type       { return TypePunchPkt }
func (p PktPunchPkt) PackPayload() []byte { return p.Duid.Pack() }

// PktP2PReq is a P2P_REQ message.
type PktP2PReq struct {
	Duid Duid
	Host Host
}

func (p PktP2PReq) MsgType() Type { return TypeP2PReq }
func (p PktP2PReq) PackPayload() []byte {
	return append(p.Duid.Pack(), p.Host.Pack()...)
}

// PktP2PReqAck is a P2P_REQ_ACK message.
type PktP2PReqAck struct {
	Mark uint32
}

func (p PktP2PReqAck) MsgType() Type       { return TypeP2PReqAck }
func (p PktP2PReqAck) PackPayload() []byte { return types.U32BE(p.Mark).Pack() }

// PktP2PReqDsk is a P2P_REQ_DSK message.
type PktP2PReqDsk struct {
	Duid    Duid
	Host    Host
	NatType uint8
	Version Version
	Dsk     Dsk
}

func (p PktP2PReqDsk) MsgType() Type { return TypeP2PReqDsk }
func (p PktP2PReqDsk) PackPayload() []byte {
	var b []byte
	b = append(b, p.Duid.Pack()...)
	b = append(b, p.Host.Pack()...)
	b = append(b, types.U8(p.NatType).Pack()...)
	b = append(b, p.Version.Pack()...)
	b = append(b, p.Dsk.Pack()...)
	return b
}

// PktP2PRdy is a P2P_RDY message.
type PktP2PRdy struct {
	Duid Duid
}

func (p PktP2PRdy) MsgType() Type       { return TypeP2PRdy }
func (p PktP2PRdy) PackPayload() []byte { return p.Duid.Pack() }

// PktP2PRdyAck is a P2P_RDY_ACK message.
type PktP2PRdyAck struct {
	Duid Duid
	Host Host
}

func (p PktP2PRdyAck) MsgType() Type { return TypeP2PRdyAck }
func (p PktP2PRdyAck) PackPayload() []byte {
	var b []byte
	b = append(b, p.Duid.Pack()...)
	b = append(b, p.Host.Pack()...)
	b = append(b, types.PackZeroes(8)...)
	return b
}

// PktAlive is an empty ALIVE message.
type PktAlive struct{}

func (p PktAlive) MsgType() Type       { return TypeAlive }
func (p PktAlive) PackPayload() []byte { return nil }

// PktAliveAck is an empty ALIVE_ACK message.
type PktAliveAck struct{}

func (p PktAliveAck) MsgType() Type       { return TypeAliveAck }
func (p PktAliveAck) PackPayload() []byte { return nil }

// PktClose is an empty CLOSE message.
type PktClose struct{}

func (p PktClose) MsgType() Type       { return TypeClose }
func (p PktClose) PackPayload() []byte { return nil }

// PktDrw is a DRW (data read/write) message.
type PktDrw struct {
	Chan  uint8
	Index uint16
	Data  []byte
}

func (p PktDrw) MsgType() Type { return TypeDrw }
func (p PktDrw) PackPayload() []byte {
	var b []byte
	b = append(b, types.PackMagic([]byte{0xd1})...)
	b = append(b, types.U8(p.Chan).Pack()...)
	b = append(b, types.U16BE(p.Index).Pack()...)
	b = append(b, p.Data...)
	return b
}

// PktDrwAck is a DRW_ACK message.
type PktDrwAck struct {
	Chan  uint8
	Count uint16
	Acks  []uint16
}

func (p PktDrwAck) MsgType() Type { return TypeDrwAck }
func (p PktDrwAck) PackPayload() []byte {
	var b []byte
	b = append(b, types.PackMagic([]byte{0xd1})...)
	b = append(b, types.U8(p.Chan).Pack()...)
	b = append(b, types.U16BE(p.Count).Pack()...)
	for _, ack := range p.Acks {
		b = append(b, types.U16BE(ack).Pack()...)
	}
	return b
}

// PktRlyHello is an empty RLY_HELLO message.
type PktRlyHello struct{}

func (p PktRlyHello) MsgType() Type       { return TypeRlyHello }
func (p PktRlyHello) PackPayload() []byte { return nil }

// PktRlyHelloAck is an empty RLY_HELLO_ACK message.
type PktRlyHelloAck struct{}

func (p PktRlyHelloAck) MsgType() Type       { return TypeRlyHelloAck }
func (p PktRlyHelloAck) PackPayload() []byte { return nil }

// PktRlyPort is an empty RLY_PORT message.
type PktRlyPort struct{}

func (p PktRlyPort) MsgType() Type       { return TypeRlyPort }
func (p PktRlyPort) PackPayload() []byte { return nil }

// PktRlyPortAck is a RLY_PORT_ACK message.
type PktRlyPortAck struct {
	Mark uint32
	Port uint16
}

func (p PktRlyPortAck) MsgType() Type { return TypeRlyPortAck }
func (p PktRlyPortAck) PackPayload() []byte {
	var b []byte
	b = append(b, types.U32BE(p.Mark).Pack()...)
	b = append(b, types.U16BE(p.Port).Pack()...)
	b = append(b, types.PackZeroes(2)...)
	return b
}

// PktRlyReq is a RLY_REQ message.
type PktRlyReq struct {
	Duid Duid
	Host Host
	Mark uint32
}

func (p PktRlyReq) MsgType() Type { return TypeRlyReq }
func (p PktRlyReq) PackPayload() []byte {
	var b []byte
	b = append(b, p.Duid.Pack()...)
	b = append(b, p.Host.Pack()...)
	b = append(b, types.U32BE(p.Mark).Pack()...)
	return b
}

// PktRlyReqAck is a RLY_REQ_ACK message.
type PktRlyReqAck struct {
	Mark uint32
}

func (p PktRlyReqAck) MsgType() Type       { return TypeRlyReqAck }
func (p PktRlyReqAck) PackPayload() []byte { return types.U32BE(p.Mark).Pack() }

// PktListReqDsk is a LIST_REQ_DSK message.
type PktListReqDsk struct {
	Duid Duid
	Dsk  Dsk
}

func (p PktListReqDsk) MsgType() Type { return TypeListReqDsk }
func (p PktListReqDsk) PackPayload() []byte {
	return append(p.Duid.Pack(), p.Dsk.Pack()...)
}

// PktListReqAck is a LIST_REQ_ACK message.
type PktListReqAck struct {
	Numr   uint8
	Relays []Host
}

func (p PktListReqAck) MsgType() Type { return TypeListReqAck }
func (p PktListReqAck) PackPayload() []byte {
	var b []byte
	b = append(b, types.U8(p.Numr).Pack()...)
	b = append(b, types.PackZeroes(3)...)
	for _, h := range p.Relays {
		b = append(b, h.Pack()...)
	}
	return b
}

// PktDevLgnCrc is a DEV_LGN_CRC message (encrypted with curse cipher).
type PktDevLgnCrc struct {
	Duid    Duid
	NatType uint8
	Version Version
	Host    Host
}

func (p PktDevLgnCrc) MsgType() Type { return TypeDevLgnCrc }
func (p PktDevLgnCrc) PackPayload() []byte {
	var b []byte
	b = append(b, p.Duid.Pack()...)
	b = append(b, types.U8(p.NatType).Pack()...)
	b = append(b, p.Version.Pack()...)
	b = append(b, p.Host.Pack()...)
	return b
}

// ─── Message Interface ───────────────────────────────────────────────────────

// Message is the interface implemented by all PPPP packet types.
type Message interface {
	MsgType() Type
	PackPayload() []byte
}

// PackMessage packs a complete PPPP message (header + payload).
func PackMessage(msg Message) []byte {
	payload := msg.PackPayload()
	header := PackMessageHeader(msg.MsgType(), uint16(len(payload)))
	return append(header, payload...)
}

// ParseMessage parses a complete PPPP message from raw bytes.
// Returns the parsed Message and any remaining bytes.
func ParseMessage(m []byte) (Message, []byte, error) {
	hdr, err := ParseMessageHeader(m)
	if err != nil {
		return nil, nil, err
	}
	totalLen := 4 + int(hdr.Size)
	if len(m) < totalLen {
		return nil, nil, fmt.Errorf("message truncated: need %d bytes, have %d", totalLen, len(m))
	}
	payload := m[4:totalLen]
	rest := m[totalLen:]

	switch hdr.Type {
	case TypeHello:
		return PktHello{}, rest, nil
	case TypeHelloAck:
		host, _, err := ParseHost(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("HELLO_ACK: %w", err)
		}
		return PktHelloAck{Host: host}, rest, nil
	case TypeLanSearch:
		return PktLanSearch{}, rest, nil
	case TypePunchTo:
		host, _, err := ParseHost(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("PUNCH_TO: %w", err)
		}
		return PktPunchTo{Host: host}, rest, nil
	case TypePunchPkt:
		duid, _, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("PUNCH_PKT: %w", err)
		}
		return PktPunchPkt{Duid: duid}, rest, nil
	case TypeP2PReq:
		duid, p, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ: %w", err)
		}
		host, _, err := ParseHost(p)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ: %w", err)
		}
		return PktP2PReq{Duid: duid, Host: host}, rest, nil
	case TypeP2PReqAck:
		mark, _, err := types.ParseU32BE(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ_ACK: %w", err)
		}
		return PktP2PReqAck{Mark: uint32(mark)}, rest, nil
	case TypeP2PReqDsk:
		duid, p, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ_DSK: %w", err)
		}
		host, p, err := ParseHost(p)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ_DSK: %w", err)
		}
		natType, p, err := types.ParseU8(p)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ_DSK: %w", err)
		}
		ver, p, err := ParseVersion(p)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ_DSK: %w", err)
		}
		dsk, _, err := ParseDsk(p)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_REQ_DSK: %w", err)
		}
		return PktP2PReqDsk{Duid: duid, Host: host, NatType: uint8(natType), Version: ver, Dsk: dsk}, rest, nil
	case TypeP2PRdy:
		duid, _, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_RDY: %w", err)
		}
		return PktP2PRdy{Duid: duid}, rest, nil
	case TypeP2PRdyAck:
		duid, p, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_RDY_ACK: %w", err)
		}
		host, _, err := ParseHost(p)
		if err != nil {
			return nil, nil, fmt.Errorf("P2P_RDY_ACK: %w", err)
		}
		return PktP2PRdyAck{Duid: duid, Host: host}, rest, nil
	case TypeAlive:
		return PktAlive{}, rest, nil
	case TypeAliveAck:
		return PktAliveAck{}, rest, nil
	case TypeClose:
		return PktClose{}, rest, nil
	case TypeDrw:
		_, p, err := types.ParseMagic(payload, []byte{0xd1})
		if err != nil {
			return nil, nil, fmt.Errorf("DRW: %w", err)
		}
		chan_, p, err := types.ParseU8(p)
		if err != nil {
			return nil, nil, fmt.Errorf("DRW: %w", err)
		}
		idx, p, err := types.ParseU16BE(p)
		if err != nil {
			return nil, nil, fmt.Errorf("DRW: %w", err)
		}
		data, _, err := types.ParseTail(p)
		if err != nil {
			return nil, nil, fmt.Errorf("DRW: %w", err)
		}
		return PktDrw{Chan: uint8(chan_), Index: uint16(idx), Data: data}, rest, nil
	case TypeDrwAck:
		_, p, err := types.ParseMagic(payload, []byte{0xd1})
		if err != nil {
			return nil, nil, fmt.Errorf("DRW_ACK: %w", err)
		}
		chan_, p, err := types.ParseU8(p)
		if err != nil {
			return nil, nil, fmt.Errorf("DRW_ACK: %w", err)
		}
		count, p, err := types.ParseU16BE(p)
		if err != nil {
			return nil, nil, fmt.Errorf("DRW_ACK: %w", err)
		}
		acks, _, err := types.ParseArray(p, int(count), func(b []byte) (types.U16BE, []byte, error) {
			return types.ParseU16BE(b)
		})
		if err != nil {
			return nil, nil, fmt.Errorf("DRW_ACK: %w", err)
		}
		ackUints := make([]uint16, len(acks))
		for i, a := range acks {
			ackUints[i] = uint16(a)
		}
		return PktDrwAck{Chan: uint8(chan_), Count: uint16(count), Acks: ackUints}, rest, nil
	case TypeRlyHello:
		return PktRlyHello{}, rest, nil
	case TypeRlyHelloAck:
		return PktRlyHelloAck{}, rest, nil
	case TypeRlyPort:
		return PktRlyPort{}, rest, nil
	case TypeRlyPortAck:
		mark, p, err := types.ParseU32BE(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("RLY_PORT_ACK: %w", err)
		}
		port, _, err := types.ParseU16BE(p)
		if err != nil {
			return nil, nil, fmt.Errorf("RLY_PORT_ACK: %w", err)
		}
		return PktRlyPortAck{Mark: uint32(mark), Port: uint16(port)}, rest, nil
	case TypeRlyReq:
		duid, p, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("RLY_REQ: %w", err)
		}
		host, p, err := ParseHost(p)
		if err != nil {
			return nil, nil, fmt.Errorf("RLY_REQ: %w", err)
		}
		mark, _, err := types.ParseU32BE(p)
		if err != nil {
			return nil, nil, fmt.Errorf("RLY_REQ: %w", err)
		}
		return PktRlyReq{Duid: duid, Host: host, Mark: uint32(mark)}, rest, nil
	case TypeRlyReqAck:
		mark, _, err := types.ParseU32BE(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("RLY_REQ_ACK: %w", err)
		}
		return PktRlyReqAck{Mark: uint32(mark)}, rest, nil
	case TypeListReqDsk:
		duid, p, err := ParseDuid(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("LIST_REQ_DSK: %w", err)
		}
		dsk, _, err := ParseDsk(p)
		if err != nil {
			return nil, nil, fmt.Errorf("LIST_REQ_DSK: %w", err)
		}
		return PktListReqDsk{Duid: duid, Dsk: dsk}, rest, nil
	case TypeListReqAck:
		numr, p, err := types.ParseU8(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("LIST_REQ_ACK: %w", err)
		}
		_, p, err = types.ParseZeroes(p, 3)
		if err != nil {
			return nil, nil, fmt.Errorf("LIST_REQ_ACK: %w", err)
		}
		relays, _, err := types.ParseArray(p, int(numr), ParseHost)
		if err != nil {
			return nil, nil, fmt.Errorf("LIST_REQ_ACK: %w", err)
		}
		return PktListReqAck{Numr: uint8(numr), Relays: relays}, rest, nil
	default:
		return nil, nil, fmt.Errorf("unknown message type 0x%02x", hdr.Type)
	}
}
