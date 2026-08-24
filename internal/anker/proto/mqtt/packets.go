// Package mqtt implements the MQTT message format used by AnkerMake M5 printers.
//
// This file defines the MQTT packet types, enums, and message parsing/packing logic.
// It is the Go equivalent of libflagship/mqtt.py from the Python implementation.
package mqtt

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/lucas/openpolyprint/internal/anker/proto/types"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

// MqttPktType is the MQTT packet type enum.
type MqttPktType uint8

const (
	PktTypeSingle      MqttPktType = 0xc0
	PktTypeMultiBegin  MqttPktType = 0xc1
	PktTypeMultiAppend MqttPktType = 0xc2
	PktTypeMultiFinish MqttPktType = 0xc3
)

func (t MqttPktType) String() string {
	switch t {
	case PktTypeSingle:
		return "Single"
	case PktTypeMultiBegin:
		return "MultiBegin"
	case PktTypeMultiAppend:
		return "MultiAppend"
	case PktTypeMultiFinish:
		return "MultiFinish"
	default:
		return fmt.Sprintf("Unknown(0x%02x)", uint8(t))
	}
}

// MqttMsgType is the MQTT command type enum.
type MqttMsgType uint16

const (
	CmdEventNotify         MqttMsgType = 0x03e8
	CmdPrintSchedule       MqttMsgType = 0x03e9
	CmdFirmwareVersion     MqttMsgType = 0x03ea
	CmdNozzleTemp          MqttMsgType = 0x03eb
	CmdHotbedTemp          MqttMsgType = 0x03ec
	CmdFanSpeed            MqttMsgType = 0x03ed
	CmdPrintSpeed          MqttMsgType = 0x03ee
	CmdAutoLeveling        MqttMsgType = 0x03ef
	CmdPrintControl        MqttMsgType = 0x03f0
	CmdFileListRequest     MqttMsgType = 0x03f1
	CmdGcodeFileRequest    MqttMsgType = 0x03f2
	CmdAllowFirmwareUpdate MqttMsgType = 0x03f3
	CmdGcodeFileDownload   MqttMsgType = 0x03fc
	CmdZAxisRecoup         MqttMsgType = 0x03fd
	CmdExtrusionStep       MqttMsgType = 0x03fe
	CmdEnterOrQuitMateriel MqttMsgType = 0x03ff
	CmdMoveStep            MqttMsgType = 0x0400
	CmdMoveDirection       MqttMsgType = 0x0401
	CmdMoveZero            MqttMsgType = 0x0402
	CmdAppQueryStatus      MqttMsgType = 0x0403
	CmdOnlineNotify        MqttMsgType = 0x0404
	CmdRecoverFactory      MqttMsgType = 0x0405
	CmdBleOnoff            MqttMsgType = 0x0407
	CmdDeleteGcodeFile     MqttMsgType = 0x0408
	CmdResetGcodeParam     MqttMsgType = 0x0409
	CmdDeviceNameSet       MqttMsgType = 0x040a
	CmdDeviceLogUpload     MqttMsgType = 0x040b
	CmdOnoffModal          MqttMsgType = 0x040c
	CmdMotorLock           MqttMsgType = 0x040d
	CmdPreheatConfig       MqttMsgType = 0x040e
	CmdBreakPoint          MqttMsgType = 0x040f
	CmdAiCalib             MqttMsgType = 0x0410
	CmdVideoOnoff          MqttMsgType = 0x0411
	CmdAdvancedParameters  MqttMsgType = 0x0412
	CmdGcodeCommand        MqttMsgType = 0x0413
	CmdPreviewImageUrl     MqttMsgType = 0x0414
	CmdSystemCheck         MqttMsgType = 0x0419
	CmdAiSwitch            MqttMsgType = 0x041a
	CmdAiInfoCheck         MqttMsgType = 0x041b
	CmdModelLayer          MqttMsgType = 0x041c
	CmdModelDlProcess      MqttMsgType = 0x041d
	CmdPrintMaxSpeed       MqttMsgType = 0x041f
	CmdAlexaMsg            MqttMsgType = 0x0bb8
)

