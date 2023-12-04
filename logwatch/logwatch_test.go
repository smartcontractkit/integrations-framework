package logwatch_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	"github.com/smartcontractkit/chainlink-testing-framework/logwatch"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/testcontext"
)

type TestCase struct {
	name                  string
	containers            int
	msg                   string
	msgsAmount            int
	msgsIntervalSeconds   float64
	exitEarly             bool
	mustNotifyList        map[string][]*regexp.Regexp
	expectedNotifications map[string][]*logwatch.LogNotification
}

// replaceContainerNamePlaceholders this function is used to replace container names with dynamic values
// so we can run tests in parallel
func replaceContainerNamePlaceholders(tc TestCase) []string {
	dynamicContainerNames := make([]string, 0)
	for i := 0; i < tc.containers; i++ {
		staticSortedIndex := strconv.Itoa(i)
		containerName := uuid.NewString()
		dynamicContainerNames = append(dynamicContainerNames, containerName)
		if tc.mustNotifyList != nil {
			tc.mustNotifyList[containerName] = tc.mustNotifyList[staticSortedIndex]
			delete(tc.mustNotifyList, staticSortedIndex)
			for _, log := range tc.expectedNotifications[staticSortedIndex] {
				log.Container = containerName
				log.Prefix = containerName
			}
			tc.expectedNotifications[containerName] = tc.expectedNotifications[staticSortedIndex]
			delete(tc.expectedNotifications, staticSortedIndex)
		}
	}
	return dynamicContainerNames
}

// startTestContainer with custom streams emitted
func startTestContainer(ctx context.Context, containerName string, msg string, amount int, intervalSeconds float64, exitEarly bool) (testcontainers.Container, error) {
	var cmd []string
	if exitEarly {
		cmd = []string{"bash", "-c",
			fmt.Sprintf(
				"for i in {1..%d}; do sleep %.2f; echo '%s'; done",
				amount,
				intervalSeconds,
				msg,
			)}
	} else {
		cmd = []string{"bash", "-c",
			fmt.Sprintf(
				"for i in {1..%d}; do sleep %.2f; echo '%s'; done; while true; do sleep 1; done",
				amount,
				intervalSeconds,
				msg,
			)}
	}
	req := testcontainers.ContainerRequest{
		Name:  containerName,
		Image: "ubuntu:latest",
		Cmd:   cmd,
	}
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

func TestLogWatchDocker(t *testing.T) {
	tests := []TestCase{
		{
			name:                "should read exactly 10 streams (1 container)",
			containers:          1,
			msg:                 "hello!",
			msgsAmount:          10,
			msgsIntervalSeconds: 0.1,
		},
		{
			name:                "should read exactly 10 streams even if container exits (1 container)",
			containers:          1,
			msg:                 "hello!",
			msgsAmount:          10,
			msgsIntervalSeconds: 0.1,
			exitEarly:           true,
		},
		{
			name:                "should read exactly 100 streams fast (1 container)",
			containers:          1,
			msg:                 "hello!",
			msgsAmount:          100,
			msgsIntervalSeconds: 0.01,
		},
		{
			name:                "should read exactly 100 streams fast even if container exits (1 container)",
			containers:          1,
			msg:                 "hello!",
			msgsAmount:          100,
			msgsIntervalSeconds: 0.01,
			exitEarly:           true,
		},
		{
			name:                "should read exactly 10 streams (2 containers)",
			msg:                 "A\nB\nC\nD",
			containers:          2,
			msgsAmount:          1,
			msgsIntervalSeconds: 0.1,
		},
	}

	var getExpectedMsgCount = func(msg string, msgAmount int) int {
		splitted := strings.Split(msg, "\n")
		return len(splitted) * msgAmount
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testcontext.Get(t)
			dynamicContainerNames := replaceContainerNamePlaceholders(tc)
			lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogTarget(logwatch.InMemory))
			require.NoError(t, err)

			for _, cn := range dynamicContainerNames {
				container, err := startTestContainer(ctx, cn, tc.msg, tc.msgsAmount, tc.msgsIntervalSeconds, tc.exitEarly)
				require.NoError(t, err)
				name, err := container.Name(ctx)
				require.NoError(t, err)
				err = lw.ConnectContainer(context.Background(), container, name)
				require.NoError(t, err)
			}

			// streams should be there with a gap of 1 second
			time.Sleep(time.Duration(int(tc.msgsIntervalSeconds*float64(tc.msgsAmount)))*time.Second + 1*time.Second)

			// all streams should be recorded
			for _, cn := range dynamicContainerNames {
				logs, err := lw.ContainerLogs(cn)
				require.NoError(t, err, "should not fail to get logs")

				require.Len(t, logs, getExpectedMsgCount(tc.msg, tc.msgsAmount))
			}

			defer func() {
				// testcontainers/ryuk:v0.5.1 will handle the shutdown automatically if container exited
				// container.IsReady() is inconsistent and not always showing that container has exited
				// ontainer.Terminate() and container.StopLogProducer() has known bugs, if you call them they can hang
				// forever if container is already exited
				// https://github.com/testcontainers/testcontainers-go/pull/1085
				// tried latest branch with a fix, but no luck
				// this code terminates the containers properly
				for _, c := range lw.GetConsumers() {
					if !tc.exitEarly {
						c.Stop()
						if err := lw.DisconnectContainer(c.GetContainer()); err != nil {
							t.Fatalf("failed to disconnect container: %s", err.Error())
						}
						container := c.GetContainer()
						if err := container.Terminate(ctx); err != nil {
							t.Fatalf("failed to terminate container: %s", err.Error())
						}
					}
				}
			}()
		})
	}
}

