// Package pppp implements the PPPP (peer-to-peer) protocol API for AnkerMake M5 printers.
//
// This file defines the PPPP API client, channel management, and file transfer logic.
// It is the Go equivalent of libflagship/ppppapi.py from the Python implementation.
package pppp

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucas/openpolyprint/internal/anker/proto/types"
)

// PPPP protocol ports
const (
	LANPort = 32108
	WANPort = 32100
)

// ─── FileUploadInfo ──────────────────────────────────────────────────────────

// FileUploadInfo contains metadata for a file being uploaded to the printer.
type FileUploadInfo struct {
	Name      string
	Size      int
	MD5       string
	UserName  string
	UserID    string
	MachineID string
	Type      int
}

// SanitizeFilename removes dangerous characters from a filename.
func SanitizeFilename(s string) string {
	var sb strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			sb.WriteRune(c)
		} else {
			sb.WriteByte('_')
		}
	}
	result := sb.String()
	// Strip leading dots
	result = strings.TrimLeft(result, ".")
	// Replace ".." with "."
	result = strings.ReplaceAll(result, "..", ".")
	return result
}

// FileUploadInfoFromFile creates a FileUploadInfo from a file on disk.
// Bug fix: uses defer for file close to prevent file handle leaks.
func FileUploadInfoFromFile(filename, userName, userID, machineID string, fileType int) (*FileUploadInfo, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	data := make([]byte, stat.Size())
	_, err = f.Read(data)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return FileUploadInfoFromData(data, filename, userName, userID, machineID, fileType), nil
}

// FileUploadInfoFromData creates a FileUploadInfo from raw file data.
func FileUploadInfoFromData(data []byte, filename, userName, userID, machineID string, fileType int) *FileUploadInfo {
	hash := md5.Sum(data)
	return &FileUploadInfo{
		Name:      SanitizeFilename(filepath.Base(filename)),
		Size:      len(data),
		MD5:       hex.EncodeToString(hash[:]),
		UserName:  userName,
		UserID:    userID,
		MachineID: machineID,
		Type:      fileType,
	}
}

// String returns the CSV representation of the file upload info.
func (f FileUploadInfo) String() string {
	return fmt.Sprintf("%d,%s,%d,%s,%s,%s,%s", f.Type, f.Name, f.Size, f.MD5, f.UserName, f.UserID, f.MachineID)
}

// Bytes returns the binary representation (CSV + null terminator).
func (f FileUploadInfo) Bytes() []byte {
	return append([]byte(f.String()), 0)
}

// ─── PPPPError ───────────────────────────────────────────────────────────────

// PPPPError represents a PPPP protocol error with a specific error code.
type PPPPError struct {
	Err     FileTransferReply
	Message string
}

func (e *PPPPError) Error() string {
	return fmt.Sprintf("pppp error: %s (code 0x%02x)", e.Message, byte(e.Err))
}

// ─── Channel ─────────────────────────────────────────────────────────────────

// Channel manages a bidirectional data channel over the PPPP protocol.
type Channel struct {
	Index       int
	rxQueue     map[types.CyclicU16][]byte
	txQueue     []txEntry
	backlog     []txEntry
	rxCtr       types.CyclicU16
	txCtr       types.CyclicU16
	txAck       types.CyclicU16
	rxBuf       []byte
	txCh        chan []byte
	rxCh        chan []byte
	timeout     time.Duration
	acks        map[types.CyclicU16]bool
	event       *sync.Cond
	maxInFlight int
	maxAgeWarn  types.CyclicU16
	mu          sync.Mutex
}

type txEntry struct {
	deadline time.Time
	index    types.CyclicU16
	data     []byte
}

// NewChannel creates a new Channel with the given index.
func NewChannel(index int) *Channel {
	c := &Channel{
		Index:       index,
		rxQueue:     make(map[types.CyclicU16][]byte),
		timeout:     500 * time.Millisecond,
		acks:        make(map[types.CyclicU16]bool),
		maxInFlight: 64,
		maxAgeWarn:  128,
		txCh:        make(chan []byte, 256),
		rxCh:        make(chan []byte, 256),
	}
	c.event = sync.NewCond(&c.mu)
	return c
}

