package wsocket

import "time"

type Config struct {
	WriteWait         time.Duration
	PongWait          time.Duration
	PingPeriod        time.Duration
	MaxMessageSize    int64
	MessageBufferSize int
}

func NewConfig() *Config {
	return &Config{
		WriteWait:         time.Second * 10,
		PongWait:          time.Second * 60,
		PingPeriod:        time.Second * 54,
		MaxMessageSize:    512,
		MessageBufferSize: 256,
	}
}
