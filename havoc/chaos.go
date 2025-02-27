package havoc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Chaos struct {
	Object        client.Object
	Description   string
	DelayCreate   time.Duration // Delay before creating the chaos object
	Status        ChaosStatus
	Client        client.Client
	listeners     []ChaosListener
	cancelMonitor context.CancelFunc
	startTime     time.Time
	endTime       time.Time
	logger        *zerolog.Logger
	remove        bool
}

// ChaosStatus represents the status of a chaos experiment.
type ChaosStatus string

// These constants define possible states of a chaos experiment.
const (
	StatusCreated        ChaosStatus = "created"
	StatusCreationFailed ChaosStatus = "creation_failed"
	StatusRunning        ChaosStatus = "running"
	StatusPaused         ChaosStatus = "paused"
	StatusFinished       ChaosStatus = "finished"
	StatusDeleted        ChaosStatus = "deleted"
	StatusUnknown        ChaosStatus = "unknown" // For any state that doesn't match the above
)

type ChaosOpts struct {
	Object      client.Object
	Description string
	DelayCreate time.Duration
	Client      client.Client
	Listeners   []ChaosListener
	Logger      *zerolog.Logger
	Remove      bool
}

// NewChaos creates a new Chaos instance based on the provided options.
// It requires a client, a chaos object, and a logger to function properly.
// This function is essential for initializing chaos experiments in a Kubernetes environment.
func NewChaos(opts ChaosOpts) (*Chaos, error) {
	if opts.Client == nil {
		return nil, errors.New("client is required")
	}
	if opts.Object == nil {
		return nil, errors.New("chaos object is required")
	}
	if opts.Logger == nil {
		return nil, errors.New("logger is required")
	}

	return &Chaos{
		Object:      opts.Object,
		Description: opts.Description,
		DelayCreate: opts.DelayCreate,
		Client:      opts.Client,
		listeners:   opts.Listeners,
		logger:      opts.Logger,
		remove:      opts.Remove,
	}, nil
}

// Create initiates a delayed creation of a chaos object, respecting context cancellation and deletion requests.
// It uses a timer based on `DelayCreate` and calls `create` method upon expiration unless preempted by deletion.
func (c *Chaos) Create(ctx context.Context) {
	done := make(chan struct{})

	// Create the timer with the delay to create the chaos object
	timer := time.NewTimer(c.DelayCreate)

	go func() {
		select {
		case <-ctx.Done():
			// If the context is canceled, stop the timer and exit
			if !timer.Stop() {
				<-timer.C // If the timer already expired, drain the channel
			}
			close(done) // Signal that the operation was canceled
		case <-timer.C:
			// Timer expired, check if deletion was not requested
			if c.Status != StatusDeleted {
				c.createNow(ctx)
			}
			close(done) // Signal that the creation process is either done or skipped
		}
	}()
}

func (c *Chaos) Update(ctx context.Context) error {
	// Modify the resource
	// For example, adding or updating an annotation
	annotations := c.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["example.com/trigger-injection"] = "true"
	c.Object.SetAnnotations(annotations)

	//nolint
	if err := c.Client.Update(ctx, c.Object); err != nil {
		return errors.Wrap(err, "failed to update chaos object")
	}

	return nil
}

// createNow is a private method that encapsulates the chaos object creation logic.
func (c *Chaos) createNow(ctx context.Context) {
	if err := c.Client.Create(ctx, c.Object); err != nil {
		c.notifyListeners(string(StatusCreationFailed), err)
		return
	}
	c.notifyListeners(string(StatusCreated), nil)

	// Create a cancellable context for monitorStatus
	monitorCtx, cancel := context.WithCancel(ctx)
	c.cancelMonitor = cancel
	go c.monitorStatus(monitorCtx)
}

func (c *Chaos) Pause(ctx context.Context) error {
	err := c.updateChaosObject(ctx)
	if err != nil {
		return errors.Wrap(err, "could not update the chaos object")
	}

	annotations := c.Object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[v1alpha1.PauseAnnotationKey] = strconv.FormatBool(true)
	c.Object.SetAnnotations(annotations)

	err = c.Client.Update(ctx, c.Object)
	if err != nil {
		return errors.Wrap(err, "could not update the annotation to set the chaos experiment into pause state")
	}

	c.notifyListeners("paused", nil)
	return nil
}

func (c *Chaos) Resume(ctx context.Context) error {
	// Implement resume logic here
	c.notifyListeners("resumed", nil)
	return nil
}

