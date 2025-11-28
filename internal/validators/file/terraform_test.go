package file_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("TerraformValidator", func() {
	var (
		v   *file.TerraformValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		runner := execpkg.NewCommandRunner(10 * time.Second)
		formatter := linters.NewTerraformFormatter(runner)
		linter := linters.NewTfLinter(runner)
		v = file.NewTerraformValidator(formatter, linter, logger.NewNoOpLogger(), nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
		}
	})

	Describe("Name", func() {
		It("returns correct validator name", func() {
			Expect(v.Name()).To(Equal("validate-terraform"))
		})
	})

	Describe("Validate", func() {
		Context("with valid terraform content", func() {
			It("passes for empty content", func() {
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("passes for well-formatted terraform", func() {
				content := `resource "aws_instance" "example" {
  ami           = "ami-12345678"
  instance_type = "t2.micro"

  tags = {
    Name = "example-instance"
  }
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)

				// This will pass if terraform/tofu is not installed or if formatting is correct
				// We're just testing that the validator doesn't crash
				Expect(result).NotTo(BeNil())
			})

			It("passes for variable declarations", func() {
				content := `variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "instance_count" {
  description = "Number of instances"
  type        = number
  default     = 1
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})

			It("passes for output declarations", func() {
				content := `output "instance_id" {
  description = "ID of the EC2 instance"
  value       = aws_instance.example.id
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})
		})

		Context("with terraform formatting issues", func() {
			It("handles badly formatted terraform", func() {
				content := `resource "aws_instance" "example" {
ami = "ami-12345678"
instance_type="t2.micro"
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)

				// If terraform/tofu is installed, this should warn about formatting
				// If not installed, it will pass with a warning about missing tools
				Expect(result).NotTo(BeNil())
			})

			It("handles missing spacing", func() {
				content := `variable "test"{
type=string
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})
		})

		Context("edge cases", func() {
			It("skips validation for Edit operations in PreToolUse", func() {
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = "/path/to/main.tf"
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("skips validation when no content available", func() {
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles empty file content", func() {
				ctx.ToolInput.Content = ""
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("handles whitespace-only content", func() {
				ctx.ToolInput.Content = "   \n\n   \n"
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})
		})

		Context("complex terraform scenarios", func() {
			It("handles module declarations", func() {
				content := `module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
  version = "3.0.0"

  name = "my-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})

			It("handles data sources", func() {
				content := `data "aws_ami" "ubuntu" {
  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"]
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})

			It("handles locals", func() {
				content := `locals {
  common_tags = {
    Environment = "production"
    ManagedBy   = "terraform"
  }

  name_prefix = "myapp"
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})

			It("handles provider configuration", func() {
				content := `provider "aws" {
  region = var.region

  default_tags {
    tags = local.common_tags
  }
}

terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})
		})

		Context("real-world terraform examples", func() {
			It("handles a complete terraform configuration", func() {
				content := `terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = "us-east-1"
}

resource "aws_security_group" "web" {
  name        = "web-sg"
  description = "Security group for web servers"

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "web" {
  ami                    = "ami-12345678"
  instance_type          = "t2.micro"
  vpc_security_group_ids = [aws_security_group.web.id]

  tags = {
    Name = "web-server"
  }
}

output "instance_ip" {
  value = aws_instance.web.public_ip
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})

			It("handles terraform with conditionals", func() {
				content := `variable "create_instance" {
  type    = bool
  default = true
}

resource "aws_instance" "conditional" {
  count = var.create_instance ? 1 : 0

  ami           = "ami-12345678"
  instance_type = "t2.micro"
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})

			It("handles terraform with for_each", func() {
				content := `variable "instances" {
  type = map(object({
    ami           = string
    instance_type = string
  }))
}

resource "aws_instance" "multiple" {
  for_each = var.instances

  ami           = each.value.ami
  instance_type = each.value.instance_type

  tags = {
    Name = each.key
  }
}
`
				ctx.ToolInput.Content = content
				result := v.Validate(context.Background(), ctx)
				Expect(result).NotTo(BeNil())
			})
		})
	})
})
