package process

import "testing"

func TestStopBeforeStartAndAfterFailedStart(t *testing.T) {
	p := NewProcess("this-executable-does-not-exist-nekobox-test", nil, true)
	p.Stop()
	if err := p.Start(); err == nil {
		t.Fatal("unexpected successful start")
	}
	p.Stop()
	p.Stop()
}