// RxAck processes acknowledgments for transmitted packets.
func (c *Channel) RxAck(acks []uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ackSet := make(map[uint16]bool, len(acks))
	for _, a := range acks {
		ackSet[a] = true
	}

	// Remove ACKed packets from tx queue
	var newTxQueue []txEntry
	for _, tx := range c.txQueue {
		if !ackSet[uint16(tx.index)] {
			newTxQueue = append(newTxQueue, tx)
		}
	}
	c.txQueue = newTxQueue

	// Record new ACKs
	for _, a := range acks {
		ca := types.CyclicU16(a)
		if ca.GreaterThanOrEqual(c.txAck) {
			c.acks[ca] = true
		}
	}

	// Advance txAck
	for c.acks[c.txAck] {
		delete(c.acks, c.txAck)
		c.txAck = c.txAck.Add(1)
	}

	c.event.Broadcast()
}

// RxDrw processes an incoming DRW (data read/write) packet.
func (c *Channel) RxDrw(index uint16, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ci := types.CyclicU16(index)

	// Drop old packets
	if c.rxCtr.GreaterThan(ci) {
		if c.maxAgeWarn > 0 && uint16(c.rxCtr.Sub(int(ci))) > uint16(c.maxAgeWarn) {
			_ = fmt.Errorf("dropping old packet: rxCtr=%d, index=%d", c.rxCtr, ci)
		}
		return
	}

	// Record packet
	c.rxQueue[ci] = data

	// Recombine data from queue
	for {
		data, ok := c.rxQueue[c.rxCtr]
		if !ok {
			break
		}
		delete(c.rxQueue, c.rxCtr)
		c.rxCtr = c.rxCtr.Add(1)
		c.rxBuf = append(c.rxBuf, data...)
		select {
		case c.rxCh <- data:
		default:
		}
	}
}

// Poll returns packets that need to be (re)transmitted.
func (c *Channel) Poll() []PktDrw {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Move backlog to tx queue
	if len(c.backlog) > 0 && len(c.txQueue) < c.maxInFlight {
		for len(c.backlog) > 0 && len(c.txQueue) < c.maxInFlight {
			c.txQueue = append(c.txQueue, c.backlog[0])
			c.backlog = c.backlog[1:]
		}
	}

	var result []PktDrw
	now := time.Now()

	for len(c.txQueue) > 0 && !c.txQueue[0].deadline.After(now) {
		entry := c.txQueue[0]
		c.txQueue = c.txQueue[1:]
		result = append(result, PktDrw{
			Chan:  uint8(c.Index),
			Index: uint16(entry.index),
			Data:  entry.data,
		})
		// Reschedule with updated deadline
		c.txQueue = append(c.txQueue, txEntry{
			deadline: now.Add(c.timeout),
			index:    entry.index,
			data:     entry.data,
		})
	}

	return result
}

// Write queues data for transmission on this channel.
// If block is true, it waits until all data is acknowledged.
func (c *Channel) Write(payload []byte, block bool) (uint16, uint16) {
	c.mu.Lock()

	txStart := c.txCtr
	deadline := time.Now()

	for len(payload) > 0 {
		var chunk []byte
		if len(payload) > 1024 {
			chunk = payload[:1024]
			payload = payload[1024:]
		} else {
			chunk = payload
			payload = nil
		}
		c.backlog = append(c.backlog, txEntry{
			deadline: deadline,
			index:    c.txCtr,
			data:     chunk,
		})
		c.txCtr = c.txCtr.Add(1)
	}

	txDone := c.txCtr
	c.mu.Unlock()

	if block {
		c.mu.Lock()
		for c.txAck.LessThan(txDone) {
			c.event.Wait()
		}
		c.mu.Unlock()
	}

	return uint16(txStart), uint16(txDone)
}

