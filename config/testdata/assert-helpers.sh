#!/usr/bin/env bash
# Shared assertion helpers for local-e2e.sh test sections.
# Source this file before using the functions below.

# assert_label_absent <namespace> <secret> <label-key>
# Fails if the label key is present (any value) on the Secret.
assert_label_absent() {
  local ns=$1 secret=$2 key=$3
  local val
  val=$(kubectl -n "$ns" get secret "$secret" \
        -o jsonpath="{.metadata.labels.$key}" 2>/dev/null || true)
  if [[ -n "$val" ]]; then
    echo "FAIL: secret $secret in $ns has unexpected label $key=$val" >&2
    exit 1
  fi
}

# assert_label_present <namespace> <secret> <label-key> <expected-value>
# Fails if the label key is absent or has a different value on the Secret.
assert_label_present() {
  local ns=$1 secret=$2 key=$3 expected=$4
  local val
  val=$(kubectl -n "$ns" get secret "$secret" \
        -o jsonpath="{.metadata.labels.$key}" 2>/dev/null || true)
  if [[ "$val" != "$expected" ]]; then
    echo "FAIL: secret $secret in $ns: label $key expected '$expected', got '$val'" >&2
    exit 1
  fi
}
