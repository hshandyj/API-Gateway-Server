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
	Server          ServerConfig         `yaml:"ServerConfig"`
	GRPC            GRPCConfig           `yaml:"GRPCConfig"`
	Kafka           KafkaConfig          `yaml:"KafkaConfig"`
	Services        ServicesConfig       `yaml:"ServicesConfig"`
	WebClientConfig WebClientConfig      `yaml:"WebClientConfig"`
	RateLimit       RateLimitConfig      `yaml:"RateLimitConfig"`
	CircuitBreaker  CircuitBreakerConfig `yaml:"CircuitBreakerConfig"`
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
	DocumentServiceAddr   string `yaml:"DocumentServiceAddr"`
	DocumentServiceWSAddr string `yaml:"DocumentServiceWSAddr"`
	UserinfoServiceAddr   string `yaml:"UserinfoServiceAddr"`
	AuthServiceHTTPAddr   string `yaml:"AuthServiceHTTPAddr"`
}

type WebClientConfig struct {
	AllowedOrigins []string `yaml:"AllowedOrigins"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	GlobalQPS  float64 `yaml:"GlobalQPS"`  // 全局 QPS 上限
	PerIPQPS   float64 `yaml:"PerIPQPS"`   // 单 IP QPS 上限
	PerUserQPS float64 `yaml:"PerUserQPS"` // 单用户 QPS 上限
	BurstSize  int     `yaml:"BurstSize"`  // 令牌桶突发容量
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	FailureThreshold    int `yaml:"FailureThreshold"`    // 连续失败阈值
	SuccessThreshold    int `yaml:"SuccessThreshold"`    // Half-Open 连续成功阈值
	RecoveryTimeoutSec  int `yaml:"RecoveryTimeoutSec"`  // Open→Half-Open 恢复超时（秒）
	HalfOpenMaxRequests int `yaml:"HalfOpenMaxRequests"` // Half-Open 最大放行请求数
}

func Load() *Config {
	// 加载.env文件（如果存在）
	loadEnvFile()

	// 默认配置
	defaultConfig := &Config{}

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

	// 限流配置默认值（YAML 未配置时使用）
	if defaultConfig.RateLimit.GlobalQPS == 0 {
		defaultConfig.RateLimit.GlobalQPS = 1000
	}
	if defaultConfig.RateLimit.PerIPQPS == 0 {
		defaultConfig.RateLimit.PerIPQPS = 100
	}
	if defaultConfig.RateLimit.PerUserQPS == 0 {
		defaultConfig.RateLimit.PerUserQPS = 50
	}
	if defaultConfig.RateLimit.BurstSize == 0 {
		defaultConfig.RateLimit.BurstSize = 20
	}

	// 熔断器配置默认值
	if defaultConfig.CircuitBreaker.FailureThreshold == 0 {
		defaultConfig.CircuitBreaker.FailureThreshold = 5
	}
	if defaultConfig.CircuitBreaker.SuccessThreshold == 0 {
		defaultConfig.CircuitBreaker.SuccessThreshold = 3
	}
	if defaultConfig.CircuitBreaker.RecoveryTimeoutSec == 0 {
		defaultConfig.CircuitBreaker.RecoveryTimeoutSec = 30
	}
	if defaultConfig.CircuitBreaker.HalfOpenMaxRequests == 0 {
		defaultConfig.CircuitBreaker.HalfOpenMaxRequests = 3
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
	env := os.Getenv("GO_ENV")
	if env == "" {
		env = "development"
	}

	// 配置文件命名规则：
	// - APIGatewayConfig.yaml (默认/生产)
	// - APIGatewayConfigDebug.yaml (开发/调试)
	var configFile string
	switch env {
	case "development":
		configFile = "APIGatewayConfigDebug.yaml"
	case "production":
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
