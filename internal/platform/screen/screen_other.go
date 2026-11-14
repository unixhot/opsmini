//go:build !windows

package screen

import (
	"context"

	"github.com/opsmini/opsmini/internal/platform/sys"
)

// capture is unavailable off Windows: the graphical terminal is a Windows-only
// feature (it exists to see the console session a Windows service cannot reach).
func capture(context.Context, int, int) ([]byte, int, int, error) {
	return nil, 0, 0, sys.ErrUnsupported
}
