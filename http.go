package pinger

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
)

type httpAddr struct {
	Method string
	URL    *url.URL
}

type httpDriver struct {
	ctx    context.Context
	cancel context.CancelFunc

	addr   *httpAddr
	client *http.Client
}

func errInvalidHTTPMethod(m string) error {
	return fmt.Errorf("invalid method %s", m)
}

// HTTP pinger.
// The standard implementation supports GET or HEAD requests without authentication.
// This is a simple pinger, and for more complex requirements you are better off writing a custom driver.
func HTTP(method string, reqURL string) (Pinger, error) {
	addrURL, err := url.Parse(reqURL)
	if err != nil {
		return nil, err
	}
	return httpWithAddr(httpAddr{
		Method: method,
		URL:    addrURL,
	})
}

func httpWithAddr(addr httpAddr) (Pinger, error) {
	switch addr.Method {
	case http.MethodGet:
		break
	case http.MethodHead:
		break
	default:
		return nil, errInvalidHTTPMethod(addr.Method)
	}

	p := &httpDriver{
		addr:   &addr,
		client: &http.Client{},
	}
	return New(p), nil
}

func (a *httpAddr) Network() string {
	return "http"
}

func (a *httpAddr) String() string {
	return fmt.Sprintf("%s %s", a.Method, a.URL.String())
}

func (d *httpDriver) Address() net.Addr {
	return d.addr
}

func (d *httpDriver) Connect(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)
	return nil
}

func (d *httpDriver) Disconnect() error {
	d.cancel()
	return nil
}

func (d *httpDriver) Ping() (RawPacket, error) {
	errc := make(chan error, 1)
	rawc := make(chan RawPacket, 1)
	defer close(errc)
	defer close(rawc)

	go func() {
		raw, err := d.send()
		if err != nil {
			errc <- err
		} else {
			rawc <- raw
		}
	}()

	select {
	case <-d.ctx.Done():
		return RawPacket{}, d.ctx.Err()
	case err := <-errc:
		return RawPacket{}, err
	case raw := <-rawc:
		return raw, nil
	}
}

func (d *httpDriver) newRequest() *http.Request {
	return &http.Request{
		Method: d.addr.Method,
		URL:    d.addr.URL,
	}
}

func (d *httpDriver) send() (RawPacket, error) {
	req := d.newRequest()
	res, err := d.client.Do(req)
	if err != nil {
		return RawPacket{}, err
	}

	msg, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return RawPacket{}, err
	}
	raw := RawPacket{
		Message: msg,
		Size:    len(msg),
		TTL:     0,
	}
	return raw, nil
}
