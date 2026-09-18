// Package sockets exposes the Socket.IO transport used by mobile clients.
package sockets

import (
	"context"
	"net/http"
	"strings"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
	socketio "github.com/zishang520/socket.io/servers/socket/v3"
)

const (
	driversNamespace = "/drivers"
	ordersNamespace  = "/orders"

	// OrderRequestedEvent is emitted to driver clients when an order is requested.
	OrderRequestedEvent = "order.requested"
	// OrderAcceptedEvent is emitted to the rider client when an order is accepted.
	OrderAcceptedEvent = "order.accepted"
)

// Server is the mobile Socket.IO gateway. Kafka consumers emit durable ride
// events through it; it does not make outbound HTTP webhook calls.
type Server struct {
	io      *socketio.Server
	drivers socketio.Namespace
	orders  socketio.Namespace
	tel     telemetry.Client
}

// NewServer creates a Socket.IO v4+ server. Mobile driver applications connect
// to /drivers; rider applications connect to /orders and subscribe to their
// order room with the "order.subscribe" event.
func NewServer(tel telemetry.Client) *Server {
	io := socketio.NewServer(nil, nil)
	drivers := io.Of(driversNamespace, nil)
	orderClients := io.Of(ordersNamespace, nil)

	_ = orderClients.On("connection", func(args ...any) {
		client, ok := args[0].(*socketio.Socket)
		if !ok {
			return
		}
		_ = client.On("order.subscribe", func(values ...any) {
			if len(values) != 1 {
				return
			}
			orderID, ok := values[0].(string)
			if !ok || strings.TrimSpace(orderID) == "" {
				return
			}
			client.Join(socketio.Room(orderRoom(orderID)))
		})
	})

	return &Server{io: io, drivers: drivers, orders: orderClients, tel: tel}
}

// Handler serves the Socket.IO Engine.IO endpoint at /socket.io/.
func (s *Server) Handler() http.Handler { return s.io.ServeHandler(nil) }

// EmitRequested broadcasts an order request to connected driver applications.
func (s *Server) EmitRequested(event orders.OrderRequested) error {
	if s.tel == nil {
		return s.drivers.Emit(OrderRequestedEvent, event)
	}
	return s.tel.WithSpan("socket.emit.order_requested", func(ctx context.Context) error {
		s.tel.Log().Info("socket.emit.order_requested",
			"order_id", event.OrderID,
			"rider_id", event.RiderID,
			"event_id", event.EventID,
			"event_version", event.EventVersion,
		)
		return s.drivers.Emit(OrderRequestedEvent, event)
	})
}

// EmitAccepted sends acceptance to the mobile client subscribed to this order.
func (s *Server) EmitAccepted(event orders.OrderAccepted) error {
	if s.tel == nil {
		return s.orders.To(socketio.Room(orderRoom(event.OrderID))).Emit(OrderAcceptedEvent, event)
	}
	return s.tel.WithSpan("socket.emit.order_accepted", func(ctx context.Context) error {
		s.tel.Log().Info("socket.emit.order_accepted",
			"order_id", event.OrderID,
			"driver_id", event.DriverID,
			"event_id", event.EventID,
			"event_version", event.EventVersion,
		)
		return s.orders.To(socketio.Room(orderRoom(event.OrderID))).Emit(OrderAcceptedEvent, event)
	})
}

func orderRoom(orderID string) string { return "order:" + orderID }
