package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Test Driver client (connects to base path, joins /drivers namespace via protocol)
	logger.Info("=== Testing Driver Client (/socket.io/ -> /drivers namespace) ===")
	driverConn := connectSocket(logger, "/socket.io/")
	defer driverConn.Close()

	// Join /drivers namespace after connection
	joinNamespace(logger, driverConn, "/drivers")

	// Test Rider client (connects to base path, joins /orders namespace)
	logger.Info("=== Testing Rider Client (/socket.io/ -> /orders namespace) ===")
	riderConn := connectSocket(logger, "/socket.io/")
	defer riderConn.Close()

	// Join /orders namespace
	joinNamespace(logger, riderConn, "/orders")

	// Subscribe to an order room
	orderID := "test-order-123"
	logger.Info("Subscribing to order", "order_id", orderID)
	subscribeMsg := `42["order.subscribe","test-order-123"]`
	if err := riderConn.WriteMessage(websocket.TextMessage, []byte(subscribeMsg)); err != nil {
		logger.Error("Failed to send subscribe", "error", err)
	} else {
		logger.Info("Subscribed to order", "order_id", orderID)
	}

	// Keep running to receive events
	logger.Info("Waiting for events... (Ctrl+C to exit)")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")
	time.Sleep(1 * time.Second)
}

func connectSocket(logger *slog.Logger, path string) *websocket.Conn {
	url := "ws://localhost:8080" + path + "?EIO=4&transport=websocket"
	logger.Info("Connecting to WebSocket", "url", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		logger.Error("Failed to connect", "path", path, "error", err)
		os.Exit(1)
	}

	logger.Info("Connected", "path", path)

	// Handle Engine.IO handshake (first message should be "0" - open)
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Error("Read error", "error", err)
				}
				return
			}
			logger.Debug("Received message", "message", string(msg))
		}
	}()

	return conn
}

func joinNamespace(logger *slog.Logger, conn *websocket.Conn, namespace string) {
	// Send Socket.IO namespace join packet: 40<namespace>
	msg := "40" + namespace
	logger.Info("Joining namespace", "namespace", namespace)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		logger.Error("Failed to join namespace", "namespace", namespace, "error", err)
	}
}
