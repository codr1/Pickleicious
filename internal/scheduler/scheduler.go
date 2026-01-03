package scheduler

import (
	"errors"
	"strings"
	"sync"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

var (
	service     *Service
	serviceOnce sync.Once
	serviceErr  error
)

var (
	ErrNotInitialized = errors.New("scheduler not initialized")
	ErrEmptyJobName   = errors.New("job name is required")
	ErrEmptyCronExpr  = errors.New("cron expression is required")
)

// Service wraps a gocron scheduler for app-wide scheduling.
type Service struct {
	scheduler gocron.Scheduler
	stopOnce  sync.Once
	stopErr   error
}

// Init initializes the scheduler singleton.
func Init() error {
	serviceOnce.Do(func() {
		sched, err := gocron.NewScheduler(
			gocron.WithGlobalJobOptions(
				gocron.WithEventListeners(
					gocron.AfterJobRunsWithPanic(func(jobID uuid.UUID, jobName string, recoverData any) {
						log.Error().
							Str("job_id", jobID.String()).
							Str("job_name", jobName).
							Interface("panic", recoverData).
							Msg("Scheduler job panicked")
					}),
				),
			),
		)
		if err != nil {
			serviceErr = err
			return
		}
		service = &Service{scheduler: sched}
		log.Info().Msg("Scheduler initialized")
	})
	return serviceErr
}

// ServiceInstance returns the initialized scheduler singleton.
func ServiceInstance() (*Service, error) {
	if service == nil && serviceErr == nil {
		return nil, ErrNotInitialized
	}
	return service, serviceErr
}

// Start begins running scheduled jobs on the singleton scheduler.
func Start() error {
	svc, err := ServiceInstance()
	if err != nil {
		return err
	}
	svc.Start()
	return nil
}

// Stop shuts down the singleton scheduler.
func Stop() error {
	svc, err := ServiceInstance()
	if err != nil {
		return err
	}
	return svc.Stop()
}

// AddJob registers a cron-based job with the singleton scheduler.
func AddJob(name, cronExpr string, task func()) (gocron.Job, error) {
	svc, err := ServiceInstance()
	if err != nil {
		return nil, err
	}
	return svc.AddJob(name, cronExpr, task)
}

// Start begins running scheduled jobs.
func (s *Service) Start() {
	if s == nil {
		log.Error().Msg("Scheduler start requested before initialization")
		return
	}
	log.Info().Msg("Scheduler starting")
	s.scheduler.Start()
}

// Stop shuts down the scheduler and prevents new jobs from running.
func (s *Service) Stop() error {
	if s == nil {
		return ErrNotInitialized
	}
	s.stopOnce.Do(func() {
		log.Info().Msg("Scheduler stopping")
		s.stopErr = s.scheduler.Shutdown()
	})
	return s.stopErr
}

// AddJob registers a cron-based job with the scheduler.
func (s *Service) AddJob(name, cronExpr string, task func()) (gocron.Job, error) {
	if s == nil {
		return nil, ErrNotInitialized
	}
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyJobName
	}
	if strings.TrimSpace(cronExpr) == "" {
		return nil, ErrEmptyCronExpr
	}
	jobLogger := log.With().Str("job_name", name).Str("cron", cronExpr).Logger()
	jobLogger.Info().Msg("Registering scheduler job")

	wrappedTask := func() {
		jobLogger.Debug().Msg("Scheduler job started")
		task()
		jobLogger.Debug().Msg("Scheduler job completed")
	}

	job, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(wrappedTask),
		gocron.WithName(name),
	)
	if err != nil {
		jobLogger.Error().Err(err).Msg("Failed to register scheduler job")
		return nil, err
	}
	jobLogger.Info().Msg("Scheduler job registered")
	return job, nil
}