func TestLogWatchConnectWithDelayDocker(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	containerName := fmt.Sprintf("%s-container-%s", "TestLogWatchConnectRetryDocker", uuid.NewString())
	message := "message"
	interval := float64(1)
	amount := 10

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err)
	container, err := startTestContainer(ctx, containerName, message, amount, interval, false)
	require.NoError(t, err)
	name, err := container.Name(ctx)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	err = lw.ConnectContainer(context.Background(), container, name)
	require.NoError(t, err)

	time.Sleep(time.Duration(int(interval*float64(amount)))*time.Second + 5*time.Second)

	logs, err := lw.ContainerLogs(containerName)
	require.NoError(t, err, "should not fail to get logs")

	require.Len(t, logs, amount)

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err.Error())
		}
	})
}

func TestLogWatchTwoDockerContainers(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	containerOneName := fmt.Sprintf("%s-container-%s", t.Name(), uuid.NewString())
	containerTwoName := fmt.Sprintf("%s-container-%s", t.Name(), uuid.NewString())
	message := "message"
	interval := float64(1)
	amountFirst := 10
	amountSecond := 20

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err, "log watch should be created")
	containerOne, err := startTestContainer(ctx, containerOneName, message, amountFirst, interval, false)
	require.NoError(t, err, "should not fail to start container")

	containerTwo, err := startTestContainer(ctx, containerTwoName, message, amountSecond, interval, false)
	require.NoError(t, err, "should not fail to start container")

	err = lw.ConnectContainer(context.Background(), containerOne, containerOneName)
	require.NoError(t, err, "log watch should connect to container")

	err = lw.ConnectContainer(context.Background(), containerTwo, containerTwoName)
	require.NoError(t, err, "log watch should connect to container")

	time.Sleep(time.Duration(int(interval*float64(amountFirst)))*time.Second + 5*time.Second)

	for _, c := range lw.GetConsumers() {
		name, err := c.GetContainer().Name(ctx)
		require.NoError(t, err, "should not fail to get container name")
		if name == containerOneName {
			c.Stop()
			err = lw.DisconnectContainer(containerOne)
			require.NoError(t, err, "log watch should disconnect from container")
		}
	}

	time.Sleep(time.Duration(int(interval*float64(amountSecond-amountFirst)))*time.Second + 5*time.Second)

	logs, err := lw.ContainerLogs(containerOneName)
	require.NoError(t, err, "should not fail to get logs")

	require.Len(t, logs, amountFirst, "wrong number of logs received from first container")

	logs, err = lw.ContainerLogs(containerTwoName)
	require.NoError(t, err, "should not fail to get logs")
	require.Len(t, logs, amountSecond, "wrong number of logs received from first container")

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
		if err := containerOne.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate first container: %s", err.Error())
		}
		if err := containerTwo.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate second container: %s", err.Error())
		}
	})
}

