package storage

import "github.com/synthesis/orchestrator/pkg/api"

// Storage defines the interface for persisting orchestrator state
type Storage interface {
	// Pod operations
	StorePod(pod *api.Pod) error
	GetPod(name string) (*api.Pod, error)
	ListPods() ([]*api.Pod, error)
	DeletePod(name string) error
	
	// Deployment operations
	StoreDeployment(deployment *api.Deployment) error
	GetDeployment(name string) (*api.Deployment, error)
	ListDeployments() ([]*api.Deployment, error)
	DeleteDeployment(name string) error
	
	// StatefulSet operations
	StoreStatefulSet(statefulset *api.StatefulSet) error
	GetStatefulSet(name string) (*api.StatefulSet, error)
	ListStatefulSets() ([]*api.StatefulSet, error)
	DeleteStatefulSet(name string) error
	
	// Service operations
	StoreService(service *api.Service) error
	GetService(name string) (*api.Service, error)
	ListServices() ([]*api.Service, error)
	DeleteService(name string) error
	
	// Node operations
	StoreNode(node *api.Node) error
	GetNode(name string) (*api.Node, error)
	ListNodes() ([]*api.Node, error)
	DeleteNode(name string) error
	
	// General operations
	Close() error
} 