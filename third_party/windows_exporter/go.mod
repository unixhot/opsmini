// Vendored copy of prometheus-community/windows_exporter v0.32.1 collector
// implementation. Kept as its own module because it uses Go 1.27 language
// features while the panel itself still builds with 1.26.
//
// Original: https://github.com/prometheus-community/windows_exporter
// Licence:  MIT (see LICENSE-MIT in this directory)
module github.com/opsmini/opsmini/third_party/windows_exporter

go 1.27.0

require (
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/prometheus/client_golang v1.25.0
	github.com/prometheus/client_model v0.6.3
	github.com/prometheus/common v0.72.0
	golang.org/x/sys v0.49.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
)
