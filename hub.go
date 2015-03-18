package easyws

//====================================================================

// hub maintains the set of active connections and broadcasts messages to connections.
type wshub struct {
	// Registered connections.
	connections map[int64]WebsocketTalker
	server      *WsServer

	// Inbound messages from the connections.
	broadcast chan []byte

	// send message directly to some connection
	send chan *MessageToSend

	// Register requests from the connections.
	register chan WebsocketTalker

	// Unregister requests from connections.
	unregister chan WebsocketTalker
}

func MakeHub(s *WsServer) *wshub {
	var h = wshub{
		broadcast:   make(chan []byte),
		register:    make(chan WebsocketTalker),
		unregister:  make(chan WebsocketTalker),
		connections: make(map[int64]WebsocketTalker),
		server:      s,
	}
	return &h
}

func (h *wshub) _deleteFromHubAndShutdown(c WebsocketTalker) {
	if _, ok := h.connections[c.GetId()]; ok {
		delete(h.connections, c.GetId())
		c.shutdown()
	}
}
func (h *wshub) run() {
	var todel = []WebsocketTalker{}
	for {
		select {
		case c := <-h.register:
			// TODO unregister prev connection if exists or change id :)
			h.connections[c.GetId()] = c
			go h.server.conf.OnConnectionStatusChanged(c, STATUS_CONNECTED)
		case c := <-h.unregister:
			h._deleteFromHubAndShutdown(c)
		case m := <-h.broadcast:
			for _, c := range h.connections {
				if !c.sendOrFalse(m) {
					todel = append(todel, c)
				}
			}
		case m := <-h.send:
			if c, ok := h.connections[m.Receiverid]; ok {
				if !c.sendOrFalse(m.Bytes) {
					todel = append(todel, c)
				}
			}
		}

		if len(todel) > 0 {
			for _, v := range todel {
				h._deleteFromHubAndShutdown(v)
			}
			todel = []WebsocketTalker{}
		}

	}
}
