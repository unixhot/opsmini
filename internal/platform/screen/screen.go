// Package screen captures the interactive desktop as JPEG frames for the
// graphical terminal.
//
// On Windows the capture uses GDI BitBlt rather than DXGI Desktop Duplication:
// BitBlt needs no COM interface setup, no output enumeration and no
// hardware-specific handling, and at the frame rates a browser terminal needs
// (1-5 fps) the difference is not visible. DXGI becomes worthwhile only when
// this grows into a full remote-desktop feature.
package screen

import "context"

// Capture returns one JPEG frame of the primary display, scaled down so its
// width does not exceed maxWidth (0 or negative means no scaling). quality is
// the JPEG quality, 1-100.
func Capture(ctx context.Context, maxWidth, quality int) (jpeg []byte, width, height int, err error) {
	return capture(ctx, maxWidth, quality)
}
