// chaos-meshorg
package chaosmeshorg

import (
	_jsii_ "github.com/aws/jsii-runtime-go/runtime"
	_init_ "github.com/smartcontractkit/chainlink-testing-framework/env/imports/k8s/iochaos/chaosmeshorg/jsii"

	"github.com/aws/constructs-go/constructs/v10"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
	"github.com/smartcontractkit/chainlink-testing-framework/env/imports/k8s/iochaos/chaosmeshorg/internal"
)

// IOChaos is the Schema for the iochaos API.
type IoChaos interface {
	cdk8s.ApiObject
	// The group portion of the API version (e.g. `authorization.k8s.io`).
	ApiGroup() *string
	// The object's API version (e.g. `authorization.k8s.io/v1`).
	ApiVersion() *string
	// The chart in which this object is defined.
	Chart() cdk8s.Chart
	// The object kind.
	Kind() *string
	// Metadata associated with this API object.
	Metadata() cdk8s.ApiObjectMetadataDefinition
	// The name of the API object.
	//
	// If a name is specified in `metadata.name` this will be the name returned.
	// Otherwise, a name will be generated by calling
	// `Chart.of(this).generatedObjectName(this)`, which by default uses the
	// construct path to generate a DNS-compatible name for the resource.
	Name() *string
	// The tree node.
	Node() constructs.Node
	// Create a dependency between this ApiObject and other constructs.
	//
	// These can be other ApiObjects, Charts, or custom.
	AddDependency(dependencies ...constructs.IConstruct)
	// Applies a set of RFC-6902 JSON-Patch operations to the manifest synthesized for this API object.
	//
	// Example:
	//     kubePod.addJsonPatch(JsonPatch.replace('/spec/enableServiceLinks', true));
	//
	AddJsonPatch(ops ...cdk8s.JsonPatch)
	// Renders the object to Kubernetes JSON.
	ToJson() interface{}
	// Returns a string representation of this construct.
	ToString() *string
}

// The jsii proxy struct for IoChaos
type jsiiProxy_IoChaos struct {
	internal.Type__cdk8sApiObject
}

func (j *jsiiProxy_IoChaos) ApiGroup() *string {
	var returns *string
	_jsii_.Get(
		j,
		"apiGroup",
		&returns,
	)
	return returns
}

func (j *jsiiProxy_IoChaos) ApiVersion() *string {
	var returns *string
	_jsii_.Get(
		j,
		"apiVersion",
		&returns,
	)
	return returns
}

func (j *jsiiProxy_IoChaos) Chart() cdk8s.Chart {
	var returns cdk8s.Chart
	_jsii_.Get(
		j,
		"chart",
		&returns,
	)
	return returns
}

func (j *jsiiProxy_IoChaos) Kind() *string {
	var returns *string
	_jsii_.Get(
		j,
		"kind",
		&returns,
	)
	return returns
}

func (j *jsiiProxy_IoChaos) Metadata() cdk8s.ApiObjectMetadataDefinition {
	var returns cdk8s.ApiObjectMetadataDefinition
	_jsii_.Get(
		j,
		"metadata",
		&returns,
	)
	return returns
}

func (j *jsiiProxy_IoChaos) Name() *string {
	var returns *string
	_jsii_.Get(
		j,
		"name",
		&returns,
	)
	return returns
}

func (j *jsiiProxy_IoChaos) Node() constructs.Node {
	var returns constructs.Node
	_jsii_.Get(
		j,
		"node",
		&returns,
	)
	return returns
}

// Defines a "IOChaos" API object.
func NewIoChaos(scope constructs.Construct, id *string, props *IoChaosProps) IoChaos {
	_init_.Initialize()

	j := jsiiProxy_IoChaos{}

	_jsii_.Create(
		"chaos-meshorg.IoChaos",
		[]interface{}{scope, id, props},
		&j,
	)

	return &j
}

// Defines a "IOChaos" API object.
func NewIoChaos_Override(i IoChaos, scope constructs.Construct, id *string, props *IoChaosProps) {
	_init_.Initialize()

	_jsii_.Create(
		"chaos-meshorg.IoChaos",
		[]interface{}{scope, id, props},
		i,
	)
}

