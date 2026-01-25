package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/db"
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
func AddJob(name, cronExpr string, task func(), options ...gocron.JobOption) (gocron.Job, error) {
	svc, err := ServiceInstance()
	if err != nil {
		return nil, err
	}
	return svc.AddJob(name, cronExpr, task, options...)
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
func (s *Service) AddJob(name, cronExpr string, task func(), options ...gocron.JobOption) (gocron.Job, error) {
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

	jobOptions := append([]gocron.JobOption{gocron.WithName(name)}, options...)
	job, err := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(wrappedTask),
		jobOptions...,
	)
	if err != nil {
		jobLogger.Error().Err(err).Msg("Failed to register scheduler job")
		return nil, err
	}
	jobLogger.Info().Msg("Scheduler job registered")
	return job, nil
}

// RegisterWaitlistJobs registers scheduled waitlist maintenance tasks.
func RegisterWaitlistJobs(database *db.DB) error {
	if database == nil {
		return fmt.Errorf("waitlist jobs require database")
	}

	expireJobName := "waitlist_offer_expiry"
	expireCronExpr := "* * * * *"
	expireLogger := log.With().
		Str("component", "waitlist_offer_expiry_job").
		Str("job_name", expireJobName).
		Str("cron", expireCronExpr).
		Logger()

	_, err := AddJob(expireJobName, expireCronExpr, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		ctx = expireLogger.WithContext(ctx)

		if err := ExpireWaitlistOffers(ctx, database, time.Now()); err != nil {
			expireLogger.Error().Err(err).Msg("Waitlist offer expiry run failed")
		}
	}, gocron.WithSingletonMode(gocron.LimitModeWait))
	if err != nil {
		return fmt.Errorf("add waitlist offer expiry job: %w", err)
	}
	expireLogger.Info().Msg("Waitlist offer expiry job registered")

	cleanupJobName := "waitlist_cleanup"
	cleanupCronExpr := "0 * * * *"
	cleanupLogger := log.With().
		Str("component", "waitlist_cleanup_job").
		Str("job_name", cleanupJobName).
		Str("cron", cleanupCronExpr).
		Logger()

	_, err = AddJob(cleanupJobName, cleanupCronExpr, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		ctx = cleanupLogger.WithContext(ctx)

		if err := CleanupPastWaitlists(ctx, database, time.Now()); err != nil {
			cleanupLogger.Error().Err(err).Msg("Waitlist cleanup run failed")
		}
	}, gocron.WithSingletonMode(gocron.LimitModeWait))
	if err != nil {
		return fmt.Errorf("add waitlist cleanup job: %w", err)
	}
	cleanupLogger.Info().Msg("Waitlist cleanup job registered")

	return nil
}
