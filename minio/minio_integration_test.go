package minio

// Integration test against a live MinIO/S3 endpoint. Skipped under -short and
// unless MINIO_ENDPOINT is set. Run locally with, e.g.:
//
//	docker run -d -p 9000:9000 -e MINIO_ROOT_USER=minio \
//	  -e MINIO_ROOT_PASSWORD=minio123 minio/minio server /data
//	MINIO_ENDPOINT=127.0.0.1:9000 MINIO_ACCESS_KEY=minio \
//	MINIO_SECRET_KEY=minio123 go test -run Integration -v ./minio/

import (
	"bytes"
	"context"
	"os"
	"testing"

	miniogo "github.com/minio/minio-go/v7"
)

func TestIntegration_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_ENDPOINT not set")
	}

	ctx := context.Background()
	c, err := New(ctx,
		WithEndpoint(endpoint),
		WithCredentials(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")),
		WithSecure(false),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	bucket := "kit4go-it"
	if err := c.MakeBucket(ctx, bucket, miniogo.MakeBucketOptions{}); err != nil {
		// tolerate already-exists from a prior run
		ok, existsErr := c.BucketExists(ctx, bucket)
		if existsErr != nil || !ok {
			t.Fatalf("MakeBucket: %v", err)
		}
	}

	payload := []byte("integration-payload")
	if _, err := c.PutObject(ctx, bucket, "obj", bytes.NewReader(payload), int64(len(payload)), miniogo.PutObjectOptions{}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	obj, err := c.GetObject(ctx, bucket, "obj", miniogo.GetObjectOptions{})
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer obj.Close()

	info, err := c.StatObject(ctx, bucket, "obj", miniogo.StatObjectOptions{})
	if err != nil {
		t.Fatalf("StatObject: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("Stat size = %d, want %d", info.Size, len(payload))
	}

	if _, err := c.ListObjects(ctx, bucket, miniogo.ListObjectsOptions{}); err != nil {
		t.Fatalf("ListObjects: %v", err)
	}

	if err := c.RemoveObject(ctx, bucket, "obj", miniogo.RemoveObjectOptions{}); err != nil {
		t.Fatalf("RemoveObject: %v", err)
	}

	m := c.Metrics()
	if m.Puts == 0 || m.Errors != 0 {
		t.Fatalf("metrics after round-trip: %+v", m)
	}
}
