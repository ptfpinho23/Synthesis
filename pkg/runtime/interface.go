package runtime

import (
	"context"
	"io"

	"github.com/synthesis/orchestrator/pkg/api"
)

// ContainerRuntime defines the interface for container operations
type ContainerRuntime interface {
	// Container lifecycle operations
	CreateContainer(ctx context.Context, spec *api.Container, podName string) (*ContainerInfo, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string, timeout int) error
	RemoveContainer(ctx context.Context, containerID string) error
	RestartContainer(ctx context.Context, containerID string) error

	// Container inspection
	InspectContainer(ctx context.Context, containerID string) (*ContainerInfo, error)
	ListContainers(ctx context.Context, filters ContainerFilter) ([]*ContainerInfo, error)
	GetContainerLogs(ctx context.Context, containerID string, opts LogOptions) (io.ReadCloser, error)

	// Container execution
	ExecContainer(ctx context.Context, containerID string, cmd []string) (*ExecResult, error)

	// Image operations
	PullImage(ctx context.Context, image string) error
	RemoveImage(ctx context.Context, image string) error
	ListImages(ctx context.Context) ([]*ImageInfo, error)

	// Network operations
	CreateNetwork(ctx context.Context, name string, opts NetworkOptions) (*NetworkInfo, error)
	RemoveNetwork(ctx context.Context, networkID string) error
	ConnectContainer(ctx context.Context, containerID, networkID string) error
	DisconnectContainer(ctx context.Context, containerID, networkID string) error

	// Stats and monitoring
	GetContainerStats(ctx context.Context, containerID string) (*ContainerStats, error)
	GetSystemInfo(ctx context.Context) (*SystemInfo, error)

	// Health check
	HealthCheck(ctx context.Context) error
}

// ContainerInfo represents information about a container
type ContainerInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Status  ContainerStatus   `json:"status"`
	State   ContainerState    `json:"state"`
	Created int64             `json:"created"`
	Started int64             `json:"started,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	Ports   []PortMapping     `json:"ports,omitempty"`
	Mounts  []MountPoint      `json:"mounts,omitempty"`
}

// ContainerStatus represents the status of a container
type ContainerStatus string

const (
	ContainerStatusCreated    ContainerStatus = "created"
	ContainerStatusRunning    ContainerStatus = "running"
	ContainerStatusPaused     ContainerStatus = "paused"
	ContainerStatusRestarting ContainerStatus = "restarting"
	ContainerStatusExited     ContainerStatus = "exited"
	ContainerStatusDead       ContainerStatus = "dead"
)

// ContainerState represents the detailed state of a container
type ContainerState struct {
	Status     ContainerStatus `json:"status"`
	Running    bool            `json:"running"`
	Paused     bool            `json:"paused"`
	Restarting bool            `json:"restarting"`
	Dead       bool            `json:"dead"`
	PID        int             `json:"pid,omitempty"`
	ExitCode   int             `json:"exitCode,omitempty"`
	Error      string          `json:"error,omitempty"`
	StartedAt  int64           `json:"startedAt,omitempty"`
	FinishedAt int64           `json:"finishedAt,omitempty"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	ContainerPort int32  `json:"containerPort"`
	HostPort      int32  `json:"hostPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIP,omitempty"`
}

// MountPoint represents a mount point
type MountPoint struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

// ContainerFilter represents filters for listing containers
type ContainerFilter struct {
	Labels map[string]string `json:"labels,omitempty"`
	Names  []string          `json:"names,omitempty"`
	Status []ContainerStatus `json:"status,omitempty"`
}

// LogOptions represents options for getting container logs
type LogOptions struct {
	Follow     bool   `json:"follow"`
	Stdout     bool   `json:"stdout"`
	Stderr     bool   `json:"stderr"`
	Since      string `json:"since,omitempty"`
	Until      string `json:"until,omitempty"`
	Timestamps bool   `json:"timestamps"`
	Tail       string `json:"tail,omitempty"`
}

