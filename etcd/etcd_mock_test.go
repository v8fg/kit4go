package etcd

// White-box mock for the etcdAPI interface. Each method delegates to an optional
// function field (tests override per-case); unset fields return sane
// zero-values so happy paths work without wiring.

import (
	"context"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type mockAPI struct {
	putFn       func(context.Context, string, string, ...clientv3.OpOption) (*clientv3.PutResponse, error)
	getFn       func(context.Context, string, ...clientv3.OpOption) (*clientv3.GetResponse, error)
	deleteFn    func(context.Context, string, ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	grantFn     func(context.Context, int64) (*clientv3.LeaseGrantResponse, error)
	keepAliveFn func(context.Context, clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error)
	revokeFn    func(context.Context, clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error)
	watchFn     func(context.Context, string, ...clientv3.OpOption) clientv3.WatchChan
	statusFn    func(context.Context, string) (*clientv3.StatusResponse, error)

	puts, gets, deletes, grants int
}

func (m *mockAPI) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	m.puts++
	if m.putFn != nil {
		return m.putFn(ctx, key, val, opts...)
	}
	return &clientv3.PutResponse{}, nil
}

func (m *mockAPI) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	m.gets++
	if m.getFn != nil {
		return m.getFn(ctx, key, opts...)
	}
	return &clientv3.GetResponse{
		Kvs: []*mvccpb.KeyValue{{Key: []byte(key), Value: []byte("v")}},
	}, nil
}

func (m *mockAPI) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	m.deletes++
	if m.deleteFn != nil {
		return m.deleteFn(ctx, key, opts...)
	}
	return &clientv3.DeleteResponse{}, nil
}

func (m *mockAPI) Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	m.grants++
	if m.grantFn != nil {
		return m.grantFn(ctx, ttl)
	}
	return &clientv3.LeaseGrantResponse{ID: clientv3.LeaseID(1)}, nil
}

func (m *mockAPI) KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	if m.keepAliveFn != nil {
		return m.keepAliveFn(ctx, id)
	}
	// Default: a closed channel (no keep-alive responses); tests that need
	// responses override keepAliveFn.
	ch := make(chan *clientv3.LeaseKeepAliveResponse)
	close(ch)
	return ch, nil
}

func (m *mockAPI) Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, id)
	}
	return &clientv3.LeaseRevokeResponse{}, nil
}

func (m *mockAPI) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	if m.watchFn != nil {
		return m.watchFn(ctx, key, opts...)
	}
	ch := make(chan clientv3.WatchResponse, 1)
	ch <- clientv3.WatchResponse{Created: true}
	close(ch)
	return ch
}

func (m *mockAPI) Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, endpoint)
	}
	return &clientv3.StatusResponse{}, nil
}

// compile-time: mockAPI satisfies etcdAPI.
var _ etcdAPI = (*mockAPI)(nil)
