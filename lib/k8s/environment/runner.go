package environment

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
	"github.com/rs/zerolog/log"

	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/config"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/imports/k8s"
	a "github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/pkg/alias"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/ptr"
)

const REMOTE_RUNNER_NAME = "remote-test-runner"

type Chart struct {
	Props *Props
}

func (m Chart) IsDeploymentNeeded() bool {
	return true
}

func (m Chart) GetName() string {
	return m.Props.BaseName
}

func (m Chart) GetProps() interface{} {
	return m.Props
}

func (m Chart) GetPath() string {
	return ""
}

func (m Chart) GetVersion() string {
	return ""
}

func (m Chart) GetValues() *map[string]interface{} {
	return nil
}

func (m Chart) ExportData(e *Environment) error {
	return nil
}

func (m Chart) GetLabels() map[string]string {
	if m.GetProps() == nil {
		return nil
	}

	if props, ok := m.GetProps().(*Props); ok {
		if props == nil {
			return nil
		}
		labels := make(map[string]string)
		for k, v := range *props.Labels {
			if v == nil {
				continue
			}
			labels[k] = *v
		}
	}

	return nil
}

func NewRunner(props *Props) func(root cdk8s.Chart) ConnectedChart {
	return func(root cdk8s.Chart) ConnectedChart {
		labels := *props.Labels
		labels["chain.link/component"] = ptr.Ptr("test-runner")
		c := &Chart{
			Props: props,
		}
		// if reportPath is given we create a persistent volume
		// so that data generated by remote-runner can be accessed from other pods
		// accessing the same pvc volume
		if props.ReportPath != "" {
			pvcVolume(root, props)
		}
		kubeSecret(root, props)
		role(root, props)
		job(root, props)
		return c
	}
}

// DataFromRunner - we create this pod to share same persistent volume as remote-runner-node container. This container
// keeps on running and stays alive after the remote-runner-node gets completed, so that
// the calling test can access all files generated by remote runner.
func DataFromRunner(props *Props) func(root cdk8s.Chart) ConnectedChart {
	labels := *props.Labels
	labels["app"] = ptr.Ptr("runner-data")
	labels["chain.link/component"] = ptr.Ptr("test-runner")
	return func(root cdk8s.Chart) ConnectedChart {
		c := &Chart{
			Props: props,
		}
		k8s.NewKubeDeployment(
			root,
			ptr.Ptr(fmt.Sprintf("%s-data", props.BaseName)),
			&k8s.KubeDeploymentProps{
				Metadata: &k8s.ObjectMeta{
					Name:   ptr.Ptr(fmt.Sprintf("%s-data", props.BaseName)),
					Labels: &labels,
				},
				Spec: &k8s.DeploymentSpec{
					Selector: &k8s.LabelSelector{
						MatchLabels: &labels,
					},
					Template: &k8s.PodTemplateSpec{
						Metadata: &k8s.ObjectMeta{
							Labels: &labels,
						},
						Spec: &k8s.PodSpec{
							ServiceAccountName: ptr.Ptr("default"),
							// try to schedule the pod on same node as remote runner job
							Affinity: &k8s.Affinity{
								PodAffinity: &k8s.PodAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &[]*k8s.PodAffinityTerm{
										{
											LabelSelector: &k8s.LabelSelector{
												MatchLabels: &map[string]*string{
													"job-name": ptr.Ptr("remote-test-runner"),
												},
											},
											TopologyKey: ptr.Ptr("kubernetes.io/hostname"),
										},
									},
								},
							},
							Volumes: ptr.Ptr([]*k8s.Volume{
								{
									Name: ptr.Ptr("reports"),
									PersistentVolumeClaim: ptr.Ptr(k8s.PersistentVolumeClaimVolumeSource{
										ClaimName: ptr.Ptr(fmt.Sprintf("%s-data-pvc", props.BaseName)),
									}),
								},
							}),
							Containers: &[]*k8s.Container{
								{
									Name:            ptr.Ptr(fmt.Sprintf("%s-data-files", props.BaseName)),
									Image:           ptr.Ptr("busybox:stable"),
									ImagePullPolicy: ptr.Ptr("Always"),
									Command:         ptr.Ptr([]*string{ptr.Ptr("/bin/sh"), ptr.Ptr("-ec"), ptr.Ptr("while :; do echo '.'; sleep 5 ; done")}),
									Ports: ptr.Ptr([]*k8s.ContainerPort{
										{
											ContainerPort: ptr.Ptr(float64(80)),
										},
									}),
									VolumeMounts: &[]*k8s.VolumeMount{
										{
											Name:      ptr.Ptr("reports"),
											MountPath: ptr.Ptr("reports"),
										},
									},
								},
							},
						},
					},
				},
			})
		return c
	}
}

