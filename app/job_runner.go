package app

import (
	"github.com/maddalax/htmgo/framework/service"
	"sync"
	"time"
)

type Job struct {
	name            string
	locator         *service.Locator
	interval        time.Duration
	lastRunDuration time.Duration
	totalRuns       int
	paused          bool
	stopped         bool
	cb              func()
}

func (j *Job) Pause() {
	j.paused = true
}

func (j *Job) Resume() {
	j.paused = false
}

func (j *Job) Stop() {
	j.stopped = true
}

type IntervalJobRunner struct {
	locator      *service.Locator
	eventHandler *EventHandler
	jobs         []Job
}

func NewIntervalJobRunner(locator *service.Locator) *IntervalJobRunner {
	return &IntervalJobRunner{
		eventHandler: NewEventHandler(locator),
		locator:      locator,
	}
}

func IntervalJobRunnerFromLocator(locator *service.Locator) *IntervalJobRunner {
	return service.Get[IntervalJobRunner](locator)
}

func (jr *IntervalJobRunner) GetJob(name string) *Job {
	for _, job := range jr.jobs {
		if job.name == name {
			return &job
		}
	}
	return nil
}

func (jr *IntervalJobRunner) Add(name string, duration time.Duration, job func()) {
	jr.jobs = append(jr.jobs, Job{
		name:     name,
		locator:  jr.locator,
		interval: duration,
		cb:       job,
	})
}

func (jr *IntervalJobRunner) Start() error {
	wg := sync.WaitGroup{}
	for _, job := range jr.jobs {
		wg.Add(1)
		go func(job Job) {
			defer wg.Done()
			for {
				if job.paused {
					time.Sleep(time.Second)
					continue
				}
				if job.stopped {
					go jr.eventHandler.OnJobStopped(&job)
					break
				}
				now := time.Now()
				go jr.eventHandler.OnJobStarted(&job)
				job.cb()
				job.totalRuns++
				go jr.eventHandler.OnJobFinished(&job)
				job.lastRunDuration = time.Since(now)
				time.Sleep(job.interval)
			}
		}(job)
	}

	wg.Wait()
	return nil
}