resource "aws_security_group" "concourse_lb_internal_security_group" {
  description = "{{.ConcourseInternalDescription}}"
  vpc_id      = "${aws_vpc.vpc.id}"

  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    protocol    = "TCP"
    from_port   = 443
    to_port     = 443
  }

  egress {
    from_port = 0
    to_port = 0
    protocol = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags {
    Name = "${var.env_id}-concourse-lb-internal-security-group"
  }
}

output "concourse_lb_internal_security_group" {
  value = "${aws_security_group.concourse_lb_internal_security_group.id}"
}

resource "aws_lb" "concourse_lb" {
  name               = "${var.short_env_id}-concourse-lb"
  load_balancer_type = "network"
}

resource  "aws_lb_listener" "concourse_lb_80" {
  load_balancer_arn = "${aws_lb.concourse_lb.arn}"
  port              = 80

  default_action {
    type             = "forward"
    target_group_arn = "${aws_lb_target_group.concourse_lb_80.arn}"
  }
}

resource "aws_lb_target_group" "concourse_lb_80" {
  name     = "${var.short_env_id}-concourse-lb-80"
  port     = 8080
  protocol = "HTTP"
  vpc_id   = "${aws_vpc.vpc.id}"

  health_check {
    healthy_threshold   = 2
    unhealthy_threshold = 10
    interval            = 30
    timeout             = 5
  }
}

resource  "aws_lb_listener" "concourse_lb_2222" {
  load_balancer_arn = "${aws_lb.concourse_lb.arn}"
  port              = 2222

  default_action {
    type             = "forward"
    target_group_arn = "${aws_lb_target_group.concourse_lb_2222.arn}"
  }
}

resource "aws_lb_target_group" "concourse_lb_2222" {
  name     = "${var.short_env_id}-concourse-lb-2222"
  port     = 2222
  protocol = "HTTP"
  vpc_id   = "${aws_vpc.vpc.id}"
}

resource  "aws_lb_listener" "concourse_lb_443" {
  load_balancer_arn = "${aws_lb.concourse_lb.arn}"
  port              = 443

  default_action {
    type             = "forward"
    target_group_arn = "${aws_lb_target_group.concourse_lb_443.arn}"
  }
}

resource "aws_lb_target_group" "concourse_lb_443" {
  name     = "${var.short_env_id}-concourse-lb-443"
  port     = 8080
  protocol = "HTTP"
  vpc_id   = "${aws_vpc.vpc.id}"
}

output "concourse_lb_name" {
  value = "${aws_lb.concourse_lb.name}"
}

output "concourse_lb_url" {
  value = "${aws_lb.concourse_lb.dns_name}"
}
