# 04 — CloudFront distribution added without a WAF

A team puts a CloudFront distribution in front of an S3 origin but forgets to attach a `web_acl_id`. The internet now sees the origin without WAF protection.

## Expected output

- `new_entry_point` (3.0) — CloudFront is an entry point
- `cloudfront_no_waf` (2.5) — distribution has no WAF

**Total: 5.5 / 10 — `medium` / `warn`**

## Run

```bash
./architex report ./examples/04-cloudfront-no-waf/base ./examples/04-cloudfront-no-waf/head --out ./.architex/
```
