package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type Client struct {
	endpoint string
	bucket   string
	logger   *zap.Logger
	s3       *s3.Client
	initErr  error
}

func New(logger *zap.Logger) *Client {
	c := &Client{
		endpoint: os.Getenv("S3_ENDPOINT"),
		bucket:   os.Getenv("S3_BUCKET"),
		logger:   logger,
	}
	if c.bucket == "" {
		return c
	}

	region := envOr("S3_REGION", "us-east-1")
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if accessKey := os.Getenv("S3_ACCESS_KEY"); accessKey != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, os.Getenv("S3_SECRET_KEY"), ""),
		))
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		c.initErr = err
		return c
	}
	c.s3 = s3.NewFromConfig(cfg, func(options *s3.Options) {
		if c.endpoint != "" {
			options.BaseEndpoint = &c.endpoint
			options.UsePathStyle = true
		}
	})
	return c
}

func (c *Client) Enabled() bool {
	return c.bucket != ""
}

func (c *Client) Bucket() string { return c.bucket }

func (c *Client) EnsureBucket(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	if c.initErr != nil {
		return c.initErr
	}
	if c.s3 == nil {
		return fmt.Errorf("S3 client unavailable")
	}
	if c.endpoint == "" {
		return nil
	}
	_, err := c.s3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &c.bucket})
	if err != nil {
		var headErr error
		_, headErr = c.s3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &c.bucket})
		if headErr != nil {
			return fmt.Errorf("ensure bucket: %w", err)
		}
	}
	return nil
}

func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	if !c.Enabled() {
		return fmt.Errorf("S3_BUCKET not configured")
	}
	if err := c.EnsureBucket(ctx); err != nil {
		return err
	}
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &c.bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: stringPtr("application/gzip"),
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("S3_BUCKET not configured")
	}
	if c.initErr != nil {
		return nil, c.initErr
	}
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &c.bucket, Key: &key})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func stringPtr(value string) *string { return &value }
