# Go Test Transform

This utility transforms the output of Go's test JSON format to handle subtest failures more intelligently. It prevents parent tests from failing when only their subtests fail, while preserving the original test structure and output format.

## Features

- Transforms Go test JSON output to modify how test failures propagate
- Parent tests won't fail if they only have failing subtests but no direct failures
- Maintains the original JSON format for compatibility with other tools

## Key Behavior

- **Subtest Failure Handling**: When a subtest fails, the parent test will not be marked as failed unless the parent itself has a direct failure
- **Important Note**: If a parent test has both log messages (`t.Log()`) and failing subtests, it will still be marked as failed, as seen in the `TestLogMessagesNotDirectFailures` test

## Usage

```bash
go test -json ./... | go-test-transform -ignore-all
```

This will transform the test output to prevent parent tests from failing when only their subtests fail.

## Options

- `-ignore-all`: Ignore all subtest failures
- `-input`: Input file (default: stdin)
- `-output`: Output file (default: stdout)

## Example

For a test structure like:

```go
func TestParent(t *testing.T) {
	t.Run("Subtest1", func(t *testing.T) {
		t.Error("Subtest 1 failed")
	})
}
```

The transformed output will be:

```json
{"Time":"2023-05-10T15:04:05.123Z","Action":"run","Package":"example/pkg","Test":"TestParent"}
{"Time":"2023-05-10T15:04:05.124Z","Action":"run","Package":"example/pkg","Test":"TestParent/Subtest1"}
{"Time":"2023-05-10T15:04:05.125Z","Action":"output","Package":"example/pkg","Test":"TestParent/Subtest1","Output":"    subtest1_test.go:12: Subtest 1 failed\n"}
{"Time":"2023-05-10T15:04:05.126Z","Action":"fail","Package":"example/pkg","Test":"TestParent/Subtest1","Elapsed":0.001}
{"Time":"2023-05-10T15:04:05.127Z","Action":"pass","Package":"example/pkg","Test":"TestParent","Elapsed":0.004}
{"Time":"2023-05-10T15:04:05.128Z","Action":"output","Package":"example/pkg","Output":"FAIL\texample/pkg\t0.004s\n"}
{"Time":"2023-05-10T15:04:05.129Z","Action":"fail","Package":"example/pkg","Elapsed":0.005}
```

Note that in the original output, both `TestParent/Subtest1` and `TestParent` would be marked as failed. After transformation, `TestParent/Subtest1` remains failed, but `TestParent` is changed to pass since it doesn't have a direct failure.
