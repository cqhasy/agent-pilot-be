package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Addr string

	FeishuBaseURL           string
	FeishuAppID             string
	FeishuAppSecret         string
	FeishuEventMode         string
	FeishuVerificationToken string
	FeishuEncryptKey        string
	FeishuRedirectURI       string
	StateSecret             string

	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string

	JwtConf           *JWTConfig
	Logconf           *LogConfig
	CorMiddlewareConf *CorMiddlewareConfig

	AssistantName string

	MongoDBUri string
	Database   string
}

type JWTConfig struct {
	SecretKey string `yaml:"secretKey"` //秘钥
	Timeout   int    `yaml:"timeout"`   //过期时间
}

type LogConfig struct {
	Path       string `yaml:"path"`
	MaxSize    int    `yaml:"maxSize"`    // 每个日志文件的最大大小，单位：MB
	MaxBackups int    `yaml:"maxBackups"` // 保留旧日志文件的最大个数
	MaxAge     int    `yaml:"maxAge"`     // 保留旧日志文件的最大天数
	Compress   int    `yaml:"compress"`   // 是否压缩旧的日志文件
}

type CorMiddlewareConfig struct {
	AllowedOrigins []string `yaml:"allowedOrigins"`
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		Addr: getEnv("ADDR", "0.0.0.0:8080"),

		FeishuBaseURL:           getEnv("FEISHU_BASE_URL", "https://open.feishu.cn"),
		FeishuAppID:             os.Getenv("FEISHU_APP_ID"),
		FeishuAppSecret:         os.Getenv("FEISHU_APP_SECRET"),
		FeishuEventMode:         getEnv("FEISHU_EVENT_MODE", "webhook"),
		FeishuVerificationToken: os.Getenv("FEISHU_VERIFICATION_TOKEN"),
		FeishuEncryptKey:        os.Getenv("FEISHU_ENCRYPT_KEY"),
		FeishuRedirectURI:       os.Getenv("FEISHU_REDIRECT_URI"),
		StateSecret:             getEnv("STATE_SECRET", os.Getenv("FEISHU_APP_SECRET")),

		OpenAIAPIKey:  os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL: getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel:   getEnv("OPENAI_MODEL", "gpt-4.1-mini"),

		JwtConf: &JWTConfig{
			SecretKey: getEnv("JWT_SECRET_KEY", "gyahuhhfiafiahe"),
			Timeout:   getEnvInt("JWT_TIMEOUT", 2592000),
		},

		Logconf: &LogConfig{
			Path:       getEnv("LOG_PATH", "./logs/app.log"),
			MaxSize:    getEnvInt("LOG_MAX_SIZE", 100),
			MaxBackups: getEnvInt("LOG_MAX_BACKUPS", 7),
			MaxAge:     getEnvInt("LOG_MAX_AGE", 30),
			Compress:   getEnvInt("LOG_MAX_COMPRESS", 1),
		},

		CorMiddlewareConf: &CorMiddlewareConfig{
			AllowedOrigins: getEnvStringSliceJSON("ALLOWED_ORIGINS", []string{"*"}),
		},

		AssistantName: getEnv("ASSISTANT_NAME", "Feishu Dev Assistant"),

		MongoDBUri: getEnv("MONGODB_URI", "uri"),
		Database:   getEnv("DATABASE", "database"),
	}

	switch {
	case cfg.FeishuAppID == "":
		return Config{}, fmt.Errorf("FEISHU_APP_ID is required")
	case cfg.FeishuAppSecret == "":
		return Config{}, fmt.Errorf("FEISHU_APP_SECRET is required")
	case cfg.FeishuRedirectURI == "":
		return Config{}, fmt.Errorf("FEISHU_REDIRECT_URI is required")
	case cfg.OpenAIAPIKey == "":
		return Config{}, fmt.Errorf("OPENAI_API_KEY is required")
	case cfg.FeishuEventMode != "webhook" && cfg.FeishuEventMode != "ws":
		return Config{}, fmt.Errorf("FEISHU_EVENT_MODE must be one of: webhook, ws")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		result, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return result
	}
	return fallback
}

func getEnvStringSliceJSON(key string, fallback []string) []string {
	value := os.Getenv(key) // 格式要求：MY_LIST=["a","b","c"]
	if value == "" {
		return fallback
	}

	var result []string
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return fallback
	}

	return result
}
