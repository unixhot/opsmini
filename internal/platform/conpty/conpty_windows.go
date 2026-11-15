//go:build windows

package conpty

import (
	"context"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)


type winConsole struct {
	hpc    windows.Handle
	in     *os.File // writes to the shell's stdin
	out    *os.File // reads the shell's stdout
	proc   windows.Handle
	thread windows.Handle
	once   sync.Once
}

func start(_ context.Context, shell string, env []string, cols, rows uint16) (Console, error) {
	// Two anonymous pipes: the conpty reads one end (shell stdin) and writes the
	// other (shell stdout). We keep the opposite ends. os.Pipe is used rather than
	// CreatePipe so the handles are exactly what os.File expects.
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		_ = inRead.Close()
		_ = inWrite.Close()
		return nil, err
	}

	var hpc windows.Handle
	coord := windows.Coord{X: int16(cols), Y: int16(rows)}
	if cerr := windows.CreatePseudoConsole(coord,
		windows.Handle(inRead.Fd()), windows.Handle(outWrite.Fd()), 0, &hpc); cerr != nil {
		_ = inRead.Close()
		_ = inWrite.Close()
		_ = outRead.Close()
		_ = outWrite.Close()
		return nil, cerr
	}
	// The conpty holds its own references now; the copies it was handed must go
	// or the pipes never reach EOF.
	_ = inRead.Close()
	_ = outWrite.Close()

	attrs, aerr := windows.NewProcThreadAttributeList(1)
	if aerr != nil {
		windows.ClosePseudoConsole(hpc)
		_ = inWrite.Close()
		_ = outRead.Close()
		return nil, aerr
	}
	defer attrs.Delete()

	// The attribute value is the HPCON itself, not a pointer to it: Update copies
	// sizeof(handle) bytes starting at the given address, and unsafe.Pointer(hpc)
	// is the address whose bytes are the handle value.
	if uerr := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(hpc), unsafe.Sizeof(hpc)); uerr != nil {
		windows.ClosePseudoConsole(hpc)
		_ = inWrite.Close()
		_ = outRead.Close()
		return nil, uerr
	}

	// windows.StartupInfoEx (x/sys) rather than a hand-rolled STARTUPINFOEXW:
	// the layout and the cb accounting are what CreateProcess validates when
	// EXTENDED_STARTUPINFO_PRESENT is set, and getting them wrong silently drops
	// the attribute list — the child then attaches to the parent console.
	si := &windows.StartupInfoEx{}
	si.Flags = windows.STARTF_USESTDHANDLES

	appName, err := windows.UTF16PtrFromString(shell)
	if err != nil {
		windows.ClosePseudoConsole(hpc)
		_ = inWrite.Close()
		_ = outRead.Close()
		return nil, err
	}
	// lpCommandLine repeats argv[0] quoted; without it CreateProcessW splits the
	// path at the first space and fails with ERROR_INVALID_PARAMETER.
	cmdline, err := windows.UTF16PtrFromString(`"` + shell + `"`)
	if err != nil {
		windows.ClosePseudoConsole(hpc)
		_ = inWrite.Close()
		_ = outRead.Close()
		return nil, err
	}

	si.ProcThreadAttributeList = attrs.List()
	si.Cb = uint32(unsafe.Sizeof(*si))

	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	cerr := windows.CreateProcess(appName, cmdline, nil, nil, false, flags, nil, nil, &si.StartupInfo, &pi)
	if cerr != nil {
		windows.ClosePseudoConsole(hpc)
		_ = inWrite.Close()
		_ = outRead.Close()
		return nil, cerr
	}

	return &winConsole{
		hpc:    hpc,
		in:     inWrite,
		out:    outRead,
		proc:   pi.Process,
		thread: pi.Thread,
	}, nil
}

func (c *winConsole) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *winConsole) Write(p []byte) (int, error) { return c.in.Write(p) }

func (c *winConsole) Resize(cols, rows uint16) error {
	return windows.ResizePseudoConsole(c.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (c *winConsole) Close() error {
	c.once.Do(func() {
		_ = c.in.Close()
		_ = c.out.Close()
		windows.ClosePseudoConsole(c.hpc)
		if c.proc != 0 {
			_ = windows.TerminateProcess(c.proc, 1)
			_ = windows.CloseHandle(c.proc)
		}
		if c.thread != 0 {
			_ = windows.CloseHandle(c.thread)
		}
	})
	return nil
}

func defaultShell() string {
	if p, ok := lookPath("pwsh.exe"); ok {
		return p
	}
	if p, ok := lookPath("powershell.exe"); ok {
		return p
	}
	comspec := os.Getenv("COMSPEC")
	if comspec == "" {
		comspec = `C:\Windows\System32\cmd.exe`
	}
	return comspec
}

// shellEnv returns the environment to hand the shell. The process-creation path
// currently inherits the panel's environment (lpEnvironment = NULL), so this is
// only used by callers that pass env explicitly.
func shellEnv(string) []string { return os.Environ() }
