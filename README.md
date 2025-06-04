# Synthesis - Like Kubernetes, but Tinier

Synthesis is a lightweight container orchestrator that's meant to be k8s compatible, but significantly smaller and simpler.

## Why Synthesis?

- **🚀 Lightning Fast**: ~5s startup vs ~60s for Kubernetes
- **💾 Tiny Footprint**: ~20MB binary vs 100MB+ for Kubernetes 
- **📋 Drop-in Compatible**: Use your existing Kubernetes YAML manifests
- **🔧 No Dependencies**: Uses containerd directly, no Docker required
- **📁 Simple Storage**: File-based persistence, no external database needed
- **⚡ Instant Setup**: Single binary deployment

## Quick Start

### Prerequisites

- **containerd** installed and running
- **Go 1.21+** (for building from source)

### 1. Build and Start

```bash
# Clone and build
git clone <repository>
cd Synthesis
make deps
make build

# Start the server
make start
```
