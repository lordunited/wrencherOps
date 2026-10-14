output "service_dns" {
  description = "The Service's name inside the cluster."
  value       = "${kubernetes_service_v1.this.metadata[0].name}.${local.namespace}.svc.cluster.local"
}

output "load_balancer_ingress" {
  description = "The external address of a LoadBalancer Service, once the cloud assigns one."
  value       = try(kubernetes_service_v1.this.status[0].load_balancer[0].ingress, [])
}
