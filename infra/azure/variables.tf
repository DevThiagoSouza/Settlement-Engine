variable "subscription_id" {
  type      = string
  sensitive = true
}

variable "location" {
  type    = string
  default = "eastus"
}

variable "resource_group_name" {
  type    = string
  default = "settlement-rg"
}

variable "name_prefix" {
  type    = string
  default = "settlement"
}

variable "container_image" {
  type    = string
  default = "settlement-api:latest"
}

variable "postgres_admin_username" {
  type    = string
  default = "settlement_admin"
}

variable "postgres_admin_password" {
  type      = string
  sensitive = true
}

variable "postgres_database_name" {
  type    = string
  default = "settlement_engine"
}

variable "eventhub_connection_string" {
  type      = string
  sensitive = true
  default   = ""
}

variable "environment" {
  type    = string
  default = "production"
}

variable "jwt_secret" {
  type      = string
  sensitive = true
}