type Props struct {
	BaseName           string
	ReportPath         string
	TargetNamespace    string
	Labels             *map[string]*string
	Image              string
	TestName           string
	SkipManifestUpdate bool
	PreventPodEviction bool
}

func role(chart cdk8s.Chart, props *Props) {
	k8s.NewKubeRole(
		chart,
		ptr.Ptr(fmt.Sprintf("%s-role", props.BaseName)),
		&k8s.KubeRoleProps{
			Metadata: &k8s.ObjectMeta{
				Name:   ptr.Ptr(props.BaseName),
				Labels: props.Labels,
			},
			Rules: &[]*k8s.PolicyRule{
				{
					ApiGroups: &[]*string{
						ptr.Ptr(""), // this empty line is needed or k8s get really angry
						ptr.Ptr("apps"),
						ptr.Ptr("batch"),
						ptr.Ptr("core"),
						ptr.Ptr("networking.k8s.io"),
						ptr.Ptr("storage.k8s.io"),
						ptr.Ptr("policy"),
						ptr.Ptr("chaos-mesh.org"),
						ptr.Ptr("monitoring.coreos.com"),
						ptr.Ptr("rbac.authorization.k8s.io"),
					},
					Resources: &[]*string{
						ptr.Ptr("*"),
					},
					Verbs: &[]*string{
						ptr.Ptr("*"),
					},
				},
			},
		})
	k8s.NewKubeRoleBinding(
		chart,
		ptr.Ptr(fmt.Sprintf("%s-role-binding", props.BaseName)),
		&k8s.KubeRoleBindingProps{
			RoleRef: &k8s.RoleRef{
				ApiGroup: ptr.Ptr("rbac.authorization.k8s.io"),
				Kind:     ptr.Ptr("Role"),
				Name:     ptr.Ptr("remote-test-runner"),
			},
			Metadata: &k8s.ObjectMeta{
				Labels: props.Labels,
			},
			Subjects: &[]*k8s.Subject{
				{
					Kind:      ptr.Ptr("ServiceAccount"),
					Name:      ptr.Ptr("default"),
					Namespace: ptr.Ptr(props.TargetNamespace),
				},
			},
		},
	)
}

func kubeSecret(chart cdk8s.Chart, props *Props) {
	k8s.NewKubeSecret(
		chart,
		ptr.Ptr("ts-secret"),
		&k8s.KubeSecretProps{
			Metadata: &k8s.ObjectMeta{
				Name:   ptr.Ptr("ts-secret"),
				Labels: props.Labels,
			},
			Type: ptr.Ptr("Opaque"), // Typical for storing arbitrary user-defined data
			StringData: &map[string]*string{
				".testsecrets": ptr.Ptr(createTestSecretsDotenvFromEnvVars()),
			},
		},
	)
}

