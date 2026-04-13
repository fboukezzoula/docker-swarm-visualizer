# Fix for the JSON Output Format

The issue is that `az graph query` returns JSON in a different format. The data is inside a `data` key. Here's the fix:

## Quick Debug Test (CORRECTED)

```bash
# Correct format for az graph query output
az graph query -q "
resources
| where type in ('Microsoft.Storage/storageAccounts', 'Microsoft.KeyVault/vaults')
| project id, name, type, properties
| limit 10
" --subscriptions "YOUR-SUB-ID" -o json | jq '.data[].type' | sort -u
```

## Updated Script (with correct JSON parsing)

```bash
#!/bin/bash
#
# Azure Multi-Tenant Endpoint Export - FIXED VERSION
#

set -euo pipefail

# ============================================
# CONFIGURATION
# ============================================

ENDPOINTS=(
    "blob.core.windows.net"
    "file.core.windows.net"
    "queue.core.windows.net"
    "table.core.windows.net"
    "hcp.westeurope.azmk8s.io"
    "northeurope.azmk8s.io"
    "vault.azure.net"
    "servicebus.windows.net"
    "azureedge.net"
    "azurewebsites.net"
    "graph.microsoft.com"
    "graph.windows.net"
)

OUTPUT_DIR="azure_endpoints_export"
PARALLEL_JOBS=20

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }
error() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2; }

check_az_cli() {
    if ! command -v az &> /dev/null; then error "Azure CLI not installed"; exit 1; fi
    if ! az account show &> /dev/null; then error "Run: az login"; exit 1; fi
    log "Azure CLI OK"
}

init_csv_files() {
    for ep in "${ENDPOINTS[@]}"; do
        echo "tenantid,subscriptionid,fqdn" > "$OUTPUT_DIR/${ep}.csv"
    done
}

get_fqdn() {
    local type=$1 props=$2
    case "$type" in
        "Microsoft.Storage/storageAccounts")
            echo "$props" | jq -r '[.primaryEndpoints.blob, .primaryEndpoints.file, .primaryEndpoints.queue, .primaryEndpoints.table, .primaryEndpoints.dfs][] | select(. != null) | gsub("https://"; "") | gsub("/"; "")' 2>/dev/null
            ;;
        "Microsoft.KeyVault/vaults")
            echo "$props" | jq -r '.vaultUri // ""' | sed 's|https://||;s|/||'
            ;;
        "Microsoft.ContainerService/managedClusters")
            echo "$props" | jq -r '.fqdn // (.agentPoolProfiles[]?.fqdn // "")'
            ;;
        "Microsoft.ServiceBus/namespaces")
            echo "$props" | jq -r '.serviceBusEndpoint // ""' | sed 's|https://||;s|/||'
            ;;
        "Microsoft.Web/sites"|"Microsoft.Web/staticsites")
            echo "$props" | jq -r '.defaultHostName // .hostName // ""'
            ;;
        "Microsoft.PowerPlatform/accounts")
            echo "$props" | jq -r '.domain // ""'
            ;;
    esac
}

export -f get_fqdn
export ENDPOINTS

export_sub() {
    local tid=$1 sid=$2
    
    # Get data using .data[] because az graph returns {data: [...], count: N, ...}
    local result
    result=$(az graph query -q "
        resources | where type in (
            'Microsoft.Storage/storageAccounts',
            'Microsoft.KeyVault/vaults',
            'Microsoft.ContainerService/managedClusters',
            'Microsoft.ServiceBus/namespaces',
            'Microsoft.Web/sites',
            'Microsoft.Web/staticsites',
            'Microsoft.PowerPlatform/accounts'
        ) | project id, name, type, properties | limit 5000
    " --subscriptions "$sid" -o json 2>/dev/null || echo '{"data":[]}')
    
    # Process .data[] instead of direct array
    echo "$result" | jq -r --arg t "$tid" '.data[] | "\($t)|\(.type)|\(.properties | tojson))"' 2>/dev/null | \
    while IFS='|' read -r t tp p; do
        # Skip empty lines
        [[ -z "$t" || -z "$tp" ]] && continue
        
        for fqdn in $(get_fqdn "$tp" "$p"); do
            [[ -n "$fqdn" && "$fqdn" != "null" ]] && echo "$tid,$sid,$fqdn"
        done
    done
}

export -f export_sub

main() {
    log "=== Azure Multi-Tenant Export ==="
    mkdir -p "$OUTPUT_DIR"
    check_az_cli
    init_csv_files
    
    # Get all tenants
    tenants=$(az account list --all -o json | jq -r '.[].tenantId')
    [[ -z "$tenants" ]] && error "No tenants" && exit 1
    
    tenant_count=$(echo "$tenants" | wc -l)
    log "Found $tenant_count tenants"
    
    # Get all subscriptions
    all_subs=""
    for t in $tenants; do
        subs=$(az account list --all --tenant "$t" -o json 2>/dev/null | jq -r --arg t "$t" '.[] | "\($t)|\(.id)"')
        all_subs+="$subs"$'\n'
    done
    
    sub_count=$(echo "$all_subs" | grep -v '^$' | wc -l)
    log "Found $sub_count subscriptions"
    
    # Show first few subscriptions for debugging
    log "Sample subscriptions:"
    echo "$all_subs" | grep -v '^$' | head -3
    
    # Collect results
    temp="$OUTPUT_DIR/tmp.txt"
    > "$temp"
    
    log "Starting parallel export ($PARALLEL_JOBS jobs)..."
    echo "$all_subs" | grep -v '^$' | xargs -P $PARALLEL_JOBS -I {} bash -c 'export_sub "$@"' _ {} >> "$temp" 2>&1 || true
    
    result_count=$(wc -l < "$temp")
    log "Raw results collected: $result_count lines"
    
    # Debug: show first few raw results
    if [[ $result_count -gt 0 ]]; then
        log "First raw results:"
        head -5 "$temp"
    fi
    
    # Write to CSVs
    log "Distributing to CSV files..."
    while IFS=',' read -r tid sid fqdn; do
        [[ -z "$tid" || -z "$sid" || -z "$fqdn" ]] && continue
        for ep in "${ENDPOINTS[@]}"; do
            if echo "$fqdn" | grep -qi "$ep"; then
                echo "$tid,$sid,$fqdn" >> "$OUTPUT_DIR/${ep}.csv"
            fi
        done
    done < "$temp"
    rm -f "$temp"
    
    # Deduplicate
    for ep in "${ENDPOINTS[@]}"; do
        [[ -f "$OUTPUT_DIR/${ep}.csv" ]] && {
            head -1 "$OUTPUT_DIR/${ep}.csv" > "$OUTPUT_DIR/${ep}.tmp"
            tail -n +2 "$OUTPUT_DIR/${ep}.csv" | sort -u >> "$OUTPUT_DIR/${ep}.tmp"
            mv "$OUTPUT_DIR/${ep}.tmp" "$OUTPUT_DIR/${ep}.csv"
        }
    done
    
    log "=== COMPLETE ==="
    for ep in "${ENDPOINTS[@]}"; do
        [[ -f "$OUTPUT_DIR/${ep}.csv" ]] && log "  $ep.csv: $(($(wc -l < "$OUTPUT_DIR/${ep}.csv") - 1))"
    done
}

main "$@"
```

## Key Fix

| Before (wrong) | After (correct) |
|----------------|-----------------|
| `jq '.[].type'` | `jq '.data[].type'` |
| `jq '.[] \| ...'` | `jq '.data[] \| ...'` |

The Azure Resource Graph API returns:
```json
{
  "data": [...],
  "count": 123,
  "skipToken": "..."
}
```

Not a direct array!

## Quick Verification

```bash
# Test on one subscription first
az graph query -q "
resources
| where type == 'Microsoft.Storage/storageAccounts'
| project name, properties.primaryEndpoints.blob
| limit 3
" --subscriptions "YOUR-SUB-ID" -o json | jq '.data'
```

This should return actual data now. Try running the updated script! 🚀
