package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/synthesis/orchestrator/pkg/api"
	"github.com/synthesis/orchestrator/pkg/runtime"
)

// WorkloadController manages workload lifecycle
type WorkloadController struct {
	server *Server
}

// NewWorkloadController creates a new workload controller
func NewWorkloadController(server *Server) *WorkloadController {
	return &WorkloadController{
		server: server,
	}
}

// Run starts the workload controller
func (c *WorkloadController) Run(ctx context.Context) {
	log.Println("Starting workload controller...")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Workload controller stopped")
			return
		case <-ticker.C:
			c.reconcileWorkloads(ctx)
		}
	}
}

// reconcileWorkloads ensures workloads match their desired state
func (c *WorkloadController) reconcileWorkloads(ctx context.Context) {
	c.server.mutex.RLock()
	
	// Process all deployments
	for _, deployment := range c.server.deployments {
		c.server.mutex.RUnlock()
		if err := c.reconcileDeployment(ctx, deployment); err != nil {
			log.Printf("Failed to reconcile deployment %s: %v", deployment.ObjectMeta.Name, err)
		}
		c.server.mutex.RLock()
	}
	
	// Process all statefulsets
	for _, statefulset := range c.server.statefulsets {
		c.server.mutex.RUnlock()
		if err := c.reconcileStatefulSet(ctx, statefulset); err != nil {
			log.Printf("Failed to reconcile statefulset %s: %v", statefulset.ObjectMeta.Name, err)
		}
		c.server.mutex.RLock()
	}
	
	c.server.mutex.RUnlock()
}

