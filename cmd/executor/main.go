package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/G-Research/armada/internal/common"
	"github.com/G-Research/armada/internal/executor"
	"github.com/G-Research/armada/internal/executor/configuration"
	"github.com/G-Research/armada/internal/executor/metrics"
)

const CustomConfigLocation string = "config"

func init() {
	pflag.String(CustomConfigLocation, "", "Fully qualified path to application configuration file")
	pflag.Parse()
}

func main() {
	common.ConfigureLogging()
	common.BindCommandlineArguments()

	var config configuration.ExecutorConfiguration
	userSpecifiedConfig := viper.GetString(CustomConfigLocation)
	common.LoadConfig(&config, "./config/executor", userSpecifiedConfig)

	shutdownChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownChannel, syscall.SIGINT, syscall.SIGTERM)

	shutdownMetricServer := common.ServeMetricsFor(config.Metric.Port,
		prometheus.Gatherers{metrics.GetMetricsGatherer()})
	defer shutdownMetricServer()

	shutdown, wg := executor.StartUp(config)
	go func() {
		<-shutdownChannel
		shutdown()
	}()
	wg.Wait()
}