// Read reads up to n bytes from the channel's receive buffer.
func (c *Channel) Read(n int, timeout time.Duration) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.rxBuf) >= n {
		result := make([]byte, n)
		copy(result, c.rxBuf[:n])
		c.rxBuf = c.rxBuf[n:]
		return result, nil
	}

	if timeout <= 0 {
		// Non-blocking, return what we have
		result := make([]byte, len(c.rxBuf))
		copy(result, c.rxBuf)
		c.rxBuf = nil
		return result, nil
	}

	// Wait for more data with timeout
	deadline := time.Now().Add(timeout)
	timer := time.AfterFunc(time.Until(deadline), func() {
		c.mu.Lock()
		c.event.Broadcast()
		c.mu.Unlock()
	})
	defer timer.Stop()

	for len(c.rxBuf) < n && time.Now().Before(deadline) {
		c.event.Wait()
	}

	if len(c.rxBuf) >= n {
		result := make([]byte, n)
		copy(result, c.rxBuf[:n])
		c.rxBuf = c.rxBuf[n:]
		return result, nil
	}

	return nil, fmt.Errorf("timeout waiting for %d bytes", n)
}

// Peek returns up to n bytes from the receive buffer without consuming them.
func (c *Channel) Peek(n int, timeout time.Duration) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.rxBuf) >= n {
		result := make([]byte, n)
		copy(result, c.rxBuf[:n])
		return result, nil
	}

	if timeout <= 0 {
		return nil, nil
	}

	deadline := time.Now().Add(timeout)
	timer := time.AfterFunc(time.Until(deadline), func() {
		c.mu.Lock()
		c.event.Broadcast()
		c.mu.Unlock()
	})
	defer timer.Stop()

	for len(c.rxBuf) < n && time.Now().Before(deadline) {
		c.event.Wait()
	}

	if len(c.rxBuf) >= n {
		result := make([]byte, n)
		copy(result, c.rxBuf[:n])
		return result, nil
	}

	return nil, nil
}

// ─── PPPPState ───────────────────────────────────────────────────────────────

// PPPPState represents the connection state of the PPPP API.
type PPPPState int

const (
	StateIdle PPPPState = iota
	StateConnecting
	StateConnected
	StateDisconnected
)

func (s PPPPState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateDisconnected:
		return "Disconnected"
	default:
		return "Unknown"
	}
}

// ─── PPPPApi ─────────────────────────────────────────────────────────────────

// PPPPApi is the PPPP protocol client for communicating with AnkerMake printers.
type PPPPApi struct {
	conn    *net.UDPConn
	duid    *Duid
	addr    *net.UDPAddr
	state   PPPPState
	chans   [8]*Channel
	running bool
	mu      sync.Mutex
	dumper  func(direction string, data []byte, addr net.Addr)
}

// NewPPPPApi creates a new PPPP API client.
func NewPPPPApi(duid *Duid, host string, port int) (*PPPPApi, error) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("resolve UDP address: %w", err)
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("dial UDP: %w", err)
	}

	api := &PPPPApi{
		conn:  conn,
		duid:  duid,
		addr:  addr,
		state: StateIdle,
	}
	for i := range api.chans {
		api.chans[i] = NewChannel(i)
	}

	return api, nil
}

// NewPPPPApiLAN creates a PPPP API client for LAN communication (port 32108).
func NewPPPPApiLAN(duid *Duid, host string) (*PPPPApi, error) {
	return NewPPPPApi(duid, host, LANPort)
}

// NewPPPPApiWAN creates a PPPP API client for WAN communication (port 32100).
func NewPPPPApiWAN(duid *Duid, host string) (*PPPPApi, error) {
	return NewPPPPApi(duid, host, WANPort)
}

// NewPPPPApiBroadcast creates a PPPP API client for broadcast communication.
func NewPPPPApiBroadcast() (*PPPPApi, error) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", LANPort))
	if err != nil {
		return nil, fmt.Errorf("resolve broadcast address: %w", err)
	}

	// Connect the socket to the broadcast address: Send() writes on a
	// connected socket, so an unconnected ListenUDP socket would fail with
	// "no address supplied". Go enables SO_BROADCAST on UDP sockets itself.
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("dial broadcast UDP: %w", err)
	}

	err = conn.SetReadBuffer(4096)
	if err != nil {
		return nil, fmt.Errorf("set read buffer: %w", err)
	}

	return &PPPPApi{
		conn:  conn,
		addr:  addr,
		state: StateIdle,
	}, nil
}

