---
name: False positive
about: A rule fired but shouldn't have
title: "[false-positive] "
labels: false-positive
---

## Which rule fired

<!-- e.g. public_exposure_introduced, new_data_resource, new_entry_point, potential_data_exposure, resource_removed -->

Rule ID: ``

## What the PR actually does

<!-- Describe the infrastructure change. Why is the rule wrong in this case? -->

## Terraform snippet

```hcl
# Minimal .tf that reproduces the false positive
```

## ArchiteX output

```
# Paste the risk score and triggered rule from the PR comment or CLI output
```

## Suggested fix

<!-- Optional: how should the rule behave instead? -->
