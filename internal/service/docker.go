package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// DockerService handles Docker container/image/volume/network management.
type DockerService struct {
	cli *client.Client
}

// NewDockerService creates a DockerService. The client is lazily connected; creation does
// not verify the daemon. An error is returned on the first API call if the daemon is unavailable.
func NewDockerService() (*DockerService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerService{cli: cli}, nil
}

// ContainerInfo is container information.
type ContainerInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Image      string  `json:"image"`
	State      string  `json:"state"`
	Status     string  `json:"status"`
	Ports      string  `json:"ports"`
	Created    int64   `json:"created"`
	CPUPercent float64 `json:"cpu_percent"` // CPU usage % (valid when running, otherwise 0)
	MemPercent float64 `json:"mem_percent"` // memory usage % (valid when running, otherwise 0)
	MemUsage   uint64  `json:"mem_usage"`   // memory usage in bytes
	MemLimit   uint64  `json:"mem_limit"`   // memory limit in bytes (0 = unlimited)
}

// ImageInfo is image information.
type ImageInfo struct {
	ID       string   `json:"id"`
	RepoTags []string `json:"repo_tags"`
	Size     int64    `json:"size"`
	Created  int64    `json:"created"`
}

// VolumeInfo is volume information.
type VolumeInfo struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

// NetworkInfo is network information.
type NetworkInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

// ctx returns a context with a unified timeout.
func (s *DockerService) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

// Containers lists containers (including CPU/memory usage of running containers).
func (s *DockerService) Containers() ([]ContainerInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]ContainerInfo, 0, len(res.Items))

	// concurrently fetch stats of running containers (each ~1s, concurrent to avoid serial accumulation)
	type stat struct {
		idx        int
		cpu, mem   float64
		memU, memL uint64
	}
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		stats = make([]stat, 0)
	)
	for i, c := range res.Items {
		if string(c.State) != "running" {
			continue
		}
		id := c.ID
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			cp, mp, memU, memL := s.containerStats(id)
			mu.Lock()
			stats = append(stats, stat{idx: i, cpu: cp, mem: mp, memU: memU, memL: memL})
			mu.Unlock()
		}(i, id)
	}
	wg.Wait()
	statMap := make(map[int]stat, len(stats))
	for _, st := range stats {
		statMap[st.idx] = st
	}

	for i, c := range res.Items {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		ci := ContainerInfo{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			State:   string(c.State),
			Status:  c.Status,
			Ports:   formatPorts(c.Ports),
			Created: c.Created,
		}
		if st, ok := statMap[i]; ok {
			ci.CPUPercent = st.cpu
			ci.MemPercent = st.mem
			ci.MemUsage = st.memU
			ci.MemLimit = st.memL
		}
		out = append(out, ci)
	}
	return out, nil
}

// containerStats fetches a single container's CPU/memory usage.
// IncludePreviousSample makes the daemon sample once (PreCPUStats) then again, directly yielding a delta to compute CPU%.
func (s *DockerService) containerStats(id string) (cpuPct, memPct float64, memUsage, memLimit uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := s.cli.ContainerStats(ctx, id, client.ContainerStatsOptions{
		Stream:                false,
		IncludePreviousSample: true,
	})
	if err != nil {
		return 0, 0, 0, 0
	}
	defer res.Body.Close()

	var st container.StatsResponse
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		return 0, 0, 0, 0
	}

	// CPU% = (cpuDelta / systemDelta) * cores * 100
	cpuDelta := float64(st.CPUStats.CPUUsage.TotalUsage) - float64(st.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(st.CPUStats.SystemUsage) - float64(st.PreCPUStats.SystemUsage)
	nCPU := st.CPUStats.OnlineCPUs
	if nCPU == 0 {
		nCPU = uint32(len(st.CPUStats.CPUUsage.PercpuUsage))
	}
	if nCPU == 0 {
		nCPU = 1
	}
	if sysDelta > 0 && cpuDelta > 0 {
		cpuPct = cpuDelta / sysDelta * float64(nCPU) * 100
		if cpuPct > 100*float64(nCPU) {
			cpuPct = 100 * float64(nCPU)
		}
	}

	// memory
	memUsage = st.MemoryStats.Usage
	memLimit = st.MemoryStats.Limit
	if memLimit > 0 {
		memPct = float64(memUsage) / float64(memLimit) * 100
	}
	return cpuPct, memPct, memUsage, memLimit
}

// StartContainer starts a container.
func (s *DockerService) StartContainer(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
	return err
}

// StopContainer stops a container.
func (s *DockerService) StopContainer(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.ContainerStop(ctx, id, client.ContainerStopOptions{})
	return err
}

// RestartContainer restarts a container.
func (s *DockerService) RestartContainer(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
	return err
}

// RemoveContainer removes a container (force).
func (s *DockerService) RemoveContainer(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
	return err
}