type MockedLogProducingContainer struct {
	name              string
	id                string
	isRunning         bool
	consumer          testcontainers.LogConsumer
	startError        error
	startSleep        time.Duration
	stopError         error
	errorChannelError error
	startCounter      int
	messages          []string
	errorCh           chan error
}

func (m *MockedLogProducingContainer) Name(ctx context.Context) (string, error) {
	return m.name, nil
}

func (m *MockedLogProducingContainer) FollowOutput(consumer testcontainers.LogConsumer) {
	m.consumer = consumer
}

func (m *MockedLogProducingContainer) StartLogProducer(ctx context.Context, timeout time.Duration) error {
	m.startCounter++
	m.errorCh = make(chan error, 1)

	if m.startError != nil {
		return m.startError
	}

	if m.startSleep > 0 {
		time.Sleep(m.startSleep)
	}

	go func() {
		lastProcessedLogIndex := -1
		for {
			time.Sleep(200 * time.Millisecond)

			m.errorCh <- m.errorChannelError
			if m.errorChannelError != nil {
				return
			}

			for i, msg := range m.messages {
				time.Sleep(50 * time.Millisecond)
				if i <= lastProcessedLogIndex {
					continue
				}
				lastProcessedLogIndex = i
				m.consumer.Accept(testcontainers.Log{
					LogType: testcontainers.StdoutLog,
					Content: []byte(msg),
				})
			}
		}
	}()

	return nil
}

func (m *MockedLogProducingContainer) StopLogProducer() error {
	return m.stopError
}

func (m *MockedLogProducingContainer) GetLogProducerErrorChannel() <-chan error {
	return m.errorCh
}

func (m *MockedLogProducingContainer) IsRunning() bool {
	return m.isRunning
}

func (m *MockedLogProducingContainer) Terminate(context.Context) error {
	return nil
}

func (m *MockedLogProducingContainer) GetContainerID() string {
	return m.id
}

func (m *MockedLogProducingContainer) SendLog(msg string) {
	m.messages = append(m.messages, msg)
}

// secenario: log watch consumes a log, then the container returns an error, log watch reconnects
// and consumes logs again. log watch should not miss any logs nor consume any log twice
func TestLogWatchConnectRetryMockContainer_FailsOnce(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	uuid := uuid.NewString()
	amount := 10
	interval := float64(1.12)

	mockedContainer := &MockedLogProducingContainer{
		name:              fmt.Sprintf("%s-container-%s", t.Name(), uuid),
		id:                uuid,
		isRunning:         true,
		startError:        nil,
		stopError:         nil,
		errorChannelError: nil,
	}

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogProducerTimeout(1*time.Second), logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err, "log watch should be created")

	go func() {
		// wait for 1 second, so that log watch has time to consume at least one log before it's stopped
		time.Sleep(1 * time.Second)
		mockedContainer.startSleep = 1 * time.Second
		logs, err := lw.ContainerLogs(mockedContainer.name)
		require.NoError(t, err, "should not fail to get logs")
		require.True(t, len(logs) > 0, "should have received at least 1 log before injecting error")
		mockedContainer.errorChannelError = errors.New("failed to read logs")

		// clear the error after 1 second, so that log producer can resume log consumption
		time.Sleep(1 * time.Second)
		mockedContainer.errorChannelError = nil
	}()

	logsSent := []string{}
	go func() {
		for i := 0; i < amount; i++ {
			toSend := fmt.Sprintf("message-%d", i)
			logsSent = append(logsSent, toSend)
			mockedContainer.SendLog(toSend)
			time.Sleep(time.Duration(time.Duration(interval) * time.Second))
		}
	}()

	err = lw.ConnectContainer(context.Background(), mockedContainer, mockedContainer.name)
	require.NoError(t, err, "log watch should connect to container")

	time.Sleep(time.Duration(int(interval*float64(amount)))*time.Second + 3*time.Second)

	logs, err := lw.ContainerLogs(mockedContainer.name)
	require.NoError(t, err, "should not fail to get logs")
	require.EqualValues(t, logs, logsSent, "log watch should receive all logs")
	require.Equal(t, 2, mockedContainer.startCounter, "log producer should be started twice")

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
	})
}

