// Package connectionseam is where package configdirectortest hands package configdirector the
// in-memory connection a test controls. Neither package can import the other's unexported
// members, so the two meet here, inside the module.
package connectionseam

import (
	"io"
	"log/slog"
)

// Receiver is the side of a client a connection talks to.
type Receiver struct {
	// Deliver hands the client one server SDK payload document, which it parses and applies on the
	// calling goroutine exactly as it would one from the streaming or polling transport.
	Deliver func(document string)

	// Logger is the client's logger, tagged with the client's component attribute.
	Logger *slog.Logger
}

// OpenConnection opens the connection a client reads config state through and returns what the
// client closes when it is closed.
type OpenConnection func(receiver Receiver) io.Closer

// NewClientOverConnection builds a configdirector.Client over the connection an OpenConnection
// opens, in place of a transport, and otherwise exactly as configdirector.NewClient builds one.
// Package configdirector sets it when it initializes and package configdirectortest is its only
// caller. This package cannot name either's types without an import cycle, so the value is held
// as any and is a func(configdirector.Options, OpenConnection) (*configdirector.Client, error).
var NewClientOverConnection any
