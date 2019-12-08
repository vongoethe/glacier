package service

import (
	"time"

	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/container"
)

type DemoService struct {
	cc      container.Container
	stopped chan interface{}
}

func (d *DemoService) Init(cc container.Container) error {
	d.cc = cc
	d.stopped = make(chan interface{}, 0)
	return nil
}

func (d *DemoService) Name() string {
	return "demo-service"
}

func (d *DemoService) Start() error {
	for {
		select {
		case <-d.stopped:
			return nil
		default:
			time.Sleep(1 * time.Second)
			log.Errorf("hello, world from %s", d.Name())
		}
	}
}

func (d *DemoService) Stop() {
	d.stopped <- struct{}{}
}

func (d *DemoService) Reload() {
	panic("implement me")
}