func job(chart cdk8s.Chart, props *Props) {
	defaultRunnerPodAnnotations := markNotSafeToEvict(props.PreventPodEviction, nil)
	restartPolicy := "Never"
	backOffLimit := float64(0)
	if os.Getenv(config.EnvVarDetachRunner) == "true" { // If we're running detached, we're likely running a long-form test
		restartPolicy = "OnFailure"
		backOffLimit = 100000 // effectively infinite (I hope)
	}
	volumes := []*k8s.Volume{
		{
			Name:     ptr.Ptr("persistence"),
			EmptyDir: &k8s.EmptyDirVolumeSource{},
		},
	}
	// if reportPath is given we create a volume attached to PVC volume claim
	if props.ReportPath != "" {
		volumes = append(volumes, &k8s.Volume{
			Name: ptr.Ptr("reports"),
			PersistentVolumeClaim: ptr.Ptr(k8s.PersistentVolumeClaimVolumeSource{
				ClaimName: ptr.Ptr(fmt.Sprintf("%s-data-pvc", props.BaseName)),
			}),
		})
	}
	volumes = append(volumes, &k8s.Volume{
		Name: ptr.Ptr("ts-volume"),
		Secret: &k8s.SecretVolumeSource{
			SecretName: ptr.Ptr("ts-secret"),
		},
	})
	k8s.NewKubeJob(
		chart,
		ptr.Ptr(fmt.Sprintf("%s-job", props.BaseName)),
		&k8s.KubeJobProps{
			Metadata: &k8s.ObjectMeta{
				Name:   ptr.Ptr(props.BaseName),
				Labels: props.Labels,
			},
			Spec: &k8s.JobSpec{
				Template: &k8s.PodTemplateSpec{
					Metadata: &k8s.ObjectMeta{
						Labels:      props.Labels,
						Annotations: a.ConvertAnnotations(defaultRunnerPodAnnotations),
					},
					Spec: &k8s.PodSpec{
						ServiceAccountName: ptr.Ptr("default"),
						Containers:         container(props),
						RestartPolicy:      ptr.Ptr(restartPolicy),
						Volumes:            &volumes,
					},
				},
				ActiveDeadlineSeconds: nil,
				BackoffLimit:          ptr.Ptr(backOffLimit),
			},
		})
}

func container(props *Props) *[]*k8s.Container {
	cpu := os.Getenv(config.EnvVarRemoteRunnerCpu)
	if cpu == "" {
		cpu = "1000m"
	}
	mem := os.Getenv(config.EnvVarRemoteRunnerMem)
	if mem == "" {
		mem = "1024Mi"
	}
	volumeMounts := []*k8s.VolumeMount{
		{
			Name:      ptr.Ptr("persistence"),
			MountPath: ptr.Ptr("/persistence"),
		},
	}
	// if reportPath is given we create a volume mount attached to PVC volume claim
	if props.ReportPath != "" {
		volumeMounts = append(volumeMounts, &k8s.VolumeMount{
			Name:      ptr.Ptr("reports"),
			MountPath: ptr.Ptr(fmt.Sprintf("/go/testdir/integration-tests/%s", props.ReportPath)),
			SubPath:   ptr.Ptr(props.ReportPath),
		})
	}
	// Mount test secrets dotenv file
	volumeMounts = append(volumeMounts, &k8s.VolumeMount{
		Name:      ptr.Ptr("ts-volume"),
		MountPath: ptr.Ptr("/etc/e2etests"),
	})
	return ptr.Ptr([]*k8s.Container{
		{
			Name:            ptr.Ptr(fmt.Sprintf("%s-node", props.BaseName)),
			Image:           ptr.Ptr(props.Image),
			ImagePullPolicy: ptr.Ptr("Always"),
			Env:             jobEnvVars(props),
			Resources:       a.ContainerResources(cpu, mem, cpu, mem),
			VolumeMounts:    &volumeMounts,
		},
	})
}

