package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// skillHubBase is the SkillHub API base URL.
const skillHubBase = "https://api.skillhub.cn"

// SkillInfo is skill information.
type SkillInfo struct {
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Installed     bool   `json:"installed"`
	Enabled       bool   `json:"enabled"`
	Downloads     int64  `json:"downloads"`
	Installs      int64  `json:"installs"`
	Stars         int64  `json:"stars"`
	Version       string `json:"version"`
	IconURL       string `json:"icon_url"`
	Verified      bool   `json:"verified"`
}

// SkillService handles skill management: skills are stored as directories (one directory per skill, containing SKILL.md).
// It integrates with the SkillHub REST API (search/download skills).
type SkillService struct {
	dataDir func() string    // data directory provider (read from settings, configurable)
	client  *http.Client
}

// NewSkillService creates a SkillService.
func NewSkillService(dataDir func() string) *SkillService {
	return &SkillService{
		dataDir: dataDir,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// root returns the skill root directory.
func (s *SkillService) root() string {
	dir := "/var/lib/opsmini/data/skills"
	if s.dataDir != nil {
		if d := s.dataDir(); d != "" {
			dir = filepath.Join(d, "skills")
		}
	}
	return dir
}

// skillDir returns the directory of a given skill.
func (s *SkillService) skillDir(name string) string {
	return filepath.Join(s.root(), filepath.Base(name))
}

var (
	nameRe = regexp.MustCompile(`(?m)^name:\s*["']?([^"'\r\n]+)`)
	descRe = regexp.MustCompile(`(?m)^description:\s*["']?([^"'\r\n]+)`)
)

// parseSkillMD parses name/description from SKILL.md content.
func parseSkillMD(content string) (name, desc string) {
	if m := nameRe.FindStringSubmatch(content); len(m) > 1 {
		name = strings.TrimSpace(m[1])
	}
	if m := descRe.FindStringSubmatch(content); len(m) > 1 {
		desc = strings.TrimSpace(m[1])
	}
	return name, desc
}

// readSkill reads the SKILL.md info of a skill directory.
func (s *SkillService) readSkill(dir string) SkillInfo {
	base := filepath.Base(dir)
	// Slug records the directory name (on install the directory name = slug), so Search can match the "installed" status.
	info := SkillInfo{Name: base, Slug: base}
	if b, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err == nil {
		name, desc := parseSkillMD(string(b))
		if name != "" {
			info.Name = name
		}
		info.Description = desc
	}
	return info
}

// List lists installed skills (scans the skill directory).
func (s *SkillService) List() []SkillInfo {
	root := s.root()
	entries, err := os.ReadDir(root)
	if err != nil {
		return []SkillInfo{}
	}
	out := make([]SkillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info := s.readSkill(filepath.Join(root, e.Name()))
		info.Installed = true
		info.Enabled = true
		// disabled if a .disabled marker file exists
		if _, err := os.Stat(filepath.Join(root, e.Name(), ".disabled")); err == nil {
			info.Enabled = false
		}
		out = append(out, info)
	}
	return out
}

// skillHubResp is the SkillHub search API response.
type skillHubResp struct {
	Code int `json:"code"`
	Data struct {
		Skills []struct {
			Name          string `json:"name"`
			Slug          string `json:"slug"`
			Description   string `json:"description"`
			DescriptionZh string `json:"description_zh"`
			Category      string `json:"category"`
			Downloads     int64  `json:"downloads"`
			Installs      int64  `json:"installs"`
			Stars         int64  `json:"stars"`
			Version       string `json:"version"`
			IconURL       string `json:"iconUrl"`
			Verified      bool   `json:"verified"`
		} `json:"skills"`
	} `json:"data"`
}

// Search calls the SkillHub API to search skills (no auth required).
func (s *SkillService) Search(keyword, category, sortBy string, page, pageSize int) ([]SkillInfo, error) {
	q := url.Values{}
	if keyword != "" {
		q.Set("keyword", keyword)
	}
	if category != "" {
		q.Set("category", category)
	}
	if sortBy != "" {
		q.Set("sortBy", sortBy)
	} else {
		q.Set("sortBy", "score")
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	} else {
		q.Set("pageSize", "20")
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}

	resp, err := s.client.Get(skillHubBase + "/api/skills?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("SkillHub 请求失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SkillHub 返回 HTTP %d", resp.StatusCode)
	}

	var r skillHubResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("SkillHub 响应解析失败：%v", err)
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("SkillHub 错误码 %d", r.Code)
	}

	// set of installed skill names (used to mark installed)
	installed := map[string]bool{}
	for _, sk := range s.List() {
		installed[sk.Name] = true
		installed[sk.Slug] = true
	}

	out := make([]SkillInfo, 0, len(r.Data.Skills))
	for _, sk := range r.Data.Skills {
		desc := sk.DescriptionZh
		if desc == "" {
			desc = sk.Description
		}
		out = append(out, SkillInfo{
			Name:        sk.Name,
			Slug:        sk.Slug,
			Description: desc,
			Category:    sk.Category,
			Downloads:   sk.Downloads,
			Installs:    sk.Installs,
			Stars:       sk.Stars,
			Version:     sk.Version,
			IconURL:     sk.IconURL,
			Verified:    sk.Verified,
			Installed:   installed[sk.Name] || installed[sk.Slug],
		})
	}
	return out, nil
}

// Catalog returns recommended skills (SkillHub sorted by score, no keyword).
func (s *SkillService) Catalog() ([]SkillInfo, error) {
	return s.Search("", "", "score", 1, 20)
}

// Install downloads and installs a skill from SkillHub (by slug).
func (s *SkillService) Install(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return errors.New("技能标识不能为空")
	}
	resp, err := s.client.Get(skillHubBase + "/api/v1/download?slug=" + url.QueryEscape(slug))
	if err != nil {
		return fmt.Errorf("下载技能失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载技能失败：HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // limit 32MB
	if err != nil {
		return fmt.Errorf("读取技能包失败：%v", err)
	}
	return s.installZip(data, slug)
}

// Create creates a skill (generates a SKILL.md template).
func (s *SkillService) Create(name, description string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("技能名不能为空")
	}
	dir := s.skillDir(name)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("技能 %s 已存在", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := skillMDSkeleton(name, description)
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
}

// Delete deletes a skill directory.
func (s *SkillService) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return errors.New("非法技能名")
	}
	dir := s.skillDir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return errors.New("技能不存在")
	}
	return os.RemoveAll(dir)
}

