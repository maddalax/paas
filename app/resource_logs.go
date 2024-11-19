package app

import (
	"context"
	"fmt"
	"github.com/maddalax/htmgo/framework/service"
	"github.com/nats-io/nats.go"
	"log/slog"
	"paas/app/subject"
	"sync"
	"time"
)

func StreamResourceLogs(locator *service.Locator, context context.Context, resource *Resource, cb func(msg *nats.Msg)) {
	doStream(locator, context, resource, cb, time.Time{})
}

func doStream(locator *service.Locator, context context.Context, resource *Resource, cb func(msg *nats.Msg), lastMessageTime time.Time) {
	natsClient := KvFromLocator(locator)
	restartStream := false

	writer := natsClient.CreateEphemeralWriterSubscriber(context, subject.RunLogsForResource(resource.Id), NatsWriterCreateOptions{
		BeforeWrite: func(data string) bool {
			lastMessageTime = time.Now()
			return true
		},
	})

	m := service.Get[ResourceMonitor](locator)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	streaming := true

	// start streaming the logs in a goroutine since its blocking
	go func() {
		streaming = true
		switch resource.BuildMeta.(type) {
		case *DockerBuildMeta:
			// this is blocking, so if this stops then we know streaming stopped
			streamDockerLogs(resource, context, StreamLogsOptions{
				Stdout: writer.Writer,
				Since:  lastMessageTime,
			})
			streaming = false
		}
	}()

	for {
		if restartStream {
			break
		}
		select {
		case <-context.Done():
			slog.Debug("context is done, stopping log stream", slog.String("resourceId", resource.Id))
			return
		case msg := <-writer.Subscriber:
			cb(msg)
		case <-ticker.C:
			if streaming {
				continue
			}
			// streaming stopped, lets see if we need to re-connect it
			status := m.GetRunStatus(resource)
			slog.Debug("streaming is stopped, checking run status", slog.String("resourceId", resource.Id))
			if status == RunStatusRunning {
				slog.Debug("container is running, restarting stream", slog.String("resourceId", resource.Id))
				restartStream = true
				break
			} else {
				slog.Debug("container is not running, waiting for it to start", slog.String("resourceId", resource.Id))
				continue
			}
		}
	}

	if restartStream {
		slog.Debug("restarting stream", slog.String("resourceId", resource.Id))
		writer = nil
		doStream(locator, context, resource, cb, lastMessageTime)
	}
}

func streamDockerLogs(resource *Resource, context context.Context, opts StreamLogsOptions) {
	client, err := DockerConnect()
	if err != nil {
		opts.Stdout.Write([]byte(err.Error()))
		return
	}

	wg := sync.WaitGroup{}
	for i := range resource.InstancesPerServer {
		wg.Add(1)
		go func() {
			containerId := fmt.Sprintf("%s-%s-container-%d", resource.Name, resource.Id, i)
			err = client.StreamLogs(containerId, context, StreamLogsOptions{
				Stdout: opts.Stdout,
				Since:  opts.Since,
			})
			if err != nil {
				opts.Stdout.Write([]byte(err.Error()))
			}
		}()
	}
	wg.Wait()
}