func pvcVolume(chart cdk8s.Chart, props *Props) {
	labels := make(map[string]*string)
	for k, v := range *props.Labels {
		labels[k] = v
	}
	labels["type"] = ptr.Ptr("local")
	k8s.NewKubePersistentVolume(
		chart,
		ptr.Ptr(fmt.Sprintf("%s-data-pv-volume", props.BaseName)),
		&k8s.KubePersistentVolumeProps{
			Metadata: &k8s.ObjectMeta{
				Name:   ptr.Ptr(fmt.Sprintf("%s-data-pv-volume", props.BaseName)),
				Labels: &labels,
			},
			Spec: &k8s.PersistentVolumeSpec{
				AccessModes: ptr.Ptr([]*string{ptr.Ptr("ReadWriteOnce")}),
				Capacity: &map[string]k8s.Quantity{
					"storage": k8s.Quantity_FromString(ptr.Ptr("256Mi")),
				},
				HostPath: &k8s.HostPathVolumeSource{
					Path: ptr.Ptr("/mnt/data"),
				},
				PersistentVolumeReclaimPolicy: ptr.Ptr("Delete"),
			},
		},
	)
	k8s.NewKubePersistentVolumeClaim(
		chart,
		ptr.Ptr(fmt.Sprintf("%s-data-pvc", props.BaseName)),
		&k8s.KubePersistentVolumeClaimProps{
			Metadata: &k8s.ObjectMeta{
				Name:   ptr.Ptr(fmt.Sprintf("%s-data-pvc", props.BaseName)),
				Labels: props.Labels,
			},
			Spec: &k8s.PersistentVolumeClaimSpec{
				AccessModes: ptr.Ptr([]*string{ptr.Ptr("ReadWriteOnce")}),
				VolumeMode:  ptr.Ptr("Filesystem"),
				Resources: &k8s.ResourceRequirements{
					Requests: &map[string]k8s.Quantity{
						"storage": k8s.Quantity_FromString(ptr.Ptr("256Mi")),
					},
				},
			},
		})
}

func createTestSecretsDotenvFromEnvVars() string {
	var buffer bytes.Buffer

	for _, pair := range os.Environ() {
		split := strings.SplitN(pair, "=", 2) // Split the pair into key and value
		if len(split) != 2 {
			continue // Skip any invalid entries
		}
		key, value := split[0], split[1]
		if strings.HasPrefix(key, config.E2ETestEnvVarPrefix) {
			buffer.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}
	}

	return buffer.String()
}

func jobEnvVars(props *Props) *[]*k8s.EnvVar {
	// Use a map to set values so we can easily overwrite duplicate values
	env := make(map[string]string)

	// Propagate common environment variables to the runner
	lookups := []string{
		config.EnvVarCLImage,
		config.EnvVarCLTag,
		config.EnvVarCLCommitSha,
		config.EnvVarLogLevel,
		config.EnvVarTestTrigger,
		config.EnvVarToleration,
		config.EnvVarSlackChannel,
		config.EnvVarSlackKey,
		config.EnvVarSlackUser,
		config.EnvVarUser,
		config.EnvVarTeam,
		config.EnvVarNodeSelector,
		config.EnvVarDBURL,
		config.EnvVarInternalDockerRepo,
		config.EnvVarLocalCharts,
		config.EnvBase64ConfigOverride,
		config.EnvSethLogLevel,
	}
	for _, k := range lookups {
		v, success := os.LookupEnv(k)
		if success && len(v) > 0 {
			log.Debug().Str(k, v).Msg("Forwarding Env Var")
			env[k] = v
		}
	}

	// Propagate prefixed variables to the runner
	// These should overwrite anything that was unprefixed if they match up
	for _, e := range os.Environ() {
		if i := strings.Index(e, "="); i >= 0 {
			if strings.HasPrefix(e[:i], config.EnvVarPrefix) {
				withoutPrefix := strings.Replace(e[:i], config.EnvVarPrefix, "", 1)
				log.Debug().Str(e[:i], e[i+1:]).Msg("Forwarding generic Env Var")
				env[withoutPrefix] = e[i+1:]
			}
		}
	}

	// Add variables that should need specific values for the remote runner
	env[config.EnvVarNamespace] = props.TargetNamespace
	env["TEST_NAME"] = props.TestName
	env[config.EnvVarInsideK8s] = "true"
	env[config.EnvVarSkipManifestUpdate] = strconv.FormatBool(props.SkipManifestUpdate)

	// convert from map to the expected array
	cdk8sVars := make([]*k8s.EnvVar, 0)
	for k, v := range env {
		cdk8sVars = append(cdk8sVars, a.EnvVarStr(k, v))
	}
	return &cdk8sVars
}
