// Package sockets exposes the Socket.IO transport used by mobile clients.
package sockets

import (
	"net/http"
	"os"
	"strings"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	socketio "github.com/zishang520/socket.io/servers/socket/v3"
)

const (
	driversNamespace = "/drivers"
	ridesNamespace   = "/rides"

	// RideRequestedEvent is emitted to driver clients when a ride is requested.
	RideRequestedEvent = "ride.requested"
	// RideAcceptedEvent is emitted to the rider client when a ride is accepted.
	RideAcceptedEvent = "ride.accepted"
)

// Server is the mobile Socket.IO gateway. Kafka consumers emit durable ride
// events through it; it does not make outbound HTTP webhook calls.
type Server struct {
	io      *socketio.Server
	drivers socketio.Namespace
	rides   socketio.Namespace
}

// NewServer creates a Socket.IO v4+ server. Mobile driver applications connect
// to /drivers; rider applications connect to /rides and subscribe to their
// ride room with the "ride.subscribe" event.
func NewServer() *Server {
	io := socketio.NewServer(nil, nil)
	drivers := io.Of(driversNamespace, nil)
	rideClients := io.Of(ridesNamespace, nil)

	_ = rideClients.On("connection", func(args ...any) {
		client, ok := args[0].(*socketio.Socket)
		if !ok {
			return
		}
		_ = client.On("ride.subscribe", func(values ...any) {
			if len(values) != 1 {
				return
			}
			rideID, ok := values[0].(string)
			if !ok || strings.TrimSpace(rideID) == "" {
				return
			}
			client.Join(socketio.Room(rideRoom(rideID)))
		})
	})

	return &Server{io: io, drivers: drivers, rides: rideClients}
}

// Handler serves the Socket.IO Engine.IO endpoint at /socket.io/.
func (s *Server) Handler() http.Handler { return s.io.ServeHandler(nil) }

// EmitRequested broadcasts a ride request to connected driver applications.
func (s *Server) EmitRequested(event rides.Requested) error {
	return s.drivers.Emit(RideRequestedEvent, event)
}

// EmitAccepted sends acceptance to the mobile client subscribed to this ride.
func (s *Server) EmitAccepted(event rides.Accepted) error {
	return s.rides.To(socketio.Room(rideRoom(event.RideID))).Emit(RideAcceptedEvent, event)
}

func rideRoom(rideID string) string { return "ride:" + rideID }

func envString(name string) string { return strings.TrimSpace(os.Getenv(name)) }
