package main

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
)

type mockCluster struct {
	brokerCount  int
	clusterName  string
	jmxExporter  bool
	nodeExporter bool
	kraft        bool
}

type mockKafkaClient struct{ clusters map[string]mockCluster }

func (m mockKafkaClient) ListClusters(ctx context.Context, params *kafka.ListClustersInput, optFns ...func(*kafka.Options)) (*kafka.ListClustersOutput, error) {
	var clusterInfoList []types.ClusterInfo
	keys := make([]string, 0)
	for k := range m.clusters {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		cArn := k
		cCluster := m.clusters[k]
		clusterInfoList = append(clusterInfoList, types.ClusterInfo{
			ClusterArn:  &cArn,
			ClusterName: &cCluster.clusterName,
			OpenMonitoring: &types.OpenMonitoring{
				Prometheus: &types.Prometheus{
					JmxExporter: &types.JmxExporter{
						EnabledInBroker: &cCluster.jmxExporter,
					},
					NodeExporter: &types.NodeExporter{
						EnabledInBroker: &cCluster.nodeExporter,
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

func (m mockKafkaClient) ListNodes(ctx context.Context, params *kafka.ListNodesInput, optFns ...func(*kafka.Options)) (*kafka.ListNodesOutput, error) {
	cluster := m.clusters[*params.ClusterArn]
	var nodeInfos []types.NodeInfo

	if cluster.kraft {
		for i := 1; i <= cluster.brokerCount; {
			n := types.NodeInfo{
				NodeType:           "CONTROLLER",
				ControllerNodeInfo: &types.ControllerNodeInfo{Endpoints: []string{fmt.Sprintf("c-1000%v.broker.com", i)}},
			}
			nodeInfos = append(nodeInfos, n)
			i++
		}
	}

	for i := 1; i <= cluster.brokerCount; {
		n := types.NodeInfo{
			NodeType:       "BROKER",
			BrokerNodeInfo: &types.BrokerNodeInfo{Endpoints: []string{fmt.Sprintf("b-%v.broker.com", i)}},
		}
		nodeInfos = append(nodeInfos, n)
		i++
	}
	out := kafka.ListNodesOutput{
		NodeInfoList: nodeInfos,
	}
	return &out, nil
}

func TestGetStaticConfigs(t *testing.T) {
	t.Run("OneClusterTwoBrokersFullMonitoring", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)
		client.clusters["arn:::my-cluster"] = mockCluster{
			brokerCount:  2,
			clusterName:  "my-cluster",
			jmxExporter:  true,
			nodeExporter: true,
		}

		got, _ := GetStaticConfigs(client)
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

	t.Run("OneClusterTwoBrokersFullMonitoringKraft", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)
		client.clusters["arn:::my-cluster"] = mockCluster{
			brokerCount:  2,
			clusterName:  "my-cluster",
			jmxExporter:  true,
			nodeExporter: true,
			kraft:        true,
		}

		got, _ := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{
			{
				Targets: []string{
					"c-10001.broker.com:11001",
					"c-10002.broker.com:11001",
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
		client.clusters["arn:::my-cluster"] = mockCluster{
			brokerCount:  2,
			clusterName:  "my-cluster",
			jmxExporter:  true,
			nodeExporter: true,
		}
		client.clusters["arn:::my-other-cluster"] = mockCluster{
			brokerCount:  2,
			clusterName:  "my-other-cluster",
			jmxExporter:  true,
			nodeExporter: false,
		}

		got, _ := GetStaticConfigs(client)
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
		client.clusters["arn:::my-cluster"] = mockCluster{
			brokerCount: 2,
			clusterName: "my-cluster",
		}

		got, _ := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s want %s", got, want)
		}
	})

	t.Run("NoClusters", func(t *testing.T) {
		var client mockKafkaClient
		client.clusters = make(map[string]mockCluster)

		got, _ := GetStaticConfigs(client)
		want := []PrometheusStaticConfig{}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s want %s", got, want)
		}
	})

}

func strPtr(str string) *string {
	return &str
}

func Test_filterClusters(t *testing.T) {
	type args struct {
		clusters kafka.ListClustersOutput
		filter   Filter
	}
	defaultFilter := Filter{
		NameFilter: *(regexp.MustCompile(``)),
	}

	testClusterFilter := Filter{
		NameFilter: *(regexp.MustCompile(`test`)),
	}

	tagFilter := Filter{
		NameFilter: *(regexp.MustCompile(``)),
		TagFilter: map[string]string{
			"Enviroment": "test",
			"SomeOther":  "tag",
		},
	}

	tests := []struct {
		name string
		args args
		want *kafka.ListClustersOutput
	}{
		{
			name: "empty-filter",
			args: args{
				clusters: kafka.ListClustersOutput{
					ClusterInfoList: []types.ClusterInfo{
						{
							ClusterName: strPtr("test-cluster"),
						},
					},
				},
				filter: defaultFilter,
			},
			want: &kafka.ListClustersOutput{
				ClusterInfoList: []types.ClusterInfo{
					{
						ClusterName: strPtr("test-cluster"),
					},
				},
			},
		},
		{
			name: "test-cluster-filter",
			args: args{
				clusters: kafka.ListClustersOutput{
					ClusterInfoList: []types.ClusterInfo{
						{
							ClusterName: strPtr("test-cluster"),
						},
						{
							ClusterName: strPtr("filtered-cluster"),
						},
					},
				},
				filter: testClusterFilter,
			},
			want: &kafka.ListClustersOutput{
				ClusterInfoList: []types.ClusterInfo{
					{
						ClusterName: strPtr("test-cluster"),
					},
				},
			},
		},
		{
			name: "test-tag-filter",
			args: args{
				clusters: kafka.ListClustersOutput{
					ClusterInfoList: []types.ClusterInfo{
						{
							ClusterName: strPtr("test-cluster"),
							Tags: map[string]string{
								"Enviroment": "test",
								"SomeOther":  "DifferentTag",
							},
						},
						{
							ClusterName: strPtr("second-test-cluster"),
							Tags: map[string]string{
								"Enviroment": "staging",
								"SomeOther":  "tag",
							},
						},
						{
							ClusterName: strPtr("filtered-cluster"),
						},
					},
				},
				filter: tagFilter,
			},
			want: &kafka.ListClustersOutput{
				ClusterInfoList: []types.ClusterInfo{
					{
						ClusterName: strPtr("test-cluster"),
						Tags: map[string]string{
							"Enviroment": "test",
							"SomeOther":  "DifferentTag",
						},
					},
					{
						ClusterName: strPtr("second-test-cluster"),
						Tags: map[string]string{
							"Enviroment": "staging",
							"SomeOther":  "tag",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterClusters(tt.args.clusters, tt.args.filter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterClusters() = %v, want %v", got, tt.want)
			}
		})
	}
}