// Checks if `x` is a construct.
//
// Use this method instead of `instanceof` to properly detect `Construct`
// instances, even when the construct library is symlinked.
//
// Explanation: in JavaScript, multiple copies of the `constructs` library on
// disk are seen as independent, completely different libraries. As a
// consequence, the class `Construct` in each copy of the `constructs` library
// is seen as a different class, and an instance of one class will not test as
// `instanceof` the other class. `npm install` will not create installations
// like this, but users may manually symlink construct libraries together or
// use a monorepo tool: in those cases, multiple copies of the `constructs`
// library can be accidentally installed, and `instanceof` will behave
// unpredictably. It is safest to avoid using `instanceof`, and using
// this type-testing method instead.
//
// Returns: true if `x` is an object created from a class which extends `Construct`.
func IoChaos_IsConstruct(x interface{}) *bool {
	_init_.Initialize()

	var returns *bool

	_jsii_.StaticInvoke(
		"chaos-meshorg.IoChaos",
		"isConstruct",
		[]interface{}{x},
		&returns,
	)

	return returns
}

// Renders a Kubernetes manifest for "IOChaos".
//
// This can be used to inline resource manifests inside other objects (e.g. as templates).
func IoChaos_Manifest(props *IoChaosProps) interface{} {
	_init_.Initialize()

	var returns interface{}

	_jsii_.StaticInvoke(
		"chaos-meshorg.IoChaos",
		"manifest",
		[]interface{}{props},
		&returns,
	)

	return returns
}

// Returns the `ApiObject` named `Resource` which is a child of the given construct.
//
// If `c` is an `ApiObject`, it is returned directly. Throws an
// exception if the construct does not have a child named `Default` _or_ if
// this child is not an `ApiObject`.
func IoChaos_Of(c constructs.IConstruct) cdk8s.ApiObject {
	_init_.Initialize()

	var returns cdk8s.ApiObject

	_jsii_.StaticInvoke(
		"chaos-meshorg.IoChaos",
		"of",
		[]interface{}{c},
		&returns,
	)

	return returns
}

func IoChaos_GVK() *cdk8s.GroupVersionKind {
	_init_.Initialize()
	var returns *cdk8s.GroupVersionKind
	_jsii_.StaticGet(
		"chaos-meshorg.IoChaos",
		"GVK",
		&returns,
	)
	return returns
}

func (i *jsiiProxy_IoChaos) AddDependency(dependencies ...constructs.IConstruct) {
	args := []interface{}{}
	for _, a := range dependencies {
		args = append(args, a)
	}

	_jsii_.InvokeVoid(
		i,
		"addDependency",
		args,
	)
}

func (i *jsiiProxy_IoChaos) AddJsonPatch(ops ...cdk8s.JsonPatch) {
	args := []interface{}{}
	for _, a := range ops {
		args = append(args, a)
	}

	_jsii_.InvokeVoid(
		i,
		"addJsonPatch",
		args,
	)
}

func (i *jsiiProxy_IoChaos) ToJson() interface{} {
	var returns interface{}

	_jsii_.Invoke(
		i,
		"toJson",
		nil, // no parameters
		&returns,
	)

	return returns
}

func (i *jsiiProxy_IoChaos) ToString() *string {
	var returns *string

	_jsii_.Invoke(
		i,
		"toString",
		nil, // no parameters
		&returns,
	)

	return returns
}

// IOChaos is the Schema for the iochaos API.
type IoChaosProps struct {
	Metadata *cdk8s.ApiObjectMetadata `field:"optional" json:"metadata" yaml:"metadata"`
	// IOChaosSpec defines the desired state of IOChaos.
	Spec *IoChaosSpec `field:"optional" json:"spec" yaml:"spec"`
}

