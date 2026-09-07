package config

import "os"

type Config struct {
	ListenAddr                 string
	PublicBaseURL              string
	DataRoot                   string
	SQLitePath                 string
	TempRoot                   string
	RecorderBaseURL            string
	RecorderUser               string
	RecorderPassword           string
	FFmpegPath                 string
	MasterKeyPath              string
	UploadMaxPartBytes         int64
	UploadMaxPartDurationSecs  int64
	LogLevel                   string
}

func LoadFromEnv() Config {
	dataRoot := env("DATA_ROOT", "/data/7grecorder")
	return Config{
		ListenAddr:                env("APP_LISTEN_ADDR", ":8080"),
		PublicBaseURL:             env("APP_PUBLIC_BASE_URL", ""),
		DataRoot:                  dataRoot,
		SQLitePath:                env("SQLITE_PATH", dataRoot+"/db/7grecorder.db"),
		TempRoot:                  env("TEMP_ROOT", dataRoot+"/temp"),
		RecorderBaseURL:           env("RECORDER_BASE_URL", "http://bililiverecorder:2356"),
		RecorderUser:              os.Getenv("RECORDER_BASIC_USER"),
		RecorderPassword:          os.Getenv("RECORDER_BASIC_PASSWORD"),
		FFmpegPath:                env("FFMPEG_PATH", "ffmpeg"),
		MasterKeyPath:             env("MASTER_KEY_PATH", "/etc/7grecorder/master.key"),
		UploadMaxPartBytes:        envInt64("UPLOAD_MAX_PART_BYTES", 4*1024*1024*1024),
		UploadMaxPartDurationSecs: envInt64("UPLOAD_MAX_PART_DURATION_SECONDS", 7200),
		LogLevel:                  env("LOG_LEVEL", "info"),
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int64
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return fallback
		}
		parsed = parsed*10 + int64(ch-'0')
	}
	if parsed <= 0 {
		return fallback
	}
	return parsed
}
