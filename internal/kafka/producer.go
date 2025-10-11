package kafka

import (
	"encoding/json"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"
)

type Producer struct {
	producer sarama.SyncProducer
	logger   *logrus.Logger
}

type UserAnalyticsEvent struct {
	UserID    string                 `json:"user_id"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
	IP        string                 `json:"ip"`
	UserAgent string                 `json:"user_agent"`
}

func NewProducer(brokers []string, username, password string) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true
	config.Producer.Compression = sarama.CompressionSnappy

	// 如果提供了认证信息，启用SASL认证（用于Confluent Cloud等云服务）
	if username != "" && password != "" {
		config.Net.SASL.Enable = true
		config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		config.Net.SASL.User = username
		config.Net.SASL.Password = password
		config.Net.TLS.Enable = true
	}

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &Producer{
		producer: producer,
		logger:   logger,
	}, nil
}

func (p *Producer) Close() error {
	return p.producer.Close()
}

func (p *Producer) SendUserAnalyticsEvent(topic string, event *UserAnalyticsEvent) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		p.logger.WithError(err).Error("Failed to marshal analytics event")
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(event.UserID),
		Value: sarama.ByteEncoder(eventBytes),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("action"),
				Value: []byte(event.Action),
			},
			{
				Key:   []byte("timestamp"),
				Value: []byte(event.Timestamp.Format(time.RFC3339)),
			},
		},
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.WithError(err).Error("Failed to send analytics event to Kafka")
		return err
	}

	p.logger.WithFields(logrus.Fields{
		"partition": partition,
		"offset":    offset,
		"user_id":   event.UserID,
		"action":    event.Action,
	}).Info("Analytics event sent to Kafka successfully")

	return nil
}
