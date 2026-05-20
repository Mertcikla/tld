package jobs

import (
	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher {
	return append(pattern.FromSpecs(Specs()), ConcreteBindings())
}

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.bullmq", "TypeScript BullMQ", "typescript", "bullmq", "new Queue", "job.queue", "enqueues"),
		spec("ts.agenda", "TypeScript Agenda", "typescript", "agenda", "agenda.define", "job.handler", "handles_job"),
		spec("ts.node_cron", "TypeScript node-cron", "typescript", "node-cron", "cron.schedule", "job.schedule", "runs_on_schedule"),
		spec("go.robfig_cron", "Go robfig/cron", "go", "github.com/robfig/cron", "cron.New", "job.schedule", "runs_on_schedule"),
		spec("go.asynq", "Go asynq", "go", "github.com/hibiken/asynq", "asynq.NewServer", "job.queue", "consumes"),
		spec("go.machinery", "Go machinery", "go", "github.com/RichardKnop/machinery", "machinery", "job.queue", "consumes"),
		spec("python.celery", "Python Celery", "python", "celery", "@shared_task", "job.handler", "handles_job"),
		spec("python.rq", "Python RQ", "python", "rq", "Queue(", "job.queue", "consumes"),
		spec("python.apscheduler", "Python APScheduler", "python", "apscheduler", "add_job", "job.schedule", "runs_on_schedule"),
		spec("java.spring_scheduling", "Java Spring Scheduling", "java", "spring-context", "@Scheduled", "job.schedule", "runs_on_schedule"),
		spec("java.quartz", "Java Quartz", "java", "org.quartz", "JobBuilder", "job.handler", "handles_job"),
		spec("rust.tokio_cron_scheduler", "Rust tokio-cron-scheduler", "rust", "tokio-cron-scheduler", "JobScheduler", "job.schedule", "runs_on_schedule"),
		spec("rust.apalis", "Rust apalis", "rust", "apalis", "apalis::", "job.queue", "consumes"),
		spec("cpp.custom_scheduler", "C++ custom schedulers", "cpp", "cron", "schedule_every", "job.schedule", "runs_on_schedule"),
		spec("cpp.queue_consumer", "C++ queue consumers", "cpp", "queue", "consume_queue", "job.queue", "consumes"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "jobs",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"jobs:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
