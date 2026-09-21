package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/guilhermelinosp/hellnet-lib-environments/environments"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
)

func main() {
	ops, err := telemetry.New()
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = ops.Shutdown() }()

	// Test Driver client (connects to base path, joins /drivers namespace via protocol)
	ops.Info("=== Testing Driver Client (/socket.io/ -> /drivers namespace) ===")
	driverConn := connectSocket(ops, "/socket.io/")
	defer func() { _ = driverConn.Close() }()

	// Join /drivers namespace after connection
	joinNamespace(ops, driverConn, "/drivers")

	// Test Rider client (connects to base path, joins /orders namespace)
	ops.Info("=== Testing Rider Client (/socket.io/ -> /orders namespace) ===")
	riderConn := connectSocket(ops, "/socket.io/")
	defer func() { _ = riderConn.Close() }()

	// Join /orders namespace
	joinNamespace(ops, riderConn, "/orders")

	// Subscribe to an order room
	orderID := "test-order-123"
	ops.Info("Subscribing to order", "order_id", orderID)
	subscribeMsg := `42["order.subscribe","test-order-123"]`
	if err := riderConn.WriteMessage(websocket.TextMessage, []byte(subscribeMsg)); err != nil {
		ops.Error("Failed to send subscribe", "error", err)
	} else {
		ops.Info("Subscribed to order", "order_id", orderID)
	}

	// Keep running to receive events
	ops.Info("Waiting for events... (Ctrl+C to exit)")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	ops.Info("Shutting down...")
	time.Sleep(1 * time.Second)
}

func connectSocket(ops *telemetry.Telemetry, path string) *websocket.Conn {
	baseURL := environments.GetString("HELLNET_", "", "SOCKET_URL", "ws://localhost:8080")
	url := baseURL + path + "?EIO=4&transport=websocket"
	ops.Info("Connecting to WebSocket", "url", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		ops.Error("Failed to connect", "path", path, "error", err)
		os.Exit(1)
	}

	ops.Info("Connected", "path", path)

	// Handle Engine.IO handshake (first message should be "0" - open)
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					ops.Error("Read error", "error", err)
				}
				return
			}
			ops.Info("Received message", "message", string(msg))
		}
	}()

	return conn
}

func joinNamespace(ops *telemetry.Telemetry, conn *websocket.Conn, namespace string) {
	// Send Socket.IO namespace join packet: 40<namespace>
	msg := "40" + namespace
	ops.Info("Joining namespace", "namespace", namespace)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		ops.Error("Failed to join namespace", "namespace", namespace, "error", err)
	}
}
