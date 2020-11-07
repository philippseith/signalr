package signalr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type clientSSEConnection struct {
	baseConnection
	reqUrl    string
	sseReader io.Reader
	sseWriter io.Writer
}

func newClientSSEConnection(parentContext context.Context, address string, connectionID string, body io.ReadCloser) (*clientSSEConnection, error) {
	// Setup request
	reqUrl, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	q := reqUrl.Query()
	q.Set("id", connectionID)
	reqUrl.RawQuery = q.Encode()
	c := clientSSEConnection{
		baseConnection: baseConnection{
			ctx:          parentContext,
			connectionID: connectionID,
		},
		reqUrl: reqUrl.String(),
	}
	c.sseReader, c.sseWriter = io.Pipe()
	go func() {
		p := make([]byte, 1<<15)
	loop:
		for {
			n, err := body.Read(p)
			if err != nil {
				break loop
			}
			lines := strings.Split(string(p[:n]), "\n")
			for _, line := range lines {
				json := strings.Replace(line, "data:", "", 1)
				_, err = c.sseWriter.Write([]byte(json))
				if err != nil {
					break loop
				}
			}
		}
		_ = body.Close()
	}()
	return &c, nil
}

func (c *clientSSEConnection) Read(p []byte) (n int, err error) {
	return c.sseReader.Read(p)
}

func (c *clientSSEConnection) Write(p []byte) (n int, err error) {
	req, err := http.NewRequest("POST", c.reqUrl, bytes.NewReader(p))
	if err != nil {
		return 0, err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("POST %v -> %v", c.reqUrl, resp.Status)
	}
	_ = resp.Body.Close()
	return len(p), err
}
