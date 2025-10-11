package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"ServerConfig"`
	GRPC     GRPCConfig     `yaml:"GRPCConfig"`
	Kafka    KafkaConfig    `yaml:"KafkaConfig"`
	Services ServicesConfig `yaml:"ServicesConfig"`
}

type ServerConfig struct {
	Port string `yaml:"Port"`
}

type GRPCConfig struct {
	AuthServiceAddr string `yaml:"AuthServiceAddr"`
}

type KafkaConfig struct {
	Brokers []string    `yaml:"Brokers"`
	Topics  KafkaTopics `yaml:"Topics"`
	Auth    KafkaAuth   `yaml:"Auth"`
}

type KafkaTopics struct {
	UserAnalytics string `yaml:"UserAnalytics"`
}

type KafkaAuth struct {
	Username string `yaml:"Username"`
	Password string `yaml:"Password"`
}

type ServicesConfig struct {
	DocumentServiceAddr string `yaml:"DocumentServiceAddr"`
}

func Load() *Config {
	// 加载.env文件（如果存在）
	loadEnvFile()

	// 默认配置
	defaultConfig := &Config{
		Server: ServerConfig{
			Port: "8080",
		},
		GRPC: GRPCConfig{
			AuthServiceAddr: "localhost:50051",
		},
		Kafka: KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Topics: KafkaTopics{
				UserAnalytics: "user-analytics",
			},
		},
		Services: ServicesConfig{
			DocumentServiceAddr: "ws://localhost:8081/ws",
		},
	}

	// 尝试加载YAML配置
	yamlPath := findYAMLConfig()
	if yamlPath != "" {
		// 读取并解析YAML文件
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			fmt.Printf("Warning: Failed to read YAML config: %v\n", err)
		} else {
			// 解析YAML到配置结构
			if err := yaml.Unmarshal(data, defaultConfig); err != nil {
				fmt.Printf("Warning: Failed to parse YAML config: %v\n", err)
			}
		}
	}

	if kafkaUsername := os.Getenv("KAFKA_USERNANME"); kafkaUsername != "" {
		defaultConfig.Kafka.Auth.Username = kafkaUsername
	}

	if kafkaPassword := os.Getenv("KAFKA_PASSWORD"); kafkaPassword != "" {
		defaultConfig.Kafka.Auth.Password = kafkaPassword
	}

	if kafkaBrokers := os.Getenv("KAFKA_BROKERS"); kafkaBrokers != "" {
		defaultConfig.Kafka.Brokers = strings.Split(kafkaBrokers, ",")
	}

	return defaultConfig
}

// 加载.env文件
func loadEnvFile() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	// 尝试加载.env文件
	envPath := filepath.Join(dir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		if err := godotenv.Load(envPath); err != nil {
			fmt.Printf("Warning: Failed to load .env file: %v\n", err)
		}
	}
}

// 查找YAML配置文件
func findYAMLConfig() string {
	// 获取当前工作目录
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	// 根据环境变量选择配置文件
	// 优先级：GO_ENV > GIN_MODE > 默认
	env := os.Getenv("GO_ENV")
	if env == "" {
		env = "production"
	}

	// 配置文件命名规则：
	// - APIGatewayConfig.yaml (默认/生产)
	// - APIGatewayConfigDebug.yaml (开发/调试)
	var configFile string
	switch env {
	case "debug", "development", "dev":
		configFile = "APIGatewayConfigDebug.yaml"
	case "production", "prod", "release":
		configFile = "APIGatewayConfig.yaml"
	default:
		// 尝试使用环境名称作为后缀
		configFile = "APIGatewayConfig.yaml"
	}

	// 尝试查找配置文件
	path := filepath.Join(dir, "internal", "config", configFile)
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Loading config from: %s (env=%s)\n", configFile, env)
		return path
	}

	return ""
}