// SetDumper sets a callback function for logging packet traffic.
func (a *PPPPApi) SetDumper(fn func(direction string, data []byte, addr net.Addr)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dumper = fn
}

// State returns the current connection state.
func (a *PPPPApi) State() PPPPState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// Channel returns the channel at the given index, or nil if out of range.
func (a *PPPPApi) Channel(idx int) *Channel {
	if idx < 0 || idx >= len(a.chans) {
		return nil
	}
	return a.chans[idx]
}

// Channels returns all channels (including nil ones).
func (a *PPPPApi) Channels() [8]*Channel {
	return a.chans
}

// ConnectLanSearch initiates a LAN search by sending a PktLanSearch.
func (a *PPPPApi) ConnectLanSearch() error {
	a.mu.Lock()
	a.state = StateConnecting
	a.mu.Unlock()
	return a.Send(PktLanSearch{})
}

// Send sends a PPPP message to the printer.
func (a *PPPPApi) Send(msg Message) error {
	a.mu.Lock()
	state := a.state
	a.mu.Unlock()

	if state == StateIdle || state == StateDisconnected {
		return fmt.Errorf("cannot send in state %s", state)
	}

	data := PackMessage(msg)
	a.mu.Lock()
	dumper := a.dumper
	addr := a.addr
	a.mu.Unlock()

	if dumper != nil {
		dumper("tx", data, addr)
	}

	_, err := a.conn.Write(data)
	if err != nil {
		return fmt.Errorf("send PPPP packet: %w", err)
	}

	return nil
}

// Recv receives a PPPP message from the printer with a timeout.
func (a *PPPPApi) Recv(timeout time.Duration) (Message, error) {
	a.mu.Lock()
	state := a.state
	a.mu.Unlock()

	if state == StateIdle || state == StateDisconnected {
		return nil, fmt.Errorf("cannot recv in state %s", state)
	}

	if err := a.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	buf := make([]byte, 4096)
	n, addr, err := a.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	data := buf[:n]

	a.mu.Lock()
	dumper := a.dumper
	a.addr = addr
	a.mu.Unlock()

	if dumper != nil {
		dumper("rx", data, addr)
	}

	msg, _, err := ParseMessage(data)
	if err != nil {
		return nil, fmt.Errorf("parse PPPP message: %w", err)
	}

	return msg, nil
}

// Process handles an incoming PPPP message.
func (a *PPPPApi) Process(msg Message) error {
	switch m := msg.(type) {
	case PktClose:
		a.mu.Lock()
		a.state = StateDisconnected
		a.mu.Unlock()
		return fmt.Errorf("received CLOSE from printer")

	case PktAlive:
		return a.Send(PktAliveAck{})

	case PktDrw:
		// Send ACK
		err := a.Send(PktDrwAck{Chan: m.Chan, Count: 1, Acks: []uint16{m.Index}})
		if err != nil {
			return err
		}
		// Deliver to channel
		if int(m.Chan) < len(a.chans) && a.chans[m.Chan] != nil {
			a.chans[m.Chan].RxDrw(m.Index, m.Data)
		}

	case PktDrwAck:
		if int(m.Chan) < len(a.chans) && a.chans[m.Chan] != nil {
			a.chans[m.Chan].RxAck(m.Acks)
		}

	case PktHello:
		host := Host{Afam: 2, Port: uint16(a.addr.Port), Addr: a.addr.IP.String()}
		return a.Send(PktHelloAck{Host: host})

	case PktP2PRdy:
		if a.duid != nil {
			host := Host{Afam: 2, Port: uint16(a.addr.Port), Addr: a.addr.IP.String()}
			err := a.Send(PktP2PRdyAck{Duid: *a.duid, Host: host})
			if err != nil {
				return err
			}
		}
		a.mu.Lock()
		a.state = StateConnected
		a.mu.Unlock()
	}

	return nil
}

