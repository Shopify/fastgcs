package fastgcs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	credentialsCacheBasename = "com.shopify.fastgcs.json"
)

type FastGCS interface {
	Open(url string) (io.ReadCloser, error)
	Copy(url, path string) error
	Read(url string) ([]byte, error)
}

func New() (FastGCS, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cacheRoot := filepath.Join(home, ".cache", "fastgcs")
	os.MkdirAll(cacheRoot, os.ModePerm)
	return &fastGCS{
		cacheRoot:       cacheRoot,
		gcloudConfigDir: filepath.Join(home, ".config", "gcloud"),
	}, nil
}

type token struct {
	Token  string
	Expiry time.Time
}

type fastGCS struct {
	cacheRoot       string
	gcloudConfigDir string

	token *token
}

func (f *fastGCS) ensureCurrentToken() error {
	tok := f.token
	if tok != nil && time.Now().Before(tok.Expiry) {
		return nil
	}

	tok, err := f.findTokenInCache()
	if err != nil {
		return err
	}

	if tok != nil {
		f.token = tok
		return nil
	}

	return errors.New("couldn't obtain access token")
}

func (f *fastGCS) findTokenInCache() (*token, error) {
	data, err := ioutil.ReadFile(filepath.Join(f.gcloudConfigDir, credentialsCacheBasename))
	if err != nil {
		// TODO(burke): certain errors should be bubbled up. ENOENT shouldn't.
		return nil, nil
	}

	var cache token

	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

func (f *fastGCS) Open(url string) (io.ReadCloser, error) {
	f.ensureCurrentToken()

	cachePath, err := f.update(url)
	if err != nil {
		return nil, err
	}
	return os.Open(cachePath)
}

func (f *fastGCS) Copy(url, path string) error {
	cachePath, err := f.update(url)
	if err != nil {
		return err
	}
	return copyFile(cachePath, path, 0644)
}

func (f *fastGCS) Read(url string) ([]byte, error) {
	cachePath, err := f.update(url)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadFile(cachePath)
}

func (f *fastGCS) update(url string) (string, error) {
	path, err := f.cachePath(url)
	if err != nil {
		return "", err
	}
	_ = path
	return "", nil
}

var gsURLRegexp = regexp.MustCompile("^gs://([^/]+)/(.*)$")

func (f *fastGCS) cachePath(url string) (string, error) {
	match := gsURLRegexp.FindStringSubmatch(url)
	if match == nil {
		return "", errors.Errorf("invalid GCS URL: %s", url)
	}
	bucket := match[1]
	object := match[2]

	return filepath.Join(
		f.cacheRoot,
		fmt.Sprintf("%s--%s", bucket, strings.ReplaceAll(object, "/", "-")),
	), nil
}

func copyFile(srcPath, dstPath string, mode fs.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
