//go:build !windows

package screen

import "github.com/opsmini/opsmini/internal/platform/sys"

// winInput is unused off Windows; the stub keeps the constructor type-checked.
type stubInput struct{}

func newInput() Input { return stubInput{} }

func (stubInput) MoveTo(float64, float64) error { return sys.ErrUnsupported }
func (stubInput) Button(int, bool) error        { return sys.ErrUnsupported }
func (stubInput) Wheel(int) error               { return sys.ErrUnsupported }
func (stubInput) Key(uint16, bool) error        { return sys.ErrUnsupported }