// IOChaosSpec defines the desired state of IOChaos.
type IoChaosSpec struct {
	// Action defines the specific pod chaos action.
	//
	// Supported action: latency / fault / attrOverride / mistake.
	Action IoChaosSpecAction `field:"required" json:"action" yaml:"action"`
	// Mode defines the mode to run chaos action.
	//
	// Supported mode: one / all / fixed / fixed-percent / random-max-percent.
	Mode IoChaosSpecMode `field:"required" json:"mode" yaml:"mode"`
	// Selector is used to select pods that are used to inject chaos action.
	Selector *IoChaosSpecSelector `field:"required" json:"selector" yaml:"selector"`
	// VolumePath represents the mount path of injected volume.
	VolumePath *string `field:"required" json:"volumePath" yaml:"volumePath"`
	// Attr defines the overrided attribution.
	Attr *IoChaosSpecAttr `field:"optional" json:"attr" yaml:"attr"`
	// ContainerNames indicates list of the name of affected container.
	//
	// If not set, all containers will be injected.
	ContainerNames *[]*string `field:"optional" json:"containerNames" yaml:"containerNames"`
	// Delay defines the value of I/O chaos action delay.
	//
	// A delay string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	Delay *string `field:"optional" json:"delay" yaml:"delay"`
	// Duration represents the duration of the chaos action.
	//
	// It is required when the action is `PodFailureAction`. A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	Duration *string `field:"optional" json:"duration" yaml:"duration"`
	// Errno defines the error code that returned by I/O action.
	//
	// refer to: https://www-numi.fnal.gov/offline_software/srt_public_context/WebDocs/Errors/unix_system_errors.html
	Errno *float64 `field:"optional" json:"errno" yaml:"errno"`
	// Methods defines the I/O methods for injecting I/O chaos action.
	//
	// default: all I/O methods.
	Methods *[]*string `field:"optional" json:"methods" yaml:"methods"`
	// Mistake defines what types of incorrectness are injected to IO operations.
	Mistake *IoChaosSpecMistake `field:"optional" json:"mistake" yaml:"mistake"`
	// Path defines the path of files for injecting I/O chaos action.
	Path *string `field:"optional" json:"path" yaml:"path"`
	// Percent defines the percentage of injection errors and provides a number from 0-100.
	//
	// default: 100.
	Percent *float64 `field:"optional" json:"percent" yaml:"percent"`
	// Value is required when the mode is set to `FixedPodMode` / `FixedPercentPodMod` / `RandomMaxPercentPodMod`.
	//
	// If `FixedPodMode`, provide an integer of pods to do chaos action. If `FixedPercentPodMod`, provide a number from 0-100 to specify the percent of pods the server can do chaos action. IF `RandomMaxPercentPodMod`,  provide a number from 0-100 to specify the max percent of pods to do chaos action
	Value *string `field:"optional" json:"value" yaml:"value"`
}

// Action defines the specific pod chaos action.
//
// Supported action: latency / fault / attrOverride / mistake.
type IoChaosSpecAction string

const (
	// latency.
	IoChaosSpecAction_LATENCY IoChaosSpecAction = "LATENCY"
	// fault.
	IoChaosSpecAction_FAULT IoChaosSpecAction = "FAULT"
	// attrOverride.
	IoChaosSpecAction_ATTR_OVERRIDE IoChaosSpecAction = "ATTR_OVERRIDE"
	// mistake.
	IoChaosSpecAction_MISTAKE IoChaosSpecAction = "MISTAKE"
)

// Attr defines the overrided attribution.
type IoChaosSpecAttr struct {
	// Timespec represents a time.
	Atime  *IoChaosSpecAttrAtime `field:"optional" json:"atime" yaml:"atime"`
	Blocks *float64              `field:"optional" json:"blocks" yaml:"blocks"`
	// Timespec represents a time.
	Ctime *IoChaosSpecAttrCtime `field:"optional" json:"ctime" yaml:"ctime"`
	Gid   *float64              `field:"optional" json:"gid" yaml:"gid"`
	Ino   *float64              `field:"optional" json:"ino" yaml:"ino"`
	// FileType represents type of a file.
	Kind *string `field:"optional" json:"kind" yaml:"kind"`
	// Timespec represents a time.
	Mtime *IoChaosSpecAttrMtime `field:"optional" json:"mtime" yaml:"mtime"`
	Nlink *float64              `field:"optional" json:"nlink" yaml:"nlink"`
	Perm  *float64              `field:"optional" json:"perm" yaml:"perm"`
	Rdev  *float64              `field:"optional" json:"rdev" yaml:"rdev"`
	Size  *float64              `field:"optional" json:"size" yaml:"size"`
	Uid   *float64              `field:"optional" json:"uid" yaml:"uid"`
}

// Timespec represents a time.
type IoChaosSpecAttrAtime struct {
	Nsec *float64 `field:"required" json:"nsec" yaml:"nsec"`
	Sec  *float64 `field:"required" json:"sec" yaml:"sec"`
}

// Timespec represents a time.
type IoChaosSpecAttrCtime struct {
	Nsec *float64 `field:"required" json:"nsec" yaml:"nsec"`
	Sec  *float64 `field:"required" json:"sec" yaml:"sec"`
}

// Timespec represents a time.
type IoChaosSpecAttrMtime struct {
	Nsec *float64 `field:"required" json:"nsec" yaml:"nsec"`
	Sec  *float64 `field:"required" json:"sec" yaml:"sec"`
}

