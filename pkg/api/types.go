package api

import (
	"time"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Re-export Kubernetes types for full compatibility
type (
	Pod           = corev1.Pod
	PodSpec       = corev1.PodSpec
	PodStatus     = corev1.PodStatus
	PodCondition  = corev1.PodCondition
	Container     = corev1.Container
	ContainerPort = corev1.ContainerPort
	EnvVar        = corev1.EnvVar
	Volume        = corev1.Volume
	VolumeMount   = corev1.VolumeMount
	
	ResourceRequirements = corev1.ResourceRequirements
	ResourceList         = corev1.ResourceList
	ResourceName         = corev1.ResourceName
	
	Probe         = corev1.Probe
	HTTPGetAction = corev1.HTTPGetAction
	TCPSocketAction = corev1.TCPSocketAction
	ExecAction    = corev1.ExecAction
	
	Deployment           = appsv1.Deployment
	DeploymentSpec       = appsv1.DeploymentSpec
	DeploymentStatus     = appsv1.DeploymentStatus
	DeploymentCondition  = appsv1.DeploymentCondition
	StatefulSet          = appsv1.StatefulSet
	StatefulSetSpec      = appsv1.StatefulSetSpec
	StatefulSetStatus    = appsv1.StatefulSetStatus
	StatefulSetCondition = appsv1.StatefulSetCondition
	ReplicaSet           = appsv1.ReplicaSet
	ReplicaSetSpec       = appsv1.ReplicaSetSpec
	ReplicaSetStatus     = appsv1.ReplicaSetStatus
	
	PodTemplateSpec = corev1.PodTemplateSpec
	LabelSelector = metav1.LabelSelector
	
	Service         = corev1.Service
	ServiceSpec     = corev1.ServiceSpec
	ServiceStatus   = corev1.ServiceStatus
	ServicePort     = corev1.ServicePort
	ServiceType     = corev1.ServiceType
	
	Node           = corev1.Node
	NodeSpec       = corev1.NodeSpec
	NodeStatus     = corev1.NodeStatus
	NodeCondition  = corev1.NodeCondition
	NodeSystemInfo = corev1.NodeSystemInfo
	NodeAddress    = corev1.NodeAddress
	
	RestartPolicy     = corev1.RestartPolicy
	Protocol          = corev1.Protocol
	URIScheme         = corev1.URIScheme
	PodPhase          = corev1.PodPhase
	ContainerState    = corev1.ContainerState
	ContainerStatus   = corev1.ContainerStatus
	ConditionStatus   = corev1.ConditionStatus
	NodeConditionType = corev1.NodeConditionType
	NodeAddressType   = corev1.NodeAddressType
	PodConditionType  = corev1.PodConditionType
	
	LoadBalancerIngress = corev1.LoadBalancerIngress
	LoadBalancerStatus  = corev1.LoadBalancerStatus
)

// Re-export Kubernetes constants
const (
	ResourceCPU              = corev1.ResourceCPU
	ResourceMemory           = corev1.ResourceMemory
	ResourceStorage          = corev1.ResourceStorage
	ResourceEphemeralStorage = corev1.ResourceEphemeralStorage
	
	RestartPolicyAlways    = corev1.RestartPolicyAlways
	RestartPolicyOnFailure = corev1.RestartPolicyOnFailure
	RestartPolicyNever     = corev1.RestartPolicyNever
	
	ProtocolTCP  = corev1.ProtocolTCP
	ProtocolUDP  = corev1.ProtocolUDP
	ProtocolSCTP = corev1.ProtocolSCTP
	
	URISchemeHTTP  = corev1.URISchemeHTTP
	URISchemeHTTPS = corev1.URISchemeHTTPS
	
	PodPending   = corev1.PodPending
	PodRunning   = corev1.PodRunning
	PodSucceeded = corev1.PodSucceeded
	PodFailed    = corev1.PodFailed
	PodUnknown   = corev1.PodUnknown
	
	ServiceTypeClusterIP    = corev1.ServiceTypeClusterIP
	ServiceTypeNodePort     = corev1.ServiceTypeNodePort
	ServiceTypeLoadBalancer = corev1.ServiceTypeLoadBalancer
	ServiceTypeExternalName = corev1.ServiceTypeExternalName
	
	ConditionTrue    = corev1.ConditionTrue
	ConditionFalse   = corev1.ConditionFalse
	ConditionUnknown = corev1.ConditionUnknown
	
	NodeReady              = corev1.NodeReady
	NodeMemoryPressure     = corev1.NodeMemoryPressure
	NodeDiskPressure       = corev1.NodeDiskPressure
	NodePIDPressure        = corev1.NodePIDPressure
	NodeNetworkUnavailable = corev1.NodeNetworkUnavailable
	
	NodeHostName    = corev1.NodeHostName
	NodeExternalIP  = corev1.NodeExternalIP
	NodeInternalIP  = corev1.NodeInternalIP
	NodeExternalDNS = corev1.NodeExternalDNS
	NodeInternalDNS = corev1.NodeInternalDNS
	
	PodScheduled       = corev1.PodScheduled
	PodReady           = corev1.PodReady
	PodInitialized     = corev1.PodInitialized
	ContainersReady    = corev1.ContainersReady
	DisruptionTarget   = corev1.DisruptionTarget
)

// Synthesis-specific types

type SynthesisMetadata struct {
	OriginalKind string `json:"originalKind,omitempty"`
	Managed   bool      `json:"managed,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// Workload interface for synthesis-specific operations
type Workload interface {
	GetName() string
	GetNamespace() string
	GetKind() string
}

type WorkloadConditionType string

const (
	WorkloadProgressing WorkloadConditionType = "Progressing"
	WorkloadAvailable   WorkloadConditionType = "Available"
)

type WorkloadCondition struct {
	Type WorkloadConditionType `json:"type"`
	Status ConditionStatus `json:"status"`
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	Reason string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type WorkloadStatus struct {
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	Conditions []WorkloadCondition `json:"conditions,omitempty"`
}

type WorkloadSpec struct {
	Replicas int32 `json:"replicas,omitempty"`
	Selector map[string]string `json:"selector,omitempty"`
	Template PodTemplateSpec `json:"template"`
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`
}

type SynthesisWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec WorkloadSpec `json:"spec,omitempty"`
	Status WorkloadStatus `json:"status,omitempty"`
}

type HealthCheck struct {
	HTTPGet *HTTPGetAction `json:"httpGet,omitempty"`
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
} 