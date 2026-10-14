# One instance and security group per entry of var.instances (see
# instances.auto.tfvars), each from the reusable module in modules/aws-ec2.
module "instance" {
  source   = "./modules/aws-ec2"
  for_each = var.instances

  name           = each.key
  instance_type  = each.value.instance_type
  ami_id         = each.value.ami_id
  subnet_id      = each.value.subnet_id
  key_name       = each.value.key_name
  public_ip      = each.value.public_ip
  root_volume_gb = each.value.root_volume_gb
  ingress        = each.value.ingress
  user_data      = each.value.user_data
  tags           = each.value.tags
}
