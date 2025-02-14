# Mallory Operator

Kubernetes controller for modeling threat scenarios. It creates resources and executes actions such as reconnaissance, privilege escalation, and data exfiltration to evaluate security measures.

## Features
- **Dynamic Resource Creation:** Automatically generate Kubernetes resources to replicate different scenarios.
  
- **Threat Simulation:** Execute actions including:
  - **Reconnaissance:** Collect cluster information.
  - **Privilege Escalation:** Imitate attempts to gain elevated access.
  - **Data Exfiltration:** Simulate the extraction of sensitive data.
  
- **Customizable Scenarios:** Configure simulation parameters to suit various testing requirements.

- **Training and Testing:** Provide a controlled environment for evaluating security defenses and training on incident response.

### Use Cases

- **Security Testing:** Assess the effectiveness of security measures, detection systems, and response processes.

- **Incident Response Drills:** Practice and refine response procedures for different threat scenarios.

- **Educational Purposes:** Demonstrate simulated threat activities for security training sessions.
> [!IMPORTANT]  
>  Mallory Operator is intended for controlled and ethical use only. Ensure that simulations are conducted in compliant environments and in accordance with legal requirements.

## Getting Started

### Deploy on the cluster

```sh
helm install mallory oci://registry-1.docker.io/explabs/mallory
```

> [!NOTE] 
>  If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

### Apply intruder
You can apply the examples from `actions` directory:

```sh
kubectl apply -f actions
```

> [!NOTE] 
>  Ensure that the samples has default values to test it out.

### Uninstall
```sh
helm uninstall mallory
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

