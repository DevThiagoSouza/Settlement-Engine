package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort int
	HTTPHost string

	DatabaseURL string

	LogLevel string

	KafkaBrokers          string
	KafkaTopic            string
	KafkaConnectionString string

	Environment string
	JWTSecret   string
}

func Load() *Config {
	_ = godotenv.Load()
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL == "" {
		databaseURL = buildDatabaseURL()
	}

	return &Config{
		HTTPPort:              getEnvInt("HTTP_PORT", 8080),
		HTTPHost:              getEnv("HTTP_HOST", "0.0.0.0"),
		DatabaseURL:           databaseURL,
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		KafkaBrokers:          getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:            getEnvWithFallback("KAFKA_TOPIC", "KAFKA_TOPIC_EVENTS", "settlement_events"),
		KafkaConnectionString: getEnv("KAFKA_CONNECTION_STRING", ""),
		Environment:           getEnvWithFallback("ENVIRONMENT", "APP_ENV", "development"),
		JWTSecret:             getEnv("JWT_SECRET", ""),
	}
}

func buildDatabaseURL() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "settlement_user")
	password := getEnv("DB_PASSWORD", "settlement_password")
	database := getEnv("DB_NAME", "settlement_engine")
	sslMode := getEnv("DB_SSL_MODE", "disable")
	return "postgres://" + user + ":" + password + "@" + host + ":" + port + "/" + database + "?sslmode=" + sslMode
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvWithFallback(primary, fallback, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	return getEnv(fallback, defaultValue)
}
