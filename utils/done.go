package utils

import (
	"sync/atomic"
)

type Done struct {
	done   chan struct{}
	closed atomic.Bool
	noCopy noCopy
}

func NewDone() *Done {
	return &Done{
		done: make(chan struct{}),
	}
}

func (d *Done) Close() {
	if d.closed.CompareAndSwap(false, true) {
		close(d.done)
	}
}

func (d *Done) Done() <-chan struct{} {
	return d.done
}

func (d *Done) IsClosed() bool {
	return d.closed.Load()
}

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
