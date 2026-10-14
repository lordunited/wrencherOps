# AWS credentials come from environment variables: on the Variables tab add
# AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY as environment variables,
# both sensitive (an IAM user with EC2 access).
provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      ManagedBy = "OpenTofu"
    }
  }
}
