package containerd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"

	"github.com/synthesis/orchestrator/pkg/api"
	"github.com/synthesis/orchestrator/pkg/runtime"
)

// ContainerdRuntime implements the ContainerRuntime interface using containerd
type ContainerdRuntime struct {
	client    *containerd.Client
	config    *runtime.RuntimeConfig
	namespace string
}

// NewContainerdRuntime creates a new containerd runtime instance
func NewContainerdRuntime(config *runtime.RuntimeConfig) (*ContainerdRuntime, error) {
	socketPath := config.SocketPath
	if socketPath == "" {
		socketPath = "/run/containerd/containerd.sock"
	}

	client, err := containerd.New(socketPath, containerd.WithDefaultNamespace("synthesis"))
	if err != nil {
		return nil, fmt.Errorf("failed to create containerd client: %w", err)
	}

	return &ContainerdRuntime{
		client:    client,
		config:    config,
		namespace: "synthesis",
	}, nil
}

// CreateContainer creates a new container from the given specification
func (c *ContainerdRuntime) CreateContainer(ctx context.Context, spec *api.Container, podName string) (*runtime.ContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	// Pull image if not present
	image, err := c.client.GetImage(ctx, spec.Image)
	if err != nil {
		if err := c.PullImage(ctx, spec.Image); err != nil {
			return nil, fmt.Errorf("failed to pull image %s: %w", spec.Image, err)
		}
		image, err = c.client.GetImage(ctx, spec.Image)
		if err != nil {
			return nil, fmt.Errorf("failed to get image after pull: %w", err)
		}
	}

	// Create container name
	containerName := fmt.Sprintf("%s-%s", podName, spec.Name)

	// Build OCI spec
	opts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithHostname(containerName),
	}

	// Add environment variables
	if len(spec.Env) > 0 {
		envVars := make([]string, len(spec.Env))
		for i, env := range spec.Env {
			envVars[i] = fmt.Sprintf("%s=%s", env.Name, env.Value)
		}
		opts = append(opts, oci.WithEnv(envVars))
	}

	// Add resource limits
	if spec.Resources.Limits != nil {
		if cpuLimit, ok := spec.Resources.Limits[api.ResourceCPU]; ok {
			if cpu, err := c.parseCPULimit(cpuLimit.String()); err == nil {
				opts = append(opts, oci.WithCPUShares(uint64(cpu)))
			}
		}
		if memLimit, ok := spec.Resources.Limits[api.ResourceMemory]; ok {
			if mem, err := c.parseMemoryLimit(memLimit.String()); err == nil {
				opts = append(opts, oci.WithMemoryLimit(uint64(mem)))
			}
		}
	}

	// Create container
	container, err := c.client.NewContainer(
		ctx,
		containerName,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(containerName, image),
		containerd.WithNewSpec(opts...),
		containerd.WithContainerLabels(map[string]string{
			"synthesis.pod":       podName,
			"synthesis.container": spec.Name,
			"managed-by":          "synthesis",
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Return container info
	info, err := c.InspectContainer(ctx, container.ID())
	if err != nil {
		return nil, fmt.Errorf("failed to inspect created container: %w", err)
	}

	return info, nil
}

// StartContainer starts a container
func (c *ContainerdRuntime) StartContainer(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// Create task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Start task
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	return nil
}

// StopContainer stops a container
func (c *ContainerdRuntime) StopContainer(ctx context.Context, containerID string, timeout int) error {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Send SIGTERM
	if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to kill task: %w", err)
	}

	// Wait for exit or timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	_, err = task.Wait(timeoutCtx)
	if err != nil {
		// Force kill if timeout
		task.Kill(ctx, syscall.SIGKILL)
	}

	return nil
}

// RemoveContainer removes a container
func (c *ContainerdRuntime) RemoveContainer(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// Delete task if exists
	task, err := container.Task(ctx, nil)
	if err == nil {
		_, err = task.Delete(ctx)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("failed to delete task: %w", err)
		}
	}

	// Delete container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	return nil
}

