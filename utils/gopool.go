package utils

import "time"

type GoPool struct {
	noCopy
	sem  chan struct{}
	work chan func()
}

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Fixed number of goroutines, reusable. M:N model
//
// M: the number of reusable goroutines,
// N: the capacity for asynchronous task processing.
func NewGoPool(sizeM, preSpawn, queueN int) *GoPool {
	if preSpawn <= 0 && queueN > 0 {
		panic("GoPool: dead queue")
	}
	if preSpawn > sizeM {
		preSpawn = sizeM
	}
	p := &GoPool{
		sem:  make(chan struct{}, sizeM),
		work: make(chan func(), queueN),
	}
	for i := 0; i < preSpawn; i++ { // pre spawn
		p.sem <- struct{}{}
		go p.coreWorker(func() {})
	}
	return p
}
func (p *GoPool) Go(task func()) {
	select {
	case p.work <- task:
	case p.sem <- struct{}{}:
		go p.worker(task)
	}
}
func (p *GoPool) worker(task func()) {
	defer func() { <-p.sem }()

	for {
		task()
		select {
		case task = <-p.work:
			continue
		case <-time.After(1 * time.Second):
			return
		}
	}
}

func (p *GoPool) coreWorker(task func()) {
	defer func() { <-p.sem }()
	for {
		task()
		task = <-p.work
	}
}
