package elasticsearch

// Integration test against a live Elasticsearch cluster. Skipped under -short
// and unless ELASTICSEARCH_ADDR is set. Run locally with, e.g.:
//
//	docker run -d -p 9200:9200 -e discovery.type=single-node -e xpack.security.enabled=false \
//	  docker.elastic.co/elasticsearch/elasticsearch:8.19.0
//	ELASTICSEARCH_ADDR=http://127.0.0.1:9200 go test -run Integration -v ./elasticsearch/

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

func TestIntegration_CRUDRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}
	addr := os.Getenv("ELASTICSEARCH_ADDR")
	if addr == "" {
		t.Skip("ELASTICSEARCH_ADDR not set")
	}

	c, err := New(WithAddresses(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.Index("kit4go-it", strings.NewReader(`{"k":"v1"}`),
		esapi.Index(nil).WithDocumentID("1"),
	); err != nil {
		t.Fatalf("Index: %v", err)
	}

	res, err := c.Get("kit4go-it", "1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "v1") {
		t.Fatalf("Get body = %s, want to contain v1", body)
	}

	if _, err := c.Search(esapi.Search(nil).WithIndex("kit4go-it")); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if _, err := c.Delete("kit4go-it", "1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	m := c.Metrics()
	if m.Indexes == 0 || m.Gets == 0 || m.Searches == 0 || m.Deletes == 0 || m.Errors != 0 {
		t.Fatalf("metrics after round-trip: %+v", m)
	}
}
