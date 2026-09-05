package recorder

type DesiredProfile struct {
	ProfileID		int64	`json:"profile_id"`
	RoomID			string	`json:"room_id"`
	LiveURL			string	`json:"live_url"`
	Enabled			bool	`json:"enabled"`
	AutoRecord		bool	`json:"auto_record"`
	Quality			string	`json:"quality"`
	RecordDanmaku		bool	`json:"record_danmaku"`
	SegmentDurationSec	int64	`json:"segment_duration_sec"`
	FinalizeGracePeriodSec	int64	`json:"finalize_grace_period_sec"`
	OutputRelativeDir	string	`json:"output_relative_dir"`
	WebhookPath		string	`json:"webhook_path"`
}

type RuntimeStatus struct {
	StreamStatus   string
	RecorderStatus string
}
