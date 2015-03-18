package easyws

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

/*

conn.SendJSON(x)
conn.Id
conn.Send(x)
conn.Close()

wss.BroadcastJSON(x)
wss.Broadcast(x)
wss is httpHandler

wsc := easyws.SetupWsClient("ws://ip/path/to/url", &easyws.ConnectionConfig{
		OnMessage: func(c *easyws.Connection, b []byte) {
			// optional block
			isSyncResponse, somejson , reply , maxTime := c.ExtractJSON(b)
			if isSyncResponse {
				return;
			}
			if reply != nil {
				// calc response
				resp := somejson
				reply(resp)
				return
			}

			if somejson != nil {
				log.Println("WS.JSONMessage:", somejson)
				// do something?
				return
			}

			log.Println("WS.Message:", string(b))
			// do something?
		},
		// status can be "connected", "disconnected"
		OnConnectionStatusChanged: func(c *easyws.Connection, status string) {
			if status == "connected" {
				history := h.Logger.GetHistory()
				c.SendJSON(history)
			}
		},
	})

wss := easyws.SetupWsServer(&easyws.WsServer{
	Log: func(s string) {
		log.Println("WS:", s)
	}, &easyws.ConnectionConfig{
		OnMessage: func(c *easyws.Connection, b []byte) {
			log.Println("WS.Message:", string(b))
		},
		// status can be "connected", "disconnected"
		OnConnectionStatusChanged: func(c *easyws.Connection, status string) {
			if status == "connected" {
				history := h.Logger.GetHistory()
				c.SendJSON(history)
			}
		},
	}
})
go wss.Run()

// client:

var ws = new WebSocket("ws://localhost:8080/wslogs/x/y/websocket");
ws.onopen = function()   {
	ws.send(JSON.stringify({ action: "helloworld" }));
	console.log("Message is sent...");
};
ws.onmessage = function (evt)    {
	console.log("Message is received...: "+evt.data);
};
ws.onclose = function()   {
	console.log("Connection is closed...");
};

*/

//==================================================== connection

type WsServer struct {
	hub      *wshub
	upgrader *websocket.Upgrader

	conf *ConnectionConfig

	CheckOriginFunc func(r *http.Request) bool

	Log LoggerFunc
}

func SetupWsServer(s *WsServer, conf *ConnectionConfig) *WsServer {
	s.hub = MakeHub(s)

	SetupConnectionConfig(conf)

	s.upgrader = &websocket.Upgrader{
		ReadBufferSize:  conf.UpgraderReadBufferSize,
		WriteBufferSize: conf.UpgraderWriteBufferSize,
	}

	s.upgrader.CheckOrigin = s.CheckOriginFunc
	if s.upgrader.CheckOrigin == nil {
		s.upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	}

	s.conf = conf
	s.conf.unregisterChannel = s.hub.unregister

	return s
}

func (s *WsServer) Broadcast(b []byte) {
	s.hub.broadcast <- b
}
func (s *WsServer) BroadcastJSON(a interface{}) error {
	j, err := json.Marshal(a)
	if err == nil {
		s.Broadcast(j)
	}
	return err
}
func (s *WsServer) Run() {
	s.hub.run()
}

type MessageToSend struct {
	Receiverid int64
	Bytes      []byte
}

func (s *WsServer) Send(id int64, b []byte) {
	s.hub.send <- &MessageToSend{id, b}
}
func (s *WsServer) SendJSON(id int64, b interface{}) error {
	j, err := json.Marshal(b)
	if err == nil {
		s.Send(id, j)
	}
	return err
}

// handles websocket requests from the peer.
func (s *WsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Log(fmt.Sprint(err))
		return
	}
	c := &Connection{send: make(chan []byte, s.conf.SendChannelSize), ws: ws, server: s, conf: s.conf, Id: rand.Int63()}
	SetupConnection(c)
	s.hub.register <- c
	go c.writePump()
	c.readPump()
}

//------------------------------------------------------

func ensureNotEmptyInt(a *int, _default int) {
	if *a == 0 {
		*a = _default
	}
}
func ensureNotEmptyDuration(a *time.Duration, _default time.Duration) {
	if *a == 0 {
		*a = _default
	}
}
