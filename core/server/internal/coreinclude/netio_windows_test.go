//go:build windows

package coreinclude_test

import (
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Exercise pending Windows overlapped UDP receives while sockets close and Go
// stacks grow/GC runs. This is a stress check, not a deterministic reproducer
// of the historical runtime access violation.
func TestWindowsPendingUDPClose(t *testing.T) {
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			address := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
			for i := 0; i < 50; i++ {
				socket, err := net.ListenUDP("udp4", address)
				if err != nil {
					t.Error(err)
					return
				}
				address = socket.LocalAddr().(*net.UDPAddr)
				socket.SetReadDeadline(time.Now().Add(time.Second))
				done := make(chan struct{})
				go func() {
					defer close(done)
					buffer := make([]byte, 2048)
					socket.ReadFromUDPAddrPort(buffer)
				}()
				runtime.Gosched()
				socket.Close()
				<-done
				time.Sleep(time.Millisecond)
			}
		}()
	}
	runtime.GC()
	workers.Wait()
}