// secenario: log watch consumes a log, then the container returns an error, log watch reconnects
// and consumes logs again, then it happens again. log watch should not miss any logs nor consume any log twice
func TestLogWatchConnectRetryMockContainer_FailsTwice(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	uuid := uuid.NewString()
	amount := 10
	interval := float64(1.12)

	mockedContainer := &MockedLogProducingContainer{
		name:              fmt.Sprintf("%s-container-%s", t.Name(), uuid),
		id:                uuid,
		isRunning:         true,
		startError:        nil,
		stopError:         nil,
		errorChannelError: nil,
	}

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogProducerTimeout(1*time.Second), logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err, "log watch should be created")

	go func() {
		// wait for 1 second, so that log watch has time to consume at least one log before it's stopped
		time.Sleep(1 * time.Second)
		mockedContainer.startSleep = 1 * time.Second
		logs, err := lw.ContainerLogs(mockedContainer.name)
		require.NoError(t, err, "should not fail to get logs")
		require.True(t, len(logs) > 0, "should have received at least 1 log before injecting error, but got 0")
		mockedContainer.errorChannelError = errors.New("failed to read logs")

		// clear the error after 1 second, so that log producer can resume log consumption
		time.Sleep(1 * time.Second)
		mockedContainer.errorChannelError = nil

		// wait for 3 seconds so that some logs are consumed before we inject error again
		time.Sleep(3 * time.Second)
		mockedContainer.startSleep = 1 * time.Second
		logs, err = lw.ContainerLogs(mockedContainer.name)
		require.NoError(t, err, "should not fail to get logs")
		require.True(t, len(logs) > 0, "should have received at least 1 log before injecting error, but got 0")
		mockedContainer.errorChannelError = errors.New("failed to read logs")

		// clear the error after 1 second, so that log producer can resume log consumption
		time.Sleep(1 * time.Second)
		mockedContainer.errorChannelError = nil
	}()

	logsSent := []string{}
	go func() {
		for i := 0; i < amount; i++ {
			toSend := fmt.Sprintf("message-%d", i)
			logsSent = append(logsSent, toSend)
			mockedContainer.SendLog(toSend)
			time.Sleep(time.Duration(time.Duration(interval) * time.Second))
		}
	}()

	err = lw.ConnectContainer(context.Background(), mockedContainer, mockedContainer.name)
	require.NoError(t, err, "log watch should connect to container")

	time.Sleep(time.Duration(int(interval*float64(amount)))*time.Second + 5*time.Second)

	logs, err := lw.ContainerLogs(mockedContainer.name)
	require.NoError(t, err, "should not fail to get logs")

	require.EqualValues(t, logs, logsSent, "log watch should receive all logs")
	require.Equal(t, 3, mockedContainer.startCounter, "log producer should be started twice")

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
	})
}

