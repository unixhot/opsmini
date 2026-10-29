package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// S3-compatible remote storage, signed with AWS Signature V4.
//
// The signature is implemented here rather than pulled in from the AWS SDK on
// purpose: the SDK drags in ~20MB of dependencies for four operations, on a binary
// whose whole selling point is staying small. Everything OSS/MinIO/B2/Ceph need is
// the same four verbs and one signing algorithm.

// S3Config configures an S3-compatible target.
type S3Config struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"` // e.g. https://oss-cn-beijing.aliyuncs.com
	Region    string `json:"region"`   // e.g. cn-beijing / us-east-1
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"` // key prefix, e.g. "opsmini/prod"
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	// PathStyle uses endpoint/bucket/key instead of bucket.endpoint/key.
	// MinIO and most private deployments need it; AWS and OSS accept it too.
	PathStyle bool `json:"path_style"`
}

type s3Store struct {
	cfg  S3Config
	http *http.Client
	// now is injectable so the signature can be checked against the published AWS
	// test vectors, which are the only real proof the implementation is correct.
	now func() time.Time
}

func newS3Store(cfg S3Config) (*s3Store, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 目标缺少 endpoint 或 bucket")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	cfg.Prefix = strings.Trim(cfg.Prefix, "/")
	return &s3Store{cfg: cfg, http: &http.Client{Timeout: 10 * time.Minute}, now: time.Now}, nil
}

func (s *s3Store) Describe() string {
	return fmt.Sprintf("S3 %s/%s", s.cfg.Bucket, strings.TrimPrefix(s.cfg.Prefix, "/"))
}

// objectKey builds the full key for a snapshot file.
func (s *s3Store) objectKey(name, rel string) string {
	parts := []string{}
	if s.cfg.Prefix != "" {
		parts = append(parts, s.cfg.Prefix)
	}
	parts = append(parts, name)
	if rel != "" {
		parts = append(parts, strings.TrimPrefix(rel, "/"))
	}
	return strings.Join(parts, "/")
}

// hostAndPath returns the Host header value and the URL path for a key.
func (s *s3Store) hostAndPath(key string) (host, rawPath string) {
	u, err := url.Parse(s.cfg.Endpoint)
	if err != nil {
		return "", "/"
	}
	if s.cfg.PathStyle {
		return u.Host, "/" + s.cfg.Bucket + "/" + key
	}
	return s.cfg.Bucket + "." + u.Host, "/" + key
}