// reconcileDeployment ensures a deployment matches its desired state
func (c *WorkloadController) reconcileDeployment(ctx context.Context, deployment *api.Deployment) error {
	// Get current containers for this deployment
	currentContainers, err := c.server.runtime.ListContainers(ctx, runtime.ContainerFilter{
		Labels: map[string]string{
			"synthesis.deployment": deployment.ObjectMeta.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}
	currentReplicas := int32(len(currentContainers))
	
	// Scale up if needed
	if currentReplicas < desiredReplicas {
		needed := int(desiredReplicas - currentReplicas)
		log.Printf("Scaling up deployment %s: need %d more replicas", deployment.ObjectMeta.Name, needed)
		
		for i := 0; i < needed; i++ {
			if err := c.createPodFromTemplate(ctx, deployment.ObjectMeta.Name, &deployment.Spec.Template, int(currentReplicas)+i); err != nil {
				log.Printf("Failed to create pod for deployment %s: %v", deployment.ObjectMeta.Name, err)
				continue
			}
		}
	}
	
	// Scale down if needed
	if currentReplicas > desiredReplicas {
		excess := int(currentReplicas - desiredReplicas)
		log.Printf("Scaling down deployment %s: removing %d replicas", deployment.ObjectMeta.Name, excess)
		
		for i := 0; i < excess && i < len(currentContainers); i++ {
			container := currentContainers[i]
			if err := c.server.runtime.StopContainer(ctx, container.ID, 30); err != nil {
				log.Printf("Failed to stop container %s: %v", container.ID, err)
				continue
			}
			if err := c.server.runtime.RemoveContainer(ctx, container.ID); err != nil {
				log.Printf("Failed to remove container %s: %v", container.ID, err)
			}
		}
	}
	
	return nil
}

// reconcileStatefulSet ensures a statefulset matches its desired state
func (c *WorkloadController) reconcileStatefulSet(ctx context.Context, statefulset *api.StatefulSet) error {
	// Get current containers for this statefulset
	currentContainers, err := c.server.runtime.ListContainers(ctx, runtime.ContainerFilter{
		Labels: map[string]string{
			"synthesis.statefulset": statefulset.ObjectMeta.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	desiredReplicas := int32(1)
	if statefulset.Spec.Replicas != nil {
		desiredReplicas = *statefulset.Spec.Replicas
	}
	currentReplicas := int32(len(currentContainers))
	
	// Scale up if needed
	if currentReplicas < desiredReplicas {
		needed := int(desiredReplicas - currentReplicas)
		log.Printf("Scaling up statefulset %s: need %d more replicas", statefulset.ObjectMeta.Name, needed)
		
		for i := 0; i < needed; i++ {
			if err := c.createPodFromTemplate(ctx, statefulset.ObjectMeta.Name, &statefulset.Spec.Template, int(currentReplicas)+i); err != nil {
				log.Printf("Failed to create pod for statefulset %s: %v", statefulset.ObjectMeta.Name, err)
				continue
			}
		}
	}
	
	// Scale down if needed
	if currentReplicas > desiredReplicas {
		excess := int(currentReplicas - desiredReplicas)
		log.Printf("Scaling down statefulset %s: removing %d replicas", statefulset.ObjectMeta.Name, excess)
		
		for i := 0; i < excess && i < len(currentContainers); i++ {
			container := currentContainers[i]
			if err := c.server.runtime.StopContainer(ctx, container.ID, 30); err != nil {
				log.Printf("Failed to stop container %s: %v", container.ID, err)
				continue
			}
			if err := c.server.runtime.RemoveContainer(ctx, container.ID); err != nil {
				log.Printf("Failed to remove container %s: %v", container.ID, err)
			}
		}
	}
	
	return nil
}

// createPodFromTemplate creates a new pod from a template
func (c *WorkloadController) createPodFromTemplate(ctx context.Context, workloadName string, template *api.PodTemplateSpec, replica int) error {
	podName := fmt.Sprintf("%s-%d", workloadName, replica)
	
	// Create containers for this pod
	for _, containerSpec := range template.Spec.Containers {
		// Pull image if not exists
		if err := c.server.runtime.PullImage(ctx, containerSpec.Image); err != nil {
			log.Printf("Warning: Failed to pull image %s: %v", containerSpec.Image, err)
		}
		
		// Create container
		container, err := c.server.runtime.CreateContainer(ctx, &containerSpec, podName)
		if err != nil {
			return fmt.Errorf("failed to create container %s: %w", containerSpec.Name, err)
		}
		
		// Start container
		if err := c.server.runtime.StartContainer(ctx, container.ID); err != nil {
			// Clean up on failure
			c.server.runtime.RemoveContainer(ctx, container.ID)
			return fmt.Errorf("failed to start container %s: %w", container.ID, err)
		}
		
		log.Printf("Created and started container %s for workload %s", container.ID[:12], workloadName)
	}
	
	return nil
}

// ServiceController manages service lifecycle
type ServiceController struct {
	server *Server
}

// NewServiceController creates a new service controller
func NewServiceController(server *Server) *ServiceController {
	return &ServiceController{
		server: server,
	}
}

// Run starts the service controller
func (c *ServiceController) Run(ctx context.Context) {
	log.Println("Starting service controller...")
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Service controller stopped")
			return
		case <-ticker.C:
			c.reconcileServices(ctx)
		}
	}
}

// reconcileServices ensures services match their desired state
func (c *ServiceController) reconcileServices(ctx context.Context) {
	c.server.mutex.RLock()
	services := make([]*api.Service, 0, len(c.server.services))
	for _, service := range c.server.services {
		services = append(services, service)
	}
	c.server.mutex.RUnlock()
	
	for _, service := range services {
		if err := c.reconcileService(ctx, service); err != nil {
			log.Printf("Failed to reconcile service %s: %v", service.ObjectMeta.Name, err)
		}
	}
}

// reconcileService ensures a single service matches its desired state
func (c *ServiceController) reconcileService(ctx context.Context, service *api.Service) error {
	// Find target containers based on selector
	targetContainers, err := c.findTargetContainers(ctx, service.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to find target containers: %w", err)
	}
	
	// For now, we'll just update the service status
	// In a real implementation, this would set up load balancing, 
	// network routing, etc.
	c.updateServiceStatus(service, targetContainers)
	
	log.Printf("Service %s targeting %d containers", service.ObjectMeta.Name, len(targetContainers))
	return nil
}

// findTargetContainers finds containers that match the service selector
func (c *ServiceController) findTargetContainers(ctx context.Context, selector map[string]string) ([]*runtime.ContainerInfo, error) {
	// Get all synthesis-managed containers
	allContainers, err := c.server.runtime.ListContainers(ctx, runtime.ContainerFilter{
		Labels: map[string]string{
			"managed-by": "synthesis",
		},
	})
	if err != nil {
		return nil, err
	}
	
	var targetContainers []*runtime.ContainerInfo
	
	// Filter based on selector
	for _, container := range allContainers {
		if c.matchesSelector(container.Labels, selector) {
			targetContainers = append(targetContainers, container)
		}
	}
	
	return targetContainers, nil
}

// matchesSelector checks if container labels match the service selector
func (c *ServiceController) matchesSelector(containerLabels, selector map[string]string) bool {
	for key, value := range selector {
		if containerLabels[key] != value {
			return false
		}
	}
	return true
}

// updateServiceStatus updates the status of a service
func (c *ServiceController) updateServiceStatus(service *api.Service, containers []*runtime.ContainerInfo) {
	c.server.mutex.Lock()
	defer c.server.mutex.Unlock()
	
	if s, exists := c.server.services[service.ObjectMeta.Name]; exists {
		// For ClusterIP services, we would typically set up internal networking
		// For NodePort services, we would expose ports on the host
		
		// Simplified status update
		if service.Spec.Type == api.ServiceTypeNodePort {
			// In a real implementation, we'd assign actual node ports
			ingress := api.LoadBalancerIngress{
				IP: "127.0.0.1", // Local node IP
			}
			s.Status.LoadBalancer.Ingress = []api.LoadBalancerIngress{ingress}
		}
	}
} 