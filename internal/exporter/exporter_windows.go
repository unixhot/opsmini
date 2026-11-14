//go:build windows

package exporter

import wx "github.com/opsmini/opsmini/third_party/windows_exporter"

// Available reports whether the windows_exporter-compatible collectors are
// compiled in. They only exist on Windows.
func Available() bool { return true }

// DefaultCollectors is upstream's [defaults] set for v0.32.1.
func DefaultCollectors() string { return wx.DefaultCollectors }

// Names lists every supported collector name.
func Names() []string { return wx.Names() }

// ParseList expands a comma-separated list, replacing "[defaults]".
func ParseList(s string) []string { return wx.ParseList(s) }

// New builds the named collectors. An unknown name returns an error, which the
// caller turns into HTTP 400 the way the standalone exporter does.
func New(names []string) (Collector, error) { return wx.New(names) }
