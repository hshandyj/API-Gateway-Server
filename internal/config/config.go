package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	if yamlPath == "" {
		return defaultConfig
	}

	// 读取并解析YAML文件
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		fmt.Printf("Warning: Failed to read YAML config: %v\n", err)
		return defaultConfig
	}

	// 解析YAML到配置结构
	if err := yaml.Unmarshal(data, defaultConfig); err != nil {
		fmt.Printf("Warning: Failed to parse YAML config: %v\n", err)
		return defaultConfig
	}

	return defaultConfig
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
		env = os.Getenv("GIN_MODE")
	}
	if env == "" {
		env = "production" // 默认使用生产配置
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
