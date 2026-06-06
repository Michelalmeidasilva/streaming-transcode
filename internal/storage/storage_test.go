package storage

import (
	"testing"

	"streaming-transcode/internal/config"
)

func TestNewSelectsMinIOProvider(t *testing.T) {
	store, err := New(config.StorageConfig{
		Provider:  "minio",
		Endpoint:  "localhost:9000",
		AccessKey: "admin",
		SecretKey: "password123",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("New(minio) error = %v", err)
	}
	if store == nil {
		t.Fatalf("New(minio) store = nil")
	}
}

func TestNewSelectsS3Provider(t *testing.T) {
	store, err := New(config.StorageConfig{
		Provider:  "aws-s3",
		AccessKey: "AKIA",
		SecretKey: "secret",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("New(aws-s3) error = %v", err)
	}
	if store == nil {
		t.Fatalf("New(aws-s3) store = nil")
	}
}

func TestNewDefaultsToMinIOWhenProviderEmpty(t *testing.T) {
	store, err := New(config.StorageConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "admin",
		SecretKey: "password123",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("New(empty) error = %v", err)
	}
	if store == nil {
		t.Fatalf("New(empty) store = nil")
	}
}

func TestNewRejectsUnknownProvider(t *testing.T) {
	if _, err := New(config.StorageConfig{Provider: "gcs"}); err == nil {
		t.Fatalf("New(gcs) error = nil, want error")
	}
}

func TestAWSS3Endpoint(t *testing.T) {
	cases := map[string]string{
		"us-east-1": "s3.us-east-1.amazonaws.com",
		"eu-west-1": "s3.eu-west-1.amazonaws.com",
		"":          "s3.us-east-1.amazonaws.com",
	}
	for region, want := range cases {
		if got := awsS3Endpoint(region); got != want {
			t.Fatalf("awsS3Endpoint(%q) = %q, want %q", region, got, want)
		}
	}
}
