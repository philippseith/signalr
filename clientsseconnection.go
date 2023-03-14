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
	ConnectionBase
	reqURL    string
	sseReader io.Reader
	sseWriter io.Writer
}

func newClientSSEConnection(address string, connectionID string, body io.ReadCloser) (*clientSSEConnection, error) {
	// Setup request
	reqURL, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	q.Set("id", connectionID)
	reqURL.RawQuery = q.Encode()
	c := clientSSEConnection{
		ConnectionBase: ConnectionBase{
			ctx:          context.Background(),
			connectionID: connectionID,
		},
		reqURL: reqURL.String(),
	}
	c.sseReader, c.sseWriter = io.Pipe()
	go func() {
		defer func() { closeResponseBody(body) }()
		p := make([]byte, 1<<15)
	loop:
		for {
			n, err := body.Read(p)
			if err != nil {
				break loop
			}
			lines := strings.Split(string(p[:n]), "\n")
			for _, line := range lines {
				line = strings.Trim(line, "\r\t ")
				// Ignore everything but data
				if strings.Index(line, "data:") != 0 {
					continue
				}
				json := strings.Replace(strings.Trim(line, "\r"), "data:", "", 1)
				// Spec says: If it starts with Space, remove it
				if len(json) > 0 && json[0] == ' ' {
					json = json[1:]
				}
				_, err = c.sseWriter.Write([]byte(json))
				if err != nil {
					break loop
				}
			}
		}
	}()
	return &c, nil
}

func (c *clientSSEConnection) Read(p []byte) (n int, err error) {
	return c.sseReader.Read(p)
}

func (c *clientSSEConnection) Write(p []byte) (n int, err error) {
	req, err := http.NewRequest("POST", c.reqURL, bytes.NewReader(p))
	if err != nil {
		return 0, err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("POST %v -> %v", c.reqURL, resp.Status)
	}
	closeResponseBody(resp.Body)
	return len(p), err
}
