package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// officialManifestURL is the official website app store template manifest URL.
const officialManifestURL = "https://www.opsmini.com/apps/manifest.json"

// OfficialManifest is the official template manifest.
type OfficialManifest struct {
	Apps []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Category    string `json:"category"`
		DownloadURL string `json:"download_url"`
	} `json:"apps"`
}

// templateRoot returns the template file root directory (holds logo.png and files/).
func (s *AppStoreService) templateRoot() string {
	dir := "/var/lib/opsmini/data/templates"
	if s.dataDir != nil {
		if d := s.dataDir(); d != "" {
			dir = filepath.Join(d, "templates")
		}
	}
	return dir
}

// templateDir returns the on-disk directory for an app template.
func (s *AppStoreService) templateDir(slug string) string {
	return filepath.Join(s.templateRoot(), slug)
}

// ExportZip exports an app template as a zip byte stream.
// Structure: README.md + logo.png + scripts/{install,upgrade,remove}.sh + files/*.
func (s *AppStoreService) ExportZip(slug string) ([]byte, error) {
	app, err := s.repo.FindBySlug(slug)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// README.md
	readme := app.README
	if readme == "" {
		readme = fmt.Sprintf("# %s\n\n%s\n", app.Name, app.Description)
	}
	if w, err := zw.Create("README.md"); err == nil {
		_, _ = w.Write([]byte(readme))
	}

	// logo.png
	if b, err := os.ReadFile(filepath.Join(s.templateDir(slug), "logo.png")); err == nil {
		if w, err := zw.Create("logo.png"); err == nil {
			_, _ = w.Write(b)
		}
	}

	// scripts/*.sh
	writeScript := func(name, content string) {
		if content == "" {
			return
		}
		if w, err := zw.Create("scripts/" + name); err == nil {
			_, _ = w.Write([]byte(content))
		}
	}
	writeScript("install.sh", app.InstallScript)
	writeScript("upgrade.sh", app.UpgradeScript)
	writeScript("remove.sh", app.RemoveScript)

	// files/*
	filesDir := filepath.Join(s.templateDir(slug), "files")
	_ = filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(filesDir, path)
		if rerr != nil {
			return nil
		}
		if b, rerr := os.ReadFile(path); rerr == nil {
			if w, werr := zw.Create("files/" + rel); werr == nil {
				_, _ = w.Write(b)
			}
		}
		return nil
	})

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportZip imports an app template from a zip byte stream. When slug/name/category
// are empty, they are inferred from the README.
func (s *AppStoreService) ImportZip(data []byte, slug, name, category string) (*model.App, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("无效的 zip 文件：" + err.Error())
	}

	app := &model.App{Category: "other", Color: "#4f6ef7"}
	var logo []byte
	files := map[string][]byte{}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, _ := io.ReadAll(rc)
		rc.Close()

		switch {
		case f.Name == "README.md":
			app.README = string(content)
		case f.Name == "logo.png":
			logo = content
		case f.Name == "scripts/install.sh":
			app.InstallScript = string(content)
		case f.Name == "scripts/upgrade.sh":
			app.UpgradeScript = string(content)
		case f.Name == "scripts/remove.sh":
			app.RemoveScript = string(content)
		case strings.HasPrefix(f.Name, "files/") && f.Name != "files/":
			files[strings.TrimPrefix(f.Name, "files/")] = content
		}
	}

	// name and identifier: prefer the caller-provided value, otherwise infer from the README first line "# name".
	if name == "" {
		for _, line := range strings.Split(app.README, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				break
			}
		}
	}
	if name == "" {
		return nil, errors.New("缺少应用名称（请填写 name 或在 README 首行写 # 名称）")
	}
	if slug == "" {
		slug = slugify(name)
	}
	if category == "" {
		category = "other"
	}
	if _, err := s.repo.FindBySlug(slug); err == nil {
		return nil, fmt.Errorf("模版标识 %s 已存在", slug)
	}

	app.Slug = slug
	app.Name = name
	app.Category = category
	app.Icon = firstRune(name)
	if app.ContainerName == "" {
		app.ContainerName = "opsmini-" + slug
	}

	// persist logo.png and files/
	if err := s.writeTemplateFiles(slug, logo, files); err != nil {
		return nil, err
	}
	app.HasLogo = logo != nil

	if err := s.repo.Upsert(app); err != nil {
		return nil, err
	}
	return app, nil
}

// writeTemplateFiles writes the logo and files into the template directory.
func (s *AppStoreService) writeTemplateFiles(slug string, logo []byte, files map[string][]byte) error {
	dir := s.templateDir(slug)
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		return err
	}
	if logo != nil {
		if err := os.WriteFile(filepath.Join(dir, "logo.png"), logo, 0o644); err != nil {
			return err
		}
	}
	for rel, content := range files {
		// prevent path traversal
		rel = filepath.Clean(rel)
		if rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		p := filepath.Join(dir, "files", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// LogoPath returns the template logo.png path (empty if it does not exist).
func (s *AppStoreService) LogoPath(slug string) string {
	p := filepath.Join(s.templateDir(slug), "logo.png")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// slugify converts a name to a simple slug (lowercase, whitespace to -, keeps alphanumerics and -).
func slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "app"
	}
	return s
}

// firstRune takes the name's first character as an icon fallback (works for Chinese too).
func firstRune(name string) string {
	rs := []rune(strings.TrimSpace(name))
	if len(rs) == 0 {
		return "A"
	}
	return string(rs[0])
}

// SyncOfficial syncs official templates from the official app store (one-click update, overwrites if already present).
// It returns the number of successfully synced templates.
func (s *AppStoreService) SyncOfficial() (int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	manifest, err := fetchOfficialManifest(client)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, app := range manifest.Apps {
		if app.Slug == "" || app.DownloadURL == "" {
			continue
		}
		data, err := downloadZip(client, app.DownloadURL)
		if err != nil {
			continue
		}
		// delete first if it already exists, to implement overwrite update
		if _, err := s.repo.FindBySlug(app.Slug); err == nil {
			_ = s.repo.DeleteBySlug(app.Slug)
		}
		if _, err := s.ImportZip(data, app.Slug, app.Name, app.Category); err == nil {
			count++
		}
	}
	return count, nil
}

// fetchOfficialManifest downloads and decodes the official app-template manifest.
func fetchOfficialManifest(client *http.Client) (*OfficialManifest, error) {
	resp, err := client.Get(officialManifestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("官网返回 HTTP %d", resp.StatusCode)
	}
	var m OfficialManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// downloadZip downloads a URL and returns its bytes, requiring a 200 status.
func downloadZip(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败 HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