// secenario: it consumes a log, then the container returns an error, but when log watch tries to reconnect log producer
// is still running, but finally it stops and log watch reconnects. log watch should not miss any logs nor consume any log twice
func TestLogWatchConnectRetryMockContainer_FailsFirstRestart(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	uuid := uuid.NewString()
	amount := 10
	interval := float64(1)

	mockedContainer := &MockedLogProducingContainer{
		name:              fmt.Sprintf("%s-container-%s", t.Name(), uuid),
		id:                uuid,
		isRunning:         true,
		startError:        nil,
		stopError:         nil,
		errorChannelError: nil,
	}

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogProducerTimeout(1*time.Second), logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err, "log watch should be created")

	go func() {
		// wait for 1 second, so that log watch has time to consume at least one log before it's stopped
		time.Sleep(1 * time.Second)
		mockedContainer.startSleep = 1 * time.Second
		logs, err := lw.ContainerLogs(mockedContainer.name)
		require.NoError(t, err, "should not fail to get logs")
		require.True(t, len(logs) > 0, "should have received at least 1 log before injecting error, but got 0")

		// introduce read error, so that log producer stops
		mockedContainer.errorChannelError = errors.New("failed to read logs")
		// inject start error, that simulates log producer still running (e.g. closing connection to the container)
		mockedContainer.startError = errors.New("still running")

		// wait for one second before clearing errors, so that we retry to connect
		time.Sleep(1 * time.Second)
		mockedContainer.startError = nil
		mockedContainer.errorChannelError = nil
	}()

	logsSent := []string{}
	go func() {
		for i := 0; i < amount; i++ {
			toSend := fmt.Sprintf("message-%d", i)
			logsSent = append(logsSent, toSend)
			mockedContainer.SendLog(toSend)
			time.Sleep(time.Duration(time.Duration(interval) * time.Second))
		}
	}()

	err = lw.ConnectContainer(context.Background(), mockedContainer, mockedContainer.name)
	require.NoError(t, err, "log watch should connect to container")

	time.Sleep(time.Duration(int(interval*float64(amount)))*time.Second + 5*time.Second)

	logs, err := lw.ContainerLogs(mockedContainer.name)
	require.NoError(t, err, "should not fail to get logs")

	require.EqualValues(t, logsSent, logs, "log watch should receive all logs")
	require.Equal(t, 3, mockedContainer.startCounter, "log producer should be started four times")

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
	})
}

// secenario: it consumes a log, then the container returns an error, but when log watch tries to reconnect log producer
// is still running and log watch never reconnects. log watch should have no logs (we could improve that in the future)
func TestLogWatchConnectRetryMockContainer_AlwaysFailsRestart(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	uuid := uuid.NewString()
	amount := 10
	interval := float64(1)

	mockedContainer := &MockedLogProducingContainer{
		name:              fmt.Sprintf("%s-container-%s", t.Name(), uuid),
		id:                uuid,
		isRunning:         true,
		startError:        nil,
		stopError:         nil,
		errorChannelError: nil,
	}

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogProducerTimeout(1*time.Second), logwatch.WithLogProducerRetryLimit(4), logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err, "log watch should be created")

	go func() {
		// wait for 1 second, so that log watch has time to consume at least one log before it's stopped
		time.Sleep(6 * time.Second)
		mockedContainer.startSleep = 1 * time.Second
		logs, err := lw.ContainerLogs(mockedContainer.name)
		require.NoError(t, err, "should not fail to get logs")
		require.True(t, len(logs) > 0, "should have received at least 1 log before injecting error, but got 0")

		// introduce read error, so that log producer stops
		mockedContainer.errorChannelError = errors.New("failed to read logs")
		// inject start error, that simulates log producer still running (e.g. closing connection to the container)
		mockedContainer.startError = errors.New("still running")
	}()

	go func() {
		for i := 0; i < amount; i++ {
			toSend := fmt.Sprintf("message-%d", i)
			mockedContainer.SendLog(toSend)
			time.Sleep(time.Duration(time.Duration(interval) * time.Second))
		}
	}()

	err = lw.ConnectContainer(context.Background(), mockedContainer, mockedContainer.name)
	require.NoError(t, err, "log watch should connect to container")

	time.Sleep(time.Duration(int(interval*float64(amount)))*time.Second + 5*time.Second)

	// it should still salvage 6 logs that were consumed before error was injected and restarting failed
	logs, err := lw.ContainerLogs(mockedContainer.name)
	require.NoError(t, err, "should not fail to get logs")
	require.Equal(t, 0, len(logs), "log watch should have no logs")
	require.Equal(t, 5, mockedContainer.startCounter, "log producer should be started seven times")

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
	})
}

