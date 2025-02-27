package parrot_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/chainlink-testing-framework/parrot"
)

func ExampleServer_internal() {
	// Create a new parrot instance with no logging and a custom save file
	saveFile := "register_example.json"
	p, err := parrot.NewServer(parrot.WithLogLevel(zerolog.NoLevel), parrot.WithSaveFile(saveFile))
	if err != nil {
		panic(err)
	}
	defer func() { // Cleanup the parrot instance
		err = p.Shutdown(context.Background()) // Gracefully shutdown the parrot instance
		if err != nil {
			panic(err)
		}
		p.WaitShutdown()    // Wait for the parrot instance to shutdown. Usually unnecessary, but we want to clean up the save file
		os.Remove(saveFile) // Cleanup the save file for the example
	}()

	// Create a new route /test that will return a 200 status code with a text/plain response body of "Squawk"
	route := &parrot.Route{
		Method:             http.MethodGet,
		Path:               "/test",
		RawResponseBody:    "Squawk",
		ResponseStatusCode: http.StatusOK,
	}

	waitForParrotServerInternal(p, time.Second) // Wait for the parrot server to start

	// Register the route with the parrot instance
	err = p.Register(route)
	if err != nil {
		panic(err)
	}

	// Call the route
	resp, err := p.Call(http.MethodGet, "/test")
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.StatusCode())
	fmt.Println(string(resp.Body()))

	// Get all routes from the parrot instance
	routes := p.Routes()
	fmt.Println(len(routes))

	// Delete the route
	p.Delete(route)

	// Get all routes from the parrot instance
	routes = p.Routes()
	fmt.Println(len(routes))
	// Output:
	// 200
	// Squawk
	// 1
	// 0
}

func ExampleServer_external() {
	var (
		saveFile = "route_example.json"
		port     = 9090
	)
	defer os.Remove(saveFile) // Cleanup the save file for the example

	go func() { // Run the parrot server as a separate instance, like in a Docker container
		_, err := parrot.NewServer(parrot.WithPort(port), parrot.WithLogLevel(zerolog.NoLevel), parrot.WithSaveFile(saveFile))
		if err != nil {
			panic(err)
		}
	}()

	// Get a client to interact with the parrot server
	client := parrot.NewClient(fmt.Sprintf("http://localhost:%d", port))
	waitForParrotServerExternal(client, time.Second) // Wait for the parrot server to start

	// Register a new route /test that will return a 200 status code with a text/plain response body of "Squawk"
	route := &parrot.Route{
		Method:             http.MethodGet,
		Path:               "/test",
		RawResponseBody:    "Squawk",
		ResponseStatusCode: http.StatusOK,
	}
	err := client.RegisterRoute(route)
	if err != nil {
		panic(err)
	}
	fmt.Println("Registered route")

	// Get all routes from the parrot server
	routes, err := client.Routes()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Found %d routes\n", len(routes))

	// Delete the route
	err = client.DeleteRoute(route)
	if err != nil {
		panic(err)
	}
	fmt.Println("Deleted route")

	// Get all routes from the parrot server
	routes, err = client.Routes()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Found %d routes\n", len(routes))

	// Output:
	// Registered route
	// Found 1 routes
	// Deleted route
	// Found 0 routes
}

func ExampleRecorder_internal() {
	saveFile := "recorder_example.json"
	p, err := parrot.NewServer(parrot.WithLogLevel(zerolog.NoLevel), parrot.WithSaveFile(saveFile))
	if err != nil {
		panic(err)
	}
	defer func() { // Cleanup the parrot instance
		err = p.Shutdown(context.Background()) // Gracefully shutdown the parrot instance
		if err != nil {
			panic(err)
		}
		p.WaitShutdown()    // Wait for the parrot instance to shutdown. Usually unnecessary, but we want to clean up the save file
		os.Remove(saveFile) // Cleanup the save file for the example
	}()

	// Create a new recorder
	recorder, err := parrot.NewRecorder()
	if err != nil {
		panic(err)
	}

	waitForParrotServerInternal(p, time.Second) // Wait for the parrot server to start

	// Register the recorder with the parrot instance
	err = p.Record(recorder.URL())
	if err != nil {
		panic(err)
	}
	defer recorder.Close()

	// Register a new route /test that will return a 200 status code with a text/plain response body of "Squawk"
	route := &parrot.Route{
		Method:             http.MethodGet,
		Path:               "/test",
		RawResponseBody:    "Squawk",
		ResponseStatusCode: http.StatusOK,
	}
	err = p.Register(route)
	if err != nil {
		panic(err)
	}

	// Call the route
	go func() {
		_, err := p.Call(http.MethodGet, "/test")
		if err != nil {
			panic(err)
		}
	}()

	// Record the route call
	for {
		select {
		case recordedRouteCall := <-recorder.Record():
			if recordedRouteCall.RouteID == route.ID() {
				fmt.Println(recordedRouteCall.RouteID)
				fmt.Println(recordedRouteCall.Request.Method)
				fmt.Println(recordedRouteCall.Response.StatusCode)
				fmt.Println(string(recordedRouteCall.Response.Body))
				return
			}
		case err := <-recorder.Err():
			panic(err)
		}
	}
	// Output:
	// GET:/test
	// GET
	// 200
	// Squawk
}

