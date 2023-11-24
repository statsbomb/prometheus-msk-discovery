[![statsbomb/prometheus-msk-discovery](https://img.shields.io/docker/pulls/statsbomb/prometheus-msk-discovery.svg)](https://hub.docker.com/r/statsbomb/prometheus-msk-discovery)
[![Test](https://github.com/statsbomb/prometheus-msk-discovery/actions/workflows/test.yaml/badge.svg)](https://github.com/statsbomb/prometheus-msk-discovery/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/statsbomb/prometheus-msk-discovery)](https://goreportcard.com/report/github.com/statsbomb/prometheus-msk-discovery)

# prometheus-msk-discovery

Service discovery for [AWS MSK](https://aws.amazon.com/msk/), compatible with [Prometheus](https://prometheus.io).

## How it works

This service gets a list of MSK clusters in an AWS account and exports each broker to a Prometheus-compatible static config to be used with the [`file_sd_config`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config) or [`http_sd_config`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#http_sd_config) mechanism.

## Pre-requisites

### 1) IAM Policy

When using AWS credentials or IAM Roles, the following policy needs to be attached to the role/user being used:

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "kafka:ListClusters",
                "kafka:ListNodes"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}
```

### 2) AWS Credentials

- No special provisions are made to obtain AWS credentials and instead that process is left to the AWS SDK. Credentials will be searched for automatically in the order specified in [the documentation](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/#specifying-credentials).

## Running it

```
Usage of ./prometheus-msk-discovery:
  -filter string
    	a regex pattern to filter cluster names from the results
  -http-sd
    	expose http_sd interface rather than writing a file
  -job-prefix string
    	string with which to prefix each job label (default "msk")
  -listen-address string
    	Address to listen on for http service discovery (default ":8080")
  -output string
    	path of the file to write MSK discovery information to (default "msk_file_sd.yml")
  -region string
    	the aws region in which to scan for MSK clusters
  -scrape-interval duration
    	interval at which to scrape the AWS API for MSK cluster information (default 5m0s)
  -tag value
    	A key=value for filtering by tags. Flag can be specified multiple times, resulting OR expression.
```

### Example output:

```
$ ./prometheus-msk-discovery -scrape-interval 10s -filter 'primary'
2021/04/04 21:02:55 Writing 1 discovered exporters to msk_file_sd.yml
```

An example output file can be found [here](examples/msk_file_sd.yml)

### http_sd

```
$ ./prometheus-msk-discovery -http-sd -listen-address :8989 -filter 'primary'
```

```
$ curl localhost:8989
[{"targets":["b-1.primary-kafka.tffs8g.c2.kafka.eu-west-2.amazonaws.com:11001","b-1.primary-kafka.tffs8g.c2.kafka.eu-west-2.amazonaws.com:11002","b-2.primary-kafka.tffs8g.c2.kafka.eu-west-2.amazonaws.com:11001","b-2.primary-kafka.tffs8g.c2.kafka.eu-west-2.amazonaws.com:11002"],"labels":{"job":"msk-primary-kafka","cluster_name":"primary-kafka","cluster_arn":"arn:aws:kafka:eu-west-2:111111111111:cluster/primary-kafka/522d90ab-d400-4ea0-b8fd-bbf3576425d4-2"}}]
```

```yaml
http_sd_configs:
  - url: http://localhost:8989
    refresh_interval: 30s
```

## Region Precedence
When no region is specified with the `-region` flag the process first attempts to load the default SDK configuration checking for an `AWS_REGION` environment variable or reading any region specified in the standard [configuration file](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html). If no region is found it will attempt to retrieve it from the EC2 Instance Metadata Service.

**Explicitly setting a region with the** `-region` **flag takes precedence over everything else.**

## Integration with kube-prometheus-stack

The [Docker image](https://hub.docker.com/r/statsbomb/prometheus-msk-discovery) for this project can be used to inject a container into the Prometheus Spec of a [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) installation by using the following snippet in `values.yaml`:

```yaml
prometheus:
  prometheusSpec:
    containers:
      - name: prometheus-msk-discovery
        image: statsbomb/prometheus-msk-discovery:latest
        args:
          - -output
          - /config-out/msk_file_sd.yml
        volumeMounts:
          - name: config-out
            mountPath: /config-out
```

You'll then need to add something similar to the following to your additionalScrapeConfig:

```yaml
- job_name: prometheus-msk-discovery
  file_sd_configs:
    - files:
        - /etc/prometheus/config_out/msk_file_sd.yml
  honor_labels: true
```

### Other Kubernetes Setups
If you're not using the kube-prometheus-stack Helm chart then the general idea for running prometheus-msk-discovery is that it needs to be run as a [sidecar container](https://kubernetes.io/docs/concepts/workloads/pods/#how-pods-manage-multiple-containers) alongside the container that is running Prometheus. You'll need to ensure that there is a [shared volume](https://kubernetes.io/docs/tasks/access-application-cluster/communicate-containers-same-pod-shared-volume/) mounted to both of the containers in order for Prometheus to be able to read the config file that prometheus-msk-discovery writes.

## Building

Building can be done just by using `go build`

## Contributing

Pull requests, issues and suggestions are appreciated.

## Credits

- Prometheus Authors
  - https://prometheus.io
- Prometheus ECS Discovery
  - Inspiration from https://github.com/teralytics/prometheus-ecs-discovery
- Prometheus Lightsail SD
  - README format from https://github.com/n888/prometheus-lightsail-sd

## License

Apache License 2.0, see [LICENSE](https://github.com/statsbomb/prometheus-msk-discovert/blob/master/LICENSE).
