output "resource_group_name" {
  value = azurerm_resource_group.main.name
}

output "container_app_url" {
  value = "https://${azurerm_container_app.api.latest_revision_fqdn}"
}

output "container_registry_login_server" {
  value = azurerm_container_registry.main.login_server
}

output "container_registry_name" {
  value = azurerm_container_registry.main.name
}

output "postgres_server" {
  value = azurerm_postgresql_flexible_server.postgres.fqdn
}

output "eventhub_kafka_broker" {
  value = local.eventhub_broker
}

output "monitor_workspace_id" {
  value = azurerm_monitor_workspace.main.id
}