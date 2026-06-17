// Zener - a post-quantum-safe end-to-end encrypted file dropbox.
// Copyright (C) 2026 Tobias von Dewitz <tobias@vondewitz.org>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package s3store

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/tob/zener/internal/blob"
)

type Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
	PartSize     int64
	Concurrency  int
}

type Store struct {
	bucket   string
	client   *s3.Client
	uploader *manager.Uploader
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(cfg.Region),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	if cfg.PartSize == 0 {
		cfg.PartSize = 32 * 1024 * 1024
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 4
	}
	return &Store{
		bucket: cfg.Bucket,
		client: client,
		uploader: manager.NewUploader(client, func(u *manager.Uploader) {
			u.PartSize = cfg.PartSize
			u.Concurrency = cfg.Concurrency
		}),
	}, nil
}

func (s *Store) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	input := uploadInput(s.bucket, key, body, contentType)
	_, err := s.uploader.Upload(ctx, input)
	return err
}

func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, downloadInput(s.bucket, key))
	if err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			return nil, blob.ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func uploadInput(bucket string, key string, body io.Reader, contentType string) *s3.PutObjectInput {
	input := &s3.PutObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		Body:              body,
		ChecksumAlgorithm: types.ChecksumAlgorithmCrc32,
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	return input
}

func downloadInput(bucket string, key string) *s3.GetObjectInput {
	return &s3.GetObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		ChecksumMode: types.ChecksumModeEnabled,
	}
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}
