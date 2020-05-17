package wsocket

import (
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type handleMessageFunc func(*Session, []byte)
type handleErrorFunc func(*Session, error)
type handleCloseFunc func(*Session, int, string) error
type handleSessionFunc func(*Session)
type filterFunc func(*Session) bool

type Manager struct {
	Config             *Config
	Upgrader           *websocket.Upgrader
	messageHandler     handleMessageFunc
	messageSentHandler handleMessageFunc
	errorHandler       handleErrorFunc
	closeHandler       handleCloseFunc
	connectHandler     handleSessionFunc
	disconnectHandler  handleSessionFunc
	pongHandler        handleSessionFunc
	hub                *hub
}

func New() *Manager {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	hub := newHub()
	go hub.run()

	return &Manager{
		Config:             NewConfig(),
		Upgrader:           upgrader,
		messageHandler:     func(*Session, []byte) {},
		messageSentHandler: func(*Session, []byte) {},
		errorHandler:       func(*Session, error) {},
		closeHandler:       nil,
		connectHandler:     func(*Session) {},
		disconnectHandler:  func(*Session) {},
		pongHandler:        func(*Session) {},
		hub:                hub,
	}
}

func (m *Manager) HandleConnect(fn func(*Session)) {
	m.connectHandler = fn
}

func (m *Manager) HandleDisconnect(fn func(*Session)) {
	m.disconnectHandler = fn
}

func (m *Manager) HandlePong(fn func(*Session)) {
	m.pongHandler = fn
}

func (m *Manager) HandleMessage(fn func(*Session, []byte)) {
	m.messageHandler = fn
}

func (m *Manager) HandleSentMessage(fn func(*Session, []byte)) {
	m.messageSentHandler = fn
}

func (m *Manager) HandleError(fn func(*Session, error)) {
	m.errorHandler = fn
}

func (m *Manager) HandleClose(fn func(*Session, int, string) error) {
	if fn != nil {
		m.closeHandler = fn
	}
}

func (m *Manager) HandleRequest(w http.ResponseWriter, r *http.Request) error {
	return m.HandleRequestWithKeys(w, r, nil)
}

func (m *Manager) HandleRequestWithKeys(w http.ResponseWriter, r *http.Request, keys map[string]interface{}) error {
	if m.hub.closed() {
		return errors.New("manager instance is closed")
	}

	conn, err := m.Upgrader.Upgrade(w, r, w.Header())
	if err != nil {
		return err
	}

	session := &Session{
		Request: r,
		Keys:    keys,
		conn:    conn,
		output:  make(chan *envelope, m.Config.MessageBufferSize),
		manager: m,
		open:    true,
		rwmutex: &sync.RWMutex{},
	}
	m.hub.register <- session
	m.connectHandler(session)

	go session.writePump()

	session.readPump()

	m.disconnectHandler(session)
	return nil
}

func (m *Manager) Broadcast(msg []byte) error {
	if m.hub.closed() {
		return errors.New("manager instance is closed")
	}
	message := &envelope{t: websocket.TextMessage, msg: msg}
	m.hub.broadcast <- message
	return nil
}

func (m *Manager) BroadcastFilter(msg []byte, fn func(*Session) bool) error {
	if m.hub.closed() {
		return errors.New("manager instance is closed")
	}
	message := &envelope{t: websocket.TextMessage, msg: msg, filter: fn}
	m.hub.broadcast <- message
	return nil
}

// BroadcastOthers broadcast a text message to all session except session s.
func (m *Manager) BroadcastOthers(msg []byte, s *Session) error {
	return m.BroadcastFilter(msg, func(q *Session) bool {
		return s != q
	})
}

// BroadcastMultiple broadcasts a text message to multiple sessions given in the sessions slice.
func (m *Manager) BroadcastMultiple(msg []byte, sessions []*Session) error {
	for _, sess := range sessions {
		if writeErr := sess.Write(msg); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (m *Manager) Close() error {
	if m.hub.closed() {
		return errors.New("manager instance is already closed")
	}
	m.hub.exit <- &envelope{t: websocket.CloseMessage, msg: []byte{}}
	return nil
}

func (m *Manager) IsClosed() bool {
	return m.hub.closed()
}

// Len return the number of connected sessions.
func (m *Manager) Len() int {
	return m.hub.len()
}
