package kafka

import (
	"encoding/json"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"
)

type Producer struct {
	producer sarama.AsyncProducer
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
	config.Producer.Return.Errors = true
	config.Producer.Compression = sarama.CompressionSnappy

	config.Producer.Flush.Messages = 100
	config.Producer.Flush.Frequency = 500 * time.Millisecond
	config.Producer.Flush.MaxMessages = 1000
	config.ChannelBufferSize = 256

	// 如果提供了认证信息，启用SASL认证（用于Confluent Cloud等云服务）
	if username != "" && password != "" {
		config.Net.SASL.Enable = true
		config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		config.Net.SASL.User = username
		config.Net.SASL.Password = password
		config.Net.TLS.Enable = true
	}

	asyncProducer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	p := &Producer{
		producer: asyncProducer,
		logger:   logger,
	}

	// 启动后台 goroutine 处理异步结果
	go p.handleSuccesses()
	go p.handleErrors()

	return p, nil
}

func (p *Producer) Close() error {
	return p.producer.Close()
}

func (p *Producer) handleSuccesses() {
	for msg := range p.producer.Successes() {
		// msg 包含：
		// - msg.Topic: 发送到哪个 topic
		// - msg.Partition: 写入的分区
		// - msg.Offset: 消息的 offset
		// - msg.Metadata: 可以在发送时附加的元数据

		// 从 metadata 中提取用户信息（需要在发送时设置）
		if meta, ok := msg.Metadata.(*UserAnalyticsEvent); ok {
			p.logger.WithFields(logrus.Fields{
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"user_id":   meta.UserID,
				"action":    meta.Action,
			}).Debug("Message sent successfully")
		}
	}
}

func (p *Producer) handleErrors() {
	for err := range p.producer.Errors() {
		// err 包含：
		// - err.Msg: 失败的消息
		// - err.Err: 错误原因

		// 从 metadata 中提取用户信息
		if meta, ok := err.Msg.Metadata.(*UserAnalyticsEvent); ok {
			p.logger.WithFields(logrus.Fields{
				"error":   err.Err,
				"user_id": meta.UserID,
				"action":  meta.Action,
				"topic":   err.Msg.Topic,
			}).Error("Failed to send message to Kafka")

			// ========== 关键决策点 ==========
			// 选项1：记录到本地文件（数据库）等待重试
			// 选项2：发送到死信队列（DLQ）
			// 选项3：触发告警
			// 选项4：丢弃（如果可以容忍数据丢失）
		}
	}
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
		// 设置 Metadata，以便在异步回调中识别消息
		Metadata: event,
	}

	// 异步发送消息到 Kafka（不阻塞，立即返回）
	// 发送结果会通过 handleSuccesses() 或 handleErrors() 异步通知
	p.producer.Input() <- msg

	// 记录消息已提交到发送队列
	p.logger.WithFields(logrus.Fields{
		"user_id": event.UserID,
		"action":  event.Action,
		"topic":   topic,
	}).Debug("Analytics event queued for sending")

	return nil
}