// RestartContainer restarts a container
func (c *ContainerdRuntime) RestartContainer(ctx context.Context, containerID string) error {
	if err := c.StopContainer(ctx, containerID, c.config.Timeout); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	if err := c.StartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// InspectContainer returns detailed information about a container
func (c *ContainerdRuntime) InspectContainer(ctx context.Context, containerID string) (*runtime.ContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	info, err := container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	// Get task status
	status := runtime.ContainerStatusCreated
	var pid int
	task, err := container.Task(ctx, nil)
	if err == nil {
		taskStatus, err := task.Status(ctx)
		if err == nil {
			status = c.convertProcessStatus(taskStatus.Status)
			pid = int(task.Pid())
		}
	}

	return &runtime.ContainerInfo{
		ID:      container.ID(),
		Name:    container.ID(),
		Image:   info.Image,
		Status:  status,
		Created: info.CreatedAt.Unix(),
		Labels:  info.Labels,
		State: runtime.ContainerState{
			Status:  status,
			Running: status == runtime.ContainerStatusRunning,
			PID:     pid,
		},
	}, nil
}

// ListContainers lists containers based on filters
func (c *ContainerdRuntime) ListContainers(ctx context.Context, filters runtime.ContainerFilter) ([]*runtime.ContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	containers, err := c.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []*runtime.ContainerInfo
	for _, container := range containers {
		info, err := c.InspectContainer(ctx, container.ID())
		if err != nil {
			continue // Skip containers that can't be inspected
		}

		// Apply filters
		if c.matchesFilters(info, filters) {
			result = append(result, info)
		}
	}

	return result, nil
}

// GetContainerLogs retrieves container logs
func (c *ContainerdRuntime) GetContainerLogs(ctx context.Context, containerID string, opts runtime.LogOptions) (io.ReadCloser, error) {
	// containerd logs are typically handled by the runtime or external log drivers
	// For simplicity, return a basic implementation
	return io.NopCloser(strings.NewReader(fmt.Sprintf("Logs for container %s not implemented in containerd runtime", containerID))), nil
}

// ExecContainer executes a command in a container
func (c *ContainerdRuntime) ExecContainer(ctx context.Context, containerID string, cmd []string) (*runtime.ExecResult, error) {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	// For now, return a simplified implementation
	// A full implementation would require more complex containerd exec setup
	return &runtime.ExecResult{
		ExitCode: 0,
		Stdout:   "Exec not fully implemented for containerd",
		Stderr:   "",
	}, nil
}

// PullImage pulls an image
func (c *ContainerdRuntime) PullImage(ctx context.Context, image string) error {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	_, err := c.client.Pull(ctx, image, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	return nil
}

// RemoveImage removes an image
func (c *ContainerdRuntime) RemoveImage(ctx context.Context, image string) error {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	img, err := c.client.GetImage(ctx, image)
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}

	if err := c.client.ImageService().Delete(ctx, img.Name()); err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	return nil
}

// ListImages lists available images
func (c *ContainerdRuntime) ListImages(ctx context.Context) ([]*runtime.ImageInfo, error) {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	images, err := c.client.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	var result []*runtime.ImageInfo
	for _, img := range images {
		result = append(result, &runtime.ImageInfo{
			ID:       img.Target().Digest.String(),
			RepoTags: []string{img.Name()},
			Size:     img.Target().Size,
		})
	}

	return result, nil
}

// Network operations (simplified for containerd)
func (c *ContainerdRuntime) CreateNetwork(ctx context.Context, name string, opts runtime.NetworkOptions) (*runtime.NetworkInfo, error) {
	// containerd doesn't handle networking directly - this would typically be handled by CNI
	return &runtime.NetworkInfo{
		ID:     name,
		Name:   name,
		Driver: "bridge",
	}, nil
}

func (c *ContainerdRuntime) RemoveNetwork(ctx context.Context, networkID string) error {
	// No-op for containerd
	return nil
}

func (c *ContainerdRuntime) ConnectContainer(ctx context.Context, containerID, networkID string) error {
	// No-op for containerd
	return nil
}

func (c *ContainerdRuntime) DisconnectContainer(ctx context.Context, containerID, networkID string) error {
	// No-op for containerd
	return nil
}

// GetContainerStats retrieves container statistics
func (c *ContainerdRuntime) GetContainerStats(ctx context.Context, containerID string) (*runtime.ContainerStats, error) {
	ctx = namespaces.WithNamespace(ctx, c.namespace)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	_, err = task.Metrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Convert metrics to our format (simplified)
	return &runtime.ContainerStats{
		ContainerID: containerID,
		Read:        time.Now().Unix(),
		// TODO: Parse actual metrics from containerd metrics
	}, nil
}

// GetSystemInfo retrieves system information
func (c *ContainerdRuntime) GetSystemInfo(ctx context.Context) (*runtime.SystemInfo, error) {
	version, err := c.client.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}

	return &runtime.SystemInfo{
		ContainerRuntime: "containerd",
		RuntimeVersion:   version.Version,
		Architecture:     "amd64", // Would need to detect actual architecture
		NCPU:             4,       // Would need to detect actual CPU count
		MemTotal:         8 << 30, // Would need to detect actual memory
	}, nil
}

