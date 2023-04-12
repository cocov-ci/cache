package housekeeping

import (
	"encoding/json"
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/locator"
	"github.com/cocov-ci/cache/redis"
	"github.com/cocov-ci/cache/storage"
	"github.com/heyvito/go-leader/leader"
	"go.uber.org/zap"
	"time"
)

func New(r redis.Client, apiClient api.Client) *Janitor {
	l, promote, demote, err := r.MakeLeader(leader.Opts{
		TTL:      5 * time.Second,
		Wait:     10 * time.Second,
		JitterMS: 500,
		Key:      "cocov:cached:leader",
	})

	return &Janitor{
		api:     apiClient,
		log:     zap.L().With(zap.String("component", "janitor")),
		redis:   r,
		leader:  l,
		promote: promote,
		demote:  demote,
		err:     err,
		tasks:   make(chan *Task, 10),
		done:    make(chan bool),
	}
}

type Janitor struct {
	redis   redis.Client
	log     *zap.Logger
	leader  leader.Leader
	promote <-chan time.Time
	demote  <-chan time.Time
	err     <-chan error
	tasks   chan *Task
	leading bool
	running bool
	done    chan bool
	api     api.Client
	storage storage.Provider
}

func (j *Janitor) performTasks() {
	defer close(j.done)
	for rawTask := range j.tasks {
		j.log.Info("Performing task", zap.String("id", rawTask.ID))
		switch rawTask.Name {
		case "evict-artifact":
			task, err := TaskInto[EvictTask](rawTask)
			if err != nil {
				j.log.Error(
					"Failed decoding task",
					zap.String("task_id", rawTask.ID),
					zap.String("name", rawTask.Name),
					zap.ByteString("data", rawTask.original),
				)
				continue
			}

			j.runEvictArtifact(task)
		case "evict-tool":
			task, err := TaskInto[EvictToolTask](rawTask)
			if err != nil {
				j.log.Error(
					"Failed decoding task",
					zap.String("task_id", rawTask.ID),
					zap.String("name", rawTask.Name),
					zap.ByteString("data", rawTask.original),
				)
				continue
			}

			j.runEvictTool(task)
		case "purge-repository":
			task, err := TaskInto[PurgeTask](rawTask)
			if err != nil {
				j.log.Error(
					"Failed decoding task",
					zap.String("task_id", rawTask.ID),
					zap.String("name", rawTask.Name),
					zap.ByteString("data", rawTask.original),
				)
				continue
			}
			j.runPurgeRepository(task)

		case "purge-tool":
			task, err := TaskInto[PurgeToolTask](rawTask)
			if err != nil {
				j.log.Error(
					"Failed decoding task",
					zap.String("task_id", rawTask.ID),
					zap.String("name", rawTask.Name),
					zap.ByteString("data", rawTask.original),
				)
				continue
			}
			j.runPurgeTool(task)
		}
	}
}

func (j *Janitor) runEvictArtifact(task *EvictTask) {
	log := j.log.With(zap.String("task_id", task.ID))
	log.Info("Starting task")
	for _, id := range task.Objects {
		err := j.storage.DeleteArtifact(&locator.ArtifactLocator{
			RepositoryID: task.Repository,
			NameHash:     id,
		})
		if err != nil {
			log.Error("Failed removing item from storage", zap.String("id", id), zap.Error(err))
			continue
		}
	}
	log.Info("Completed")
}

func (j *Janitor) runPurgeRepository(task *PurgeTask) {
	log := j.log.With(zap.String("task_id", task.ID))
	log.Info("Starting task")
	err := j.storage.PurgeRepository(task.Repository)
	if err != nil {
		log.Error("Failed purging repository", zap.Error(err))
		return
	}
	log.Info("Completed")
}

func (j *Janitor) runEvictTool(task *EvictToolTask) {
	log := j.log.With(zap.String("task_id", task.ID))
	log.Info("Starting task")
	for _, id := range task.Objects {
		err := j.storage.DeleteTool(&locator.ToolLocator{
			NameHash: id,
		})

		if err != nil {
			log.Error("Failed removing item from storage", zap.String("id", id), zap.Error(err))
			continue
		}
	}
	log.Info("Completed")
}

func (j *Janitor) runPurgeTool(task *PurgeToolTask) {
	log := j.log.With(zap.String("task_id", task.ID))
	log.Info("Starting task")
	err := j.storage.PurgeTool()
	if err != nil {
		log.Error("Failed purging repository", zap.Error(err))
		return
	}
	log.Info("Completed")
}

func (j *Janitor) collectTasks() {
	for j.leading {
		v, err := j.redis.NextHousekeepingTask()
		if err != nil {
			j.log.Error("Failed obtaining task from Redis", zap.Error(err))
			time.Sleep(2 * time.Second)
		}
		if v == "" {
			continue
		}
		var t Task
		err = json.Unmarshal([]byte(v), &t)
		if err != nil {
			j.log.Error("Failed unmarshalling base task", zap.Error(err), zap.String("data", v))
			continue
		}
		j.tasks <- &t
	}
}

func (j *Janitor) Start() {
	j.leader.Start()
	j.running = true
	go j.performTasks()

	for {
		select {
		case <-j.promote:
			j.log.Info("Became leader")
			j.leading = true
			go j.collectTasks()

		case <-j.demote:
			j.log.Info("Stopped leading")
			j.leading = false
			if !j.running {
				return
			}

		case err := <-j.err:
			j.log.Error("Leading mechanism reported error", zap.Error(err))
		}
	}
}

func (j *Janitor) Stop() {
	if !j.running {
		return
	}
	j.log.Info("Stopping...")
	j.running = false

	resigned := false
	for i := 0; i < 10; i++ {
		if err := j.leader.Stop(); err != nil {
			j.log.Error("Failed resigning leadership. Retrying in one second.", zap.Error(err))
			time.Sleep(1 * time.Second)
			continue
		}
		resigned = true
		break
	}
	if !resigned {
		j.log.Warn("Failed to resign for 10 seconds. Attempting to forcibly interrupt task handling. Cluster may run without a leader until next election.")
		j.leading = false
	}
	close(j.tasks)
	j.log.Info("Waiting for tasks to be finished (if any)...")
	<-j.done
}
