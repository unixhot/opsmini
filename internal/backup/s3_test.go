package backup

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The published AWS SigV4 test vector is the only independent proof that the
// signature implementation is right. A self-consistent mock S3 server would agree
// with any bug in both halves.
//
// From "Signature Calculations for the Authorization Header: Transferring Payload
// in a Single Chunk (AWS Signature Version 4)", the GET Object with Range example.
func TestSigV4MatchesAWSTestVector(t *testing.T) {
	s := &s3Store{
		cfg: S3Config{
			Endpoint:  "https://s3.amazonaws.com",
			Region:    "us-east-1",
			Bucket:    "examplebucket",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		now: func() time.Time {
			return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
		},
	}
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{
		"host":                 "examplebucket.s3.amazonaws.com",
		"range":                "bytes=0-9",
		"x-amz-content-sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	s.sign(req, http.MethodGet, "/test.txt", nil, headers)

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request," +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date," +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("signature mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestURIEncoding(t *testing.T) {
	cases := map[string]string{
		"simple.txt":        "simple.txt",
		"dir/file name.log": "dir/file%20name.log",
		"a+b":               "a%2Bb",
		"测试.txt":            "%E6%B5%8B%E8%AF%95.txt",
	}
	for in, want := range cases {
		if got := awsURIEncode(in, false); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
	// In query values the slash must be encoded; in paths it must not.
	if got := awsURIEncode("a/b", true); got != "a%2Fb" {
		t.Fatalf("query encoding of slash: %q", got)
	}
}

func TestObjectKeyAndHost(t *testing.T) {
	s := &s3Store{cfg: S3Config{Endpoint: "https://oss-cn-beijing.aliyuncs.com", Bucket: "bkt", Prefix: "opsmini/prod"}}
	if got := s.objectKey("snap-1", "data/a.tar.gz"); got != "opsmini/prod/snap-1/data/a.tar.gz" {
		t.Fatalf("key=%s", got)
	}
	host, path := s.hostAndPath("k")
	if host != "bkt.oss-cn-beijing.aliyuncs.com" || path != "/k" {
		t.Fatalf("virtual-host style: host=%s path=%s", host, path)
	}

	s.cfg.PathStyle = true
	host, path = s.hostAndPath("k")
	if host != "oss-cn-beijing.aliyuncs.com" || path != "/bkt/k" {
		t.Fatalf("path style: host=%s path=%s", host, path)
	}
}

func TestS3ConfigValidation(t *testing.T) {
	if _, err := newS3Store(S3Config{Bucket: "b"}); err == nil {
		t.Fatal("missing endpoint must fail")
	}
	if _, err := newS3Store(S3Config{Endpoint: "https://x"}); err == nil {
		t.Fatal("missing bucket must fail")
	}
	st, err := newS3Store(S3Config{Endpoint: "https://x/", Bucket: "b", Prefix: "/p/"})
	if err != nil {
		t.Fatal(err)
	}
	if st.cfg.Endpoint != "https://x" || st.cfg.Prefix != "p" {
		t.Fatalf("normalisation failed: %+v", st.cfg)
	}
	if !strings.Contains(st.Describe(), "b") {
		t.Fatalf("describe=%s", st.Describe())
	}
}