// Images lists images.
func (s *DockerService) Images() ([]ImageInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.cli.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]ImageInfo, 0, len(res.Items))
	for _, im := range res.Items {
		out = append(out, ImageInfo{
			ID:       im.ID,
			RepoTags: im.RepoTags,
			Size:     im.Size,
			Created:  im.Created,
		})
	}
	return out, nil
}

// RemoveImage removes an image (force).
func (s *DockerService) RemoveImage(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.ImageRemove(ctx, id, client.ImageRemoveOptions{Force: true})
	return err
}

// Volumes lists volumes.
func (s *DockerService) Volumes() ([]VolumeInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]VolumeInfo, 0, len(res.Items))
	for _, v := range res.Items {
		out = append(out, VolumeInfo{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
		})
	}
	return out, nil
}

// RemoveVolume removes a volume.
func (s *DockerService) RemoveVolume(name string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{})
	return err
}

// Networks lists networks.
func (s *DockerService) Networks() ([]NetworkInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]NetworkInfo, 0, len(res.Items))
	for _, n := range res.Items {
		out = append(out, NetworkInfo{
			ID:     n.ID,
			Name:   n.Name,
			Driver: n.Driver,
			Scope:  n.Scope,
		})
	}
	return out, nil
}

// RemoveNetwork removes a network.
func (s *DockerService) RemoveNetwork(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
	return err
}

// PullImage pulls an image (waits for completion).
func (s *DockerService) PullImage(ref string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	resp, err := s.cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()
	return resp.Wait(ctx)
}

// CreateVolume creates a volume.
func (s *DockerService) CreateVolume(name, driver string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Driver: driver})
	return err
}

// CreateNetwork creates a network (with optional subnet/gateway).
func (s *DockerService) CreateNetwork(name, driver, subnet, gateway string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	opts := client.NetworkCreateOptions{Driver: driver}
	if subnet != "" {
		prefix, err := netip.ParsePrefix(subnet)
		if err != nil {
			return fmt.Errorf("无效的子网 %s: %v", subnet, err)
		}
		cfg := network.IPAMConfig{Subnet: prefix}
		if gateway != "" {
			gw, err := netip.ParseAddr(gateway)
			if err != nil {
				return fmt.Errorf("无效的网关 %s: %v", gateway, err)
			}
			cfg.Gateway = gw
		}
		opts.IPAM = &network.IPAM{Config: []network.IPAMConfig{cfg}}
	}
	_, err := s.cli.NetworkCreate(ctx, name, opts)
	return err
}

// ContainerLogs fetches container logs (the most recent tail lines, up to 2000 lines, limited to 2MB).
// Non-TTY container log streams multiplex stdout/stderr (8-byte header), demultiplexed with stdcopy;
// TTY containers are raw streams.
func (s *DockerService) ContainerLogs(id string, tail int) (string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}
	reader, err := s.cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(tail),
	})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// check whether the container is a TTY, to decide whether to demultiplex
	isTTY := false
	if insp, err := s.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{}); err == nil &&
		insp.Container.Config != nil {
		isTTY = insp.Container.Config.Tty
	}

	limited := io.LimitReader(reader, 2<<20) // limit 2MB
	if isTTY {
		data, err := io.ReadAll(limited)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, limited); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ExecShell starts an interactive shell exec inside a container (default /bin/sh, present in almost all Linux containers).
// It returns the exec session ID and the hijacked connection after attach (resp.Conn for writing input, resp.Reader for reading output).
func (s *DockerService) ExecShell(id string, cols, rows uint) (string, client.ExecAttachResult, error) {
	// Note: s.ctx() (15s timeout) must not be used. exec attach hijacks the underlying connection, and
	// otelhttp's RoundTrip uses context.AfterFunc to close the connection when the ctx is cancelled — if a
	// timed ctx is used and cancelled when ExecShell returns, the interactive terminal disconnects
	// immediately with no output. So use Background.
	//
	// Note: exec start hijacks the connection successfully even if the target binary does not exist (e.g.
	// /bin/bash), then returns "OCI runtime exec failed" over the stream instead of making ExecAttach return
	// an error, so the "bash failed fall back to sh" idea cannot work — ExecAttach returns a nil error even
	// for a non-existent bash. So we always fix /bin/sh (the most universal); users can run bash themselves
	// inside the terminal if needed.
	ctx := context.Background()
	return s.execAttach(ctx, id, []string{"/bin/sh"}, cols, rows)
}

// execAttach creates and attaches an interactive exec.
func (s *DockerService) execAttach(ctx context.Context, id string, cmd []string, cols, rows uint) (string, client.ExecAttachResult, error) {
	exec, err := s.cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
		Cmd:          cmd,
		ConsoleSize:  client.ConsoleSize{Height: rows, Width: cols},
	})
	if err != nil {
		return "", client.ExecAttachResult{}, err
	}
	resp, err := s.cli.ExecAttach(ctx, exec.ID, client.ExecAttachOptions{
		TTY:         true,
		ConsoleSize: client.ConsoleSize{Height: rows, Width: cols},
	})
	if err != nil {
		return "", client.ExecAttachResult{}, err
	}
	return exec.ID, resp, nil
}

