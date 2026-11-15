// Package daemon hosts the panel as a background service on each platform.
//
// The only thing that differs is who asks the process to stop: a Unix init
// system sends SIGTERM, the Windows Service Control Manager sends a Stop or
// Shutdown control through the service handler registered here. Everything
// above this line stays platform-neutral.
package daemon

import "context"

// Run starts serve and blocks until the platform asks the process to stop.
//
// On Windows, when the process was launched by the Service Control Manager it is
// hosted as a service and reports StartPending / Running / StopPending / Stopped.
// When launched from a console the behaviour matches Unix: Ctrl+C cancels ctx.
func Run(name string, serve func(ctx context.Context) error) error {
	return run(name, serve)
}
