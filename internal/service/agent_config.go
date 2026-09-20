package service

import (
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
	settings  *SettingService
	mgr       *agent.Manager
	cfg       config.Agent // fallback from config.yaml
	panelPort int32
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
