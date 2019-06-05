package client

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"

	"tcp-over-http/protocol"
)

type connectionWrapper struct {
	m            sync.Mutex
	responseDone bool
	disconnected uint32
	onDisconnect func()
	logger       *log.Entry

	net.Conn
}

func (cw *connectionWrapper) Read(b []byte) (n int, err error) {
	cw.ensureResponse()
	return cw.Conn.Read(b)
}

func (cw *connectionWrapper) Close() error {
	if atomic.SwapUint32(&cw.disconnected, 1) == 0 {
		cw.logger.Info("disconnected")

		if cw.onDisconnect != nil {
			defer cw.onDisconnect()
		}
	}

	return cw.Conn.Close()
}

func (cw *connectionWrapper) ensureResponse() {
	cw.m.Lock()
	defer cw.m.Unlock()
	if cw.responseDone {
		return
	}
	defer func() { cw.responseDone = true }()

	resp, err := protocol.ReadResponse(context.TODO(), cw.Conn)
	if err != nil || resp.Err != nil {
		if err == nil {
			err = fmt.Errorf("remote: %v", *resp.Err)
		} else {
			err = fmt.Errorf("local: %v", err)
		}

		if atomic.LoadUint32(&cw.disconnected) == 0 {
			cw.logger.WithError(err).Error("dial error")
		} else {
			cw.logger.Warn("close called while reading initial response")
		}

		_ = cw.Conn.Close()
		_, _ = io.Copy(ioutil.Discard, cw.Conn)
		return
	}

	cw.logger.Debug("remote end connected")
}