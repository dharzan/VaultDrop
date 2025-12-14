package s3storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/dharsanguruparan/VaultDrop/internal/config"
)

// Storage wraps MinIO/S3 interactions for raw and processed artifacts.
type Storage struct {
	client          *minio.Client
	rawBucket       string
	processedBucket string
	region          string
}

// New creates a MinIO client from the Config.
func New(cfg *config.Config) (*Storage, error) {
	client, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("init minio: %w", err)
	}
	return &Storage{
		client:          client,
		rawBucket:       cfg.RawBucket,
		processedBucket: cfg.ProcessedBucket,
		region:          cfg.S3Region,
	}, nil
}

// EnsureBuckets makes sure the raw/processed buckets exist before use.
func (s *Storage) EnsureBuckets(ctx context.Context) error {
	for _, bucket := range []string{s.rawBucket, s.processedBucket} {
		exists, err := s.client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: s.region}); err != nil {
				return fmt.Errorf("make bucket %s: %w", bucket, err)
			}
		}
	}
	return nil
}

// UploadRaw uploads the PDF into the raw bucket.
func (s *Storage) UploadRaw(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{ContentType: contentType}
	_, err := s.client.PutObject(ctx, s.rawBucket, objectKey, reader, size, opts)
	if err != nil {
		return fmt.Errorf("upload raw object: %w", err)
	}
	return nil
}

// UploadProcessed uploads the extracted text output into the processed bucket.
func (s *Storage) UploadProcessed(ctx context.Context, objectKey string, data []byte) error {
	reader := bytes.NewReader(data)
	opts := minio.PutObjectOptions{ContentType: "text/plain; charset=utf-8"}
	_, err := s.client.PutObject(ctx, s.processedBucket, objectKey, reader, int64(len(data)), opts)
	if err != nil {
		return fmt.Errorf("upload processed object: %w", err)
	}
	return nil
}

// DownloadRaw fetches the raw PDF bytes from storage.
func (s *Storage) DownloadRaw(ctx context.Context, objectKey string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.rawBucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get raw object: %w", err)
	}
	defer obj.Close()
	buf, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read raw object: %w", err)
	}
	return buf, nil
}

// PresignProcessedURL returns a signed GET URL for the processed text file.
func (s *Storage) PresignProcessedURL(ctx context.Context, objectKey string, expirySeconds int64) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.processedBucket, objectKey, time.Duration(expirySeconds)*time.Second, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign processed object: %w", err)
	}
	return u.String(), nil
}
