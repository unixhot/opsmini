package v1

import (
	"testing"

	"github.com/opsmini/opsmini/internal/config"
)

func TestAgentIsAllowed(t *testing.T) {
	h := &AgentHandler{cfg: &config.Agent{Commands: []string{"ps", "df", "docker", "systemctl"}}}

	cases := []struct {
		cmd  string
		want bool
	}{
		{"ps aux", true},
		{"df -h", true},
		{"docker ps", true},
		{"/usr/bin/ps aux", true}, // full path compatibility
		{"systemctl restart nginx", true},
		{"rm -rf /", false},
		{"cat /etc/passwd", false},
		{"", false},
		{"   ", false},
	}
	for _, c := range cases {
		if got := h.isAllowed(c.cmd); got != c.want {
			t.Errorf("isAllowed(%q) = %v, want %v", c.cmd, got, c.want)
		}
	}
}
