package kafka

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestCodec_JSON_RoundTrip(t *testing.T) {
	c := CodecJSON{}
	if c.ContentType() != "application/json" {
		t.Errorf("ContentType=%q want application/json", c.ContentType())
	}
	type evt struct {
		ID   int      `json:"id"`
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	in := evt{ID: 7, Name: "bid", Tags: []string{"a", "b"}}
	b, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var out evt
	if err := c.Decode(b, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.ID != in.ID || out.Name != in.Name || len(out.Tags) != len(in.Tags) || out.Tags[0] != in.Tags[0] {
		t.Errorf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestCodec_Raw_Bytes(t *testing.T) {
	c := CodecRaw{}
	if c.ContentType() != "application/octet-stream" {
		t.Errorf("ContentType=%q", c.ContentType())
	}
	in := []byte("raw-bytes")
	b, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Equal(b, in) {
		t.Errorf("Encode bytes mismatch")
	}
	var out []byte
	if err := c.Decode(b, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("Decode bytes mismatch: got %q want %q", out, in)
	}
}

func TestCodec_Raw_NonBytesJSONFallback(t *testing.T) {
	c := CodecRaw{}
	b, err := c.Encode(map[string]int{"n": 3})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]int
	if err := c.Decode(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["n"] != 3 {
		t.Errorf("got %v", out)
	}
}

func TestCodec_Proto_RoundTrip(t *testing.T) {
	c := CodecProto{}
	if c.ContentType() != "application/x-protobuf" {
		t.Errorf("ContentType=%q", c.ContentType())
	}
	in := wrapperspb.String("hello-proto")
	b, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out := &wrapperspb.StringValue{}
	if err := c.Decode(b, out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.GetValue() != in.GetValue() {
		t.Errorf("got %q want %q", out.GetValue(), in.GetValue())
	}
	if !proto.Equal(in, out) {
		t.Errorf("proto not equal")
	}
}

func TestCodec_Proto_NotAMessage(t *testing.T) {
	c := CodecProto{}
	if _, err := c.Encode("not-a-proto"); err == nil {
		t.Error("Encode non-proto should error")
	}
	if err := c.Decode([]byte("x"), "not-a-proto"); err == nil {
		t.Error("Decode into non-proto should error")
	}
}
