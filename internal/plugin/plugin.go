// Package plugin provides internal plugin loading and execution infrastructure.
package plugin

//go:generate mockgen -source=plugin.go -destination=plugin_mock.go -package=plugin

import (
	"context"

	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

// Plugin represents an internal plugin instance that can be invoked.
// This is the internal interface used by the dispatcher, separate from
// the public API in pkg/plugin.
type Plugin interface {
	// Info returns metadata about the plugin.
	Info() plugin.Info

	// Validate performs validation and returns a response.
	// Context can be used for timeouts and cancellation.
	Validate(ctx context.Context, req *plugin.ValidateRequest) (*plugin.ValidateResponse, error)

	// Close releases any resources held by the plugin.
	// For Go plugins this is a no-op, for gRPC this closes connections,
	// and for exec plugins this may clean up temp files.
	Close() error
}
