package executor

import (
	"os"
	"sync"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/G-Research/armada/internal/common/task"
	"github.com/G-Research/armada/internal/executor/cluster"
	"github.com/G-Research/armada/internal/executor/configuration"
	"github.com/G-Research/armada/internal/executor/context"
	"github.com/G-Research/armada/internal/executor/job_context"
	"github.com/G-Research/armada/internal/executor/metrics"
	"github.com/G-Research/armada/internal/executor/metrics/pod_metrics"
	"github.com/G-Research/armada/internal/executor/reporter"
	"github.com/G-Research/armada/internal/executor/service"
	"github.com/G-Research/armada/pkg/api"
	"github.com/G-Research/armada/pkg/client"
)

func StartUp(config configuration.ExecutorConfiguration) (func(), *sync.WaitGroup) {

	kubernetesClientProvider, err := cluster.NewKubernetesClientProvider(&config.Kubernetes)

	if err != nil {
		log.Errorf("Failed to connect to kubernetes because %s", err)
		os.Exit(-1)
	}

	clusterContext := context.NewClusterContext(
		config.Application,
		2*time.Minute,
		kubernetesClientProvider)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	taskManager := task.NewBackgroundTaskManager(metrics.ArmadaExecutorMetricsPrefix)
	taskManager.Register(clusterContext.ProcessPodsToDelete, config.Task.PodDeletionInterval, "pod_deletion")

	return StartUpWithContext(config, clusterContext, kubernetesClientProvider, taskManager, wg)
}

func StartUpWithContext(config configuration.ExecutorConfiguration, clusterContext context.ClusterContext, kubernetesClientProvider cluster.KubernetesClientProvider, taskManager *task.BackgroundTaskManager, wg *sync.WaitGroup) (func(), *sync.WaitGroup) {

	conn, err := createConnectionToApi(config)
	if err != nil {
		log.Errorf("Failed to connect to API because: %s", err)
		os.Exit(-1)
	}

	queueClient := api.NewAggregatedQueueClient(conn)
	usageClient := api.NewUsageClient(conn)
	eventClient := api.NewEventClient(conn)

	eventReporter, stopReporter := reporter.NewJobEventReporter(
		clusterContext,
		eventClient)

	jobContext := job_context.NewClusterJobContext(clusterContext)

	jobLeaseService := service.NewJobLeaseService(
		clusterContext,
		jobContext,
		queueClient,
		config.Kubernetes.MinimumPodAge,
		config.Kubernetes.FailedPodExpiry,
		config.Kubernetes.MinimumJobSize)

	queueUtilisationService := service.NewMetricsServerQueueUtilisationService(
		clusterContext)

	clusterUtilisationService := service.NewClusterUtilisationService(
		clusterContext,
		queueUtilisationService,
		usageClient,
		config.Kubernetes.TrackedNodeLabels,
		config.Kubernetes.ToleratedTaints)

	stuckPodDetector := service.NewPodProgressMonitorService(
		clusterContext,
		jobContext,
		eventReporter,
		jobLeaseService,
		config.Kubernetes.StuckPodExpiry)

	clusterAllocationService := service.NewClusterAllocationService(
		clusterContext,
		eventReporter,
		jobLeaseService,
		clusterUtilisationService)

	pod_metrics.ExposeClusterContextMetrics(clusterContext, clusterUtilisationService, queueUtilisationService)

	taskManager.Register(clusterUtilisationService.ReportClusterUtilisation, config.Task.UtilisationReportingInterval, "utilisation_reporting")
	taskManager.Register(clusterAllocationService.AllocateSpareClusterCapacity, config.Task.AllocateSpareClusterCapacityInterval, "job_lease_request")
	taskManager.Register(jobLeaseService.ManageJobLeases, config.Task.JobLeaseRenewalInterval, "job_lease_renewal")
	taskManager.Register(eventReporter.ReportMissingJobEvents, config.Task.MissingJobEventReconciliationInterval, "event_reconciliation")
	taskManager.Register(stuckPodDetector.HandleStuckPods, config.Task.StuckPodScanInterval, "stuck_pod")

	if config.Metric.ExposeQueueUsageMetrics {
		taskManager.Register(queueUtilisationService.RefreshUtilisationData, config.Task.QueueUsageDataRefreshInterval, "pod_usage_data_refresh")

		if config.Task.UtilisationEventReportingInterval > 0 {
			podUtilisationReporter := service.NewUtilisationEventReporter(
				clusterContext,
				queueUtilisationService,
				eventReporter,
				config.Task.UtilisationEventReportingInterval)
			taskManager.Register(podUtilisationReporter.ReportUtilisationEvents, config.Task.UtilisationEventProcessingInterval, "pod_utilisation_event_reporting")
		}
	}

	return func() {
		stopReporter <- true
		clusterContext.Stop()
		conn.Close()
		if taskManager.StopAll(2 * time.Second) {
			log.Warnf("Graceful shutdown timed out")
		}
		log.Infof("Shutdown complete")
		wg.Done()
	}, wg
}

func createConnectionToApi(config configuration.ExecutorConfiguration) (*grpc.ClientConn, error) {
	return client.CreateApiConnection(&config.ApiConnection,
		grpc.WithChainUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithChainStreamInterceptor(grpc_prometheus.StreamClientInterceptor))
}
