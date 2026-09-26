import { NewRawServiceForm } from "@/components/NewRawServiceForm";

const PLACEHOLDER = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: app
  template:
    metadata:
      labels:
        app: app
    spec:
      containers:
        - name: app
          image: nginx:latest
          ports:
            - containerPort: 80
`;

export default function NewKubernetesServicePage() {
  return (
    <NewRawServiceForm
      title="New Kubernetes service"
      description="Paste your own manifest. It's applied with kubectl apply, using whatever cluster access is already configured for the SSH user on the target — this tool doesn't manage kubeconfigs."
      endpoint="/api/services/kubernetes"
      basePath="/deploy/kubernetes"
      backLabel="Kubernetes"
      manifestLabel="Manifest"
      manifestPlaceholder={PLACEHOLDER}
      defaultDeployDir="/opt/app"
    />
  );
}
