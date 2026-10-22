//go:build with_awg

package awg

import (
	"os"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/v3/tun"
)

// NewDevice starts reading immediately, before IpcSet sets S4. A read already
// waiting with the old offset would encrypt the first packet with stale padding.
// Restart that read after configuration and defer the automatic EventUp too.
type startupTun struct {
	tunAdapter
	ready       chan struct{}
	done        chan struct{}
	events      chan tun.Event
	releaseOnce sync.Once
	readOnce    sync.Once
	closeOnce   sync.Once
}

func newStartupTun(device tunAdapter) *startupTun {
	t := &startupTun{tunAdapter: device, ready: make(chan struct{}), done: make(chan struct{}), events: make(chan tun.Event, 8)}
	go func() {
		defer close(t.events)
		select {
		case <-t.ready:
		case <-t.done:
			return
		}
		for {
			select {
			case <-t.done:
				return
			case event, ok := <-device.Events():
				if !ok {
					return
				}
				select {
				case t.events <- event:
				case <-t.done:
					return
				}
			}
		}
	}()
	return t
}

func (t *startupTun) release()                 { t.releaseOnce.Do(func() { close(t.ready) }) }
func (t *startupTun) Events() <-chan tun.Event { return t.events }
func (t *startupTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	first := false
	t.readOnce.Do(func() { first = true })
	if first {
		select {
		case <-t.ready:
			return 0, nil
		case <-t.done:
			return 0, os.ErrClosed
		}
	}
	select {
	case <-t.done:
		return 0, os.ErrClosed
	case <-t.ready:
		return t.tunAdapter.Read(bufs, sizes, offset)
	default:
		select {
		case <-t.ready:
			return 0, nil
		case <-t.done:
			return 0, os.ErrClosed
		}
	}
}
func (t *startupTun) Close() error {
	var err error
	t.closeOnce.Do(func() { close(t.done); err = t.tunAdapter.Close() })
	return err
}
