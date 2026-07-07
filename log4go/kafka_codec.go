package log4go

// KafkaCodec serializes a kafkaPayload to bytes for the Kafka message value.
// Implementations must be zero-alloc on the steady-state hot path (append-based,
// pooled buffer) and CPU-cheap — this runs on the KafkaWriter.Write path at
// 100k–1M+ records/sec.
//
// Two built-in implementations:
//   - KafkaCodecJSON (default): wraps the existing hand-rolled MarshalJSON.
//   - KafkaCodecProto: hand-rolled protobuf wire format (~4× smaller, ~2× faster).
//
// Switch at runtime via SetKafkaCodec (atomic.Pointer, lock-free, hot-path safe).
type KafkaCodec interface {
	// Encode serializes p to a new []byte. Must not retain p after return.
	Encode(p *kafkaPayload) []byte
	// ContentType returns the MIME type for downstream consumers.
	ContentType() string
}

// KafkaCodecJSON wraps the existing hand-rolled MarshalJSON as a KafkaCodec.
// Zero-alloc, zero-reflection, append-based — the default and the fastest
// portable JSON path.
type KafkaCodecJSON struct{}

// Encode serializes p via MarshalJSON.
func (KafkaCodecJSON) Encode(p *kafkaPayload) []byte {
	b, _ := p.MarshalJSON()
	return b
}

// ContentType returns "application/json".
func (KafkaCodecJSON) ContentType() string { return "application/json" }
