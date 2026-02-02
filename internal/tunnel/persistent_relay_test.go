package tunnel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// mockRelayServer simulates the coordination server's persistent relay endpoint.
type mockRelayServer struct {
	t           *testing.T
	server      *httptest.Server
	connections map[string]*websocket.Conn
	mu          sync.Mutex
	received    chan relayMessage
}

type relayMessage struct {
	source string
	target string
	data   []byte
}

func newMockRelayServer(t *testing.T) *mockRelayServer {
	m := &mockRelayServer{
		t:           t,
		connections: make(map[string]*websocket.Conn),
		received:    make(chan relayMessage, 10),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/relay/persistent", m.handlePersistentRelay)
	m.server = httptest.NewServer(mux)

	return m
}

func (m *mockRelayServer) handlePersistentRelay(w http.ResponseWriter, r *http.Request) {
	// Extract peer name from auth header (simplified)
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	peerName := strings.TrimPrefix(auth, "Bearer ")

	conn, err := testUpgrader.Upgrade(w, r, nil)
	if err != nil {
		m.t.Logf("upgrade failed: %v", err)
		return
	}

	m.mu.Lock()
	m.connections[peerName] = conn
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connections, peerName)
		m.mu.Unlock()
		conn.Close()
	}()

	// Read and route messages
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if len(data) < 3 {
			continue
		}

		msgType := data[0]
		if msgType == MsgTypeSendPacket {
			targetLen := int(data[1])
			if len(data) < 2+targetLen {
				continue
			}
			targetPeer := string(data[2 : 2+targetLen])
			packetData := data[2+targetLen:]

			m.received <- relayMessage{
				source: peerName,
				target: targetPeer,
				data:   packetData,
			}

			// Route to target if connected
			m.mu.Lock()
			targetConn, ok := m.connections[targetPeer]
			m.mu.Unlock()

			if ok {
				// Build recv message
				msg := make([]byte, 2+len(peerName)+len(packetData))
				msg[0] = MsgTypeRecvPacket
				msg[1] = byte(len(peerName))
				copy(msg[2:], peerName)
				copy(msg[2+len(peerName):], packetData)
				_ = targetConn.WriteMessage(websocket.BinaryMessage, msg)
			}
		}
	}
}

func (m *mockRelayServer) URL() string {
	return m.server.URL
}

func (m *mockRelayServer) Close() {
	m.server.Close()
}

func TestPersistentRelay_Connect(t *testing.T) {
	server := newMockRelayServer(t)
	defer server.Close()

	relay := NewPersistentRelay(server.URL(), "peer1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := relay.Connect(ctx)
	require.NoError(t, err)
	assert.True(t, relay.IsConnected())

	relay.Close()
	assert.False(t, relay.IsConnected())
}

func TestPersistentRelay_SendTo(t *testing.T) {
	server := newMockRelayServer(t)
	defer server.Close()

	relay := NewPersistentRelay(server.URL(), "peer1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := relay.Connect(ctx)
	require.NoError(t, err)
	defer relay.Close()

	// Send a packet
	testData := []byte("hello world")
	err = relay.SendTo("peer2", testData)
	require.NoError(t, err)

	// Verify server received it
	select {
	case msg := <-server.received:
		assert.Equal(t, "peer1", msg.source)
		assert.Equal(t, "peer2", msg.target)
		assert.Equal(t, testData, msg.data)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestPersistentRelay_ReceivePacket(t *testing.T) {
	server := newMockRelayServer(t)
	defer server.Close()

	// Connect two peers
	relay1 := NewPersistentRelay(server.URL(), "peer1")
	relay2 := NewPersistentRelay(server.URL(), "peer2")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := relay1.Connect(ctx)
	require.NoError(t, err)
	defer relay1.Close()

	err = relay2.Connect(ctx)
	require.NoError(t, err)
	defer relay2.Close()

	// Set up packet handler on peer2
	received := make(chan []byte, 1)
	relay2.SetPacketHandler(func(sourcePeer string, data []byte) {
		assert.Equal(t, "peer1", sourcePeer)
		received <- data
	})

	// Give time for both connections to be established
	time.Sleep(100 * time.Millisecond)

	// Send from peer1 to peer2
	testData := []byte("hello from peer1")
	err = relay1.SendTo("peer2", testData)
	require.NoError(t, err)

	// Verify peer2 received it
	select {
	case data := <-received:
		assert.Equal(t, testData, data)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestPersistentRelay_SendNotConnected(t *testing.T) {
	relay := NewPersistentRelay("http://localhost:9999", "peer1")

	err := relay.SendTo("peer2", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestPeerTunnel_ReadWrite(t *testing.T) {
	server := newMockRelayServer(t)
	defer server.Close()

	// Connect two peers
	relay1 := NewPersistentRelay(server.URL(), "peer1")
	relay2 := NewPersistentRelay(server.URL(), "peer2")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := relay1.Connect(ctx)
	require.NoError(t, err)
	defer relay1.Close()

	err = relay2.Connect(ctx)
	require.NoError(t, err)
	defer relay2.Close()

	// Give time for connections
	time.Sleep(100 * time.Millisecond)

	// Create peer tunnels
	tunnel1to2 := relay1.NewPeerTunnel("peer2")
	tunnel2to1 := relay2.NewPeerTunnel("peer1")

	// Write from tunnel1 to tunnel2
	testData := []byte("hello via peer tunnel")
	n, err := tunnel1to2.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Read on tunnel2
	buf := make([]byte, 100)
	n, err = tunnel2to1.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])
}

func TestPeerTunnel_Close(t *testing.T) {
	relay := NewPersistentRelay("http://localhost:9999", "peer1")
	tunnel := relay.NewPeerTunnel("peer2")

	assert.False(t, tunnel.IsClosed())

	err := tunnel.Close()
	require.NoError(t, err)

	assert.True(t, tunnel.IsClosed())

	// Write should fail after close
	_, err = tunnel.Write([]byte("test"))
	assert.Error(t, err)
}
