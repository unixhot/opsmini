//go:build windows

package service

import (
	"sort"
	"strings"
)

// listUsers returns local accounts via the LocalAccounts module.
//
// PowerShell rather than /etc/passwd obviously, and rather than NetUserEnum,
// because Get-LocalUser reports the enabled flag and last logon in one call and
// its output is structured JSON — parsing `net user` means parsing localised
// column headers, which breaks on a Chinese Windows.
func listUsers() []UserAccount {
	var raw []struct {
		Name        string `json:"Name"`
		Enabled     bool   `json:"Enabled"`
		Description string `json:"Description"`
		SID         string `json:"SID"`
	}
	// SID is a structured type that ConvertTo-Json expands into an object; only
	// its Value is a string, so the projection has to be explicit.
	const q = `Get-LocalUser | ForEach-Object { [pscustomobject]@{ Name=$_.Name; Enabled=$_.Enabled; Description=$_.Description; SID=$_.SID.Value } }`
	if err := psJSON(q, &raw); err != nil {
		return nil
	}
	out := make([]UserAccount, 0, len(raw))
	for _, u := range raw {
		out = append(out, UserAccount{
			Username: u.Name,
			// Windows has no uid/gid; the SID is the closest stable identifier and
			// is what an operator actually needs when granting rights.
			UID:       u.SID,
			GID:       "",
			Home:      "",
			Shell:     "",
			Loginable: u.Enabled,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

// listGroups returns local groups with their members.
func listGroups() []GroupAccount {
	var raw []struct {
		Name    string `json:"Name"`
		SID     string `json:"SID"`
		Members string `json:"Members"`
	}
	const q = `Get-LocalGroup | ForEach-Object { [pscustomobject]@{ Name=$_.Name; SID=$_.SID.Value; Members=(($_.Members | ForEach-Object { $_.Name }) -join ',') } }`
	if err := psJSON(q, &raw); err != nil {
		return nil
	}
	out := make([]GroupAccount, 0, len(raw))
	for _, g := range raw {
		var members []string
		if g.Members != "" {
			members = strings.Split(g.Members, ",")
		}
		out = append(out, GroupAccount{Groupname: g.Name, GID: g.SID, Members: members})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Groupname < out[j].Groupname })
	return out
}