// ExecResult represents the result of container execution
type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ImageInfo represents information about an image
type ImageInfo struct {
	ID       string            `json:"id"`
	RepoTags []string          `json:"repoTags"`
	Size     int64             `json:"size"`
	Created  int64             `json:"created"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// NetworkOptions represents options for creating a network
type NetworkOptions struct {
	Driver     string            `json:"driver"`
	Internal   bool              `json:"internal"`
	Attachable bool              `json:"attachable"`
	Options    map[string]string `json:"options,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// NetworkInfo represents information about a network
type NetworkInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Driver  string            `json:"driver"`
	Scope   string            `json:"scope"`
	Options map[string]string `json:"options,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// ContainerStats represents container resource usage statistics
type ContainerStats struct {
	ContainerID string       `json:"containerID"`
	Read        int64        `json:"read"`
	CPU         CPUStats     `json:"cpu"`
	Memory      MemoryStats  `json:"memory"`
	Network     NetworkStats `json:"network"`
	BlockIO     BlockIOStats `json:"blockIO"`
}

// CPUStats represents CPU usage statistics
type CPUStats struct {
	TotalUsage  uint64  `json:"totalUsage"`
	UsageInKern uint64  `json:"usageInKern"`
	UsageInUser uint64  `json:"usageInUser"`
	SystemUsage uint64  `json:"systemUsage"`
	PercentUsage float64 `json:"percentUsage"`
}

// MemoryStats represents memory usage statistics
type MemoryStats struct {
	Usage     uint64 `json:"usage"`
	Limit     uint64 `json:"limit"`
	Cache     uint64 `json:"cache"`
	RSS       uint64 `json:"rss"`
	Swap      uint64 `json:"swap"`
	Failcnt   uint64 `json:"failcnt"`
}

// NetworkStats represents network usage statistics
type NetworkStats struct {
	RxBytes   uint64 `json:"rxBytes"`
	RxPackets uint64 `json:"rxPackets"`
	RxErrors  uint64 `json:"rxErrors"`
	RxDropped uint64 `json:"rxDropped"`
	TxBytes   uint64 `json:"txBytes"`
	TxPackets uint64 `json:"txPackets"`
	TxErrors  uint64 `json:"txErrors"`
	TxDropped uint64 `json:"txDropped"`
}

// BlockIOStats represents block I/O statistics
type BlockIOStats struct {
	ReadBytes  uint64 `json:"readBytes"`
	WriteBytes uint64 `json:"writeBytes"`
	ReadOps    uint64 `json:"readOps"`
	WriteOps   uint64 `json:"writeOps"`
}

// SystemInfo represents system information
type SystemInfo struct {
	ContainerRuntime        string `json:"containerRuntime"`
	RuntimeVersion          string `json:"runtimeVersion"`
	KernelVersion           string `json:"kernelVersion"`
	OperatingSystem         string `json:"operatingSystem"`
	Architecture            string `json:"architecture"`
	NCPU                    int    `json:"ncpu"`
	MemTotal                int64  `json:"memTotal"`
	DockerRootDir           string `json:"dockerRootDir,omitempty"`
	HTTPProxy               string `json:"httpProxy,omitempty"`
	HTTPSProxy              string `json:"httpsProxy,omitempty"`
	NoProxy                 string `json:"noProxy,omitempty"`
}

// RuntimeConfig represents runtime configuration
type RuntimeConfig struct {
	// Socket path for runtime communication
	SocketPath string `json:"socketPath"`
	
	// API version to use
	APIVersion string `json:"apiVersion"`
	
	// Timeout for operations
	Timeout int `json:"timeout"`
	
	// Default network for containers
	DefaultNetwork string `json:"defaultNetwork"`
	
	// Container labels to apply by default
	DefaultLabels map[string]string `json:"defaultLabels"`
} 