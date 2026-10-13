data "azurerm_client_config" "current" {}

resource "random_string" "suffix" {
  length  = 6
  special = false
  upper   = false
}

locals {
  base_name       = "${var.name_prefix}-${random_string.suffix.result}"
  acr_name        = replace("${var.name_prefix}${random_string.suffix.result}", "-", "")
  postgres_host   = azurerm_postgresql_flexible_server.postgres.fqdn
  database_url    = "postgres://${var.postgres_admin_username}:${var.postgres_admin_password}@${local.postgres_host}:5432/${var.postgres_database_name}?sslmode=require"
  eventhub_broker = "${azurerm_eventhub_namespace.events.name}.servicebus.windows.net:9093"
}

resource "azurerm_resource_group" "main" {
  name     = var.resource_group_name
  location = var.location
}

resource "azurerm_log_analytics_workspace" "main" {
  name                = "${local.base_name}-logs"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

resource "azurerm_application_insights" "main" {
  name                = "${local.base_name}-insights"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  application_type    = "web"
  workspace_id        = azurerm_log_analytics_workspace.main.id
}

resource "azurerm_monitor_workspace" "main" {
  name                = "${local.base_name}-prometheus"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_container_registry" "main" {
  name                = local.acr_name
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  sku                 = "Basic"
  admin_enabled       = false
}

resource "azurerm_user_assigned_identity" "acr_pull" {
  name                = "${local.base_name}-acr-pull"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_role_assignment" "acr_pull" {
  scope                = azurerm_container_registry.main.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.acr_pull.principal_id
}

resource "azurerm_postgresql_flexible_server" "postgres" {
  name                   = "${local.base_name}-postgres"
  resource_group_name    = azurerm_resource_group.main.name
  location               = azurerm_resource_group.main.location
  version                = "15"
  administrator_login    = var.postgres_admin_username
  administrator_password = var.postgres_admin_password
  storage_mb             = 32768
  sku_name               = "B_Standard_B1ms"
  zone                   = "1"
}

resource "azurerm_postgresql_flexible_server_database" "main" {
  name      = var.postgres_database_name
  server_id = azurerm_postgresql_flexible_server.postgres.id
  charset   = "UTF8"
  collation = "en_US.utf8"
}

resource "azurerm_postgresql_flexible_server_firewall_rule" "container_apps" {
  name             = "allow-container-apps"
  server_id        = azurerm_postgresql_flexible_server.postgres.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "0.0.0.0"
}

resource "azurerm_eventhub_namespace" "events" {
  name                = "${local.base_name}-events"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "Standard"
  capacity            = 1
}

resource "azurerm_eventhub" "settlement" {
  name                = "settlement-events"
  namespace_name      = azurerm_eventhub_namespace.events.name
  resource_group_name = azurerm_resource_group.main.name
  partition_count     = 2
  message_retention   = 7
}

resource "azurerm_eventhub_namespace_authorization_rule" "app" {
  name                = "settlement-app"
  namespace_name      = azurerm_eventhub_namespace.events.name
  resource_group_name = azurerm_resource_group.main.name
  listen              = true
  send                = true
  manage              = false
}

resource "azurerm_key_vault" "main" {
  name                       = "${local.base_name}-vault"
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "standard"
  purge_protection_enabled   = true
  soft_delete_retention_days = 7

  access_policy {
    tenant_id = data.azurerm_client_config.current.tenant_id
    object_id = data.azurerm_client_config.current.object_id

    secret_permissions = [
      "Get",
      "List",
      "Set",
      "Delete",
      "Recover",
      "Purge",
    ]
  }
}

resource "azurerm_key_vault_secret" "database_url" {
  name         = "database-url"
  value        = local.database_url
  key_vault_id = azurerm_key_vault.main.id
}

resource "azurerm_key_vault_secret" "eventhub_connection_string" {
  name         = "eventhub-connection-string"
  value        = azurerm_eventhub_namespace_authorization_rule.app.primary_connection_string
  key_vault_id = azurerm_key_vault.main.id
}

resource "azurerm_key_vault_secret" "jwt_secret" {
  name         = "jwt-secret"
  value        = var.jwt_secret
  key_vault_id = azurerm_key_vault.main.id
}

resource "azurerm_container_app_environment" "main" {
  name                       = "${local.base_name}-env"
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.main.id
}

resource "azurerm_container_app" "api" {
  name                         = "${local.base_name}-api"
  container_app_environment_id = azurerm_container_app_environment.main.id
  resource_group_name          = azurerm_resource_group.main.name
  revision_mode                = "Single"

  identity {
    type         = "SystemAssigned, UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.acr_pull.id]
  }

  registry {
    server   = azurerm_container_registry.main.login_server
    identity = azurerm_user_assigned_identity.acr_pull.id
  }

  secret {
    name                = "database-url"
    identity            = "System"
    key_vault_secret_id = azurerm_key_vault_secret.database_url.versionless_id
  }

  secret {
    name                = "eventhub-connection-string"
    identity            = "System"
    key_vault_secret_id = azurerm_key_vault_secret.eventhub_connection_string.versionless_id
  }

  secret {
    name                = "jwt-secret"
    identity            = "System"
    key_vault_secret_id = azurerm_key_vault_secret.jwt_secret.versionless_id
  }

  template {
    min_replicas = 1
    max_replicas = 3

    container {
      name   = "settlement-api"
      image  = var.container_image
      cpu    = 0.5
      memory = "1Gi"

      env {
        name  = "HTTP_HOST"
        value = "0.0.0.0"
      }

      env {
        name  = "HTTP_PORT"
        value = "8080"
      }

      env {
        name        = "DATABASE_URL"
        secret_name = "database-url"
      }

      env {
        name  = "KAFKA_BROKERS"
        value = local.eventhub_broker
      }

      env {
        name        = "KAFKA_CONNECTION_STRING"
        secret_name = "eventhub-connection-string"
      }

      env {
        name  = "KAFKA_TOPIC_EVENTS"
        value = azurerm_eventhub.settlement.name
      }

      env {
        name  = "APP_ENV"
        value = var.environment
      }

      env {
        name        = "JWT_SECRET"
        secret_name = "jwt-secret"
      }

      env {
        name  = "APPLICATIONINSIGHTS_CONNECTION_STRING"
        value = azurerm_application_insights.main.connection_string
      }
    }
  }

  ingress {
    external_enabled = true
    target_port      = 8080
    transport        = "auto"

    traffic_weight {
      percentage      = 100
      latest_revision = true
    }
  }
}

resource "azurerm_key_vault_access_policy" "container_app" {
  key_vault_id = azurerm_key_vault.main.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_container_app.api.identity[0].principal_id

  secret_permissions = [
    "Get",
  ]
}