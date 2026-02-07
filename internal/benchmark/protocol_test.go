package benchmark

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageType_String(t *testing.T) {
	tests := []struct {
		mt   MessageType
		want string
	}{
		{MsgStart, "Start"},
		{MsgAck, "Ack"},
		{MsgData, "Data"},
		{MsgPing, "Ping"},
		{MsgPong, "Pong"},
		{MsgComplete, "Complete"},
		{MsgError, "Error"},
		{MessageType(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mt.String())
		})
	}
}

func TestStartMessage_Encode_Decode(t *testing.T) {
	original := StartMessage{
		Size:      100 * 1024 * 1024, // 100 MB
		Direction: DirectionUpload,
	}

	// Encode
	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	// Decode
	var decoded StartMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.Size, decoded.Size)
	assert.Equal(t, original.Direction, decoded.Direction)
}

func TestAckMessage_Encode_Decode(t *testing.T) {
	original := AckMessage{
		Accepted: true,
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded AckMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.Accepted, decoded.Accepted)
}

func TestAckMessage_Rejected(t *testing.T) {
	original := AckMessage{
		Accepted: false,
		Error:    "benchmark in progress",
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded AckMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.False(t, decoded.Accepted)
	assert.Equal(t, "benchmark in progress", decoded.Error)
}

func TestDataMessage_Encode_Decode(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	original := DataMessage{
		SeqNum: 42,
		Data:   data,
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded DataMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.SeqNum, decoded.SeqNum)
	assert.Equal(t, original.Data, decoded.Data)
}

func TestDataMessage_LargeData(t *testing.T) {
	// Test with 64KB chunk
	data := make([]byte, ChunkSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	original := DataMessage{
		SeqNum: 1000,
		Data:   data,
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded DataMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.SeqNum, decoded.SeqNum)
	assert.Equal(t, len(original.Data), len(decoded.Data))
}

func TestPingMessage_Encode_Decode(t *testing.T) {
	original := PingMessage{
		SeqNum:    5,
		Timestamp: time.Now().UnixNano(),
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded PingMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.SeqNum, decoded.SeqNum)
	assert.Equal(t, original.Timestamp, decoded.Timestamp)
}

func TestPongMessage_Encode_Decode(t *testing.T) {
	original := PongMessage{
		SeqNum:        5,
		PingTimestamp: time.Now().UnixNano(),
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded PongMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.SeqNum, decoded.SeqNum)
	assert.Equal(t, original.PingTimestamp, decoded.PingTimestamp)
}

func TestCompleteMessage_Encode_Decode(t *testing.T) {
	original := CompleteMessage{
		BytesTransferred: 100 * 1024 * 1024,
		DurationNs:       5 * int64(time.Second),
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded CompleteMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.BytesTransferred, decoded.BytesTransferred)
	assert.Equal(t, original.DurationNs, decoded.DurationNs)
}

func TestErrorMessage_Encode_Decode(t *testing.T) {
	original := ErrorMessage{
		Error: "connection timeout",
	}

	var buf bytes.Buffer
	err := original.Encode(&buf)
	require.NoError(t, err)

	var decoded ErrorMessage
	err = decoded.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.Error, decoded.Error)
}

func TestReadMessage(t *testing.T) {
	// Write a Start message using WriteMessage (with proper framing)
	start := StartMessage{
		Size:      1024,
		Direction: DirectionDownload,
	}

	var buf bytes.Buffer
	err := WriteMessage(&buf, MsgStart, &start)
	require.NoError(t, err)

	// Read it back
	mt, data, err := ReadMessage(&buf)
	require.NoError(t, err)
	assert.Equal(t, MsgStart, mt)

	// Decode the payload
	var decoded StartMessage
	err = decoded.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, start.Size, decoded.Size)
}

func TestWriteMessage(t *testing.T) {
	ping := PingMessage{
		SeqNum:    10,
		Timestamp: 12345678,
	}

	var buf bytes.Buffer
	err := WriteMessage(&buf, MsgPing, &ping)
	require.NoError(t, err)

	// Read it back
	mt, data, err := ReadMessage(&buf)
	require.NoError(t, err)
	assert.Equal(t, MsgPing, mt)

	var decoded PingMessage
	err = decoded.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, ping.SeqNum, decoded.SeqNum)
}

func TestReadMessage_EmptyReader(t *testing.T) {
	var buf bytes.Buffer
	_, _, err := ReadMessage(&buf)
	assert.Error(t, err)
}

func BenchmarkDataMessage_Encode(b *testing.B) {
	data := make([]byte, ChunkSize)
	msg := DataMessage{SeqNum: 1, Data: data}
	var buf bytes.Buffer

	b.ResetTimer()
	b.SetBytes(ChunkSize)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = msg.Encode(&buf)
	}
}

func BenchmarkDataMessage_Decode(b *testing.B) {
	data := make([]byte, ChunkSize)
	msg := DataMessage{SeqNum: 1, Data: data}
	var buf bytes.Buffer
	_ = msg.Encode(&buf)
	encoded := buf.Bytes()

	b.ResetTimer()
	b.SetBytes(ChunkSize)

	for i := 0; i < b.N; i++ {
		var decoded DataMessage
		_ = decoded.Decode(bytes.NewReader(encoded))
	}
}
