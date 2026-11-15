//go:build windows

package screen

import (
	"errors"
	"unsafe"
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseMove       = 0x0001
	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseMiddleDown = 0x0020
	mouseMiddleUp   = 0x0040
	mouseWheel      = 0x0800
	mouseAbsolute   = 0x8000
	mouseVirtualDesk = 0x4000

	keyEventKeyUp = 0x0002
)

var procSendInput = user32.NewProc("SendInput")

// rawInput mirrors the Win32 INPUT struct (40 bytes on amd64).
type rawInput struct {
	Type uint32
	_    uint32
	Data [32]byte
}

// mouseInputData mirrors MOUSEINPUT, padded to the union size.
type mouseInputData struct {
	Dx        int32
	Dy        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	_         uint32
	ExtraInfo uintptr
}

// keyInputData mirrors KEYBDINPUT.
type keyInputData struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	_         uint32
	ExtraInfo uintptr
}

type winInput struct{}

func newInput() Input { return &winInput{} }

func send(inraw []rawInput) error {
	if len(inraw) == 0 {
		return nil
	}
	r, _, err := procSendInput.Call(
		uintptr(len(inraw)),
		uintptr(unsafe.Pointer(&inraw[0])),
		unsafe.Sizeof(inraw[0]),
	)
	if r == 0 {
		return errors.New("SendInput 被拒绝（需要与当前会话处于同一 window station）")
	}
	return err
}

// MoveTo maps 0..1 onto the virtual desktop in the 0..65535 space that
// MOUSEEVENTF_ABSOLUTE expects.
func (w *winInput) MoveTo(x, y float64) error {
	clamp := func(v float64) int32 {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		return int32(v * 65535)
	}
	m := mouseInputData{
		Dx:    clamp(x),
		Dy:    clamp(y),
		Flags: mouseMove | mouseAbsolute | mouseVirtualDesk,
	}
	var in rawInput
	in.Type = inputMouse
	*(*mouseInputData)(unsafe.Pointer(&in.Data[0])) = m
	return send([]rawInput{in})
}

func (w *winInput) Button(button int, down bool) error {
	var flag uint32
	switch button {
	case 2:
		if down {
			flag = mouseRightDown
		} else {
			flag = mouseRightUp
		}
	case 3:
		if down {
			flag = mouseMiddleDown
		} else {
			flag = mouseMiddleUp
		}
	default:
		if down {
			flag = mouseLeftDown
		} else {
			flag = mouseLeftUp
		}
	}
	var in rawInput
	in.Type = inputMouse
	*(*mouseInputData)(unsafe.Pointer(&in.Data[0])) = mouseInputData{Flags: flag}
	return send([]rawInput{in})
}

func (w *winInput) Wheel(delta int) error {
	var in rawInput
	in.Type = inputMouse
	*(*mouseInputData)(unsafe.Pointer(&in.Data[0])) = mouseInputData{
		MouseData: uint32(int32(delta * 120)), // one notch is WHEEL_DELTA
		Flags:     mouseWheel,
	}
	return send([]rawInput{in})
}

func (w *winInput) Key(vk uint16, down bool) error {
	var flags uint32
	if !down {
		flags = keyEventKeyUp
	}
	var in rawInput
	in.Type = inputKeyboard
	*(*keyInputData)(unsafe.Pointer(&in.Data[0])) = keyInputData{Vk: vk, Scan: uint16(vkKeyboardScan(vk)), Flags: flags}
	return send([]rawInput{in})
}

// vkKeyboardScan asks the keyboard layout for the scan code that matches the
// virtual key. Using the physical scan code keeps key events working for
// software that reads the keyboard state instead of the message queue.
func vkKeyboardScan(vk uint16) uint32 {
	r, _, _ := procMapVirtualKey.Call(uintptr(vk), 0 /* MAPVK_VK_TO_VSC */)
	return uint32(r)
}
