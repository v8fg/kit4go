package aerospike

// White-box mock for the asAPI interface. Methods return as.Error; the success
// path returns nil, the error path returns asErrSentinel (a real as.Error
// obtained from a public aerospike function, since as.Error has unexported
// methods and cannot be constructed directly).

import (
	as "github.com/aerospike/aerospike-client-go/v8"
)

// asErrSentinel is a real as.Error used for mock error paths.
var asErrSentinel as.Error = mustSentinelErr()

func mustSentinelErr() as.Error {
	_, err := as.Base64ToCDTContext("@@@not-base64@@@") // invalid base64 -> PARSE_ERROR
	if err == nil {
		panic("expected a non-nil as.Error sentinel")
	}
	return err
}

type mockAPI struct {
	putFn       func(*as.WritePolicy, *as.Key, as.BinMap) as.Error
	getFn       func(*as.BasePolicy, *as.Key, ...string) (*as.Record, as.Error)
	deleteFn    func(*as.WritePolicy, *as.Key) (bool, as.Error)
	batchGetFn  func(*as.BatchPolicy, []*as.Key, ...string) ([]*as.Record, as.Error)
	closeCalled bool

	puts, gets, deletes int
}

func (m *mockAPI) Put(p *as.WritePolicy, k *as.Key, b as.BinMap) as.Error {
	m.puts++
	if m.putFn != nil {
		return m.putFn(p, k, b)
	}
	return nil
}

func (m *mockAPI) Get(p *as.BasePolicy, k *as.Key, binNames ...string) (*as.Record, as.Error) {
	m.gets++
	if m.getFn != nil {
		return m.getFn(p, k, binNames...)
	}
	return &as.Record{Key: k}, nil
}

func (m *mockAPI) Delete(p *as.WritePolicy, k *as.Key) (bool, as.Error) {
	m.deletes++
	if m.deleteFn != nil {
		return m.deleteFn(p, k)
	}
	return true, nil
}

func (m *mockAPI) BatchGet(p *as.BatchPolicy, keys []*as.Key, binNames ...string) ([]*as.Record, as.Error) {
	m.gets++
	if m.batchGetFn != nil {
		return m.batchGetFn(p, keys, binNames...)
	}
	return []*as.Record{}, nil
}

func (m *mockAPI) Close() { m.closeCalled = true }

// compile-time: mockAPI satisfies asAPI.
var _ asAPI = (*mockAPI)(nil)
