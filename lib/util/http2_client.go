package util

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type HTTP2Client struct {
	sync.Mutex

	client http.Client
	conn   net.Conn
}

func NewHTTP2Client(timeout, idleTimeout time.Duration, keepAlive bool) (client *HTTP2Client, err error) {
	if keepAlive {
		timeout, idleTimeout = 0, 0
	}

	dialer := &net.Dialer{
		Timeout:   1 * time.Second,
		KeepAlive: 100000 * time.Second,
		DualStack: true,
	}

	client = &HTTP2Client{}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		IdleConnTimeout:   idleTimeout,
		DisableKeepAlives: !keepAlive,
		DialContext: func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
			conn, err = dialer.DialContext(ctx, network, addr)

			client.conn = conn
			return conn, err
		},
	}

	if err = http2.ConfigureTransport(transport); err != nil {
		return
	}

	client.client = http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // NOTE prevent redirect
		},
	}

	return
}

func (c *HTTP2Client) Close() {
	if c.conn == nil {
		return
	}
	c.conn.Close()
}

func (c *HTTP2Client) Get(url string, headers http.Header) (response *http.Response, err error) {
	var request *http.Request
	if request, err = http.NewRequest("GET", url, nil); err != nil {
		return
	}
	request.Header = headers

	if response, err = c.client.Do(request); err != nil {
		return
	}

	return
}

func (c *HTTP2Client) Post(url string, b []byte, headers http.Header) (response *http.Response, err error) {
	var request *http.Request
	if request, err = http.NewRequest("POST", url, bytes.NewBuffer(b)); err != nil {
		return
	}
	request.Header = headers

	if response, err = c.client.Do(request); err != nil {
		return
	}
	return
}

func HTTP2ClientPing(client *HTTP2Client, url string, interval time.Duration) (err error) {

	do := func(url string) (err error) {
		_, err = client.Get(url, nil)
		return
	}

	if err = do(url); err != nil {
		return
	}

	ticker := time.NewTicker(interval)
	for _ = range ticker.C {
		if err = do(url); err != nil {
			return
		}
	}

	return
}

type HTTP2StreamWriter struct {
	DataChannel chan []byte
	Error       error

	DataChannelClosed bool
}

func NewHTTP2StreamWriter() *HTTP2StreamWriter {
	return &HTTP2StreamWriter{
		DataChannel: make(chan []byte),
	}
}

func (r *HTTP2StreamWriter) Write(b []byte) (int, error) {
	if r.DataChannelClosed {
		return 0, nil
	}

	r.DataChannel <- b
	return len(b), nil
}

func (r *HTTP2StreamWriter) Close() error {
	r.DataChannelClosed = true
	close(r.DataChannel)

	return nil
}

func GetHTTP2Stream(response *http.Response) (cw *HTTP2StreamWriter, err error) {
	cw = NewHTTP2StreamWriter()
	go func() {
		defer func() {
			response.Body.Close()
			cw.Close()
		}()

		_, err := io.Copy(cw, response.Body)
		if err != nil {
			cw.Error = err // maybe `io.ErrUnexpectedEOF`; connection lost
		}
	}()

	return
}
