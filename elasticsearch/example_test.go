package elasticsearch_test

import (
	"context"
	"log"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/v8fg/kit4go/elasticsearch"
)

// ExampleNew shows connect + Index/Search. It is a compile-checked illustration
// (an Example without an // Output: comment is compiled but not executed); wire
// your own address to run it against a live cluster.
//
// v8.19 options are methods on the named func types: esapi.Index(nil).WithXxx(v)
// builds an option func(*IndexRequest) without invoking the (nil) receiver.
func ExampleNew() {
	ctx := context.Background()
	c, err := elasticsearch.New(ctx, elasticsearch.WithAddresses("http://localhost:9200"))
	if err != nil {
		log.Fatal(err)
	}

	if _, err := c.Index(ctx, "creatives", strings.NewReader(`{"name":"banner-1"}`),
		esapi.Index(nil).WithDocumentID("1"),
	); err != nil {
		log.Fatal(err)
	}

	res, err := c.Search(ctx,
		esapi.Search(nil).WithIndex("creatives"),
		esapi.Search(nil).WithBody(strings.NewReader(`{"query":{"match_all":{}}}`)),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
}