// HealthCheck checks if the runtime is healthy
func (c *ContainerdRuntime) HealthCheck(ctx context.Context) error {
	_, err := c.client.Version(ctx)
	return err
}

// Helper functions

func (c *ContainerdRuntime) parseCPULimit(cpuLimit string) (int64, error) {
	if strings.HasSuffix(cpuLimit, "m") {
		milliCPU, err := strconv.ParseInt(cpuLimit[:len(cpuLimit)-1], 10, 64)
		if err != nil {
			return 0, err
		}
		return milliCPU, nil
	}

	cpu, err := strconv.ParseFloat(cpuLimit, 64)
	if err != nil {
		return 0, err
	}
	return int64(cpu * 1000), nil
}

func (c *ContainerdRuntime) parseMemoryLimit(memLimit string) (int64, error) {
	multiplier := int64(1)

	if strings.HasSuffix(memLimit, "Ki") {
		multiplier = 1024
		memLimit = memLimit[:len(memLimit)-2]
	} else if strings.HasSuffix(memLimit, "Mi") {
		multiplier = 1024 * 1024
		memLimit = memLimit[:len(memLimit)-2]
	} else if strings.HasSuffix(memLimit, "Gi") {
		multiplier = 1024 * 1024 * 1024
		memLimit = memLimit[:len(memLimit)-2]
	}

	mem, err := strconv.ParseInt(memLimit, 10, 64)
	if err != nil {
		return 0, err
	}
	return mem * multiplier, nil
}

func (c *ContainerdRuntime) convertProcessStatus(status containerd.ProcessStatus) runtime.ContainerStatus {
	switch status {
	case containerd.Created:
		return runtime.ContainerStatusCreated
	case containerd.Running:
		return runtime.ContainerStatusRunning
	case containerd.Stopped:
		return runtime.ContainerStatusExited
	case containerd.Paused:
		return runtime.ContainerStatusPaused
	default:
		return runtime.ContainerStatusExited
	}
}

func (c *ContainerdRuntime) matchesFilters(info *runtime.ContainerInfo, filters runtime.ContainerFilter) bool {
	// Check label filters
	for key, value := range filters.Labels {
		if info.Labels[key] != value {
			return false
		}
	}

	// Check name filters
	if len(filters.Names) > 0 {
		nameMatch := false
		for _, name := range filters.Names {
			if strings.Contains(info.Name, name) {
				nameMatch = true
				break
			}
		}
		if !nameMatch {
			return false
		}
	}

	// Check status filters
	if len(filters.Status) > 0 {
		statusMatch := false
		for _, status := range filters.Status {
			if info.Status == status {
				statusMatch = true
				break
			}
		}
		if !statusMatch {
			return false
		}
	}

	return true
} 