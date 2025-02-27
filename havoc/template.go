package havoc

import (
	"context"
	"fmt"
	"time"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func defaultListeners(l zerolog.Logger) []ChaosListener {
	return []ChaosListener{
		NewChaosLogger(l),
	}
}

type NamespaceScopedChaosRunner struct {
	l      zerolog.Logger
	c      client.Client
	remove bool
}

// NewNamespaceRunner creates a new namespace-scoped chaos runner
func NewNamespaceRunner(l zerolog.Logger, c client.Client, remove bool) *NamespaceScopedChaosRunner {
	return &NamespaceScopedChaosRunner{
		l:      l,
		c:      c,
		remove: remove,
	}
}

type PodPartitionCfg struct {
	Namespace             string
	Description           string
	LabelFromKey          string
	LabelFromValues       []string
	LabelToKey            string
	LabelToValues         []string
	InjectionDuration     time.Duration
	ExperimentCreateDelay time.Duration
}

// RunPodPartition initiates a network partition chaos experiment on specified pods.
// It configures the experiment based on the provided PodPartitionCfg and executes it.
// This function is useful for testing the resilience of applications under network partition scenarios.
func (cr *NamespaceScopedChaosRunner) RunPodPartition(ctx context.Context, cfg PodPartitionCfg) (*Chaos, error) {
	experiment, err := NewChaos(ChaosOpts{
		Object: &v1alpha1.NetworkChaos{
			TypeMeta: metav1.TypeMeta{
				Kind:       string(v1alpha1.TypeNetworkChaos),
				APIVersion: "chaos-mesh.org/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("partition-%s", uuid.NewString()[0:5]),
				Namespace: cfg.Namespace,
			},
			Spec: v1alpha1.NetworkChaosSpec{
				Action:   v1alpha1.PartitionAction,
				Duration: ptr.To[string]((cfg.InjectionDuration).String()),
				PodSelector: v1alpha1.PodSelector{
					Mode: v1alpha1.AllMode,
					Selector: v1alpha1.PodSelectorSpec{
						GenericSelectorSpec: v1alpha1.GenericSelectorSpec{
							Namespaces: []string{cfg.Namespace},
							ExpressionSelectors: v1alpha1.LabelSelectorRequirements{
								{
									Operator: "In",
									Key:      cfg.LabelFromKey,
									Values:   cfg.LabelFromValues,
								},
							},
						},
					},
				},
				Target: &v1alpha1.PodSelector{
					Mode: v1alpha1.AllMode,
					Selector: v1alpha1.PodSelectorSpec{
						GenericSelectorSpec: v1alpha1.GenericSelectorSpec{
							Namespaces: []string{cfg.Namespace},
							ExpressionSelectors: v1alpha1.LabelSelectorRequirements{
								{
									Operator: "In",
									Key:      cfg.LabelToKey,
									Values:   cfg.LabelToValues,
								},
							},
						},
					},
				},
			},
		},
		Listeners: defaultListeners(cr.l),
		Logger:    &cr.l,
		Client:    cr.c,
		Remove:    cr.remove,
	})
	if err != nil {
		return nil, err
	}
	experiment.Create(ctx)
	return experiment, nil
}

type PodDelayCfg struct {
	Namespace             string
	Description           string
	Latency               time.Duration
	Jitter                time.Duration
	Correlation           string
	LabelKey              string
	LabelValues           []string
	InjectionDuration     time.Duration
	ExperimentCreateDelay time.Duration
}

// RunPodDelay initiates a network delay chaos experiment on specified pods.
// It configures the delay parameters and applies them to the targeted namespace.
// This function is useful for testing the resilience of applications under network latency conditions.
func (cr *NamespaceScopedChaosRunner) RunPodDelay(ctx context.Context, cfg PodDelayCfg) (*Chaos, error) {
	experiment, err := NewChaos(ChaosOpts{
		Object: &v1alpha1.NetworkChaos{
			TypeMeta: metav1.TypeMeta{
				Kind:       string(v1alpha1.TypeNetworkChaos),
				APIVersion: "chaos-mesh.org/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("delay-%s", uuid.NewString()[0:5]),
				Namespace: cfg.Namespace,
			},
			Spec: v1alpha1.NetworkChaosSpec{
				Action:   v1alpha1.DelayAction,
				Duration: ptr.To[string]((cfg.InjectionDuration).String()),
				TcParameter: v1alpha1.TcParameter{
					Delay: &v1alpha1.DelaySpec{
						Latency:     cfg.Latency.String(),
						Correlation: cfg.Correlation,
						Jitter:      cfg.Jitter.String(),
					},
				},
				PodSelector: v1alpha1.PodSelector{
					Mode: v1alpha1.AllMode,
					Selector: v1alpha1.PodSelectorSpec{
						GenericSelectorSpec: v1alpha1.GenericSelectorSpec{
							Namespaces: []string{cfg.Namespace},
							ExpressionSelectors: v1alpha1.LabelSelectorRequirements{
								{
									Operator: "In",
									Key:      cfg.LabelKey,
									Values:   cfg.LabelValues,
								},
							},
						},
					},
				},
			},
		},
		Listeners: defaultListeners(cr.l),
		Logger:    &cr.l,
		Client:    cr.c,
		Remove:    cr.remove,
	})
	if err != nil {
		return nil, err
	}
	experiment.Create(ctx)
	return experiment, nil
}

type PodFailCfg struct {
	Namespace             string
	Description           string
	LabelKey              string
	LabelValues           []string
	InjectionDuration     time.Duration
	ExperimentCreateDelay time.Duration
}

// RunPodFail initiates a pod failure experiment based on the provided configuration.
// It creates a Chaos object that simulates pod failures for a specified duration,
// allowing users to test the resilience of their applications under failure conditions.
func (cr *NamespaceScopedChaosRunner) RunPodFail(ctx context.Context, cfg PodFailCfg) (*Chaos, error) {
	experiment, err := NewChaos(ChaosOpts{
		Description: cfg.Description,
		DelayCreate: cfg.ExperimentCreateDelay,
		Object: &v1alpha1.PodChaos{
			TypeMeta: metav1.TypeMeta{
				Kind:       string(v1alpha1.TypePodChaos),
				APIVersion: "chaos-mesh.org/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("fail-%s", uuid.NewString()[0:5]),
				Namespace: cfg.Namespace,
			},
			Spec: v1alpha1.PodChaosSpec{
				Action:   v1alpha1.PodFailureAction,
				Duration: ptr.To[string](cfg.InjectionDuration.String()),
				ContainerSelector: v1alpha1.ContainerSelector{
					PodSelector: v1alpha1.PodSelector{
						Mode: v1alpha1.AllMode,
						Selector: v1alpha1.PodSelectorSpec{
							GenericSelectorSpec: v1alpha1.GenericSelectorSpec{
								Namespaces: []string{cfg.Namespace},
								ExpressionSelectors: v1alpha1.LabelSelectorRequirements{
									{
										Operator: "In",
										Key:      cfg.LabelKey,
										Values:   cfg.LabelValues,
									},
								},
							},
						},
					},
				},
			},
		},
		Listeners: defaultListeners(cr.l),
		Logger:    &cr.l,
		Client:    cr.c,
		Remove:    cr.remove,
	})
	if err != nil {
		return nil, err
	}
	experiment.Create(ctx)
	return experiment, nil
}

type NodeCPUStressConfig struct {
	Namespace               string
	Description             string
	Cores                   int
	CoreLoadPercentage      int // 0-100
	LabelKey                string
	LabelValues             []string
	InjectionDuration       time.Duration
	ExperimentTotalDuration time.Duration
	ExperimentCreateDelay   time.Duration
}

// RunPodStressCPU initiates a CPU stress test on specified pods within a namespace.
// It creates a scheduled chaos experiment that applies CPU load based on the provided configuration.
// This function is useful for testing the resilience of applications under CPU stress conditions.
func (cr *NamespaceScopedChaosRunner) RunPodStressCPU(ctx context.Context, cfg NodeCPUStressConfig) (*Chaos, error) {
	experiment, err := NewChaos(ChaosOpts{
		Description: cfg.Description,
		DelayCreate: cfg.ExperimentCreateDelay,
		Object: &v1alpha1.Schedule{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Schedule",
				APIVersion: "chaos-mesh.org/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "stress",
				Namespace: cfg.Namespace,
			},
			Spec: v1alpha1.ScheduleSpec{
				Schedule:          "@every 1m",
				ConcurrencyPolicy: v1alpha1.ForbidConcurrent,
				Type:              v1alpha1.ScheduleTypeStressChaos,
				HistoryLimit:      2,
				ScheduleItem: v1alpha1.ScheduleItem{
					EmbedChaos: v1alpha1.EmbedChaos{
						StressChaos: &v1alpha1.StressChaosSpec{
							ContainerSelector: v1alpha1.ContainerSelector{
								PodSelector: v1alpha1.PodSelector{
									Mode: v1alpha1.AllMode,
									Selector: v1alpha1.PodSelectorSpec{
										GenericSelectorSpec: v1alpha1.GenericSelectorSpec{
											Namespaces: []string{cfg.Namespace},
											ExpressionSelectors: v1alpha1.LabelSelectorRequirements{
												{
													Operator: "In",
													Key:      cfg.LabelKey,
													Values:   cfg.LabelValues,
												},
											},
										},
									},
								},
							},
							Stressors: &v1alpha1.Stressors{
								CPUStressor: &v1alpha1.CPUStressor{
									Stressor: v1alpha1.Stressor{
										Workers: cfg.Cores,
									},
									Load: ptr.To[int](cfg.CoreLoadPercentage),
								},
							},
							Duration: ptr.To[string](cfg.InjectionDuration.String()),
						},
					},
				},
			},
		},
		Listeners: defaultListeners(cr.l),
		Logger:    &cr.l,
		Client:    cr.c,
		Remove:    cr.remove,
	})
	if err != nil {
		return nil, err
	}
	experiment.Create(ctx)
	return experiment, nil
}
