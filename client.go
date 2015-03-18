package easyws

import (
	"errors"
	"github.com/gorilla/websocket"
	"math/rand"
	"time"
)

type ReconnectionConfig struct {
	waitBeforeRetry time.Duration
}

func ConnectToWebsocket(wsurl string, conf *ConnectionConfig) (WebsocketTalker, error) {
	SetupConnectionConfig(conf)
	d := &websocket.Dialer{ReadBufferSize: conf.UpgraderReadBufferSize, WriteBufferSize: conf.UpgraderWriteBufferSize}
	ws, _, err := d.Dial(wsurl, nil)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return nil, errors.New("ws is nil")
	}

	cc := &Connection{send: make(chan []byte, conf.SendChannelSize), ws: ws, conf: conf, Id: rand.Int63()}
	SetupConnection(cc)

	go cc.writePump()
	go cc.readPump()

	conf.OnConnectionStatusChanged(cc, STATUS_CONNECTED)

	return cc, nil
}
