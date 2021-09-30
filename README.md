# chaos-mesh

> Re-implemented chaos engineering platform inspired by Chaos Mesh, Litmus, and ChaosBlade with an event-driven architecture

## What's Different?

This project reimplements chaos engineering concepts from three major projects using a completely different technical approach:

**Original Projects:**
- [Chaos Mesh](https://github.com/chaos-mesh/chaos-mesh) - Kubernetes-native chaos engineering with CRD-based experiments
- [Litmus](https://github.com/litmuschaos/litmus) - Cloud-native chaos with centralized control plane and GraphQL API
- [ChaosBlade](https://github.com/chaosblade-io/chaosblade) - Multi-platform chaos toolkit with CLI-first approach

**Our Implementation:**
- **Event-Driven Architecture**: NATS JetStream event bus instead of reconciliation loops or GraphQL subscriptions
- **HCL Configuration**: Terraform-like DSL instead of YAML CRDs
- **SQLite State Storage**: Embedded database instead of etcd or MongoDB
- **gRPC API**: Modern RPC protocol instead of REST/GraphQL
- **Plugin System**: Dynamic plugins via hashicorp/go-plugin instead of compiled-in controllers

## Features

### Core Capabilities
- **Event-Driven Execution**: Asynchronous chaos injection with event sourcing
- **State Machine**: Explicit state transitions with guard conditions
- **HCL Configuration**: Declarative experiment definitions with Terraform-like syntax
- **Workflow Orchestration**: DAG-based multi-step chaos scenarios
- **Probe System**: Health validation before/during/after experiments
- **Multi-Cluster**: Agent-based architecture for distributed deployments

### Fault Injection Types
- **Kubernetes**: Pod kill, container kill, pod failure
- **Network**: Delay, loss, corruption, partition, bandwidth limit
- **Compute**: CPU stress, memory stress
- **Storage**: Disk I/O, disk fill
- **Application**: JVM faults, HTTP faults, DNS faults

### Advanced Features
- **Workflow DAG**: Complex experiment orchestration with dependencies
- **Conditional Execution**: Run steps based on probe results or metrics
- **Event Streaming**: Real-time experiment monitoring via event bus
- **Observability**: OpenTelemetry tracing, Prometheus metrics, structured logging
- **Plugin Architecture**: Extend with custom fault injectors

## Architecture

```
┌─────────────────────────────────────────────┐
│   CLI / gRPC Client                         │
└─────────────────┬───────────────────────────┘
                  │
         ┌────────▼────────┐
         │   gRPC Server   │
         └────────┬────────┘
                  │
    ┌─────────────┼─────────────┐
    │             │             │
┌───▼───┐   ┌────▼────┐   ┌───▼────┐
│SQLite │   │ NATS    │   │ State  │
│ DB    │   │ Event   │   │Machine │
│       │   │ Bus     │   │        │
└───────┘   └────┬────┘   └────────┘
                 │
        ┌────────┼────────┐
        │        │        │
    ┌───▼──┐ ┌──▼───┐ ┌──▼───┐
    │ Pod  │ │Network│ │ CPU  │
    │Plugin│ │Plugin │ │Plugin│
    └──────┘ └───────┘ └──────┘
```

## Installation

### From Source

```bash
# Clone repository
git clone https://github.com/vjranagit/chaos-mesh
cd chaos-mesh

# Build
go build -o chaos ./cmd/chaos

# Install
sudo mv chaos /usr/local/bin/
```

### Dependencies

- **NATS Server** (optional, for distributed deployments):
  ```bash
  docker run -d --name nats -p 4222:4222 nats:latest
  ```

- **Kubernetes** (for K8s experiments):
  - kubectl configured with cluster access
  - Appropriate RBAC permissions

## Quick Start

### 1. Create an Experiment

Create `pod-chaos.hcl`:
```hcl
experiment "pod-failure" {
  target = "kubernetes.pod"

  selector {
    namespace = "default"
    labels = {
      app = "nginx"
    }
  }

  fault "kill" {
    mode     = "one"
    duration = "30s"
  }

  probe "http-check" {
    type   = "http"
    url    = "http://nginx-service"
    method = "GET"
    run_at = ["before", "after"]

    expect {
      status_code = 200
    }
  }
}
```

### 2. Run the Experiment

```bash
# Create experiment
chaos create -f pod-chaos.hcl

# List experiments
chaos list

# Get details
chaos get <experiment-id>

# Watch events
chaos watch <experiment-id>

# Delete experiment
chaos delete <experiment-id>
```

### 3. Workflow Example

Create `workflow.hcl`:
```hcl
workflow "cascade-test" {
  step "network-delay" {
    experiment = "network-latency"
  }

  step "pod-kill" {
    experiment = "pod-failure"
    depends_on = ["network-delay"]

    condition {
      probe = "health-check"
    }
  }
}
```

## Configuration Examples

### Network Chaos
```hcl
experiment "network-delay" {
  target = "kubernetes.network"

  selector {
    namespace = "default"
    labels = {
      app = "api"
    }
  }

  fault "delay" {
    mode    = "all"
    delay   = "100ms"
    jitter  = "10ms"
    duration = "60s"
  }

  probe "latency-check" {
    type     = "prometheus"
    endpoint = "http://prometheus:9090"
    query    = "histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))"

    expect {
      threshold = "< 0.5"
    }
  }
}
```

### CPU Stress
```hcl
experiment "cpu-burn" {
  target = "kubernetes.stress"

  selector {
    namespace = "production"
    labels = {
      app = "worker"
    }
  }

  fault "cpu" {
    mode      = "all"
    cpu_load  = 80
    duration  = "300s"
  }
}
```

### Workflow with Conditions
```hcl
workflow "resilience-test" {
  step "baseline" {
    experiment = "health-check"
  }

  step "inject-faults" {
    experiment = "pod-chaos"
    depends_on = ["baseline"]

    condition {
      state = "baseline.completed"
    }
  }

  step "validate" {
    experiment = "validation"
    depends_on = ["inject-faults"]

    condition {
      metric   = "error_rate"
      operator = "<"
      value    = "0.05"
    }
  }
}
```

## API Usage

### gRPC Server

Start the server:
```bash
chaos server --port 9090
```

### Example Client (Go)

```go
import (
    "context"
    pb "github.com/vjranagit/chaos-mesh/api/chaos/v1"
    "google.golang.org/grpc"
)

conn, _ := grpc.Dial("localhost:9090", grpc.WithInsecure())
client := pb.NewChaosServiceClient(conn)

// Create experiment
req := &pb.CreateExperimentRequest{
    Name:   "pod-failure",
    Target: "kubernetes.pod",
    Config: "...", // HCL config
}

resp, _ := client.CreateExperiment(context.Background(), req)
fmt.Printf("Created: %s\n", resp.Id)

// Watch events
stream, _ := client.WatchExperiments(context.Background(), &pb.WatchExperimentsRequest{
    ExperimentId: resp.Id,
})

for {
    event, _ := stream.Recv()
    fmt.Printf("Event: %s\n", event.Type)
}
```

## Development

### Project Structure

```
chaos-mesh/
├── cmd/
│   ├── chaos/          # CLI tool
│   ├── server/         # gRPC server
│   └── agent/          # Cluster agent
├── api/
│   └── chaos/v1/       # Protobuf definitions
├── pkg/
│   ├── eventbus/       # Event bus (NATS)
│   ├── statemachine/   # FSM engine
│   ├── config/         # HCL parser
│   ├── executor/       # Workflow DAG executor
│   └── plugin/         # Plugin framework
├── plugins/
│   ├── kubernetes/     # K8s fault injectors
│   ├── network/        # Network faults
│   └── os/             # OS-level faults
├── internal/
│   ├── db/             # SQLite database
│   └── auth/           # Authentication
└── examples/           # HCL examples
```

### Building

```bash
# Build all components
make build

# Run tests
make test

