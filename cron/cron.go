package cron

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/container"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
)

var logger = log.Module("glacier.cron")

// Manager is a manager object to manage cron jobs
type Manager interface {
	// Add add a cron job
	Add(name string, plan string, handler interface{}) error
	// Remove remove a cron job
	Remove(name string) error
	// Pause set job status to paused
	Pause(name string) error
	// Continue set job status to continue
	Continue(name string) error
	// Info get job info
	Info(name string) (Job, error)

	// Start start cron manager
	Start()
	// Stop stop cron job manager
	Stop()

	// DistributeLockManager is a setter method for distribute lock manager
	DistributeLockManager(lockManager DistributeLockManager)
}

// DistributeLockManager is a distribute lock manager interface
type DistributeLockManager interface {
	// TryLock try to get lock
	// this method will be called every 60s
	// you should set a ttl for lock since unlock method may be not be called in some case
	TryLock() error
	// TryUnlock try to release the lock
	TryUnLock() error
	// HasLock return whether manager has lock
	HasLock() bool
}

type cronManager struct {
	lock sync.RWMutex
	cc   container.Container
	cr   *cron.Cron

	distributeLockManager DistributeLockManager

	jobs map[string]*Job
}

// Job is a job object
type Job struct {
	ID      cron.EntryID
	Name    string
	Plan    string
	handler func()
	Paused  bool
}

// Next get execute plan for job
func (job Job) Next(nextNum int) ([]time.Time, error) {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sc, err := parser.Parse(job.Plan)
	if err != nil {
		return nil, err
	}

	results := make([]time.Time, nextNum)
	lastTs := time.Now()
	for i := 0; i < nextNum; i++ {
		lastTs = sc.Next(lastTs)
		results[i] = lastTs
	}

	return results, nil
}

// NewManager create a new Manager
func NewManager(cc container.Container) Manager {
	m := cronManager{cc: cc, jobs: make(map[string]*Job)}
	cc.MustResolve(func(cr *cron.Cron) { m.cr = cr })

	return &m
}

func (c *cronManager) DistributeLockManager(lockManager DistributeLockManager) {
	c.distributeLockManager = lockManager
}

func (c *cronManager) Add(name string, plan string, handler interface{}) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if reg, existed := c.jobs[name]; existed {
		return fmt.Errorf("job with name [%s] has existed: %d | %s", name, reg.ID, reg.Plan)
	}

	jobHandler := func() {
		if c.distributeLockManager != nil && !c.distributeLockManager.HasLock() {
			log.Debugf("cron job [%s] can not start because it doesn't get the lock", name)
			return
		}

		logger.Debugf("cron job [%s] running", name)
		startTs := time.Now()
		defer func() {
			logger.Debugf("cron job [%s] stopped, elapse %s", name, time.Now().Sub(startTs))
		}()
		if err := c.cc.ResolveWithError(handler); err != nil {
			logger.Errorf("cron job [%s] failed, Err: %v, Stack: %s", err, debug.Stack())
		}
	}
	id, err := c.cr.AddFunc(plan, jobHandler)

	if err != nil {
		return errors.Wrap(err, "add cron job failed")
	}

	c.jobs[name] = &Job{
		ID:      id,
		Name:    name,
		Plan:    plan,
		handler: jobHandler,
		Paused:  false,
	}

	logger.Debugf("add job [%s] to cron manager with plan %s", name, plan)

	return nil
}

func (c *cronManager) Remove(name string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	reg, exist := c.jobs[name]
	if !exist {
		return errors.Errorf("job with name [%s] not found", name)
	}

	delete(c.jobs, name)
	if !reg.Paused {
		c.cr.Remove(reg.ID)
	}

	logger.Debugf("remove job [%s] from cron manager", name)

	return nil
}

func (c *cronManager) Pause(name string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	reg, exist := c.jobs[name]
	if !exist {
		return errors.Errorf("job with name [%s] not found", name)
	}

	if reg.Paused {
		return nil
	}

	c.cr.Remove(reg.ID)
	reg.Paused = true

	logger.Debugf("change job [%s] to paused", name)

	return nil
}

func (c *cronManager) Continue(name string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	reg, exist := c.jobs[name]
	if !exist {
		return errors.Errorf("job with name [%s] not found", name)
	}

	if !reg.Paused {
		return nil
	}

	id, err := c.cr.AddFunc(reg.Plan, reg.handler)
	if err != nil {
		return errors.Wrap(err, "change job from paused to continue failed")
	}

	reg.Paused = false
	reg.ID = id

	logger.Debugf("change job [%s] to continue", name)

	return nil
}

func (c *cronManager) Info(name string) (Job, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if job, ok := c.jobs[name]; ok {
		return *job, nil
	}

	return Job{}, fmt.Errorf("job with name [%s] not found", name)
}

func (c *cronManager) Start() {
	if c.distributeLockManager != nil {
		getDistributeLock := func() {
			if err := c.distributeLockManager.TryLock(); err != nil {
				log.Warningf("try to get distribute lock failed: %v", err)
			}
		}

		getDistributeLock()
		if _, err := c.cr.AddFunc("@every 60s", getDistributeLock); err != nil  {
			log.Errorf("initialize cron failed: can not create distribute lock task: %v", err)
		}
	}

	c.cr.Start()
}

func (c *cronManager) Stop() {
	c.cr.Stop()
	if c.distributeLockManager != nil {
		if err := c.distributeLockManager.TryUnLock(); err != nil {
			log.Warningf("try to release distribute lock failed: %v", err)
		}
	}
}
