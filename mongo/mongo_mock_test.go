package mongo

// White-box mock for the collectionAPI interface. Each method delegates to an
// optional function field (tests override per-case); unset fields return sane
// zero-values so happy paths work without wiring.

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mockAPI struct {
	insertOneFn  func(context.Context, any, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
	insertManyFn func(context.Context, []any, ...*options.InsertManyOptions) (*mongo.InsertManyResult, error)
	findFn       func(context.Context, any, ...*options.FindOptions) (*mongo.Cursor, error)
	findOneFn    func(context.Context, any, ...*options.FindOneOptions) *mongo.SingleResult
	updateOneFn  func(context.Context, any, any, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	updateManyFn func(context.Context, any, any, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	deleteOneFn  func(context.Context, any, ...*options.DeleteOptions) (*mongo.DeleteResult, error)
	deleteManyFn func(context.Context, any, ...*options.DeleteOptions) (*mongo.DeleteResult, error)

	inserts, finds, updates, deletes int
}

func (m *mockAPI) InsertOne(ctx context.Context, doc any, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
	m.inserts++
	if m.insertOneFn != nil {
		return m.insertOneFn(ctx, doc, opts...)
	}
	return &mongo.InsertOneResult{InsertedID: "id"}, nil
}

func (m *mockAPI) InsertMany(ctx context.Context, docs []any, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
	m.inserts++
	if m.insertManyFn != nil {
		return m.insertManyFn(ctx, docs, opts...)
	}
	return &mongo.InsertManyResult{InsertedIDs: []any{"id"}}, nil
}

func (m *mockAPI) Find(ctx context.Context, filter any, opts ...*options.FindOptions) (*mongo.Cursor, error) {
	m.finds++
	if m.findFn != nil {
		return m.findFn(ctx, filter, opts...)
	}
	return &mongo.Cursor{}, nil
}

func (m *mockAPI) FindOne(ctx context.Context, filter any, opts ...*options.FindOneOptions) *mongo.SingleResult {
	m.finds++
	if m.findOneFn != nil {
		return m.findOneFn(ctx, filter, opts...)
	}
	return &mongo.SingleResult{}
}

func (m *mockAPI) UpdateOne(ctx context.Context, filter, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	m.updates++
	if m.updateOneFn != nil {
		return m.updateOneFn(ctx, filter, update, opts...)
	}
	return &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil
}

func (m *mockAPI) UpdateMany(ctx context.Context, filter, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	m.updates++
	if m.updateManyFn != nil {
		return m.updateManyFn(ctx, filter, update, opts...)
	}
	return &mongo.UpdateResult{MatchedCount: 2, ModifiedCount: 2}, nil
}

func (m *mockAPI) DeleteOne(ctx context.Context, filter any, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
	m.deletes++
	if m.deleteOneFn != nil {
		return m.deleteOneFn(ctx, filter, opts...)
	}
	return &mongo.DeleteResult{DeletedCount: 1}, nil
}

func (m *mockAPI) DeleteMany(ctx context.Context, filter any, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
	m.deletes++
	if m.deleteManyFn != nil {
		return m.deleteManyFn(ctx, filter, opts...)
	}
	return &mongo.DeleteResult{DeletedCount: 2}, nil
}

// compile-time: mockAPI satisfies collectionAPI.
var _ collectionAPI = (*mockAPI)(nil)