func (t MqttMsgType) String() string {
	switch t {
	case CmdEventNotify:
		return "EVENT_NOTIFY"
	case CmdPrintSchedule:
		return "PRINT_SCHEDULE"
	case CmdFirmwareVersion:
		return "FIRMWARE_VERSION"
	case CmdNozzleTemp:
		return "NOZZLE_TEMP"
	case CmdHotbedTemp:
		return "HOTBED_TEMP"
	case CmdFanSpeed:
		return "FAN_SPEED"
	case CmdPrintSpeed:
		return "PRINT_SPEED"
	case CmdAutoLeveling:
		return "AUTO_LEVELING"
	case CmdPrintControl:
		return "PRINT_CONTROL"
	case CmdFileListRequest:
		return "FILE_LIST_REQUEST"
	case CmdGcodeFileRequest:
		return "GCODE_FILE_REQUEST"
	case CmdAllowFirmwareUpdate:
		return "ALLOW_FIRMWARE_UPDATE"
	case CmdGcodeFileDownload:
		return "GCODE_FILE_DOWNLOAD"
	case CmdZAxisRecoup:
		return "Z_AXIS_RECOUP"
	case CmdExtrusionStep:
		return "EXTRUSION_STEP"
	case CmdEnterOrQuitMateriel:
		return "ENTER_OR_QUIT_MATERIEL"
	case CmdMoveStep:
		return "MOVE_STEP"
	case CmdMoveDirection:
		return "MOVE_DIRECTION"
	case CmdMoveZero:
		return "MOVE_ZERO"
	case CmdAppQueryStatus:
		return "APP_QUERY_STATUS"
	case CmdOnlineNotify:
		return "ONLINE_NOTIFY"
	case CmdRecoverFactory:
		return "RECOVER_FACTORY"
	case CmdBleOnoff:
		return "BLE_ONOFF"
	case CmdDeleteGcodeFile:
		return "DELETE_GCODE_FILE"
	case CmdResetGcodeParam:
		return "RESET_GCODE_PARAM"
	case CmdDeviceNameSet:
		return "DEVICE_NAME_SET"
	case CmdDeviceLogUpload:
		return "DEVICE_LOG_UPLOAD"
	case CmdOnoffModal:
		return "ONOFF_MODAL"
	case CmdMotorLock:
		return "MOTOR_LOCK"
	case CmdPreheatConfig:
		return "PREHEAT_CONFIG"
	case CmdBreakPoint:
		return "BREAK_POINT"
	case CmdAiCalib:
		return "AI_CALIB"
	case CmdVideoOnoff:
		return "VIDEO_ONOFF"
	case CmdAdvancedParameters:
		return "ADVANCED_PARAMETERS"
	case CmdGcodeCommand:
		return "GCODE_COMMAND"
	case CmdPreviewImageUrl:
		return "PREVIEW_IMAGE_URL"
	case CmdSystemCheck:
		return "SYSTEM_CHECK"
	case CmdAiSwitch:
		return "AI_SWITCH"
	case CmdAiInfoCheck:
		return "AI_INFO_CHECK"
	case CmdModelLayer:
		return "MODEL_LAYER"
	case CmdModelDlProcess:
		return "MODEL_DL_PROCESS"
	case CmdPrintMaxSpeed:
		return "PRINT_MAX_SPEED"
	case CmdAlexaMsg:
		return "ALEXA_MSG"
	default:
		return fmt.Sprintf("Unknown(0x%04x)", uint16(t))
	}
}

// ─── MqttMsg ─────────────────────────────────────────────────────────────────

