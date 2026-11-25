package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Photos keeps card photos in S3-compatible storage (Cloudflare R2 in production) behind a CDN.
// Each upload gets a new random name, so a URL never changes content and can be cached forever.
type Photos struct {
	s3     *s3.Client
	bucket string
	base   string // public URL prefix the CDN serves the bucket at, no trailing slash
}

func newPhotos(endpoint, region, bucket, accessKey, secretKey, publicBase string) *Photos {
	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true, // R2 and self-hosted stores address buckets by path
	})
	return &Photos{s3: client, bucket: bucket, base: publicBase}
}

func photoPrefix(uid string) string { return "p/" + uid + "/" }

func (p *Photos) url(key string) string { return p.base + "/" + key }

// put stores a cleaned JPEG and returns its key.
func (p *Photos) put(ctx context.Context, uid string, jpeg []byte) (string, error) {
	key := fmt.Sprintf("%s%s.jpg", photoPrefix(uid), randomHex(12))
	_, err := p.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(p.bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(jpeg),
		ContentType:  aws.String("image/jpeg"),
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	})
	return key, err
}

func (p *Photos) delete(ctx context.Context, key string) error {
	_, err := p.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(p.bucket), Key: aws.String(key)})
	return err
}

// deleteAll removes every photo of a deleted account.
func (p *Photos) deleteAll(ctx context.Context, uid string) error {
	pages := s3.NewListObjectsV2Paginator(p.s3, &s3.ListObjectsV2Input{Bucket: aws.String(p.bucket), Prefix: aws.String(photoPrefix(uid))})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, obj := range page.Contents {
			if err := p.delete(ctx, aws.ToString(obj.Key)); err != nil {
				return err
			}
		}
	}
	return nil
}
