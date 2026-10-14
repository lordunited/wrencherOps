locals {
  labels = {
    "app.kubernetes.io/name"       = var.name
    "app.kubernetes.io/managed-by" = "opentofu"
  }
  namespace = var.create_namespace ? kubernetes_namespace_v1.this[0].metadata[0].name : var.namespace
}

resource "kubernetes_namespace_v1" "this" {
  count = var.create_namespace ? 1 : 0

  metadata {
    name = var.namespace
  }
}

resource "kubernetes_deployment_v1" "this" {
  metadata {
    name      = var.name
    namespace = local.namespace
    labels    = local.labels
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = { "app.kubernetes.io/name" = var.name }
    }

    template {
      metadata {
        labels = local.labels
      }

      spec {
        security_context {
          run_as_non_root = true
          seccomp_profile {
            type = "RuntimeDefault"
          }
        }

        container {
          name  = var.name
          image = var.image

          port {
            container_port = var.container_port
          }

          dynamic "env" {
            for_each = var.env
            content {
              name  = env.key
              value = env.value
            }
          }

          resources {
            requests = {
              cpu    = var.cpu_request
              memory = var.memory_request
            }
            limits = {
              cpu    = var.cpu_limit
              memory = var.memory_limit
            }
          }

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = var.read_only_root_filesystem
            capabilities {
              drop = ["ALL"]
            }
          }

          dynamic "readiness_probe" {
            for_each = var.health_path != "" ? [var.health_path] : []
            content {
              http_get {
                path = readiness_probe.value
                port = var.container_port
              }
              period_seconds = 10
            }
          }

          dynamic "liveness_probe" {
            for_each = var.health_path != "" ? [var.health_path] : []
            content {
              http_get {
                path = liveness_probe.value
                port = var.container_port
              }
              initial_delay_seconds = 15
              period_seconds        = 20
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "this" {
  metadata {
    name      = var.name
    namespace = local.namespace
    labels    = local.labels
  }

  spec {
    type     = var.service_type
    selector = { "app.kubernetes.io/name" = var.name }

    port {
      port        = var.service_port
      target_port = var.container_port
      protocol    = "TCP"
    }
  }
}
