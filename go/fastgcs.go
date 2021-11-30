package fastgcs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
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
	Open(gsURL string) (io.ReadCloser, error)
	Copy(gsURL, path string) error
	Read(gsURL string) ([]byte, error)
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

func (f *fastGCS) Open(gsURL string) (io.ReadCloser, error) {
	f.ensureCurrentToken()

	cachePath, err := f.update(gsURL)
	if err != nil {
		return nil, err
	}
	return os.Open(cachePath)
}

func (f *fastGCS) Copy(gsURL, path string) error {
	cachePath, err := f.update(gsURL)
	if err != nil {
		return err
	}
	return copyFile(cachePath, path, 0644)
}

func (f *fastGCS) Read(gsURL string) ([]byte, error) {
	cachePath, err := f.update(gsURL)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadFile(cachePath)
}

func (f *fastGCS) update(gsURL string) (string, error) {
	path, err := f.cachePath(gsURL)
	if err != nil {
		return "", err
	}
	_ = path

	url, err := apiFetchURL(gsURL)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", f.token.Token))
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	_, err = io.Copy(dst, res.Body)
	if err != nil {
		return "", err
	}

	return path, nil
}

var gsURLRegexp = regexp.MustCompile("^gs://([^/]+)/(.*)$")

func (f *fastGCS) cachePath(gsURL string) (string, error) {
	bucket, object, err := parseGSURL(gsURL)
	if err != nil {
		return "", err
	}

	return filepath.Join(
		f.cacheRoot,
		fmt.Sprintf("%s--%s", bucket, strings.ReplaceAll(object, "/", "-")),
	), nil
}

func apiFetchURL(gsURL string) (string, error) {
	bucket, object, err := parseGSURL(gsURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"https://storage.googleapis.com/storage/v1/b/%s/o/%s?alt=media",
		bucket, object,
	), nil
}

func parseGSURL(gsURL string) (string, string, error) {
	match := gsURLRegexp.FindStringSubmatch(gsURL)
	if match == nil {
		return "", "", errors.Errorf("invalid GCS URL: %s", gsURL)
	}
	bucket := match[1]
	object := match[2]

	return bucket, object, nil
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
