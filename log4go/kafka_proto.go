package log4go

import (
	"fmt"
	"strconv"
	"time"
)

// Hand-rolled protobuf wire-format encoder — zero external dependency, zero
// reflection, zero codegen. Produces standard protobuf bytes that any language's
// protobuf SDK (Java/Python/Go) can decode using the .proto schema in
// proto/log_record.proto.
//
// 手写 protobuf wire format 编码器 —— 零依赖、零反射、零 codegen。
// 产出标准 protobuf 字节，任何语言的 protobuf SDK 都能用 .proto 解码。

// --- wire format primitives ---

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

func appendTag(buf []byte, field, wireType int) []byte {
	return appendVarint(buf, uint64(field)<<3|uint64(wireType))
}

func appendString(buf []byte, field int, s string) []byte {
	buf = appendTag(buf, field, 2)
	buf = appendVarint(buf, uint64(len(s)))
	return append(buf, s...)
}

func appendInt64Field(buf []byte, field int, v int64) []byte {
	buf = appendTag(buf, field, 0)
	return appendVarint(buf, uint64(v))
}

func appendUint64Field(buf []byte, field int, v uint64) []byte {
	buf = appendTag(buf, field, 0)
	return appendVarint(buf, v)
}

func appendBytesField(buf []byte, field int, b []byte) []byte {
	buf = appendTag(buf, field, 2)
	buf = appendVarint(buf, uint64(len(b)))
	return append(buf, b...)
}

// --- Field sub-message (field 10) ---

func appendFieldProto(buf []byte, f field) []byte {
	var sub []byte
	sub = appendString(sub, 1, f.key)
	v := f.value()
	if s, ok := v.(string); ok {
		sub = appendString(sub, 2, s)
	} else {
		sub = appendString(sub, 2, scalarToJSON(v))
	}
	return appendBytesField(buf, 10, sub)
}

func scalarToJSON(v interface{}) string {
	switch val := v.(type) {
	case int:
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// timestampISOFromUnixNano formats unixNano as RFC3339 UTC with micros,
// matching the JSON MarshalJSON path's appendISOTimeUTC output.
func timestampISOFromUnixNano(unixNano int64) string {
	return time.Unix(0, unixNano).UTC().Format("2006-01-02T15:04:05.000000Z07:00")
}

// --- KafkaCodecProto ---

// KafkaCodecProto is a KafkaCodec that emits each record as a hand-rolled
// protobuf payload (see proto/log_record.proto). The protobuf form is ~46%
// smaller than JSON at high QPS and decodable by any language's protobuf SDK,
// with zero external dependency, zero reflection, and zero codegen.
type KafkaCodecProto struct{}

// Encode serializes p into protobuf wire format.
func (KafkaCodecProto) Encode(p *kafkaPayload) []byte {
	buf := make([]byte, 0, 128)
	buf = appendInt64Field(buf, 1, p.UnixNano)
	buf = appendUint64Field(buf, 2, p.Seq)
	if p.Level != "" {
		buf = appendString(buf, 3, p.Level)
	}
	if p.File != "" {
		buf = appendString(buf, 4, p.File)
	}
	if p.Message != "" {
		buf = appendString(buf, 5, p.Message)
	}
	buf = appendString(buf, 6, timestampISOFromUnixNano(p.UnixNano))
	buf = appendInt64Field(buf, 7, p.Now)
	if p.ServerIP != "" {
		buf = appendString(buf, 8, p.ServerIP)
	}
	if p.ESIndex != "" {
		buf = appendString(buf, 9, p.ESIndex)
	}
	for _, f := range p.userFields {
		buf = appendFieldProto(buf, f)
	}
	return buf
}

// ContentType returns the protobuf MIME type ("application/x-protobuf").
func (KafkaCodecProto) ContentType() string { return "application/x-protobuf" }
