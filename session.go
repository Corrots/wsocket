package wsocket

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	errSessionAlreadyClosed = errors.New("session is already closed")
)

type Session struct {
	Request *http.Request
	Keys    map[string]interface{}
	conn    *websocket.Conn
	output  chan *envelope
	manager *Manager
	open    bool
	rwmutex *sync.RWMutex
}

func (s *Session) writeMessage(message *envelope) {
	if s.closed() {
		s.manager.errorHandler(s, errors.New("tried to write to a closed session"))
		return
	}

	select {
	case s.output <- message:
	default:
		s.manager.errorHandler(s, errors.New("session message buffer is full"))
		return
	}
}

func (s *Session) writeRaw(message *envelope) error {
	if s.closed() {
		return errors.New("tried to write to a closed session")
	}
	s.conn.SetWriteDeadline(time.Now().Add(s.manager.Config.WriteWait))
	err := s.conn.WriteMessage(message.t, message.msg)
	if err != nil {
		return fmt.Errorf("write message err: %v\n", err)
	}
	return nil
}

func (s *Session) closed() bool {
	s.rwmutex.Lock()
	defer s.rwmutex.Unlock()
	return !s.open
}

func (s *Session) close() {
	if !s.closed() {
		s.rwmutex.Lock()
		s.open = false
		s.conn.Close()
		close(s.output)
		s.rwmutex.Unlock()
	}
}

func (s *Session) ping() {
	s.writeRaw(&envelope{t: websocket.PingMessage, msg: []byte{}})
}

func (s *Session) writePump() {
	ticker := time.NewTicker(s.manager.Config.PingPeriod)
	defer ticker.Stop()
loop:
	for {
		select {
		case msg, ok := <-s.output:
			if !ok {
				break loop
			}
			err := s.writeRaw(msg)
			if err != nil {
				s.manager.errorHandler(s, err)
				break loop
			}
			if msg.t == websocket.CloseMessage {
				break loop
			}
			if msg.t == websocket.TextMessage {
				s.manager.messageSentHandler(s, msg.msg)
			}
		case <-ticker.C:
			s.ping()
		}
	}
}

func (s *Session) readPump() {
	s.conn.SetReadLimit(s.manager.Config.MaxMessageSize)
	readDeadLine := time.Now().Add(s.manager.Config.PongWait)
	s.conn.SetReadDeadline(readDeadLine)

	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(readDeadLine)
		s.manager.pongHandler(s)
		return nil
	})

	if s.manager.closeHandler != nil {
		s.conn.SetCloseHandler(func(code int, text string) error {
			return s.manager.closeHandler(s, code, text)
		})
	}

	for {
		t, message, err := s.conn.ReadMessage()
		if err != nil {
			s.manager.errorHandler(s, err)
			break
		}
		if t == websocket.TextMessage {
			s.manager.messageHandler(s, message)
		}
	}
}

//
//
func (s *Session) Write(msg []byte) error {
	if s.closed() {
		return errors.New("session is closed")
	}
	s.writeMessage(&envelope{t: websocket.TextMessage, msg: msg})
	return nil
}

func (s *Session) Close() error {
	if s.closed() {
		return errSessionAlreadyClosed
	}
	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: []byte{}})
	return nil
}

func (s *Session) CloseWithMsg(msg []byte) error {
	if s.closed() {
		return errSessionAlreadyClosed
	}
	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: msg})
	return nil
}

func (s *Session) Set(key string, value interface{}) {
	if s.Keys == nil {
		s.Keys = make(map[string]interface{})
	}
	s.Keys[key] = value
}

func (s *Session) Get(key string) (value interface{}, exists bool) {
	if s.Keys != nil {
		value, exists = s.Keys[key]
	}
	return
}

func (s *Session) MustGet(key string) interface{} {
	if value, exists := s.Keys[key]; exists {
		return value
	}
	panic("Key \"" + key + "\" does not exist")
}

func (s *Session) IsClosed() bool {
	return s.closed()
}
