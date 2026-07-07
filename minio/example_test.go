package minio_test

import (
	"bytes"
	"context"
	"log"

	miniogo "github.com/minio/minio-go/v7"

	"github.com/v8fg/kit4go/minio"
)

// ExampleNew shows construction + put/presign. It is a compile-checked
// illustration (an Example without an // Output: comment is compiled but not
// executed); wire your own endpoint/credentials to run it against a live
// MinIO/S3 endpoint.
func ExampleNew() {
	c, err := minio.New(context.Background(),
		minio.WithEndpoint("play.min.io"),
		minio.WithCredentials("ACCESS", "SECRET"),
		minio.WithSecure(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Upload an object from memory (e.g. an ad creative). body.Size() is the
	// known length; pass -1 for streaming an unknown size.
	body := bytes.NewReader([]byte("creative-payload"))
	if _, err := c.PutObject(context.Background(), "creatives", "banner-1.png", body, body.Size(), miniogo.PutObjectOptions{}); err != nil {
		log.Fatal(err)
	}

	// Presign a short-lived download URL (no credentials handed to the caller).
	if _, err := c.PresignedGetObject(context.Background(), "creatives", "banner-1.png", 0, nil); err != nil {
		log.Fatal(err)
	}
}
