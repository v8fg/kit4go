//go:build !franzgo

package kafka

import (
	"github.com/IBM/sarama"
)

// toSaramaProducerMessage maps a library Message to a sarama ProducerMessage.
// Topic falls back to defTopic (the Options.Topic) when msg.Topic is empty, so a
// producer constructed WithTopic can Send messages that omit Topic. Key/Value
// are passed through; Headers are copied. Partition/Offset/Timestamp are
// broker-assigned and therefore ignored on the produce path.
func toSaramaProducerMessage(msg Message, defTopic string) *sarama.ProducerMessage {
	topic := msg.Topic
	if topic == "" {
		topic = defTopic
	}
	pm := &sarama.ProducerMessage{
		Topic: topic,
	}
	if len(msg.Key) > 0 {
		pm.Key = sarama.ByteEncoder(msg.Key)
	}
	if len(msg.Value) > 0 {
		pm.Value = sarama.ByteEncoder(msg.Value)
	}
	if n := len(msg.Headers); n > 0 {
		hdrs := make([]sarama.RecordHeader, n)
		for i, h := range msg.Headers {
			hdrs[i] = sarama.RecordHeader{Key: h.Key, Value: h.Value}
		}
		pm.Headers = hdrs
	}
	return pm
}

// (consumer-side mapping fromSaramaConsumerMessage + the ConsumerGroupHandler
// adapter are added in the consumer phase — sarama_consumer_group.go /
// sarama_partition_consumer.go.)

// fromSaramaConsumerMessage maps a sarama ConsumerMessage to a library Message.
func fromSaramaConsumerMessage(cm *sarama.ConsumerMessage) Message {
	m := Message{
		Topic:     cm.Topic,
		Partition: cm.Partition,
		Offset:    cm.Offset,
		Key:       cm.Key,
		Value:     cm.Value,
		Timestamp: cm.Timestamp,
	}
	if n := len(cm.Headers); n > 0 {
		m.Headers = make([]Header, n)
		for i, h := range cm.Headers {
			m.Headers[i] = Header{Key: h.Key, Value: h.Value}
		}
	}
	return m
}

// cgHandler adapts a library MessageHandler to sarama.ConsumerGroupHandler. On a
// nil handler result it MarkMessage's (ACK/commit); on a non-nil result it does
// NOT mark (NACK) and surfaces the failure via the parent's accounting. It
// implements at-least-once semantics — a NACK'd message is re-delivered next
// session, so the handler must be idempotent or the caller must retry.
type cgHandler struct {
	parent  *saramaConsumerGroup
	handler MessageHandler
}

func (h *cgHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *cgHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *cgHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for cm := range claim.Messages() {
		msg := fromSaramaConsumerMessage(cm)
		h.parent.bumpReceived(len(cm.Value))
		h.parent.fire(ConsumerEvent{Name: "message", Msg: msg})
		if err := h.handler(msg); err != nil {
			h.parent.failed.Add(1)
			h.parent.fire(ConsumerEvent{Name: "nack", Msg: msg, Err: err})
			continue // NACK: do not MarkMessage; re-delivered next session
		}
		sess.MarkMessage(cm, "")
		h.parent.acked.Add(1)
		h.parent.fire(ConsumerEvent{Name: "ack", Msg: msg})
	}
	return nil
}
