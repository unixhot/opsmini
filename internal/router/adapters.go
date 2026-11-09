package router

import (
	"github.com/opsmini/opsmini/internal/plugin"
)

// Adapters between the runtime subsystems and the narrow interfaces the Agent
// expects. They exist so the agent package does not import plugin: the wire
// protocol should describe actions ("install this package"), not the shape of the
// local implementation, otherwise every internal refactor becomes a protocol
// change.

// agentPluginAdapter exposes the plugin manager to platform tasks.
type agentPluginAdapter struct{ mgr *plugin.Manager }

func (a *agentPluginAdapter) Install(pkgPath, wantSHA, sig string) (string, string, error) {
	m, err := a.mgr.Install(pkgPath, wantSHA, sig)
	if err != nil {
		return "", "", err
	}
	return m.ID, m.Version, nil
}

func (a *agentPluginAdapter) Remove(id string, purge bool) error { return a.mgr.Remove(id, purge) }

func (a *agentPluginAdapter) Enable(id string, on bool) error { return a.mgr.Enable(id, on) }

func (a *agentPluginAdapter) Restart(id string) error { return a.mgr.Restart(id) }

func (a *agentPluginAdapter) State(id string) (string, string, string, bool) {
	return a.mgr.State(id)
}
