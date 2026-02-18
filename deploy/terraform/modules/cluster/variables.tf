variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
}

variable "subnet_ids" {
  description = "Subnet IDs for the cluster"
  type        = list(string)
}

variable "node_instance_type" {
  description = "EC2 instance type for node group"
  type        = string
}

variable "desired_nodes" {
  description = "Desired number of nodes in the node group"
  type        = number
}
