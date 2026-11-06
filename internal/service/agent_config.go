package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/config"
)

const (
	// settingAgentServerAddr is the panel-overridable OpsAnt "OpsMini Server" address.
	settingAgentServerAddr = "agent_server_addr"
	// settingAgentServerToken is the panel-overridable token for the OpsAnt Server.
	settingAgentServerToken = "agent_server_token"
)

// AgentConfigService manages the OpsAnt connection configuration. The effective
// config is: panel settings (database) override config.yaml; when the setting is
// absent, config.yaml's agent section is used as the fallback.
type AgentConfigService struct {
	settings *SettingService
	mgr      *agent.Manager
	cfg      config.Agent // fallback from config.yaml
	panelPort int32
	// probeMu serializes connectivity tests: they are operator-triggered and
	// short, and running two at once only produces confusing interleaved
	// registrations on the Server side.
	probeMu sync.Mutex
}

// NewAgentConfigService creates an AgentConfigService.
func NewAgentConfigService(settings *SettingService, mgr *agent.Manager, cfg config.Agent, panelPort int32) *AgentConfigService {
	return &AgentConfigService{settings: settings, mgr: mgr, cfg: cfg, panelPort: panelPort}
}

// effective returns the effective (addr, token), with panel settings taking precedence.
func (s *AgentConfigService) effective() (string, string) {
	addr, token := s.cfg.ServerAddr, s.cfg.ServerToken
	if kv, err := s.settings.List(); err == nil {
		if v, ok := kv[settingAgentServerAddr]; ok {
			addr = v
		}
		if v, ok := kv[settingAgentServerToken]; ok {
			token = v
		}
	}
	return addr, token
}

// Current returns the current configuration and running state.
func (s *AgentConfigService) Current() (addr, token string, running bool) {
	addr, token = s.effective()
	return addr, token, s.mgr.Running()
}

// Init starts the Agent with the effective config (without persisting settings),
// called once on process startup.
func (s *AgentConfigService) Init() {
	addr, token := s.effective()
	s.apply(addr, token)
}

// Save persists the configuration to settings and applies it immediately
// (reconnecting the Agent). An empty addr disconnects the Agent.
func (s *AgentConfigService) Save(addr, token string) error {
	if err := s.settings.Set(map[string]string{
		settingAgentServerAddr:  addr,
		settingAgentServerToken: token,
	}); err != nil {
		return err
	}
	s.apply(addr, token)
	return nil
}

func (s *AgentConfigService) apply(addr, token string) {
	heartbeat := time.Duration(s.cfg.HeartbeatSeconds) * time.Second
	s.mgr.Start(agent.Config{
		ServerAddr: addr,
		Token:      token,
		PanelPort:  s.panelPort,
		Heartbeat:  heartbeat,
	})
}

// Probe tests whether the given address + token can actually register with the
// OpsAnt Server, without persisting anything. It answers the question the
// settings page could not: "is what I typed usable?", with the failure reason
// split into address / network / token.
//
// The probe registers under a fresh throwaway identity (<hostname>-probe-xxxx)
// rather than reusing the panel's hostname. The Server keys agents by identity
// and kicks the previous stream when the same one registers again, so reusing
// it would drop the production link on every test. A unique name also keeps the
// probe away from the double-close panic that unpatched Servers hit when a host
// re-registers (fixed in opsant afd7464, but not every deployment is updated).
//
// The live agent is not touched, and the probe entry disappears on its own when
// the stream closes.
func (s *AgentConfigService) Probe(ctx context.Context, addr, token string) agent.ProbeResult {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()

	hostname, version, instanceID := s.mgr.Identity()
	// 4 hex chars are enough to keep back-to-back tests distinct; the value only
	// needs to be unique within the few seconds a probe lives.
	suffix := fmt.Sprintf("%04x", uint16(time.Now().UnixNano()))
	return agent.ProbeOnce(ctx, agent.Config{
		ServerAddr: addr,
		Token:      token,
		PanelPort:  s.panelPort,
	}, hostname+"-probe-"+suffix, version, instanceID+"-probe-"+suffix)
}
