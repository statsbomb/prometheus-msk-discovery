[![statsbomb/prometheus-msk-discovery](https://img.shields.io/docker/pulls/statsbomb/prometheus-msk-discovery.svg)](https://hub.docker.com/r/statsbomb/prometheus-msk-discovery)
[![Test](https://github.com/statsbomb/prometheus-msk-discovery/actions/workflows/test.yaml/badge.svg)](https://github.com/statsbomb/prometheus-msk-discovery/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/statsbomb/prometheus-msk-discovery)](https://goreportcard.com/report/github.com/statsbomb/prometheus-msk-discovery)

# prometheus-msk-discovery

Service discovery for [AWS MSK](https://aws.amazon.com/msk/), compatible with [Prometheus](https://prometheus.io).

## How it works

This service gets a list of MSK clusters in an AWS account and exports each broker to a Prometheus-compatible static config to be used with the [`file_sd_config`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config) mechanism.

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
                "kafka:GetBootstrapBrokers
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
  -job-prefix string
    	string with which to prefix each job label (default "msk")
  -output string
    	path of the file to write MSK discovery information to (default "msk_file_sd.yml")
  -scrape-interval duration
    	interval at which to scrape the AWS API for MSK cluster information (default 5m0s)
```

### Example output:

```
$ ./prometheus-msk-discovery -scrape-interval 10s
2021/04/04 21:02:55 Writing 1 discovered exporters to msk_file_sd.yml
```

An example output file can be found [here](examples/msk_file_sd.yml)

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