// Delete stops the chaos operation, updates its status, and removes the chaos object if specified. 
// It notifies listeners of the operation's completion and handles any errors encountered during the process.
func (c *Chaos) Delete(ctx context.Context) error {
	defer func() {
		// Cancel the monitoring goroutine
		if c.cancelMonitor != nil {
			c.cancelMonitor()
		}
	}()

	// If the chaos was running or paused, update the status and notify listeners
	if c.Status == StatusPaused || c.Status == StatusRunning {
		err := c.updateChaosObject(ctx)
		if err != nil {
			return errors.Wrap(err, "could not update the chaos object")
		}
		c.Status = StatusFinished
		c.endTime = time.Now()
		c.notifyListeners("finished", nil)
	}

	if c.remove {
		if err := c.Client.Delete(ctx, c.Object); err != nil {
			return errors.Wrap(err, "failed to delete chaos object")
		}
		c.Status = StatusDeleted
		c.logger.Info().Str("name", c.GetChaosName()).Msg("Chaos deleted")
	}
	return nil
}

func (c *Chaos) GetObject() client.Object {
	return c.Object
}

func (c *Chaos) GetChaosName() string {
	return c.Object.GetName()
}

func (c *Chaos) GetChaosDescription() string {
	return c.Description
}

func (c *Chaos) GetChaosTypeStr() string {
	switch c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		return "NetworkChaos"
	case *v1alpha1.IOChaos:
		return "IOChaos"
	case *v1alpha1.StressChaos:
		return "StressChaos"
	case *v1alpha1.PodChaos:
		return "PodChaos"
	case *v1alpha1.HTTPChaos:
		return "HTTPChaos"
	default:
		return "Unknown"
	}
}

func (c *Chaos) GetChaosSpec() interface{} {
	switch spec := c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		return spec.Spec
	case *v1alpha1.IOChaos:
		return spec.Spec
	case *v1alpha1.StressChaos:
		return spec.Spec
	case *v1alpha1.PodChaos:
		return spec.Spec
	case *v1alpha1.HTTPChaos:
		return spec.Spec
	default:
		return nil
	}
}

func (c *Chaos) GetChaosDuration() (time.Duration, error) {
	var durationStr *string
	switch spec := c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		durationStr = spec.Spec.Duration
	case *v1alpha1.IOChaos:
		durationStr = spec.Spec.Duration
	case *v1alpha1.StressChaos:
		durationStr = spec.Spec.Duration
	case *v1alpha1.PodChaos:
		durationStr = spec.Spec.Duration
	case *v1alpha1.HTTPChaos:
		durationStr = spec.Spec.Duration
	}

	if durationStr == nil {
		return time.Duration(0), fmt.Errorf("could not get duration for chaos object: %v", c.Object)
	}
	duration, err := time.ParseDuration(*durationStr)
	if err != nil {
		return time.Duration(0), fmt.Errorf("could not parse duration: %w", err)
	}
	return duration, nil
}

func (c *Chaos) GetChaosEvents() (*corev1.EventList, error) {
	listOpts := []client.ListOption{
		client.InNamespace(c.Object.GetNamespace()),
		client.MatchingFields{"involvedObject.name": c.Object.GetName(), "involvedObject.kind": c.GetChaosKind()},
	}
	events := &corev1.EventList{}
	if err := c.Client.List(context.Background(), events, listOpts...); err != nil {
		return nil, fmt.Errorf("could not list chaos events: %w", err)
	}

	return events, nil
}

func (c *Chaos) GetChaosKind() string {
	switch c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		return "NetworkChaos"
	case *v1alpha1.IOChaos:
		return "IOChaos"
	case *v1alpha1.StressChaos:
		return "StressChaos"
	case *v1alpha1.PodChaos:
		return "PodChaos"
	case *v1alpha1.HTTPChaos:
		return "HTTPChaos"
	default:
		panic(fmt.Sprintf("could not get chaos kind for object: %v", c.Object))
	}
}

func (c *Chaos) GetChaosStatus() (*v1alpha1.ChaosStatus, error) {
	switch obj := c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		return obj.GetStatus(), nil
	case *v1alpha1.IOChaos:
		return obj.GetStatus(), nil
	case *v1alpha1.StressChaos:
		return obj.GetStatus(), nil
	case *v1alpha1.PodChaos:
		return obj.GetStatus(), nil
	case *v1alpha1.HTTPChaos:
		return obj.GetStatus(), nil
	default:
		return nil, fmt.Errorf("could not get chaos status for %s", c.GetChaosKind())
	}
}

