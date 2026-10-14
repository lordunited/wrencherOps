data "aws_ami" "ubuntu" {
  count       = var.ami_id == "" ? 1 : 0
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

data "aws_vpc" "default" {
  count   = var.subnet_id == "" ? 1 : 0
  default = true
}

data "aws_subnets" "default" {
  count = var.subnet_id == "" ? 1 : 0

  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default[0].id]
  }
}

data "aws_subnet" "this" {
  id = var.subnet_id != "" ? var.subnet_id : sort(data.aws_subnets.default[0].ids)[0]
}

resource "aws_security_group" "this" {
  name        = var.name
  description = "${var.name}, managed by OpenTofu"
  vpc_id      = data.aws_subnet.this.vpc_id
  tags        = merge(var.tags, { Name = var.name })
}

resource "aws_vpc_security_group_ingress_rule" "this" {
  for_each = { for r in var.ingress : "${r.protocol}-${r.port}-${r.cidr}" => r }

  security_group_id = aws_security_group.this.id
  ip_protocol       = each.value.protocol
  from_port         = each.value.port
  to_port           = each.value.port
  cidr_ipv4         = each.value.cidr
  description       = each.value.description
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.this.id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
  description       = "All outbound"
}

resource "aws_instance" "this" {
  ami                         = var.ami_id != "" ? var.ami_id : data.aws_ami.ubuntu[0].id
  instance_type               = var.instance_type
  subnet_id                   = data.aws_subnet.this.id
  vpc_security_group_ids      = [aws_security_group.this.id]
  key_name                    = var.key_name != "" ? var.key_name : null
  associate_public_ip_address = var.public_ip
  user_data                   = var.user_data != "" ? var.user_data : null
  tags                        = merge(var.tags, { Name = var.name })

  root_block_device {
    volume_size = var.root_volume_gb
    volume_type = "gp3"
    encrypted   = true
  }

  # IMDSv2 only: a token is needed to read instance metadata.
  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  lifecycle {
    # A newer Ubuntu AMI appearing must not replace a running instance.
    ignore_changes = [ami]
  }
}
