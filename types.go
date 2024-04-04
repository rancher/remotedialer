package remotedialer

import "time"

const (
	PingWaitDuration        = 60 * time.Second
	PingWriteInterval       = 5 * time.Second
	SyncConnectionsInterval = 60 * time.Second
	SyncConnectionsTimeout  = 60 * time.Second
	MaxRead                 = 8192
	HandshakeTimeOut        = 10 * time.Second
)
