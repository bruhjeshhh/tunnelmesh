package benchmark

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MessageType identifies the type of benchmark protocol message.
type MessageType byte

// Protocol message types.
const (
	MsgStart    MessageType = 0x30 // Initiator -> Receiver: start benchmark
	MsgAck      MessageType = 0x31 // Receiver -> Initiator: acknowledge start
	MsgData     MessageType = 0x32 // Data transfer chunk
	MsgPing     MessageType = 0x34 // Latency probe
	MsgPong     MessageType = 0x35 // Latency response
	MsgComplete MessageType = 0x33 // Transfer complete with stats
	MsgError    MessageType = 0x3F // Error message
)

// String returns a human-readable name for the message type.
func (m MessageType) String() string {
	switch m {
	case MsgStart:
		return "Start"
	case MsgAck:
		return "Ack"
	case MsgData:
		return "Data"
	case MsgPing:
		return "Ping"
	case MsgPong:
		return "Pong"
	case MsgComplete:
		return "Complete"
	case MsgError:
		return "Error"
	default:
		return fmt.Sprintf("Unknown(%d)", m)
	}
}

// Message is the interface implemented by all protocol messages.
type Message interface {
	Encode(w io.Writer) error
	Decode(r io.Reader) error
}

// StartMessage initiates a benchmark.
type StartMessage struct {
	Size      int64  // Total bytes to transfer
	Direction string // "upload" or "download"
}

// Encode writes the message to the writer.
func (m *StartMessage) Encode(w io.Writer) error {
	// Size (8 bytes) + Direction length (2 bytes) + Direction string
	if err := binary.Write(w, binary.BigEndian, m.Size); err != nil {
		return err
	}
	return writeString(w, m.Direction)
}

// Decode reads the message from the reader.
func (m *StartMessage) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.Size); err != nil {
		return err
	}
	var err error
	m.Direction, err = readString(r)
	return err
}

// AckMessage acknowledges a benchmark start.
type AckMessage struct {
	Accepted bool   // true if benchmark accepted
	Error    string // error message if not accepted
}

// Encode writes the message to the writer.
func (m *AckMessage) Encode(w io.Writer) error {
	var accepted byte
	if m.Accepted {
		accepted = 1
	}
	if err := binary.Write(w, binary.BigEndian, accepted); err != nil {
		return err
	}
	return writeString(w, m.Error)
}

// Decode reads the message from the reader.
func (m *AckMessage) Decode(r io.Reader) error {
	var accepted byte
	if err := binary.Read(r, binary.BigEndian, &accepted); err != nil {
		return err
	}
	m.Accepted = accepted != 0
	var err error
	m.Error, err = readString(r)
	return err
}

// DataMessage carries benchmark data.
type DataMessage struct {
	SeqNum uint32 // Sequence number
	Data   []byte // Payload data
}

// Encode writes the message to the writer.
func (m *DataMessage) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, m.SeqNum); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(m.Data))); err != nil {
		return err
	}
	_, err := w.Write(m.Data)
	return err
}

// Decode reads the message from the reader.
func (m *DataMessage) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.SeqNum); err != nil {
		return err
	}
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return err
	}
	m.Data = make([]byte, length)
	_, err := io.ReadFull(r, m.Data)
	return err
}

// PingMessage is a latency probe.
type PingMessage struct {
	SeqNum    uint32 // Sequence number
	Timestamp int64  // Sender timestamp (nanoseconds)
}

// Encode writes the message to the writer.
func (m *PingMessage) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, m.SeqNum); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, m.Timestamp)
}

// Decode reads the message from the reader.
func (m *PingMessage) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.SeqNum); err != nil {
		return err
	}
	return binary.Read(r, binary.BigEndian, &m.Timestamp)
}

// PongMessage is a latency response.
type PongMessage struct {
	SeqNum        uint32 // Sequence number (matches Ping)
	PingTimestamp int64  // Original ping timestamp
}

// Encode writes the message to the writer.
func (m *PongMessage) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, m.SeqNum); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, m.PingTimestamp)
}

// Decode reads the message from the reader.
func (m *PongMessage) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.SeqNum); err != nil {
		return err
	}
	return binary.Read(r, binary.BigEndian, &m.PingTimestamp)
}

// CompleteMessage signals benchmark completion.
type CompleteMessage struct {
	BytesTransferred int64 // Total bytes transferred
	DurationNs       int64 // Duration in nanoseconds
}

// Encode writes the message to the writer.
func (m *CompleteMessage) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, m.BytesTransferred); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, m.DurationNs)
}

// Decode reads the message from the reader.
func (m *CompleteMessage) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.BytesTransferred); err != nil {
		return err
	}
	return binary.Read(r, binary.BigEndian, &m.DurationNs)
}

// ErrorMessage signals an error.
type ErrorMessage struct {
	Error string
}

// Encode writes the message to the writer.
func (m *ErrorMessage) Encode(w io.Writer) error {
	return writeString(w, m.Error)
}

// Decode reads the message from the reader.
func (m *ErrorMessage) Decode(r io.Reader) error {
	var err error
	m.Error, err = readString(r)
	return err
}

// ReadMessage reads a message type and payload from the reader.
// Returns the message type and the raw payload bytes.
func ReadMessage(r io.Reader) (MessageType, []byte, error) {
	var msgType byte
	if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
		return 0, nil, err
	}

	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return 0, nil, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil, err
	}

	return MessageType(msgType), data, nil
}

// WriteMessage writes a message to the writer with its type prefix.
func WriteMessage(w io.Writer, msgType MessageType, msg Message) error {
	// First encode the message to get its length
	var buf [64 * 1024]byte // Stack buffer for small messages
	temp := &limitedBuffer{buf: buf[:0]}
	if err := msg.Encode(temp); err != nil {
		return err
	}

	// Write type
	if err := binary.Write(w, binary.BigEndian, byte(msgType)); err != nil {
		return err
	}

	// Write length
	if err := binary.Write(w, binary.BigEndian, uint32(len(temp.buf))); err != nil {
		return err
	}

	// Write payload
	_, err := w.Write(temp.buf)
	return err
}

// limitedBuffer is a simple buffer that grows as needed.
type limitedBuffer struct {
	buf []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// Helper functions for string encoding.

func writeString(w io.Writer, s string) error {
	if err := binary.Write(w, binary.BigEndian, uint16(len(s))); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

func readString(r io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	return string(data), nil
}
