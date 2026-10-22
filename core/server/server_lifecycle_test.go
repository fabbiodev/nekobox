//go:build !darwin && with_awg && with_clash_api

package main

import (
	"context"
	"fmt"
	"io"
	"nekobox_core/gen"
	"nekobox_core/internal"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func switchTestConfig(port int) string {
	return fmt.Sprintf(`{
      "log":{"disabled":true},
      "inbounds":[{"type":"mixed","tag":"mixed","listen":"127.0.0.1","listen_port":%d}],
      "outbounds":[{"type":"direct","tag":"direct"}],
      "route":{"final":"direct"},
      "experimental":{"clash_api":{}},
      "endpoints":[{"type":"awg","tag":"awg-test","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
        "address":["10.90.0.1/32"],"mtu":1280,"s1":32,"s2":32,"s3":32,"s4":32,
        "peers":[{"public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","address":"127.0.0.1","port":9}]}]
    }`, port)
}

func TestRepeatedProfileSwitchWithLiveConnections(t *testing.T) {
	s := &server{}
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ready\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(func() { _, _ = s.Stop(ctx, &gen.EmptyReq{}); target.Close() })
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	reservation.Close()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}

	for cycle := range 30 {
		config := &gen.LoadConfigReq{CoreConfig: switchTestConfig(port)}
		started, err := s.Start(ctx, config)
		if err != nil || started.Error != "" {
			t.Fatalf("cycle %d start: %v %s", cycle, err, started.Error)
		}
		original := internal.CurrentBox()
		duplicate, err := s.Start(ctx, config)
		if err != nil || duplicate.Error == "" || internal.CurrentBox() != original {
			t.Fatal("rejected duplicate start lost the running instance")
		}
		response, err := client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		ready := make([]byte, 6)
		if _, err = io.ReadFull(response.Body, ready); err != nil {
			t.Fatal(err)
		}
		if string(ready) != "ready\n" {
			t.Fatal("incorrect proxied payload")
		}
		start := time.Now()
		stopped, err := s.Stop(ctx, &gen.EmptyReq{})
		if err != nil || stopped.Error != "" {
			t.Fatalf("cycle %d stop: %v %s", cycle, err, stopped.Error)
		}
		if time.Since(start) > time.Second {
			t.Fatal("stop stalled with an active connection")
		}
		if internal.CurrentBox() != nil {
			t.Fatal("closed instance retained")
		}
		var b [1]byte
		if _, err = response.Body.Read(b[:]); err == nil {
			t.Fatal("connection survived shutdown")
		}
		response.Body.Close()
	}
}

func TestFailedStartAndBusyOperations(t *testing.T) {
	ctx := context.Background()
	s := &server{}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	config := &gen.LoadConfigReq{CoreConfig: switchTestConfig(occupied.Addr().(*net.TCPAddr).Port)}
	failed, err := s.Start(ctx, config)
	if err != nil || failed.Error == "" || internal.CurrentBox() != nil {
		t.Fatal("failed startup left an instance behind")
	}
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	started, _ := s.Start(ctx, config)
	stopped, _ := s.Stop(ctx, &gen.EmptyReq{})
	if started.Error == "" || stopped.Error == "" {
		t.Fatal("overlapping operations were accepted")
	}
}
