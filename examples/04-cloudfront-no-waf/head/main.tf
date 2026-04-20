resource "aws_s3_bucket" "origin" {
  bucket = "static-site-origin"
}

# NEW: CloudFront distribution with no web_acl_id. WAF is missing.
resource "aws_cloudfront_distribution" "site" {
  enabled         = true
  is_ipv6_enabled = true

  origin {
    domain_name = "static-site-origin.s3.amazonaws.com"
    origin_id   = "s3-origin"
  }

  default_cache_behavior {
    target_origin_id       = "s3-origin"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }
}
