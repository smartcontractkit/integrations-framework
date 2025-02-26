# BenchSpy - Custom Loki Metrics

In this chapter, we’ll explore how to use custom `LogQL` queries in the performance report. For this more advanced use case, we’ll manually compose the performance report.

The load generation part is the same as in the standard Loki metrics example and will be skipped.

## Defining Custom Metrics

Let’s define two illustrative metrics:
- **`vu_over_time`**: The rate of virtual users generated by WASP, using a 10-second window.
- **`responses_over_time`**: The number of AUT's responses, using a 1-second window.

```go
lokiQueryExecutor := benchspy.NewLokiQueryExecutor(
    map[string]string{
        "vu_over_time":        fmt.Sprintf("max_over_time({branch=~\"%s\", commit=~\"%s\", go_test_name=~\"%s\", test_data_type=~\"stats\", gen_name=~\"%s\"} | json | unwrap current_instances [10s]) by (node_id, go_test_name, gen_name)", label, label, t.Name(), gen.Cfg.GenName),
        "responses_over_time": fmt.Sprintf("sum(count_over_time({branch=~\"%s\", commit=~\"%s\", go_test_name=~\"%s\", test_data_type=~\"responses\", gen_name=~\"%s\"} [1s])) by (node_id, go_test_name, gen_name)", label, label, t.Name(), gen.Cfg.GenName),
    },
    gen.Cfg.LokiConfig,
)
```

> [!NOTE]
> These `LogQL` queries use the standard labels that `WASP` applies when sending data to Loki.

## Creating a `StandardReport` with Custom Queries

Now, let’s create a `StandardReport` using our custom queries:

```go
baseLineReport, err := benchspy.NewStandardReport(
    "v1.0.0",
    // notice the different functional option used to pass Loki executor with custom queries
    benchspy.WithQueryExecutors(lokiQueryExecutor),
    benchspy.WithGenerators(gen),
)
require.NoError(t, err, "failed to create baseline report")
```

## Wrapping Up

The rest of the code remains unchanged, except for the names of the metrics being asserted. You can find the full example [here](...).

Now it’s time to look at the last of the bundled `QueryExecutors`. Proceed to the [next chapter to read about Prometheus](./prometheus_std.md).

> [!NOTE]
> You can find the full example [here](https://github.com/smartcontractkit/chainlink-testing-framework/tree/main/wasp/examples/benchspy/loki_query_executor/loki_query_executor_test.go).