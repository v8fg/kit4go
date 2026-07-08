package mongo_test

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/v8fg/kit4go/mongo"
)

// ExampleNew shows connect + collection CRUD. It is a compile-checked
// illustration (an Example without an // Output: comment is compiled but not
// executed); wire your own URI to run it against a live MongoDB.
func ExampleNew() {
	ctx := context.Background()
	c, err := mongo.New(ctx, mongo.WithURI("mongodb://localhost:27017"), mongo.WithDatabase("ads"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Disconnect(ctx)

	creatives := c.Collection("", "creatives") // "" -> the WithDatabase default

	if _, err := creatives.InsertOne(ctx, bson.M{"name": "banner-1", "size": "300x250"}); err != nil {
		log.Fatal(err)
	}

	cur, err := creatives.Find(ctx, bson.M{"size": "300x250"})
	if err != nil {
		log.Fatal(err)
	}
	defer cur.Close(ctx)

	var results []bson.M
	if err := cur.All(ctx, &results); err != nil {
		log.Fatal(err)
	}
	_ = results
}
