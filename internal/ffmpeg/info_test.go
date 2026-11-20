package ffmpeg

import (
	"math"
	"testing"
	"time"
)

func TestParseMediaInfo(t *testing.T) {
	data := []byte(`{
		"format": {
			"duration": "125.5",
			"size": "10485760",
			"bit_rate": "668000",
			"format_name": "mov,mp4,m4a,3gp,3g2,mj2",
			"format_long_name": "QuickTime / MOV"
		},
		"streams": [
			{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,
			 "avg_frame_rate":"30000/1001","r_frame_rate":"30000/1001",
			 "bit_rate":"600000","pix_fmt":"yuv420p","duration":"125.5",
			 "disposition":{"attached_pic":0}},
			{"codec_type":"audio","codec_name":"aac","channels":2,
			 "channel_layout":"stereo","sample_rate":"48000","bit_rate":"128000",
			 "duration":"125.5"}
		]
	}`)
	info, err := parseMediaInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 10485760 || info.Bitrate != 668000 {
		t.Fatalf("size/bitrate = %d/%d", info.Size, info.Bitrate)
	}
	if info.FormatName != "mov,mp4,m4a,3gp,3g2,mj2" {
		t.Fatalf("format = %q", info.FormatName)
	}
	if math.Abs(info.Duration.Seconds()-125.5) > 0.01 {
		t.Fatalf("duration = %v", info.Duration)
	}
	if info.Video == nil || info.Video.Width != 1920 || info.Video.Height != 1080 {
		t.Fatalf("video = %+v", info.Video)
	}
	if math.Abs(info.Video.FPS-29.970029) > 0.01 {
		t.Fatalf("fps = %v", info.Video.FPS)
	}
	if info.Audio == nil || info.Audio.Codec != "aac" || info.Audio.Channels != 2 {
		t.Fatalf("audio = %+v", info.Audio)
	}
	if info.Audio.SampleRate != 48000 || info.Audio.ChannelLayout != "stereo" {
		t.Fatalf("audio detail = %+v", info.Audio)
	}
}

func TestParseMediaInfoIgnoresCoverArt(t *testing.T) {
	data := []byte(`{
		"format":{"duration":"10","size":"100"},
		"streams":[
			{"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,
			 "disposition":{"attached_pic":1}},
			{"codec_type":"video","codec_name":"hevc","width":1280,"height":720,
			 "avg_frame_rate":"25/1","duration":"10","disposition":{"attached_pic":0}},
			{"codec_type":"audio","codec_name":"opus","channels":1,
			 "channel_layout":"mono","sample_rate":"48000"}
		]
	}`)
	info, err := parseMediaInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video == nil || info.Video.Codec != "hevc" {
		t.Fatalf("video = %+v", info.Video)
	}
	if info.Duration != 10*time.Second {
		t.Fatalf("duration = %v", info.Duration)
	}
}

func TestParseMediaInfoFallsBackToStreamDuration(t *testing.T) {
	data := []byte(`{
		"format":{"size":"100"},
		"streams":[
			{"codec_type":"audio","codec_name":"mp3","channels":2,
			 "channel_layout":"stereo","sample_rate":"44100","duration":"42.25"}
		]
	}`)
	info, err := parseMediaInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video != nil {
		t.Fatalf("unexpected video: %+v", info.Video)
	}
	if info.Audio == nil || info.Audio.Codec != "mp3" {
		t.Fatalf("audio = %+v", info.Audio)
	}
	if info.Duration != 42*time.Second+250*time.Millisecond {
		t.Fatalf("duration = %v", info.Duration)
	}
}

func TestMediaInfoNeedsFFprobe(t *testing.T) {
	if _, err := (Tool{}).MediaInfo("x.mp4"); err == nil {
		t.Fatal("expected an error without ffprobe")
	}
}
