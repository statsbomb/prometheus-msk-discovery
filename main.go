package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
	"gopkg.in/yaml.v2"
)

const jmxExporterPort = 11001
const nodeExporterPort = 11002

var outFile = flag.String("output", "msk_file_sd.yml", "path of the file to write MSK discovery information to")
var interval = flag.Duration("scrape-interval", 5*time.Minute, "interval at which to scrape the AWS API for MSK cluster information")
var jobPrefix = flag.String("job-prefix", "msk", "string with which to prefix each job label")

type kafkaClient interface {
	ListClusters(ctx context.Context, params *kafka.ListClustersInput, optFns ...func(*kafka.Options)) (*kafka.ListClustersOutput, error)
	GetBootstrapBrokers(ctx context.Context, params *kafka.GetBootstrapBrokersInput, optFns ...func(*kafka.Options)) (*kafka.GetBootstrapBrokersOutput, error)
}

type labels struct {
	Job         string `yaml:"job"`
	ClusterName string `yaml:"cluster_name"`
	ClusterArn  string `yaml:"cluster_arn"`
}

// PrometheusStaticConfig is the final structure of a single static config that
// will be outputted to the Prometheus file service discovery config
type PrometheusStaticConfig struct {
	Targets []string `yaml:"targets"`
	Labels  labels   `yaml:"labels"`
}

// clusterDetails holds details of cluster, each broker, and which OpenMetrics endpoints are enabled
type clusterDetails struct {
	ClusterName  string
	ClusterArn   string
	Brokers      []string
	JmxExporter  bool
	NodeExporter bool
}

// (ClusterDetails).StaticConfig generates a PrometheusStaticConfig based on the cluster's details
func (c clusterDetails) StaticConfig() PrometheusStaticConfig {
	ret := PrometheusStaticConfig{}
	ret.Labels = labels{
		Job:         strings.Join([]string{*jobPrefix, c.ClusterName}, "-"),
		ClusterName: c.ClusterName,
		ClusterArn:  c.ClusterArn,
	}

	var targets []string
	for _, b := range c.Brokers {
		if c.JmxExporter {
			targets = append(targets, fmt.Sprintf("%s:%d", b, jmxExporterPort))
		}
		if c.NodeExporter {
			targets = append(targets, fmt.Sprintf("%s:%d", b, nodeExporterPort))
		}
	}
	ret.Targets = targets
	return ret
}

// GetClusters returns a ListClusterOutput of MSK cluster details
func GetClusters(svc kafkaClient) (*kafka.ListClustersOutput, error) {
	input := &kafka.ListClustersInput{}
	output := &kafka.ListClustersOutput{}

	p := kafka.NewListClustersPaginator(svc, input)
	for p.HasMorePages() {
		page, err := p.NextPage(context.TODO())
		if err != nil {
			log.Fatal("failed to get page", err)
		}
		output.ClusterInfoList = append(output.ClusterInfoList, page.ClusterInfoList...)
	}
	return output, nil
}

// GetBrokers returns a slice of broker hosts without ports
func GetBrokers(svc kafkaClient, arn string) ([]string, error) {
	input := &kafka.GetBootstrapBrokersInput{ClusterArn: &arn}
	r, err := svc.GetBootstrapBrokers(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var brokers []string
	for _, b := range strings.Split(*r.BootstrapBrokerString, ",") {
		brokers = append(brokers, strings.Split(b, ":")[0])
	}
	return brokers, nil
}

// BuildClusterDetails extracts the relevant details from a ClusterInfo and returns a ClusterDetails
func BuildClusterDetails(svc kafkaClient, c types.ClusterInfo) (clusterDetails, error) {
	brokers, err := GetBrokers(svc, *c.ClusterArn)
	if err != nil {
		fmt.Println(err)
		return clusterDetails{}, err
	}

	cluster := clusterDetails{
		ClusterName:  *c.ClusterName,
		ClusterArn:   *c.ClusterArn,
		Brokers:      brokers,
		JmxExporter:  c.OpenMonitoring.Prometheus.JmxExporter.EnabledInBroker,
		NodeExporter: c.OpenMonitoring.Prometheus.NodeExporter.EnabledInBroker,
	}
	return cluster, nil
}

func GetStaticConfigs(svc kafkaClient) []PrometheusStaticConfig {
	clusters, _ := GetClusters(svc)
	staticConfigs := []PrometheusStaticConfig{}

	for _, cluster := range clusters.ClusterInfoList {
		clusterDetails, _ := BuildClusterDetails(svc, cluster)
		if !clusterDetails.JmxExporter && !clusterDetails.NodeExporter {
			continue
		}
		staticConfigs = append(staticConfigs, clusterDetails.StaticConfig())
	}
	return staticConfigs
}

func main() {
	flag.Parse()

	cfg, _ := config.LoadDefaultConfig(context.TODO())
	client := kafka.NewFromConfig(cfg)

	work := func() {

		staticConfigs := GetStaticConfigs(client)
		m, err := yaml.Marshal(staticConfigs)
		if err != nil {
			fmt.Println(err)
			return
		}

		log.Printf("Writing %d discovered exporters to %s", len(staticConfigs), *outFile)
		err = ioutil.WriteFile(*outFile, m, 0644)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	s := time.NewTimer(1 * time.Millisecond)
	t := time.NewTicker(*interval)
	for {
		select {
		case <-s.C:
		case <-t.C:
		}
		work()
	}

}
