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
