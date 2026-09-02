package api

import "net/http"

// Response is the transport-neutral view of what a handler produced.
// The active adapter serializes Body as JSON and merges Header before
// writing Status. Response never references Gin.
type Response struct {
	Status int         // HTTP status; zero → 200 by convention
	Header http.Header // optional extra headers (Location, Retry-After, ...)
	Body   any         // nil → empty body; otherwise JSON-encoded
}

// JSON builds a success-style response with an explicit status code.
func JSON(status int, body any) Response {
	return Response{Status: status, Body: body}
}

// NoContent builds a 204 response.
func NoContent() Response {
	return Response{Status: http.StatusNoContent}
}
