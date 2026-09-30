package main

import (
	"context"
	"log"

	"mindstrata-stage1/api/internal/aireport"
	"mindstrata-stage1/api/internal/anonymizer"
	"mindstrata-stage1/api/internal/billing"
	"mindstrata-stage1/api/internal/db"
	"mindstrata-stage1/api/internal/dbbackup"
	"mindstrata-stage1/api/internal/dbmaintenance"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	anonCfg := anonymizer.ConfigFromEnv()
	billingCfg := billing.ConfigFromEnv()
	reportCfg := aireport.ConfigFromEnv()
	if anonCfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := db.OpenPool(ctx, anonCfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database pool: %v", err)
	}
	defer pool.Close()

	temporalClient, err := client.Dial(client.Options{HostPort: anonCfg.TemporalAddress})
	if err != nil {
		log.Fatalf("create temporal client: %v", err)
	}
	defer temporalClient.Close()

	if err := billing.EnsureDailyRenewalsSchedule(ctx, temporalClient, billingCfg); err != nil {
		log.Fatalf("ensure billing renewal schedule: %v", err)
	}
	if err := anonymizer.EnsureAnonymizationSchedule(ctx, temporalClient, anonCfg); err != nil {
		log.Fatalf("ensure anonymization schedule: %v", err)
	}
	if err := dbbackup.EnsureBackupSchedule(ctx, temporalClient, dbbackup.ConfigFromEnv()); err != nil {
		log.Fatalf("ensure db backup schedule: %v", err)
	}
	if err := dbmaintenance.EnsureModesMarkdownSchedule(ctx, temporalClient, dbmaintenance.ConfigFromEnv()); err != nil {
		log.Fatalf("ensure modes markdown normalization schedule: %v", err)
	}
	if err := aireport.EnsureDailyReportSchedule(ctx, temporalClient, reportCfg); err != nil {
		log.Fatalf("ensure ai provider daily report schedule: %v", err)
	}

	anonWorker := worker.New(temporalClient, anonymizer.GoTaskQueue+anonCfg.QueueSuffix, worker.Options{})
	anonActivities := anonymizer.NewActivities(pool, anonCfg)
	maintenanceActivities := dbmaintenance.NewActivities(pool)
	reportActivities := aireport.NewActivities(pool, reportCfg.EnvLabel)
	anonWorker.RegisterWorkflow(anonymizer.AnonymizeMessagesWorkflow)
	anonWorker.RegisterWorkflow(dbbackup.DatabaseBackupWorkflow)
	anonWorker.RegisterWorkflow(dbmaintenance.NormalizeModesMarkdownWorkflow)
	anonWorker.RegisterWorkflow(aireport.DailyAIProviderReportWorkflow)
	anonWorker.RegisterActivity(anonActivities.GetMessageBatch)
	anonWorker.RegisterActivity(anonActivities.WriteAnonymizedBatch)
	anonWorker.RegisterActivity(anonActivities.CountPendingMessages)
	anonWorker.RegisterActivityWithOptions(maintenanceActivities.NormalizeModesMarkdown, activity.RegisterOptions{Name: dbmaintenance.NormalizeModesActivity})
	anonWorker.RegisterActivityWithOptions(reportActivities.SendAIProviderDailyReport, activity.RegisterOptions{Name: aireport.ReportActivity})

	billingWorker := worker.New(temporalClient, billing.TaskQueue+billingCfg.QueueSuffix, worker.Options{})
	billingActivities := billing.NewActivities(pool, billingCfg)
	billingWorker.RegisterWorkflow(billing.DailyYooKassaRenewalsWorkflow)
	billingWorker.RegisterActivityWithOptions(billingActivities.StartBillingRun, activity.RegisterOptions{Name: billing.StartBillingRunActivity})
	billingWorker.RegisterActivityWithOptions(billingActivities.ReconcilePendingYooKassaInvoices, activity.RegisterOptions{Name: billing.ReconcileInvoicesActivity})
	billingWorker.RegisterActivityWithOptions(billingActivities.RunDueYooKassaRenewals, activity.RegisterOptions{Name: billing.RunDueRenewalsActivity})
	billingWorker.RegisterActivityWithOptions(billingActivities.MarkExhaustedRenewals, activity.RegisterOptions{Name: billing.MarkExhaustedRenewalsActivity})
	billingWorker.RegisterActivityWithOptions(billingActivities.FinishBillingRun, activity.RegisterOptions{Name: billing.FinishBillingRunActivity})

	log.Printf("starting anonymizer Go worker on task queue %q via %s", anonymizer.GoTaskQueue+anonCfg.QueueSuffix, anonCfg.TemporalAddress)
	if err := anonWorker.Start(); err != nil {
		log.Fatalf("start anonymizer worker: %v", err)
	}
	defer anonWorker.Stop()

	log.Printf("starting billing worker on task queue %q via %s", billing.TaskQueue+billingCfg.QueueSuffix, anonCfg.TemporalAddress)
	if err := billingWorker.Start(); err != nil {
		log.Fatalf("start billing worker: %v", err)
	}
	defer billingWorker.Stop()

	<-worker.InterruptCh()
}
