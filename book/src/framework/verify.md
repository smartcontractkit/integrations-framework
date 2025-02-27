# Verifying Contracts

## Using Foundry

You need to install [Foundry](https://book.getfoundry.sh/getting-started/installation) first, `forge` should be available in your `$PATH`.

Check out our [example](https://github.com/smartcontractkit/chainlink-testing-framework/blob/main/framework/examples/myproject/verify_test.go) of programmatically verifying contracts using `Blockscout` and `Foundry`. You'll need to provide:

- The path to your Foundry directory
- The path to the contract
- The contract name

```golang
		err := blockchain.VerifyContract(blockchainComponentOutput, c.Addresses[0].String(),
			"example_components/onchain",
			"src/Counter.sol",
			"Counter",
            "0.8.20" // check your file compiler version on the first line
		)
		require.NoError(t, err)
```

## Using Seth

If you don't want to verify contracts or you can't or don't want to use `Blockscout` not all is lost.

### With CLI

You can use `Seth` to trace your transaction both from your Go code or from [the CLI](https://smartcontractkit.github.io/chainlink-testing-framework/libs/seth.html#single-transaction-tracing). Remember that you need to adjust `seth.toml` to point to gethwrappers or ABIs of contracts you want to trace.

### Programmatic

If you want to use from Go code, you need to have a couple of things in mind:
* you need to point Seth to your Gethwrappers, so that it can extract ABIs from them
* you need to decide, when it should trace transactions: reverted, none or all (by default, only reverted ones are traced)
* you need to decide, where to output tracing results: console, dot graphs, json files (be default, to console)

If printing to console remember to set `Seth` log level to `debug`, otherwise you won't see anything relevant printed:
```
```



```go
client, err := NewClientBuilder().
    WithNetworkName("my network").
    WithRpcUrl("ws://localhost:8546").
    WithPrivateKeys([]string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"}).
    // tracing
    WithTracing(seth.TracingLevel_All, []string{seth.TraceOutput_Console}).
    // folder with gethwrappers for ABI decoding
    WithGethWrappersFolders([]string{"./gethwrappers/ccip", "./gethwrappers/keystone"}).
    Build()

if err != nil {
    log.Fatal(err)
}
```

For more information about configuring `Seth` please read about [TOML config](https://smartcontractkit.github.io/chainlink-testing-framework/libs/seth.html#toml-configuration) and [programmatic builder](https://smartcontractkit.github.io/chainlink-testing-framework/libs/seth.html#config).