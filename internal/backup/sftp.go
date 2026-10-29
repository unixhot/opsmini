package backup

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SFTP target.
//
// The reason this exists alongside S3: many private environments have a jump host
// with an SFTP directory and no object storage at all. Supporting only S3 would
// mean "off-host backup" is unavailable exactly where losing the machine hurts
// most.

// SFTPConfig configures an SFTP target.
type SFTPConfig struct {
	Name       string `json:"name"`
	Host       string `json:"host"` // host:port, port defaults to 22
	User       string `json:"user"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"` // PEM content, not a path
	// BaseDir is the remote directory holding snapshots; created if missing.
	BaseDir string `json:"base_dir"`
	// KnownHostsFile enables host key verification. Empty means verification is
	// skipped, which the panel warns about: an unverified host key makes the
	// transport encrypt to whoever answered first.
	KnownHostsFile string `json:"known_hosts_file"`
}

type sftpStore struct {
	cfg SFTPConfig
}

func newSFTPStore(cfg SFTPConfig) (*sftpStore, error) {
	if cfg.Host == "" || cfg.User == "" {
		return nil, fmt.Errorf("SFTP 目标缺少 host 或 user")
	}
	if !strings.Contains(cfg.Host, ":") {
		cfg.Host = net.JoinHostPort(cfg.Host, "22")
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = "/backup/opsmini"
	}
	if cfg.Password == "" && cfg.PrivateKey == "" {
		return nil, fmt.Errorf("SFTP 目标需要密码或私钥")
	}
	return &sftpStore{cfg: cfg}, nil
}

func (s *sftpStore) Describe() string {
	return fmt.Sprintf("SFTP %s:%s", s.cfg.Host, s.cfg.BaseDir)
}

// connect opens one SSH connection and an SFTP session on top of it.
func (s *sftpStore) connect(ctx context.Context) (*ssh.Client, *sftp.Client, error) {
	var auths []ssh.AuthMethod
	if s.cfg.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(s.cfg.PrivateKey))
		if err != nil {
			return nil, nil, fmt.Errorf("解析私钥失败：%w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if s.cfg.Password != "" {
		auths = append(auths, ssh.Password(s.cfg.Password))
	}

	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	if s.cfg.KnownHostsFile != "" {
		cb, err := knownHostsCallback(s.cfg.KnownHostsFile)
		if err != nil {
			return nil, nil, err
		}
		hostKeyCallback = cb
	}

	cfg := &ssh.ClientConfig{
		User:            s.cfg.User,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback,
		Timeout:         20 * time.Second,
	}
	d := net.Dialer{Timeout: 20 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", s.cfg.Host)
	if err != nil {
		return nil, nil, fmt.Errorf("连接 %s 失败：%w", s.cfg.Host, err)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, s.cfg.Host, cfg)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("SSH 握手失败：%w", err)
	}
	client := ssh.NewClient(c, chans, reqs)
	sc, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("打开 SFTP 会话失败：%w", err)
	}
	return client, sc, nil
}

// PutDir uploads a snapshot directory.
func (s *sftpStore) PutDir(ctx context.Context, localDir, name string) error {
	client, sc, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	defer sc.Close()

	remoteRoot := path.Join(s.cfg.BaseDir, name)
	if err := sc.MkdirAll(remoteRoot); err != nil {
		return fmt.Errorf("创建远端目录失败：%w", err)
	}
	return filepath.Walk(localDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localDir, p)
		if err != nil {
			return err
		}
		remote := path.Join(remoteRoot, filepath.ToSlash(rel))
		if err := sc.MkdirAll(path.Dir(remote)); err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := sc.Create(remote)
		if err != nil {
			return fmt.Errorf("创建远端文件 %s 失败：%w", remote, err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return fmt.Errorf("上传 %s 失败：%w", remote, err)
		}
		return dst.Close()
	})
}

// List returns the snapshot directories under BaseDir.
func (s *sftpStore) List(ctx context.Context) ([]RemoteSnapshot, error) {
	client, sc, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	defer sc.Close()

	entries, err := sc.ReadDir(s.cfg.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取远端目录失败：%w", err)
	}
	out := make([]RemoteSnapshot, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, RemoteSnapshot{Name: e.Name(), Modified: e.ModTime(), Store: s.Describe()})
	}
	return out, nil
}

// GetDir downloads one snapshot into dstParent/name.
func (s *sftpStore) GetDir(ctx context.Context, name, dstParent string) error {
	client, sc, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	defer sc.Close()

	remoteRoot := path.Join(s.cfg.BaseDir, name)
	walker := sc.Walk(remoteRoot)
	dst := filepath.Join(dstParent, name)
	found := false
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}
		if walker.Stat().IsDir() {
			continue
		}
		found = true
		rel := strings.TrimPrefix(walker.Path(), remoteRoot+"/")
		local := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
			return err
		}
		src, err := sc.Open(walker.Path())
		if err != nil {
			return err
		}
		out, err := os.OpenFile(local, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			src.Close()
			return err
		}
		out.Close()
		src.Close()
	}
	if !found {
		return fmt.Errorf("远端不存在快照 %s", name)
	}
	return nil
}

// Delete removes a snapshot directory recursively.
func (s *sftpStore) Delete(ctx context.Context, name string) error {
	client, sc, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	defer sc.Close()

	remoteRoot := path.Join(s.cfg.BaseDir, name)
	walker := sc.Walk(remoteRoot)
	var dirs []string
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}
		if walker.Stat().IsDir() {
			dirs = append(dirs, walker.Path())
			continue
		}
		if err := sc.Remove(walker.Path()); err != nil {
			return fmt.Errorf("删除 %s 失败：%w", walker.Path(), err)
		}
	}
	// Deepest first, otherwise RemoveDirectory fails on non-empty parents.
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := sc.RemoveDirectory(dirs[i]); err != nil {
			return fmt.Errorf("删除目录 %s 失败：%w", dirs[i], err)
		}
	}
	return nil
}

// knownHostsCallback verifies the server key against a known_hosts file, the same
// way OpenSSH does: any of its entries may match, so the file is parsed as a whole
// rather than taking the first key.
func knownHostsCallback(file string) (ssh.HostKeyCallback, error) {
	cb, err := knownhosts.New(file)
	if err != nil {
		return nil, fmt.Errorf("解析 known_hosts 失败：%w", err)
	}
	return cb, nil
}
