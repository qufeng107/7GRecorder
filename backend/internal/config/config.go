package config

import "os"

type Config struct {
	ListenAddr                string
	PublicBaseURL             string
	DataRoot                  string
	SQLitePath                string
	TempRoot                  string
	RecorderBaseURL           string
	RecorderUser              string
	RecorderPassword          string
	FFmpegPath                string
	BiliupPath                string
	MasterKeyPath             string
	UploadMaxPartBytes        int64
	UploadMaxPartDurationSecs int64
	COSUploadMaxBytesPerSec   int64
	COSCompressionEnabled     bool
	COSCompressionPreset      string
	COSCompressionThreads     int64
	LogLevel                  string
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
		BiliupPath:                env("BILIUP_PATH", "biliup"),
		MasterKeyPath:             env("MASTER_KEY_PATH", "/etc/7grecorder/master.key"),
		UploadMaxPartBytes:        envInt64("UPLOAD_MAX_PART_BYTES", 3800000000),
		UploadMaxPartDurationSecs: envInt64("UPLOAD_MAX_PART_DURATION_SECONDS", 7200),
		COSUploadMaxBytesPerSec:   envInt64("COS_UPLOAD_MAX_BYTES_PER_SECOND", 0),
		COSCompressionEnabled:     envBool("COS_COMPRESSION_ENABLED", true),
		COSCompressionPreset:      env("COS_COMPRESSION_PRESET", "h264_crf23_medium_mp4"),
		COSCompressionThreads:     envInt64("COS_COMPRESSION_THREADS", 2),
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

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	switch value {
	case "":
		return fallback
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}
