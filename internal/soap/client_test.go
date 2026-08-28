package soap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommandReportsSOAPFaultReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/"><SOAP-ENV:Body><SOAP-ENV:Fault><faultcode>SOAP-ENV:Client</faultcode><faultstring>Command failed: permission denied</faultstring></SOAP-ENV:Fault></SOAP-ENV:Body></SOAP-ENV:Envelope>`))
	}))
	defer server.Close()

	client := New(server.URL, "soap-user", "secret")
	_, err := client.Command(context.Background(), "send items Arthoria subject body 123:1")
	if err == nil || !strings.Contains(err.Error(), "SOAP-ENV:Client: Command failed: permission denied") {
		t.Fatalf("expected parsed SOAP fault, got %v", err)
	}
}

func TestCommandReportsNonSOAPErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream unavailable"))
	}))
	defer server.Close()

	client := New(server.URL, "soap-user", "secret")
	_, err := client.Command(context.Background(), "server info")
	if err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("expected response body, got %v", err)
	}
}
