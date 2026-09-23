package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type COSStorage struct {
	client    *cos.Client
	secretID  string
	secretKey string
	bucketURL *url.URL
	urlTTL    time.Duration
}

func NewCOSStorage(bucketURL, secretID, secretKey string, timeout, urlTTL time.Duration) (*COSStorage, error) {
	baseURL, err := url.Parse(bucketURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid COS bucket URL")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if urlTTL <= 0 {
		urlTTL = 15 * time.Minute
	}
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: &cos.CredentialTransport{Credential: cos.NewTokenCredential(secretID, secretKey, "")},
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: baseURL}, httpClient)
	return &COSStorage{client: client, secretID: secretID, secretKey: secretKey, bucketURL: baseURL, urlTTL: urlTTL}, nil
}

func (s *COSStorage) Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	cleanKey, err := safeKey(key)
	if err != nil {
		return err
	}
	_, err = s.client.Object.Put(ctx, cleanKey, content, &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: contentType, ContentLength: size}})
	if err != nil {
		return fmt.Errorf("upload COS object: %w", err)
	}
	return nil
}

func (s *COSStorage) Read(ctx context.Context, key string) ([]byte, error) {
	cleanKey, err := safeKey(key)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Object.Get(ctx, cleanKey, nil)
	if err != nil {
		return nil, fmt.Errorf("read COS object: %w", err)
	}
	defer response.Body.Close()
	value, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read COS object body: %w", err)
	}
	return value, nil
}

func (s *COSStorage) Delete(ctx context.Context, key string) error {
	cleanKey, err := safeKey(key)
	if err != nil {
		return err
	}
	if _, err := s.client.Object.Delete(ctx, cleanKey); err != nil {
		return fmt.Errorf("delete COS object: %w", err)
	}
	return nil
}

func (s *COSStorage) URL(ctx context.Context, key string) (string, error) {
	cleanKey, err := safeKey(key)
	if err != nil {
		return "", err
	}
	presigned, err := s.client.Object.GetPresignedURL(ctx, http.MethodGet, cleanKey, s.secretID, s.secretKey, s.urlTTL, nil, false)
	if err != nil {
		return "", fmt.Errorf("generate COS object URL: %w", err)
	}
	return presigned.String(), nil
}

func safeKey(key string) (string, error) {
	clean := strings.TrimLeft(key, "/")
	if clean == "" || strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid storage key")
	}
	return clean, nil
}