// Example of how to use parrot recording when calling it from an external service
func ExampleRecorder_external() {
	var (
		saveFile = "recorder_example.json"
		port     = 9091
	)
	defer os.Remove(saveFile) // Cleanup the save file for the example

	go func() { // Run the parrot server as a separate instance, like in a Docker container
		_, err := parrot.NewServer(parrot.WithPort(port), parrot.WithLogLevel(zerolog.NoLevel), parrot.WithSaveFile(saveFile))
		if err != nil {
			panic(err)
		}
	}()

	client := parrot.NewClient(fmt.Sprintf("http://localhost:%d", port))
	waitForParrotServerExternal(client, time.Second) // Wait for the parrot server to start

	// Register a new route /test that will return a 200 status code with a text/plain response body of "Squawk"
	route := &parrot.Route{
		Method:             http.MethodGet,
		Path:               "/test",
		RawResponseBody:    "Squawk",
		ResponseStatusCode: http.StatusOK,
	}

	// Register the route with the parrot instance
	err := client.RegisterRoute(route)
	if err != nil {
		panic(err)
	}

	// Use the recorderHost of the machine your recorder is running on
	// This should not be localhost if you are running the parrot server on a different machine
	// It should be the public IP address of the machine running your code, so that the parrot can call back to it
	recorderHost := "localhost"

	// Create a new recorder with our host
	recorder, err := parrot.NewRecorder(parrot.WithRecorderHost(recorderHost))
	if err != nil {
		panic(err)
	}

	// Register the recorder with the parrot instance
	err = client.RegisterRecorder(recorder)
	if err != nil {
		panic(err)
	}

	recorders, err := client.Recorders()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Found %d recorders\n", len(recorders))

	go func() { // Some other service calls the /test route
		_, err := client.CallRoute(http.MethodGet, "/test")
		if err != nil {
			panic(err)
		}
	}()

	// You can now listen to the recorder for all route calls
	for {
		select {
		case recordedRouteCall := <-recorder.Record():
			if recordedRouteCall.RouteID == route.ID() {
				fmt.Println(recordedRouteCall.RouteID)
				fmt.Println(recordedRouteCall.Request.Method)
				fmt.Println(recordedRouteCall.Response.StatusCode)
				fmt.Println(string(recordedRouteCall.Response.Body))
				return
			}
		case err := <-recorder.Err():
			panic(err)
		}
	}
	// Output:
	// Found 1 recorders
	// GET:/test
	// GET
	// 200
	// Squawk
}

// waitForParrotServerExternal checks the parrot server health endpoint until it returns a 200 status code or the timeout is reached
func waitForParrotServerExternal(client *parrot.Client, timeoutDur time.Duration) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(timeoutDur)
	for { // Wait for the parrot server to start
		select {
		case <-ticker.C:
			healthy, err := client.Healthy()
			if err != nil {
				continue // Ignore errors for health check as the server may not be ready yet
			}
			if healthy {
				return
			}
		case <-timeout.C:
			panic("timeout waiting for parrot server to start")
		}
	}
}

func waitForParrotServerInternal(p *parrot.Server, timeoutDur time.Duration) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(timeoutDur)
	for { // Wait for the parrot server to start
		select {
		case <-ticker.C:
			if err := p.Healthy(); err == nil {
				return
			}
		case <-timeout.C:
			panic("timeout waiting for parrot server to start")
		}
	}
}
