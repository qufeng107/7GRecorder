package config

import "os"

type Config struct {
	ListenAddr       string
	PublicBaseURL    string
	DataRoot         string
	SQLitePath       string
	TempRoot         string
	RecorderBaseURL  string
	RecorderUser     string
	RecorderPassword string
	FFmpegPath       string
	MasterKeyPath    string
	LogLevel         string
}

func LoadFromEnv() Config {
	dataRoot := env("DATA_ROOT", "/data/7grecorder")
	return Config{
		ListenAddr:       env("APP_LISTEN_ADDR", ":8080"),
		PublicBaseURL:    env("APP_PUBLIC_BASE_URL", ""),
		DataRoot:         dataRoot,
		SQLitePath:       env("SQLITE_PATH", dataRoot+"/db/7grecorder.db"),
		TempRoot:         env("TEMP_ROOT", dataRoot+"/temp"),
		RecorderBaseURL:  env("RECORDER_BASE_URL", "http://bililiverecorder:2356"),
		RecorderUser:     os.Getenv("RECORDER_BASIC_USER"),
		RecorderPassword: os.Getenv("RECORDER_BASIC_PASSWORD"),
		FFmpegPath:       env("FFMPEG_PATH", "ffmpeg"),
		MasterKeyPath:    env("MASTER_KEY_PATH", "/etc/7grecorder/master.key"),
		LogLevel:         env("LOG_LEVEL", "info"),
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
