package easyws

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

/*
let's imagine we have some full-duplex communication channel
many messages run across it
some messages coming from side 2 are answers to some messages-requests from side 1
while some code on side 1 is waiting for reply, it does not prevent
another code on side 1 to send other requests.. and receive responses
router idea is actually routing all that
*/

var MessagesRouterTimeoutError = errors.New("timeout")
var MessagesRouterShutdownError = errors.New("shutdown")

type MessagesRouterSendFunc func(id int, data interface{}, timeout time.Duration) error
type MessagesRouterUnknownProcessorFunc func(id int, data interface{})

type MessagesRouter interface {
	// called from communication layer
	ProcessIncomingMessage(id int, data interface{}) bool
	// called from code, sync
	SendMessage(data interface{}, timeout time.Duration) (interface{}, error)
	// called from communication layer on shutdown
	Shutdown(err error)
}

func NewMessagesRouter(send MessagesRouterSendFunc, handleunknown MessagesRouterUnknownProcessorFunc) MessagesRouter {
	return &MessagesRouterImpl{
		waiters:         make(map[int]chan interface{}),
		send:            send,
		handleUnknown:   handleunknown,
		shutdownChannel: make(chan struct{}, 10),
		counter:         rand.Int(),
	}
}

//------------------------------------------------------------

type MessagesRouterImpl struct {
	waiters map[int]chan interface{}
	counter int
	mutex   sync.Mutex

	send          MessagesRouterSendFunc
	handleUnknown MessagesRouterUnknownProcessorFunc

	shutdownChannel chan struct{}
	shutdownReason  error
}

func (r *MessagesRouterImpl) ProcessIncomingMessage(id int, data interface{}) bool {
	r.mutex.Lock()
	waiter, ok := r.waiters[id]
	r.mutex.Unlock()

	if ok {
		waiter <- data
	} else if r.handleUnknown != nil {
		go r.handleUnknown(id, data)
	}

	return ok
}

func (r *MessagesRouterImpl) Shutdown(e error) {
	if r.shutdownReason != nil {
		return
	}
	if e == nil {
		e = MessagesRouterShutdownError
	}
	r.shutdownReason = e
	close(r.shutdownChannel)
}

func (r *MessagesRouterImpl) SendMessage(data interface{}, timeout time.Duration) (answer interface{}, err error) {
	// prepare
	r.mutex.Lock()
	c := r.counter
	r.counter++
	waiter := make(chan interface{}, 1)
	r.waiters[c] = waiter
	r.mutex.Unlock()

	// let it be sync
	if err = r.send(c, data, timeout); err == nil {
		// now, wait...
		select {
		case answer = <-waiter:
		case <-r.shutdownChannel:
			err = r.shutdownReason
		case <-time.After(timeout):
			err = MessagesRouterTimeoutError
		}
	}

	r.mutex.Lock()
	delete(r.waiters, c)
	r.mutex.Unlock()

	return answer, err
}
