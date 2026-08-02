provider "aws" {
  region = var.aws_region
}

# 1. AWS SQS Queue & Dead-Letter Queue (DLQ)
resource "aws_sqs_queue" "dlq" {
  name                      = "metastackr-cascade-dlq"
  message_retention_seconds = 1209600 # 14 days
}

resource "aws_sqs_queue" "queue" {
  name                      = "metastackr-cascade-queue"
  delay_seconds             = 0
  max_message_size          = 262144
  message_retention_seconds = 345600 # 4 days
  receive_wait_time_seconds = 10
  visibility_timeout_seconds = 30

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq.arn
    maxReceiveCount     = 5
  })
}

# 2. IAM Role for Lambda
resource "aws_iam_role" "lambda_exec" {
  name = "metastackr-lambda-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
    }]
  })
}

# IAM Policies: SQS Access and Logging
resource "aws_iam_role_policy_attachment" "lambda_logs" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_policy" "sqs_policy" {
  name = "metastackr-lambda-sqs-policy"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes",
          "sqs:SendMessage"
        ]
        Resource = [
          aws_sqs_queue.queue.arn,
          aws_sqs_queue.dlq.arn
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_sqs" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.sqs_policy.arn
}

# 3. Go Backend Lambda Function (ARM64 Graviton under AL2023)
resource "aws_lambda_function" "backend" {
  function_name    = "metastackr-backend"
  role             = aws_iam_role.lambda_exec.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  memory_size      = 256
  timeout          = 15
  filename         = "bootstrap.zip"
  source_code_hash = fileexists("bootstrap.zip") ? filebase64sha256("bootstrap.zip") : null

  environment {
    variables = {
      DATABASE_URL = var.database_url
      ENVIRONMENT  = "production"
      SQS_QUEUE_URL = aws_sqs_queue.queue.id
    }
  }
}

# Lambda SQS trigger integration
resource "aws_lambda_event_source_mapping" "sqs_trigger" {
  event_source_arn = aws_sqs_queue.queue.arn
  function_name    = aws_lambda_function.backend.arn
  batch_size       = 10
}

# 4. AWS HTTP API Gateway v2
resource "aws_apigatewayv2_api" "http_api" {
  name          = "metastackr-http-api"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "lambda" {
  api_id                 = aws_apigatewayv2_api.http_api.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.backend.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "any_route" {
  api_id    = aws_apigatewayv2_api.http_api.id
  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.lambda.id}"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.http_api.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "apigw" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.backend.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.http_api.execution_arn}/*/*"
}

# 5. ACM SSL Certificate for api.metastac.kr
resource "aws_acm_certificate" "cert" {
  domain_name       = "api.metastac.kr"
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

# Route 53 Hosted Zone lookup
data "aws_route53_zone" "primary" {
  name         = "metastac.kr."
  private_zone = false
}

# Route 53 Record for certificate validation
resource "aws_route53_record" "cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.cert.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.primary.zone_id
}

# ACM Certificate Validation trigger
resource "aws_acm_certificate_validation" "cert" {
  certificate_arn         = aws_acm_certificate.cert.arn
  validation_record_fqdns = [for record in aws_route53_record.cert_validation : record.fqdn]
}

# API Gateway v2 Custom Domain Name
resource "aws_apigatewayv2_domain_name" "custom_domain" {
  domain_name = "api.metastac.kr"

  domain_name_configuration {
    certificate_arn = aws_acm_certificate_validation.cert.certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

# Mapping between Custom Domain and HTTP API Gateway
resource "aws_apigatewayv2_api_mapping" "mapping" {
  api_id      = aws_apigatewayv2_api.http_api.id
  domain_name = aws_apigatewayv2_domain_name.custom_domain.id
  stage       = aws_apigatewayv2_stage.default.id
}

# Route 53 Record for Custom Domain pointing to API Gateway
resource "aws_route53_record" "api" {
  name    = aws_apigatewayv2_domain_name.custom_domain.domain_name
  type    = "A"
  zone_id = data.aws_route53_zone.primary.zone_id

  alias {
    name                   = aws_apigatewayv2_domain_name.custom_domain.domain_name_configuration[0].target_domain_name
    zone_id                = aws_apigatewayv2_domain_name.custom_domain.domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = false
  }
}

# 6. GitHub Pages DNS Settings
# Apex Domain A-records pointing to GitHub Pages CDN IPs
resource "aws_route53_record" "github_pages_apex" {
  zone_id = data.aws_route53_zone.primary.zone_id
  name    = "" # Root apex
  type    = "A"
  ttl     = 3600
  records = [
    "185.199.108.153",
    "185.199.109.153",
    "185.199.110.153",
    "185.199.111.153"
  ]
}

# Subdomain CNAME pointing www to GitHub Pages CDN
resource "aws_route53_record" "github_pages_www" {
  zone_id = data.aws_route53_zone.primary.zone_id
  name    = "www"
  type    = "CNAME"
  ttl     = 3600
  records = ["eliotstocker.github.io"]
}


