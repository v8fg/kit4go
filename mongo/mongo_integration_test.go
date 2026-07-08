package mongo

// Integration test against a live MongoDB. Skipped under -short and unless
// MONGO_URI is set. Run locally with, e.g.:
//
//	docker run -d -p 27017:27017 mongo:7
//	MONGO_URI=mongodb://127.0.0.1:27017 go test -run Integration -v ./mongo/

import (
	"context"
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestIntegration_CRUDRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI not set")
	}

	ctx := context.Background()
	c, err := New(ctx, WithURI(uri), WithDatabase("kit4go_it"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Disconnect(ctx)

	col := c.Collection("", "things")

	res, err := col.InsertOne(ctx, bson.M{"k": "v1"})
	if err != nil {
		t.Fatalf("InsertOne: %v", err)
	}

	if _, err := col.UpdateOne(ctx, bson.M{"_id": res.InsertedID}, bson.M{"$set": bson.M{"k": "v2"}}); err != nil {
		t.Fatalf("UpdateOne: %v", err)
	}

	cur, err := col.Find(ctx, bson.M{"_id": res.InsertedID})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		t.Fatalf("cursor All: %v", err)
	}
	if len(docs) != 1 || docs[0]["k"] != "v2" {
		t.Fatalf("Find result = %+v, want one doc k=v2", docs)
	}

	if _, err := col.DeleteOne(ctx, bson.M{"_id": res.InsertedID}); err != nil {
		t.Fatalf("DeleteOne: %v", err)
	}

	m := c.Metrics()
	if m.Inserts == 0 || m.Finds == 0 || m.Updates == 0 || m.Deletes == 0 || m.Errors != 0 {
		t.Fatalf("metrics after round-trip: %+v", m)
	}
}
