<div align="center">

# Chainlink Testing Framework

[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/smartcontractkit/chainlink-testing-framework)](https://github.com/smartcontractkit/chainlink-testing-framework/tags)
[![Go Report Card](https://goreportcard.com/badge/github.com/smartcontractkit/chainlink-testing-framework)](https://goreportcard.com/report/github.com/smartcontractkit/chainlink-testing-framework)
[![Go Reference](https://pkg.go.dev/badge/github.com/smartcontractkit/chainlink-testing-framework.svg)](https://pkg.go.dev/github.com/smartcontractkit/chainlink-testing-framework)
[![Go Version](https://img.shields.io/github/go-mod/go-version/smartcontractkit/chainlink-testing-framework)](https://go.dev/)
![Tests](https://github.com/smartcontractkit/chainlink-testing-framework/actions/workflows/test.yaml/badge.svg)
![Lint](https://github.com/smartcontractkit/chainlink-testing-framework/actions/workflows/lint.yaml/badge.svg)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

</div>

The Chainlink Testing Framework is a blockchain development framework written in Go. Its primary purpose is to help chainlink developers create extensive integration, e2e, performance, and chaos tests to ensure the stability of the chainlink project. It can also be helpful to those who just want to use chainlink oracles in their projects to help test their contracts, or even for those that aren't using chainlink.

If you're looking to implement a new chain integration for the testing framework, head over to the [blockchain](./blockchain/) directory for more info.

## k8s package
We have a k8s package we are using in tests, it provides:
- [cdk8s](https://cdk8s.io/) based wrappers
- High-level k8s API
- Automatic port forwarding

You can also use this package to spin up standalone environments.

### Local k8s cluster
Read [here](./k8s/KUBERNETES.md) about how to spin up a local cluster

#### Install
Set up deps, you need to have `node 14.x.x`, [helm](https://helm.sh/docs/intro/install/) and [yarn](https://classic.yarnpkg.com/lang/en/docs/install/#mac-stable)

Then use
```shell
make install_deps
```

### Running tests in k8s
To read how to run a test in k8s, read [here](./k8s/REMOTE_RUN.md)

### Usage
Create an env in a separate file and run it
```
export CHAINLINK_IMAGE="public.ecr.aws/chainlink/chainlink"
export CHAINLINK_TAG="1.4.0-root"
export CHAINLINK_ENV_USER="Satoshi"
go run k8s/examples/simple/env.go
```
For more features follow [tutorial](./k8s/TUTORIAL.md)

### Development
#### Running standalone example environment
```shell
go run k8s/examples/simple/env.go
```
If you have another env of that type, you can connect by overriding environment name
```
ENV_NAMESPACE="..."  go run k8s/examples/chainlink/env.go
```

Add more presets [here](./k8s/presets)

Add more programmatic examples [here](./k8s/examples/)

If you have [chaosmesh]() installed in your cluster you can pull and generated CRD in go like that
```
make chaosmesh
```

If you need to check your system tests coverage, use [that](./k8s/TUTORIAL.md#coverage)

# Chainlink Charts

This repository contains helm charts used by the chainlink organization mostly in QA.

## Chart Repository

You can add the published chart repository by pointing helm to the `gh-pages` branch with a personal access token (PAT) that has at least read-only access to the repository.

```sh
helm repo add chainlink-qa https://raw.githubusercontent.com/smartcontractkit/qa-charts/gh-pages/
helm search repo chainlink
```

## Releasing Charts

The following cases will trigger a chart release once a PR is merged into the `main` branch.
Modified packages or new packages get added and pushed to the `gh-pages` branch of the [qa-charts](https://github.com/smartcontractkit/qa-charts) repository.

- An existing chart is version bumped
- A new chart is added

Removed charts do not trigger a re-publish, the packages have to be removed and the index file regenerated in the `gh-pages` branch of the [qa-charts](https://github.com/smartcontractkit/qa-charts) repository.

Note: The qa-charts repository is scheduled to look for changes to the charts once every hour. This can be expedited by going to that repo and running the cd action via github UI.


# Simulated EVM chains

We have extended support for execution layer clients in simulated networks. Following ones are supported:
* `Geth`
* `Nethermind`
* `Besu`
* `Erigon`

When it comes to consensus layer we currently support only `Prysm`.

## Command line

You can start a simulated network with a single command:
```
go run docker/test_env/cmd/main.go start-test-env private-chain
```

By default it will start a network with 1 node running `Geth` and `Prysm`. It will use default chain id of `1337` and won't wait for the chain to finalize at least one epoch. Once the chain is started it will save the network configuration in a `JSON` file, which then you can use in your tests to connect to that chain (and thus save time it takes to start a new chain each time you run your test).

Following cmd line flags are available:
```
  -c, --chain-id int             chain id (default 1337)
  -l, --consensus-layer string   consensus layer (prysm) (default "prysm")
  -t, --consensus-type string    consensus type (pow or pos) (default "pos")
  -e, --execution-layer string   execution layer (geth, nethermind, besu or erigon) (default "geth")
  -w, --wait-for-finalization    wait for finalization of at least 1 epoch (might take up to 5 mintues)
```

To connect to that environment in your tests use the following code:
```
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		FromEnvVar().
		Build()

    if err != nil {
        return err
    }

	net, rpc, err := cfg.Start()
    if err != nil {
        return err
    }
```
Builder will read the location of chain configuration from env var named `PRIVATE_ETHEREUM_NETWORK_CONFIG_PATH` (it will be printed in the console once the chain starts).

`net` is an instance of `blockchain.EVMNetwork`, which contains characteristics of the network and can be used to connect to it using an EVM client. `rpc` variable contains arrays of public and private RPC endpoints, where "private" means URL that's accessible from the same Docker network as the chain is running in.
