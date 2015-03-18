package easyws

import (
	"errors"
	"net/http"
	"time"
)

type WebsocketsHandler interface {
	http.Handler

	Run()
	Send(id int64, b []byte)
	SendJSON(id int64, a interface{}) error
	Broadcast(b []byte)
	BroadcastJSON(a interface{}) error
}

var SyncSendTimeout = errors.New("got timeout waiting for response")

type JSONSender func(something interface{})

type WebsocketTalker interface {
	GetId() int64
	Close()

	Send(x []byte)
	SendJSON(x interface{}) error

	// high-level!
	SendJSONSync(x interface{}, timeout time.Duration) (interface{}, error)
	ExtractJSON(data []byte) (isSyncResponse bool, incoming interface{}, reply JSONSender, maxTime time.Duration)

	GetConfig() *ConnectionConfig
	GetServer() WebsocketsHandler

	GetCloseNotifyChannel() chan struct{}
	IsClosed() bool

	// used to store some user-defined values inside connection object
	// not protected from concurrent access
	GetTag() int64
	SetTag(v int64)
	GetProperties() map[string]interface{}

	// private
	shutdown()
	sendOrFalse(x []byte) bool
}

type WsMessage struct {
	bytes []byte
	err   error
}

type OnWsConnectionStatusChangedHandler func(c WebsocketTalker, status int)
type OnWsMessageHanler func(c WebsocketTalker, b []byte)
type LoggerFunc func(s string)

var WsStatuses = []string{"unknown", "connected", "disconnected"}

const (
	STATUS_UNKNOWN      = iota
	STATUS_CONNECTED    = iota
	STATUS_DISCONNECTED = iota
)
