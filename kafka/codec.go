package kafka

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// CodecJSON (de)serialises Message.Value as JSON. It uses encoding/json by
// default; callers wanting a faster codec (goccy/sonic) can implement Codec
// themselves — the interface is open.
type CodecJSON struct{}

// Encode marshals v as JSON.
func (CodecJSON) Encode(v any) ([]byte, error) { return json.Marshal(v) }

// Decode unmarshals b into out (a pointer).
func (CodecJSON) Decode(b []byte, out any) error { return json.Unmarshal(b, out) }

// ContentType returns "application/json".
func (CodecJSON) ContentType() string { return "application/json" }

// CodecProto (de)serialises Message.Value as protobuf. It uses
// google.golang.org/protobuf/proto (proto.Marshal / proto.Unmarshal), so out
// must be a concrete proto.Message. (pulled in transitively by sarama's grpc
// usage; if you want zero protobuf dep, use a custom Codec.)
type CodecProto struct{}

// Encode marshals v via proto.Marshal. v must be a proto.Message.
func (CodecProto) Encode(v any) ([]byte, error) {
	pm, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("kafka: CodecProto.Encode: %T is not a proto.Message", v)
	}
	return proto.Marshal(pm)
}

// Decode unmarshals b into out via proto.Unmarshal. out must be a proto.Message.
func (CodecProto) Decode(b []byte, out any) error {
	pm, ok := out.(proto.Message)
	if !ok {
		return fmt.Errorf("kafka: CodecProto.Decode: %T is not a proto.Message", out)
	}
	return proto.Unmarshal(b, pm)
}

// ContentType returns "application/x-protobuf".
func (CodecProto) ContentType() string { return "application/x-protobuf" }

// CodecRaw is the identity codec — bytes in, bytes out. Passing nil as Options
// Codec is equivalent (raw pass-through), but CodecRaw is useful when you want
// an explicit, non-nil value (e.g. ContentType() for headers).
type CodecRaw struct{}

// Encode returns v as []byte if it is already bytes, else JSON-marshals it.
func (CodecRaw) Encode(v any) ([]byte, error) {
	if b, ok := v.([]byte); ok {
		return b, nil
	}
	return json.Marshal(v)
}

// Decode copies b into out if out is *[]byte, else JSON-unmarshals.
func (CodecRaw) Decode(b []byte, out any) error {
	if p, ok := out.(*[]byte); ok {
		*p = append((*p)[:0], b...)
		return nil
	}
	return json.Unmarshal(b, out)
}

// ContentType returns "application/octet-stream".
func (CodecRaw) ContentType() string { return "application/octet-stream" }
