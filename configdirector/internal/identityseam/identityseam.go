// Package identityseam is where package wrapper hands package configdirector the identity a
// module built on the SDK reports in the SDK's place. Neither package can import the other's
// unexported members, so the two meet here, inside the module.
package identityseam

// Identity is the name and version a client reports to ConfigDirector in the User-Agent header
// and in telemetry.
type Identity struct {
	SDKName    string
	SDKVersion string
}

// NewClientIdentifiedAs builds a configdirector.Client that reports identity in place of the
// SDK's own, and otherwise exactly as configdirector.NewClient builds one. Package configdirector
// sets it when it initializes and package wrapper is its only caller. This package cannot name
// either's types without an import cycle, so the value is held as any and is a
// func(Identity, configdirector.Options) (*configdirector.Client, error).
var NewClientIdentifiedAs any