func (c *Chaos) GetExperimentStatus() (v1alpha1.ExperimentStatus, error) {
	switch obj := c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		return obj.Status.Experiment, nil
	case *v1alpha1.IOChaos:
		return obj.Status.Experiment, nil
	case *v1alpha1.StressChaos:
		return obj.Status.Experiment, nil
	case *v1alpha1.PodChaos:
		return obj.Status.Experiment, nil
	case *v1alpha1.HTTPChaos:
		return obj.Status.Experiment, nil
	default:
		return v1alpha1.ExperimentStatus{}, fmt.Errorf("could not experiment status for object: %v", c.Object)
	}
}

func ChaosObjectExists(object client.Object, c client.Client) (bool, error) {
	switch obj := object.(type) {
	case *v1alpha1.NetworkChaos, *v1alpha1.IOChaos, *v1alpha1.StressChaos, *v1alpha1.PodChaos, *v1alpha1.HTTPChaos, *v1alpha1.Schedule:
		err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				// If the error is NotFound, the object does not exist.
				return false, nil
			}
			// For any other errors, return the error.
			return false, err
		}
		// If there's no error, the object exists.
		return true, nil
	default:
		return false, fmt.Errorf("unsupported chaos object type: %T", obj)
	}
}

func (c *Chaos) updateChaosObject(ctx context.Context) error {
	switch obj := c.Object.(type) {
	case *v1alpha1.NetworkChaos:
		var objOut = &v1alpha1.NetworkChaos{}
		err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), objOut)
		if err != nil {
			return errors.Wrap(err, "could not get network chaos object")
		}
		c.Object = objOut
	case *v1alpha1.IOChaos:
		var objOut = &v1alpha1.IOChaos{}
		err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), objOut)
		if err != nil {
			return errors.Wrap(err, "could not get IO chaos object")
		}
		c.Object = objOut
	case *v1alpha1.StressChaos:
		var objOut = &v1alpha1.StressChaos{}
		err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), objOut)
		if err != nil {
			return errors.Wrap(err, "could not get stress chaos object")
		}
		c.Object = objOut
	case *v1alpha1.PodChaos:
		var objOut = &v1alpha1.PodChaos{}
		err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), objOut)
		if err != nil {
			return errors.Wrap(err, "could not get pod chaos object")
		}
		c.Object = objOut
	case *v1alpha1.HTTPChaos:
		var objOut = &v1alpha1.HTTPChaos{}
		err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), objOut)
		if err != nil {
			return errors.Wrap(err, "could not get HTTP chaos object")
		}
		c.Object = objOut
	case *v1alpha1.Schedule:
		var objOut = &v1alpha1.Schedule{}
		err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), objOut)
		if err != nil {
			return errors.Wrap(err, "could not get schedule object")
		}
		c.Object = objOut
	default:
		return fmt.Errorf("unsupported chaos object type: %T", obj)
	}

	return nil
}

func isConditionTrue(status *v1alpha1.ChaosStatus, expectedCondition v1alpha1.ChaosCondition) bool {
	if status == nil {
		return false
	}

	for _, condition := range status.Conditions {
		if condition.Type == expectedCondition.Type {
			return condition.Status == expectedCondition.Status
		}
	}
	return false
}

func (c *Chaos) AddListener(listener ChaosListener) {
	c.listeners = append(c.listeners, listener)
}

// GetStartTime returns the time when the chaos experiment started
func (c *Chaos) GetStartTime() time.Time {
	return c.startTime
}

// GetEndTime returns the time when the chaos experiment ended
func (c *Chaos) GetEndTime() time.Time {
	return c.endTime
}

// GetExpectedEndTime returns the time when the chaos experiment is expected to end
func (c *Chaos) GetExpectedEndTime() (time.Time, error) {
	duration, err := c.GetChaosDuration()
	if err != nil {
		return time.Time{}, err
	}
	return c.startTime.Add(duration), nil
}

type ChaosEventDetails struct {
	Event string
	Chaos *Chaos
	Error error
}

func (c *Chaos) notifyListeners(event string, err error) {
	for _, listener := range c.listeners {
		switch event {
		case "created":
			listener.OnChaosCreated(*c)
		case string(StatusCreationFailed):
			listener.OnChaosCreationFailed(*c, err)
		case "started":
			listener.OnChaosStarted(*c)
		case "paused":
			listener.OnChaosPaused(*c)
		case "resumed":
			listener.OnChaosStarted(*c) // Assuming "resumed" triggers "started"
		case "finished":
			listener.OnChaosEnded(*c)
		case "unknown":
			listener.OnChaosStatusUnknown(*c)
		}
	}
}

