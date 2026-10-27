package sentinel

import (
	"context"

	"github.com/opsmini/opsmini/internal/model"
)

// metricCollector bridges the existing alert engine into the event bus: the
// alert monitor already owns metric polling, so there is no point polling twice.
type metricCollector struct {
	in <-chan model.Signal
}

func (c *metricCollector) Name() string { return "metricwatch" }

func (c *metricCollector) Run(ctx context.Context, out func(Signal)) {
	if c.in == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-c.in:
			if !ok {
				return
			}
			out(Signal{s})
		}
	}
}