// Run is the main event loop for the PPPP API.
// It runs until Stop() is called.
func (a *PPPPApi) Run() {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()

	for a.running {
		msg, err := a.Recv(50 * time.Millisecond)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Normal timeout, continue
			} else {
				break
			}
		} else {
			_ = a.Process(msg)
		}

		// Poll channels for retransmission
		for _, ch := range a.chans {
			if ch == nil {
				continue
			}
			for _, pkt := range ch.Poll() {
				_ = a.Send(pkt)
			}
		}
	}

	_ = a.Send(PktClose{})
}

// Stop stops the PPPP API event loop.
func (a *PPPPApi) Stop() {
	a.mu.Lock()
	a.running = false
	a.mu.Unlock()
}

// Close closes the underlying UDP connection.
func (a *PPPPApi) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// SendXzyh sends an XZYH command on the specified channel.
func (a *PPPPApi) SendXzyh(data []byte, cmd P2PCmdType, chanIdx int, block bool) (uint16, uint16) {
	xzyh := Xzyh{
		Cmd:  cmd,
		Len:  uint32(len(data)),
		Data: data,
		Chan: uint8(chanIdx),
	}
	return a.chans[chanIdx].Write(xzyh.Pack(), block)
}

// SendAabb sends an AABB file transfer packet on the specified channel.
func (a *PPPPApi) SendAabb(data []byte, sn uint8, pos uint32, frameType FileTransfer, chanIdx int, block bool) (uint16, uint16) {
	aabb := Aabb{
		FrameType: frameType,
		SN:        sn,
		Pos:       pos,
		Len:       uint32(len(data)),
	}
	return a.chans[chanIdx].Write(PackAabbWithCRC(aabb, data), block)
}

// RecvXzyh receives an XZYH command on the specified channel.
func (a *PPPPApi) RecvXzyh(chanIdx int, timeout time.Duration) (*Xzyh, error) {
	ch := a.chans[chanIdx]

	hdr, err := ch.Peek(16, timeout)
	if err != nil || hdr == nil {
		return nil, fmt.Errorf("timeout waiting for XZYH header")
	}

	xzyh, _, err := ParseXzyh(hdr)
	if err != nil {
		return nil, fmt.Errorf("parse XZYH: %w", err)
	}

	fullData, err := ch.Read(int(xzyh.Len)+16, timeout)
	if err != nil {
		return nil, fmt.Errorf("read XZYH data: %w", err)
	}

	xzyh.Data = fullData[16:]
	return &xzyh, nil
}

// RecvAabb receives an AABB file transfer packet on the specified channel.
func (a *PPPPApi) RecvAabb(chanIdx int) (*Aabb, []byte, error) {
	ch := a.chans[chanIdx]

	hdr, err := ch.Read(12, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("read AABB header: %w", err)
	}

	aabb, _, err := ParseAabb(hdr)
	if err != nil {
		return nil, nil, fmt.Errorf("parse AABB: %w", err)
	}

	rest, err := ch.Read(int(aabb.Len)+2, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("read AABB data: %w", err)
	}

	fullData := append(hdr, rest...)
	parsedAabb, data, _, err := ParseAabbWithCRC(fullData)
	if err != nil {
		return nil, nil, fmt.Errorf("parse AABB with CRC: %w", err)
	}

	return &parsedAabb, data, nil
}

// RecvAabbReply receives an AABB reply and checks the result.
func (a *PPPPApi) RecvAabbReply(chanIdx int, check bool) (FileTransferReply, error) {
	_, data, err := a.RecvAabb(chanIdx)
	if err != nil {
		return 0, err
	}

	if len(data) != 1 {
		return 0, fmt.Errorf("unexpected reply length: %d", len(data))
	}

	res := FileTransferReply(data[0])
	if check && res != FTReplyOK {
		return res, &PPPPError{Err: res, Message: fmt.Sprintf("AABB request failed: %d", res)}
	}

	return res, nil
}

// AabbRequest sends an AABB request and waits for the reply.
func (a *PPPPApi) AabbRequest(data []byte, frameType FileTransfer, pos uint32, chanIdx int, check bool) (FileTransferReply, error) {
	_, _ = a.SendAabb(data, 0, pos, frameType, chanIdx, true)
	return a.RecvAabbReply(chanIdx, check)
}
