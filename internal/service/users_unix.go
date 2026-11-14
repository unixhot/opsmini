//go:build !windows

package service

import (
	"os"
	"sort"
	"strings"
)

// listUsers reads /etc/passwd and returns the system user list, distinguishing
// loggable accounts from service accounts by their shell.
func listUsers() []UserAccount {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	var out []UserAccount
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		shell := parts[6]
		out = append(out, UserAccount{
			Username:  parts[0],
			UID:       parts[2],
			GID:       parts[3],
			Home:      parts[5],
			Shell:     shell,
			Loginable: isLoginShell(shell),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out
}

// listGroups reads /etc/group and returns the user group list.
func listGroups() []GroupAccount {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return nil
	}
	var out []GroupAccount
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		var members []string
		if parts[3] != "" {
			members = strings.Split(parts[3], ",")
		}
		out = append(out, GroupAccount{Groupname: parts[0], GID: parts[2], Members: members})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GID < out[j].GID })
	return out
}