// request builds a signed request for one object operation.
func (s *s3Store) request(ctx context.Context, method, key string, query url.Values, body io.Reader, size int64, extra map[string]string) (*http.Request, error) {
	host, rawPath := s.hostAndPath(key)
	base := strings.TrimRight(s.cfg.Endpoint, "/")
	var target string
	if s.cfg.PathStyle {
		target = base + "/" + s.cfg.Bucket + "/" + awsURIEncode(key, false)
	} else {
		target = base + "/" + awsURIEncode(key, false)
	}
	if len(query) > 0 {
		target += "?" + canonicalQuery(query)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	req.Host = host
	headers := map[string]string{"host": host}
	// UNSIGNED-PAYLOAD lets us stream a multi-GB archive without hashing it twice.
	headers["x-amz-content-sha256"] = unsignedPayload
	for k, v := range extra {
		headers[k] = v
	}
	s.sign(req, method, rawPath, query, headers)
	return req, nil
}

const unsignedPayload = "UNSIGNED-PAYLOAD"

// sign adds the Authorization header for AWS Signature V4.
func (s *s3Store) sign(req *http.Request, method, rawPath string, query url.Values, headers map[string]string) {
	now := s.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	headers["x-amz-date"] = amzDate
	if s.cfg.AccessKey == "" {
		// Anonymous access: no Authorization header at all. Useful for a public
		// read-only endpoint, and it must not send a bogus signature.
		for k, v := range headers {
			if k != "host" {
				req.Header.Set(k, v)
			}
		}
		return
	}

	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, strings.ToLower(k))
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k)
		canonHeaders.WriteString(":")
		canonHeaders.WriteString(strings.TrimSpace(headers[k]))
		canonHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonical := strings.Join([]string{
		method,
		rawPath,
		canonicalQuery(query),
		canonHeaders.String(),
		signedHeaders,
		headers["x-amz-content-sha256"],
	}, "\n")

	scope := dateStamp + "/" + s.cfg.Region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(sum[:]),
	}, "\n")

	key := hmacSHA256([]byte("AWS4"+s.cfg.SecretKey), dateStamp)
	key = hmacSHA256(key, s.cfg.Region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))

	for k, v := range headers {
		if k != "host" {
			req.Header.Set(k, v)
		}
	}
	// No spaces after the commas: this is exactly the form the AWS documentation
	// shows, and it keeps the header byte-identical to the published test vector.
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		s.cfg.AccessKey, scope, signedHeaders, sig))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// canonicalQuery sorts and encodes query parameters the way SigV4 requires.
func canonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := append([]string{}, q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, awsURIEncode(k, true)+"="+awsURIEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// awsURIEncode implements RFC 3986 encoding with the S3 twist: '/' is preserved in
// paths but encoded in query values.
func awsURIEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// PutDir uploads every file of a snapshot directory.
func (s *s3Store) PutDir(ctx context.Context, localDir, name string) error {
	var files []string
	err := filepath.Walk(localDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		rel, err := filepath.Rel(localDir, f)
		if err != nil {
			return err
		}
		if err := s.putFile(ctx, s.objectKey(name, filepath.ToSlash(rel)), f); err != nil {
			return err
		}
	}
	return nil
}

func (s *s3Store) putFile(ctx context.Context, key, local string) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := s.request(ctx, http.MethodPut, key, nil, f, st.Size(), nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("上传 %s 失败：%w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("上传 %s 失败：HTTP %d %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// RemoteSnapshot is a snapshot that exists off-host.
type RemoteSnapshot struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	// Store identifies which remote it lives on (empty for local).
	Store string `json:"store,omitempty"`
}

// List returns the snapshot names under the prefix, using delimiter "/" so each
// snapshot is one common prefix instead of thousands of object keys.
func (s *s3Store) List(ctx context.Context) ([]RemoteSnapshot, error) {
	q := url.Values{}
	q.Set("list-type", "2")
	q.Set("delimiter", "/")
	if s.cfg.Prefix != "" {
		q.Set("prefix", s.cfg.Prefix+"/")
	}
	req, err := s.request(ctx, http.MethodGet, "", q, nil, -1, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("列举 S3 对象失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("列举 S3 对象失败：HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		CommonPrefixes []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}
	if err := xml.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("解析 S3 列举结果失败：%w", err)
	}
	res := make([]RemoteSnapshot, 0, len(out.CommonPrefixes))
	for _, cp := range out.CommonPrefixes {
		name := strings.TrimSuffix(strings.TrimPrefix(cp.Prefix, strings.TrimSuffix(s.cfg.Prefix, "/")), "/")
		name = strings.Trim(name, "/")
		if name == "" {
			continue
		}
		res = append(res, RemoteSnapshot{Name: name, Store: s.Describe()})
	}
	return res, nil
}

// GetDir downloads one snapshot into dstParent/name.
func (s *s3Store) GetDir(ctx context.Context, name, dstParent string) error {
	keys, err := s.listKeys(ctx, s.objectKey(name, ""))
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return fmt.Errorf("远端不存在快照 %s", name)
	}
	base := strings.TrimSuffix(s.objectKey(name, ""), "/") + "/"
	dst := filepath.Join(dstParent, name)
	for _, key := range keys {
		rel := strings.TrimPrefix(key, base)
		out := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
			return err
		}
		if err := s.getFile(ctx, key, out); err != nil {
			return err
		}
	}
	return nil
}

func (s *s3Store) getFile(ctx context.Context, key, local string) error {
	req, err := s.request(ctx, http.MethodGet, key, nil, nil, -1, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("下载 %s 失败：%w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("下载 %s 失败：HTTP %d %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	f, err := os.OpenFile(local, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Delete removes every object of one snapshot.
func (s *s3Store) Delete(ctx context.Context, name string) error {
	keys, err := s.listKeys(ctx, s.objectKey(name, ""))
	if err != nil {
		return err
	}
	for _, key := range keys {
		req, err := s.request(ctx, http.MethodDelete, key, nil, nil, -1, nil)
		if err != nil {
			return err
		}
		resp, err := s.http.Do(req)
		if err != nil {
			return fmt.Errorf("删除 %s 失败：%w", key, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("删除 %s 失败：HTTP %d", key, resp.StatusCode)
		}
	}
	return nil
}

// listKeys returns all object keys under a prefix.
func (s *s3Store) listKeys(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		q := url.Values{}
		q.Set("list-type", "2")
		q.Set("prefix", strings.TrimSuffix(prefix, "/")+"/")
		if token != "" {
			q.Set("continuation-token", token)
		}
		req, err := s.request(ctx, http.MethodGet, "", q, nil, -1, nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("列举 S3 对象失败：%w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("列举 S3 对象失败：HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var out struct {
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
			Next string `xml:"NextContinuationToken"`
		}
		if err := xml.Unmarshal(body, &out); err != nil {
			return nil, err
		}
		for _, c := range out.Contents {
			keys = append(keys, c.Key)
		}
		if out.Next == "" {
			return keys, nil
		}
		token = out.Next
	}
}

// cleanKey is a small helper used by tests and adapters.
func cleanKey(k string) string { return path.Clean("/" + k)[1:] }
