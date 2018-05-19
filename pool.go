package ants

import (
	"runtime"
	"sync/atomic"
	"sync"
)

type sig struct{}

type f func()

type Pool struct {
	capacity int32
	running  int32
	tasks    chan f
	workers  chan *Worker
	destroy  chan sig
	m        sync.Mutex
}

func NewPool(size int) *Pool {
	p := &Pool{
		capacity: int32(size),
		tasks:    make(chan f, 1000),
		//workers:  &sync.Pool{New: func() interface{} { return &Worker{} }},
		workers: make(chan *Worker, size),
		destroy: make(chan sig, runtime.GOMAXPROCS(-1)),
	}
	p.loop()
	return p
}

//-------------------------------------------------------------------------

func (p *Pool) loop() {
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		go func() {
			for {
				select {
				case task := <-p.tasks:
					p.getWorker().sendTask(task)
				case <-p.destroy:
					return
				}
			}
		}()
	}
}

func (p *Pool) Push(task f) error {
	if len(p.destroy) > 0 {
		return nil
	}
	p.tasks <- task
	return nil
}
func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *Pool) Free() int {
	return int(atomic.LoadInt32(&p.capacity) - atomic.LoadInt32(&p.running))
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *Pool) Destroy() error {
	p.m.Lock()
	defer p.m.Unlock()
	for i := 0; i < runtime.GOMAXPROCS(-1)+1; i++ {
		p.destroy <- sig{}
	}
	return nil
}

//-------------------------------------------------------------------------

func (p *Pool) reachLimit() bool {
	return p.Running() >= p.Cap()
}

func (p *Pool) newWorker() *Worker {
	worker := &Worker{
		pool: p,
		task: make(chan f),
		exit: make(chan sig),
	}
	worker.run()
	return worker
}

func (p *Pool) getWorker() *Worker {
	defer atomic.AddInt32(&p.running, 1)
	var worker *Worker
	if p.reachLimit() {
		worker = <-p.workers
	} else {
		select {
		case worker = <-p.workers:
			return worker
		default:
			worker = p.newWorker()
		}
	}
	return worker
}