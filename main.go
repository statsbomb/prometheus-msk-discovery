package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
	"gopkg.in/yaml.v2"
)

const (
	jmxExporterPort  = 11001
	nodeExporterPort = 11002
)

type tags map[string]string

var (
	outFile       = flag.String("output", "msk_file_sd.yml", "path of the file to write MSK discovery information to")
	interval      = flag.Duration("scrape-interval", 5*time.Minute, "interval at which to scrape the AWS API for MSK cluster information")
	jobPrefix     = flag.String("job-prefix", "msk", "string with which to prefix each job label")
	clusterFilter = flag.String("filter", "", "a regex pattern to filter cluster names from the results")
	awsRegion     = flag.String("region", "", "the aws region in which to scan for MSK clusters")
)

type kafkaClient interface {
	ListClusters(ctx context.Context, params *kafka.ListClustersInput, optFns ...func(*kafka.Options)) (*kafka.ListClustersOutput, error)
	ListNodes(ctx context.Context, params *kafka.ListNodesInput, optFns ...func(*kafka.Options)) (*kafka.ListNodesOutput, error)
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

type Filter struct {
	NameFilter regexp.Regexp
	TagFilter  tags
}

func (i *tags) String() string {
	return fmt.Sprint(*i)
}
func (i *tags) Set(value string) error {
	split := strings.Split(value, "=")

	(*i)[split[0]] = split[1]
	return nil
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

// getClusters returns a ListClusterOutput of MSK cluster details
func getClusters(svc kafkaClient) (*kafka.ListClustersOutput, error) {
	input := &kafka.ListClustersInput{}
	output := &kafka.ListClustersOutput{}

	p := kafka.NewListClustersPaginator(svc, input)
	for p.HasMorePages() {
		page, err := p.NextPage(context.TODO())
		if err != nil {
			return &kafka.ListClustersOutput{}, err
		}
		output.ClusterInfoList = append(output.ClusterInfoList, page.ClusterInfoList...)
	}
	return output, nil
}

// getBrokers returns a slice of broker hosts without ports
func getBrokers(svc kafkaClient, arn string) ([]string, error) {
	input := kafka.ListNodesInput{ClusterArn: &arn}
	var brokers []string

	p := kafka.NewListNodesPaginator(svc, &input)
	for p.HasMorePages() {
		page, err := p.NextPage(context.Background())
		if err != nil {
			return nil, err
		}

		for _, b := range page.NodeInfoList {
			brokers = append(brokers, b.BrokerNodeInfo.Endpoints...)
		}
	}

	return brokers, nil
}

// buildClusterDetails extracts the relevant details from a ClusterInfo and returns a ClusterDetails
func buildClusterDetails(svc kafkaClient, c types.ClusterInfo) (clusterDetails, error) {
	brokers, err := getBrokers(svc, *c.ClusterArn)
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

func filterClusters(clusters kafka.ListClustersOutput, filter Filter) *kafka.ListClustersOutput {
	var filteredClusters []types.ClusterInfo
	var tagMatch bool
	for _, cluster := range clusters.ClusterInfoList {
		if len(filter.TagFilter) == 0 {
			tagMatch = true
		} else {
			tagMatch = false
		}
		for tagKey, tagValue := range filter.TagFilter {
			if cluster.Tags[tagKey] == tagValue {
				tagMatch = true
				break
			}
		}
		if filter.NameFilter.MatchString(*cluster.ClusterName) && tagMatch {
			filteredClusters = append(filteredClusters, cluster)
		}
	}

	return &kafka.ListClustersOutput{ClusterInfoList: filteredClusters}
}

// GetStaticConfigs pulls a list of MSK clusters and brokers and returns a slice of PrometheusStaticConfigs
func GetStaticConfigs(svc kafkaClient, opt_filter ...Filter) ([]PrometheusStaticConfig, error) {
	clusters, err := getClusters(svc)
	if err != nil {
		return []PrometheusStaticConfig{}, err
	}
	staticConfigs := []PrometheusStaticConfig{}

	// Assign a default Filter, if none is passed.
	defaultNameRegex, _ := regexp.Compile(``)
	filter := Filter{
		NameFilter: *defaultNameRegex,
	}
	if len(opt_filter) > 0 {
		filter = opt_filter[0]
	}

	clusters = filterClusters(*clusters, filter)

	for _, cluster := range clusters.ClusterInfoList {
		clusterDetails, err := buildClusterDetails(svc, cluster)
		if err != nil {
			return []PrometheusStaticConfig{}, err
		}

		if !clusterDetails.JmxExporter && !clusterDetails.NodeExporter {
			continue
		}
		staticConfigs = append(staticConfigs, clusterDetails.StaticConfig())
	}
	return staticConfigs, nil
}

func fileSD(client *kafka.Client, filter Filter) {
	work := func() {

		staticConfigs, err := GetStaticConfigs(client, filter)
		if err != nil {
			fmt.Println(err)
			return
		}

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

func main() {
	var tagFilters tags = make(tags)
	flag.Var(&tagFilters, "tag", "A key=value for filtering by tags. Flag can be specified multiple times, resulting OR expression.")
	flag.Parse()

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(*awsRegion), config.WithEC2IMDSRegion())
	if err != nil {
		fmt.Println(err)
		return
	}

	client := kafka.NewFromConfig(cfg)

	regexpFilter, err := regexp.Compile(*clusterFilter)
	if err != nil {
		fmt.Println(err)
		return
	}

	filter := Filter{
		NameFilter: *regexpFilter,
		TagFilter:  tagFilters,
	}

	fileSD(client, filter)
}