// Mistake defines what types of incorrectness are injected to IO operations.
type IoChaosSpecMistake struct {
	// Filling determines what is filled in the miskate data.
	Filling IoChaosSpecMistakeFilling `field:"optional" json:"filling" yaml:"filling"`
	// Max length of each wrong data segment in bytes.
	MaxLength *float64 `field:"optional" json:"maxLength" yaml:"maxLength"`
	// There will be [1, MaxOccurrences] segments of wrong data.
	MaxOccurrences *float64 `field:"optional" json:"maxOccurrences" yaml:"maxOccurrences"`
}

// Filling determines what is filled in the miskate data.
type IoChaosSpecMistakeFilling string

const (
	// zero.
	IoChaosSpecMistakeFilling_ZERO IoChaosSpecMistakeFilling = "ZERO"
	// random.
	IoChaosSpecMistakeFilling_RANDOM IoChaosSpecMistakeFilling = "RANDOM"
)

// Mode defines the mode to run chaos action.
//
// Supported mode: one / all / fixed / fixed-percent / random-max-percent.
type IoChaosSpecMode string

const (
	// one.
	IoChaosSpecMode_ONE IoChaosSpecMode = "ONE"
	// all.
	IoChaosSpecMode_ALL IoChaosSpecMode = "ALL"
	// fixed.
	IoChaosSpecMode_FIXED IoChaosSpecMode = "FIXED"
	// fixed-percent.
	IoChaosSpecMode_FIXED_PERCENT IoChaosSpecMode = "FIXED_PERCENT"
	// random-max-percent.
	IoChaosSpecMode_RANDOM_MAX_PERCENT IoChaosSpecMode = "RANDOM_MAX_PERCENT"
)

// Selector is used to select pods that are used to inject chaos action.
type IoChaosSpecSelector struct {
	// Map of string keys and values that can be used to select objects.
	//
	// A selector based on annotations.
	AnnotationSelectors *map[string]*string `field:"optional" json:"annotationSelectors" yaml:"annotationSelectors"`
	// a slice of label selector expressions that can be used to select objects.
	//
	// A list of selectors based on set-based label expressions.
	ExpressionSelectors *[]*IoChaosSpecSelectorExpressionSelectors `field:"optional" json:"expressionSelectors" yaml:"expressionSelectors"`
	// Map of string keys and values that can be used to select objects.
	//
	// A selector based on fields.
	FieldSelectors *map[string]*string `field:"optional" json:"fieldSelectors" yaml:"fieldSelectors"`
	// Map of string keys and values that can be used to select objects.
	//
	// A selector based on labels.
	LabelSelectors *map[string]*string `field:"optional" json:"labelSelectors" yaml:"labelSelectors"`
	// Namespaces is a set of namespace to which objects belong.
	Namespaces *[]*string `field:"optional" json:"namespaces" yaml:"namespaces"`
	// Nodes is a set of node name and objects must belong to these nodes.
	Nodes *[]*string `field:"optional" json:"nodes" yaml:"nodes"`
	// Map of string keys and values that can be used to select nodes.
	//
	// Selector which must match a node's labels, and objects must belong to these selected nodes.
	NodeSelectors *map[string]*string `field:"optional" json:"nodeSelectors" yaml:"nodeSelectors"`
	// PodPhaseSelectors is a set of condition of a pod at the current time.
	//
	// supported value: Pending / Running / Succeeded / Failed / Unknown.
	PodPhaseSelectors *[]*string `field:"optional" json:"podPhaseSelectors" yaml:"podPhaseSelectors"`
	// Pods is a map of string keys and a set values that used to select pods.
	//
	// The key defines the namespace which pods belong, and the each values is a set of pod names.
	Pods *map[string]*[]*string `field:"optional" json:"pods" yaml:"pods"`
}

// A label selector requirement is a selector that contains values, a key, and an operator that relates the key and values.
type IoChaosSpecSelectorExpressionSelectors struct {
	// key is the label key that the selector applies to.
	Key *string `field:"required" json:"key" yaml:"key"`
	// operator represents a key's relationship to a set of values.
	//
	// Valid operators are In, NotIn, Exists and DoesNotExist.
	Operator *string `field:"required" json:"operator" yaml:"operator"`
	// values is an array of string values.
	//
	// If the operator is In or NotIn, the values array must be non-empty. If the operator is Exists or DoesNotExist, the values array must be empty. This array is replaced during a strategic merge patch.
	Values *[]*string `field:"optional" json:"values" yaml:"values"`
}
