package minio

// White-box mock for the minioAPI interface. Each method delegates to an
// optional function field (tests override per-case to inject success/error);
// unset fields return sane zero-values so happy paths work without wiring.

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
)

type mockAPI struct {
	putFn         func(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error)
	getFn         func(context.Context, string, string, minio.GetObjectOptions) (*minio.Object, error)
	statFn        func(context.Context, string, string, minio.StatObjectOptions) (minio.ObjectInfo, error)
	removeFn      func(context.Context, string, string, minio.RemoveObjectOptions) error
	bucketExistsF func(context.Context, string) (bool, error)
	makeBucketFn  func(context.Context, string, minio.MakeBucketOptions) error
	listObjectsFn func(context.Context, string, minio.ListObjectsOptions) <-chan minio.ObjectInfo
	listBucketsFn func(context.Context) ([]minio.BucketInfo, error)
	presignFn     func(context.Context, string, string, time.Duration, url.Values) (*url.URL, error)

	puts, gets, stats, removes int
}

func (m *mockAPI) PutObject(ctx context.Context, bucket, object string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	m.puts++
	if m.putFn != nil {
		return m.putFn(ctx, bucket, object, r, size, opts)
	}
	return minio.UploadInfo{Bucket: bucket, Key: object, Size: 42}, nil
}

func (m *mockAPI) GetObject(ctx context.Context, bucket, object string, opts minio.GetObjectOptions) (*minio.Object, error) {
	m.gets++
	if m.getFn != nil {
		return m.getFn(ctx, bucket, object, opts)
	}
	return &minio.Object{}, nil
}

func (m *mockAPI) StatObject(ctx context.Context, bucket, object string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	m.stats++
	if m.statFn != nil {
		return m.statFn(ctx, bucket, object, opts)
	}
	return minio.ObjectInfo{Key: object, Size: 42}, nil
}

func (m *mockAPI) RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error {
	m.removes++
	if m.removeFn != nil {
		return m.removeFn(ctx, bucket, object, opts)
	}
	return nil
}

func (m *mockAPI) BucketExists(ctx context.Context, bucket string) (bool, error) {
	if m.bucketExistsF != nil {
		return m.bucketExistsF(ctx, bucket)
	}
	return true, nil
}

func (m *mockAPI) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	if m.makeBucketFn != nil {
		return m.makeBucketFn(ctx, bucket, opts)
	}
	return nil
}

func (m *mockAPI) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	if m.listObjectsFn != nil {
		return m.listObjectsFn(ctx, bucket, opts)
	}
	// Default: a channel with two clean objects then close.
	ch := make(chan minio.ObjectInfo, 2)
	ch <- minio.ObjectInfo{Key: "a", Size: 1}
	ch <- minio.ObjectInfo{Key: "b", Size: 2}
	close(ch)
	return ch
}

func (m *mockAPI) ListBuckets(ctx context.Context) ([]minio.BucketInfo, error) {
	if m.listBucketsFn != nil {
		return m.listBucketsFn(ctx)
	}
	return []minio.BucketInfo{{Name: "default"}}, nil
}

func (m *mockAPI) PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	if m.presignFn != nil {
		return m.presignFn(ctx, bucket, object, expires, reqParams)
	}
	return &url.URL{Scheme: "https", Host: "example.com", Path: "/" + bucket + "/" + object}, nil
}

// compile-time: mockAPI satisfies minioAPI.
var _ minioAPI = (*mockAPI)(nil)
