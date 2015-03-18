package easyws

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"time"
)

type ConnectionConfig struct {
	OnMessage                 OnWsMessageHanler
	OnConnectionStatusChanged OnWsConnectionStatusChangedHandler

	unregisterChannel chan WebsocketTalker

	// Time allowed to write a message to the peer.
	WriteWait time.Duration
	// Time allowed to read the next pong message from the peer.
	PongWait time.Duration
	// Send pings to peer with this period. Must be less than pongWait.
	PingPeriod time.Duration
	// Maximum message size allowed from peer.
	MaxMessageSize int

	UpgraderReadBufferSize  int
	UpgraderWriteBufferSize int

	SendChannelSize int
}

func SetupConnectionConfig(conf *ConnectionConfig) *ConnectionConfig {
	ensureNotEmptyInt(&conf.UpgraderReadBufferSize, 1024)
	ensureNotEmptyInt(&conf.UpgraderWriteBufferSize, 1024)
	ensureNotEmptyInt(&conf.SendChannelSize, 255)
	if conf.OnMessage == nil {
		conf.OnMessage = func(c WebsocketTalker, b []byte) {
			isSyncResponse, somejson, reply, _ := c.ExtractJSON(b)
			if reply != nil { // pong
				reply(somejson)
				return
			}

			msg := "message"
			if isSyncResponse {
				msg = "response"
			}
			log.Println(fmt.Sprint("WS: conn [", c.GetId(), "] sent "+msg+": ", string(b)))
		}
	}
	if conf.OnConnectionStatusChanged == nil {
		conf.OnConnectionStatusChanged = func(c WebsocketTalker, status int) {
			log.Println(fmt.Sprint("WS: conn [", c.GetId(), "] status changed: ", WsStatuses[status]))
		}
	}

	ensureNotEmptyDuration(&conf.WriteWait, 10*time.Second)
	ensureNotEmptyDuration(&conf.PongWait, 60*time.Second)
	ensureNotEmptyDuration(&conf.PingPeriod, (conf.PongWait*9)/10)
	ensureNotEmptyInt(&conf.MaxMessageSize, 32000)

	return conf
}

// connection is an middleman between the websocket connection and the hub.
type Connection struct {
	// The websocket connection.
	ws *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	server WebsocketsHandler
	conf   *ConnectionConfig

	Id  int64
	tag int64

	closeChannel chan struct{}

	props  map[string]interface{}
	router MessagesRouter
}

func SetupConnection(c *Connection) *Connection {
	c.props = make(map[string]interface{})
	c.closeChannel = make(chan struct{}, 1)
	c.router = NewMessagesRouter(c.ActuallySendJSONSync, nil)
	return c
}

func (c *Connection) ActuallySendJSONSync(id int, data interface{}, timeout time.Duration) error {
	r := map[string]interface{}{"ReqId": id, "Data": data, "Timeout": int(timeout / time.Millisecond)}
	return c.SendJSON(r)
}
func (c *Connection) SendJSONSync(x interface{}, timeout time.Duration) (interface{}, error) {
	answer, err := c.router.SendMessage(x, timeout)
	return answer, err
}
func (c *Connection) ExtractJSON(bytes []byte) (isSyncResponse bool, incoming interface{}, reply JSONSender, maxTime time.Duration) {
	m := make(map[string]interface{})
	err := json.Unmarshal(bytes, &m)
	if err != nil {
		return false, nil, nil, 0
	}

	reqid, reqidok := m["ReqId"]
	data, _ := m["Data"]
	timeoutms, _ := m["Timeout"]

	if reqidok {
		// it is sync request, i'll need to answer it via special func
		_timeoutms, _ := IntValue(timeoutms)
		return false, data, func(some interface{}) {
			_reqid, _ := IntValue(reqid)
			go c.sendJsonSyncResponse(_reqid, some)
		}, time.Duration(int(time.Millisecond) * _timeoutms)
	}

	if respid, respidok := m["RespId"]; respidok {
		_respid, _ := IntValue(respid)
		c.router.ProcessIncomingMessage(_respid, data)
		return true, data, nil, 0
	}

	return false, data, nil, 0
}
func (c *Connection) sendJsonSyncResponse(id int, data interface{}) {
	resp := map[string]interface{}{
		"RespId": id,
		"Data":   data,
	}
	c.SendJSON(resp)
}

func (c *Connection) getSendChannel() chan []byte {
	return c.send
}
func (c *Connection) GetConfig() *ConnectionConfig {
	return c.conf
}
func (c *Connection) GetId() int64 {
	return c.Id
}
func (c *Connection) GetServer() WebsocketsHandler {
	return c.server
}

func (c *Connection) GetCloseNotifyChannel() chan struct{} {
	return c.closeChannel
}
func (c *Connection) IsClosed() bool {
	select {
	case <-c.GetCloseNotifyChannel():
		return true
	default:
		return false
	}
}

func (c *Connection) GetTag() int64 {
	return c.tag
}
func (c *Connection) SetTag(v int64) {
	c.tag = v
}
func (c *Connection) GetProperties() map[string]interface{} {
	return c.props
}

func (c *Connection) sendOrFalse(b []byte) bool {
	if c.server == nil || c.IsClosed() {
		return false
	}
	select {
	case c.send <- b:
		return true
	default:
		log.Println("websocket conn channel overflow: ", c.Id)
		return false
	}
}
func (c *Connection) Send(b []byte) {
	if !c.sendOrFalse(b) {
		c.Close()
	}
}
func (c *Connection) SendJSON(jsonencodable interface{}) error {
	j, e := json.Marshal(jsonencodable)
	if e == nil {
		c.Send(j)
	}
	return e
}
func (c *Connection) Close() {
	if c.IsClosed() {
		return
	}
	c.router.Shutdown(errors.New("manual ws shutdown"))
	if c.conf.unregisterChannel != nil {
		c.conf.unregisterChannel <- c
	} else { // client connection
		c.shutdown()
	}
}

// internal
func (c *Connection) shutdown() {
	c.conf.OnConnectionStatusChanged(c, STATUS_DISCONNECTED)
	close(c.closeChannel)
	close(c.send)
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Connection) readPump() {
	defer func() {
		c.Close()
		c.ws.Close()
		c.router.Shutdown(nil)
	}()
	c.ws.SetReadLimit(int64(c.conf.MaxMessageSize))
	c.ws.SetReadDeadline(time.Now().Add(c.conf.PongWait))
	c.ws.SetPongHandler(func(string) error { c.ws.SetReadDeadline(time.Now().Add(c.conf.PongWait)); return nil })

	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		c.conf.OnMessage(c, message)
		//h.broadcast <- message
	}
}

// write writes a message with the given message type and payload.
func (c *Connection) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.conf.WriteWait))
	return c.ws.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Connection) writePump() {
	ticker := time.NewTicker(c.conf.PingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}