func (c *Chaos) monitorStatus(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := c.updateChaosObject(ctx)
			if err != nil {
				c.logger.Error().Err(err).Msg("failed to update chaos object")
				continue
			}
			chaosStatus, err := c.GetChaosStatus()
			if err != nil {
				c.logger.Error().Err(err).Msg("failed to get chaos status")
				continue
			}

			var currentStatus ChaosStatus

			allRecovered := v1alpha1.ChaosCondition{
				Type:   v1alpha1.ConditionAllRecovered,
				Status: corev1.ConditionTrue,
			}
			allInjected := v1alpha1.ChaosCondition{
				Type:   v1alpha1.ConditionAllInjected,
				Status: corev1.ConditionTrue,
			}
			selected := v1alpha1.ChaosCondition{
				Type:   v1alpha1.ConditionSelected,
				Status: corev1.ConditionTrue,
			}
			paused := v1alpha1.ChaosCondition{
				Type:   v1alpha1.ConditionPaused,
				Status: corev1.ConditionTrue,
			}

			if isConditionTrue(chaosStatus, selected) && isConditionTrue(chaosStatus, allInjected) {
				currentStatus = StatusRunning
			} else if isConditionTrue(chaosStatus, allRecovered) {
				currentStatus = StatusFinished
			} else if !isConditionTrue(chaosStatus, paused) && !isConditionTrue(chaosStatus, selected) {
				currentStatus = StatusUnknown
			}

			// If the status is unknown, always notify listeners
			if currentStatus == StatusUnknown {
				c.notifyListeners(string(StatusUnknown), nil)
				continue
			}

			// If the status has changed, update internal status and notify listeners
			if c.Status != currentStatus {
				c.Status = currentStatus

				switch c.Status {
				case StatusCreated:
					c.notifyListeners("created", nil)
				case StatusRunning:
					c.startTime = time.Now()
					c.notifyListeners("started", nil)
				case StatusPaused:
					c.notifyListeners("paused", nil)
				case StatusFinished:
					c.endTime = time.Now()
					c.notifyListeners("finished", nil)

					err := c.Delete(ctx)
					if err != nil {
						c.logger.Error().Err(err).Msg("failed to delete chaos object")
					}
				case StatusCreationFailed:
					panic("not implemented")
				case StatusDeleted:
					panic("not implemented")
				case StatusUnknown:
					panic("not implemented")
				}
			}
		}
	}
}

type NetworkChaosOpts struct {
	Name        string
	Description string
	DelayCreate time.Duration
	Delay       *v1alpha1.DelaySpec
	Loss        *v1alpha1.LossSpec
	NodeCount   int
	Duration    time.Duration
	Selector    v1alpha1.PodSelectorSpec
	K8sClient   client.Client
}

func (o *NetworkChaosOpts) Validate() error {
	if o.Delay != nil {
		latency, err := time.ParseDuration(o.Delay.Latency)
		if err != nil {
			return fmt.Errorf("invalid latency: %v", err)
		}
		if latency > 500*time.Millisecond {
			return fmt.Errorf("duration should be less than 500ms")
		}
	}
	if o.Loss != nil {
		lossInt, err := strconv.Atoi(o.Loss.Loss) // Convert the string to an integer
		if err != nil {
			return fmt.Errorf("invalid loss value: %s", err)
		}
		if lossInt > 100 {
			return fmt.Errorf("loss should be less than 100")
		}
	}
	if o.Loss == nil && o.Delay == nil {
		return fmt.Errorf("either delay or loss should be specified")
	}
	return nil

}

type PodChaosOpts struct {
	Name        string
	Description string
	DelayCreate time.Duration
	NodeCount   int
	Duration    time.Duration
	Spec        v1alpha1.PodChaosSpec
	K8sClient   client.Client
}

type StressChaosOpts struct {
	Name        string
	Description string
	DelayCreate time.Duration
	NodeCount   int
	Stressors   *v1alpha1.Stressors
	Duration    time.Duration
	Selector    v1alpha1.PodSelectorSpec
	K8sClient   client.Client
}

// NewChaosMeshClient initializes and returns a new Kubernetes client configured for Chaos Mesh
func NewChaosMeshClient() (client.Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load kubeconfig")
	}

	// Ensure the Chaos Mesh types are added to the scheme
	if err := v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return nil, errors.Wrap(err, "could not add the Chaos Mesh scheme")
	}

	// Create a new client for the Chaos Mesh API
	chaosClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a client for Chaos Mesh")
	}

	return chaosClient, nil
}
