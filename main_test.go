package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
)

type mockCluster struct {
	brokerCount  int
	clusterName  string
	jmxExporter  bool
	nodeExporter bool
}

type mockKafkaClient struct{ clusters map[string]mockCluster }

func (m mockKafkaClient) ListClusters(ctx context.Context, params *kafka.ListClustersInput, optFns ...func(*kafka.Options)) (*kafka.ListClustersOutput, error) {
	var clusterInfoList []types.ClusterInfo
	for arn, cluster := range m.clusters {
		cArn := arn
		cCluster := cluster
		clusterInfoList = append(clusterInfoList, types.ClusterInfo{
			ClusterArn:  &cArn,
			ClusterName: &cCluster.clusterName,
			OpenMonitoring: &types.OpenMonitoring{
				Prometheus: &types.Prometheus{
					JmxExporter: &types.JmxExporter{
						EnabledInBroker: cCluster.jmxExporter,
					},
					NodeExporter: &types.NodeExporter{
						EnabledInBroker: cCluster.nodeExporter,
					},
				},
			},
		})
	}
	output := kafka.ListClustersOutput{
		ClusterInfoList: clusterInfoList,
	}
	return &output, nil
}

func (m mockKafkaClient) GetBootstrapBrokers(ctx context.Context, params *kafka.GetBootstrapBrokersInput, optFns ...func(*kafka.Options)) (*kafka.GetBootstrapBrokersOutput, error) {
	cluster := m.clusters[*params.ClusterArn]
	var brokers []string

	for i := 1; i <= cluster.brokerCount; {
		brokers = append(brokers, fmt.Sprintf("b-%v.broker.com:9002", i))
		i++
	}

	brokerString := strings.Join(brokers, ",")
	output := kafka.GetBootstrapBrokersOutput{
		BootstrapBrokerString: &brokerString,
	}
	return &output, nil
}

func TestGetStaticConfigs(t *testing.T) {
	t.Run("OneClusterTwoBrokersFullMonitoring", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)
		client.clusters["arn:::my-cluster"] = mockCluster{2, "my-cluster", true, true}

		got := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{
			{
				Targets: []string{
					"b-1.broker.com:11001",
					"b-1.broker.com:11002",
					"b-2.broker.com:11001",
					"b-2.broker.com:11002",
				},
				Labels: labels{
					Job:         "msk-my-cluster",
					ClusterName: "my-cluster",
					ClusterArn:  "arn:::my-cluster",
				},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s want %s", got, want)
		}
	})

	t.Run("TwoClusterTwoBrokersFullAndLimitedMonitoring", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)
		client.clusters["arn:::my-cluster"] = mockCluster{2, "my-cluster", true, true}
		client.clusters["arn:::my-other-cluster"] = mockCluster{2, "my-other-cluster", true, false}

		got := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{
			{
				Targets: []string{
					"b-1.broker.com:11001",
					"b-1.broker.com:11002",
					"b-2.broker.com:11001",
					"b-2.broker.com:11002",
				},
				Labels: labels{
					Job:         "msk-my-cluster",
					ClusterName: "my-cluster",
					ClusterArn:  "arn:::my-cluster",
				},
			},
			{
				Targets: []string{
					"b-1.broker.com:11001",
					"b-2.broker.com:11001",
				},
				Labels: labels{
					Job:         "msk-my-other-cluster",
					ClusterName: "my-other-cluster",
					ClusterArn:  "arn:::my-other-cluster",
				},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s want %s", got, want)
		}
	})

	t.Run("NoMonitoringEnabled", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)
		client.clusters["arn:::my-cluster"] = mockCluster{2, "my-cluster", false, false}

		got := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s want %s", got, want)
		}
	})

	t.Run("NoClusters", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)

		got := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s want %s", got, want)
		}
	})

}
