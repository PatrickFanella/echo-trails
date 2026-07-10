package media

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3ObjectWriter struct{ client *s3.Client }

func NewS3ObjectWriter(client *s3.Client) (*S3ObjectWriter, error) {
	if client == nil {
		return nil, ErrProcessorMisconfigured
	}
	return &S3ObjectWriter{client: client}, nil
}

func (w *S3ObjectWriter) GetObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, error) {
	result, err := w.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(result.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("object exceeds maximum size")
	}
	return data, nil
}

func (w *S3ObjectWriter) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := w.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return err
}

func (w *S3ObjectWriter) PutObject(ctx context.Context, bucket, key, contentType string, body []byte) error {
	_, err := w.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), ContentType: aws.String(contentType),
		ContentLength: aws.Int64(int64(len(body))), Body: bytes.NewReader(body),
	})
	return err
}
