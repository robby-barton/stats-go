package writer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	awsSigner "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/digitalocean/godo"
)

const (
	timeout = 1 * time.Second
)

type DigitalOceanWriter struct {
	endpoint string
	signer   *awsSigner.Signer
	client   *godo.Client
	cdnID    string
}

var _ Writer = (*DigitalOceanWriter)(nil)

type DOConfig struct {
	Key      string
	Secret   string
	Endpoint string
	Bucket   string
	APIToken string
	CDNID    string
}

func NewDigitalOceanWriter(cfg *DOConfig) (*DigitalOceanWriter, error) {
	return &DigitalOceanWriter{
		endpoint: fmt.Sprintf("https://%s.%s/", cfg.Bucket, cfg.Endpoint),
		signer:   awsSigner.NewSigner(credentials.NewStaticCredentials(cfg.Key, cfg.Secret, "")),
		client:   godo.NewFromToken(cfg.APIToken),
		cdnID:    cfg.CDNID,
	}, nil
}

func (w *DigitalOceanWriter) WriteData(ctx context.Context, fileName string, input any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	g := gzip.NewWriter(buf)
	if _, err := g.Write(data); err != nil {
		return err
	}
	if err := g.Close(); err != nil {
		return err
	}

	compressedData := buf.Bytes()
	bodyReader := bytes.NewReader(compressedData)

	client := &http.Client{
		Timeout: timeout,
	}

	endpoint := fmt.Sprintf("%s%s", w.endpoint, fileName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bodyReader)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Cache-Control":       "s-maxage=604800",
		"Content-Disposition": "inline",
		"Content-Encoding":    "gzip",
		"Content-Length":      strconv.FormatInt(int64(len(compressedData)), 10),
		"Content-Type":        "application/json",
		"x-amz-acl":           "public-read",
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	_, err = w.signer.Sign(req, bodyReader, "s3", "nyc3", time.Now())
	if err != nil {
		return err
	}

	var res *http.Response
	count := 0
	for ok := true; ok; ok = (count < 5 && err != nil) {
		res, err = client.Do(req)
		if err == nil {
			break
		}
		count++
		time.Sleep(1 * time.Second)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("non-OK status writing %s: %s", fileName, res.Status)
	}

	return err
}

func (w *DigitalOceanWriter) PurgeCache(ctx context.Context) error {
	flushRequest := &godo.CDNFlushCacheRequest{
		Files: []string{"*"},
	}

	resp, err := w.client.CDNs.FlushCache(ctx, w.cdnID, flushRequest)
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("non-204 status purging CDN: %s", resp.Status)
	}

	return nil
}
