package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	StorageMounts map[string]string // name -> path
	Password      string
	JwtSecret     string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	return &Config{
		Port:          getEnv("APP_PORT", "3000"),
		StorageMounts: parseStorageMounts(getEnv("STORAGE_MOUNTS", "default:/tmp")),
		Password:      getEnv("PASSWORD", "admin"),
		JwtSecret:     getEnv("JWT_SECRET", "default_secret"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Parse "name1:path1,name2:path2" jadi map
func parseStorageMounts(mountsStr string) map[string]string {
	mounts := make(map[string]string)

	pairs := strings.Split(mountsStr, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			path := strings.TrimSpace(parts[1])
			if name != "" && path != "" {
				mounts[name] = path
			}
		}
	}

	return mounts
}