// MqttMsg represents a parsed MQTT message from/to the AnkerMake M5 printer.
type MqttMsg struct {
	Signature  []byte // Magic: 'MA' (2 bytes)
	Size       uint16 // Length of packet including header and checksum (min 65)
	M3         uint8  // Magic constant: 5
	M4         uint8  // Magic constant: 1
	M5         uint8  // Magic constant: 2
	M6         uint8  // Magic constant: 5
	M7         uint8  // Magic constant: 'F' (0x46)
	PacketType MqttPktType
	PacketNum  uint16
	Time       uint32
	DeviceGuid string // 36 chars + null (37 bytes)
	Padding    []byte // 11 bytes
	Data       []byte // Encrypted payload
}

// HeaderSize is the fixed size of the MQTT message header (excluding variable-length data).
const HeaderSize = 64

// Magic constants for MQTT messages
var (
	mqttSignature = []byte{'M', 'A'}
	mqttM3        = types.U8(5)
	mqttM4        = types.U8(1)
	mqttM5        = types.U8(2)
	mqttM6        = types.U8(5)
	mqttM7        = types.U8(0x46) // 'F'
)

// ParseMqttMsg parses a raw (already decrypted) MQTT message body.
// This is the equivalent of _MqttMsg.parse() in Python.
func ParseMqttMsg(p []byte) (MqttMsg, []byte, error) {
	var msg MqttMsg
	var rest []byte
	var err error

	msg.Signature, rest, err = types.ParseMagic(p, mqttSignature)
	if err != nil {
		return msg, nil, fmt.Errorf("mqtt signature: %w", err)
	}
	size, rest, err := types.ParseU16LE(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.Size = uint16(size)
	var m types.U8
	m, rest, err = types.ParseU8(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.M3 = uint8(m)
	m, rest, err = types.ParseU8(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.M4 = uint8(m)
	m, rest, err = types.ParseU8(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.M5 = uint8(m)
	m, rest, err = types.ParseU8(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.M6 = uint8(m)
	m, rest, err = types.ParseU8(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.M7 = uint8(m)
	var pt types.U8
	pt, rest, err = types.ParseU8(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.PacketType = MqttPktType(pt)
	pn, rest, err := types.ParseU16LE(rest)
	if err != nil {
		return msg, nil, err
	}
	msg.PacketNum = uint16(pn)
	if msg.M5 == 1 {
		// M5C: 12 bytes padding, no timestamp, no GUID
		msg.Padding, rest, err = types.ParseFixedBytes(rest, 12)
		if err != nil {
			return msg, nil, err
		}
	} else {
		// M5: timestamp + 37-byte GUID string + 11 bytes padding
		var t types.U32LE
		t, rest, err = types.ParseU32LE(rest)
		if err != nil {
			return msg, nil, err
		}
		msg.Time = uint32(t)
		msg.DeviceGuid, rest, err = types.ParseFixedString(rest, 37)
		if err != nil {
			return msg, nil, err
		}
		msg.Padding, rest, err = types.ParseFixedBytes(rest, 11)
		if err != nil {
			return msg, nil, err
		}
	}
	msg.Data, rest, err = types.ParseTail(rest)
	if err != nil {
		return msg, nil, err
	}
	return msg, rest, nil
}

// Pack packs the MqttMsg header (first 64 bytes) + data into raw bytes.
// This is the equivalent of _MqttMsg.pack() in Python.
func (msg MqttMsg) Pack() []byte {
	var p []byte
	p = append(p, types.PackMagic(mqttSignature)...)
	p = append(p, types.U16LE(msg.Size).Pack()...)
	p = append(p, types.U8(msg.M3).Pack()...)
	p = append(p, types.U8(msg.M4).Pack()...)
	p = append(p, types.U8(msg.M5).Pack()...)
	p = append(p, types.U8(msg.M6).Pack()...)
	p = append(p, types.U8(msg.M7).Pack()...)
	p = append(p, types.U8(msg.PacketType).Pack()...)
	p = append(p, types.U16LE(msg.PacketNum).Pack()...)
	p = append(p, types.U32LE(msg.Time).Pack()...)
	p = append(p, types.PackFixedString(msg.DeviceGuid, 37)...)
	p = append(p, msg.Padding...)
	p = append(p, msg.Data...)
	return p
}

// PackHeader packs only the first 64 bytes of the MQTT message (header without data).
func (msg MqttMsg) PackHeader() []byte {
	var p []byte
	p = append(p, types.PackMagic(mqttSignature)...)
	p = append(p, types.U16LE(msg.Size).Pack()...)
	p = append(p, types.U8(msg.M3).Pack()...)
	p = append(p, types.U8(msg.M4).Pack()...)
	p = append(p, types.U8(msg.M5).Pack()...)
	p = append(p, types.U8(msg.M6).Pack()...)
	p = append(p, types.U8(msg.M7).Pack()...)
	p = append(p, types.U8(msg.PacketType).Pack()...)
	p = append(p, types.U16LE(msg.PacketNum).Pack()...)
	p = append(p, types.U32LE(msg.Time).Pack()...)
	p = append(p, types.PackFixedString(msg.DeviceGuid, 37)...)
	pad := make([]byte, 11)
	if len(msg.Padding) == 11 {
		copy(pad, msg.Padding)
	}
	p = append(p, pad...)
	return p
}

// GetJSON deserializes the Data field as JSON.
func (msg MqttMsg) GetJSON() (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("mqtt data JSON parse: %w", err)
	}
	return result, nil
}

// SetJSON serializes val as JSON into the Data field.
func (msg *MqttMsg) SetJSON(val any) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("mqtt data JSON marshal: %w", err)
	}
	msg.Data = data
	return nil
}

// NewMqttMsg creates a new MqttMsg with default magic constants.
func NewMqttMsg(pktType MqttPktType, pktNum uint16, deviceGuid string, data []byte) MqttMsg {
	return MqttMsg{
		Signature:  mqttSignature,
		Size:       0, // Set by Pack/Encrypt
		M3:         uint8(mqttM3),
		M4:         uint8(mqttM4),
		M5:         uint8(mqttM5),
		M6:         uint8(mqttM6),
		M7:         uint8(mqttM7),
		PacketType: pktType,
		PacketNum:  pktNum,
		Time:       0, // Set by caller
		DeviceGuid: deviceGuid,
		Padding:    make([]byte, 11),
		Data:       data,
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// ParseMqttMsgTypeFromByte parses a single byte as MqttMsgType.
// Note: In the Python implementation, MqttMsgType.parse uses struct.unpack("B", ...)
// which reads only the low byte. The actual command type is a u16le in the payload.
func ParseMqttMsgTypeFromByte(p []byte) (MqttMsgType, []byte, error) {
	v, rest, err := types.ParseU8(p)
	if err != nil {
		return 0, nil, err
	}
	return MqttMsgType(v), rest, nil
}

// ParseMqttMsgTypeFromU16LE parses a u16le as MqttMsgType.
func ParseMqttMsgTypeFromU16LE(p []byte) (MqttMsgType, []byte, error) {
	v, rest, err := types.ParseU16LE(p)
	if err != nil {
		return 0, nil, err
	}
	return MqttMsgType(v), rest, nil
}

// PackMqttMsgTypeAsU16LE packs a MqttMsgType as u16le.
func PackMqttMsgTypeAsU16LE(t MqttMsgType) []byte {
	return types.U16LE(t).Pack()
}

// ParseMqttPktType parses a single byte as MqttPktType.
func ParseMqttPktType(p []byte) (MqttPktType, []byte, error) {
	v, rest, err := types.ParseU8(p)
	if err != nil {
		return 0, nil, err
	}
	return MqttPktType(v), rest, nil
}

// PackMqttPktType packs a MqttPktType as a single byte.
func PackMqttPktType(t MqttPktType) []byte {
	return types.U8(t).Pack()
}

// Ensure binary import is used (for future use)
var _ = binary.BigEndian
