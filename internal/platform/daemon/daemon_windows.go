//go:build windows

package daemon

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sys/windows/svc"
)

// run hosts serve as a Windows service when the SCM launched this process, and
// otherwise behaves like a console program so `opsmini.exe` can be started in a
// terminal for debugging.
func run(name string, serve func(ctx context.Context) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return serve(ctx)
	}
	return svc.Run(name, &handler{serve: serve})
}

// handler bridges the SCM control protocol onto a context.
type handler struct {
	serve func(ctx context.Context) error
}

// shutdownGrace bounds how long a Stop request waits for in-flight requests
// before the SCM is told the service stopped anyway. The SCM itself allows 30s
// by default; staying well under it avoids a "did not respond" kill.
const shutdownGrace = 15 * time.Second

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- h.serve(ctx) }()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case err := <-done:
			if err != nil {
				log.Printf("service exited with error: %v", err)
				changes <- svc.Status{State: svc.Stopped}
				return false, 1
			}
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(shutdownGrace):
					log.Printf("shutdown did not finish within %s", shutdownGrace)
				}
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		}
	}
}