// Toggle toggles a skill's enabled/disabled state, returning the resulting enabled state.
func (s *SkillService) Toggle(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return false, errors.New("非法技能名")
	}
	dir := s.skillDir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return false, errors.New("技能不存在")
	}
	marker := filepath.Join(dir, ".disabled")
	if _, err := os.Stat(marker); err == nil {
		// currently disabled -> remove marker to enable
		if err := os.Remove(marker); err != nil {
			return false, err
		}
		return true, nil
	}
	// currently enabled -> create marker to disable
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		return false, err
	}
	return false, nil
}

// InstallFromZip installs a skill from an uploaded zip (directory name uses SKILL.md's name). The zip must contain SKILL.md.
func (s *SkillService) InstallFromZip(data []byte) error {
	return s.installZip(data, "")
}

// installZip extracts and installs a skill. When slug is non-empty, slug is preferred as the directory name
// (so it matches the SkillHub search slug; otherwise the frontend "installed" status will not match, causing
// duplicate installs); when slug is empty (upload scenario), it falls back to SKILL.md's name.
func (s *SkillService) installZip(data []byte, slug string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return errors.New("无效的 zip 文件：" + err.Error())
	}

	// find SKILL.md first to determine the skill name
	var skillName, skillDesc string
	var mdPath string
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "SKILL.md" {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			content, _ := io.ReadAll(io.LimitReader(rc, 64<<10))
			rc.Close()
			skillName, skillDesc = parseSkillMD(string(content))
			mdPath = f.Name
			break
		}
	}
	if mdPath == "" {
		return errors.New("zip 内缺少 SKILL.md")
	}
	if slug != "" {
		// one-click install: directory name uses slug, ensuring consistency with the SkillHub search slug (the name in SKILL.md may differ from slug)
		skillName = sanitizeSlug(slug)
	} else {
		if skillName == "" {
			skillName = filepath.Base(filepath.Dir(mdPath))
			if skillName == "." || skillName == "" {
				skillName = "uploaded-skill"
			}
		}
		skillName = sanitizeSlug(skillName)
	}

	dir := s.skillDir(skillName)
	if _, err := os.Stat(dir); err == nil {
		_ = os.RemoveAll(dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for _, f := range zr.File {
		rel := filepath.Clean(f.Name)
		if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		if mdDir := filepath.Dir(mdPath); mdDir != "." && mdDir != "/" {
			if rel == mdDir || strings.HasPrefix(rel, mdDir+"/") {
				rel = strings.TrimPrefix(rel, mdDir+"/")
			}
		}
		if rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		target := filepath.Join(dir, rel)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
	}
	_ = skillDesc
	return nil
}

// skillMDSkeleton generates a skill SKILL.md template.
func skillMDSkeleton(name, desc string) string {
	if desc == "" {
		desc = name + " 技能"
	}
	return fmt.Sprintf(`---
name: %s
description: %s
agent_created: true
---

# %s

## 概述

%s

## 使用场景

待补充：描述该技能在什么情况下使用。

## 工作流程

待补充：描述执行该技能的具体步骤。
`, name, desc, name, desc)
}

// sanitizeSlug converts a name into a valid directory name.
func sanitizeSlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "skill"
	}
	return s
}