// ExecResize resizes an exec terminal.
func (s *DockerService) ExecResize(execID string, cols, rows uint) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.cli.ExecResize(ctx, execID, client.ExecResizeOptions{Height: rows, Width: cols})
	return err
}

// ContainerNetwork is container network endpoint information.
type ContainerNetwork struct {
	Name       string `json:"name"`
	IPAddress  string `json:"ip_address"`
	Gateway    string `json:"gateway"`
	MacAddress string `json:"mac_address"`
}

// ContainerMount is container mount information (volume or bind mount).
type ContainerMount struct {
	Type        string `json:"type"`        // volume / bind / tmpfs
	Name        string `json:"name"`        // volume name (empty for bind)
	Source      string `json:"source"`      // host path / volume storage path
	Destination string `json:"destination"` // path inside the container
	Mode        string `json:"mode"`        // rw / ro
}

// ContainerDetail is container detail (basic info + network + storage volumes).
type ContainerDetail struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Image         string             `json:"image"`
	State         string             `json:"state"`
	Status        string             `json:"status"`
	Created       string             `json:"created"` // ISO time string
	StartedAt     string             `json:"started_at"`
	Platform      string             `json:"platform"`
	Command       string             `json:"command"`
	WorkingDir    string             `json:"working_dir"`
	RestartPolicy string             `json:"restart_policy"`
	Env           []string           `json:"env"`
	Labels        map[string]string  `json:"labels"`
	Networks      []ContainerNetwork `json:"networks"`
	PortBindings  []string           `json:"port_bindings"`
	Mounts        []ContainerMount   `json:"mounts"`
}

// Inspect returns container detail (basic info + network + storage volumes).
func (s *DockerService) Inspect(id string) (*ContainerDetail, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}
	c := res.Container

	d := &ContainerDetail{
		ID:           c.ID,
		Name:         strings.TrimPrefix(c.Name, "/"),
		Image:        c.Image,
		Created:      c.Created,
		Platform:     c.Platform,
		Labels:       map[string]string{},
		Networks:     []ContainerNetwork{},
		PortBindings: []string{},
		Mounts:       []ContainerMount{},
	}
	if c.State != nil {
		d.State = string(c.State.Status)
		d.StartedAt = c.State.StartedAt
	}
	if c.Config != nil {
		// use Config.Image (the friendly image name the user launched with, e.g. registry.../guacd:1.5.0);
		// c.Image is a sha256 digest, which is not friendly.
		if c.Config.Image != "" {
			d.Image = c.Config.Image
		}
		d.Command = strings.Join(c.Config.Cmd, " ")
		if c.Config.Entrypoint != nil && len(c.Config.Entrypoint) > 0 {
			d.Command = strings.Join(c.Config.Entrypoint, " ") + " " + d.Command
		}
		d.WorkingDir = c.Config.WorkingDir
		d.Env = c.Config.Env
		if c.Config.Labels != nil {
			d.Labels = c.Config.Labels
		}
	}
	if c.HostConfig != nil && c.HostConfig.RestartPolicy.Name != "" {
		d.RestartPolicy = string(c.HostConfig.RestartPolicy.Name)
	}

	// network
	if c.NetworkSettings != nil {
		for name, ep := range c.NetworkSettings.Networks {
			cn := ContainerNetwork{
				Name:      name,
				IPAddress: ep.IPAddress.String(),
				Gateway:   ep.Gateway.String(),
			}
			if mac, err := ep.MacAddress.MarshalText(); err == nil {
				cn.MacAddress = string(mac)
			}
			d.Networks = append(d.Networks, cn)
		}
		seenPorts := map[string]bool{}
		for port, bindings := range c.NetworkSettings.Ports {
			for _, b := range bindings {
				host := b.HostIP.String()
				if host == "" || host == "0.0.0.0" || host == "::" {
					host = ""
				}
				if host != "" {
					host += ":"
				}
				pb := host + b.HostPort + "->" + port.String()
				if !seenPorts[pb] {
					seenPorts[pb] = true
					d.PortBindings = append(d.PortBindings, pb)
				}
			}
		}
	}

	// storage volumes
	for _, m := range c.Mounts {
		d.Mounts = append(d.Mounts, ContainerMount{
			Type:        string(m.Type),
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
		})
	}

	return d, nil
}

// formatPorts formats port mappings as "host:container/tcp, ...".
func formatPorts(ports []container.PortSummary) string {
	if len(ports) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.PublicPort > 0 {
			parts = append(parts, fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, p.Type))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	return strings.Join(parts, ", ")
}
