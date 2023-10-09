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
)

const (
	timeout = 1 * time.Second
)

type DigitalOceanWriter struct {
	endpoint string
	signer   *awsSigner.Signer
}

var _ Writer = (*DigitalOceanWriter)(nil)

type S3Config struct {
	Key      string
	Secret   string
	Endpoint string
	Bucket   string
}

func NewDigitalOceanWriter(
	key string,
	secret string,
	endpoint string,
	bucket string,
) (*DigitalOceanWriter, error) {
	return &DigitalOceanWriter{
		endpoint: fmt.Sprintf("https://%s.%s/", bucket, endpoint),
		signer:   awsSigner.NewSigner(credentials.NewStaticCredentials(key, secret, "")),
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
		"Cache-Control":       "max-age=60",
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
		res, err = client.Do(req) //nolint:bodyclose // allow since close is outside loop
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
