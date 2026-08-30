package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	URL, User, Password string
	HTTP                *http.Client
}

func New(url, user, password string) *Client {
	return &Client{url, user, password, &http.Client{Timeout: 12 * time.Second}}
}
func (c *Client) Enabled() bool { return c.URL != "" && c.User != "" && c.Password != "" }

type responseEnvelope struct {
	Body struct {
		Response *struct {
			Result string `xml:"result"`
		} `xml:"executeCommandResponse"`
		Fault *struct {
			Code   string `xml:"faultcode"`
			Reason string `xml:"faultstring"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

func (c *Client) Command(ctx context.Context, command string) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("SOAP delivery is not configured")
	}
	payload := `<?xml version="1.0" encoding="UTF-8"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="urn:AC"><SOAP-ENV:Body><ns1:executeCommand><command>` + escape(command) + `</command></ns1:executeCommand></SOAP-ENV:Body></SOAP-ENV:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewBufferString(payload))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.User, c.Password)
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "urn:AC#executeCommand")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	fault := soapFault(b)
	if resp.StatusCode/100 != 2 || fault != "" {
		if fault != "" {
			return "", fmt.Errorf("SOAP returned %s: %s", resp.Status, fault)
		}
		return "", fmt.Errorf("SOAP returned %s: %s", resp.Status, compact(string(b)))
	}
	var envelope responseEnvelope
	if err := xml.Unmarshal(b, &envelope); err != nil {
		return "", fmt.Errorf("decode SOAP response: %w", err)
	}
	if envelope.Body.Response == nil {
		return "", fmt.Errorf("SOAP response did not contain executeCommandResponse")
	}
	result := strings.ReplaceAll(envelope.Body.Response.Result, "\r\n", "\n")
	result = strings.ReplaceAll(result, "\r", "\n")
	return strings.TrimSpace(result), nil
}

func soapFault(payload []byte) string {
	var envelope responseEnvelope
	if xml.Unmarshal(payload, &envelope) != nil || envelope.Body.Fault == nil {
		return ""
	}
	reason := compact(envelope.Body.Fault.Reason)
	code := compact(envelope.Body.Fault.Code)
	if reason == "" {
		return code
	}
	if code == "" {
		return reason
	}
	return code + ": " + reason
}
func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func compact(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