// scenario: log listening loops are independent for all containers/consumers and even if one of them stops
// due to errors, second one continues and receives all logs
func TestLogWatchConnectRetryTwoMockContainers_FirstAlwaysFailsRestart_SecondWorks(t *testing.T) {
	t.Parallel()
	ctx := testcontext.Get(t)
	uuid_1 := uuid.NewString()
	uuid_2 := uuid.NewString()
	amountFirst := 10
	amountSecond := 20
	interval := float64(1)

	mockedContainer_1 := &MockedLogProducingContainer{
		name:              fmt.Sprintf("%s-container-%s", t.Name(), uuid_1),
		id:                uuid_1,
		isRunning:         true,
		startError:        nil,
		stopError:         nil,
		errorChannelError: nil,
	}

	mockedContainer_2 := &MockedLogProducingContainer{
		name:              fmt.Sprintf("%s-container-%s", t.Name(), uuid_2),
		id:                uuid_2,
		isRunning:         true,
		startError:        nil,
		stopError:         nil,
		errorChannelError: nil,
	}

	lw, err := logwatch.NewLogWatch(t, nil, logwatch.WithLogProducerTimeout(1*time.Second), logwatch.WithLogProducerRetryLimit(4), logwatch.WithLogTarget(logwatch.InMemory))
	require.NoError(t, err, "log watch should be created")

	go func() {
		// wait for 1 second, so that log watch has time to consume at least one log before it's stopped
		time.Sleep(6 * time.Second)
		mockedContainer_1.startSleep = 1 * time.Second
		logs, err := lw.ContainerLogs(mockedContainer_1.name)
		require.NoError(t, err, "should not fail to get logs")
		require.True(t, len(logs) > 0, "should have received at least 1 log before injecting error, but got 0")

		// introduce read error, so that log producer stops
		mockedContainer_1.errorChannelError = errors.New("failed to read logs")
		// inject start error, that simulates log producer still running (e.g. closing connection to the container)
		mockedContainer_1.startError = errors.New("still running")
	}()

	go func() {
		for i := 0; i < amountFirst; i++ {
			toSend := fmt.Sprintf("message-%d", i)
			mockedContainer_1.SendLog(toSend)
			time.Sleep(time.Duration(time.Duration(interval) * time.Second))
		}
	}()

	logsSent := []string{}
	go func() {
		for i := 0; i < amountSecond; i++ {
			toSend := fmt.Sprintf("message-%d", i)
			logsSent = append(logsSent, toSend)
			mockedContainer_2.SendLog(toSend)
			time.Sleep(time.Duration(time.Duration(interval) * time.Second))
		}
	}()

	err = lw.ConnectContainer(context.Background(), mockedContainer_1, mockedContainer_1.name)
	require.NoError(t, err, "log watch should connect to container")

	err = lw.ConnectContainer(context.Background(), mockedContainer_2, mockedContainer_2.name)
	require.NoError(t, err, "log watch should connect to container")

	time.Sleep(time.Duration(int(interval*float64(amountSecond)))*time.Second + 5*time.Second)

	logs_1, err := lw.ContainerLogs(mockedContainer_1.name)
	require.NoError(t, err, "should not fail to get logs")
	require.Equal(t, 0, len(logs_1), "log watch should have no logs")
	require.Equal(t, 5, mockedContainer_1.startCounter, "log producer should be started seven times for first container")

	logs_2, err := lw.ContainerLogs(mockedContainer_2.name)
	require.NoError(t, err, "should not fail to get logs")
	require.Equal(t, amountSecond, len(logs_2), "log watch should have all logs for second container")
	require.EqualValues(t, logsSent, logs_2, "log watch had different logs for second container than expected")
	require.Equal(t, 1, mockedContainer_2.startCounter, "log producer should be started one time for second container")

	t.Cleanup(func() {
		if err := lw.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shutodwn logwatch: %s", err.Error())
		}
	})
}
