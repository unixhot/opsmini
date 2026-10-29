package router

import (
	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/logpipe"
	"github.com/opsmini/opsmini/internal/plugin"
)

// Adapters between the runtime subsystems and the narrow interfaces the Agent
// expects. They exist so the agent package does not import logpipe or plugin:
// the wire protocol should describe actions ("apply this task"), not the shape of
// the local implementation, otherwise every internal refactor becomes a protocol
// change.

// agentLogAdapter exposes the log pipeline to platform tasks.
type agentLogAdapter struct{ mgr *logpipe.Manager }

func (a *agentLogAdapter) ApplyTask(filename string, content []byte, sha string, strict bool) (string, []string, error) {
	return a.mgr.ApplyTask(filename, content, sha, strict)
}

func (a *agentLogAdapter) RemoveTask(filename string) error { return a.mgr.RemoveTask(filename) }

func (a *agentLogAdapter) ReloadNow() { a.mgr.ReloadNow() }

func (a *agentLogAdapter) TaskByFile(filename string) (agent.LogState, bool) {
	st, ok := a.mgr.TaskByFile(filename)
	if !ok {
		return agent.LogState{}, false
	}
	cfg := a.mgr.Config()
	return agent.LogState{
		TaskID:        st.TaskID,
		State:         st.State,
		FilesOpen:     int32(st.FilesOpen),
		BytesRead:     st.BytesRead,
		EventsSent:    st.EventsSent,
		EventsDropped: st.EventsDropped,
		LastError:     st.LastError,
		Output:        cfg.Output.Type,
	}, true
}

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
